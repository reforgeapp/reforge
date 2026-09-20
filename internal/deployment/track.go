package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
	"strconv"
	"time"
)

type TrackRequest struct {
	PreviewRequest
	RunID          string `json:"run_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

func observationMatches(in forge.PipelineRequest, out forge.DeploymentStatus) bool {
	if !in.ObserveOnly {
		return forge.PipelineMatches(in, out)
	}
	if in.RunID == "" || out.ID != in.RunID {
		return false
	}
	out.CorrelationID = in.CorrelationID
	return forge.PipelineMatches(in, out)
}
func (s *Service) Track(ctx context.Context, session auth.Session, org, environment string, in TrackRequest, request string) (Operation, error) {
	var out Operation
	var cfg Configuration
	var c connections.Connection
	var ref forge.RepoRef
	var cid string
	var resolved policy.Resolved
	n, err := strconv.ParseInt(in.RunID, 10, 64)
	change, changeErr := strconv.ParseInt(in.ChangeID, 10, 64)
	if !forge.ValidEnvironment(environment) || err != nil || n < 1 || strconv.FormatInt(n, 10) != in.RunID || changeErr != nil || change < 1 || strconv.FormatInt(change, 10) != in.ChangeID || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 128 || in.RecoveryOf != "" || in.RestoreDeploymentID != "" || !source.ValidSHA(in.SourceSHA, "sha1") || !forge.ValidArtifactDigest(in.ArtifactDigest) {
		return out, auth.ErrInvalid
	}
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		existing, e := scanOperation(tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM deployments WHERE org_id=$1 AND idempotency_key=$2`, org, in.IdempotencyKey))
		if e == nil {
			if !manage(a, existing.RepositoryID) {
				return auth.ErrForbidden
			}
			g, e := gateTx(ctx, tx, org, existing.GateID)
			if e != nil {
				return e
			}
			left, _ := json.Marshal(g.Request)
			right, _ := json.Marshal(in.PreviewRequest)
			if !g.Pipeline.ObserveOnly || g.Pipeline.RunID != in.RunID || existing.Environment != environment || !bytes.Equal(left, right) {
				return auth.ErrConflict
			}
			out = existing
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		cfg, e = configTx(ctx, tx, org, environment)
		if e != nil {
			return e
		}
		if !manage(a, cfg.RepositoryID) {
			return auth.ErrForbidden
		}
		if !cfg.Enabled || cfg.Mode != "observe" || !ValidProvenance(cfg, org, in.PreviewRequest, time.Now()) {
			return auth.ErrConflict
		}
		ref, cid, e = repository(ctx, tx, org, cfg.RepositoryID)
		if e != nil {
			return e
		}
		c, e = s.connections.MetadataTx(ctx, tx, org, cid)
		if e != nil {
			return e
		}
		resolved, e = s.policies.ResolveTx(ctx, tx, org, cfg.RepositoryID)
		return e
	})
	if err != nil || out.ID != "" {
		return out, err
	}
	check := func(ctx context.Context, tx pgx.Tx, current connections.Connection) error {
		a, e := s.auth.ActorTx(ctx, tx, session, org)
		if e != nil {
			return e
		}
		if !manage(a, cfg.RepositoryID) {
			return auth.ErrForbidden
		}
		now, e := configTx(ctx, tx, org, environment)
		if e != nil {
			return e
		}
		if now.Version != cfg.Version || !now.Enabled || now.Mode != "observe" || current.ID != cid || current.Version != c.Version || !ValidProvenance(now, org, in.PreviewRequest, time.Now()) {
			return auth.ErrConflict
		}
		native, id, e := repository(ctx, tx, org, cfg.RepositoryID)
		if e != nil {
			return e
		}
		if native != ref || id != cid {
			return auth.ErrConflict
		}
		return nil
	}
	changeResult, err := s.providers.Read(ctx, org, cid, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: ref, ChangeID: in.ChangeID}}, check)
	if err != nil {
		return out, err
	}
	if changeResult.Change == nil || changeResult.Change.Repository != ref || changeResult.Change.ID != in.ChangeID || changeResult.Change.State != "merged" || changeResult.Change.MergeSHA != in.SourceSHA {
		return out, auth.ErrConflict
	}
	result, err := s.providers.Read(ctx, org, cid, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeDeliveryStatus, Delivery: &privateconnector.DeliveryArgs{Repository: ref, RunID: in.RunID}}, check)
	if err != nil {
		return out, err
	}
	w := cfg.Workflow
	g := Gate{ID: domain.NewID(), Environment: environment, RepositoryID: cfg.RepositoryID, ConnectionID: cid, ConnectionVersion: c.Version, ConfigurationVersion: cfg.Version, Request: in.PreviewRequest, Blockers: []string{}, ExpiresAt: time.Now().Add(2 * time.Minute), Native: forge.DeploymentGates{State: "observed_only", NativeEnforced: domain.Unknown, Environment: cfg.NativeEnvironment, Blockers: []string{"Read-only tracking supplies no approval authority"}}}
	g.Pipeline = forge.PipelineRequest{Repository: ref, WorkflowID: w.ID, WorkflowPath: w.Path, WorkflowSHA: w.SHA, ConfigSHA256: w.ConfigSHA256, Ref: w.Ref, SourceSHA: in.SourceSHA, ArtifactDigest: in.ArtifactDigest, Environment: cfg.NativeEnvironment, CorrelationID: domain.NewID(), Inputs: w.Inputs, ObserveOnly: true, RunID: in.RunID}
	if result.Deployment == nil || !observationMatches(g.Pipeline, *result.Deployment) {
		return out, auth.ErrConflict
	}
	g.Binding = policy.Binding{SourceSHA: in.SourceSHA, Artifact: in.ArtifactDigest, PolicyHash: resolved.Hash, CapabilityVersion: strconv.FormatInt(c.Version, 10)}
	g.Decision = policy.Evaluate(resolved, policy.Input{Action: policy.Read, Current: g.Binding, Now: time.Now()})
	if g.Decision.Outcome != "allow" {
		return out, auth.ErrForbidden
	}
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		currentConnection, e := s.connections.MetadataTx(ctx, tx, org, cid)
		if e != nil {
			return e
		}
		if e := check(ctx, tx, currentConnection); e != nil {
			return e
		}
		current, e := s.policies.ResolveTx(ctx, tx, org, cfg.RepositoryID)
		if e != nil {
			return e
		}
		if current.Hash != resolved.Hash {
			return auth.ErrConflict
		}
		raw, _ := json.Marshal(g)
		if _, e = tx.Exec(ctx, `INSERT INTO deployment_gates(org_id,id,repository_id,environment,document) VALUES($1,$2,$3,$4,$5)`, org, g.ID, cfg.RepositoryID, environment, raw); e != nil {
			return e
		}
		native, _ := json.Marshal(result.Deployment)
		state := nativeState(*result.Deployment)
		if _, e = tx.Exec(ctx, `INSERT INTO deployments(org_id,id,repository_id,environment,gate_id,requested_by,idempotency_key,state,reason,native_result,native_environment) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'Observe only; native approval authority is unchanged',$9,$10)`, org, g.Pipeline.CorrelationID, cfg.RepositoryID, environment, g.ID, a.UserID, in.IdempotencyKey, state, native, cfg.NativeEnvironment); e != nil {
			return e
		}
		out, e = operationTx(ctx, tx, org, g.Pipeline.CorrelationID)
		if e != nil {
			return e
		}
		return emit(ctx, tx, org, cfg.RepositoryID, a.UserID, "deployment.tracked", out.ID, out.Version, request, map[string]any{"run_id": in.RunID})
	})
	return out, err
}
