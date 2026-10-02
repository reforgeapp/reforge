package autopilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"reforge/internal/heartbeat"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/deployment"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/gitops"
	"reforge/internal/inventory"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/maintenance/recipes"
	"reforge/internal/maintenance/repair"
	"reforge/internal/mergecontrol"
	"reforge/internal/policy"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

const (
	escalateAfter = 4
	maxRuns       = 6
	maxRebases    = 3
	turnCost      = "(coalesce((r.config->>'input_micro_usd_per_million')::numeric,0)*0.04+coalesce((r.config->>'output_micro_usd_per_million')::numeric,0)*0.001+coalesce((r.config->>'request_micro_usd')::numeric,0))"
)

const active = `('queued','reproducing','planning','repairing','validating','publishing','reconciling','cancelling')`

type Settings struct {
	Enabled   bool       `json:"enabled"`
	Status    string     `json:"status"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	Active    int        `json:"active"`
	Queued    int        `json:"queued"`
	Skipped   int        `json:"skipped"`
	Blocked   int        `json:"blocked"`
	Awaiting  int        `json:"awaiting"`
	Version   int64      `json:"version"`
}

type Service struct {
	db          *store.Store
	auth        *auth.Service
	repairs     *repair.Service
	merges      *mergecontrol.Service
	budgets     *budget.Service
	policies    *policy.Service
	Scan        func(context.Context, auth.Session, string, string, string) error
	Tasks       *workflow.Service
	Deployments *deployment.Service
	Promotions  *gitops.Service
}

func New(db *store.Store, identity *auth.Service, repairs *repair.Service, merges *mergecontrol.Service, budgets *budget.Service, policies *policy.Service) *Service {
	return &Service{db: db, auth: identity, repairs: repairs, merges: merges, budgets: budgets, policies: policies}
}

func read(ctx context.Context, tx pgx.Tx, org string) (Settings, error) {
	var out Settings
	err := tx.QueryRow(ctx, `SELECT enabled,status,checked_at,version FROM autopilot_settings WHERE org_id=$1`, org).Scan(&out.Enabled, &out.Status, &out.CheckedAt, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE t.state IN `+active+`),count(*) FILTER (WHERE a.outcome='queued'),count(*) FILTER (WHERE a.outcome='skipped'),count(*) FILTER (WHERE a.outcome='blocked'),count(*) FILTER (WHERE a.merge_reason LIKE 'Awaiting human%') FROM autopilot_attempts a LEFT JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id WHERE a.org_id=$1`, org).Scan(&out.Active, &out.Queued, &out.Skipped, &out.Blocked, &out.Awaiting)
	return out, err
}

func (s *Service) Get(ctx context.Context, session auth.Session, org string) (Settings, error) {
	var out Settings
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = read(ctx, tx, org)
		return err
	})
	return out, err
}

func (s *Service) Put(ctx context.Context, session auth.Session, org string, enabled bool, expected int64, request string) (Settings, error) {
	var out Settings
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner || !a.AllRepositories {
			return auth.ErrForbidden
		}
		current, err := read(ctx, tx, org)
		if err != nil {
			return err
		}
		if current.Version != expected {
			return auth.ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO autopilot_settings(org_id,enabled,enabled_by,status,version) VALUES($1,$2,$3,'',1) ON CONFLICT(org_id) DO UPDATE SET enabled=EXCLUDED.enabled,enabled_by=EXCLUDED.enabled_by,status='',checked_at=NULL,version=autopilot_settings.version+1`, org, enabled, a.UserID); err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]bool{"enabled": enabled})
		if _, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,'autopilot.changed',$4,$5,$6)`, domain.NewID(), org, a.UserID, org, request, data); err != nil {
			return err
		}
		out, err = read(ctx, tx, org)
		return err
	})
	return out, err
}

func (s *Service) Request(ctx context.Context, session auth.Session, org, repo, request string) error {
	if !auth.ValidID(repo) {
		return auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) || a.Role != domain.Owner && a.Role != domain.Admin && a.Role != domain.Maintainer {
			return auth.ErrForbidden
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repositories WHERE org_id=$1 AND id=$2 AND accessible AND NOT archived)`, org, repo).Scan(&exists); err != nil || !exists {
			return errors.Join(err, auth.ErrForbidden)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO autopilot_requests(org_id,repository_id,requested_by) VALUES($1,$2,$3) ON CONFLICT(org_id,repository_id) DO UPDATE SET requested_by=EXCLUDED.requested_by,requested_at=clock_timestamp()`, org, repo, a.UserID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO autopilot_settings(org_id) VALUES($1) ON CONFLICT DO NOTHING`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM autopilot_attempts a USING maintenance_findings f WHERE a.org_id=$1 AND f.org_id=a.org_id AND f.id=a.finding_id AND f.repository_id=$2 AND NOT EXISTS(SELECT 1 FROM workflow_tasks t WHERE t.org_id=a.org_id AND t.id=a.task_id AND t.state IN `+active+`)`, org, repo); err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]string{"repository_id": repo})
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,'autopilot.run_requested',$4,$5,$6)`, domain.NewID(), org, a.UserID, repo, request, data)
		return err
	})
	if err == nil && s.Scan != nil {
		if scanErr := s.Scan(ctx, session, org, repo, request); scanErr != nil {
			slog.WarnContext(ctx, "autopilot run scan not started", "org_id", org, "repository_id", repo, "error", scanErr)
		}
	}
	return err
}

func (s *Service) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		var orgs []string
		err := pgx.BeginFunc(ctx, s.db.Pool, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT org_id::text FROM autopilot_settings WHERE enabled UNION SELECT org_id::text FROM autopilot_requests ORDER BY 1`)
			if err != nil {
				return err
			}
			orgs, err = pgx.CollectRows(rows, pgx.RowTo[string])
			return err
		})
		if err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "autopilot listing failed", "error", err)
		}
		for _, org := range orgs {
			step, cancel := context.WithTimeout(ctx, 8*time.Minute)
			if e := s.Step(step, org); e != nil && ctx.Err() == nil {
				slog.WarnContext(ctx, "autopilot step failed", "org_id", org, "error", e)
				err = errors.Join(err, e)
			}
			cancel()
		}
		heartbeat.Beat("autopilot", 10*time.Minute, err)
		timer := time.NewTimer(20 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return ctx.Err()
}

