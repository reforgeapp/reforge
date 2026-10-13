package autopilot

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

type RetryCommand struct {
	FindingVersion int64  `json:"finding_version"`
	Reason         string `json:"reason"`
}

func (s *Service) RetryFinding(ctx context.Context, session auth.Session, org, finding string, in RetryCommand, request string) error {
	if !auth.ValidID(finding) || in.FindingVersion < 1 || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 2000 {
		return auth.ErrInvalid
	}
	var repo string
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner && a.Role != domain.Admin && a.Role != domain.Maintainer {
			return auth.ErrForbidden
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM organisations WHERE id=$1 FOR UPDATE`, org); err != nil {
			return err
		}
		var version int64
		if err := tx.QueryRow(ctx, `SELECT repository_id::text,version FROM maintenance_findings WHERE org_id=$1 AND id=$2 AND state='open'`, org, finding).Scan(&repo, &version); err != nil {
			return auth.ErrConflict
		}
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		if version != in.FindingVersion {
			return auth.ErrConflict
		}
		tag, err := tx.Exec(ctx, `UPDATE autopilot_attempts a SET max_runs=runs+1,outcome='retry',blocker_kind='',retry_after=clock_timestamp(),updated_at=clock_timestamp() WHERE org_id=$1 AND finding_id=$2 AND finding_version=$3 AND blocker_kind='exhausted' AND outcome='blocked' AND runs>=max_runs AND runs<100 AND NOT EXISTS(SELECT 1 FROM workflow_tasks t WHERE t.org_id=a.org_id AND t.id=a.task_id AND t.state IN ('queued','reproducing','planning','repairing','validating','publishing','reconciling','cancelling'))`, org, finding, version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return auth.ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO autopilot_requests(org_id,repository_id,requested_by) VALUES($1,$2,$3) ON CONFLICT(org_id,repository_id) DO UPDATE SET requested_by=EXCLUDED.requested_by,requested_at=clock_timestamp()`, org, repo, a.UserID); err != nil {
			return err
		}
		data, _ := json.Marshal(in)
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,repository_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,'finding.retry_requested',$5,$6,$7)`, domain.NewID(), org, a.UserID, repo, finding, request, data)
		return err
	})
	if err == nil && s.Scan != nil {
		_ = s.Scan(ctx, session, org, repo, request)
	}
	return err
}
