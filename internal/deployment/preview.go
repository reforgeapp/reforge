package deployment

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
)

func (s *Service) Preview(ctx context.Context, session auth.Session, org, environment string, in PreviewRequest, request string) (Gate, error) {
	out := Gate{ID: domain.NewID(), Environment: environment, Request: in, Blockers: []string{}, ExpiresAt: time.Now().Add(2 * time.Minute)}
	n, err := strconv.ParseInt(in.ChangeID, 10, 64)
	if !forge.ValidEnvironment(environment) || err != nil || n <= 0 || strconv.FormatInt(n, 10) != in.ChangeID || !source.ValidSHA(in.SourceSHA, "sha1") || !forge.ValidArtifactDigest(in.ArtifactDigest) || (in.RecoveryOf != "" && (!auth.ValidID(in.RecoveryOf) || !auth.ValidID(in.RestoreDeploymentID))) || (in.RecoveryOf == "" && in.RestoreDeploymentID != "") {
		return out, auth.ErrInvalid
	}
	var cfg Configuration
	var ref forge.RepoRef
	var connection connections.Connection
	var resolved policy.Resolved
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		cfg, err = configTx(ctx, tx, org, environment)
		if err != nil {
			return err
		}
		if !manage(a, cfg.RepositoryID) {
			return auth.ErrForbidden
		}
		out.RepositoryID = cfg.RepositoryID
		out.ConfigurationVersion = cfg.Version
		ref, out.ConnectionID, err = repository(ctx, tx, org, cfg.RepositoryID)
		if err != nil {
			return err
		}
		connection, err = s.connections.MetadataTx(ctx, tx, org, out.ConnectionID)
		if err != nil {
			return err
		}
		out.ConnectionVersion = connection.Version
		resolved, err = s.policies.ResolveTx(ctx, tx, org, cfg.RepositoryID)
		if err != nil {
			return err
		}
		return s.checkRecovery(ctx, tx, org, cfg, in)
	})
	if err != nil {
		return out, err
	}
	if resolved.Policy.MaxEvidenceAgeSeconds != nil {
		expires := time.Now().Add(time.Duration(*resolved.Policy.MaxEvidenceAgeSeconds) * time.Second)
		if expires.Before(out.ExpiresAt) {
			out.ExpiresAt = expires
		}
	}
	workflow := cfg.Workflow
	action := policy.Deploy
	if in.RecoveryOf != "" {
		workflow = *cfg.RecoveryWorkflow
		action = policy.Recover
	}
	out.Pipeline = forge.PipelineRequest{Repository: ref, WorkflowID: workflow.ID, WorkflowPath: workflow.Path, WorkflowSHA: workflow.SHA, ConfigSHA256: workflow.ConfigSHA256, Ref: workflow.Ref, SourceSHA: in.SourceSHA, ArtifactDigest: in.ArtifactDigest, Environment: cfg.NativeEnvironment, CorrelationID: domain.NewID(), Inputs: workflow.Inputs}
	if !qualified(cfg, connection, time.Now()) {
		out.Blockers = append(out.Blockers, "Enable a currently qualified native workflow; observe-only connections cannot deploy")
	}
	if !ValidProvenance(cfg, org, in, time.Now()) {
		out.Blockers = append(out.Blockers, "Provide fresh signed build provenance for this source SHA and artifact digest")
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		a, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !manage(a, cfg.RepositoryID) {
			return auth.ErrForbidden
		}
		current, err := configTx(ctx, tx, org, environment)
		if err != nil {
			return err
		}
		if current.Version != cfg.Version || c.ID != out.ConnectionID || c.Version != out.ConnectionVersion {
			return auth.ErrConflict
		}
		_, id, err := repository(ctx, tx, org, cfg.RepositoryID)
		if err != nil {
			return err
		}
		if id != c.ID {
			return auth.ErrConflict
		}
		return nil
	}
	if len(out.Blockers) == 0 {
		result, readErr := s.providers.Read(ctx, org, out.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: ref, ChangeID: in.ChangeID}}, check)
		if readErr != nil {
			return out, readErr
		}
		if result.Change == nil || result.Change.Repository != ref || result.Change.ID != in.ChangeID || result.Change.State != "merged" || result.Change.MergeSHA != in.SourceSHA {
			out.Blockers = append(out.Blockers, "Canonical merged change does not prove this source SHA")
		}
		result, readErr = s.providers.Read(ctx, org, out.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeDeliveryGates, Delivery: &privateconnector.DeliveryArgs{Repository: ref, Environment: cfg.NativeEnvironment}}, check)
		if readErr != nil {
			return out, readErr
		}
		if result.DeploymentGates == nil {
			return out, auth.ErrConflict
		}
		out.Native = *result.DeploymentGates
		out.Pipeline.RulesHash = out.Native.RulesHash
		inspection := out.Pipeline
		inspection.ObserveOnly = true
		inspected, inspectErr := s.providers.Read(ctx, org, out.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgePipelineInspect, Pipeline: &inspection}, check)
		if inspectErr != nil {
			return out, inspectErr
		}
		if inspected.DeploymentGates == nil || inspected.DeploymentGates.RulesHash != out.Native.RulesHash {
			return out, auth.ErrConflict
		}
		if out.Native.NativeEnforced != domain.Supported || out.Native.Environment != cfg.NativeEnvironment || out.Native.RulesHash == "" {
			out.Blockers = append(out.Blockers, "Native deployment enforcement is unverified")
		}
	}
	out.Binding = policy.Binding{SourceSHA: in.SourceSHA, Artifact: in.ArtifactDigest, PolicyHash: resolved.Hash, ProviderRules: out.Native.RulesHash, CapabilityVersion: strconv.FormatInt(connection.Version, 10)}
	out.Decision = permitted(resolved, cfg, workflow, action, out.Binding, time.Now())
	if len(out.Blockers) > 0 {
		out.Decision.Outcome = "deny"
	}
	out.Blockers = append(out.Blockers, out.Decision.Blockers...)
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, cfg.RepositoryID) {
			return auth.ErrForbidden
		}
		current, err := configTx(ctx, tx, org, environment)
		if err != nil {
			return err
		}
		if current.Version != cfg.Version {
			return auth.ErrConflict
		}
		currentPolicy, err := s.policies.ResolveTx(ctx, tx, org, cfg.RepositoryID)
		if err != nil {
			return err
		}
		if currentPolicy.Hash != resolved.Hash {
			return auth.ErrConflict
		}
		c, err := s.connections.MetadataTx(ctx, tx, org, out.ConnectionID)
		if err != nil {
			return err
		}
		if c.Version != out.ConnectionVersion {
			return auth.ErrConflict
		}
		raw, _ := json.Marshal(out)
		if _, err = tx.Exec(ctx, `INSERT INTO deployment_gates(org_id,id,repository_id,environment,document) VALUES($1,$2,$3,$4,$5)`, org, out.ID, out.RepositoryID, environment, raw); err != nil {
			return err
		}
		return emit(ctx, tx, org, out.RepositoryID, a.UserID, "deployment.preview", out.ID, 1, request, map[string]any{"outcome": out.Decision.Outcome, "environment": environment})
	})
	return out, err
}

