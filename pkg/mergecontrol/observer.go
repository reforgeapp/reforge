package mergecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/reforgeapp/reforge/pkg/heartbeat"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

func (s *Service) Observe(ctx context.Context, org, id string) (Operation, error) {
	var out Operation
	var gate Gate
	if !auth.ValidID(org) || !auth.ValidID(id) || s.providers == nil {
		return out, auth.ErrInvalid
	}
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lock(ctx, tx, org); err != nil {
			return err
		}
		var err error
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		gate, err = gateTx(ctx, tx, org, out.GateID)
		if err != nil || !observing(out.State) {
			return err
		}
		out, err = s.revokeStaleQueueTx(ctx, tx, org, out, gate)
		return err
	})
	if err != nil || !observing(out.State) {
		return out, err
	}
	if out.State == "requested" && time.Since(out.UpdatedAt) < 2*time.Minute {
		return out, nil
	}
	if out.CancelRequested && out.NativeQueueID != "" {
		out, err = s.cancelNative(ctx, nil, org, out, "")
		if err != nil || out.State == "cancelled" {
			return out, err
		}
	}
	result, err := s.providers.Read(ctx, org, gate.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeMergeResult, Change: &privateconnector.ChangeArgs{Repository: gate.Snapshot.Change.Repository, ChangeID: gate.Snapshot.Change.ID}}, func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		ref, connection, err := repository(ctx, tx, org, out.RepositoryID)
		if err != nil {
			return err
		}
		if c.ID != connection || ref != gate.Snapshot.Change.Repository {
			return auth.ErrConflict
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	if result.Merge == nil {
		return out, privateconnector.ErrInvalid
	}
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lock(ctx, tx, org); err != nil {
			return err
		}
		current, err := operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if current.Version != out.Version {
			return auth.ErrConflict
		}
		state, reason := observedState(*result.Merge, gate)
		queueID := current.NativeQueueID
		if state == "queued" {
			if queueID == "" {
				queueID = result.Merge.NativeID
			} else if queueID != result.Merge.NativeID {
				state, reason = "reconciling", "Native queue admission changed; fresh authorization is required"
			}
		}
		if current.State == "requested" && state == "reconciling" {
			state, reason = "blocked", "Controller stopped before dispatch; create a fresh merge preview"
		}
		if current.CancelRequested && state == "merged" {
			reason = "Cancellation lost the race; canonical native merge observed"
		}
		if state == current.State && reason == current.Reason && queueID == current.NativeQueueID {
			return nil
		}
		raw, _ := json.Marshal(result.Merge)
		if _, err = tx.Exec(ctx, `UPDATE merge_operations SET state=$3,reason=$4,native_result=$5,native_queue_id=$6,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, state, reason, raw, queueID); err != nil {
			return err
		}
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		return emit(ctx, tx, org, out.RepositoryID, "", "merge."+state, id, out.Version, "", map[string]any{"state": state, "cancel_requested": out.CancelRequested})
	})
	if err == nil && out.State == "queued" && !out.CancelRequested && gate.Phase == "queue_admission" {
		return s.executeQueueGate(ctx, org, out, gate)
	}
	return out, err
}

func (s *Service) revokeStaleQueueTx(ctx context.Context, tx pgx.Tx, org string, operation Operation, gate Gate) (Operation, error) {
	if operation.CancelRequested || operation.NativeQueueID == "" {
		return operation, nil
	}
	cfg, err := configTx(ctx, tx, org, operation.RepositoryID)
	if err != nil {
		return operation, err
	}
	resolved, err := s.policies.ResolveTx(ctx, tx, org, operation.RepositoryID)
	if err != nil {
		return operation, err
	}
	var active bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM merge_operations o JOIN memberships m ON m.org_id=o.org_id AND m.user_id=o.requested_by JOIN connections c ON c.org_id=o.org_id AND c.id=$3 WHERE o.org_id=$1 AND o.id=$2 AND c.state='healthy' AND c.version=$4 AND m.role IN ('owner','admin','maintainer') AND (m.all_repositories OR EXISTS(SELECT 1 FROM member_repositories r WHERE r.org_id=o.org_id AND r.user_id=m.user_id AND r.repository_id=o.repository_id) OR EXISTS(SELECT 1 FROM team_memberships tm JOIN team_repositories tr ON tr.org_id=tm.org_id AND tr.team_id=tm.team_id WHERE tm.org_id=o.org_id AND tm.user_id=m.user_id AND tr.repository_id=o.repository_id)))`, org, operation.ID, gate.ConnectionID, gate.ConnectionVersion).Scan(&active)
	if err != nil {
		return operation, err
	}
	if active && s.changeAuthority != nil {
		actor, actorErr := s.inspectionActor(ctx, tx, nil, org, operation.RepositoryID, operation.ID)
		active = actorErr == nil
		if active {
			_, authorityErr := s.changeAuthority(ctx, tx, org, operation.RepositoryID, gate.Snapshot, actor)
			active = authorityErr == nil
		}
	}
	if active && cfg.Enabled && cfg.Version == gate.ConfigurationVersion && cfg.Qualification.ExpiresAt.After(time.Now()) && !resolved.Paused && len(resolved.Problems) == 0 && resolved.Hash == gate.Binding.PolicyHash {
		return operation, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE merge_operations SET cancel_requested=true,state='reconciling',reason='Merge authority changed; native cancellation requested',version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, operation.ID); err != nil {
		return operation, err
	}
	operation, err = operationTx(ctx, tx, org, operation.ID)
	if err != nil {
		return operation, err
	}
	err = emit(ctx, tx, org, operation.RepositoryID, "", "merge.authority_revoked", operation.ID, operation.Version, "", map[string]any{"cancel_requested": true})
	return operation, err
}

func observing(state string) bool {
	return state == "requested" || state == "dispatching" || state == "queued" || state == "reconciling"
}

func (s *Service) Run(ctx context.Context) error {
	cursor := "00000000-0000-0000-0000-000000000000"
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		heartbeat.Beat("merges", time.Second, nil)
		var org string
		err := s.db.Pool.QueryRow(ctx, `SELECT org_id::text FROM inventory_tenants WHERE org_id>$1::uuid ORDER BY org_id LIMIT 1`, cursor).Scan(&org)
		if errors.Is(err, pgx.ErrNoRows) {
			cursor = "00000000-0000-0000-0000-000000000000"
			continue
		}
		if err != nil {
			continue
		}
		cursor = org
		ids := []string{}
		err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `UPDATE merge_operations SET observe_after=now()+interval '30 seconds' WHERE org_id=$1 AND id IN (SELECT id FROM merge_operations WHERE org_id=$1 AND state IN ('requested','dispatching','queued','reconciling') AND observe_after<=now() AND (state NOT IN ('requested','dispatching') OR updated_at<now()-interval '2 minutes') ORDER BY observe_after,id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id::text`, org)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return rows.Err()
		})
		if err != nil {
			continue
		}
		for _, id := range ids {
			bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
			_, _ = s.Observe(bounded, org, id)
			cancel()
		}
		s.observeNextBotRepair(ctx, org)
	}
}
