package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/reforgeapp/reforge/internal/heartbeat"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/privateconnector"
)

func (s *Service) ObserveAs(ctx context.Context, session auth.Session, org, id string) (Operation, error) {
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		o, err := operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !manage(a, o.RepositoryID) {
			return auth.ErrForbidden
		}
		return nil
	})
	if err != nil {
		return Operation{}, err
	}
	return s.Observe(ctx, org, id)
}

func (s *Service) Observe(ctx context.Context, org, id string) (Operation, error) {
	var out Operation
	var gate Gate
	if !auth.ValidID(org) || !auth.ValidID(id) {
		return out, auth.ErrInvalid
	}
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var err error
		out, err = operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		gate, err = gateTx(ctx, tx, org, out.GateID)
		return err
	})
	if err != nil {
		return out, err
	}
	if out.FinishedAt != nil || out.State == "healthy" || out.State == "recovered" || out.State == "failed" || out.State == "recovery_failed" || out.State == "cancelled" || out.State == "blocked" {
		return out, nil
	}
	if (out.State == "dispatching" || out.State == "requested") && time.Since(out.UpdatedAt) < time.Minute {
		return out, nil
	}
	pipeline := gate.Pipeline
	pipeline.ObserveOnly = true
	pipeline.RequestedAt = out.CreatedAt
	if out.Native != nil {
		pipeline.RunID = out.Native.ID
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		ref, id, err := repository(ctx, tx, org, out.RepositoryID)
		if err != nil {
			return err
		}
		if id != c.ID || c.ID != gate.ConnectionID || ref != pipeline.Repository {
			return auth.ErrConflict
		}
		return nil
	}
	op := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgePipelineObserve, Pipeline: &pipeline}
	if gate.Pipeline.ObserveOnly {
		op = privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeDeliveryStatus, Delivery: &privateconnector.DeliveryArgs{Repository: pipeline.Repository, RunID: gate.Pipeline.RunID}}
	}
	result, err := s.providers.Read(ctx, org, gate.ConnectionID, op, check)
	if err != nil {
		return out, err
	}
	if result.Deployment == nil {
		return out, auth.ErrConflict
	}
	native := *result.Deployment
	if native.ID != "" && !observationMatches(gate.Pipeline, native) {
		return out, auth.ErrConflict
	}
	cancelNative := false
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lock(ctx, tx, org); err != nil {
			return err
		}
		current, err := operationTx(ctx, tx, org, out.ID)
		if err != nil {
			return err
		}
		if current.Version != out.Version {
			return auth.ErrConflict
		}
		cfg, err := configTx(ctx, tx, org, out.Environment)
		if err != nil {
			return err
		}
		state := nativeState(native)
		reason := "Canonical native workflow observed; health is separate"
		if native.ID == "" {
			state, reason = "reconciling", "Native run not uniquely located; dispatch will not be repeated"
		}
		if state == "completed_unverified" {
			state, reason, err = healthState(ctx, tx, org, out, gate, cfg, native)
			if err != nil {
				return err
			}
		}
		deadline := time.Since(out.CreatedAt) > time.Duration(cfg.DeadlineSeconds)*time.Second
		if deadline && (state == "completed_unverified" || state == "verifying") {
			state, reason = "completed_unverified", "Pipeline completed; rollout health was not verified before the deadline"
		}
		finished := state == "healthy" || state == "failed" || state == "cancelled" || (deadline && state == "completed_unverified")
		if out.RecoveryOf != "" {
			if state == "healthy" {
				state = "recovered"
			}
			if state == "failed" {
				state = "recovery_failed"
			}
			if state == "running" {
				state = "recovering"
			}
		}
		raw, _ := json.Marshal(native)
		if _, err = tx.Exec(ctx, `UPDATE deployments SET state=$3,reason=$4,native_result=$5,version=version+1,observe_after=now()+interval '20 seconds',finished_at=CASE WHEN $6 THEN now() ELSE NULL END,cancel_state=CASE WHEN cancel_requested AND $3='cancelled' THEN 'confirmed' ELSE cancel_state END,updated_at=now() WHERE org_id=$1 AND id=$2`, org, out.ID, state, reason, raw, finished); err != nil {
			return err
		}
		out, err = operationTx(ctx, tx, org, out.ID)
		if err != nil {
			return err
		}
		if err == nil && !gate.Pipeline.ObserveOnly && native.ID != "" && (state == "running" || state == "awaiting_gates" || state == "recovering") {
			if out.CancelRequested {
				cancelNative = out.CancelState == "pending"
			} else {
				cancelNative, err = s.cancellationRequired(ctx, tx, org, out, gate)
			}
			if err != nil {
				return err
			}
		}

		return emit(ctx, tx, org, out.RepositoryID, "", "deployment.observed", out.ID, out.Version, "", map[string]any{"state": state, "run_id": native.ID, "run_attempt": native.RunAttempt})
	})
	if err == nil && cancelNative {
		return s.cancel(ctx, nil, org, out.ID, out.Version, "")
	}
	return out, err
}

