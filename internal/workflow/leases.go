package workflow

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
)

func validTTL(ttl time.Duration) bool { return ttl >= time.Second && ttl <= 5*time.Minute }
func validLease(l Lease) bool {
	for _, id := range []string{l.OrgID, l.RepositoryID, l.TaskID, l.JobID, l.AttemptID, l.OperationID} {
		if !auth.ValidID(id) {
			return false
		}
	}
	return l.WorkerID != "" && len(l.WorkerID) <= 200 && l.Fence > 0
}

func (s *Service) nextOrg(ctx context.Context, allowed []string) (string, error) {
	var orgID string
	err := pgx.BeginFunc(ctx, s.db.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.scheduler','on',true)`); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT org_id::text FROM workflow_scheduler WHERE org_id=ANY($1::uuid[]) ORDER BY last_claimed_at,org_id FOR UPDATE SKIP LOCKED LIMIT 1`, allowed).Scan(&orgID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE workflow_scheduler SET last_claimed_at=clock_timestamp() WHERE org_id=$1`, orgID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoWork
	}
	return orgID, err
}

func (s *Service) Claim(ctx context.Context, workerID string, allowedOrgIDs []string, ttl time.Duration) (Lease, error) {
	var lease Lease
	if workerID == "" || len(workerID) > 200 || len(allowedOrgIDs) == 0 || len(allowedOrgIDs) > 200 || !validTTL(ttl) {
		return lease, auth.ErrInvalid
	}
	allowed := append([]string(nil), allowedOrgIDs...)
	for _, id := range allowed {
		if !auth.ValidID(id) {
			return lease, auth.ErrInvalid
		}
	}
	for len(allowed) > 0 {
		orgID, err := s.nextOrg(ctx, allowed)
		if err != nil {
			return lease, err
		}
		remaining := allowed[:0]
		for _, id := range allowed {
			if id != orgID {
				remaining = append(remaining, id)
			}
		}
		allowed = remaining
		err = s.db.Tenant(ctx, orgID, "", func(tx pgx.Tx) error {
			var claimErr error
			lease, claimErr = s.claimTx(ctx, tx, workerID, orgID, "", nil, ttl)
			if errors.Is(claimErr, ErrNoWork) {
				return nil
			}
			return claimErr
		})
		if err != nil {
			return Lease{}, err
		}
		if lease.JobID != "" {
			return lease, nil
		}
	}
	return Lease{}, ErrNoWork
}

