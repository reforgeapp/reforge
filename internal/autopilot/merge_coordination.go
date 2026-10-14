package autopilot

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ownerRepairPinsTarget(ctx context.Context, org, repository, target string) (bool, error) {
	var active bool
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repair_runs rr JOIN workflow_tasks t ON t.org_id=rr.org_id AND t.id=rr.task_id WHERE rr.org_id=$1 AND rr.repository_id=$2 AND rr.state<>'published' AND t.state IN ('reproducing','planning','repairing','validating','publishing','reconciling','cancelling') AND rr.context#>>'{request,owner}'='true' AND rr.context#>>'{plan,owner}'='true' AND coalesce(rr.context->>'follow_up_branch','')='' AND rr.context#>>'{finding,evidence,target_branch}'=$3)`, org, repository, target).Scan(&active)
	})
	return active, err
}

const pinnedByOwner = "Waiting for active owner repairs pinned to this target"

func (s *Service) mergeWaiting(ctx context.Context, org, repository string) (string, error) {
	var change string
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT coalesce(min(rr.native_change->>'id'),'') FROM repair_runs rr LEFT JOIN autopilot_attempts a ON a.org_id=rr.org_id AND a.task_id=rr.task_id LEFT JOIN autopilot_bot_merges d ON d.org_id=rr.org_id AND d.repository_id=rr.repository_id AND d.change_id=rr.native_change->>'id' WHERE rr.org_id=$1 AND rr.repository_id=$2 AND rr.state='published' AND coalesce(rr.native_change->>'state','open')='open' AND (a.merge_reason=$3 OR d.reason LIKE '%'||$3)`, org, repository, pinnedByOwner).Scan(&change)
	})
	return change, err
}