type candidate struct {
	finding, repository, name, previous, category, ecosystem, requestGeneration string
	version                                                                     int64
	runs                                                                        int
}

func repairIdempotencyKey(c candidate, digest string) string {
	return "autopilot/" + c.finding + "/" + strconv.FormatInt(c.version, 10) + "/" + digest[:16] + "/" + strconv.Itoa(c.runs+1) + "/" + c.requestGeneration
}

func (s *Service) capability() string {
	var images map[string]string
	if s.repairs != nil {
		images = s.repairs.Recipes()
	}
	body, _ := json.Marshal([]any{recipes.CurrentVersion, images})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:8])
}

func (s *Service) status(ctx context.Context, org, message string) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE autopilot_settings SET status=$2,checked_at=clock_timestamp() WHERE org_id=$1`, org, message)
		return err
	})
}

func (s *Service) record(ctx context.Context, org string, c candidate, task, outcome, reason string, retry time.Duration) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO autopilot_attempts(org_id,finding_id,finding_version,task_id,outcome,reason,retry_after,runs,capability) VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,CASE WHEN $7::bigint>0 THEN clock_timestamp()+make_interval(secs=>$7::bigint) END,CASE WHEN $5='queued' THEN 1 ELSE 0 END,$8) ON CONFLICT(org_id,finding_id,finding_version) DO UPDATE SET task_id=coalesce(EXCLUDED.task_id,autopilot_attempts.task_id),outcome=EXCLUDED.outcome,reason=EXCLUDED.reason,retry_after=EXCLUDED.retry_after,runs=autopilot_attempts.runs+EXCLUDED.runs,capability=EXCLUDED.capability,updated_at=clock_timestamp()`, org, c.finding, c.version, task, outcome, reason, int64(retry.Seconds()), s.capability())
		return err
	})
}

func (s *Service) recordRebase(ctx context.Context, org string, c candidate, outcome, reason string, retry time.Duration) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO autopilot_attempts(org_id,finding_id,finding_version,outcome,reason,retry_after,runs) VALUES($1,$2,$3,$4,$5,clock_timestamp()+make_interval(secs=>$6::bigint),1) ON CONFLICT(org_id,finding_id,finding_version) DO UPDATE SET outcome=EXCLUDED.outcome,reason=EXCLUDED.reason,retry_after=EXCLUDED.retry_after,runs=autopilot_attempts.runs+1,updated_at=clock_timestamp()`, org, c.finding, c.version, outcome, reason, int64(retry.Seconds()))
		return err
	})
}

type scope struct {
	enabled   bool
	requested []string
}

func (s *Service) session(ctx context.Context, org string) (auth.Session, scope, error) {
	var user string
	var sc scope
	var repos []string
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT enabled,coalesce(enabled_by::text,'') FROM autopilot_settings WHERE org_id=$1`, org).Scan(&sc.enabled, &user)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if !sc.enabled {
			if err = tx.QueryRow(ctx, `SELECT requested_by::text FROM autopilot_requests WHERE org_id=$1 AND requested_at>clock_timestamp()-interval '1 day' ORDER BY requested_at DESC LIMIT 1`, org).Scan(&user); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `SELECT repository_id::text FROM autopilot_requests WHERE org_id=$1 AND requested_at>clock_timestamp()-interval '1 day' AND ($2 OR requested_by=$3::uuid) ORDER BY repository_id`, org, sc.enabled, user)
		if err != nil {
			return err
		}
		if sc.requested, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return err
		}
		if !sc.enabled {
			repos = sc.requested
			return nil
		}
		rows, err = tx.Query(ctx, `SELECT id::text FROM repositories WHERE org_id=$1 AND accessible AND NOT archived ORDER BY id LIMIT 2000`, org)
		if err != nil {
			return err
		}
		repos, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return auth.Session{}, sc, err
	}
	session, err := auth.NewAutomationSession(org, user, org, repos, func(ctx context.Context, tx pgx.Tx) error {
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM autopilot_settings WHERE org_id=$1 AND enabled AND enabled_by=$2) OR EXISTS(SELECT 1 FROM autopilot_requests WHERE org_id=$1 AND requested_by=$2)`, org, user).Scan(&allowed); err != nil || !allowed {
			return auth.ErrForbidden
		}
		return nil
	})
	return session, sc, err
}

func releaseTerminalRepairsTx(ctx context.Context, tx pgx.Tx, org string) error {
	if _, err := tx.Exec(ctx, `UPDATE repair_runs rr SET native_change=jsonb_set(rr.native_change,'{state}',to_jsonb(coalesce((SELECT 'merged'::text FROM merge_operations o WHERE o.org_id=rr.org_id AND o.repository_id=rr.repository_id AND o.change_id=rr.native_change->>'id' AND o.state='merged' LIMIT 1),(SELECT c.snapshot->>'state' FROM inventory_changes c WHERE c.org_id=rr.org_id AND c.repository_id=rr.repository_id AND c.snapshot->>'id'=rr.native_change->>'id' AND c.snapshot->>'state' IN ('closed','merged') LIMIT 1)))),version=rr.version+1,updated_at=clock_timestamp() WHERE rr.org_id=$1 AND rr.state='published' AND rr.native_change IS NOT NULL AND coalesce(rr.native_change->>'state','open')='open' AND (EXISTS(SELECT 1 FROM merge_operations o WHERE o.org_id=rr.org_id AND o.repository_id=rr.repository_id AND o.change_id=rr.native_change->>'id' AND o.state='merged') OR EXISTS(SELECT 1 FROM inventory_changes c WHERE c.org_id=rr.org_id AND c.repository_id=rr.repository_id AND c.snapshot->>'id'=rr.native_change->>'id' AND c.snapshot->>'state' IN ('closed','merged')))`, org); err != nil {
		return err
	}
	if err := verifyMergesTx(ctx, tx, org); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE maintenance_findings f SET state='resolved',reason='Fix merged',version=f.version+1 FROM repair_runs rr WHERE rr.org_id=f.org_id AND rr.finding_id=f.id AND f.org_id=$1 AND f.source='repository_review' AND f.state='open' AND rr.native_change->>'state'='merged'`, org); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE maintenance_repairs m SET active=false FROM repair_runs rr WHERE rr.org_id=m.org_id AND rr.task_id=m.task_id AND m.org_id=$1 AND m.active AND rr.native_change IS NOT NULL AND (rr.native_change->>'state' IN ('closed','merged') OR EXISTS(SELECT 1 FROM inventory_changes c WHERE c.org_id=rr.org_id AND c.repository_id=rr.repository_id AND c.snapshot->>'id'=rr.native_change->>'id' AND c.snapshot->>'state' IN ('closed','merged')) OR EXISTS(SELECT 1 FROM merge_operations o WHERE o.org_id=rr.org_id AND o.repository_id=rr.repository_id AND o.change_id=rr.native_change->>'id' AND o.state='merged'))`, org)
	return err
}

func verifyMergesTx(ctx context.Context, tx pgx.Tx, org string) error {
	if _, err := tx.Exec(ctx, `UPDATE repair_runs SET post_merge_state='pending',post_merge_since=clock_timestamp() WHERE org_id=$1 AND post_merge_state='' AND native_change->>'state'='merged'`, org); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `WITH regressed AS (UPDATE repair_runs rr SET post_merge_state='regressed' WHERE rr.org_id=$1 AND rr.post_merge_state='pending' AND EXISTS(SELECT 1 FROM maintenance_findings f WHERE f.org_id=rr.org_id AND f.repository_id=rr.repository_id AND f.source='native_ci' AND f.state='open' AND f.first_seen>rr.post_merge_since) RETURNING rr.finding_id) UPDATE maintenance_findings f SET state='open',reason='Regressed after its fix merged: default branch CI failed',version=f.version+1 FROM regressed WHERE f.org_id=$1 AND f.id=regressed.finding_id AND f.source IN ('repository_review','forge_issue') AND f.state<>'open'`, org); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE repair_runs rr SET post_merge_state='verified' WHERE rr.org_id=$1 AND rr.post_merge_state='pending' AND EXISTS(SELECT 1 FROM maintenance_scans s WHERE s.org_id=rr.org_id AND s.repository_id=rr.repository_id AND s.state='complete' AND s.observed_at>rr.post_merge_since+interval '30 minutes')`, org)
	return err
}

