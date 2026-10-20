package repair

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

func (s *Service) ObserveChange(ctx context.Context, session auth.Session, org, id string) (forge.Change, error) {
	r, e := s.Get(ctx, session, org, id)
	if e != nil {
		return forge.Change{}, e
	}
	if r.Change == nil || r.State != "published" {
		return forge.Change{}, auth.ErrConflict
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		a, e := s.auth.ActorTx(ctx, tx, session, org)
		if e != nil {
			return e
		}
		if !auth.CanReadRepository(a, r.Task.RepositoryID) {
			return auth.ErrForbidden
		}
		var ref forge.RepoRef
		var connection string
		if e = tx.QueryRow(ctx, `SELECT native_id,name,connection_id::text FROM repositories WHERE org_id=$1 AND id=$2 AND accessible AND NOT archived`, org, r.Task.RepositoryID).Scan(&ref.NativeID, &ref.FullName, &connection); e != nil {
			return e
		}
		if ref != r.Context.Repository || connection != r.Context.ConnectionID || c.ID != connection || c.Version != r.Context.ConnectionVersion || c.State != "healthy" {
			return auth.ErrConflict
		}
		return nil
	}
	result, e := s.reader.Read(ctx, org, r.Context.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: r.Context.Repository, ChangeID: r.Change.ID}}, check)
	if e != nil {
		return forge.Change{}, e
	}
	if result.Change == nil || result.Change.ID != r.Change.ID || result.Change.Repository != r.Context.Repository {
		return forge.Change{}, privateconnector.ErrInvalid
	}
	return *result.Change, nil
}
