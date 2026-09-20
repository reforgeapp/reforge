package mergecontrol

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/privateconnector"
)

func (s *Service) validateQueueEffectTx(ctx context.Context, tx pgx.Tx, session *auth.Session, org string, expected Operation, gate Gate) error {
	actor, err := s.inspectionActor(ctx, tx, session, org, expected.RepositoryID, expected.ID)
	if err != nil {
		return err
	}
	if !manage(actor, expected.RepositoryID) {
		return auth.ErrForbidden
	}
	op, err := operationTx(ctx, tx, org, expected.ID)
	if err != nil {
		return err
	}
	if op.Version != expected.Version || op.CancelRequested || gate.Phase != "queue_admission" && gate.Phase != "queue_execution" || gate.Phase == "queue_admission" && op.State != "requested" || gate.Phase == "queue_execution" && (op.State != "queued" || op.NativeQueueID == "" || op.NativeQueueID != gate.Snapshot.Queue.ID) {
		return auth.ErrConflict
	}
	original, err := gateTx(ctx, tx, org, op.GateID)
	if err != nil {
		return err
	}
	if !sameQueueAuthority(original, gate) {
		return auth.ErrConflict
	}
	return s.validateGateTx(ctx, tx, org, gate)
}

func sameQueueAuthority(original, fresh Gate) bool {
	a, b := original.Binding, fresh.Binding
	a.Tested, b.Tested = "", ""
	return original.RepositoryID == fresh.RepositoryID && original.Snapshot.Change.ID == fresh.Snapshot.Change.ID && original.Method == fresh.Method && original.ConfigurationVersion == fresh.ConfigurationVersion && original.ConnectionID == fresh.ConnectionID && original.ConnectionVersion == fresh.ConnectionVersion && a == b && slices.Equal(original.Companions, fresh.Companions)
}

func (s *Service) publishQueueCheck(ctx context.Context, session *auth.Session, org string, operation Operation, gate Gate) (bool, error) {
	var id, state, nativeID string
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lock(ctx, tx, org); err != nil {
			return err
		}
		if err := s.validateQueueEffectTx(ctx, tx, session, org, operation, gate); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO merge_execution_checks(org_id,id,operation_id,gate_id,sha,phase,state) VALUES($1,$2,$3,$4,$5,$6,'prepared') ON CONFLICT(org_id,operation_id,sha) DO NOTHING`, org, domain.NewID(), operation.ID, gate.ID, gate.Binding.Tested, gate.Phase); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id::text,state,native_id FROM merge_execution_checks WHERE org_id=$1 AND operation_id=$2 AND sha=$3 AND phase=$4`, org, operation.ID, gate.Binding.Tested, gate.Phase).Scan(&id, &state, &nativeID)
	})
	if err != nil {
		return false, err
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		if c.ID != gate.ConnectionID || c.Version != gate.ConnectionVersion {
			return auth.ErrConflict
		}
		return s.validateQueueEffectTx(ctx, tx, session, org, operation, gate)
	}
	request := forge.ExecutionCheckRequest{Repository: gate.Snapshot.Change.Repository, SHA: gate.Binding.Tested, Name: forge.QueueExecutionCheckName, State: "success", OperationID: id, CheckID: nativeID}
	valid := func(value *forge.ExecutionCheck) bool {
		return value != nil && value.ID != "" && value.SHA == request.SHA && value.Name == request.Name && value.State == "success" && value.OperationID == id && value.PublisherID == gate.Snapshot.ExecutionCheck.PublisherID
	}
	if state == "confirmed" {
		result, err := s.providers.Read(ctx, org, gate.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadExecutionCheck, ExecutionCheck: &request}, check)
		return err == nil && valid(result.ExecutionCheck) && result.ExecutionCheck.ID == nativeID, err
	}
	if state != "prepared" {
		return false, privateconnector.ErrUncertain
	}
	op := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeWriteExecutionCheck, ExecutionCheck: &request}
	result, writeErr := s.providers.Write(ctx, org, gate.ConnectionID, op, func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
		if err := check(ctx, tx, c); err != nil {
			return "", err
		}
		changed, err := tx.Exec(ctx, `UPDATE merge_execution_checks SET state='dispatching',dispatch_id=$3,updated_at=now() WHERE org_id=$1 AND id=$2 AND state='prepared'`, org, id, op.ID)
		if err == nil && changed.RowsAffected() != 1 {
			err = auth.ErrConflict
		}
		if err == nil {
			actor := ""
			if session != nil {
				actor = session.User.ID
			}
			err = emit(ctx, tx, org, operation.RepositoryID, actor, "merge.execution_check.dispatched", id, 1, "", map[string]any{"operation_id": operation.ID, "gate_id": gate.ID, "sha": request.SHA, "phase": gate.Phase})
		}
		return operation.ID, err
	}, func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		if err := check(ctx, tx, c); err != nil {
			return err
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT state='dispatching' AND dispatch_id=$3 FROM merge_execution_checks WHERE org_id=$1 AND id=$2`, org, id, op.ID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return auth.ErrConflict
		}
		return nil
	})
	confirmed := writeErr == nil && valid(result.ExecutionCheck)
	next, checkID := "uncertain", ""
	if confirmed {
		next, checkID = "confirmed", result.ExecutionCheck.ID
	}
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err = s.db.Tenant(persist, org, "", func(tx pgx.Tx) error {
		changed, err := tx.Exec(persist, `UPDATE merge_execution_checks SET state=$3,native_id=$4,updated_at=now() WHERE org_id=$1 AND id=$2 AND state='dispatching' AND dispatch_id=$5`, org, id, next, checkID, op.ID)
		if err != nil || changed.RowsAffected() == 0 {
			return err
		}
		return emit(persist, tx, org, operation.RepositoryID, "", "merge.execution_check."+next, id, 2, "", map[string]any{"operation_id": operation.ID, "native_id": checkID, "sha": request.SHA, "phase": gate.Phase})
	})
	if err != nil {
		return false, err
	}
	if !confirmed {
		return false, privateconnector.ErrUncertain
	}
	return true, nil
}
