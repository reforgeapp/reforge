package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

const operationColumns = `id::text,environment,repository_id::text,gate_id::text,state,reason,version,requested_by::text,native_result,COALESCE(recovery_of::text,''),created_at,updated_at,finished_at,cancel_requested,cancel_state`

func scanOperation(row pgx.Row) (Operation, error) {
	var out Operation
	var raw []byte
	err := row.Scan(&out.ID, &out.Environment, &out.RepositoryID, &out.GateID, &out.State, &out.Reason, &out.Version, &out.RequestedBy, &raw, &out.RecoveryOf, &out.CreatedAt, &out.UpdatedAt, &out.FinishedAt, &out.CancelRequested, &out.CancelState)
	if err == nil && len(raw) > 0 {
		err = json.Unmarshal(raw, &out.Native)
	}
	return out, err
}
func operationTx(ctx context.Context, tx pgx.Tx, org, id string) (Operation, error) {
	return scanOperation(tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM deployments WHERE org_id=$1 AND id=$2`, org, id))
}

type Detail struct {
	Health    *HealthReport `json:"health,omitempty"`
	Operation Operation     `json:"operation"`
	Gate      Gate          `json:"gate"`
}

func (s *Service) Get(ctx context.Context, session auth.Session, org, id string) (Detail, error) {
	var out Detail
	if !auth.ValidID(id) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out.Operation, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !auth.CanReadRepository(a, out.Operation.RepositoryID) {
			return auth.ErrForbidden
		}
		out.Gate, err = gateTx(ctx, tx, org, out.Operation.GateID)
		if err != nil {
			return err
		}
		var raw []byte
		err = tx.QueryRow(ctx, `SELECT document FROM deployment_health_reports WHERE org_id=$1 AND deployment_id=$2 ORDER BY received_at DESC LIMIT 1`, org, id).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &out.Health)
	})
	return out, err
}
func (s *Service) List(ctx context.Context, session auth.Session, org, repo, env, cursor string, limit int) (domain.Page[Operation], error) {
	out := domain.Page[Operation]{Items: []Operation{}}
	if repo != "" && !auth.ValidID(repo) || cursor != "" && !auth.ValidID(cursor) || env != "" && !forge.ValidEnvironment(env) || limit < 1 || limit > 100 {
		return out, auth.ErrInvalid
	}
	if cursor == "" {
		cursor = "ffffffff-ffff-ffff-ffff-ffffffffffff"
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if repo != "" && !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT `+operationColumns+` FROM deployments WHERE org_id=$1 AND ($2='' OR repository_id::text=$2) AND ($3='' OR environment=$3) AND id<$4::uuid AND ($5 OR repository_id=ANY($6::uuid[])) ORDER BY id DESC LIMIT $7`, org, repo, env, cursor, a.AllRepositories, a.RepositoryIDs, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanOperation(rows)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, v)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		out.Complete = len(out.Items) <= limit
		if !out.Complete {
			out.Items = out.Items[:limit]
			out.NextCursor = out.Items[len(out.Items)-1].ID
		}
		return nil
	})
	return out, err
}

