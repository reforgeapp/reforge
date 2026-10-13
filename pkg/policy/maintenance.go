package policy

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func (s *Service) MaintenanceTx(ctx context.Context, tx pgx.Tx, org, repo, mode, model, route string, cap int64, initialize bool) (Resolved, Policy, error) {
	r, err := s.ResolveTx(ctx, tx, org, repo)
	if err != nil {
		return r, Policy{}, err
	}
	p := Policy{Schema: "maintenance/v1"}
	orgExists := false
	layers := make([]Layer, 0, len(r.Layers)+1)
	for _, layer := range r.Layers {
		if layer.Scope.Kind == "repository" {
			p = layer.Policy
			continue
		}
		orgExists = orgExists || layer.Scope.Kind == "organisation"
		layers = append(layers, layer)
	}
	if !orgExists && initialize {
		layers = append(layers, Layer{Scope: Scope{Kind: "organisation", ID: org}, Policy: Policy{Schema: "maintenance/v1"}})
	}
	p.Deny = slices.DeleteFunc(p.Deny, func(action Action) bool { return action != Read })
	switch mode {
	case "observe":
		p.Deny = append(p.Deny, Repair, Publish, Merge, Deploy, Recover)
	case "propose":
		p.Deny = append(p.Deny, Merge, Deploy, Recover)
	case "merge":
		p.Deny = append(p.Deny, Deploy, Recover)
	case "deliver":
		p.Deny = append(p.Deny, Recover)
	case "recover":
	default:
		return r, p, auth.ErrInvalid
	}
	if mode != "observe" {
		if model != "" && route != "" {
			p.Defaults.Model, p.Defaults.Route = model, route
			p.Allow.Models, p.Allow.Routes = []string{model}, []string{route}
		}
		p.Limits.Budget = &cap
	}
	p.Paused = false
	p = canonical(p)
	layers = append(layers, Layer{Scope: Scope{Kind: "repository", ID: repo}, Policy: p})
	return Resolve(layers, repo, r.PrimaryTeamID, r.ScopePaused), p, nil
}

func (s *Service) ApplyMaintenanceTx(ctx context.Context, tx pgx.Tx, org, repo, actor, request string, p Policy, initialize bool) error {
	if initialize {
		if err := bindMaintenanceTx(ctx, tx, org, actor, request, Scope{Kind: "organisation", ID: org}, Policy{Schema: "maintenance/v1"}); err != nil {
			return err
		}
	}
	return bindMaintenanceTx(ctx, tx, org, actor, request, Scope{Kind: "repository", ID: repo}, p)
}

func bindMaintenanceTx(ctx context.Context, tx pgx.Tx, org, actor, request string, scope Scope, p Policy) error {
	id := domain.NewID()
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO policy_versions(org_id,id,scope_kind,scope_id,document,policy_hash,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,'Set up maintenance')`, org, id, scope.Kind, scope.ID, b, hash(p), actor); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO policy_bindings(org_id,scope_kind,scope_id,policy_version_id,version,simulation_hash) VALUES($1,$2,$3,$4,1,$5) ON CONFLICT(org_id,scope_kind,scope_id) DO UPDATE SET policy_version_id=EXCLUDED.policy_version_id,version=policy_bindings.version+1,simulation_hash=EXCLUDED.simulation_hash`, org, scope.Kind, scope.ID, id, hash(p)); err != nil {
		return err
	}
	return emit(ctx, tx, org, actor, "policy.changed", id, request, map[string]any{"scope": scope, "hash": hash(p), "reason": "Set up maintenance"})
}
