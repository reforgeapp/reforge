package delivery

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

func (s *Service) currentHead(ctx context.Context, session auth.Session, org, repo, expected string) error {
	if s.Providers == nil {
		return &domain.ProviderError{Kind: "unsupported", Message: "Source verification unavailable"}
	}
	var ref forge.RepoRef
	var connection, branch string
	var version int64
	read := func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		return tx.QueryRow(ctx, `SELECT r.native_id,r.name,r.connection_id::text,r.default_branch,c.version FROM repositories r JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE r.org_id=$1 AND r.id=$2 AND r.accessible AND NOT r.archived AND NOT r.paused AND c.state='healthy'`, org, repo).Scan(&ref.NativeID, &ref.FullName, &connection, &branch, &version)
	}
	if err := s.Releases.Auth.WithActor(ctx, session, org, read); err != nil {
		return err
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		if c.ID != connection || c.Version != version || c.State != "healthy" {
			return auth.ErrConflict
		}
		a, err := s.Releases.Auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		var currentRef forge.RepoRef
		var currentConnection, currentBranch string
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		if err = tx.QueryRow(ctx, `SELECT native_id,name,connection_id::text,default_branch FROM repositories WHERE org_id=$1 AND id=$2 AND accessible AND NOT archived AND NOT paused`, org, repo).Scan(&currentRef.NativeID, &currentRef.FullName, &currentConnection, &currentBranch); err != nil {
			return err
		}
		if currentRef != ref || currentConnection != connection || currentBranch != branch {
			return auth.ErrConflict
		}
		return nil
	}
	result, err := s.Providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: ref, Ref: branch}}, check)
	if err != nil {
		return err
	}
	if result.SHA != expected {
		return &domain.ProviderError{Kind: "release_superseded", Message: "Newer code is available; waiting for its CI release"}
	}
	return nil
}
