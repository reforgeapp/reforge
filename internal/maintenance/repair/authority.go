package repair

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/policy"
	"reforge/internal/workflow"
)

func (s *Service) CheckModelTx(ctx context.Context, tx pgx.Tx, t workflow.Task, reservation budget.Reservation) error {
	run, err := loadRun(ctx, tx, t.OrgID, t.ID)
	if err != nil {
		return err
	}
	p := run.Context.Plan
	active := false
	if run.State == "handoff" {
		if run.Branch != "" || run.CandidateSHA != "" || len(run.CandidateChecks) > 0 || run.Change != nil || len(run.CandidateArtifacts) > 0 {
			return workflow.ErrReconciliation
		}
		if err = tx.QueryRow(ctx, `SELECT active FROM maintenance_repairs WHERE org_id=$1 AND task_id=$2`, t.OrgID, t.ID).Scan(&active); err != nil {
			return err
		}
		if !active {
			if err = checkRepairOverlapTx(ctx, tx, run); err != nil {
				return err
			}
		}
		var started time.Time
		if err = tx.QueryRow(ctx, `SELECT started_at FROM workflow_attempts WHERE org_id=$1 AND task_id=$2 AND id=$3 AND state='running'`, t.OrgID, t.ID, reservation.Lease.AttemptID).Scan(&started); err != nil {
			return err
		}
		if !started.After(run.UpdatedAt) {
			return workflow.ErrPolicy
		}
	}
	if !p.Valid() || (run.State != "queued" && run.State != "handoff") || reservation.Quote.MaxOutputTokens > int64(run.Context.MaxOutputTokens) || reservation.Quote.MaxMilliseconds > run.Context.TurnTimeoutMS {
		return workflow.ErrPolicy
	}
	var turns int
	var started *time.Time
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM model_turns WHERE org_id=$1 AND task_id=$2),(SELECT min(created_at) FROM model_turns WHERE org_id=$1 AND task_id=$2 AND attempt_id=$3)`, t.OrgID, t.ID, reservation.Lease.AttemptID).Scan(&turns, &started); err != nil {
		return err
	}
	if turns >= p.Recipe.MaxTurns || started != nil && time.Since(*started) > AttemptTimeout(p) {
		return ErrRunLimit
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
	if run.State == "handoff" && !active {
		open++
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
	if run.State == "handoff" {
		if !active {
			tag, updateErr := tx.Exec(ctx, `UPDATE maintenance_repairs SET active=true WHERE org_id=$1 AND task_id=$2 AND active=false`, t.OrgID, t.ID)
			if updateErr != nil {
				return updateErr
			}
			if tag.RowsAffected() != 1 {
				return auth.ErrConflict
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE repair_runs SET state='queued',report=NULL,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND state='handoff'`, t.OrgID, t.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return auth.ErrConflict
		}
	}
	return nil
}

func checkRepairOverlapTx(ctx context.Context, tx pgx.Tx, run Run) error {
	return discovery.CheckRepairOverlapTx(ctx, tx, run.Context.Finding)
}
