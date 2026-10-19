package gitops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/policy"
	"github.com/reforgeapp/reforge/internal/privateconnector"
	"github.com/reforgeapp/reforge/internal/source"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (s *Service) Preview(ctx context.Context, session auth.Session, org, env string, in PreviewRequest, request string) (Gate, error) {
	g := Gate{ID: domain.NewID(), OperationID: domain.NewID(), Request: in, Blockers: []string{}, ExpiresAt: time.Now().Add(2 * time.Minute)}
	n, err := strconv.ParseInt(in.ChangeID, 10, 64)
	if !forge.ValidEnvironment(env) || err != nil || n < 1 || strconv.FormatInt(n, 10) != in.ChangeID || !source.ValidSHA(in.SourceSHA, "sha1") || !forge.ValidArtifactDigest(in.ArtifactDigest) || (in.RecoveryOf != "" && (!auth.ValidID(in.RecoveryOf) || !auth.ValidID(in.RestorePromotionID))) || (in.RecoveryOf == "" && in.RestorePromotionID != "") {
		return g, auth.ErrInvalid
	}
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var e error
		g.Configuration, e = configTx(ctx, tx, org, env)
		if e != nil {
			return e
		}
		c := g.Configuration
		if !manage(a, c.SourceRepositoryID, c.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		if !c.Enabled {
			g.Blockers = append(g.Blockers, "Enable this GitOps environment")
		}
		if !proofValid(c, org, in, time.Now()) {
			g.Blockers = append(g.Blockers, "Fresh signed source/artifact provenance is required")
		}
		g.Source, g.SourceConnectionID, e = repository(ctx, tx, org, c.SourceRepositoryID)
		if e != nil {
			return e
		}
		g.Delivery, g.DeliveryConnectionID, e = repository(ctx, tx, org, c.DeliveryRepositoryID)
		if e != nil {
			return e
		}
		for _, item := range []struct {
			id      string
			version *int64
		}{{g.SourceConnectionID, &g.SourceConnectionVersion}, {g.DeliveryConnectionID, &g.DeliveryConnectionVersion}} {
			c, e := s.connections.MetadataTx(ctx, tx, org, item.id)
			if e != nil {
				return e
			}
			if c.State != "healthy" {
				g.Blockers = append(g.Blockers, "Both forge connections must be healthy")
			}
			*item.version = c.Version
		}
		left, e := s.policies.ResolveTx(ctx, tx, org, c.SourceRepositoryID)
		if e != nil {
			return e
		}
		right, e := s.policies.ResolveTx(ctx, tx, org, c.DeliveryRepositoryID)
		if e != nil {
			return e
		}
		g.SourcePolicyHash, g.DeliveryPolicyHash = left.Hash, right.Hash
		for _, p := range []policy.Resolved{left, right} {
			if p.Policy.MaxEvidenceAgeSeconds != nil {
				g.ExpiresAt = minTime(g.ExpiresAt, time.Now().Add(time.Duration(*p.Policy.MaxEvidenceAgeSeconds)*time.Second))
			}
		}
		return s.recoveryTx(ctx, tx, org, g)
	})
	if err != nil {
		return g, err
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		a, e := s.auth.ActorTx(ctx, tx, session, org)
		if e != nil {
			return e
		}
		if !manage(a, g.Configuration.SourceRepositoryID, g.Configuration.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		return s.currentTx(ctx, tx, org, g, c.ID)
	}
	if len(g.Blockers) == 0 {
		result, e := s.providers.Read(ctx, org, g.SourceConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: g.Source, ChangeID: in.ChangeID}}, check)
		if e != nil {
			return g, e
		}
		if result.Change == nil || result.Change.Repository != g.Source || result.Change.ID != in.ChangeID || result.Change.State != "merged" || result.Change.MergeSHA != in.SourceSHA {
			return g, auth.ErrConflict
		}
		result, e = s.providers.Read(ctx, org, g.DeliveryConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: g.Delivery, Ref: g.Configuration.TargetBranch}}, check)
		if e != nil {
			return g, e
		}
		if !source.ValidSHA(result.SHA, "sha1") {
			return g, auth.ErrConflict
		}
		g.TargetSHA = result.SHA
		content, e := s.manifestFile(ctx, org, g, g.TargetSHA, check)
		if e != nil {
			return g, e
		}
		g.Before, e = ReadManifestField(content, g.Configuration.Pointer)
		if e != nil {
			return g, manifestUnsupported()
		}
		g.After = g.Configuration.ImageRepository + "@" + in.ArtifactDigest
		if g.Before == g.After {
			return g, &domain.ProviderError{Kind: "conflict", Message: "The manifest already names this immutable artifact"}
		}
		if !strings.HasPrefix(g.Before, g.Configuration.ImageRepository+"@") && !strings.HasPrefix(g.Before, g.Configuration.ImageRepository+":") {
			return g, &domain.ProviderError{Kind: "conflict", Message: "The configured field names a different image repository"}
		}
		g.PatchedManifest, e = PatchManifest(content, g.Configuration.Pointer, g.Before, g.After)
		if e != nil {
			return g, manifestUnsupported()
		}
		sum := sha256.Sum256(content)
		g.ManifestSHA256 = hex.EncodeToString(sum[:])
	}
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, g.Configuration.SourceRepositoryID, g.Configuration.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		current, e := configTx(ctx, tx, org, env)
		if e != nil {
			return e
		}
		if current.Version != g.Configuration.Version {
			return auth.ErrConflict
		}
		left, e := s.policies.ResolveTx(ctx, tx, org, current.SourceRepositoryID)
		if e != nil {
			return e
		}
		right, e := s.policies.ResolveTx(ctx, tx, org, current.DeliveryRepositoryID)
		if e != nil {
			return e
		}
		if left.Hash != g.SourcePolicyHash || right.Hash != g.DeliveryPolicyHash {
			return auth.ErrConflict
		}
		usage, e := usageTx(ctx, tx, org, g)
		if e != nil {
			return e
		}
		g.Decision = proposalDecision(left, g, usage)
		other := proposalDecision(right, g, usage)
		g.Blockers = append(g.Blockers, g.Decision.Blockers...)
		g.Blockers = append(g.Blockers, other.Blockers...)
		if g.Decision.Outcome != "allow" || other.Outcome != "allow" || len(g.Blockers) > 0 {
			g.Decision.Outcome = "deny"
		}
		raw, _ := json.Marshal(g)
		_, e = tx.Exec(ctx, `INSERT INTO gitops_gates(org_id,id,environment,source_repository_id,delivery_repository_id,document) VALUES($1,$2,$3,$4,$5,$6)`, org, g.ID, env, current.SourceRepositoryID, current.DeliveryRepositoryID, raw)
		if e != nil {
			return e
		}
		return emit(ctx, tx, org, current.DeliveryRepositoryID, a.UserID, "gitops.preview", g.ID, 1, request, map[string]any{"outcome": g.Decision.Outcome, "environment": env})
	})
	return g, err
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func proposalDecision(p policy.Resolved, g Gate, usage policy.Limits) policy.Result {
	now := time.Now()
	binding := policy.Binding{Head: g.TargetSHA, Target: g.TargetSHA, Tested: g.TargetSHA, SourceSHA: g.Request.SourceSHA, Artifact: g.Request.ArtifactDigest, PolicyHash: p.Hash}
	facts := []policy.Evidence{}
	for _, id := range []string{"execution_authority", "validation", "branch_ownership", "exact_head_guard"} {
		facts = append(facts, policy.Evidence{ID: id, State: "satisfied", Binding: binding, ObservedAt: now, Reference: "gitops:" + g.OperationID})
	}
	result := policy.Evaluate(p, policy.Input{Action: policy.Publish, Recipe: "gitops-manifest", Current: binding, Environment: g.Configuration.Environment, Workflow: "gitops:" + g.Configuration.Environment, Paths: []string{g.Configuration.ManifestPath}, Evidence: facts, Now: now, Usage: usage})
	action := policy.Deploy
	if g.Request.RecoveryOf != "" {
		action = policy.Recover
	}
	if slices.Contains(p.Policy.Deny, action) || !slices.Contains(p.Policy.Allow.Environments, g.Configuration.Environment) || !slices.Contains(p.Policy.Allow.Workflows, "gitops:"+g.Configuration.Environment) {
		result.Outcome = "deny"
		result.Blockers = append(result.Blockers, "GitOps environment/workflow must be explicitly allowed for the source and delivery repositories")
	}
	return result
}

func manifestUnsupported() error {
	return &domain.ProviderError{Kind: "gitops_manifest_unsupported", Message: "Configure an existing single-line string in one bounded YAML/JSON document; aliases, tags, duplicate keys and ambiguous edits are unsupported"}
}
