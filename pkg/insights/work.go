package insights

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/attention"
)

func (s *Service) Work(ctx context.Context, session auth.Session, org string, f Filter) (domain.Page[attention.Work], error) {
	var out domain.Page[attention.Work]
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = attention.WorkTx(ctx, tx, a, f.State, f.Limit, f.Cursor)
		return err
	})
	return out, err
}
