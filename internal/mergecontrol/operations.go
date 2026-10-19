package mergecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/privateconnector"
	"github.com/reforgeapp/reforge/internal/source"
)

func gateTx(ctx context.Context, tx pgx.Tx, org, id string) (Gate, error) {
	var out Gate
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT document FROM merge_gates WHERE org_id=$1 AND id=$2`, org, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, auth.ErrForbidden
	}
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}

func operationTx(ctx context.Context, tx pgx.Tx, org, id string) (Operation, error) {
	return scanOperation(tx.QueryRow(ctx, `SELECT id::text,repository_id::text,gate_id::text,requested_gate_id::text,change_id,state,reason,cancel_requested,native_result,version,created_at,updated_at,native_queue_id FROM merge_operations WHERE org_id=$1 AND id=$2`, org, id))
}

func scanOperation(row interface{ Scan(...any) error }) (Operation, error) {
	var out Operation
	var raw []byte
	err := row.Scan(&out.ID, &out.RepositoryID, &out.GateID, &out.RequestedGateID, &out.ChangeID, &out.State, &out.Reason, &out.CancelRequested, &raw, &out.Version, &out.CreatedAt, &out.UpdatedAt, &out.NativeQueueID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, auth.ErrForbidden
	}
	if err == nil && len(raw) > 0 {
		err = json.Unmarshal(raw, &out.NativeResult)
	}
	return out, err
}

func (s *Service) List(ctx context.Context, session auth.Session, org, repo, change, cursor string, limit int) (domain.Page[Operation], error) {
	out := domain.Page[Operation]{Items: []Operation{}, Complete: true}
	if !auth.ValidID(repo) || len(change) > 100 || cursor != "" && !auth.ValidID(cursor) || limit < 1 || limit > 100 {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		if _, _, err := repository(ctx, tx, org, repo); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text,repository_id::text,gate_id::text,requested_gate_id::text,change_id,state,reason,cancel_requested,native_result,version,created_at,updated_at,native_queue_id FROM merge_operations WHERE org_id=$1 AND repository_id=$2 AND ($3='' OR change_id=$3) AND ($4='' OR (created_at,id)<(SELECT created_at,id FROM merge_operations WHERE org_id=$1 AND repository_id=$2 AND id::text=$4)) ORDER BY created_at DESC,id DESC LIMIT $5`, org, repo, change, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanOperation(rows)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, item)
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			out.NextCursor = out.Items[limit-1].ID
			out.Complete = false
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) Get(ctx context.Context, session auth.Session, org, id string) (Operation, error) {
	var out Operation
	if !auth.ValidID(id) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !auth.CanReadRepository(a, out.RepositoryID) {
			return auth.ErrForbidden
		}
		return nil
	})
	return out, err
}

