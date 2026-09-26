package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/artifact"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/workflow"
)

func (s *Service) Claim(ctx context.Context, raw string) (Assignment, error) {
	var result Assignment
	org, id, err := parse(raw, "sup")
	if err != nil {
		return result, err
	}
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, org); err != nil {
			return err
		}
		r, err := supervisorTx(ctx, tx, org, id, raw)
		if err != nil {
			return err
		}
		var busy bool
		if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM workflow_jobs WHERE org_id=$1 AND lease_owner=$2 AND state='running' AND lease_expires_at>clock_timestamp())>=(SELECT slots FROM runners WHERE org_id=$1 AND id=$2::uuid)`, org, id).Scan(&busy); err != nil {
			return err
		}
		if busy {
			return workflow.ErrNoWork
		}
		rows, err := tx.Query(ctx, `SELECT repository_id::text FROM runner_pool_repositories WHERE org_id=$1 AND pool_id=$2 ORDER BY repository_id`, org, r.PoolID)
		if err != nil {
			return err
		}
		var repos []string
		for rows.Next() {
			var repo string
			if err = rows.Scan(&repo); err != nil {
				rows.Close()
				return err
			}
			repos = append(repos, repo)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(repos) == 0 {
			return workflow.ErrNoWork
		}
		l, err := s.workflow.ClaimScopedTx(ctx, tx, id, org, r.PoolID, repos, time.Minute)
		if errors.Is(err, workflow.ErrNoWork) {
			return nil
		}
		if err != nil {
			return err
		}
		if l.JobID == "" {
			return nil
		}
		task, err := s.workflow.ValidateFenceTx(ctx, tx, l, "dispatch")
		if err != nil {
			return err
		}
		credentialID := domain.NewID()
		credential := token("job", org, credentialID)
		expires := time.Now().UTC().Add(5 * time.Minute)
		if r.CredentialExpiresAt.Before(expires) {
			expires = r.CredentialExpiresAt
		}
		if _, err = tx.Exec(ctx, `INSERT INTO runner_job_credentials(org_id,id,runner_id,pool_id,repository_id,task_id,job_id,attempt_id,operation_id,fencing_token,policy_hash,token_hash,methods,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, org, credentialID, id, r.PoolID, l.RepositoryID, l.TaskID, l.JobID, l.AttemptID, l.OperationID, l.Fence, l.PolicyHash, hash(credential), []string{"heartbeat", "progress", "result", "artifact.upload", "artifact.download", "broker"}, expires); err != nil {
			return err
		}
		result = Assignment{Credential: Credential{Token: credential, ExpiresAt: expires, Runner: r}, Lease: l, Task: task}
		return nil
	})
	if err == nil && result.Lease.JobID == "" {
		err = workflow.ErrNoWork
	}
	if errors.Is(err, workflow.ErrNoWork) {
		if seen := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE runners SET last_seen_at=clock_timestamp() WHERE org_id=$1 AND id=$2 AND last_seen_at<clock_timestamp()-interval '30 seconds'`, org, id)
			return err
		}); seen != nil {
			err = seen
		}
	}
	if err != nil {
		result = Assignment{}
	}
	return result, err
}
func (s *Service) withJob(ctx context.Context, raw, method, action string, fn func(pgx.Tx, workflow.Lease, workflow.Task, string) error) error {
	org, id, err := parse(raw, "job")
	if err != nil {
		return err
	}
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error { return s.jobTx(ctx, tx, raw, org, id, method, action, fn) })
	if errors.Is(err, workflow.ErrPolicy) && action != "policy-revocation" {
		blockErr := s.withJob(ctx, raw, method, "policy-revocation", func(tx pgx.Tx, l workflow.Lease, _ workflow.Task, _ string) error {
			if _, err := tx.Exec(ctx, `UPDATE runner_job_credentials SET revoked_at=clock_timestamp() WHERE org_id=$1 AND runner_id=$2 AND job_id=$3 AND revoked_at IS NULL`, l.OrgID, l.WorkerID, l.JobID); err != nil {
				return err
			}
			return s.workflow.InvalidateJobTx(ctx, tx, l.OrgID, l.WorkerID, l.JobID)
		})
		if blockErr != nil && !errors.Is(blockErr, auth.ErrUnauthenticated) && !errors.Is(blockErr, workflow.ErrFence) {
			return blockErr
		}
	}
	return err
}

func (s *Service) Heartbeat(ctx context.Context, raw string) (Heartbeat, error) {
	var result Heartbeat
	err := s.withJob(ctx, raw, "heartbeat", "observe", func(tx pgx.Tx, l workflow.Lease, t workflow.Task, id string) error {
		result.Lease = l
		if t.State == domain.TaskCancelling {
			result.Stop = true
			result.Reason = "Cancellation requested"
			return nil
		}
		if _, err := s.workflow.ValidateFenceTx(ctx, tx, l, "heartbeat"); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `UPDATE workflow_jobs SET lease_expires_at=clock_timestamp()+interval '1 minute' WHERE org_id=$1 AND id=$2 RETURNING lease_expires_at`, l.OrgID, l.JobID).Scan(&result.Lease.ExpiresAt); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE runner_job_credentials c SET expires_at=LEAST(clock_timestamp()+interval '5 minutes',r.credential_expires_at) FROM runners r WHERE c.org_id=$1 AND c.id=$2 AND r.org_id=c.org_id AND r.id=c.runner_id`, l.OrgID, id)
		return err
	})
	return result, err
}
func (s *Service) Complete(ctx context.Context, raw string, in workflow.Completion) (workflow.Task, error) {
	var result workflow.Task
	err := s.withJob(ctx, raw, "result", "observe", func(tx pgx.Tx, l workflow.Lease, t workflow.Task, id string) error {
		var err error
		if s.CompletionCheck != nil {
			if err = s.CompletionCheck(ctx, tx, l, t, in); err != nil {
				return err
			}
		}
		result, err = s.workflow.CompleteTx(ctx, tx, l, in)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE runner_job_credentials SET revoked_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, l.OrgID, id)
		return err
	})
	return result, err
}
func (s *Service) Progress(ctx context.Context, raw string, next domain.TaskState) (workflow.Task, error) {
	var result workflow.Task
	err := s.withJob(ctx, raw, "progress", "advance", func(tx pgx.Tx, l workflow.Lease, t workflow.Task, id string) error {
		var err error
		result, err = s.workflow.AdvanceTx(ctx, tx, l, next)
		return err
	})
	return result, err
}
func (s *Service) Upload(ctx context.Context, raw, name, media string, input io.Reader) (artifact.Metadata, error) {
	var result artifact.Metadata
	if s.artifacts == nil {
		return result, ErrUnsupported
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, _, err := parse(raw, "job"); err != nil {
		return result, err
	}
	data, err := io.ReadAll(io.LimitReader(input, artifact.MaxSize+1))
	if err != nil {
		return result, err
	}
	if int64(len(data)) > artifact.MaxSize {
		return result, artifact.ErrQuota
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	err = s.withJob(ctx, raw, "artifact.upload", "artifact", func(tx pgx.Tx, l workflow.Lease, t workflow.Task, id string) error {
		var err error
		result, err = s.artifacts.PutTx(ctx, tx, artifact.Metadata{OrgID: l.OrgID, RepositoryID: l.RepositoryID, TaskID: l.TaskID, Name: name, MediaType: media, ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour)}, l.AttemptID, bytes.NewReader(data))
		return err
	})
	if err != nil && result.ID != "" {
		_ = s.artifacts.RemoveBlob(result.OrgID, result.ID)
		result = artifact.Metadata{}
	}
	return result, err
}
func (s *Service) Metadata(ctx context.Context, session auth.Session, org, id string) (artifact.Metadata, error) {
	var m artifact.Metadata
	if s.artifacts == nil {
		return m, ErrUnsupported
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		m, err = s.artifacts.MetadataTx(ctx, tx, a, id)
		return err
	})
	return m, err
}
func (s *Service) Download(ctx context.Context, session auth.Session, org, id string) (artifact.Metadata, io.ReadCloser, error) {
	m, err := s.Metadata(ctx, session, org, id)
	if err != nil {
		return m, nil, err
	}
	reader, err := s.artifacts.Open(m)
	if err != nil {
		return m, nil, err
	}
	return m, &checkedReader{reader: reader, check: func() error {
		return s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error { _, err := s.artifacts.MetadataTx(ctx, tx, a, id); return err })
	}}, nil
}

