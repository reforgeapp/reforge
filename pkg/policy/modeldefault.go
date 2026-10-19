package policy

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func (s *Service) UseModelDefault(ctx context.Context, session auth.Session, org, connection, model, route string) error {
	return s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if session.AutomationID() != "" || a.Role != domain.Owner || !a.AllRepositories {
			return auth.ErrForbidden
		}
		if !auth.ValidID(connection) || model == "" || route == "" {
			return auth.ErrInvalid
		}
		var available bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connections WHERE org_id=$1 AND id=$2 AND state='healthy' AND settings->>'model'=$3)`, org, connection, model).Scan(&available); err != nil {
			return err
		}
		if !available {
			return auth.ErrConflict
		}
		resolved, err := s.ResolveTx(ctx, tx, org, "")
		if err != nil {
			return err
		}
		if s.deployment.Allow.Models != nil && !contains(s.deployment.Allow.Models, model) || !RouteAllowed(s.deployment.Allow.Routes, connection, route) {
			return auth.ErrForbidden
		}
		p := Policy{Schema: "maintenance/v1", RepositorySetupRequired: true}
		for _, layer := range resolved.Layers {
			if layer.Scope.Kind == "organisation" {
				p = layer.Policy
			}
		}
		p.Defaults.Model, p.Defaults.Route, p.Defaults.ConnectionID = model, route, connection
		if p.Allow.Models != nil && !slices.Contains(p.Allow.Models, model) {
			p.Allow.Models = append(p.Allow.Models, model)
		}
		if p.Allow.Routes != nil && !RouteAllowed(p.Allow.Routes, connection, route) {
			p.Allow.Routes = append(p.Allow.Routes, connection+"/"+route)
		}
		if err = Validate(p); err != nil {
			return err
		}
		return bindMaintenanceTx(ctx, tx, org, a.UserID, domain.NewID(), Scope{Kind: "organisation", ID: org}, canonical(p), "Use agent for Autopilot")
	})
}
