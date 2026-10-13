package autopilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/policy"
)

type MaintenanceCommand struct {
	ConnectionID string `json:"connection_id,omitempty"`
	Mode         string `json:"mode"`
	MicroUSD     int64  `json:"micro_usd"`
	Continuous   bool   `json:"continuous"`
	Proof        string `json:"proof,omitempty"`
}

type MaintenancePlan struct {
	RepositoryID    string                   `json:"repository_id"`
	Repository      string                   `json:"repository"`
	Mode            string                   `json:"mode"`
	MicroUSD        int64                    `json:"micro_usd"`
	Continuous      bool                     `json:"continuous"`
	Actions         []policy.Action          `json:"actions"`
	Decisions       map[policy.Action]string `json:"decisions"`
	Blockers        []string                 `json:"blockers"`
	Model           string                   `json:"model,omitempty"`
	ConnectionID    string                   `json:"connection_id,omitempty"`
	Route           string                   `json:"route,omitempty"`
	PoolID          string                   `json:"pool_id,omitempty"`
	BudgetPeriod    string                   `json:"budget_period"`
	OrgMicroUSD     *int64                   `json:"organisation_micro_usd,omitempty"`
	OrgPeriod       string                   `json:"organisation_period,omitempty"`
	CreatesOrgLimit bool                     `json:"creates_organisation_limit"`
	Ready           bool                     `json:"ready"`
	Proof           string                   `json:"proof"`
	policy          policy.Policy
	initialize      bool
}

func (s *Service) MaintenanceStatus(ctx context.Context, session auth.Session, org, repo string) (MaintenanceCommand, error) {
	out := MaintenanceCommand{Mode: "observe", MicroUSD: 10_000_000}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.ValidID(repo) || !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		r, err := s.policies.ResolveTx(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		if len(r.Problems) == 0 && !slices.Contains(r.Policy.Deny, policy.Repair) {
			out.ConnectionID = r.Policy.Defaults.ConnectionID
			out.Mode = "propose"
			if !slices.Contains(r.Policy.Deny, policy.Merge) {
				out.Mode = "merge"
				if !slices.Contains(r.Policy.Deny, policy.Deploy) {
					out.Mode = "deliver"
					if !slices.Contains(r.Policy.Deny, policy.Recover) {
						out.Mode = "recover"
					}
				}
			}
		}
		return tx.QueryRow(ctx, `SELECT coalesce((SELECT continuous FROM autopilot_requests WHERE org_id=$1 AND repository_id=$2),false),coalesce((SELECT (caps->>'micro_usd')::bigint FROM budget_limits WHERE org_id=$1 AND scope_kind='repository' AND scope_id=$2),$3)`, org, repo, out.MicroUSD).Scan(&out.Continuous, &out.MicroUSD)
	})
	return out, err
}