func healthState(ctx context.Context, tx pgx.Tx, org string, operation Operation, gate Gate, cfg Configuration, native forge.DeploymentStatus) (string, string, error) {
	reason := "Pipeline completed; authenticated fresh rollout health is required"
	if cfg.Version != gate.ConfigurationVersion {
		return "completed_unverified", "Deployment configuration changed; health cannot use the previous qualification", nil
	}
	var raw []byte
	var first, last *time.Time
	err := tx.QueryRow(ctx, `SELECT h.document,d.first_healthy_at,d.last_health_at FROM deployments d JOIN deployment_health_reports h ON h.org_id=d.org_id AND h.deployment_id=d.id WHERE d.org_id=$1 AND d.id=$2 ORDER BY h.received_at DESC LIMIT 1`, org, operation.ID).Scan(&raw, &first, &last)
	if errors.Is(err, pgx.ErrNoRows) {
		return "completed_unverified", reason, nil
	}
	if err != nil {
		return "", "", err
	}
	var h HealthReport
	if json.Unmarshal(raw, &h) != nil {
		return "completed_unverified", reason, nil
	}
	if h.RunID != native.ID || h.RunAttempt != native.RunAttempt || h.SourceSHA != gate.Request.SourceSHA || h.ArtifactDigest != gate.Request.ArtifactDigest || last == nil || time.Since(*last) > time.Duration(cfg.MaxEvidenceAgeSeconds)*time.Second {
		return "completed_unverified", reason, nil
	}
	good := h.Healthy
	for _, name := range cfg.HealthChecks {
		if !h.Checks[name] {
			good = false
		}
	}
	if !good {
		return "failed", "Authenticated rollout health failed", nil
	}
	if first == nil || time.Since(*first) < time.Duration(cfg.ObservationSeconds)*time.Second || h.ObservedAt.Sub(*first) < time.Duration(cfg.ObservationSeconds)*time.Second {
		return "verifying", "Fresh health observations have not completed the configured window", nil
	}
	return "healthy", "Native completion, artifact provenance and attributed rollout health verified", nil
}