func (s *Service) Step(ctx context.Context, org string) error {
	session, sc, err := s.session(ctx, org)
	if err != nil {
		return s.status(ctx, org, "Import a repository to start")
	}
	if sc.enabled {
		if err = s.merge(ctx, session, org); err != nil {
			return err
		}
		if err = s.unblock(ctx, session, org); err != nil {
			slog.WarnContext(ctx, "autopilot unblock failed", "org_id", org, "error", err)
		}
		if err = s.rebase(ctx, org); err != nil {
			slog.WarnContext(ctx, "autopilot rebase failed", "org_id", org, "error", err)
		}
		if err = s.mergeBot(ctx, session, org); err != nil {
			slog.WarnContext(ctx, "autopilot bot merge failed", "org_id", org, "error", err)
		}
	}
	if sc.enabled {
		if err = s.promote(ctx, session, org); err != nil {
			slog.WarnContext(ctx, "autopilot promotion failed", "org_id", org, "error", err)
		}
	}
	if err = s.observeDeployments(ctx, session, org); err != nil {
		slog.WarnContext(ctx, "autopilot deployment observation failed", "org_id", org, "error", err)
	}
	if err = s.reconcile(ctx, session, org); err != nil {
		slog.WarnContext(ctx, "autopilot reconcile failed", "org_id", org, "error", err)
	}
	if err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := releaseTerminalRepairsTx(ctx, tx, org); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE autopilot_attempts a SET outcome='blocked',reason='Fix attempts exhausted: '||coalesce(nullif((SELECT rr.report->>'reason' FROM repair_runs rr WHERE rr.org_id=a.org_id AND rr.task_id=a.task_id),''),nullif(t.reason,''),'no reason recorded'),capability=$3,updated_at=clock_timestamp() FROM workflow_tasks t WHERE t.org_id=a.org_id AND t.id=a.task_id AND a.org_id=$1 AND a.outcome='queued' AND t.state IN ('failed','cancelled') AND a.runs>=$2`, org, maxRuns, s.capability())
		return err
	}); err != nil {
		return err
	}
	var filter []string
	if !sc.enabled {
		filter = sc.requested
	}
	var busy bool
	var model, route, headroom, blocked string
	var next *candidate
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM autopilot_attempts a JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id WHERE a.org_id=$1 AND t.state IN `+active+`)>=coalesce((SELECT (caps->>'concurrency')::bigint FROM budget_limits WHERE org_id=$1 AND scope_kind='organisation'),1)`, org).Scan(&busy); err != nil || busy {
			return err
		}
		switch err := s.budgets.HeadroomTx(ctx, tx, org); {
		case errors.Is(err, budget.ErrUnknown):
			headroom = "Set a spend limit in Usage → Budgets"
			return nil
		case errors.Is(err, budget.ErrRevoked):
			headroom = "Budget is paused"
			return nil
		case errors.Is(err, budget.ErrCapacity):
			headroom = "Budget used up for this period"
			return nil
		case err != nil:
			return err
		}
		var err error
		model, route, headroom, err = s.pickModel(ctx, tx, org, false, "")
		if err != nil || model == "" {
			return err
		}
		var c candidate
		err = tx.QueryRow(ctx, `SELECT f.id::text,f.version,f.repository_id::text,r.name,coalesce((SELECT a.task_id::text FROM autopilot_attempts a WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version),''),coalesce((SELECT a.runs FROM autopilot_attempts a WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version),0),f.category,coalesce(f.evidence->'dependencies'->0->>'ecosystem',''),coalesce((SELECT to_char(q.requested_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISSUS') FROM autopilot_requests q WHERE q.org_id=f.org_id AND q.repository_id=f.repository_id),'') FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE f.org_id=$1 AND f.state='open' AND f.last_seen>clock_timestamp()-interval '1 hour' AND coalesce((f.evidence->>'complete')::boolean,false) AND jsonb_array_length(coalesce(f.evidence->'blockers','[]'))=0 AND r.accessible AND NOT r.archived AND NOT r.paused AND NOT EXISTS(SELECT 1 FROM autopilot_attempts a LEFT JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version AND NOT (a.outcome='retry' AND a.retry_after<=clock_timestamp() OR a.outcome='blocked' AND a.capability<>$5 OR a.outcome='queued' AND t.state IN ('failed','cancelled') AND a.runs<$4 AND a.updated_at<clock_timestamp()-interval '10 minutes' AND NOT EXISTS(SELECT 1 FROM repair_runs rr WHERE rr.org_id=a.org_id AND rr.task_id=a.task_id AND rr.report->>'reason' LIKE 'Skipped:%' AND rr.updated_at>clock_timestamp()-interval '6 hours') OR a.outcome='queued' AND t.state='completed' AND a.runs<$4 AND EXISTS(SELECT 1 FROM maintenance_repairs m WHERE m.org_id=a.org_id AND m.task_id=a.task_id AND NOT m.active))) AND NOT EXISTS(SELECT 1 FROM maintenance_repairs m WHERE m.org_id=f.org_id AND m.finding_id=f.id AND m.evidence_digest=f.evidence_digest AND m.supersession IS NOT NULL) AND NOT EXISTS(SELECT 1 FROM maintenance_repairs m JOIN workflow_tasks t ON t.org_id=m.org_id AND t.id=m.task_id WHERE m.org_id=f.org_id AND m.finding_id=f.id AND m.active AND t.state NOT IN ('failed','cancelled')) AND ($2::text[] IS NULL OR f.repository_id::text=ANY($2)) ORDER BY f.repository_id::text=ANY($3) DESC,f.source='native_ci' DESC,coalesce(f.evidence->>'ownership','')='reforge' DESC,f.first_seen,f.id LIMIT 1`, org, filter, sc.requested, maxRuns, s.capability()).Scan(&c.finding, &c.version, &c.repository, &c.name, &c.previous, &c.runs, &c.category, &c.ecosystem, &c.requestGeneration)
		if errors.Is(err, pgx.ErrNoRows) {
			var count int
			err = tx.QueryRow(ctx, `SELECT count(*),coalesce(min(coalesce(f.evidence->'blockers'->>0,a.reason)),'') FROM maintenance_findings f LEFT JOIN autopilot_attempts a ON a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version AND a.outcome='blocked' WHERE f.org_id=$1 AND f.state='open' AND (jsonb_array_length(coalesce(f.evidence->'blockers','[]'))>0 OR a.finding_id IS NOT NULL)`, org).Scan(&count, &blocked)
			if count > 0 {
				blocked = strconv.Itoa(count) + " blocked: " + blocked
			}
			return err
		}
		next = &c
		if err == nil && c.runs >= escalateAfter {
			previousModel := ""
			if c.previous != "" {
				err = tx.QueryRow(ctx, `SELECT coalesce(model_connection_id::text,'') FROM workflow_tasks WHERE org_id=$1 AND id=$2`, org, c.previous).Scan(&previousModel)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
			model, route, headroom, err = s.pickModel(ctx, tx, org, true, previousModel)
		}
		return err
	})
	if err != nil {
		return err
	}
	switch {
	case busy:
		return s.status(ctx, org, "Fixing a finding")
	case headroom != "":
		return s.status(ctx, org, headroom)
	case model == "":
		return s.status(ctx, org, "Add an AI model with pricing")
	case next == nil:
		message := "No findings to fix"
		if blocked != "" {
			message = blocked
		}
		return errors.Join(s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM autopilot_requests q WHERE q.org_id=$1 AND q.repository_id::text=ANY($2) AND EXISTS(SELECT 1 FROM maintenance_scans s WHERE s.org_id=q.org_id AND s.repository_id=q.repository_id AND s.state='complete' AND s.observed_at>q.requested_at) AND NOT EXISTS(SELECT 1 FROM workflow_tasks t WHERE t.org_id=q.org_id AND t.repository_id=q.repository_id AND t.state IN `+active+`) AND NOT EXISTS(SELECT 1 FROM autopilot_attempts a JOIN maintenance_findings f ON f.org_id=a.org_id AND f.id=a.finding_id AND f.version=a.finding_version LEFT JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id WHERE a.org_id=q.org_id AND f.repository_id=q.repository_id AND f.state='open' AND (a.outcome='retry' OR a.outcome='queued' AND t.state IN ('failed','cancelled') AND a.runs<$3))`, org, sc.requested, maxRuns)
			return err
		}), s.status(ctx, org, message))
	}
	return s.queue(ctx, session, org, *next, model, route)
}

func (s *Service) pickModel(ctx context.Context, tx pgx.Tx, org string, largest bool, avoid string) (string, string, string, error) {
	order := "(" + turnCost + "+1)*(1+(SELECT count(*) FROM workflow_tasks t WHERE t.org_id=c.org_id AND t.model_connection_id=c.id AND t.state IN " + active + "))"
	if largest {
		order = turnCost + " DESC"
	}
	rows, err := tx.Query(ctx, `SELECT c.id::text,r.name FROM connections c JOIN budget_routes r ON r.org_id=c.org_id AND r.connection_id=c.id WHERE c.org_id=$1 AND c.kind='model' AND c.state='healthy' AND r.config->>'mode'='priced' AND NOT coalesce((r.config->>'paused')::boolean,false) AND NOT coalesce((SELECT m.state='failed' AND m.failure<>'orphaned' FROM model_turns m JOIN budget_reservations b ON b.org_id=m.org_id AND b.id=m.reservation_id WHERE m.org_id=c.org_id AND b.record->>'connection_id'=c.id::text AND m.created_at>clock_timestamp()-interval '30 minutes' ORDER BY m.created_at DESC LIMIT 1),false) ORDER BY `+order+`,c.created_at,r.name`, org)
	if err != nil {
		return "", "", "", err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Connection, Route string }])
	if err != nil {
		return "", "", "", err
	}
	var fallback struct{ Connection, Route string }
	for _, c := range candidates {
		switch err := s.budgets.ConnectionHeadroomTx(ctx, tx, org, c.Connection); {
		case err == nil:
			if c.Connection != avoid {
				return c.Connection, c.Route, "", nil
			}
			if fallback.Connection == "" {
				fallback = c
			}
		case !errors.Is(err, budget.ErrCapacity) && !errors.Is(err, budget.ErrRevoked):
			return "", "", "", err
		}
	}
	if fallback.Connection != "" {
		return fallback.Connection, fallback.Route, "", nil
	}
	if len(candidates) > 0 {
		return "", "", "Model connection budgets used up", nil
	}
	return "", "", "", nil
}

func (s *Service) queue(ctx context.Context, session auth.Session, org string, c candidate, model, route string) error {
	var pool string
	var resolved policy.Resolved
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT p.id::text FROM runner_pools p JOIN runner_pool_repositories rp ON rp.org_id=p.org_id AND rp.pool_id=p.id WHERE p.org_id=$1 AND rp.repository_id=$2 AND p.state='active' ORDER BY p.builtin DESC,p.created_at LIMIT 1`, org, c.repository).Scan(&pool)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		resolved, err = s.policies.ResolveTx(ctx, tx, org, c.repository)
		return err
	})
	if err != nil {
		return err
	}
	if resolved.Hash == "" || len(resolved.Problems) > 0 || resolved.Paused || slices.Contains(resolved.Policy.Deny, policy.Repair) {
		return errors.Join(s.record(ctx, org, c, "", "retry", "Mode does not allow fixes", 10*time.Minute), s.status(ctx, org, "Set Mode to Propose fixes or higher"))
	}
	if pool == "" {
		return errors.Join(s.record(ctx, org, c, "", "retry", "No active runner pool includes "+c.name, 10*time.Minute), s.status(ctx, org, "No runner for "+c.name))
	}
	if c.category == "deploy_failure" {
		reason, err := s.rollback(ctx, session, org, c)
		if err != nil || reason == "" {
			return err
		}
		slog.InfoContext(ctx, "autopilot rollback unavailable; fixing forward", "org_id", org, "finding_id", c.finding, "reason", reason)
	}
	for _, recipe := range recipes.ForFinding(c.category, c.ecosystem) {
		in := repair.Input{FindingID: c.finding, FindingVersion: c.version, Recipe: recipe, ModelConnectionID: model, ModelRoute: route, RunnerPoolID: pool, Owner: true}
		preview, err := s.repairs.Preview(ctx, session, org, in)
		if errors.Is(err, recipes.ErrUnsupported) {
			continue
		}
		if err != nil {
			wait, message := 15*time.Minute, "Could not prepare a fix for "+c.name
			if errors.Is(err, discovery.ErrStale) || errors.Is(err, inventory.ErrStale) {
				wait, message = time.Minute, "Waiting for fresh data from "+c.name
			}
			return errors.Join(s.record(ctx, org, c, "", "retry", err.Error(), wait), s.status(ctx, org, message))
		}
		if len(preview.Blockers) > 0 {
			if duplicate := preview.Blockers[0] == repair.ErrDuplicateFix.Error(); duplicate || preview.Blockers[0] == repair.ErrFollowUpsExhausted.Error() {
				if err = s.repairs.CloseFix(ctx, session, org, preview.Context.Finding, duplicate); err != nil {
					return errors.Join(s.record(ctx, org, c, "", "retry", err.Error(), 15*time.Minute), s.status(ctx, org, "Could not close a fix on "+c.name))
				}
				return s.record(ctx, org, c, "", "skipped", "Closed: "+preview.Blockers[0], 0)
			}
			return s.record(ctx, org, c, "", "blocked", preview.Blockers[0], 0)
		}
		in.PlanDigest = preview.Context.Plan.Digest
		in.IdempotencyKey = repairIdempotencyKey(c, preview.Context.Plan.Digest)
		run, err := s.repairs.Enqueue(ctx, session, org, in, "autopilot")
		if err != nil {
			return errors.Join(s.record(ctx, org, c, "", "retry", err.Error(), 15*time.Minute), s.status(ctx, org, "Could not queue a fix for "+c.name))
		}
		return errors.Join(s.record(ctx, org, c, run.Task.ID, "queued", "", 0), s.status(ctx, org, "Fixing a finding in "+c.name))
	}
	return s.record(ctx, org, c, "", "blocked", "No supported recipe for this repository", 0)
}

