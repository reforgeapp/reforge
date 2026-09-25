package autopilot

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/domain"
	"reforge/internal/inventory"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/maintenance/recipes"
	"reforge/internal/maintenance/repair"
	"reforge/internal/mergecontrol"
	"reforge/internal/policy"
	"reforge/internal/store"
)

const active = `('queued','reproducing','planning','repairing','validating','publishing','reconciling','cancelling')`

type Settings struct {
	Enabled   bool       `json:"enabled"`
	Status    string     `json:"status"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	Active    int        `json:"active"`
	Queued    int        `json:"queued"`
	Skipped   int        `json:"skipped"`
	Version   int64      `json:"version"`
}

type Service struct {
	db       *store.Store
	auth     *auth.Service
	repairs  *repair.Service
	merges   *mergecontrol.Service
	budgets  *budget.Service
	policies *policy.Service
	Scan     func(context.Context, auth.Session, string, string, string) error
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
	err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE t.state IN `+active+`),count(*) FILTER (WHERE a.outcome='queued'),count(*) FILTER (WHERE a.outcome='skipped') FROM autopilot_attempts a LEFT JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id WHERE a.org_id=$1`, org).Scan(&out.Active, &out.Queued, &out.Skipped)
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
			if err := s.Step(step, org); err != nil && ctx.Err() == nil {
				slog.WarnContext(ctx, "autopilot step failed", "org_id", org, "error", err)
			}
			cancel()
		}
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
	finding, repository, name, previous, ecosystem string
	version                             int64
	runs                                int
}

func (s *Service) status(ctx context.Context, org, message string) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE autopilot_settings SET status=$2,checked_at=clock_timestamp() WHERE org_id=$1`, org, message)
		return err
	})
}

func (s *Service) record(ctx context.Context, org string, c candidate, task, outcome, reason string, retry time.Duration) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO autopilot_attempts(org_id,finding_id,finding_version,task_id,outcome,reason,retry_after,runs) VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,CASE WHEN $7::bigint>0 THEN clock_timestamp()+make_interval(secs=>$7::bigint) END,CASE WHEN $5='queued' THEN 1 ELSE 0 END) ON CONFLICT(org_id,finding_id,finding_version) DO UPDATE SET task_id=coalesce(EXCLUDED.task_id,autopilot_attempts.task_id),outcome=EXCLUDED.outcome,reason=EXCLUDED.reason,retry_after=EXCLUDED.retry_after,runs=autopilot_attempts.runs+EXCLUDED.runs,updated_at=clock_timestamp()`, org, c.finding, c.version, task, outcome, reason, int64(retry.Seconds()))
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

