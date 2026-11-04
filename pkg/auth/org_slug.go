package auth

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/reforgeapp/reforge/pkg/domain"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

var reservedSlugs = map[string]bool{
	"admin": true, "api": true, "app": true, "assets": true, "auth": true, "billing": true, "blog": true, "cdn": true,
	"dev": true, "docs": true, "help": true, "login": true, "mail": true, "reforge": true, "smtp": true, "sso": true,
	"staff": true, "staging": true, "static": true, "status": true, "support": true, "test": true, "tunnel": true, "www": true,
}

func NormaliseSlug(raw string) (string, bool) {
	slug := strings.ToLower(strings.TrimSpace(raw))
	return slug, slugPattern.MatchString(slug) && !reservedSlugs[slug]
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Service) SetOrganisationSlug(ctx context.Context, session Session, orgID, raw, requestID string) (string, error) {
	slug, ok := NormaliseSlug(raw)
	if raw != "" && !ok {
		return "", ErrInvalid
	}
	err := s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		if _, err := tx.Exec(ctx, `UPDATE organisations SET slug=nullif($2,'') WHERE id=$1`, orgID, slug); err != nil {
			if uniqueViolation(err) {
				return ErrConflict
			}
			return err
		}
		return audit(ctx, tx, orgID, session.User.ID, "organisation.slug_changed", orgID, requestID, 1)
	})
	return slug, err
}

func (s *Service) OrgForSlug(ctx context.Context, raw string) (string, error) {
	slug, ok := NormaliseSlug(raw)
	if !ok {
		return "", ErrInvalid
	}
	var id string
	err := s.identity(ctx, "", map[string]string{"reforge.login_slug": slug}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE slug=$1`, slug).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnauthenticated
	}
	return id, err
}
