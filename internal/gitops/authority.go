package gitops

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/mergecontrol"
	"reforge/internal/policy"
	"slices"
	"strings"
	"time"
)

func (s *Service) currentTx(ctx context.Context, tx pgx.Tx, org string, g Gate, connection string) error {
	c, err := configTx(ctx, tx, org, g.Configuration.Environment)
	if err != nil {
		return err
	}
	if !c.Enabled || c.Version != g.Configuration.Version || !proofValid(c, org, g.Request, time.Now()) {
		return auth.ErrConflict
	}
	if connection != "" && connection != g.SourceConnectionID && connection != g.DeliveryConnectionID {
		return auth.ErrForbidden
	}
	usage, err := usageTx(ctx, tx, org, g)
	if err != nil {
		return err
	}
	for _, item := range []struct {
		repo, id string
		ref      forge.RepoRef
		version  int64
		hash     string
	}{{c.SourceRepositoryID, g.SourceConnectionID, g.Source, g.SourceConnectionVersion, g.SourcePolicyHash}, {c.DeliveryRepositoryID, g.DeliveryConnectionID, g.Delivery, g.DeliveryConnectionVersion, g.DeliveryPolicyHash}} {
		ref, id, err := repository(ctx, tx, org, item.repo)
		if err != nil {
			return err
		}
		if ref != item.ref || id != item.id {
			return auth.ErrConflict
		}
		current, err := s.connections.MetadataTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if current.State != "healthy" || current.Version != item.version {
			return auth.ErrConflict
		}
		p, err := s.policies.ResolveTx(ctx, tx, org, item.repo)
		if err != nil {
			return err
		}
		if p.Hash != item.hash || p.Paused || len(p.Problems) > 0 || slices.Contains(p.Policy.Deny, policy.Publish) || (len(g.PatchedManifest) > 0 && proposalDecision(p, g, usage).Outcome != "allow") {
			return auth.ErrForbidden
		}
	}
	return s.recoveryTx(ctx, tx, org, g)
}
func (s *Service) recoveryTx(ctx context.Context, tx pgx.Tx, org string, g Gate) error {
	if g.Request.RecoveryOf == "" {
		return nil
	}
	if !g.Configuration.RecoveryAllowed {
		return &domain.ProviderError{Kind: "unsupported", Message: "Enable the preauthorised GitOps revert procedure first"}
	}
	failed, err := promotionTx(ctx, tx, org, g.Request.RecoveryOf)
	if err != nil {
		return err
	}
	known, err := promotionTx(ctx, tx, org, g.Request.RestorePromotionID)
	if err != nil {
		return err
	}
	if failed.Environment != g.Configuration.Environment || known.Environment != failed.Environment || failed.SourceRepositoryID != g.Configuration.SourceRepositoryID || failed.DeliveryRepositoryID != g.Configuration.DeliveryRepositoryID || known.SourceRepositoryID != failed.SourceRepositoryID || known.DeliveryRepositoryID != failed.DeliveryRepositoryID || (failed.State != "failed" && failed.State != "recovery_failed") || (known.State != "healthy" && known.State != "recovered") {
		return auth.ErrConflict
	}
	failedGate, err := gateTx(ctx, tx, org, failed.GateID)
	if err != nil {
		return err
	}
	if !sameTarget(failedGate.Configuration, g.Configuration) {
		return auth.ErrConflict
	}
	old, err := gateTx(ctx, tx, org, known.GateID)
	if err != nil {
		return err
	}
	if !sameTarget(old.Configuration, g.Configuration) || old.Request.SourceSHA != g.Request.SourceSHA || old.Request.ArtifactDigest != g.Request.ArtifactDigest || old.Request.ChangeID != g.Request.ChangeID {
		return auth.ErrConflict
	}
	return nil
}
func originalAuthority(ctx context.Context, tx pgx.Tx, org, user string, repos ...string) error {
	for _, repo := range repos {
		var allowed bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships m WHERE m.org_id=$1 AND m.user_id=$2 AND m.role IN ('owner','admin','maintainer') AND (m.all_repositories OR EXISTS(SELECT 1 FROM member_repositories r WHERE r.org_id=m.org_id AND r.user_id=m.user_id AND r.repository_id=$3) OR EXISTS(SELECT 1 FROM team_memberships tm JOIN team_repositories tr ON tr.org_id=tm.org_id AND tr.team_id=tm.team_id WHERE tm.org_id=m.org_id AND tm.user_id=m.user_id AND tr.repository_id=$3)))`, org, user, repo).Scan(&allowed)
		if err != nil {
			return err
		}
		if !allowed {
			return auth.ErrForbidden
		}
	}
	return nil
}
func (s *Service) CheckMergeTx(ctx context.Context, tx pgx.Tx, org, repo string, snapshot forge.MergeEvidence, actor domain.Actor) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM gitops_promotions WHERE org_id=$1 AND delivery_repository_id=$2 AND (native_change->>'id'=$3 OR branch=$4)`, org, repo, snapshot.Change.ID, snapshot.Change.HeadBranch).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if strings.HasPrefix(snapshot.Change.HeadBranch, "reforge/promote/") {
			return "", auth.ErrForbidden
		}
		return "", nil
	}
	if err != nil {
		return "", err
	}
	op, err := promotionTx(ctx, tx, org, id)
	if err != nil {
		return "", err
	}
	if !auth.CanReadRepository(actor, op.SourceRepositoryID) || !auth.CanReadRepository(actor, op.DeliveryRepositoryID) {
		return "", auth.ErrForbidden
	}
	g, err := gateTx(ctx, tx, org, op.GateID)
	if err != nil {
		return "", err
	}
	if op.CancelRequested || op.State != "awaiting_merge" || op.Change == nil || op.Change.ID != snapshot.Change.ID || op.CandidateSHA != snapshot.Change.HeadSHA || g.TargetSHA != snapshot.Change.TargetSHA || snapshot.Change.Repository != g.Delivery || snapshot.Change.HeadRepository != g.Delivery || snapshot.Change.TargetRepository != g.Delivery || snapshot.Change.TargetBranch != g.Configuration.TargetBranch {
		return "", auth.ErrConflict
	}
	if err = s.checkGateAuthority(ctx, tx, org, g.ID); err != nil {
		return "", &mergecontrol.AuthorityBlocker{Reason: "Campaign is paused or its authority changed; review the campaign before merging"}
	}
	if err = originalAuthority(ctx, tx, org, op.RequestedBy, op.SourceRepositoryID, op.DeliveryRepositoryID); err != nil {
		return "", err
	}
	if err = s.currentTx(ctx, tx, org, g, g.DeliveryConnectionID); err != nil {
		return "", err
	}
	reviewers := map[string]bool{}
	for _, a := range snapshot.Approvals {
		if a.ActorID != "" && a.ActorID != snapshot.Change.AuthorID && a.HeadSHA == op.CandidateSHA && !a.Dismissed && strings.EqualFold(a.State, "approved") {
			reviewers[a.ActorID] = true
		}
	}
	if snapshot.Native.State != "eligible" || len(reviewers) < snapshot.Rules.RequiredApprovals || snapshot.Rules.State != domain.Supported || snapshot.Rules.ActorCanBypass {
		return "", &mergecontrol.AuthorityBlocker{Reason: "GitOps delivery requires current native checks and approvals from a non-bypass actor"}
	}
	usage, err := usageTx(ctx, tx, org, g)
	if err != nil {
		return "", err
	}
	action := policy.Deploy
	if g.Request.RecoveryOf != "" {
		action = policy.Recover
	}
	for _, repo := range []string{op.SourceRepositoryID, op.DeliveryRepositoryID} {
		p, e := s.policies.ResolveTx(ctx, tx, org, repo)
		if e != nil {
			return "", e
		}
		binding := policy.Binding{Head: op.CandidateSHA, Target: g.TargetSHA, Tested: op.CandidateSHA, SourceSHA: g.Request.SourceSHA, Artifact: g.Request.ArtifactDigest, PolicyHash: p.Hash, ProviderRules: snapshot.Rules.Hash, CapabilityVersion: snapshot.Capabilities.Provider + "/" + snapshot.Capabilities.ServerVersion}
		evidence := []policy.Evidence{}
		for _, id := range []string{"execution_authority", "native_approvals", "artifact_provenance", "workflow_authority", "recovery_authority", "known_good_artifact"} {
			evidence = append(evidence, policy.Evidence{ID: id, State: "satisfied", Binding: binding, ObservedAt: snapshot.ObservedAt, Approvals: len(reviewers), Reference: "gitops:" + op.ID})
		}
		decision := policy.Evaluate(p, policy.Input{Action: action, Current: binding, Environment: g.Configuration.Environment, Workflow: "gitops:" + g.Configuration.Environment, Paths: []string{g.Configuration.ManifestPath}, Evidence: evidence, Now: time.Now(), Usage: usage})
		if decision.Outcome != "allow" {
			return "", &mergecontrol.AuthorityBlocker{Reason: "GitOps source/delivery policy blocks promotion: " + strings.Join(decision.Blockers, "; ")}
		}
	}
	return "gitops-manifest:" + op.ID, nil
}