func (s *Service) Step(ctx context.Context, org string) error {
	session, sc, err := s.session(ctx, org)
	if err != nil {
		return s.status(ctx, org, "Import a repository to start")
	}
	if sc.enabled {
		if err = s.merge(ctx, session, org); err != nil {
			return err
		}
	}
	if err = s.reconcile(ctx, session, org); err != nil {
		slog.WarnContext(ctx, "autopilot reconcile failed", "org_id", org, "error", err)
	}
	var filter []string
	if !sc.enabled {
		filter = sc.requested
	}
	var busy bool
	var model, route, headroom, blocked string
	var next *candidate
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM autopilot_attempts a JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id WHERE a.org_id=$1 AND t.state IN `+active+`)`, org).Scan(&busy); err != nil || busy {
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
		err := tx.QueryRow(ctx, `SELECT c.id::text,r.name FROM connections c JOIN budget_routes r ON r.org_id=c.org_id AND r.connection_id=c.id WHERE c.org_id=$1 AND c.kind='model' AND c.state='healthy' AND r.config->>'mode'='priced' AND NOT coalesce((r.config->>'paused')::boolean,false) ORDER BY c.created_at,r.name LIMIT 1`, org).Scan(&model, &route)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		var c candidate
		err = tx.QueryRow(ctx, `SELECT f.id::text,f.version,f.repository_id::text,r.name,coalesce((SELECT a.task_id::text FROM autopilot_attempts a WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version),''),coalesce((SELECT a.runs FROM autopilot_attempts a WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version),0),coalesce(f.evidence->'dependencies'->0->>'ecosystem','') FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE f.org_id=$1 AND f.state='open' AND f.last_seen>clock_timestamp()-interval '14 minutes' AND coalesce((f.evidence->>'complete')::boolean,false) AND jsonb_array_length(coalesce(f.evidence->'blockers','[]'))=0 AND r.accessible AND NOT r.archived AND NOT r.paused AND NOT EXISTS(SELECT 1 FROM autopilot_attempts a LEFT JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version AND NOT (a.outcome='retry' AND a.retry_after<=clock_timestamp() OR a.outcome='queued' AND t.state IN ('failed','cancelled') AND a.runs<3 AND a.updated_at<clock_timestamp()-interval '10 minutes' AND NOT EXISTS(SELECT 1 FROM repair_runs rr WHERE rr.org_id=a.org_id AND rr.task_id=a.task_id AND rr.report->>'reason' LIKE 'Skipped:%'))) AND NOT EXISTS(SELECT 1 FROM maintenance_repairs m JOIN workflow_tasks t ON t.org_id=m.org_id AND t.id=m.task_id WHERE m.org_id=f.org_id AND m.finding_id=f.id AND t.state NOT IN ('failed','cancelled')) AND ($2::text[] IS NULL OR f.repository_id::text=ANY($2)) ORDER BY f.repository_id::text=ANY($3) DESC,f.first_seen,f.id LIMIT 1`, org, filter, sc.requested).Scan(&c.finding, &c.version, &c.repository, &c.name, &c.previous, &c.runs, &c.ecosystem)
		if errors.Is(err, pgx.ErrNoRows) {
			var count int
			err = tx.QueryRow(ctx, `SELECT count(*),coalesce(min(f.evidence->'blockers'->>0),'') FROM maintenance_findings f WHERE f.org_id=$1 AND f.state='open' AND jsonb_array_length(coalesce(f.evidence->'blockers','[]'))>0`, org).Scan(&count, &blocked)
			if count > 0 {
				blocked = strconv.Itoa(count) + " blocked: " + blocked
			}
			return err
		}
		next = &c
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
			_, err := tx.Exec(ctx, `DELETE FROM autopilot_requests WHERE org_id=$1 AND repository_id::text=ANY($2)`, org, sc.requested)
			return err
		}), s.status(ctx, org, message))
	}
	return s.queue(ctx, session, org, *next, model, route)
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
	for _, recipe := range recipesFor(c.ecosystem) {
		in := repair.Input{FindingID: c.finding, FindingVersion: c.version, Recipe: recipe, ModelConnectionID: model, ModelRoute: route, RunnerPoolID: pool}
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
			return s.record(ctx, org, c, "", "skipped", preview.Blockers[0], 0)
		}
		in.PlanDigest = preview.Context.Plan.Digest
		in.IdempotencyKey = "autopilot/" + c.finding + "/" + preview.Context.Plan.Digest[:16] + "/" + strconv.Itoa(c.runs+1)
		run, err := s.repairs.Enqueue(ctx, session, org, in, "autopilot")
		if err != nil {
			return errors.Join(s.record(ctx, org, c, "", "retry", err.Error(), 15*time.Minute), s.status(ctx, org, "Could not queue a fix for "+c.name))
		}
		return errors.Join(s.record(ctx, org, c, run.Task.ID, "queued", "", 0), s.status(ctx, org, "Fixing a finding in "+c.name))
	}
	return s.record(ctx, org, c, "", "skipped", "No supported recipe for this repository", 0)
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

func (s *Service) merge(ctx context.Context, session auth.Session, org string) error {
	var finding, repo, change string
	var version int64
	var resolved policy.Resolved
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT a.finding_id::text,a.finding_version,rr.repository_id::text,rr.native_change->>'id' FROM autopilot_attempts a JOIN repair_runs rr ON rr.org_id=a.org_id AND rr.task_id=a.task_id WHERE a.org_id=$1 AND rr.state='published' AND coalesce(rr.native_change->>'state','open')='open' AND (a.merge_after IS NULL OR a.merge_after<=clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM merge_operations m WHERE m.org_id=rr.org_id AND m.repository_id=rr.repository_id AND m.change_id=rr.native_change->>'id') ORDER BY a.updated_at LIMIT 1`, org).Scan(&finding, &version, &repo, &change)
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
			_, err := tx.Exec(ctx, `UPDATE autopilot_attempts SET merge_reason=$4,merge_after=clock_timestamp()+make_interval(secs=>$5::bigint),updated_at=clock_timestamp() WHERE org_id=$1 AND finding_id=$2 AND finding_version=$3`, org, finding, version, reason, int64(after.Seconds()))
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
		return later(err.Error(), 5*time.Minute)
	}
	if gate.Decision.Outcome != "allow" {
		reason := "Waiting for merge checks"
		if len(gate.Decision.Blockers) > 0 {
			reason = gate.Decision.Blockers[0]
		}
		return later(reason, 5*time.Minute)
	}
	if _, err = s.merges.Request(ctx, session, org, gate.ID, "autopilot/"+finding+"/"+change, "autopilot"); err != nil {
		return later(err.Error(), 5*time.Minute)
	}
	return later("Merge requested", 24*time.Hour)
}

func recipesFor(ecosystem string) []string {
	switch strings.ToLower(ecosystem) {
	case "go", "gomod", "go_modules":
		return []string{"go"}
	case "npm", "yarn", "pnpm", "javascript":
		return []string{"javascript"}
	case "pip", "pypi", "python", "poetry":
		return []string{"python"}
	}
	return []string{"go", "javascript", "python"}
}
