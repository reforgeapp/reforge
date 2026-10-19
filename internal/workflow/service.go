package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/store"
)

type Service struct {
	db            *store.Store
	auth          *auth.Service
	policy        PolicyCheck
	scopeMu       sync.RWMutex
	scopeCheck    ScopeCheck
	campaignCheck func(context.Context, pgx.Tx, Task) error
}

func New(db *store.Store, identity *auth.Service, policy PolicyCheck) *Service {
	return &Service{db: db, auth: identity, policy: policy}
}
func digest(value []byte) string { h := sha256.Sum256(value); return hex.EncodeToString(h[:]) }
func optional(id string) any {
	if id == "" {
		return nil
	}
	return id
}
func hidden(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrForbidden
	}
	return err
}
func canManage(a domain.Actor, repoID string) bool {
	return auth.CanReadRepository(a, repoID) && (a.Role == domain.Owner || a.Role == domain.Admin || a.Role == domain.Maintainer)
}
func (s *Service) checkPolicy(ctx context.Context, tx pgx.Tx, t Task, action string) (string, error) {
	if s.policy == nil {
		return "", ErrPolicy
	}
	if t.CampaignID != "" && action != "enqueue" {
		s.scopeMu.RLock()
		check := s.campaignCheck
		s.scopeMu.RUnlock()
		if check == nil {
			return "", ErrPolicy
		}
		if err := check(ctx, tx, t); err != nil {
			return "", err
		}
	}
	hash, err := s.policy(ctx, tx, t, action)
	if err != nil || hash == "" {
		return "", ErrPolicy
	}
	return hash, nil
}

const taskColumns = `id::text,org_id::text,repository_id::text,operation_id::text,recipe,recipe_version,target_branch,coalesce(model_connection_id::text,''),coalesce(campaign_id::text,''),coalesce(runner_pool_id::text,''),policy_hash,starting_policy_hash,state,reason,version,cancel_version,cancellation_requested,max_attempts,created_at,model_route`

func scanTask(row pgx.Row) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.OrgID, &t.RepositoryID, &t.OperationID, &t.Recipe, &t.RecipeVersion, &t.TargetBranch, &t.ModelConnectionID, &t.CampaignID, &t.RunnerPoolID, &t.PolicyHash, &t.StartingPolicyHash, &t.State, &t.Reason, &t.Version, &t.CancelVersion, &t.CancellationRequested, &t.MaxAttempts, &t.CreatedAt, &t.ModelRoute)
	return t, hidden(err)
}
func (s *Service) PausedTx(ctx context.Context, tx pgx.Tx, orgID, id string) (bool, error) {
	t, err := loadTask(ctx, tx, orgID, id)
	if err != nil {
		return false, err
	}
	if err = checkPauses(ctx, tx, t); errors.Is(err, ErrPaused) {
		return true, nil
	}
	return false, err
}

func loadTask(ctx context.Context, tx pgx.Tx, orgID, id string) (Task, error) {
	return scanTask(tx.QueryRow(ctx, `SELECT `+taskColumns+` FROM workflow_tasks WHERE org_id=$1 AND id=$2`, orgID, id))
}
func lockOrg(ctx context.Context, tx pgx.Tx, orgID string) error {
	var id string
	return hidden(tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, orgID).Scan(&id))
}

func (s *Service) Enqueue(ctx context.Context, session auth.Session, orgID string, in EnqueueInput, requestID string) (Task, error) {
	if in.CampaignID != "" {
		return Task{}, auth.ErrInvalid
	}
	return s.EnqueuePrepared(ctx, session, orgID, in, requestID, nil)
}