func (s *Service) ClaimScoped(ctx context.Context, workerID, orgID, poolID string, repositories []string, ttl time.Duration) (Lease, error) {
	var result Lease
	err := s.db.Tenant(ctx, orgID, "", func(tx pgx.Tx) error {
		var err error
		result, err = s.ClaimScopedTx(ctx, tx, workerID, orgID, poolID, repositories, ttl)
		if errors.Is(err, ErrNoWork) {
			return nil
		}
		return err
	})
	if err == nil && result.JobID == "" {
		err = ErrNoWork
	}
	return result, err
}
func (s *Service) ClaimScopedTx(ctx context.Context, tx pgx.Tx, workerID, orgID, poolID string, repositories []string, ttl time.Duration) (Lease, error) {
	if workerID == "" || len(workerID) > 200 || !auth.ValidID(orgID) || !auth.ValidID(poolID) || len(repositories) == 0 || len(repositories) > 1000 || !validTTL(ttl) {
		return Lease{}, auth.ErrInvalid
	}
	for _, id := range repositories {
		if !auth.ValidID(id) {
			return Lease{}, auth.ErrInvalid
		}
	}
	return s.claimTx(ctx, tx, workerID, orgID, poolID, repositories, ttl)
}
func (s *Service) claimTx(ctx context.Context, tx pgx.Tx, workerID, orgID, poolID string, repositories []string, ttl time.Duration) (Lease, error) {
	if err := lockOrg(ctx, tx, orgID); err != nil {
		return Lease{}, err
	}
	if err := s.recoverTx(ctx, tx, orgID); err != nil {
		return Lease{}, err
	}
	var jobID, taskID, operationID string
	var fence int64
	var attempts int
	err := tx.QueryRow(ctx, `SELECT j.id::text,j.task_id::text,j.operation_id::text,j.fence,j.attempts FROM workflow_jobs j JOIN workflow_tasks t ON t.org_id=j.org_id AND t.id=j.task_id JOIN workflow_repo_fairness f ON f.org_id=t.org_id AND f.repository_id=t.repository_id WHERE j.org_id=$1 AND ($2::uuid IS NULL OR (t.runner_pool_id=$2 AND t.repository_id=ANY($3::uuid[]))) AND j.state='queued' AND j.available_at<=clock_timestamp() AND NOT EXISTS(SELECT 1 FROM organisations o WHERE o.id=j.org_id AND o.paused) AND NOT EXISTS(SELECT 1 FROM repositories r WHERE r.org_id=t.org_id AND r.id=t.repository_id AND (r.paused OR r.archived OR NOT r.accessible)) AND NOT EXISTS(SELECT 1 FROM workflow_pauses p WHERE p.org_id=t.org_id AND p.paused AND ((p.scope_kind='organisation' AND p.scope_id=t.org_id::text) OR (p.scope_kind='repository' AND p.scope_id=t.repository_id::text) OR (p.scope_kind='recipe' AND p.scope_id=t.recipe) OR (p.scope_kind='campaign' AND p.scope_id=t.campaign_id::text) OR (p.scope_kind='model' AND p.scope_id=t.model_connection_id::text) OR (p.scope_kind='runner_pool' AND p.scope_id=t.runner_pool_id::text))) AND NOT EXISTS(SELECT 1 FROM connections c WHERE c.org_id=t.org_id AND c.id=t.model_connection_id AND c.state IN ('revoked','disabled')) AND NOT EXISTS(SELECT 1 FROM workflow_jobs active JOIN workflow_tasks at ON at.org_id=active.org_id AND at.id=active.task_id WHERE active.org_id=j.org_id AND (active.state='reconciling' OR at.state='reconciling' OR active.state='running' AND active.lease_expires_at<=clock_timestamp() OR EXISTS(SELECT 1 FROM workflow_outbox pending WHERE pending.org_id=at.org_id AND pending.task_id=at.id AND pending.state IN ('dispatching','unknown'))) AND at.repository_id=t.repository_id AND at.target_branch=t.target_branch) ORDER BY f.last_claimed_at,j.priority DESC,t.created_at,j.id FOR UPDATE OF j SKIP LOCKED LIMIT 1`, orgID, optional(poolID), repositories).Scan(&jobID, &taskID, &operationID, &fence, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lease{}, ErrNoWork
	}
	if err != nil {
		return Lease{}, err
	}
	task, err := loadTask(ctx, tx, orgID, taskID)
	if err != nil {
		return Lease{}, err
	}
	if err = checkPauses(ctx, tx, task); err != nil {
		if errors.Is(err, ErrPaused) {
			return Lease{}, ErrNoWork
		}
		return Lease{}, err
	}
	hash, err := s.checkPolicy(ctx, tx, task, "dispatch")
	if err != nil || hash != task.PolicyHash {
		if _, err = tx.Exec(ctx, `UPDATE workflow_jobs SET state='blocked' WHERE org_id=$1 AND id=$2`, orgID, jobID); err != nil {
			return Lease{}, err
		}
		return Lease{}, setTaskState(ctx, tx, &task, domain.TaskBlocked, "Policy changed; review and resume with current policy", "")
	}
	if attempts >= task.MaxAttempts {
		if _, err = tx.Exec(ctx, `UPDATE workflow_jobs SET state='failed' WHERE org_id=$1 AND id=$2`, orgID, jobID); err != nil {
			return Lease{}, err
		}
		return Lease{}, setTaskState(ctx, tx, &task, domain.TaskFailed, "Attempt limit reached", "")
	}
	lease := Lease{OrgID: orgID, RepositoryID: task.RepositoryID, TaskID: taskID, JobID: jobID, AttemptID: domain.NewID(), OperationID: operationID, WorkerID: workerID, Fence: fence + 1, PolicyHash: task.PolicyHash}
	if err = tx.QueryRow(ctx, `UPDATE workflow_jobs SET state='running',fence=fence+1,attempts=attempts+1,lease_owner=$3,lease_expires_at=clock_timestamp()+$4::bigint*interval '1 millisecond' WHERE org_id=$1 AND id=$2 RETURNING lease_expires_at`, orgID, jobID, workerID, ttl.Milliseconds()).Scan(&lease.ExpiresAt); err != nil {
		return Lease{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workflow_attempts(org_id,id,task_id,job_id,number,fence,lease_owner,state) VALUES($1,$2,$3,$4,$5,$6,$7,'running')`, orgID, lease.AttemptID, taskID, jobID, attempts+1, lease.Fence, workerID); err != nil {
		return Lease{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workflow_repo_fairness SET last_claimed_at=clock_timestamp() WHERE org_id=$1 AND repository_id=$2`, orgID, task.RepositoryID); err != nil {
		return Lease{}, err
	}
	if err = setTaskState(ctx, tx, &task, domain.TaskReproducing, "", ""); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

func (s *Service) ValidateFenceTx(ctx context.Context, tx pgx.Tx, l Lease, action string) (Task, error) {
	var task Task
	if !validLease(l) {
		return task, ErrFence
	}
	if err := lockOrg(ctx, tx, l.OrgID); err != nil {
		return task, err
	}
	var valid bool
	err := tx.QueryRow(ctx, `SELECT j.state='running' AND j.fence=$5 AND j.lease_owner=$6 AND j.lease_expires_at>clock_timestamp() AND j.operation_id=$7 AND a.state='running' AND a.fence=j.fence AND a.lease_owner=j.lease_owner FROM workflow_jobs j JOIN workflow_attempts a ON a.org_id=j.org_id AND a.job_id=j.id AND a.task_id=j.task_id WHERE j.org_id=$1 AND j.id=$2 AND j.task_id=$3 AND a.id=$4 FOR UPDATE OF j,a`, l.OrgID, l.JobID, l.TaskID, l.AttemptID, l.Fence, l.WorkerID, l.OperationID).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !valid {
		return task, ErrFence
	}
	if err != nil {
		return task, err
	}
	task, err = loadTask(ctx, tx, l.OrgID, l.TaskID)
	if err != nil {
		return task, err
	}
	if task.RepositoryID != l.RepositoryID || task.PolicyHash != l.PolicyHash {
		return task, ErrFence
	}
	if action == "observe" {
		return task, nil
	}
	artifactUpload := action == "artifact.upload"
	if artifactUpload {
		action = "artifact"
	}
	if task.State == domain.TaskCancelled || task.State == domain.TaskCancelling && !artifactUpload {
		return task, ErrPaused
	}
	if err = checkPauses(ctx, tx, task); err != nil {
		return task, err
	}
	hash, err := s.checkPolicy(ctx, tx, task, action)
	if err != nil || hash != task.PolicyHash {
		return task, ErrPolicy
	}
	return task, nil
}

func (s *Service) withLease(ctx context.Context, l Lease, action string, fn func(pgx.Tx, *Task) error) error {
	if !validLease(l) {
		return ErrFence
	}
	var boundary error
	err := s.db.Tenant(ctx, l.OrgID, "", func(tx pgx.Tx) error {
		task, err := s.ValidateFenceTx(ctx, tx, l, action)
		if err == nil {
			err = fn(tx, &task)
		}
		if errors.Is(err, ErrPolicy) {
			boundary = err
			if _, err = tx.Exec(ctx, `UPDATE workflow_jobs SET state='blocked',lease_expires_at=NULL WHERE org_id=$1 AND id=$2`, l.OrgID, l.JobID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE workflow_attempts SET state='failed',ended_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, l.OrgID, l.AttemptID); err != nil {
				return err
			}
			return setTaskState(ctx, tx, &task, domain.TaskBlocked, "Policy changed; review and resume with current policy", "")
		}
		return err
	})
	if err != nil {
		return err
	}
	return boundary
}

func (s *Service) Heartbeat(ctx context.Context, l Lease, ttl time.Duration) (Lease, error) {
	if !validTTL(ttl) {
		return l, auth.ErrInvalid
	}
	err := s.withLease(ctx, l, "heartbeat", func(tx pgx.Tx, t *Task) error {
		return tx.QueryRow(ctx, `UPDATE workflow_jobs SET lease_expires_at=clock_timestamp()+$3::bigint*interval '1 millisecond' WHERE org_id=$1 AND id=$2 RETURNING lease_expires_at`, l.OrgID, l.JobID, ttl.Milliseconds()).Scan(&l.ExpiresAt)
	})
	return l, err
}

func (s *Service) Advance(ctx context.Context, l Lease, next domain.TaskState) (Task, error) {
	var result Task
	err := s.withLease(ctx, l, "advance", func(tx pgx.Tx, t *Task) error {
		transitions := map[domain.TaskState]domain.TaskState{domain.TaskReproducing: domain.TaskPlanning, domain.TaskPlanning: domain.TaskRepairing, domain.TaskRepairing: domain.TaskValidating, domain.TaskValidating: domain.TaskPublishing}
		if transitions[t.State] != next {
			return auth.ErrConflict
		}
		if err := setTaskState(ctx, tx, t, next, "", ""); err != nil {
			return err
		}
		result = *t
		return nil
	})
	return result, err
}

func (s *Service) Complete(ctx context.Context, l Lease, result Completion) (Task, error) {
	var task Task
	action := "complete"
	if result.Outcome == "cancelled" || result.Outcome == "uncertain" {
		action = "observe"
	}
	err := s.withLease(ctx, l, action, func(tx pgx.Tx, t *Task) error {
		if err := s.completeTx(ctx, tx, l, t, result); err != nil {
			return err
		}
		task = *t
		return nil
	})
	return task, err
}

func (s *Service) CompleteTx(ctx context.Context, tx pgx.Tx, l Lease, result Completion) (Task, error) {
	action := "complete"
	if result.Outcome == "cancelled" || result.Outcome == "uncertain" {
		action = "observe"
	}
	task, err := s.ValidateFenceTx(ctx, tx, l, action)
	if err != nil {
		return task, err
	}
	err = s.completeTx(ctx, tx, l, &task, result)
	return task, err
}
func (s *Service) completeTx(ctx context.Context, tx pgx.Tx, l Lease, t *Task, result Completion) error {
	state := domain.TaskFailed
	reason := strings.TrimSpace(result.Reason)
	if reason == "" {
		reason = "Attempt failed"
	}
	jobState, attemptState := "failed", "failed"
	paused := false
	switch result.Outcome {
	case "completed":
		if t.State != domain.TaskPublishing {
			return auth.ErrConflict
		}
		var incomplete bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_outbox WHERE org_id=$1 AND task_id=$2 AND state<>'succeeded')`, l.OrgID, l.TaskID).Scan(&incomplete); err != nil {
			return err
		}
		if incomplete {
			return ErrReconciliation
		}
		if err := checkUncertain(ctx, tx, l.OrgID, l.TaskID); err != nil {
			return err
		}
		state = domain.TaskCompleted
		jobState = "completed"
		attemptState = "completed"
		reason = ""
	case "cancelled":
		if t.State != domain.TaskCancelling {
			return auth.ErrConflict
		}
		if err := checkUncertain(ctx, tx, l.OrgID, l.TaskID); err != nil {
			return err
		}
		state = domain.TaskCancelled
		jobState = "cancelled"
		attemptState = "cancelled"
		reason = "Runner confirmed stopped; prior external effects retained"
	case "uncertain":
		state = domain.TaskReconciling
		jobState = "reconciling"
		attemptState = "uncertain"
		if result.Reason == "" {
			reason = "External outcome unknown; reconcile before retry"
		} else {
			reason = "External outcome unknown: " + reason
		}
	case "paused":
		if err := checkUncertain(ctx, tx, l.OrgID, l.TaskID); err != nil {
			if !errors.Is(err, ErrReconciliation) {
				return err
			}
			state, jobState, attemptState, reason = domain.TaskReconciling, "reconciling", "uncertain", "External outcome unknown; reconcile before retry"
		} else {
			state, jobState, paused = domain.TaskQueued, "queued", true
			reason = "Paused: " + strings.TrimPrefix(reason, "Paused: ") + "; resumes automatically"
		}
	case "failed":
		if err := checkUncertain(ctx, tx, l.OrgID, l.TaskID); err != nil {
			if !errors.Is(err, ErrReconciliation) {
				return err
			}
			state = domain.TaskReconciling
			jobState = "reconciling"
			attemptState = "uncertain"
			reason = "External outcome unknown; reconcile before retry"
		} else if result.Retryable {
			var attempts int
			if err = tx.QueryRow(ctx, `SELECT attempts FROM workflow_jobs WHERE org_id=$1 AND id=$2`, l.OrgID, l.JobID).Scan(&attempts); err != nil {
				return err
			}
			if attempts < t.MaxAttempts {
				state = domain.TaskQueued
				jobState = "queued"
				reason = "Retry scheduled after: " + reason
			}
		}
	default:
		return auth.ErrInvalid
	}
	if _, err := tx.Exec(ctx, `UPDATE workflow_attempts SET state=$3,ended_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, l.OrgID, l.AttemptID, attemptState); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workflow_jobs SET state=$3,lease_expires_at=NULL,lease_owner='',attempts=CASE WHEN $4 THEN greatest(attempts-1,0) ELSE attempts END,available_at=clock_timestamp()+CASE WHEN $4 THEN interval '15 minutes' ELSE LEAST(attempts*attempts,60)*interval '1 second' END WHERE org_id=$1 AND id=$2`, l.OrgID, l.JobID, jobState, paused); err != nil {
		return err
	}
	if err := setTaskState(ctx, tx, t, state, reason, ""); err != nil {
		return err
	}
	return nil
}

func (s *Service) recoverTx(ctx context.Context, tx pgx.Tx, orgID string) error {
	rows, err := tx.Query(ctx, `SELECT id::text,task_id::text,fence,attempts FROM workflow_jobs WHERE org_id=$1 AND state='running' AND lease_expires_at<=clock_timestamp() ORDER BY id FOR UPDATE LIMIT 200`, orgID)
	if err != nil {
		return err
	}
	type expired struct {
		id, task string
		fence    int64
		attempts int
	}
	var jobs []expired
	for rows.Next() {
		var j expired
		if err = rows.Scan(&j.id, &j.task, &j.fence, &j.attempts); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		task, err := loadTask(ctx, tx, orgID, j.task)
		if err != nil {
			return err
		}
		state, jobState, reason := domain.TaskQueued, "queued", "Lease expired; bounded retry from reproduction"
		uncertainty := checkUncertain(ctx, tx, orgID, j.task)
		if uncertainty != nil && !errors.Is(uncertainty, ErrReconciliation) {
			return uncertainty
		}
		if task.State == domain.TaskPublishing || task.State == domain.TaskCancelling || uncertainty != nil {
			state = domain.TaskReconciling
			jobState = "reconciling"
			reason = "Lease expired with uncertain external or cancellation outcome"
		} else if j.attempts >= task.MaxAttempts {
			state = domain.TaskFailed
			jobState = "failed"
			reason = "Attempt limit reached after lease expiry"
		}
		if _, err = tx.Exec(ctx, `UPDATE workflow_attempts SET state='lost',ended_at=clock_timestamp() WHERE org_id=$1 AND job_id=$2 AND fence=$3 AND state='running'`, orgID, j.id, j.fence); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workflow_jobs SET state=$3,lease_owner='',lease_expires_at=NULL,available_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, orgID, j.id, jobState); err != nil {
			return err
		}
		if err = setTaskState(ctx, tx, &task, state, reason, ""); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Recover(ctx context.Context, orgID string) error {
	if !auth.ValidID(orgID) {
		return auth.ErrInvalid
	}
	return s.db.Tenant(ctx, orgID, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, orgID); err != nil {
			return err
		}
		return s.recoverTx(ctx, tx, orgID)
	})
}

func (s *Service) InvalidateWorkerTx(ctx context.Context, tx pgx.Tx, orgID, workerID string) error {
	return s.invalidateTx(ctx, tx, orgID, workerID, "")
}

func (s *Service) InvalidateJobTx(ctx context.Context, tx pgx.Tx, orgID, workerID, jobID string) error {
	if !auth.ValidID(jobID) {
		return auth.ErrInvalid
	}
	return s.invalidateTx(ctx, tx, orgID, workerID, jobID)
}

func (s *Service) invalidateTx(ctx context.Context, tx pgx.Tx, orgID, workerID, jobID string) error {
	if !auth.ValidID(orgID) || workerID == "" {
		return auth.ErrInvalid
	}
	if err := lockOrg(ctx, tx, orgID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `UPDATE workflow_jobs SET state='blocked',fence=fence+1,lease_owner='',lease_expires_at=NULL WHERE org_id=$1 AND lease_owner=$2 AND state='running' AND ($3='' OR id::text=$3) RETURNING id::text,task_id::text`, orgID, workerID, jobID)
	if err != nil {
		return err
	}
	type affected struct{ job, task string }
	var list []affected
	for rows.Next() {
		var a affected
		if err = rows.Scan(&a.job, &a.task); err != nil {
			rows.Close()
			return err
		}
		list = append(list, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, a := range list {
		if _, err = tx.Exec(ctx, `UPDATE workflow_attempts SET state='lost',ended_at=clock_timestamp() WHERE org_id=$1 AND job_id=$2 AND state='running'`, orgID, a.job); err != nil {
			return err
		}
		task, err := loadTask(ctx, tx, orgID, a.task)
		if err != nil {
			return err
		}
		state, reason := domain.TaskBlocked, "Runner authority revoked; prior process stop unconfirmed"
		uncertain := checkUncertain(ctx, tx, orgID, a.task)
		if errors.Is(uncertain, ErrReconciliation) || task.State == domain.TaskPublishing || task.State == domain.TaskCancelling {
			state = domain.TaskReconciling
			reason = "Runner revoked with external or cancellation outcome requiring reconciliation"
		} else if uncertain != nil {
			return uncertain
		}
		if err = setTaskState(ctx, tx, &task, state, reason, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) AdvanceTx(ctx context.Context, tx pgx.Tx, l Lease, next domain.TaskState) (Task, error) {
	task, err := s.ValidateFenceTx(ctx, tx, l, "advance")
	if err != nil {
		return task, err
	}
	transitions := map[domain.TaskState]domain.TaskState{domain.TaskReproducing: domain.TaskPlanning, domain.TaskPlanning: domain.TaskRepairing, domain.TaskRepairing: domain.TaskValidating, domain.TaskValidating: domain.TaskPublishing}
	if transitions[task.State] != next {
		return task, auth.ErrConflict
	}
	err = setTaskState(ctx, tx, &task, next, "", "")
	return task, err
}