func (s *Service) maintenanceTx(ctx context.Context, tx pgx.Tx, a domain.Actor, repo string, in MaintenanceCommand) (MaintenancePlan, error) {
	out := MaintenancePlan{RepositoryID: repo, Mode: in.Mode, MicroUSD: in.MicroUSD, Continuous: in.Continuous, Actions: []policy.Action{}, Decisions: map[policy.Action]string{}, Blockers: []string{}}
	if !auth.ValidID(repo) || in.ConnectionID != "" && !auth.ValidID(in.ConnectionID) || !auth.CanReadRepository(a, repo) || in.MicroUSD < 0 || in.MicroUSD > 1_000_000_000_000 || in.Mode != "observe" && in.MicroUSD == 0 {
		return out, auth.ErrInvalid
	}
	var forgeHealthy, global, orgPolicy bool
	err := tx.QueryRow(ctx, `SELECT r.name,c.state='healthy',coalesce((SELECT enabled FROM autopilot_settings WHERE org_id=r.org_id),false),EXISTS(SELECT 1 FROM policy_bindings WHERE org_id=r.org_id AND scope_kind='organisation') FROM repositories r JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE r.org_id=$1 AND r.id=$2 AND r.accessible AND NOT r.archived`, a.OrgID, repo).Scan(&out.Repository, &forgeHealthy, &global, &orgPolicy)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, auth.ErrForbidden
	}
	if err != nil {
		return out, err
	}
	if !forgeHealthy {
		out.Blockers = append(out.Blockers, "Reconnect repository code host")
	}
	out.initialize = !orgPolicy && !global && a.Role == domain.Owner && a.AllRepositories
	current, err := s.policies.ResolveTx(ctx, tx, a.OrgID, repo)
	if err != nil {
		return out, err
	}
	parent := policy.Resolve(slices.DeleteFunc(slices.Clone(current.Layers), func(layer policy.Layer) bool { return layer.Scope.Kind == "repository" }), repo, current.PrimaryTeamID, current.ScopePaused)
	var connectionVersion, routeVersion, poolVersion, orgBudgetVersion, repoBudgetVersion int64
	out.BudgetPeriod = "monthly"
	if in.Mode != "observe" {
		rows, err := tx.Query(ctx, `SELECT c.id::text,c.settings->>'model',r.name,c.version,r.version FROM connections c JOIN budget_routes r ON r.org_id=c.org_id AND r.connection_id=c.id AND r.model=c.settings->>'model' WHERE c.org_id=$1 AND c.kind='model' AND c.state='healthy' AND `+budget.AutomaticRouteSQL("r")+` ORDER BY (c.settings->>'model'=$2 AND r.name=$3) DESC,c.created_at,r.name`, a.OrgID, current.Policy.Defaults.Model, current.Policy.Defaults.Route)
		if err != nil {
			return out, err
		}
		models, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct {
			Connection, Model, Route        string
			ConnectionVersion, RouteVersion int64
		}])
		if err != nil {
			return out, err
		}
		for _, m := range models {
			selected := in.ConnectionID
			if selected == "" {
				selected = current.Policy.Defaults.ConnectionID
			}
			if selected != "" && selected != m.Connection {
				continue
			}
			if parent.Policy.Allow.Models != nil && !slices.Contains(parent.Policy.Allow.Models, m.Model) || !policy.RouteAllowed(parent.Policy.Allow.Routes, m.Connection, m.Route) {
				continue
			}
			out.ConnectionID, out.Model, out.Route = m.Connection, m.Model, m.Route
			connectionVersion, routeVersion = m.ConnectionVersion, m.RouteVersion
			break
		}
		if out.Model == "" {
			out.Blockers = append(out.Blockers, "Operator: reconnect approved model or explicitly choose a replacement with known pricing")
		}
		err = tx.QueryRow(ctx, `SELECT p.id::text,p.version FROM runner_pools p WHERE p.org_id=$1 AND p.state='active' AND (p.builtin OR EXISTS(SELECT 1 FROM runner_pool_repositories rp WHERE rp.org_id=p.org_id AND rp.pool_id=p.id AND rp.repository_id=$2)) AND EXISTS(SELECT 1 FROM runners n WHERE n.org_id=p.org_id AND n.pool_id=p.id AND n.state='active' AND n.credential_expires_at>now() AND n.last_seen_at>now()-interval '5 minutes') ORDER BY p.builtin DESC,p.created_at LIMIT 1`, a.OrgID, repo).Scan(&out.PoolID, &poolVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			out.Blockers = append(out.Blockers, "Operator: restore a runner for this repository")
		} else if err != nil {
			return out, err
		}
		if s.repairs == nil || len(s.repairs.Recipes()) == 0 {
			out.Blockers = append(out.Blockers, "Operator: install maintenance execution images")
		}
		err = tx.QueryRow(ctx, `SELECT (caps->>'micro_usd')::bigint,period,version FROM budget_limits WHERE org_id=$1 AND scope_kind='organisation' AND scope_id=$1`, a.OrgID).Scan(&out.OrgMicroUSD, &out.OrgPeriod, &orgBudgetVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			if a.Role == domain.Owner && a.AllRepositories {
				out.CreatesOrgLimit = true
				out.OrgMicroUSD = &in.MicroUSD
				out.OrgPeriod = "monthly"
			} else {
				out.Blockers = append(out.Blockers, "Operator: set an organisation dollar ceiling")
			}
		} else if err != nil {
			return out, err
		} else {
			err = s.budgets.HeadroomTx(ctx, tx, a.OrgID)
			if errors.Is(err, budget.ErrUnknown) {
				out.Blockers = append(out.Blockers, "Operator: set an organisation dollar ceiling")
			} else if errors.Is(err, budget.ErrRevoked) || errors.Is(err, budget.ErrCapacity) {
				out.Blockers = append(out.Blockers, "Resume or raise organisation spending limit")
			} else if err != nil {
				return out, err
			}
		}
		err = tx.QueryRow(ctx, `SELECT period,version FROM budget_limits WHERE org_id=$1 AND scope_kind='repository' AND scope_id=$2`, a.OrgID, repo).Scan(&out.BudgetPeriod, &repoBudgetVersion)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
	}
	r, p, err := s.policies.MaintenanceTx(ctx, tx, a.OrgID, repo, in.Mode, out.Model, out.Route, out.ConnectionID, in.MicroUSD, out.initialize)
	if err != nil {
		return out, err
	}
	out.policy = p
	out.Blockers = append(out.Blockers, r.Problems...)
	if r.Paused {
		out.Blockers = append(out.Blockers, "Resume repository or organisation")
	}
	for _, action := range []policy.Action{policy.Read, policy.Repair, policy.Publish, policy.Merge, policy.Deploy, policy.Recover} {
		decision := policy.Evaluate(r, policy.Input{Action: action, Model: out.Model, Route: out.Route, Current: policy.Binding{PolicyHash: r.Hash}, Now: time.Now().UTC()})
		out.Decisions[action] = string(decision.Outcome)
		allowed := !slices.Contains(r.Policy.Deny, action)
		if action == policy.Deploy || action == policy.Recover {
			allowed = allowed && (r.Policy.Allow.Environments == nil || len(r.Policy.Allow.Environments) > 0) && (r.Policy.Allow.Workflows == nil || len(r.Policy.Allow.Workflows) > 0)
		}
		if allowed {
			out.Actions = append(out.Actions, action)
		}
	}
	if in.Mode != "observe" && !slices.Contains(out.Actions, policy.Repair) {
		out.Blockers = append(out.Blockers, "Organisation or team controls prohibit fixes")
	}
	if slices.Contains([]string{"merge", "deliver", "recover"}, in.Mode) && !slices.Contains(out.Actions, policy.Merge) {
		out.Blockers = append(out.Blockers, "Organisation or team controls prohibit merging")
	}
	if slices.Contains([]string{"deliver", "recover"}, in.Mode) && !slices.Contains(out.Actions, policy.Deploy) {
		out.Blockers = append(out.Blockers, "Organisation or team controls prohibit delivery")
	}
	if in.Mode == "recover" && !slices.Contains(out.Actions, policy.Recover) {
		out.Blockers = append(out.Blockers, "Organisation or team controls prohibit recovery")
	}
	out.Ready = len(out.Blockers) == 0
	body, _ := json.Marshal([]any{out, current.Hash, connectionVersion, routeVersion, poolVersion, orgBudgetVersion, repoBudgetVersion})
	sum := sha256.Sum256(body)
	out.Proof = hex.EncodeToString(sum[:])
	return out, nil
}