func (s *Service) EnqueuePrepared(ctx context.Context, session auth.Session, orgID string, in EnqueueInput, requestID string, prepare func(context.Context, pgx.Tx, Task) error) (Task, error) {
	var result Task
	if !auth.ValidID(in.RepositoryID) || len(in.Recipe) < 1 || len(in.Recipe) > 100 || len(in.RecipeVersion) < 1 || len(in.RecipeVersion) > 100 || len(in.TargetBranch) < 1 || len(in.TargetBranch) > 255 || len(in.IdempotencyKey) < 1 || len(in.IdempotencyKey) > 200 || in.Priority < -10 || in.Priority > 10 {
		return result, auth.ErrInvalid
	}
	for _, id := range []string{in.ModelConnectionID, in.CampaignID, in.RunnerPoolID} {
		if id != "" && !auth.ValidID(id) {
			return result, auth.ErrInvalid
		}
	}
	if in.ModelRoute == "" {
		in.ModelRoute = "default"
	}
	if len(in.ModelRoute) > 100 || strings.TrimSpace(in.ModelRoute) != in.ModelRoute {
		return result, auth.ErrInvalid
	}
	if in.MaxAttempts == 0 {
		in.MaxAttempts = 3
	}
	if in.MaxAttempts < 1 || in.MaxAttempts > 10 {
		return result, auth.ErrInvalid
	}
	raw, _ := json.Marshal(in)
	requestHash := digest(raw)
	err := s.auth.WithMutation(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if !canManage(a, in.RepositoryID) {
			return auth.ErrForbidden
		}
		var oldID, oldHash string
		err := tx.QueryRow(ctx, `SELECT id::text,request_hash FROM workflow_tasks WHERE org_id=$1 AND idempotency_key=$2`, orgID, in.IdempotencyKey).Scan(&oldID, &oldHash)
		if err == nil {
			if oldHash != requestHash {
				return auth.ErrConflict
			}
			result, err = loadTask(ctx, tx, orgID, oldID)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		result = Task{ID: domain.NewID(), OrgID: orgID, RepositoryID: in.RepositoryID, OperationID: domain.NewID(), Recipe: in.Recipe, RecipeVersion: in.RecipeVersion, TargetBranch: in.TargetBranch, ModelConnectionID: in.ModelConnectionID, ModelRoute: in.ModelRoute, CampaignID: in.CampaignID, RunnerPoolID: in.RunnerPoolID, State: domain.TaskQueued, Version: 1, MaxAttempts: in.MaxAttempts}
		for kind, id := range map[string]string{"model": result.ModelConnectionID, "campaign": result.CampaignID, "runner_pool": result.RunnerPoolID} {
			if id != "" {
				if err = s.scopeExists(ctx, tx, orgID, kind, id); err != nil {
					return err
				}
			}
		}
		if err = checkPauses(ctx, tx, result); err != nil {
			return err
		}
		hash, err := s.checkPolicy(ctx, tx, result, "enqueue")
		if err != nil {
			return err
		}
		if in.PolicyHash != "" && in.PolicyHash != hash {
			return ErrPolicy
		}
		result.PolicyHash = hash
		result.StartingPolicyHash = hash
		_, err = tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,model_connection_id,campaign_id,runner_pool_id,policy_hash,starting_policy_hash,state,max_attempts,created_by,model_route) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13,'queued',$14,$15,$16)`, orgID, result.ID, result.RepositoryID, result.OperationID, in.IdempotencyKey, requestHash, result.Recipe, result.RecipeVersion, result.TargetBranch, optional(result.ModelConnectionID), optional(result.CampaignID), optional(result.RunnerPoolID), hash, result.MaxAttempts, a.UserID, result.ModelRoute)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO workflow_jobs(org_id,id,task_id,operation_id,state,priority) VALUES($1,$2,$3,$4,'queued',$5)`, orgID, domain.NewID(), result.ID, result.OperationID, in.Priority); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO workflow_repo_fairness(org_id,repository_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, orgID, result.RepositoryID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO workflow_scheduler(org_id) VALUES($1) ON CONFLICT DO NOTHING`, orgID); err != nil {
			return err
		}
		if prepare != nil {
			if err = prepare(ctx, tx, result); err != nil {
				return err
			}
		}
		if err = emitTask(ctx, tx, result, "task.queued", requestID); err != nil {
			return err
		}
		if err = audit(ctx, tx, orgID, a.UserID, "task.enqueued", result.ID, requestID, result.Version); err != nil {
			return err
		}
		result, err = loadTask(ctx, tx, orgID, result.ID)
		return err
	})
	return result, err
}

func checkPauses(ctx context.Context, tx pgx.Tx, t Task) error {
	var paused bool
	err := tx.QueryRow(ctx, `SELECT o.paused OR r.paused OR r.archived OR NOT r.accessible OR EXISTS(SELECT 1 FROM workflow_pauses p WHERE p.org_id=$1::uuid AND p.paused AND ((p.scope_kind='organisation' AND p.scope_id=$1::text) OR (p.scope_kind='repository' AND p.scope_id=$2::text) OR (p.scope_kind='recipe' AND p.scope_id=$3) OR (p.scope_kind='campaign' AND p.scope_id=$4) OR (p.scope_kind='model' AND p.scope_id=$5) OR (p.scope_kind='runner_pool' AND p.scope_id=$6))) OR ($5<>'' AND EXISTS(SELECT 1 FROM connections c WHERE c.org_id=$1::uuid AND c.id::text=$5 AND c.state IN ('revoked','disabled'))) FROM organisations o JOIN repositories r ON r.org_id=o.id WHERE o.id=$1::uuid AND r.id=$2::uuid`, t.OrgID, t.RepositoryID, t.Recipe, t.CampaignID, t.ModelConnectionID, t.RunnerPoolID).Scan(&paused)
	if err != nil {
		return hidden(err)
	}
	if paused {
		return ErrPaused
	}
	return nil
}

func (s *Service) Get(ctx context.Context, session auth.Session, orgID, id string) (Task, error) {
	var result Task
	if !auth.ValidID(id) {
		return result, auth.ErrForbidden
	}
	err := s.auth.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		result, err = loadTask(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		if !auth.CanReadRepository(a, result.RepositoryID) {
			return auth.ErrForbidden
		}
		return nil
	})
	return result, err
}
func validTaskState(state string) bool {
	switch domain.TaskState(state) {
	case "", "active", domain.TaskQueued, domain.TaskReproducing, domain.TaskPlanning, domain.TaskRepairing, domain.TaskValidating, domain.TaskPublishing, domain.TaskCompleted, domain.TaskBlocked, domain.TaskFailed, domain.TaskCancelling, domain.TaskCancelled, domain.TaskReconciling:
		return true
	}
	return false
}

func (s *Service) List(ctx context.Context, session auth.Session, orgID, state string, limit int, cursor string) (domain.Page[Task], error) {
	page := domain.Page[Task]{Items: []Task{}}
	if limit < 1 || limit > 200 || (cursor != "" && !auth.ValidID(cursor)) || !validTaskState(state) {
		return page, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		rows, err := tx.Query(ctx, `SELECT `+taskColumns+` FROM workflow_tasks WHERE org_id=$1 AND ($2='' OR (created_at,id)<(SELECT c.created_at,c.id FROM workflow_tasks c WHERE c.org_id=$1 AND c.id=nullif($2,'')::uuid)) AND ($3 OR repository_id::text=ANY($4::text[])) AND ($5='' OR state=$5 OR $5='active' AND state IN ('queued','reproducing','planning','repairing','validating','publishing')) ORDER BY created_at DESC,id DESC LIMIT $6`, orgID, cursor, a.AllRepositories, a.RepositoryIDs, state, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t, err := scanTask(rows)
			if err != nil {
				return err
			}
			page.Items = append(page.Items, t)
		}
		return rows.Err()
	})
	page.Complete = len(page.Items) <= limit
	if !page.Complete {
		page.Items = page.Items[:limit]
		page.NextCursor = page.Items[limit-1].ID
	}
	return page, err
}

func setTaskState(ctx context.Context, tx pgx.Tx, t *Task, state domain.TaskState, reason, requestID string) error {
	t.State = state
	t.Reason = reason
	t.Version++
	if _, err := tx.Exec(ctx, `UPDATE workflow_tasks SET state=$3,reason=$4,version=$5,cancel_version=$6,policy_hash=$7,cancellation_requested=$8 WHERE org_id=$1 AND id=$2`, t.OrgID, t.ID, t.State, t.Reason, t.Version, t.CancelVersion, t.PolicyHash, t.CancellationRequested); err != nil {
		return err
	}
	return emitTask(ctx, tx, *t, "task."+string(state), requestID)
}

func (s *Service) Cancel(ctx context.Context, session auth.Session, orgID, id string, expected int64, requestID string) (Task, error) {
	var t Task
	if !auth.ValidID(id) || expected < 1 {
		return t, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		t, err = loadTask(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		if !canManage(a, t.RepositoryID) {
			return auth.ErrForbidden
		}
		if t.Version != expected {
			return auth.ErrConflict
		}
		if t.State == domain.TaskCompleted || t.State == domain.TaskCancelled || t.State == domain.TaskFailed {
			return auth.ErrConflict
		}
		var running bool
		if err = tx.QueryRow(ctx, `SELECT state='running' FROM workflow_jobs WHERE org_id=$1 AND task_id=$2 FOR UPDATE`, orgID, id).Scan(&running); err != nil {
			return err
		}
		t.CancelVersion++
		t.CancellationRequested = true
		if err = checkUncertain(ctx, tx, orgID, id); errors.Is(err, ErrReconciliation) && !running {
			if _, err = tx.Exec(ctx, `UPDATE workflow_jobs SET state='reconciling',fence=fence+1 WHERE org_id=$1 AND task_id=$2`, orgID, id); err != nil {
				return err
			}
			return userTransition(ctx, tx, &t, a.UserID, domain.TaskReconciling, "Cancellation requested; external outcome remains unknown", requestID)
		} else if err != nil && !errors.Is(err, ErrReconciliation) {
			return err
		}
		if !running {
			if _, err = tx.Exec(ctx, `UPDATE workflow_jobs SET state='cancelled',fence=fence+1 WHERE org_id=$1 AND task_id=$2`, orgID, id); err != nil {
				return err
			}
			return userTransition(ctx, tx, &t, a.UserID, domain.TaskCancelled, "Pending work cancelled; recorded external outcomes retained", requestID)
		}
		return userTransition(ctx, tx, &t, a.UserID, domain.TaskCancelling, "Cancellation requested; runner or external outcome not yet confirmed", requestID)
	})
	return t, err
}

func (s *Service) Resume(ctx context.Context, session auth.Session, orgID, id string, expected int64, requestID string) (Task, error) {
	var t Task
	if !auth.ValidID(id) || expected < 1 {
		return t, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		t, err = loadTask(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		if !canManage(a, t.RepositoryID) {
			return auth.ErrForbidden
		}
		if t.Version != expected {
			return auth.ErrConflict
		}
		if t.State != domain.TaskBlocked && t.State != domain.TaskFailed && t.State != domain.TaskReconciling {
			return auth.ErrConflict
		}
		if err = checkPauses(ctx, tx, t); err != nil {
			return err
		}
		if err = checkUncertain(ctx, tx, orgID, id); err != nil {
			return err
		}
		hash, err := s.checkPolicy(ctx, tx, t, "resume")
		if err != nil {
			return err
		}
		t.PolicyHash = hash
		t.CancellationRequested = false
		var attempts int
		if err = tx.QueryRow(ctx, `SELECT attempts FROM workflow_jobs WHERE org_id=$1 AND task_id=$2 FOR UPDATE`, orgID, id).Scan(&attempts); err != nil {
			return err
		}
		if attempts >= t.MaxAttempts {
			return auth.ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE workflow_jobs SET state='queued',available_at=clock_timestamp(),lease_owner='',lease_expires_at=NULL WHERE org_id=$1 AND task_id=$2`, orgID, id); err != nil {
			return err
		}
		return userTransition(ctx, tx, &t, a.UserID, domain.TaskQueued, "Resumed with current policy; new attempt required", requestID)
	})
	return t, err
}

