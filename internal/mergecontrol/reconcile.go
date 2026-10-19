package mergecontrol

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/privateconnector"
)

func (s *Service) Reconcile(ctx context.Context, session auth.Session, org, id string, expected int64, request string) (Operation, error) {
	var out Operation
	var gate Gate
	if !auth.ValidID(id) || expected < 1 {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !manage(a, out.RepositoryID) {
			return auth.ErrForbidden
		}
		if out.Version != expected {
			return auth.ErrConflict
		}
		gate, err = gateTx(ctx, tx, org, out.GateID)
		return err
	})
	if err != nil {
		return out, err
	}
	if out.State == "merged" || out.State == "closed" || out.State == "cancelled" || out.State == "blocked" {
		return out, nil
	}
	result, err := s.providers.Read(ctx, org, gate.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeMergeResult, Change: &privateconnector.ChangeArgs{Repository: gate.Snapshot.Change.Repository, ChangeID: gate.Snapshot.Change.ID}}, func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		a, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !manage(a, out.RepositoryID) {
			return auth.ErrForbidden
		}
		ref, connection, err := repository(ctx, tx, org, out.RepositoryID)
		if err != nil {
			return err
		}
		if c.ID != connection || ref != gate.Snapshot.Change.Repository {
			return auth.ErrConflict
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	if result.Merge == nil {
		return out, privateconnector.ErrInvalid
	}
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, out.RepositoryID) {
			return auth.ErrForbidden
		}
		current, err := operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if current.Version != expected {
			return auth.ErrConflict
		}
		state, reason := observedState(*result.Merge, gate)
		if current.State == "requested" && state == "reconciling" {
			state, reason = "blocked", "Controller stopped before dispatch; create a fresh merge preview"
		}
		if current.CancelRequested && state == "merged" {
			reason = "Cancellation lost the race; canonical native merge observed"
		}
		raw, _ := json.Marshal(result.Merge)
		if _, err = tx.Exec(ctx, `UPDATE merge_operations SET state=$3,reason=$4,native_result=$5,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, state, reason, raw); err != nil {
			return err
		}
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		return emit(ctx, tx, org, out.RepositoryID, a.UserID, "merge."+state, id, out.Version, request, map[string]any{"state": state, "cancel_requested": out.CancelRequested})
	})
	return out, err
}

func (s *Service) Cancel(ctx context.Context, session auth.Session, org, id string, expected int64, request string) (Operation, error) {
	var out Operation
	if !auth.ValidID(id) || expected < 1 {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !manage(a, out.RepositoryID) {
			return auth.ErrForbidden
		}
		if out.Version != expected {
			return auth.ErrConflict
		}
		if out.State == "merged" || out.State == "closed" || out.State == "cancelled" {
			return nil
		}
		state, reason := "reconciling", "Cancellation requested; reconcile the native outcome. Native admission may already have executed"
		if out.State == "requested" || out.State == "blocked" {
			state, reason = "cancelled", "Cancelled before native dispatch"
		}
		if _, err = tx.Exec(ctx, `UPDATE merge_operations SET state=$3,reason=$4,cancel_requested=true,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, state, reason); err != nil {
			return err
		}
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		return emit(ctx, tx, org, out.RepositoryID, a.UserID, "merge.cancellation_requested", id, out.Version, request, map[string]any{"state": state})
	})
	if err == nil && out.CancelRequested && out.State == "reconciling" && out.NativeQueueID != "" {
		return s.cancelNative(ctx, &session, org, out, request)
	}
	return out, err
}

func (s *Service) cancelNative(ctx context.Context, session *auth.Session, org string, operation Operation, request string) (Operation, error) {
	var gate Gate
	actorID := ""
	if session != nil {
		actorID = session.User.ID
	}
	checkActor := func(ctx context.Context, tx pgx.Tx) error {
		if session != nil {
			actor, err := s.auth.ActorTx(ctx, tx, *session, org)
			if err != nil {
				return err
			}
			if !manage(actor, operation.RepositoryID) {
				return auth.ErrForbidden
			}
		}
		return nil
	}
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := checkActor(ctx, tx); err != nil {
			return err
		}
		var err error
		gate, err = gateTx(ctx, tx, org, operation.GateID)
		return err
	})
	if err != nil {
		return operation, err
	}
	check := func(ctx context.Context, tx pgx.Tx, connection connections.Connection) error {
		if err := checkActor(ctx, tx); err != nil {
			return err
		}
		current, err := operationTx(ctx, tx, org, operation.ID)
		if err != nil {
			return err
		}
		ref, id, err := repository(ctx, tx, org, operation.RepositoryID)
		if err != nil {
			return err
		}
		if current.Version != operation.Version || current.State != "reconciling" || !current.CancelRequested || current.NativeQueueID != operation.NativeQueueID || connection.ID != id || id != gate.ConnectionID || ref != gate.Snapshot.Change.Repository {
			return auth.ErrConflict
		}
		return nil
	}
	id := domain.NewID()
	result, writeErr := s.providers.Write(ctx, org, gate.ConnectionID, privateconnector.Operation{ID: id, Kind: privateconnector.ForgeCancelQueue, CancelQueue: &forge.QueueCancelRequest{Repository: gate.Snapshot.Change.Repository, ChangeID: operation.ChangeID, QueueID: operation.NativeQueueID, ExpectedHeadSHA: gate.Binding.Head, OperationID: id}}, func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
		return operation.ID, check(ctx, tx, c)
	}, check)
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err = s.db.Tenant(persist, org, "", func(tx pgx.Tx) error {
		if err := lock(persist, tx, org); err != nil {
			return err
		}
		current, err := operationTx(persist, tx, org, operation.ID)
		if err != nil {
			return err
		}
		if current.Version != operation.Version {
			operation = current
			return nil
		}
		state, reason := "reconciling", "Native queue cancellation is unconfirmed; reconcile the provider outcome before retrying"
		if writeErr == nil && result.Queue != nil && result.Queue.ID == "" && result.Queue.State == "not_queued" && result.Queue.HeadSHA == gate.Binding.Head {
			state, reason = "cancelled", "Native queue removal and unchanged open change observed; subsequent native actions remain possible"
		}
		if _, err = tx.Exec(persist, `UPDATE merge_operations SET state=$3,reason=$4,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, operation.ID, state, reason); err != nil {
			return err
		}
		operation, err = operationTx(persist, tx, org, operation.ID)
		if err != nil {
			return err
		}
		return emit(persist, tx, org, operation.RepositoryID, actorID, "merge.queue_cancellation", operation.ID, operation.Version, request, map[string]any{"state": state})
	})
	return operation, err
}