func (s *Service) reconcile(ctx context.Context, session auth.Session, org string) error {
	var tasks []string
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT a.task_id::text FROM autopilot_attempts a JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id WHERE a.org_id=$1 AND t.state IN ('failed','cancelled') AND (EXISTS(SELECT 1 FROM model_turns m WHERE m.org_id=a.org_id AND m.task_id=a.task_id AND m.state IN ('unknown','dispatched')) OR EXISTS(SELECT 1 FROM maintenance_repairs r WHERE r.org_id=a.org_id AND r.task_id=a.task_id AND r.active))`, org)
		if err != nil {
			return err
		}
		tasks, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	for _, task := range tasks {
		run, e := s.repairs.Get(ctx, session, org, task)
		if e == nil {
			_, e = s.repairs.Reconcile(ctx, session, org, task, run.Version, "autopilot")
		}
		if e != nil {
			err = errors.Join(err, e)
		}
	}
	return err
}

func (s *Service) unblock(ctx context.Context, session auth.Session, org string) error {
	if s.Tasks == nil {
		return nil
	}
	type blockedTask struct {
		id          string
		version     int64
		attempts    int
		reconciling bool
		paused      bool
	}
	var tasks []blockedTask
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT t.id::text,t.version,(SELECT count(*) FROM workflow_attempts w WHERE w.org_id=t.org_id AND w.task_id=t.id),t.state='reconciling' FROM workflow_tasks t WHERE t.org_id=$1 AND t.state IN ('blocked','reconciling') ORDER BY t.created_at LIMIT 10`, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t blockedTask
			if err = rows.Scan(&t.id, &t.version, &t.attempts, &t.reconciling); err != nil {
				return err
			}
			tasks = append(tasks, t)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		rows.Close()
		for i := range tasks {
			if tasks[i].paused, err = s.Tasks.PausedTx(ctx, tx, org, tasks[i].id); err != nil {
				return err
			}
		}
		return nil
	})
	for _, t := range tasks {
		if t.paused {
			continue
		}
		if t.reconciling {
			run, e := s.repairs.Get(ctx, session, org, t.id)
			if e == nil {
				_, e = s.repairs.Reconcile(ctx, session, org, t.id, run.Version, "autopilot")
			}
			if e == nil {
				run, e = s.repairs.Get(ctx, session, org, t.id)
			}
			if e != nil {
				err = errors.Join(err, e)
				continue
			}
			t.version = run.Task.Version
		}
		if t.attempts < 3 {
			_, e := s.Tasks.Resume(ctx, session, org, t.id, t.version, "autopilot")
			if !errors.Is(e, workflow.ErrPolicy) {
				err = errors.Join(err, e)
				continue
			}
		}
		if _, e := s.Tasks.Cancel(ctx, session, org, t.id, t.version, "autopilot"); e != nil {
			err = errors.Join(err, e)
		}
	}
	return err
}

