package discovery

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/privateconnector"
)

func (s *Service) RecheckAccess(ctx context.Context, org string) error {
	if s.reader == nil {
		return nil
	}
	type pending struct {
		ID, Connection string
		Ref            forge.RepoRef
	}
	var items []pending
	if err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT f.id::text,coalesce(f.evidence->>'connection_id',''),r.native_id,r.name FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE f.org_id=$1 AND f.state='open' AND f.source_id='provider-access' ORDER BY f.last_seen LIMIT 10`, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.ID, &p.Connection, &p.Ref.NativeID, &p.Ref.FullName); err != nil {
				return err
			}
			items = append(items, p)
		}
		return rows.Err()
	}); err != nil {
		return err
	}
	for _, p := range items {
		authorize := func(_ context.Context, _ pgx.Tx, c connections.Connection) error {
			if c.ID != p.Connection || c.OrgID != org || c.State != "healthy" {
				return ErrStale
			}
			return nil
		}
		granted := true
		for _, kind := range []privateconnector.Kind{privateconnector.ForgeIssues, privateconnector.ForgeAdvisories} {
			_, err := s.reader.Read(ctx, org, p.Connection, privateconnector.Operation{ID: domain.NewID(), Kind: kind, Repository: &privateconnector.RepositoryArgs{Repository: p.Ref}}, authorize)
			var providerError *domain.ProviderError
			if errors.Is(err, privateconnector.ErrUnsupported) || errors.As(err, &providerError) && (providerError.Kind == "forbidden" || providerError.Kind == "unsupported" || providerError.Kind == "scope" || providerError.Kind == "not_found") {
				granted = false
				break
			}
			if err != nil {
				return err
			}
		}
		if !granted {
			continue
		}
		if err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE maintenance_findings SET state='resolved',reason='Access granted',version=version+1 WHERE org_id=$1 AND id=$2 AND state='open'`, org, p.ID)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