func (s *Service) PreviewMaintenance(ctx context.Context, session auth.Session, org, repo string, in MaintenanceCommand) (MaintenancePlan, error) {
	var out MaintenancePlan
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = s.maintenanceTx(ctx, tx, a, repo, in)
		return err
	})
	return out, err
}

func (s *Service) ConfigureMaintenance(ctx context.Context, session auth.Session, org, repo string, in MaintenanceCommand, request string) (MaintenancePlan, error) {
	var out MaintenancePlan
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner || !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM organisations WHERE id=$1 FOR UPDATE`, org); err != nil {
			return err
		}
		var err error
		out, err = s.maintenanceTx(ctx, tx, a, repo, in)
		if err != nil {
			return err
		}
		if in.Proof == "" || in.Proof != out.Proof {
			return auth.ErrConflict
		}
		if !out.initialize && slices.Contains(out.Blockers, "organisation policy missing") {
			return auth.ErrForbidden
		}
		if err = s.policies.ApplyMaintenanceTx(ctx, tx, org, repo, a.UserID, request, out.policy, out.initialize); err != nil {
			return err
		}
		if in.Mode != "observe" {
			caps, _ := json.Marshal(map[string]int64{"micro_usd": in.MicroUSD})
			if out.CreatesOrgLimit {
				if _, err = tx.Exec(ctx, `INSERT INTO budget_limits(org_id,scope_kind,scope_id,period,period_start,period_end,caps,version) VALUES($1,'organisation',$1,'monthly',date_trunc('month',now()),date_trunc('month',now())+interval '1 month',$2,1) ON CONFLICT DO NOTHING`, org, caps); err != nil {
					return err
				}
			}
			if _, err = tx.Exec(ctx, `INSERT INTO budget_limits(org_id,scope_kind,scope_id,period,period_start,period_end,caps,version) VALUES($1,'repository',$2,'monthly',date_trunc('month',now()),date_trunc('month',now())+interval '1 month',$3,1) ON CONFLICT(org_id,scope_kind,scope_id) DO UPDATE SET caps=budget_limits.caps||EXCLUDED.caps,version=budget_limits.version+1`, org, repo, caps); err != nil {
				return err
			}
			if out.PoolID != "" {
				if _, err = tx.Exec(ctx, `INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, org, out.PoolID, repo); err != nil {
					return err
				}
			}
			if _, err = tx.Exec(ctx, `INSERT INTO autopilot_settings(org_id) VALUES($1) ON CONFLICT DO NOTHING`, org); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO autopilot_requests(org_id,repository_id,requested_by,continuous) VALUES($1,$2,$3,$4) ON CONFLICT(org_id,repository_id) DO UPDATE SET requested_by=EXCLUDED.requested_by,continuous=EXCLUDED.continuous,requested_at=clock_timestamp()`, org, repo, a.UserID, in.Continuous)
		} else {
			_, err = tx.Exec(ctx, `DELETE FROM autopilot_requests WHERE org_id=$1 AND repository_id=$2`, org, repo)
		}
		return err
	})
	if err == nil && s.Scan != nil {
		if scanErr := s.Scan(ctx, session, org, repo, request); scanErr != nil {
			out.Blockers = append(out.Blockers, "Scan could not start; use Check now")
			out.Ready = false
		}
	}
	return out, err
}
