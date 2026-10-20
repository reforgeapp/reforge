package discovery

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

func (s *Service) recheckAllAccess(ctx context.Context) error {
	var orgs []string
	if err := pgx.BeginFunc(ctx, s.db.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.maintenance_scheduler','true',true)`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT org_id::text FROM inventory_tenants ORDER BY org_id`)
		if err != nil {
			return err
		}
		orgs, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		return err
	}
	var errs []error
	for _, org := range orgs {
		errs = append(errs, s.recheckAccess(ctx, org))
	}
	return errors.Join(errs...)
}

func (s *Service) recheckAccess(ctx context.Context, org string) error {
	if s.reader == nil {
		return nil
	}
	type pending struct {
		ID, Source, Connection string
		Ref                    forge.RepoRef
	}
	var items []pending
	if err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT f.id::text,f.source_id,coalesce(f.evidence->>'connection_id',''),r.native_id,r.name FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id JOIN connections c ON c.org_id=f.org_id AND c.id::text=f.evidence->>'connection_id' AND c.state='healthy' WHERE f.org_id=$1 AND f.state='open' AND f.source_id IN ('provider-access','dependabot-alerts') ORDER BY f.last_seen LIMIT 10`, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.ID, &p.Source, &p.Connection, &p.Ref.NativeID, &p.Ref.FullName); err != nil {
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
		kinds := []privateconnector.Kind{privateconnector.ForgeIssues, privateconnector.ForgeAdvisories}
		if p.Source == "dependabot-alerts" {
			kinds = kinds[1:]
		}
		for _, kind := range kinds {
			_, err := s.reader.Read(ctx, org, p.Connection, privateconnector.Operation{ID: domain.NewID(), Kind: kind, Repository: &privateconnector.RepositoryArgs{Repository: p.Ref}}, authorize)
			var providerError *domain.ProviderError
			if errors.As(err, &providerError) && providerError.Kind == "configuration" {
				granted = p.Source != "dependabot-alerts"
				if !granted {
					break
				}
				continue
			}
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
