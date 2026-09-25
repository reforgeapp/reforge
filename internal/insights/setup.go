package insights

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/policy"
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

func (s *Service) Setup(ctx context.Context, session auth.Session, org string, resolve PolicyResolver, repairReady, builtin bool) (Setup, error) {
	out := Setup{}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var forge, model, priced, repository, scanned, runner, assigned string
		var budgeted bool
		var scanReason, scanRepository string
		err := tx.QueryRow(ctx, `SELECT
			coalesce((SELECT id::text FROM connections WHERE org_id=$1 AND kind='forge' AND state='healthy' ORDER BY created_at LIMIT 1),''),
			coalesce((SELECT id::text FROM connections WHERE org_id=$1 AND kind='model' AND state='healthy' ORDER BY created_at LIMIT 1),''),
			coalesce((SELECT c.id::text FROM connections c JOIN budget_routes r ON r.org_id=c.org_id AND r.connection_id=c.id WHERE c.org_id=$1 AND c.kind='model' AND c.state='healthy' AND r.config->>'mode'='priced' AND NOT coalesce((r.config->>'paused')::boolean,false) LIMIT 1),''),
			coalesce((SELECT id::text FROM repositories WHERE org_id=$1 AND accessible AND ($2 OR id=ANY($3::uuid[])) ORDER BY name LIMIT 1),''),
			coalesce((SELECT repository_id::text FROM maintenance_scans WHERE org_id=$1 AND state='complete' AND ($2 OR repository_id=ANY($3::uuid[])) LIMIT 1),''),
			coalesce((SELECT id::text FROM runners WHERE org_id=$1 AND state='active' AND last_seen_at>now()-interval '5 minutes' LIMIT 1),''),
			coalesce((SELECT r.repository_id::text FROM runner_pool_repositories r JOIN runner_pools p ON p.org_id=r.org_id AND p.id=r.pool_id WHERE r.org_id=$1 AND p.state='active' LIMIT 1),''),
			coalesce((SELECT reason FROM maintenance_scans WHERE org_id=$1 AND state<>'complete' AND reason<>'' AND ($2 OR repository_id=ANY($3::uuid[])) ORDER BY available_at DESC LIMIT 1),''),
			coalesce((SELECT repository_id::text FROM maintenance_scans WHERE org_id=$1 AND state<>'complete' AND reason<>'' AND ($2 OR repository_id=ANY($3::uuid[])) ORDER BY available_at DESC LIMIT 1),''),
			EXISTS(SELECT 1 FROM budget_limits WHERE org_id=$1 AND scope_kind='organisation' AND NOT paused AND caps->>'micro_usd' IS NOT NULL)`,
			org, a.AllRepositories, a.RepositoryIDs).Scan(&forge, &model, &priced, &repository, &scanned, &runner, &assigned, &scanReason, &scanRepository, &budgeted)
		if err != nil {
			return err
		}
		allowed := false
		if repository != "" {
			resolved, err := resolve(ctx, tx, org, repository)
			if err != nil {
				return err
			}
			allowed = resolved.Hash != "" && len(resolved.Problems) == 0 && !resolved.Paused && !slices.Contains(resolved.Policy.Deny, policy.Repair)
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
			{ID: "policy", Done: allowed},
			{ID: "server", Done: repairReady},
		}
		if builtin {
			out.Steps = slices.DeleteFunc(out.Steps, func(step SetupStep) bool { return step.ID == "assignment" || step.ID == "server" })
		}
		return nil
	})
	return out, err
}