func (s *Service) currentAuthority(ctx context.Context, tx pgx.Tx, session auth.Session, org string, gate Gate) (Configuration, error) {
	if e := s.checkGateAuthority(ctx, tx, org, gate.ID); e != nil {
		return Configuration{}, e
	}
	if gate.Pipeline.ObserveOnly {
		return Configuration{}, auth.ErrForbidden
	}
	a, err := s.auth.ActorTx(ctx, tx, session, org)
	if err != nil {
		return Configuration{}, err
	}
	if !manage(a, gate.RepositoryID) {
		return Configuration{}, auth.ErrForbidden
	}
	cfg, err := configTx(ctx, tx, org, gate.Environment)
	if err != nil {
		return cfg, err
	}
	if cfg.Version != gate.ConfigurationVersion || cfg.RepositoryID != gate.RepositoryID {
		return cfg, auth.ErrConflict
	}
	_, id, err := repository(ctx, tx, org, gate.RepositoryID)
	if err != nil {
		return cfg, err
	}
	if id != gate.ConnectionID {
		return cfg, auth.ErrConflict
	}
	c, err := s.connections.MetadataTx(ctx, tx, org, id)
	if err != nil {
		return cfg, err
	}
	if c.Version != gate.ConnectionVersion || !qualified(cfg, c, time.Now()) || !ValidProvenance(cfg, org, gate.Request, time.Now()) {
		return cfg, auth.ErrConflict
	}
	resolved, err := s.policies.ResolveTx(ctx, tx, org, gate.RepositoryID)
	if err != nil {
		return cfg, err
	}
	if resolved.Hash != gate.Binding.PolicyHash {
		return cfg, auth.ErrConflict
	}
	w := cfg.Workflow
	action := policy.Deploy
	if gate.Request.RecoveryOf != "" {
		if cfg.RecoveryWorkflow == nil {
			return cfg, auth.ErrConflict
		}
		w = *cfg.RecoveryWorkflow
		action = policy.Recover
	}
	if permitted(resolved, cfg, w, action, gate.Binding, time.Now()).Outcome != "allow" || !gate.ExpiresAt.After(time.Now()) || gate.Decision.Outcome != "allow" || len(gate.Blockers) > 0 {
		return cfg, auth.ErrConflict
	}
	return cfg, s.checkRecovery(ctx, tx, org, cfg, gate.Request)
}