func usageTx(ctx context.Context, tx pgx.Tx, org string, g Gate) (policy.Limits, error) {
	zero, one, lines := int64(0), int64(1), int64(2)
	var concurrency, open int64
	err := tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM gitops_promotions WHERE org_id=$1 AND id<>$2 AND finished_at IS NULL AND state NOT IN ('blocked','cancelled'))+1,
 (SELECT count(*) FROM inventory_changes i WHERE i.org_id=$1 AND i.snapshot->>'state' IN ('open','opened') AND NOT EXISTS(SELECT 1 FROM gitops_promotions p WHERE p.org_id=i.org_id AND p.id=$2 AND p.delivery_repository_id=i.repository_id AND p.native_change->>'id'=i.native_id))+
 (SELECT count(*) FROM gitops_promotions p WHERE p.org_id=$1 AND p.id<>$2 AND p.finished_at IS NULL AND p.state NOT IN ('blocked','cancelled') AND NOT EXISTS(SELECT 1 FROM inventory_changes i WHERE i.org_id=p.org_id AND i.repository_id=p.delivery_repository_id AND i.native_id=p.native_change->>'id' AND i.snapshot->>'state' IN ('open','opened')))+1`, org, g.OperationID).Scan(&concurrency, &open)
	return policy.Limits{Budget: &zero, Concurrency: &concurrency, Attempts: &one, ChangedFiles: &one, ChangedLines: &lines, OpenChanges: &open}, err
}

func sameTarget(a, b Configuration) bool {
	return a.Environment == b.Environment && a.SourceRepositoryID == b.SourceRepositoryID && a.DeliveryRepositoryID == b.DeliveryRepositoryID && a.TargetBranch == b.TargetBranch && a.ManifestPath == b.ManifestPath && a.Pointer == b.Pointer && a.ImageRepository == b.ImageRepository
}
