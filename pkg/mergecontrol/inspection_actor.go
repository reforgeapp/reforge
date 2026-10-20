package mergecontrol

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func (s *Service) inspectionActor(ctx context.Context, tx pgx.Tx, session *auth.Session, org, repo, operation string) (domain.Actor, error) {
	if session != nil {
		return s.auth.ActorTx(ctx, tx, *session, org)
	}
	a := domain.Actor{OrgID: org, RepositoryIDs: []string{repo}}
	err := tx.QueryRow(ctx, `SELECT m.user_id::text,m.role FROM memberships m WHERE m.org_id=$1 AND m.user_id IN (SELECT requested_by FROM merge_operations WHERE org_id=$1 AND id=$2 AND repository_id=$3 AND state='queued' AND NOT cancel_requested UNION ALL SELECT requested_by FROM repair_runs WHERE org_id=$1 AND task_id=$2 AND repository_id=$3 AND state='published') AND m.role IN ('owner','admin','maintainer') AND (m.all_repositories OR EXISTS(SELECT 1 FROM member_repositories r WHERE r.org_id=m.org_id AND r.user_id=m.user_id AND r.repository_id=$3) OR EXISTS(SELECT 1 FROM team_memberships tm JOIN team_repositories tr ON tr.org_id=tm.org_id AND tr.team_id=tm.team_id WHERE tm.org_id=m.org_id AND tm.user_id=m.user_id AND tr.repository_id=$3))`, org, operation, repo).Scan(&a.UserID, &a.Role)
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
