package autopilot

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
)

func (s *Service) RequestFinding(ctx context.Context, session auth.Session, org string, finding discovery.Finding, request string) error {
	c := candidate{finding: finding.ID, repository: finding.RepositoryID, version: finding.Version, category: finding.Category, runLimit: maxRuns, requestGeneration: "report/" + finding.ID + "/" + strconv.FormatInt(finding.Version, 10)}
	var pending bool
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.ValidID(finding.ID) || !auth.CanReadRepository(a, finding.RepositoryID) || a.Role != domain.Owner && a.Role != domain.Admin && a.Role != domain.Maintainer {
			return auth.ErrForbidden
		}
		var source string
		if err := tx.QueryRow(ctx, `SELECT r.name,f.source,EXISTS(SELECT 1 FROM maintenance_repairs m JOIN workflow_tasks t ON t.org_id=m.org_id AND t.id=m.task_id WHERE m.org_id=f.org_id AND m.finding_id=f.id AND m.active AND t.state NOT IN ('failed','cancelled')),coalesce((SELECT runs FROM autopilot_attempts a WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version),0),coalesce((SELECT max_runs FROM autopilot_attempts a WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version),6) FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE f.org_id=$1 AND f.id=$2 AND f.repository_id=$3 AND f.version=$4 AND f.state='open'`, org, finding.ID, finding.RepositoryID, finding.Version).Scan(&c.name, &source, &pending, &c.runs, &c.runLimit); err != nil {
			return err
		}
		if source != "user_report" {
			return auth.ErrInvalid
		}
		if _, err := tx.Exec(ctx, `INSERT INTO autopilot_requests(org_id,repository_id,requested_by) VALUES($1,$2,$3) ON CONFLICT(org_id,repository_id) DO UPDATE SET requested_by=EXCLUDED.requested_by,requested_at=clock_timestamp()`, org, finding.RepositoryID, a.UserID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO autopilot_settings(org_id) VALUES($1) ON CONFLICT DO NOTHING`, org); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,repository_id,actor_id,action,object_id,request_id) VALUES($1,$2,$3,$4,'finding.fix_requested',$5,$6)`, domain.NewID(), org, finding.RepositoryID, a.UserID, finding.ID, request)
		return err
	})
	if err != nil || pending {
		return err
	}
	if c.runs >= c.runLimit {
		return s.record(ctx, org, c, "", "blocked", "No verified fix yet; waiting for new evidence or repair tools", 0, "exhausted")
	}
	if s.Scan != nil {
		_ = s.Scan(ctx, session, org, finding.RepositoryID, request)
	}
	var model, route, reason string
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := s.budgets.HeadroomTx(ctx, tx, org); err != nil {
			if !errors.Is(err, budget.ErrCapacity) && !errors.Is(err, budget.ErrUnknown) && !errors.Is(err, budget.ErrRevoked) {
				return err
			}
			reason = "Set up maintenance or raise spending limit"
			return nil
		}
		var err error
		model, route, reason, err = s.pickModel(ctx, tx, org, false, "")
		return err
	})
	if err != nil {
		return err
	}
	if model == "" {
		if reason == "" {
			reason = "Connect an approved model and complete maintenance setup"
		}
		return s.record(ctx, org, c, "", "retry", reason, 10*time.Minute)
	}
	grant, err := s.automationSession(org, session.User.ID, []string{finding.RepositoryID})
	if err != nil {
		return err
	}
	if s.repairs == nil {
		return errors.New("maintenance execution unavailable")
	}
	return s.queue(ctx, grant, org, c, model, route)
}