func (s *Service) Request(ctx context.Context, session auth.Session, org, gateID, key, request string) (Operation, error) {
	var out Operation
	var gate Gate
	fresh := false
	if !auth.ValidID(gateID) || key == "" || len(key) > 128 {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		existing, err := scanOperation(tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM deployments WHERE org_id=$1 AND idempotency_key=$2`, org, key))
		if err == nil {
			if !manage(a, existing.RepositoryID) {
				return auth.ErrForbidden
			}
			if existing.GateID != gateID {
				return auth.ErrConflict
			}
			out = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		gate, err = gateTx(ctx, tx, org, gateID)
		if err != nil {
			return err
		}
		if _, err = s.currentAuthority(ctx, tx, session, org, gate); err != nil {
			return err
		}
		out = Operation{ID: gate.Pipeline.CorrelationID, Environment: gate.Environment, RepositoryID: gate.RepositoryID, GateID: gate.ID, State: "requested", Version: 1, RequestedBy: a.UserID, RecoveryOf: gate.Request.RecoveryOf}
		if _, err = tx.Exec(ctx, `INSERT INTO deployments(org_id,id,repository_id,environment,gate_id,requested_by,idempotency_key,state,recovery_of,native_environment) VALUES($1,$2,$3,$4,$5,$6,$7,'requested',NULLIF($8,'')::uuid,$9)`, org, out.ID, out.RepositoryID, out.Environment, gateID, a.UserID, key, out.RecoveryOf, gate.Pipeline.Environment); err != nil {
			return err
		}
		fresh = true
		return emit(ctx, tx, org, out.RepositoryID, a.UserID, "deployment.requested", out.ID, 1, request, map[string]any{"environment": out.Environment, "gate_id": gate.ID, "recovery_of": out.RecoveryOf})
	})
	if err != nil || !fresh {
		return out, err
	}
	return s.dispatchOperation(ctx, session, org, out, gate, request)
}

func (s *Service) Continue(ctx context.Context, session auth.Session, org, id, request string) (Operation, error) {
	var out Operation
	var gate Gate
	ready := false
	if !auth.ValidID(id) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !manage(a, out.RepositoryID) {
			return auth.ErrForbidden
		}
		if out.State != "requested" {
			return nil
		}
		if err = tx.QueryRow(ctx, `SELECT dispatch_id IS NULL FROM deployments WHERE org_id=$1 AND id=$2`, org, id).Scan(&ready); err != nil {
			return err
		}
		if !ready {
			return nil
		}
		gate, err = gateTx(ctx, tx, org, out.GateID)
		if err != nil {
			return err
		}
		_, err = s.currentAuthority(ctx, tx, session, org, gate)
		return err
	})
	if err != nil || !ready {
		return out, err
	}
	return s.dispatchOperation(ctx, session, org, out, gate, request)
}

func (s *Service) dispatchOperation(ctx context.Context, session auth.Session, org string, out Operation, gate Gate, request string) (Operation, error) {
	dispatch := domain.NewID()
	dispatched := false
	authorize := func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
		if c.ID != gate.ConnectionID || c.Version != gate.ConnectionVersion {
			return "", auth.ErrConflict
		}
		if _, err := s.currentAuthority(ctx, tx, session, org, gate); err != nil {
			return "", err
		}
		tag, err := tx.Exec(ctx, `UPDATE deployments SET state='dispatching',dispatch_id=$3,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2 AND state='requested' AND dispatch_id IS NULL`, org, out.ID, dispatch)
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() != 1 {
			return "", auth.ErrConflict
		}
		dispatched = true
		return dispatch, nil
	}
	validate := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		if c.ID != gate.ConnectionID || c.Version != gate.ConnectionVersion {
			return auth.ErrConflict
		}
		if _, err := s.currentAuthority(ctx, tx, session, org, gate); err != nil {
			return err
		}
		var valid bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE org_id=$1 AND id=$2 AND state='dispatching' AND dispatch_id=$3)`, org, out.ID, dispatch).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return auth.ErrConflict
		}
		return nil
	}
	kind := privateconnector.ForgePipelineTrigger
	if out.RecoveryOf != "" {
		kind = privateconnector.ForgePipelineRecover
	}
	pipeline := gate.Pipeline
	pipeline.RequestedAt = time.Now().UTC()
	result, callErr := s.providers.Write(ctx, org, gate.ConnectionID, privateconnector.Operation{ID: out.ID, Kind: kind, Pipeline: &pipeline}, authorize, validate)
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err := s.db.Tenant(persist, org, "", func(tx pgx.Tx) error {
		if err := lock(persist, tx, org); err != nil {
			return err
		}
		current, err := operationTx(persist, tx, org, out.ID)
		if err != nil {
			return err
		}
		if current.State != "requested" && current.State != "dispatching" {
			out = current
			return nil
		}
		state, reason := "reconciling", "Dispatch outcome requires canonical native observation; no automatic repeat"
		if !dispatched {
			state, reason = "blocked", "Current deployment authority could not be established"
		}
		var native []byte
		if callErr == nil && result.Deployment != nil {
			native, _ = json.Marshal(result.Deployment)
			if forge.PipelineMatches(pipeline, *result.Deployment) {
				state, reason = nativeState(*result.Deployment), "Native execution accepted; health remains unverified"
			}
		}
		tag, err := tx.Exec(persist, `UPDATE deployments SET state=$4,reason=$5,native_result=$6,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2 AND (dispatch_id=$3 OR dispatch_id IS NULL AND state='requested')`, org, out.ID, dispatch, state, reason, native)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return auth.ErrConflict
		}
		out, err = operationTx(persist, tx, org, out.ID)
		if err != nil {
			return err
		}
		return emit(persist, tx, org, out.RepositoryID, "", "deployment.dispatched", out.ID, out.Version, request, map[string]any{"state": state, "reason": reason})
	})
	if err != nil {
		return out, err
	}
	return out, nil
}

func nativeState(v forge.DeploymentStatus) string {
	switch v.State {
	case "queued", "running":
		return "running"
	case "awaiting_approval":
		return "awaiting_gates"
	case "success":
		return "completed_unverified"
	case "failed":
		return "failed"
	case "cancelled":
		return "cancelled"
	}
	return "reconciling"
}
