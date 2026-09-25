package autopilot

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/domain"
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
}

func New(db *store.Store, identity *auth.Service, repairs *repair.Service, merges *mergecontrol.Service, budgets *budget.Service, policies *policy.Service) *Service {
	return &Service{db, identity, repairs, merges, budgets, policies}
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

func (s *Service) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		var orgs []string
		err := pgx.BeginFunc(ctx, s.db.Pool, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT org_id::text FROM autopilot_settings WHERE enabled ORDER BY org_id`)
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
	finding, repository, name string
	version                   int64
}

func (s *Service) status(ctx context.Context, org, message string) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE autopilot_settings SET status=$2,checked_at=clock_timestamp() WHERE org_id=$1`, org, message)
		return err
	})
}

func (s *Service) record(ctx context.Context, org string, c candidate, task, outcome, reason string, retry time.Duration) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO autopilot_attempts(org_id,finding_id,finding_version,task_id,outcome,reason,retry_after) VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,CASE WHEN $7::bigint>0 THEN clock_timestamp()+make_interval(secs=>$7::bigint) END) ON CONFLICT(org_id,finding_id,finding_version) DO UPDATE SET task_id=EXCLUDED.task_id,outcome=EXCLUDED.outcome,reason=EXCLUDED.reason,retry_after=EXCLUDED.retry_after,updated_at=clock_timestamp()`, org, c.finding, c.version, task, outcome, reason, int64(retry.Seconds()))
		return err
	})
}

func (s *Service) session(ctx context.Context, org string) (auth.Session, error) {
	var user string
	var repos []string
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT enabled_by::text FROM autopilot_settings WHERE org_id=$1 AND enabled`, org).Scan(&user); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text FROM repositories WHERE org_id=$1 AND accessible AND NOT archived ORDER BY id LIMIT 2000`, org)
		if err != nil {
			return err
		}
		repos, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return auth.Session{}, err
	}
	return auth.NewAutomationSession(org, user, org, repos, func(ctx context.Context, tx pgx.Tx) error {
		var enabled bool
		if err := tx.QueryRow(ctx, `SELECT enabled FROM autopilot_settings WHERE org_id=$1 AND enabled_by=$2 FOR SHARE`, org, user).Scan(&enabled); err != nil || !enabled {
			return auth.ErrForbidden
		}
		return nil
	})
}

func (s *Service) Step(ctx context.Context, org string) error {
	session, err := s.session(ctx, org)
	if err != nil {
		return s.status(ctx, org, "Import a repository to start")
	}
	if err = s.merge(ctx, session, org); err != nil {
		return err
	}
	var busy bool
	var model, route, headroom string
	var next *candidate
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM autopilot_attempts a JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id WHERE a.org_id=$1 AND t.state IN `+active+`)`, org).Scan(&busy); err != nil || busy {
			return err
		}
		switch err := s.budgets.HeadroomTx(ctx, tx, org); {
		case errors.Is(err, budget.ErrUnknown):
			headroom = "Set a spend and concurrency limit in Usage → Budgets"
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
		err = tx.QueryRow(ctx, `SELECT f.id::text,f.version,f.repository_id::text,r.name FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE f.org_id=$1 AND f.state='open' AND f.last_seen>clock_timestamp()-interval '14 minutes' AND r.accessible AND NOT r.archived AND NOT r.paused AND NOT EXISTS(SELECT 1 FROM autopilot_attempts a WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version AND (a.outcome<>'retry' OR a.retry_after>clock_timestamp())) AND NOT EXISTS(SELECT 1 FROM maintenance_repairs m JOIN workflow_tasks t ON t.org_id=m.org_id AND t.id=m.task_id WHERE m.org_id=f.org_id AND m.finding_id=f.id AND t.state NOT IN ('failed','cancelled')) ORDER BY f.first_seen,f.id LIMIT 1`, org).Scan(&c.finding, &c.version, &c.repository, &c.name)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
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
		return s.status(ctx, org, "No findings to fix")
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
	for _, recipe := range []string{"go", "javascript", "python"} {
		in := repair.Input{FindingID: c.finding, FindingVersion: c.version, Recipe: recipe, ModelConnectionID: model, ModelRoute: route, RunnerPoolID: pool}
		preview, err := s.repairs.Preview(ctx, session, org, in)
		if errors.Is(err, recipes.ErrUnsupported) {
			continue
		}
		if err != nil {
			return errors.Join(s.record(ctx, org, c, "", "retry", err.Error(), 15*time.Minute), s.status(ctx, org, "Could not prepare a fix for "+c.name))
		}
		if len(preview.Blockers) > 0 {
			return s.record(ctx, org, c, "", "skipped", preview.Blockers[0], 0)
		}
		in.PlanDigest = preview.Context.Plan.Digest
		in.IdempotencyKey = "autopilot/" + c.finding + "/" + preview.Context.Plan.Digest[:16]
		run, err := s.repairs.Enqueue(ctx, session, org, in, "autopilot")
		if err != nil {
			return errors.Join(s.record(ctx, org, c, "", "retry", err.Error(), 15*time.Minute), s.status(ctx, org, "Could not queue a fix for "+c.name))
		}
		return errors.Join(s.record(ctx, org, c, run.Task.ID, "queued", "", 0), s.status(ctx, org, "Fixing a finding in "+c.name))
	}
	return s.record(ctx, org, c, "", "skipped", "No supported recipe for this repository", 0)
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