func (s *Service) SetPause(ctx context.Context, session auth.Session, orgID string, p Pause, expected int64, requestID string) (Pause, error) {
	if expected < 0 || p.ID == "" || len(p.ID) > 100 {
		return p, auth.ErrInvalid
	}
	switch p.Kind {
	case "organisation":
		if p.ID != orgID {
			return p, auth.ErrInvalid
		}
	case "recipe":
		if strings.TrimSpace(p.ID) == "" {
			return p, auth.ErrInvalid
		}
	case "repository", "campaign", "model", "runner_pool":
		if !auth.ValidID(p.ID) {
			return p, auth.ErrInvalid
		}
	default:
		return p, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		if p.Kind == "model" || p.Kind == "campaign" || p.Kind == "runner_pool" {
			if err := s.scopeExists(ctx, tx, orgID, p.Kind, p.ID); err != nil {
				return err
			}
		}
		var version int64
		err := tx.QueryRow(ctx, `SELECT version FROM workflow_pauses WHERE org_id=$1 AND scope_kind=$2 AND scope_id=$3`, orgID, p.Kind, p.ID).Scan(&version)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if version != expected {
			return auth.ErrConflict
		}
		p.Version = version + 1
		if _, err = tx.Exec(ctx, `INSERT INTO workflow_pauses(org_id,scope_kind,scope_id,paused,version) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,scope_kind,scope_id) DO UPDATE SET paused=EXCLUDED.paused,version=EXCLUDED.version`, orgID, p.Kind, p.ID, p.Paused, p.Version); err != nil {
			return err
		}
		if p.Kind == "organisation" {
			_, err = tx.Exec(ctx, `UPDATE organisations SET paused=$2,version=version+1 WHERE id=$1`, orgID, p.Paused)
		} else if p.Kind == "repository" {
			tag, e := tx.Exec(ctx, `UPDATE repositories SET paused=$3,version=version+1 WHERE org_id=$1 AND id=$2`, orgID, p.ID, p.Paused)
			err = e
			if err == nil && tag.RowsAffected() != 1 {
				return auth.ErrForbidden
			}
		}
		if err != nil {
			return err
		}
		if p.Paused {
			if err = s.revokePausedTx(ctx, tx, orgID, p, requestID); err != nil {
				return err
			}
		}
		if err = audit(ctx, tx, orgID, a.UserID, "automation.pause_changed", p.Kind+":"+p.ID, requestID, p.Version); err != nil {
			return err
		}
		repoID := ""
		if p.Kind == "repository" {
			repoID = p.ID
		}
		return EmitTx(ctx, tx, domain.Event{OrgID: orgID, RepositoryID: repoID, Type: "automation.paused", AggregateType: p.Kind, AggregateID: orgID, AggregateVersion: p.Version, RequestID: requestID, DataVersion: 1, Data: mustJSON(p)})
	})
	return p, err
}
func mustJSON(value any) json.RawMessage { b, _ := json.Marshal(value); return b }

