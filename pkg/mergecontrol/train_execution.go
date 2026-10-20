package mergecontrol

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

func (s *Service) releaseTrainGate(ctx context.Context, org string, operation Operation, gate Gate) error {
	native := gate.Snapshot.TrainGate
	if native == nil || native.State != "manual" || !native.ChecksReady || native.SHA != gate.Binding.Tested {
		return auth.ErrConflict
	}
	var id, state, job string
	var cfg Configuration
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lock(ctx, tx, org); err != nil {
			return err
		}
		if err := s.validateQueueEffectTx(ctx, tx, nil, org, operation, gate); err != nil {
			return err
		}
		var err error
		cfg, err = configTx(ctx, tx, org, operation.RepositoryID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO merge_execution_checks(org_id,id,operation_id,gate_id,sha,phase,state,native_id) VALUES($1,$2,$3,$4,$5,'queue_execution','prepared',$6) ON CONFLICT(org_id,operation_id,sha) DO NOTHING`, org, domain.NewID(), operation.ID, gate.ID, native.SHA, native.JobID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id::text,state,native_id FROM merge_execution_checks WHERE org_id=$1 AND operation_id=$2 AND sha=$3`, org, operation.ID, native.SHA).Scan(&id, &state, &job)
	})
	if err != nil {
		return err
	}
	if state != "prepared" || job != native.JobID {
		return privateconnector.ErrUncertain
	}
	request := forge.TrainGateRequest{RulesHash: gate.Binding.ProviderRules, Repository: gate.Snapshot.Change.Repository, ChangeID: operation.ChangeID, Gate: *native, OperationID: domain.NewID()}
	op := privateconnector.Operation{ID: request.OperationID, Kind: privateconnector.ForgeReleaseTrainGate, TrainGate: &request}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		if c.ID != gate.ConnectionID || c.Version != gate.ConnectionVersion {
			return auth.ErrConflict
		}
		return s.validateQueueEffectTx(ctx, tx, nil, org, operation, gate)
	}
	result, writeErr := s.providers.ForProtection(cfg.InspectorConnectionID, cfg.CheckPublishers).Write(ctx, org, gate.ConnectionID, op, func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
		if err := check(ctx, tx, c); err != nil {
			return "", err
		}
		changed, err := tx.Exec(ctx, `UPDATE merge_execution_checks SET state='dispatching',dispatch_id=$3,updated_at=now() WHERE org_id=$1 AND id=$2 AND state='prepared'`, org, id, op.ID)
		if err == nil && changed.RowsAffected() != 1 {
			err = auth.ErrConflict
		}
		if err == nil {
			err = emit(ctx, tx, org, operation.RepositoryID, "", "merge.train_job.dispatched", id, 1, "", map[string]any{"operation_id": operation.ID, "job_id": native.JobID, "pipeline_id": native.PipelineID, "sha": native.SHA})
		}
		return operation.ID, err
	}, func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		if err := check(ctx, tx, c); err != nil {
			return err
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT state='dispatching' AND dispatch_id=$3 FROM merge_execution_checks WHERE org_id=$1 AND id=$2`, org, id, op.ID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return auth.ErrConflict
		}
		return nil
	})
	confirmed := writeErr == nil && result.TrainGate != nil && sameTrainRelease(*native, *result.TrainGate)
	next := "uncertain"
	if confirmed {
		next = "confirmed"
	}
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err = s.db.Tenant(persist, org, "", func(tx pgx.Tx) error {
		changed, err := tx.Exec(persist, `UPDATE merge_execution_checks SET state=$3,updated_at=now() WHERE org_id=$1 AND id=$2 AND state='dispatching' AND dispatch_id=$4`, org, id, next, op.ID)
		if err != nil || changed.RowsAffected() == 0 {
			return err
		}
		return emit(persist, tx, org, operation.RepositoryID, "", "merge.train_job."+next, id, 2, "", map[string]any{"operation_id": operation.ID, "job_id": native.JobID, "sha": native.SHA})
	})
	if err != nil {
		return err
	}
	if !confirmed {
		return privateconnector.ErrUncertain
	}
	return nil
}

func sameTrainRelease(expected, actual forge.TrainGate) bool {
	state := actual.State
	actual.State = expected.State
	return expected == actual && (state == "pending" || state == "running" || state == "success")
}