func (s *Service) checkRecovery(ctx context.Context, tx pgx.Tx, org string, cfg Configuration, in PreviewRequest) error {
	if in.RecoveryOf == "" {
		return nil
	}
	if cfg.RecoveryWorkflow == nil {
		return &domain.ProviderError{Kind: "unsupported", Message: "Configure and qualify a separate recovery workflow first"}
	}
	failed, err := operationTx(ctx, tx, org, in.RecoveryOf)
	if err != nil {
		return err
	}
	known, err := operationTx(ctx, tx, org, in.RestoreDeploymentID)
	if err != nil {
		return err
	}
	if failed.Environment != cfg.Environment || known.Environment != cfg.Environment || failed.RepositoryID != cfg.RepositoryID || known.RepositoryID != cfg.RepositoryID || (failed.State != "failed" && failed.State != "recovery_failed") || (known.State != "healthy" && known.State != "recovered") {
		return auth.ErrConflict
	}
	gate, err := gateTx(ctx, tx, org, known.GateID)
	if err != nil {
		return err
	}
	if gate.Request.SourceSHA != in.SourceSHA || gate.Request.ArtifactDigest != in.ArtifactDigest || gate.Request.ChangeID != in.ChangeID {
		return auth.ErrConflict
	}
	return nil
}