func audit(ctx context.Context, tx pgx.Tx, orgID, actor, action, object, requestID string, version int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, domain.NewID(), orgID, actor, action, object, requestID, mustJSON(map[string]int64{"version": version}))
	return err
}
func userTransition(ctx context.Context, tx pgx.Tx, t *Task, actor string, state domain.TaskState, reason, requestID string) error {
	if err := setTaskState(ctx, tx, t, state, reason, requestID); err != nil {
		return err
	}
	return audit(ctx, tx, t.OrgID, actor, "task."+string(state), t.ID, requestID, t.Version)
}

type ScopeCheck func(context.Context, pgx.Tx, string, string, string) error

func (s *Service) RegisterScopeCheck(check ScopeCheck) {
	s.scopeMu.Lock()
	defer s.scopeMu.Unlock()
	s.scopeCheck = check
}
func (s *Service) scopeExists(ctx context.Context, tx pgx.Tx, orgID, kind, id string) error {
	if kind == "model" {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connections WHERE org_id=$1 AND id=$2 AND kind IN ('model','agent'))`, orgID, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return auth.ErrForbidden
		}
		return nil
	}
	s.scopeMu.RLock()
	check := s.scopeCheck
	s.scopeMu.RUnlock()
	if check == nil {
		return auth.ErrInvalid
	}
	return check(ctx, tx, orgID, kind, id)
}

func (s *Service) revokePausedTx(ctx context.Context, tx pgx.Tx, orgID string, p Pause, requestID string) error {
	rows, err := tx.Query(ctx, `UPDATE workflow_jobs j SET state='blocked',fence=j.fence+1,lease_owner='',lease_expires_at=NULL FROM workflow_tasks t WHERE j.org_id=$1 AND j.state='running' AND t.org_id=j.org_id AND t.id=j.task_id AND (($2='organisation') OR ($2='repository' AND t.repository_id::text=$3) OR ($2='recipe' AND t.recipe=$3) OR ($2='model' AND t.model_connection_id::text=$3) OR ($2='campaign' AND t.campaign_id::text=$3) OR ($2='runner_pool' AND t.runner_pool_id::text=$3)) RETURNING j.id::text,j.task_id::text`, orgID, p.Kind, p.ID)
	if err != nil {
		return err
	}
	type affected struct{ job, task string }
	var changed []affected
	for rows.Next() {
		var a affected
		if err = rows.Scan(&a.job, &a.task); err != nil {
			rows.Close()
			return err
		}
		changed = append(changed, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, a := range changed {
		if _, err = tx.Exec(ctx, `UPDATE workflow_attempts SET state='lost',ended_at=clock_timestamp() WHERE org_id=$1 AND job_id=$2 AND state='running'`, orgID, a.job); err != nil {
			return err
		}
		task, err := loadTask(ctx, tx, orgID, a.task)
		if err != nil {
			return err
		}
		state := domain.TaskBlocked
		reason := "Paused; lease revoked, prior runner stop unconfirmed; review before resume"
		if uncertainty := checkUncertain(ctx, tx, orgID, a.task); errors.Is(uncertainty, ErrReconciliation) || task.State == domain.TaskPublishing || task.State == domain.TaskCancelling {
			state = domain.TaskReconciling
			reason = "Paused with external or cancellation outcome requiring reconciliation"
		} else if uncertainty != nil {
			return uncertainty
		}
		if err = setTaskState(ctx, tx, &task, state, reason, requestID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) CheckProposalTx(ctx context.Context, tx pgx.Tx, t Task) (string, error) {
	if err := checkPauses(ctx, tx, t); err != nil {
		return "", err
	}
	return s.checkPolicy(ctx, tx, t, "enqueue")
}