type rebaseCandidate struct {
	finding discovery.Finding
	runs    int
}

func loadRebaseCandidates(ctx context.Context, tx pgx.Tx, org string) ([]rebaseCandidate, error) {
	rows, err := tx.Query(ctx, `SELECT f.id::text,f.version,f.repository_id::text,f.evidence,coalesce(a.runs,0) FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id LEFT JOIN autopilot_attempts a ON a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version WHERE f.org_id=$1 AND f.state='open' AND f.last_seen>clock_timestamp()-interval '1 hour' AND f.evidence->>'bot'='dependabot' AND f.evidence->'blockers' ? $2 AND r.accessible AND NOT r.archived AND NOT r.paused AND (a.finding_id IS NULL OR a.outcome IN ('retry','rebase') AND (a.retry_after IS NULL OR a.retry_after<=clock_timestamp())) AND NOT EXISTS(SELECT 1 FROM autopilot_attempts pending JOIN maintenance_findings active ON active.org_id=pending.org_id AND active.id=pending.finding_id AND active.version=pending.finding_version WHERE pending.org_id=f.org_id AND active.repository_id=f.repository_id AND active.id<>f.id AND active.state='open' AND pending.outcome IN ('retry','rebase') AND pending.retry_after>clock_timestamp()) ORDER BY coalesce(a.retry_after,'-infinity'::timestamptz),f.first_seen,f.id LIMIT 1`, org, discovery.WaitingForRebase)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	waiting := []rebaseCandidate{}
	for rows.Next() {
		var entry rebaseCandidate
		var raw []byte
		if err = rows.Scan(&entry.finding.ID, &entry.finding.Version, &entry.finding.RepositoryID, &raw, &entry.runs); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &entry.finding.Evidence); err != nil {
			return nil, err
		}
		waiting = append(waiting, entry)
	}
	return waiting, rows.Err()
}

