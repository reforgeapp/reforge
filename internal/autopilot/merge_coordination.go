package autopilot

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ownerRepairPinsTarget(ctx context.Context, org, repository, target string) (bool, error) {
	var active bool
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repair_runs rr JOIN workflow_tasks t ON t.org_id=rr.org_id AND t.id=rr.task_id WHERE rr.org_id=$1 AND rr.repository_id=$2 AND rr.state<>'published' AND t.state IN ('queued','reproducing','planning','repairing','validating','publishing','reconciling','cancelling') AND rr.context#>>'{request,owner}'='true' AND rr.context#>>'{plan,owner}'='true' AND coalesce(rr.context->>'follow_up_branch','')='' AND rr.context#>>'{finding,evidence,target_branch}'=$3)`, org, repository, target).Scan(&active)
	})
	return active, err
}
