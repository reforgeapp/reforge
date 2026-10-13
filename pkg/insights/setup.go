package insights

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/policy"
)

type SetupStep struct {
	ID           string `json:"id"`
	Done         bool   `json:"done"`
	Reason       string `json:"reason,omitempty"`
	RepositoryID string `json:"repository_id,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
}

type Setup struct {
	Steps []SetupStep `json:"steps"`
}

type PolicyResolver func(context.Context, pgx.Tx, string, string) (policy.Resolved, error)

func (s *Service) Setup(ctx context.Context, session auth.Session, org string, resolve PolicyResolver, repairReady, builtin bool, selected ...string) (Setup, error) {
	out := Setup{}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		requested := ""
		if len(selected) > 0 {
			requested = selected[0]
		}
		if requested != "" && (!auth.ValidID(requested) || !auth.CanReadRepository(a, requested)) {
			return auth.ErrForbidden
		}
		var forge, model, priced, repository, scanned, runner, assigned string
		var budgeted bool
		var scanReason, scanRepository string
		err := tx.QueryRow(ctx, `WITH chosen AS (
SELECT id,connection_id FROM repositories WHERE org_id=$1 AND accessible AND NOT archived AND ($2 OR id=ANY($3::uuid[])) AND ($4='' OR id::text=$4) ORDER BY name LIMIT 1
), priced_model AS (
SELECT c.id FROM connections c JOIN budget_routes r ON r.org_id=c.org_id AND r.connection_id=c.id AND r.model=c.settings->>'model' WHERE c.org_id=$1 AND c.kind='model' AND c.state='healthy' AND r.config->>'mode'='priced' AND NOT coalesce((r.config->>'paused')::boolean,false) ORDER BY c.created_at,r.name LIMIT 1
) SELECT
coalesce((SELECT c.id::text FROM chosen r JOIN connections c ON c.org_id=$1 AND c.id=r.connection_id AND c.state='healthy'),CASE WHEN NOT EXISTS(SELECT 1 FROM chosen) THEN (SELECT id::text FROM connections WHERE org_id=$1 AND kind='forge' AND state='healthy' ORDER BY created_at LIMIT 1) END,''),
coalesce((SELECT id::text FROM priced_model),(SELECT id::text FROM connections WHERE org_id=$1 AND kind='model' AND state='healthy' ORDER BY created_at LIMIT 1),''),
coalesce((SELECT id::text FROM priced_model),''),
coalesce((SELECT id::text FROM chosen),''),
coalesce((SELECT s.repository_id::text FROM maintenance_scans s JOIN chosen r ON r.id=s.repository_id WHERE s.org_id=$1 AND s.state='complete' AND s.observed_at IS NOT NULL),''),
coalesce((SELECT n.id::text FROM runners n JOIN runner_pools p ON p.org_id=n.org_id AND p.id=n.pool_id JOIN runner_pool_repositories rp ON rp.org_id=p.org_id AND rp.pool_id=p.id JOIN chosen r ON r.id=rp.repository_id WHERE n.org_id=$1 AND n.state='active' AND p.state='active' AND n.credential_expires_at>now() AND n.last_seen_at>now()-interval '5 minutes' ORDER BY n.last_seen_at DESC LIMIT 1),''),
coalesce((SELECT rp.repository_id::text FROM runner_pool_repositories rp JOIN runner_pools p ON p.org_id=rp.org_id AND p.id=rp.pool_id JOIN chosen r ON r.id=rp.repository_id WHERE rp.org_id=$1 AND p.state='active' LIMIT 1),''),
coalesce((SELECT s.reason FROM maintenance_scans s JOIN chosen r ON r.id=s.repository_id WHERE s.org_id=$1 AND s.state<>'complete'),''),
coalesce((SELECT s.repository_id::text FROM maintenance_scans s JOIN chosen r ON r.id=s.repository_id WHERE s.org_id=$1 AND s.state<>'complete'),''),
EXISTS(SELECT 1 FROM budget_limits WHERE org_id=$1 AND scope_kind='organisation' AND NOT paused AND coalesce((caps->>'micro_usd')::bigint,0)>0)`,
			org, a.AllRepositories, a.RepositoryIDs, requested).Scan(&forge, &model, &priced, &repository, &scanned, &runner, &assigned, &scanReason, &scanRepository, &budgeted)
		if err != nil {
			return err
		}
		if requested != "" && repository == "" {
			return auth.ErrForbidden
		}
		policyReady, needsRepair := false, true
		if repository != "" {
			resolved, err := resolve(ctx, tx, org, repository)
			if err != nil {
				return err
			}
			policyReady = resolved.Hash != "" && len(resolved.Problems) == 0 && !resolved.Paused
			needsRepair = !policyReady || !slices.Contains(resolved.Policy.Deny, policy.Repair)
			if needsRepair {
				model, priced = "", ""
				err = tx.QueryRow(ctx, `SELECT c.id::text,CASE WHEN r.config->>'mode'='priced' AND NOT coalesce((r.config->>'paused')::boolean,false) THEN c.id::text ELSE '' END FROM connections c LEFT JOIN budget_routes r ON r.org_id=c.org_id AND r.connection_id=c.id AND r.model=c.settings->>'model' AND ($3='' OR r.name=$3) WHERE c.org_id=$1 AND c.state='healthy' AND c.kind='model' AND ($6='' OR c.id::text=$6) AND ($2='' OR c.settings->>'model'=$2) AND ($4::text[] IS NULL OR c.settings->>'model'=ANY($4)) AND ($5::text[] IS NULL OR r.name=ANY($5)) ORDER BY (r.config->>'mode'='priced' AND NOT coalesce((r.config->>'paused')::boolean,false)) DESC NULLS LAST,c.created_at LIMIT 1`, org, resolved.Policy.Defaults.Model, resolved.Policy.Defaults.Route, resolved.Policy.Allow.Models, resolved.Policy.Allow.Routes, resolved.Policy.Defaults.ConnectionID).Scan(&model, &priced)
				if err != nil && err != pgx.ErrNoRows {
					return err
				}
				var limits budget.Service
				err = limits.HeadroomTx(ctx, tx, org)
				budgeted = err == nil
				if err != nil && !errors.Is(err, budget.ErrUnknown) && !errors.Is(err, budget.ErrCapacity) && !errors.Is(err, budget.ErrRevoked) {
					return err
				}
			}
		}
		scan := SetupStep{ID: "scan", Done: scanned != "", RepositoryID: repository}
		if !scan.Done && scanReason != "" {
			scan.Reason, scan.RepositoryID = scanReason, scanRepository
		}
		out.Steps = []SetupStep{
			{ID: "forge", Done: forge != "", ConnectionID: forge},
			{ID: "repository", Done: repository != "", ConnectionID: forge},
			scan,
			{ID: "model", Done: model != "", ConnectionID: model},
			{ID: "pricing", Done: priced != "", ConnectionID: model},
			{ID: "budget", Done: budgeted},
			{ID: "runner", Done: runner != ""},
			{ID: "assignment", Done: assigned != "", RepositoryID: repository},
			{ID: "policy", Done: policyReady, RepositoryID: repository},
			{ID: "server", Done: repairReady},
		}
		if builtin {
			out.Steps = slices.DeleteFunc(out.Steps, func(step SetupStep) bool { return step.ID == "assignment" || step.ID == "server" })
		}
		if !needsRepair {
			out.Steps = slices.DeleteFunc(out.Steps, func(step SetupStep) bool {
				return step.ID != "forge" && step.ID != "repository" && step.ID != "scan" && step.ID != "policy"
			})
		}
		return nil
	})
	return out, err
}
