package mergecontrol

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
)

func (s *Service) inspectionActor(ctx context.Context, tx pgx.Tx, session *auth.Session, org, repo, operation string) (domain.Actor, error) {
	if session != nil {
		return s.auth.ActorTx(ctx, tx, *session, org)
	}
	a := domain.Actor{OrgID: org, RepositoryIDs: []string{repo}}
	err := tx.QueryRow(ctx, `SELECT m.user_id::text,m.role FROM merge_operations o JOIN memberships m ON m.org_id=o.org_id AND m.user_id=o.requested_by WHERE o.org_id=$1 AND o.id=$2 AND o.repository_id=$3 AND o.state='queued' AND NOT o.cancel_requested AND m.role IN ('owner','admin','maintainer') AND (m.all_repositories OR EXISTS(SELECT 1 FROM member_repositories r WHERE r.org_id=o.org_id AND r.user_id=m.user_id AND r.repository_id=o.repository_id) OR EXISTS(SELECT 1 FROM team_memberships tm JOIN team_repositories tr ON tr.org_id=tm.org_id AND tr.team_id=tm.team_id WHERE tm.org_id=o.org_id AND tm.user_id=m.user_id AND tr.repository_id=o.repository_id))`, org, operation, repo).Scan(&a.UserID, &a.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		err = auth.ErrForbidden
	}
	return a, err
}

func (s *Service) withInspectionActor(ctx context.Context, session *auth.Session, org, repo, operation string, fn func(pgx.Tx, domain.Actor) error) error {
	if session != nil {
		return s.auth.WithMutation(ctx, *session, org, fn)
	}
	if !auth.ValidID(operation) {
		return auth.ErrInvalid
	}
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lock(ctx, tx, org); err != nil {
			return err
		}
		a, err := s.inspectionActor(ctx, tx, nil, org, repo, operation)
		if err != nil {
			return err
		}
		return fn(tx, a)
	})
}
