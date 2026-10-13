package autopilot

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/attention"
)

type Need = attention.Need

func (s *Service) Needs(ctx context.Context, session auth.Session, org string) ([]Need, error) {
	page, err := s.NeedsPage(ctx, session, org, 100, "")
	return page.Items, err
}

func (s *Service) NeedsPage(ctx context.Context, session auth.Session, org string, limit int, cursor string) (attention.Page, error) {
	var out attention.Page
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = attention.ListTx(ctx, tx, a, limit, cursor)
		return err
	})
	return out, err
}
