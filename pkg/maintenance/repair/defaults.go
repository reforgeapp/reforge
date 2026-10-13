package repair

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

type Defaults struct {
	ModelConnectionID string   `json:"model_connection_id"`
	ModelRoute        string   `json:"model_route"`
	RunnerPoolID      string   `json:"runner_pool_id"`
	Model             string   `json:"model"`
	Pool              string   `json:"pool"`
	Ceiling           *int64   `json:"micro_usd,omitempty"`
	Period            string   `json:"period,omitempty"`
	Blockers          []string `json:"blockers"`
}

func (s *Service) Defaults(ctx context.Context, session auth.Session, org, repo string) (Defaults, error) {
	out := Defaults{Blockers: []string{}}
	if !auth.ValidID(repo) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, repo) {
			return auth.ErrForbidden
		}
		resolved, err := s.policies.ResolveTx(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		if len(resolved.Problems) > 0 || resolved.Paused || slices.Contains(resolved.Policy.Deny, policy.Repair) {
			out.Blockers = append(out.Blockers, "Set up maintenance to propose fixes")
			return nil
		}
		if resolved.Policy.Defaults.Model == "" || resolved.Policy.Defaults.Route == "" {
			out.Blockers = append(out.Blockers, "Choose an approved default in maintenance setup")
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT c.id::text,c.name,r.name FROM connections c JOIN budget_routes r ON r.org_id=c.org_id AND r.connection_id=c.id AND r.model=c.settings->>'model' WHERE c.org_id=$1 AND c.kind='model' AND c.state='healthy' AND c.settings->>'model'=$2 AND r.name=$3 AND ($4='' OR c.id::text=$4) AND `+budget.AutomaticRouteSQL("r")+` ORDER BY c.id LIMIT 2`, org, resolved.Policy.Defaults.Model, resolved.Policy.Defaults.Route, resolved.Policy.Defaults.ConnectionID)
		if err != nil {
			return err
		}
		candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ ID, Name, Route string }])
		if err != nil {
			return err
		}
		if len(candidates) != 1 {
			out.Blockers = append(out.Blockers, "Reconnect approved model or choose its exact connection in maintenance setup")
		} else {
			out.ModelConnectionID, out.Model, out.ModelRoute = candidates[0].ID, candidates[0].Name, candidates[0].Route
		}
		err = tx.QueryRow(ctx, `SELECT p.id::text,p.name FROM runner_pools p JOIN runner_pool_repositories pr ON pr.org_id=p.org_id AND pr.pool_id=p.id WHERE p.org_id=$1 AND pr.repository_id=$2 AND p.state='active' AND EXISTS(SELECT 1 FROM runners n WHERE n.org_id=p.org_id AND n.pool_id=p.id AND n.state='active' AND n.credential_expires_at>now() AND n.last_seen_at>now()-interval '5 minutes') ORDER BY p.builtin DESC,p.created_at LIMIT 1`, org, repo).Scan(&out.RunnerPoolID, &out.Pool)
		if errors.Is(err, pgx.ErrNoRows) {
			out.Blockers = append(out.Blockers, "Operator: restore this repository's runner")
		} else if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT (caps->>'micro_usd')::bigint,period FROM budget_limits WHERE org_id=$1 AND (scope_kind='organisation' OR scope_kind='repository' AND scope_id=$2) AND caps->>'micro_usd' IS NOT NULL ORDER BY scope_kind='repository' DESC LIMIT 1`, org, repo).Scan(&out.Ceiling, &out.Period)
		if errors.Is(err, pgx.ErrNoRows) {
			out.Blockers = append(out.Blockers, "Set a maintenance spending ceiling")
		} else if err != nil {
			return err
		}
		return nil
	})
	return out, err
}
