package delivery

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func (s *Service) gitopsSession(ctx context.Context, parent auth.Session, org, source, environment string) (auth.Session, error) {
	if parent.AutomationID() == "" {
		return parent, nil
	}
	var target string
	var version int64
	err := s.Releases.Auth.WithActor(ctx, parent, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, source) {
			return auth.ErrForbidden
		}
		return tx.QueryRow(ctx, `SELECT delivery_repository_id::text,version FROM gitops_configurations WHERE org_id=$1 AND source_repository_id=$2 AND environment=$3 AND (document->>'enabled')::boolean`, org, source, environment).Scan(&target, &version)
	})
	if err != nil {
		return auth.Session{}, err
	}
	repositories := []string{source}
	if target != source {
		repositories = append(repositories, target)
	}
	return auth.NewAutomationSession(org, parent.User.ID, domain.StableID("approved-delivery-path", parent.AutomationID(), source, target, environment, strconv.FormatInt(version, 10)), repositories, func(ctx context.Context, tx pgx.Tx) error {
		actor, err := s.Releases.Auth.ActorTx(ctx, tx, parent, org)
		if err != nil {
			return err
		}
		if !auth.CanReadRepository(actor, source) {
			return auth.ErrForbidden
		}
		var matches bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM gitops_configurations WHERE org_id=$1 AND source_repository_id=$2 AND delivery_repository_id=$3 AND environment=$4 AND version=$5 AND (document->>'enabled')::boolean)`, org, source, target, environment, version).Scan(&matches)
		if err != nil {
			return err
		}
		if !matches {
			return auth.ErrForbidden
		}
		return nil
	})
}

func (s *Service) restoreSession(ctx context.Context, parent auth.Session, org, kind, id string) (auth.Session, error) {
	if kind != "gitops" || parent.AutomationID() == "" {
		return parent, nil
	}
	var source, environment string
	err := s.Releases.Auth.WithActor(ctx, parent, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := tx.QueryRow(ctx, `SELECT source_repository_id::text,environment FROM gitops_promotions WHERE org_id=$1 AND id=$2`, org, id).Scan(&source, &environment); err != nil {
			return err
		}
		if !auth.CanReadRepository(a, source) {
			return auth.ErrForbidden
		}
		return nil
	})
	if err != nil {
		return auth.Session{}, err
	}
	return s.gitopsSession(ctx, parent, org, source, environment)
}