func (s *Service) ReceiveHealth(ctx context.Context, org, id string, in HealthReport, signature string) error {
	if !auth.ValidID(org) || !auth.ValidID(id) || !auth.ValidID(in.Nonce) || in.OrgID != org || in.DeploymentID != id || len(in.Checks) > 50 || len(signature) > 128 {
		return auth.ErrInvalid
	}
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lock(ctx, tx, org); err != nil {
			return err
		}
		o, err := operationTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		gate, err := gateTx(ctx, tx, org, o.GateID)
		if err != nil {
			return err
		}
		cfg, err := configTx(ctx, tx, org, o.Environment)
		if err != nil {
			return err
		}
		if o.FinishedAt != nil || o.State == "healthy" || o.State == "recovered" || o.State == "failed" || o.State == "recovery_failed" || o.State == "cancelled" || o.State == "blocked" {
			return auth.ErrConflict
		}
		if !VerifySignature(cfg.HealthPublicKey, in, signature) {
			return auth.ErrForbidden
		}
		if cfg.Version != gate.ConfigurationVersion || in.ConfigurationVersion != cfg.Version || in.Environment != cfg.Environment || in.SourceSHA != gate.Request.SourceSHA || in.ArtifactDigest != gate.Request.ArtifactDigest || in.Revision != in.SourceSHA || o.Native == nil || o.Native.ID == "" || in.RunID != o.Native.ID || in.RunAttempt != o.Native.RunAttempt || in.RunAttempt != 1 {
			return auth.ErrConflict
		}
		now := time.Now()
		if in.ObservedAt.Before(o.CreatedAt) || in.ObservedAt.After(now.Add(5*time.Second)) || in.ObservedAt.Before(now.Add(-time.Duration(cfg.MaxEvidenceAgeSeconds)*time.Second)) {
			return auth.ErrConflict
		}
		var old []byte
		err = tx.QueryRow(ctx, `SELECT document FROM deployment_health_reports WHERE org_id=$1 AND deployment_id=$2 AND nonce=$3`, org, id, in.Nonce).Scan(&old)
		raw, _ := json.Marshal(in)
		if err == nil {
			var previous HealthReport
			if json.Unmarshal(old, &previous) != nil || forge.DeliveryRulesHash(previous) != forge.DeliveryRulesHash(in) {
				return auth.ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var first, last *time.Time
		if err = tx.QueryRow(ctx, `SELECT first_healthy_at,last_health_at FROM deployments WHERE org_id=$1 AND id=$2`, org, id).Scan(&first, &last); err != nil {
			return err
		}
		if last != nil && !in.ObservedAt.After(*last) {
			return auth.ErrConflict
		}
		healthy := in.Healthy
		for _, name := range cfg.HealthChecks {
			if !in.Checks[name] {
				healthy = false
			}
		}
		if !healthy {
			first = nil
		} else if first == nil || last == nil || in.ObservedAt.Sub(*last) > time.Duration(cfg.MaxEvidenceAgeSeconds)*time.Second {
			v := in.ObservedAt
			first = &v
		}
		if _, err = tx.Exec(ctx, `INSERT INTO deployment_health_reports(org_id,deployment_id,nonce,document) VALUES($1,$2,$3,$4)`, org, id, in.Nonce, raw); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE deployments SET first_healthy_at=$3,last_health_at=$4,observe_after=now(),version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, first, in.ObservedAt); err != nil {
			return err
		}
		return emit(ctx, tx, org, o.RepositoryID, "", "deployment.health", id, o.Version+1, "", map[string]any{"healthy": healthy, "run_id": in.RunID, "observed_at": in.ObservedAt})
	})
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
		heartbeat.Beat("deployments", time.Second, nil)
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
		var id string
		err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `UPDATE deployments SET observe_after=now()+interval '60 seconds' WHERE org_id=$1 AND id=(SELECT id FROM deployments WHERE org_id=$1 AND finished_at IS NULL AND state IN ('requested','dispatching','awaiting_gates','running','verifying','reconciling','completed_unverified','recovering') AND observe_after<=now() AND (state NOT IN ('requested','dispatching') OR updated_at<now()-interval '2 minutes') ORDER BY observe_after,id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id::text`, org).Scan(&id)
		})
		if err != nil {
			continue
		}
		bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
		if _, err = s.Observe(bounded, org, id); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "deployment observation failed", "org_id", org, "deployment_id", id, "error", err)
			heartbeat.Beat("deployments", time.Second, err)
		}
		cancel()
	}
}
