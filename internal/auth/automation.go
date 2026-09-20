package auth

import (
	"context"

	"github.com/jackc/pgx/v5"
	"reforge/internal/domain"
)

type automationGrant struct {
	org, user, id string
	repositories  []string
	verify        func(context.Context, pgx.Tx) error
}

func NewAutomationSession(org, user, grant string, repositories []string, verify func(context.Context, pgx.Tx) error) (Session, error) {
	if !ValidID(org) || !ValidID(user) || !ValidID(grant) || len(repositories) < 1 || len(repositories) > 2000 || verify == nil {
		return Session{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range repositories {
		if !ValidID(id) || seen[id] {
			return Session{}, ErrInvalid
		}
		seen[id] = true
	}
	return Session{User: User{ID: user}, automation: &automationGrant{org: org, user: user, id: grant, repositories: append([]string(nil), repositories...), verify: verify}}, nil
}

func (s Session) AutomationID() string {
	if s.automation == nil {
		return ""
	}
	return s.automation.id
}

func (g *automationGrant) actor(ctx context.Context, tx pgx.Tx, org, user string) (domain.Actor, error) {
	a := domain.Actor{OrgID: org, UserID: user, Role: domain.Maintainer, RepositoryIDs: append([]string(nil), g.repositories...)}
	if org != g.org || user != g.user {
		return a, ErrForbidden
	}
	var current string
	if e := tx.QueryRow(ctx, `SELECT current_setting('reforge.org_id',true)`).Scan(&current); e != nil {
		return a, e
	}
	if current != org {
		return a, ErrForbidden
	}
	if e := g.verify(ctx, tx); e != nil {
		return a, e
	}
	var allowed bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships m WHERE m.org_id=$1 AND m.user_id=$2 AND m.role IN ('owner','admin','maintainer') AND NOT EXISTS(SELECT 1 FROM unnest($3::uuid[]) wanted(repository_id) WHERE NOT (m.all_repositories OR EXISTS(SELECT 1 FROM member_repositories r WHERE r.org_id=m.org_id AND r.user_id=m.user_id AND r.repository_id=wanted.repository_id) OR EXISTS(SELECT 1 FROM team_memberships tm JOIN team_repositories tr ON tr.org_id=tm.org_id AND tr.team_id=tm.team_id WHERE tm.org_id=m.org_id AND tm.user_id=m.user_id AND tr.repository_id=wanted.repository_id))))`, org, user, g.repositories).Scan(&allowed)
	if e != nil {
		return a, e
	}
	if !allowed {
		return a, ErrForbidden
	}
	return a, nil
}