func (s *Service) Request(ctx context.Context, session auth.Session, org, gateID, key, request string) (Operation, error) {
	var out Operation
	if !auth.ValidID(gateID) || !auth.ValidID(key) {
		return out, auth.ErrInvalid
	}
	var original Gate
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		original, err = gateTx(ctx, tx, org, gateID)
		if err != nil {
			return err
		}
		if !manage(a, original.RepositoryID) {
			return auth.ErrForbidden
		}
		var id, previousGate string
		err = tx.QueryRow(ctx, `SELECT id::text,requested_gate_id::text FROM merge_operations WHERE org_id=$1 AND repository_id=$2 AND idempotency_key=$3`, org, original.RepositoryID, key).Scan(&id, &previousGate)
		if err == nil {
			if previousGate != gateID {
				return auth.ErrConflict
			}
			out, err = operationTx(ctx, tx, org, id)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if original.Decision.Outcome != "allow" || !original.ExpiresAt.After(time.Now()) {
			return auth.ErrConflict
		}
		return nil
	})
	if err != nil || out.ID != "" {
		return out, err
	}
	fresh, err := s.Inspect(ctx, session, org, original.RepositoryID, original.Snapshot.Change.ID, original.Method, request)
	if err != nil {
		return out, err
	}
	if fresh.Phase == "queue_execution" || fresh.Decision.Outcome != "allow" || fresh.Binding != original.Binding || fresh.ConfigurationVersion != original.ConfigurationVersion || fresh.ConnectionVersion != original.ConnectionVersion || !companionsEqual(fresh.Companions, original.Companions) {
		return out, auth.ErrConflict
	}
	created := false
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, fresh.RepositoryID) {
			return auth.ErrForbidden
		}
		if err := s.validateGateTx(ctx, tx, org, fresh, a); err != nil {
			return err
		}
		var existing, previousGate string
		err := tx.QueryRow(ctx, `SELECT id::text,requested_gate_id::text FROM merge_operations WHERE org_id=$1 AND repository_id=$2 AND idempotency_key=$3`, org, fresh.RepositoryID, key).Scan(&existing, &previousGate)
		if err == nil {
			if previousGate != gateID {
				return auth.ErrConflict
			}
			out, err = operationTx(ctx, tx, org, existing)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		id := domain.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO merge_operations(org_id,id,repository_id,gate_id,requested_gate_id,change_id,target_branch,idempotency_key,requested_by,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'requested')`, org, id, fresh.RepositoryID, fresh.ID, gateID, fresh.Snapshot.Change.ID, fresh.Snapshot.Change.TargetBranch, key, a.UserID)
		if err != nil {
			return err
		}
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		created = true
		return emit(ctx, tx, org, out.RepositoryID, a.UserID, "merge.requested", id, out.Version, request, map[string]any{"gate_id": fresh.ID})
	})
	if err != nil || !created {
		return out, err
	}
	if fresh.Phase == "queue_admission" && fresh.Snapshot.Capabilities.Provider == "github" {
		ready, err := s.publishQueueCheck(ctx, &session, org, out, fresh)
		if err != nil || !ready {
			return out, err
		}
	}
	return s.dispatch(ctx, session, org, out, fresh, request)
}

func (s *Service) validateGateTx(ctx context.Context, tx pgx.Tx, org string, gate Gate, actor domain.Actor) error {
	if !gate.ExpiresAt.After(time.Now()) || gate.Decision.Outcome != "allow" {
		return auth.ErrConflict
	}
	_, connectionID, err := repository(ctx, tx, org, gate.RepositoryID)
	if err != nil {
		return err
	}
	if connectionID != gate.ConnectionID {
		return auth.ErrConflict
	}
	c, err := s.connections.MetadataTx(ctx, tx, org, gate.ConnectionID)
	if err != nil {
		return err
	}
	if c.State != "healthy" || c.Version != gate.ConnectionVersion {
		return auth.ErrConflict
	}
	cfg, err := configTx(ctx, tx, org, gate.RepositoryID)
	if err != nil {
		return err
	}
	if cfg.Version != gate.ConfigurationVersion {
		return auth.ErrConflict
	}
	resolved, err := s.policies.ResolveTx(ctx, tx, org, gate.RepositoryID)
	if err != nil {
		return err
	}
	if resolved.Hash != gate.Binding.PolicyHash {
		return auth.ErrConflict
	}
	authority, err := s.authorityTx(ctx, tx, org, gate.RepositoryID, cfg, c, gate.Snapshot, actor)
	if err != nil {
		return err
	}
	authority.Paths, authority.PathsVerified = gate.Paths, true
	validCompanions, err := validateCompanionsTx(ctx, tx, org, gate)
	if err != nil {
		return err
	}
	authority.CompanionsBlocked = !validCompanions
	files := int64(len(gate.Paths))
	authority.Usage.ChangedFiles, authority.Usage.ChangedLines = &files, &gate.ChangedLines
	if current := Evaluate(gate.Snapshot, resolved, gate.Method, authority, time.Now().UTC()); current.Decision.Outcome != "allow" || current.ReforgeEnforced != gate.ReforgeEnforced {
		return auth.ErrForbidden
	}
	return nil
}

func (s *Service) dispatch(ctx context.Context, session auth.Session, org string, operation Operation, gate Gate, request string) (Operation, error) {
	var cfg Configuration
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		cfg, err = configTx(ctx, tx, org, operation.RepositoryID)
		return err
	})
	if err != nil {
		return operation, err
	}
	current := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		a, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !manage(a, operation.RepositoryID) {
			return auth.ErrForbidden
		}
		op, err := operationTx(ctx, tx, org, operation.ID)
		if err != nil {
			return err
		}
		if op.CancelRequested || op.State != "dispatching" || c.ID != gate.ConnectionID || c.Version != gate.ConnectionVersion {
			return auth.ErrConflict
		}
		return s.validateGateTx(ctx, tx, org, gate, a)
	}
	authorize := func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
		a, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return "", err
		}
		if !manage(a, operation.RepositoryID) {
			return "", auth.ErrForbidden
		}
		op, err := operationTx(ctx, tx, org, operation.ID)
		if err != nil {
			return "", err
		}
		if op.State != "requested" || op.CancelRequested {
			return "", auth.ErrConflict
		}
		if err = s.validateGateTx(ctx, tx, org, gate, a); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `UPDATE merge_operations SET state='dispatching',version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, op.ID); err != nil {
			return "", err
		}
		return op.ID, nil
	}
	op := privateconnector.Operation{ID: operation.ID, Kind: privateconnector.ForgeMerge, Merge: &forge.MergeRequest{Repository: gate.Snapshot.Change.Repository, ChangeID: gate.Snapshot.Change.ID, ExpectedHeadSHA: gate.Binding.Head, ExpectedTargetSHA: gate.Binding.Target, RulesHash: gate.Binding.ProviderRules, GateID: gate.ID, Method: gate.Method, OperationID: operation.ID, Queue: gate.Snapshot.Rules.RequireQueue, ReforgeEnforced: gate.ReforgeEnforced}}
	result, writeErr := s.providers.ForProtection(cfg.InspectorConnectionID, cfg.CheckPublishers).Write(ctx, org, gate.ConnectionID, op, authorize, current)
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err = s.db.Tenant(persist, org, "", func(tx pgx.Tx) error {
		if err := lock(persist, tx, org); err != nil {
			return err
		}
		stored, err := operationTx(persist, tx, org, operation.ID)
		if err != nil {
			return err
		}
		if stored.State == "cancelled" {
			operation = stored
			return nil
		}
		state, reason := "reconciling", "Native outcome requires reconciliation before any further mutation"
		if stored.State == "requested" {
			state, reason = "blocked", "Fresh authority rejected before native dispatch"
		}
		if writeErr == nil && result.Merge != nil {
			state, reason = observedState(*result.Merge, gate)
		}
		var raw []byte
		if result.Merge != nil {
			raw, _ = json.Marshal(result.Merge)
		}
		queueID := ""
		if result.Merge != nil && result.Merge.State == "queued" && result.Merge.HeadSHA == gate.Binding.Head {
			queueID = result.Merge.NativeID
		}
		if _, err = tx.Exec(persist, `UPDATE merge_operations SET state=$3,reason=$4,native_result=$5,native_queue_id=$6,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, operation.ID, state, reason, raw, queueID); err != nil {
			return err
		}
		operation, err = operationTx(persist, tx, org, operation.ID)
		if err != nil {
			return err
		}
		var actor string
		if err = tx.QueryRow(persist, `SELECT requested_by::text FROM merge_operations WHERE org_id=$1 AND id=$2`, org, operation.ID).Scan(&actor); err != nil {
			return err
		}
		return emit(persist, tx, org, operation.RepositoryID, actor, "merge."+state, operation.ID, operation.Version, request, map[string]any{"state": state, "cancel_requested": operation.CancelRequested})
	})
	return operation, err
}

func observedState(result forge.MergeResult, gate Gate) (string, string) {
	if result.HeadSHA != gate.Binding.Head {
		return "reconciling", "Native change head differs from the requested candidate"
	}
	if result.State == "merged" && source.ValidSHA(result.MergeSHA, "sha1") && result.NativeID == gate.Snapshot.Change.ID {
		return "merged", "Canonical provider merge observed"
	}
	if result.State == "queued" {
		return "queued", "Native admission observed; cancellation may race with execution"
	}
	if result.State == "closed" {
		return "closed", "Native change is closed without merge"
	}
	return "reconciling", "Native completion is not yet authoritative"
}
