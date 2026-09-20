package mergecontrol

import (
	"context"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
)

func (s *Service) executeQueueGate(ctx context.Context, org string, operation Operation, original Gate) (Operation, error) {
	fresh, err := s.inspect(ctx, nil, org, operation.RepositoryID, operation.ChangeID, original.Method, "", operation.ID)
	if err != nil {
		return operation, err
	}
	if !sameQueueAuthority(original, fresh) || fresh.Snapshot.Queue.ID != operation.NativeQueueID {
		err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			if err := lock(ctx, tx, org); err != nil {
				return err
			}
			changed, err := tx.Exec(ctx, `UPDATE merge_operations SET state='reconciling',cancel_requested=true,reason='Queue candidate authority changed; cancellation requested',version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2 AND version=$3 AND state='queued'`, org, operation.ID, operation.Version)
			if err != nil {
				return err
			}
			if changed.RowsAffected() != 1 {
				return auth.ErrConflict
			}
			operation, err = operationTx(ctx, tx, org, operation.ID)
			if err != nil {
				return err
			}
			return emit(ctx, tx, org, operation.RepositoryID, "", "merge.queue_invalidated", operation.ID, operation.Version, "", map[string]any{"cancel_requested": true})
		})
		if err != nil {
			return operation, err
		}
		return s.cancelNative(ctx, nil, org, operation, "")
	}
	if fresh.Phase != "queue_execution" || fresh.Decision.Outcome != "allow" {
		return operation, nil
	}
	if fresh.Snapshot.TrainGate != nil {
		return operation, s.releaseTrainGate(ctx, org, operation, fresh)
	}
	_, err = s.publishQueueCheck(ctx, nil, org, operation, fresh)
	return operation, err
}
