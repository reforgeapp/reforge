package repair

import (
	"context"
	"github.com/jackc/pgx/v5"
	"reforge/internal/budget"
	"reforge/internal/policy"
	"reforge/internal/workflow"
	"time"
)

func (s *Service) CheckModelTx(ctx context.Context, tx pgx.Tx, t workflow.Task, reservation budget.Reservation) error {
	run, err := loadRun(ctx, tx, t.OrgID, t.ID)
	if err != nil {
		return err
	}
	p := run.Context.Plan
	if !p.Valid() || run.State != "queued" || reservation.Quote.MaxOutputTokens > int64(run.Context.MaxOutputTokens) || reservation.Quote.MaxMilliseconds > run.Context.TurnTimeoutMS {
		return workflow.ErrPolicy
	}
	var turns int
	var started *time.Time
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM model_turns WHERE org_id=$1 AND task_id=$2),(SELECT min(created_at) FROM model_turns WHERE org_id=$1 AND task_id=$2 AND attempt_id=$3)`, t.OrgID, t.ID, reservation.Lease.AttemptID).Scan(&turns, &started); err != nil {
		return err
	}
	if turns >= p.Recipe.MaxTurns || started != nil && time.Since(*started) > time.Duration(p.Recipe.TimeoutSeconds)*time.Second {
		return budget.ErrCapacity
	}
	var money, concurrency, attempts, open int64
	if err = tx.QueryRow(ctx, `SELECT coalesce(sum(coalesce((record->'actual'->>'micro_usd')::bigint,(record->'maximum'->>'micro_usd')::bigint,0)),0) FROM budget_reservations WHERE org_id=$1 AND task_id=$2 AND state<>'cancelled'`, t.OrgID, t.ID).Scan(&money); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM workflow_jobs WHERE org_id=$1 AND state='running' AND lease_expires_at>clock_timestamp()`, t.OrgID).Scan(&concurrency); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM workflow_attempts WHERE org_id=$1 AND task_id=$2`, t.OrgID, t.ID).Scan(&attempts); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM maintenance_repairs WHERE org_id=$1 AND active`, t.OrgID).Scan(&open); err != nil {
		return err
	}
	zero := int64(0)
	binding := policy.Binding{Head: p.BaselineSHA, Target: p.TargetSHA, PolicyHash: t.PolicyHash}
	now := time.Now().UTC()
	evidence := []policy.Evidence{{ID: "execution_authority", State: "satisfied", Binding: binding, ObservedAt: now, Reference: "runner-job:" + reservation.Lease.JobID}, {ID: "budget_capacity", State: "satisfied", Binding: binding, ObservedAt: now, Reference: "reservation:" + reservation.ID}}
	decision, err := s.policies.EvaluateTx(ctx, tx, t.OrgID, t.RepositoryID, policy.Input{Action: policy.Repair, Recipe: t.Recipe, Model: run.Context.Model, Route: t.ModelConnectionID + "/" + t.ModelRoute, Current: binding, StartingPolicyHash: t.StartingPolicyHash, Evidence: evidence, Usage: policy.Limits{Budget: &money, Concurrency: &concurrency, Attempts: &attempts, ChangedFiles: &zero, ChangedLines: &zero, OpenChanges: &open}})
	if err != nil {
		return err
	}
	if decision.Outcome != "allow" {
		return workflow.ErrPolicy
	}
	return nil
}
