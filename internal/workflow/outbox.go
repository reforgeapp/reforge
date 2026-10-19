package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
)

func checkUncertain(ctx context.Context, tx pgx.Tx, orgID, taskID string) error {
	var unknown bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_outbox WHERE org_id=$1 AND task_id=$2 AND state IN ('dispatching','unknown'))`, orgID, taskID).Scan(&unknown); err != nil {
		return err
	}
	if unknown {
		return ErrReconciliation
	}
	return nil
}

const intentColumns = `id::text,org_id::text,task_id::text,repository_id::text,operation_id::text,kind,state,payload,dispatch_count,evidence`

func scanIntent(row pgx.Row) (Intent, error) {
	var i Intent
	err := row.Scan(&i.ID, &i.OrgID, &i.TaskID, &i.RepositoryID, &i.OperationID, &i.Kind, &i.State, &i.Payload, &i.DispatchCount, &i.Evidence)
	return i, hidden(err)
}
func (s *Service) PrepareIntentTx(ctx context.Context, tx pgx.Tx, l Lease, kind, key string, payload json.RawMessage) (Intent, error) {
	var result Intent
	if kind != "stage" && kind != "publish" && kind != "merge" && kind != "deploy" && kind != "recover" {
		return result, auth.ErrInvalid
	}
	if len(key) < 1 || len(key) > 200 || len(payload) > 65536 || !json.Valid(payload) {
		return result, auth.ErrInvalid
	}
	task, err := s.ValidateFenceTx(ctx, tx, l, kind)
	if err != nil {
		return result, err
	}
	var canonical any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err = decoder.Decode(&canonical); err != nil {
		return result, auth.ErrInvalid
	}
	payload = mustJSON(canonical)
	hash := digest(payload)
	var oldHash string
	err = tx.QueryRow(ctx, `SELECT payload_hash FROM workflow_outbox WHERE org_id=$1 AND task_id=$2 AND kind=$3 AND idempotency_key=$4`, l.OrgID, l.TaskID, kind, key).Scan(&oldHash)
	if err == nil {
		if oldHash != hash {
			return result, auth.ErrConflict
		}
		return scanIntent(tx.QueryRow(ctx, `SELECT `+intentColumns+` FROM workflow_outbox WHERE org_id=$1 AND task_id=$2 AND kind=$3 AND idempotency_key=$4`, l.OrgID, l.TaskID, kind, key))
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	result = Intent{ID: domain.NewID(), OrgID: l.OrgID, TaskID: l.TaskID, RepositoryID: l.RepositoryID, OperationID: domain.NewID(), Kind: kind, State: "pending", Payload: payload}
	if _, err = tx.Exec(ctx, `INSERT INTO workflow_outbox(org_id,id,task_id,repository_id,operation_id,kind,idempotency_key,payload,payload_hash,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'pending')`, result.OrgID, result.ID, result.TaskID, result.RepositoryID, result.OperationID, kind, key, payload, hash); err != nil {
		return Intent{}, err
	}
	return result, emitTask(ctx, tx, task, "operation.prepared", "")
}
func (s *Service) BeginIntent(ctx context.Context, l Lease, id string) (Intent, error) {
	var result Intent
	err := s.withLease(ctx, l, "outbox.dispatch", func(tx pgx.Tx, t *Task) error {
		var err error
		result, err = s.BeginIntentTx(ctx, tx, l, id)
		return err
	})
	return result, err
}
func (s *Service) BeginIntentTx(ctx context.Context, tx pgx.Tx, l Lease, id string) (Intent, error) {
	if !auth.ValidID(id) {
		return Intent{}, auth.ErrInvalid
	}
	t, err := s.ValidateFenceTx(ctx, tx, l, "outbox.dispatch")
	if err != nil {
		return Intent{}, err
	}

	var result Intent
	result, err = scanIntent(tx.QueryRow(ctx, `SELECT `+intentColumns+` FROM workflow_outbox WHERE org_id=$1 AND id=$2 AND task_id=$3 FOR UPDATE`, l.OrgID, id, l.TaskID))
	if err != nil {
		return result, err
	}
	if result.State != "pending" && result.State != "absent" {
		return result, ErrReconciliation
	}
	if result.DispatchCount >= 3 {
		return result, auth.ErrConflict
	}
	hash, err := s.checkPolicy(ctx, tx, t, result.Kind)
	if err != nil || hash != t.PolicyHash {
		return result, ErrPolicy
	}
	if err = tx.QueryRow(ctx, `UPDATE workflow_outbox SET state='dispatching',dispatch_count=dispatch_count+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING dispatch_count`, l.OrgID, id).Scan(&result.DispatchCount); err != nil {
		return result, err
	}
	result.State = "dispatching"
	return result, emitTask(ctx, tx, t, "operation.dispatched", "")
}

func (s *Service) CompleteIntent(ctx context.Context, l Lease, id, outcome, evidence string) (Intent, error) {
	var result Intent
	err := s.withLease(ctx, l, "observe", func(tx pgx.Tx, t *Task) error {
		var err error
		result, err = resolveIntent(ctx, tx, l.OrgID, l.TaskID, id, outcome, evidence)
		if err != nil {
			return err
		}
		return emitTask(ctx, tx, *t, "operation.observed", "")
	})
	return result, err
}
func resolveIntent(ctx context.Context, tx pgx.Tx, orgID, taskID, id, outcome, evidence string) (Intent, error) {
	var result Intent
	if !auth.ValidID(id) || len(evidence) < 1 || len(evidence) > 2000 || (outcome != "succeeded" && outcome != "absent" && outcome != "unknown") {
		return result, auth.ErrInvalid
	}
	var err error
	result, err = scanIntent(tx.QueryRow(ctx, `SELECT `+intentColumns+` FROM workflow_outbox WHERE org_id=$1 AND id=$2 AND task_id=$3 FOR UPDATE`, orgID, id, taskID))
	if err != nil {
		return result, err
	}
	if result.State == outcome && result.Evidence == evidence {
		return result, nil
	}
	if result.State != "dispatching" && result.State != "unknown" {
		return result, auth.ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE workflow_outbox SET state=$3,evidence=$4,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, orgID, id, outcome, evidence)
	result.State = outcome
	result.Evidence = evidence
	return result, err
}
func (s *Service) ReconcileIntentTx(ctx context.Context, tx pgx.Tx, orgID, taskID, id, outcome, evidence string) (Intent, error) {
	result, err := resolveIntent(ctx, tx, orgID, taskID, id, outcome, evidence)
	if err != nil {
		return result, err
	}
	task, err := loadTask(ctx, tx, orgID, taskID)
	if err != nil {
		return result, err
	}
	return result, emitTask(ctx, tx, task, "operation.reconciled", "")
}
func (s *Service) ConcludeReconciledTx(ctx context.Context, tx pgx.Tx, orgID, taskID, actor, request string) error {
	t, err := loadTask(ctx, tx, orgID, taskID)
	if err != nil {
		return err
	}
	if t.State == domain.TaskCompleted || t.State == domain.TaskCancelled {
		return nil
	}
	var active, unknown bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_jobs WHERE org_id=$1 AND task_id=$2 AND state='running' AND lease_expires_at>clock_timestamp()), EXISTS(SELECT 1 FROM budget_reservations WHERE org_id=$1 AND task_id=$2 AND state IN ('reserved','dispatched','unknown'))`, orgID, taskID).Scan(&active, &unknown); err != nil {
		return err
	}
	if active || unknown {
		return ErrReconciliation
	}
	if err = checkUncertain(ctx, tx, orgID, taskID); err != nil {
		return err
	}
	state := domain.TaskCompleted
	if t.CancellationRequested {
		state = domain.TaskCancelled
	}
	if _, err = tx.Exec(ctx, `UPDATE workflow_jobs SET state=$3,lease_owner='',lease_expires_at=NULL WHERE org_id=$1 AND task_id=$2`, orgID, taskID, state); err != nil {
		return err
	}
	if t.CancellationRequested {
		return userTransition(ctx, tx, &t, actor, domain.TaskCancelled, "Cancellation reconciled; native publication already occurred", request)
	}
	return userTransition(ctx, tx, &t, actor, domain.TaskCompleted, "Native publication authoritatively reconciled", request)
}

func (s *Service) ReconcileIntent(ctx context.Context, orgID, id, outcome, evidence string) (Intent, error) {
	var result Intent
	if !auth.ValidID(orgID) || !auth.ValidID(id) {
		return result, auth.ErrInvalid
	}
	err := s.db.Tenant(ctx, orgID, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, orgID); err != nil {
			return err
		}
		var taskID string
		if err := tx.QueryRow(ctx, `SELECT task_id::text FROM workflow_outbox WHERE org_id=$1 AND id=$2`, orgID, id).Scan(&taskID); err != nil {
			return hidden(err)
		}
		var err error
		result, err = resolveIntent(ctx, tx, orgID, taskID, id, outcome, evidence)
		if err != nil {
			return err
		}
		task, err := loadTask(ctx, tx, orgID, taskID)
		if err != nil {
			return err
		}
		return emitTask(ctx, tx, task, "operation.reconciled", "")
	})
	return result, err
}