func (s *Service) rebase(ctx context.Context, org string) error {
	var waiting []rebaseCandidate
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var err error
		waiting, err = loadRebaseCandidates(ctx, tx, org)
		return err
	})
	if err != nil {
		return err
	}
	for _, entry := range waiting {
		f := entry.finding
		c := candidate{finding: f.ID, version: f.Version}
		if entry.runs >= maxRebases {
			err = errors.Join(err, s.record(ctx, org, c, "", "skipped", "Dependabot rebase did not clear this pull request after 3 attempts; manually rebase or close it", 0))
			continue
		}
		if f.Evidence.Change == nil {
			continue
		}
		if e := s.repairs.CommandBot(ctx, org, f.Evidence.ConnectionID, *f.Evidence.Change, "@dependabot rebase"); e != nil {
			err = errors.Join(err, e, s.recordRebase(ctx, org, c, "retry", "Dependabot rebase: "+e.Error(), 15*time.Minute))
			continue
		}
		err = errors.Join(err, s.recordRebase(ctx, org, c, "rebase", "Dependabot rebase: request accepted", time.Hour))
	}
	return err
}

func (s *Service) merge(ctx context.Context, session auth.Session, org string) error {
	var finding, repo, change, task, head, proof string
	var adopted, reviewOnly bool
	var version int64
	var resolved policy.Resolved
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		candidate, err := selectMergeCandidate(ctx, tx, org)
		finding, version, repo, change, task, head, adopted, proof = candidate.finding, candidate.version, candidate.repository, candidate.change, candidate.task, candidate.head, candidate.adopted, candidate.proof
		reviewOnly = candidate.reviewOnly
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		resolved, err = s.policies.ResolveTx(ctx, tx, org, repo)
		return err
	})
	if err != nil || change == "" {
		return err
	}
	later := func(reason string, after time.Duration) error {
		return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			if adopted {
				_, err := tx.Exec(ctx, `INSERT INTO autopilot_bot_merges(org_id,repository_id,change_id,head_sha,reason,merge_after) VALUES($1,$2,$3,$4,'Adopted existing Reforge repair: '||$5,clock_timestamp()+make_interval(secs=>$6::bigint)) ON CONFLICT(org_id,repository_id,change_id) DO UPDATE SET head_sha=EXCLUDED.head_sha,reason=EXCLUDED.reason,merge_after=EXCLUDED.merge_after,updated_at=clock_timestamp()`, org, repo, change, head, reason, int64(after.Seconds()))
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE autopilot_attempts SET merge_reason=$4,merge_after=clock_timestamp()+make_interval(secs=>$5::bigint),updated_at=clock_timestamp() WHERE org_id=$1 AND finding_id=$2 AND finding_version=$3`, org, finding, version, reason, int64(after.Seconds()))
			return err
		})
	}
	if slices.Contains(resolved.Policy.Deny, policy.Merge) {
		return later("Mode does not allow merging", 10*time.Minute)
	}
	if reviewOnly {
		return later("Awaiting human review: new validation must be reviewed before merge", 30*time.Minute)
	}
	if run, err := s.repairs.Get(ctx, session, org, task); err != nil {
		return later(err.Error(), 5*time.Minute)
	} else if run.Report != nil && repair.TouchesTests(run.Report.Patches) {
		return later("Awaiting human review: fix changes tests", 30*time.Minute)
	}
	if slices.Contains(resolved.Policy.ReviewProofs, proof) {
		return later("Awaiting human review: policy requires review for "+proof+" proof", 30*time.Minute)
	}
	if waiting, err := s.repairs.RefreshFix(ctx, session, org, task); err != nil {
		if errors.Is(err, repair.ErrBranchConflict) {
			return later("Repair branch conflicts; waiting for owner repair", 30*time.Minute)
		}
		return later("Branch refresh: "+err.Error(), 5*time.Minute)
	} else if waiting {
		return later("Waiting for repair branch refresh and fresh CI", 2*time.Minute)
	}
	if reason, err := s.repairs.CISuperset(ctx, org, task); err != nil || reason != "" {
		if err != nil {
			reason = err.Error()
		}
		return later(reason, 5*time.Minute)
	}
	gate, err := s.merges.Inspect(ctx, session, org, repo, change, "merge", "autopilot")
	if err == nil && !slices.Contains(gate.Snapshot.Rules.AllowedMergeMethods, "merge") && len(gate.Snapshot.Rules.AllowedMergeMethods) > 0 {
		gate, err = s.merges.Inspect(ctx, session, org, repo, change, gate.Snapshot.Rules.AllowedMergeMethods[0], "autopilot")
	}
	if err != nil {
		return later(err.Error(), 5*time.Minute)
	}
	if gate.Decision.Outcome != "allow" {
		reason := "Waiting for merge checks"
		if len(gate.Decision.Blockers) > 0 {
			reason = gate.Decision.Blockers[0]
		}
		if human := humanMergeReason(gate.Decision.Blockers); human != "" {
			return later(human, 6*time.Hour)
		}
		return later(reason, 5*time.Minute)
	}
	waiting, err := s.ownerRepairPinsTarget(ctx, org, repo, gate.Snapshot.Change.TargetBranch)
	if err != nil {
		return err
	}
	if waiting {
		return later("Waiting for active owner repairs pinned to this target", time.Minute)
	}
	if _, err = s.merges.Request(ctx, session, org, gate.ID, domain.StableID("autopilot-merge", finding, change, gate.ID), "autopilot"); err != nil {
		return later(err.Error(), 5*time.Minute)
	}
	return later("Merge requested", 24*time.Hour)
}

func humanMergeReason(blockers []string) string {
	for _, blocker := range blockers {
		switch {
		case strings.Contains(blocker, "execution_authority"):
			return "Awaiting human merge: branch protection needs approvals or bypass Reforge does not have"
		case strings.Contains(blocker, "native_reviews"):
			return "Awaiting human merge: required reviews are missing"
		case strings.Contains(blocker, "merge_authority"):
			return "Awaiting human merge: this repository keeps merge authority with people"
		}
	}
	return ""
}

type mergeCandidate struct {
	finding, repository, change, task, head, proof string
	version                                        int64
	adopted, reviewOnly                            bool
}

func selectMergeCandidate(ctx context.Context, tx pgx.Tx, org string) (mergeCandidate, error) {
	var candidate mergeCandidate
	err := tx.QueryRow(ctx, `SELECT rr.finding_id::text,rr.finding_version,rr.repository_id::text,rr.native_change->>'id',rr.task_id::text,coalesce(rr.native_change->>'head_sha',''),a.task_id IS NULL,coalesce(nullif(rr.context#>>'{plan,recipe,min_proof}',''),'tests'),coalesce(rr.context#>>'{plan,recipe,review_only}'='true',false) FROM repair_runs rr JOIN repositories r ON r.org_id=rr.org_id AND r.id=rr.repository_id LEFT JOIN autopilot_attempts a ON a.org_id=rr.org_id AND a.finding_id=rr.finding_id AND a.finding_version=rr.finding_version AND a.task_id=rr.task_id LEFT JOIN autopilot_bot_merges d ON d.org_id=rr.org_id AND d.repository_id=rr.repository_id AND d.change_id=rr.native_change->>'id' WHERE rr.org_id=$1 AND rr.state='published' AND coalesce(rr.native_change->>'state','open')='open' AND r.accessible AND NOT r.archived AND NOT r.paused AND (CASE WHEN a.task_id IS NOT NULL THEN a.merge_after ELSE d.merge_after END IS NULL OR CASE WHEN a.task_id IS NOT NULL THEN a.merge_after ELSE d.merge_after END<=clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM merge_operations m WHERE m.org_id=rr.org_id AND m.repository_id=rr.repository_id AND m.change_id=rr.native_change->>'id' AND m.state<>'blocked') AND NOT EXISTS(SELECT 1 FROM maintenance_repairs m WHERE m.org_id=rr.org_id AND m.task_id=rr.task_id AND m.supersession IS NOT NULL) ORDER BY coalesce(a.updated_at,d.updated_at,rr.created_at),rr.created_at,rr.task_id LIMIT 1`, org).Scan(&candidate.finding, &candidate.version, &candidate.repository, &candidate.change, &candidate.task, &candidate.head, &candidate.adopted, &candidate.proof, &candidate.reviewOnly)
	return candidate, err
}

func (s *Service) mergeBot(ctx context.Context, session auth.Session, org string) error {
	var repo, change, head, bot string
	var resolved policy.Resolved
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT c.repository_id::text,c.snapshot->>'id',c.snapshot->>'head_sha',b->>'kind' FROM inventory_changes c JOIN maintenance_configs m ON m.org_id=c.org_id AND m.repository_id=c.repository_id CROSS JOIN LATERAL jsonb_array_elements(m.trusted_bots) b JOIN repositories r ON r.org_id=c.org_id AND r.id=c.repository_id LEFT JOIN autopilot_bot_merges d ON d.org_id=c.org_id AND d.repository_id=c.repository_id AND d.change_id=c.snapshot->>'id' WHERE c.org_id=$1 AND c.snapshot->>'state'='open' AND b->>'actor_id'=c.snapshot->>'author_id' AND r.accessible AND NOT r.archived AND NOT r.paused AND NOT coalesce(d.head_sha=c.snapshot->>'head_sha' AND d.merge_after>clock_timestamp(),false) AND NOT EXISTS(SELECT 1 FROM merge_operations o WHERE o.org_id=c.org_id AND o.repository_id=c.repository_id AND o.change_id=c.snapshot->>'id' AND o.state<>'blocked') AND NOT EXISTS(SELECT 1 FROM maintenance_findings f WHERE f.org_id=c.org_id AND f.repository_id=c.repository_id AND f.source='forge_change' AND f.source_id=c.snapshot->>'id' AND f.state='open') ORDER BY d.updated_at NULLS FIRST,c.native_id LIMIT 1`, org).Scan(&repo, &change, &head, &bot)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		resolved, err = s.policies.ResolveTx(ctx, tx, org, repo)
		return err
	})
	if err != nil || change == "" {
		return err
	}
	later := func(reason string, after time.Duration) error {
		return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO autopilot_bot_merges(org_id,repository_id,change_id,head_sha,reason,merge_after) VALUES($1,$2,$3,$4,$5,clock_timestamp()+make_interval(secs=>$6::bigint)) ON CONFLICT(org_id,repository_id,change_id) DO UPDATE SET head_sha=EXCLUDED.head_sha,reason=EXCLUDED.reason,merge_after=EXCLUDED.merge_after,updated_at=clock_timestamp()`, org, repo, change, head, reason, int64(after.Seconds()))
			return err
		})
	}
	if slices.Contains(resolved.Policy.Deny, policy.Merge) {
		return later("Mode does not allow merging", 10*time.Minute)
	}
	gate, err := s.merges.Inspect(ctx, session, org, repo, change, "merge", "autopilot")
	if err == nil && !slices.Contains(gate.Snapshot.Rules.AllowedMergeMethods, "merge") && len(gate.Snapshot.Rules.AllowedMergeMethods) > 0 {
		gate, err = s.merges.Inspect(ctx, session, org, repo, change, gate.Snapshot.Rules.AllowedMergeMethods[0], "autopilot")
	}
	if err != nil {
		return later(err.Error(), 10*time.Minute)
	}
	if gate.Decision.Outcome == "allow" {
		waiting, err := s.ownerRepairPinsTarget(ctx, org, repo, gate.Snapshot.Change.TargetBranch)
		if err != nil {
			return err
		}
		if waiting {
			return later("Waiting for active owner repairs pinned to this target", time.Minute)
		}
		if _, err = s.merges.Request(ctx, session, org, gate.ID, domain.StableID("autopilot-bot-merge", repo, change, head, gate.ID), "autopilot"); err != nil {
			return later(err.Error(), 10*time.Minute)
		}
		return later("Merge requested", 24*time.Hour)
	}
	if bot == "dependabot" && gate.ReforgeEnforced && !gate.Snapshot.UpToDate {
		if err = s.repairs.CommandBot(ctx, org, gate.ConnectionID, gate.Snapshot.Change, "@dependabot rebase"); err != nil {
			return later(err.Error(), 10*time.Minute)
		}
		return later("Asked Dependabot to rebase", time.Hour)
	}
	if bot == "dependabot" && gate.ReforgeEnforced && mergecontrol.StaleChecks(gate.Snapshot.Checks, gate.Binding.Tested, time.Now()) {
		if err = s.repairs.CommandBot(ctx, org, gate.ConnectionID, gate.Snapshot.Change, "@dependabot recreate"); err != nil {
			return later(err.Error(), 10*time.Minute)
		}
		return later("Asked Dependabot to recreate for fresh checks", time.Hour)
	}
	reason, delay := botMergeRetry(gate)
	return later(reason, delay)
}

func botMergeRetry(gate mergecontrol.Gate) (string, time.Duration) {
	if _, pending := forge.MissingChecks(gate.Snapshot.TargetChecks, gate.Snapshot.Checks, discovery.BotUpdateJob); pending {
		return "Waiting for merge checks", time.Minute
	}
	reason := "Waiting for merge checks"
	if len(gate.Decision.Blockers) > 0 {
		reason = gate.Decision.Blockers[0]
	}
	return reason, 10 * time.Minute
}