type checkedReader struct {
	reader io.ReadCloser
	check  func() error
}

func (r *checkedReader) Read(p []byte) (int, error) {
	if err := r.check(); err != nil {
		return 0, err
	}
	if len(p) > 32768 {
		p = p[:32768]
	}
	return r.reader.Read(p)
}
func (r *checkedReader) Close() error { return r.reader.Close() }
func (s *Service) DownloadJob(ctx context.Context, raw, id string) (artifact.Metadata, io.ReadCloser, error) {
	var m artifact.Metadata
	if s.artifacts == nil {
		return m, nil, ErrUnsupported
	}
	check := func() error {
		return s.withJob(ctx, raw, "artifact.download", "artifact", func(tx pgx.Tx, l workflow.Lease, t workflow.Task, _ string) error {
			var err error
			m, err = s.artifacts.MetadataTx(ctx, tx, domain.Actor{OrgID: l.OrgID, RepositoryIDs: []string{l.RepositoryID}}, id)
			if err == nil && m.TaskID != l.TaskID {
				return auth.ErrForbidden
			}
			return err
		})
	}
	if err := check(); err != nil {
		return m, nil, err
	}
	reader, err := s.artifacts.Open(m)
	if err != nil {
		return m, nil, err
	}
	return m, &checkedReader{reader: reader, check: check}, nil
}
func (s *Service) RegisterOperation(name string, handler FixedOperation) error {
	if name == "" || len(name) > 100 || handler == nil {
		return auth.ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.operations[name]; exists {
		return auth.ErrConflict
	}
	s.operations[name] = handler
	return nil
}
func (s *Service) Broker(ctx context.Context, raw, name, connection string, input json.RawMessage) (json.RawMessage, error) {
	if !auth.ValidID(connection) || len(input) > 32768 || !json.Valid(input) {
		return nil, auth.ErrInvalid
	}
	s.mu.RLock()
	handler := s.operations[name]
	s.mu.RUnlock()
	if handler == nil {
		return nil, ErrUnsupported
	}
	var result json.RawMessage
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := s.withJob(ctx, raw, "broker", name, func(tx pgx.Tx, l workflow.Lease, t workflow.Task, _ string) error {
		var permitted bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connection_routes route JOIN connections c ON c.org_id=route.org_id AND c.id=route.connection_id WHERE route.org_id=$1 AND route.connection_id=$2 AND route.runner_id=$3 AND route.revoked_at IS NULL AND c.state NOT IN ('revoked','disabled') AND (c.id=$4::uuid OR EXISTS(SELECT 1 FROM repositories r WHERE r.org_id=$1 AND r.id=$5 AND r.connection_id=c.id)))`, l.OrgID, connection, l.WorkerID, optional(t.ModelConnectionID), l.RepositoryID).Scan(&permitted)
		if err != nil {
			return err
		}
		if !permitted {
			return auth.ErrForbidden
		}
		result, err = handler(ctx, tx, Operation{Lease: l, Task: t, RunnerID: l.WorkerID, PoolID: t.RunnerPoolID, ConnectionID: connection, Input: append(json.RawMessage(nil), input...)})
		if err != nil {
			return err
		}
		if len(result) > 65536 || !json.Valid(result) {
			return auth.ErrInvalid
		}
		return nil
	})
	return result, err
}
func optional(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Service) WithJob(ctx context.Context, raw, action string, fn func(pgx.Tx, workflow.Lease, workflow.Task) error) error {
	if fn == nil {
		return auth.ErrInvalid
	}
	return s.withJob(ctx, raw, "broker", action, func(tx pgx.Tx, l workflow.Lease, t workflow.Task, _ string) error { return fn(tx, l, t) })
}

func (s *Service) jobTx(ctx context.Context, tx pgx.Tx, raw, org, id, method, action string, fn func(pgx.Tx, workflow.Lease, workflow.Task, string) error) error {
	if err := lockOrg(ctx, tx, org); err != nil {
		return err
	}
	var l workflow.Lease
	var digest, pool string
	var valid bool
	err := tx.QueryRow(ctx, `SELECT c.org_id::text,c.repository_id::text,c.task_id::text,c.job_id::text,c.attempt_id::text,c.operation_id::text,c.runner_id::text,c.fencing_token,c.policy_hash,c.pool_id::text,c.token_hash,c.revoked_at IS NULL AND c.expires_at>clock_timestamp() AND $3=ANY(c.methods) AND r.state='active' AND r.credential_expires_at>clock_timestamp() AND p.state='active' AND EXISTS(SELECT 1 FROM runner_pool_repositories rp WHERE rp.org_id=c.org_id AND rp.pool_id=c.pool_id AND rp.repository_id=c.repository_id) FROM runner_job_credentials c JOIN runners r ON r.org_id=c.org_id AND r.id=c.runner_id JOIN runner_pools p ON p.org_id=c.org_id AND p.id=c.pool_id WHERE c.org_id=$1 AND c.id=$2 FOR UPDATE OF c,r`, org, id, method).Scan(&l.OrgID, &l.RepositoryID, &l.TaskID, &l.JobID, &l.AttemptID, &l.OperationID, &l.WorkerID, &l.Fence, &l.PolicyHash, &pool, &digest, &valid)
	if err != nil || !valid || !matches(raw, digest) {
		return auth.ErrUnauthenticated
	}
	checkAction := action
	if action == "policy-revocation" {
		checkAction = "observe"
	}
	task, err := s.workflow.ValidateFenceTx(ctx, tx, l, checkAction)
	if err != nil {
		return err
	}
	if task.RunnerPoolID != pool {
		return auth.ErrForbidden
	}
	if _, err = tx.Exec(ctx, `UPDATE runners SET last_seen_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, org, l.WorkerID); err != nil {
		return err
	}
	return fn(tx, l, task, id)
}

func (s *Service) WithJobTx(ctx context.Context, tx pgx.Tx, raw, action string, fn func(pgx.Tx, workflow.Lease, workflow.Task) error) error {
	org, id, err := parse(raw, "job")
	if err != nil {
		return err
	}
	if fn == nil {
		return auth.ErrInvalid
	}
	return s.jobTx(ctx, tx, raw, org, id, "broker", action, func(tx pgx.Tx, l workflow.Lease, t workflow.Task, _ string) error { return fn(tx, l, t) })
}
