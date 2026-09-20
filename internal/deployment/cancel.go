package deployment

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/privateconnector"
)

func (s *Service) Cancel(ctx context.Context, session auth.Session, org, id string, expected int64, request string) (Operation, error) {
	return s.cancel(ctx, &session, org, id, expected, request)
}
func (s *Service) cancel(ctx context.Context, session *auth.Session, org, id string, expected int64, request string) (Operation, error) {
	var out Operation
	var gate Gate
	var dispatch string
	send := false
	if !auth.ValidID(id) || expected < 1 {
		return out, auth.ErrInvalid
	}
	err := s.withCancellationActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if session != nil && !manage(a, out.RepositoryID) {
			return auth.ErrForbidden
		}
		if session == nil && !out.CancelRequested {
			g, e := gateTx(ctx, tx, org, out.GateID)
			if e != nil {
				return e
			}
			required, e := s.cancellationRequired(ctx, tx, org, out, g)
			if e != nil {
				return e
			}
			if !required {
				return auth.ErrForbidden
			}
		}
		if out.CancelRequested && out.CancelState != "pending" {
			return nil
		}
		if out.Version != expected || out.FinishedAt != nil || out.Native == nil || out.Native.ID == "" || out.State == "healthy" || out.State == "recovered" || out.State == "failed" || out.State == "recovery_failed" || out.State == "cancelled" || out.State == "blocked" {
			return auth.ErrConflict
		}
		gate, err = gateTx(ctx, tx, org, out.GateID)
		if err != nil {
			return err
		}
		if gate.Pipeline.ObserveOnly || !forge.PipelineMatches(gate.Pipeline, *out.Native) {
			return auth.ErrConflict
		}
		if out.CancelRequested {
			err = tx.QueryRow(ctx, `SELECT cancel_dispatch_id::text FROM deployments WHERE org_id=$1 AND id=$2`, org, id).Scan(&dispatch)
			if err != nil {
				return err
			}
		} else {
			dispatch = domain.NewID()
			if _, err = tx.Exec(ctx, `UPDATE deployments SET cancel_requested=true,cancel_state='pending',cancel_dispatch_id=$3,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, dispatch); err != nil {
				return err
			}
		}
		send = true
		return emit(ctx, tx, org, out.RepositoryID, a.UserID, "deployment.cancel_requested", id, out.Version+1, request, map[string]any{"native_id": out.Native.ID})
	})
	if err != nil || !send {
		return out, err
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		if session != nil {
			a, err := s.auth.ActorTx(ctx, tx, *session, org)
			if err != nil {
				return err
			}
			if !manage(a, out.RepositoryID) {
				return auth.ErrForbidden
			}
		}

		ref, connection, err := repository(ctx, tx, org, out.RepositoryID)
		if err != nil {
			return err
		}
		if c.ID != gate.ConnectionID || connection != c.ID || ref != gate.Pipeline.Repository {
			return auth.ErrConflict
		}
		var valid bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE org_id=$1 AND id=$2 AND cancel_requested AND cancel_dispatch_id=$3 AND cancel_state='dispatching' AND finished_at IS NULL)`, org, id, dispatch).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return auth.ErrConflict
		}
		return nil
	}
	authorize := func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
		tag, err := tx.Exec(ctx, `UPDATE deployments SET cancel_state='dispatching',version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2 AND cancel_dispatch_id=$3 AND cancel_state='pending' AND finished_at IS NULL`, org, id, dispatch)
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() != 1 {
			return "", auth.ErrConflict
		}
		if err = check(ctx, tx, c); err != nil {
			return "", err
		}
		return dispatch, nil
	}
	pipeline := gate.Pipeline
	pipeline.RunID = out.Native.ID
	pipeline.RequestedAt = out.CreatedAt
	result, callErr := s.providers.Write(ctx, org, gate.ConnectionID, privateconnector.Operation{ID: dispatch, Kind: privateconnector.ForgePipelineCancel, Pipeline: &pipeline}, authorize, check)
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err = s.db.Tenant(persist, org, "", func(tx pgx.Tx) error {
		if err := lock(persist, tx, org); err != nil {
			return err
		}
		current, err := operationTx(persist, tx, org, id)
		if err != nil {
			return err
		}
		if current.FinishedAt != nil || current.CancelState == "confirmed" {
			out = current
			return nil
		}
		state, reason, cancelState := current.State, "Cancellation outcome requires native observation; no automatic repeat", "uncertain"
		if current.CancelState == "pending" {
			cancelState = "pending"
			reason = "Cancellation was not dispatched; restore connection authority and retry"
		}
		var raw []byte
		confirmed := callErr == nil && result.Deployment != nil && forge.PipelineMatches(pipeline, *result.Deployment) && result.Deployment.State == "cancelled"
		if confirmed {
			state, reason, cancelState = "cancelled", "Native execution confirms cancellation", "confirmed"
			raw, _ = json.Marshal(result.Deployment)
		}
		if _, err = tx.Exec(persist, `UPDATE deployments SET state=$4,reason=$5,cancel_state=$6,native_result=COALESCE($7,native_result),finished_at=CASE WHEN $8 THEN now() ELSE finished_at END,observe_after=now(),version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2 AND cancel_dispatch_id=$3`, org, id, dispatch, state, reason, cancelState, raw, confirmed); err != nil {
			return err
		}
		out, err = operationTx(persist, tx, org, id)
		if err != nil {
			return err
		}
		return emit(persist, tx, org, out.RepositoryID, "", "deployment.cancel_observed", id, out.Version, request, map[string]any{"state": state, "cancel_state": cancelState})
	})
	return out, err
}

func (s *Service) withCancellationActor(ctx context.Context, session *auth.Session, org string, fn func(pgx.Tx, domain.Actor) error) error {
	if session != nil {
		return s.auth.WithMutation(ctx, *session, org, fn)
	}
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lock(ctx, tx, org); err != nil {
			return err
		}
		return fn(tx, domain.Actor{OrgID: org})
	})
}
func (s *Service) cancellationRequired(ctx context.Context, tx pgx.Tx, org string, o Operation, g Gate) (bool, error) {
	cfg, err := configTx(ctx, tx, org, o.Environment)
	if err != nil {
		return false, err
	}
	c, err := s.connections.MetadataTx(ctx, tx, org, g.ConnectionID)
	if err != nil {
		return false, err
	}
	resolved, err := s.policies.ResolveTx(ctx, tx, org, o.RepositoryID)
	if err != nil {
		return false, err
	}
	if cfg.Version != g.ConfigurationVersion || !qualified(cfg, c, time.Now()) || c.Version != g.ConnectionVersion || resolved.Paused || len(resolved.Problems) > 0 || resolved.Hash != g.Binding.PolicyHash {
		return true, nil
	}
	var active bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships m WHERE m.org_id=$1 AND m.user_id=$2 AND m.role IN ('owner','admin','maintainer') AND (m.all_repositories OR EXISTS(SELECT 1 FROM member_repositories r WHERE r.org_id=m.org_id AND r.user_id=m.user_id AND r.repository_id=$3) OR EXISTS(SELECT 1 FROM team_memberships tm JOIN team_repositories tr ON tr.org_id=tm.org_id AND tr.team_id=tm.team_id WHERE tm.org_id=m.org_id AND tm.user_id=m.user_id AND tr.repository_id=$3)))`, org, o.RequestedBy, o.RepositoryID).Scan(&active)
	return !active, err
}
