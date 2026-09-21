package integration

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/deployment"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/policy"
)

func TestDeploymentContinuePersistsDispatchAndRejectsDuplicate(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET state='healthy',server_version='3.0',verified_at=now(),settings=jsonb_build_object('auth_kind','github_app','app_id','42','billing_route','forge') WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PutConfig(ctx, f.owner, f.org, f.repo, discovery.Config{MergeAuthority: "reforge"}, 0, "deployment-resume"); err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{Environments: []string{"production"}, Workflows: []string{"pipeline"}}}, "deployment resume", "deployment-resume")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Deploy, Environment: "production", Workflow: "pipeline"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, sim.Hash, "deployment resume", "deployment-resume"); err != nil {
		t.Fatal(err)
	}
	pub, priv, _ := ed25519.GenerateKey(nil)
	hpub, hpriv, _ := ed25519.GenerateKey(nil)
	head := strings.Repeat("a", 40)
	artifact := "sha256:" + strings.Repeat("b", 64)
	workflowSHA := strings.Repeat("d", 40)
	configHash := strings.Repeat("e", 64)
	repo := forge.RepoRef{NativeID: "1", FullName: "acme/repository-0000"}
	provider := &deploymentContractProvider{queueContractProvider: &queueContractProvider{db: f.db, connections: f.connections, org: f.org, repo: repo, head: head, target: strings.Repeat("b", 40), tested: head, ciPassing: true, files: map[string]map[string][]byte{}}, native: map[string]forge.DeploymentStatus{}, failed: map[string]bool{}, running: map[string]bool{}}
	svc := deployment.New(f.db, f.identity, f.connections, policies, provider)
	config := deployment.Configuration{Environment: "production", RepositoryID: f.repo, Enabled: true, Mode: "pipeline", Workflow: deployment.Workflow{ID: "pipeline", Path: ".gitlab-ci.yml", Ref: "refs/heads/main", SHA: workflowSHA, ConfigSHA256: configHash}, ProvenancePublicKey: base64.StdEncoding.EncodeToString(pub), HealthPublicKey: base64.StdEncoding.EncodeToString(hpub), HealthChecks: []string{"smoke"}, ObservationSeconds: 0, MaxEvidenceAgeSeconds: 300, DeadlineSeconds: 600, Qualification: deployment.Qualification{Provider: "github", ServerVersion: "3.0", ConnectionVersion: f.connection.Version, EvidenceReference: "deployment-resume", EvidenceSHA256: strings.Repeat("a", 64), VerifiedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour), PinnedInputs: true, NativeEnforcement: true, EnvironmentSerialization: true, NoBypass: true}}
	config, err = svc.PutConfiguration(ctx, f.owner, f.org, config, 0, "deployment-resume")
	if err != nil {
		t.Fatal(err)
	}
	provenance := deployment.Provenance{OrgID: f.org, RepositoryID: f.repo, SourceSHA: head, ArtifactDigest: artifact, BuildID: "resume-build", IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour)}
	raw, _ := json.Marshal(provenance)
	signed := deployment.SignedProvenance{Document: provenance, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))}
	gate, err := svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "deployment-resume")
	if err != nil || gate.Decision.Outcome != "allow" {
		t.Fatalf("preview: %+v %v", gate, err)
	}
	seed := func(g deployment.Gate, key string) string {
		t.Helper()
		if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO deployments(org_id,id,repository_id,environment,gate_id,requested_by,idempotency_key,state,recovery_of,native_environment) VALUES($1,$2,$3,$4,$5,$6,$7,'requested',NULLIF($8,'')::uuid,$9)`, f.org, g.Pipeline.CorrelationID, g.RepositoryID, g.Environment, g.ID, f.owner.User.ID, key, g.Request.RecoveryOf, g.Pipeline.Environment)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return g.Pipeline.CorrelationID
	}
	opID := seed(gate, "resume-key")
	op := deployment.Operation{ID: opID}
	continued, err := svc.Continue(ctx, f.owner, f.org, op.ID, "resume-after-crash")
	if err != nil || continued.State != "running" {
		t.Fatalf("continue: %+v %v", continued, err)
	}
	var persisted bool
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT dispatch_id IS NOT NULL FROM deployments WHERE org_id=$1 AND id=$2`, f.org, op.ID).Scan(&persisted)
	}); err != nil || !persisted {
		t.Fatalf("dispatch identity was not persisted: %v", err)
	}
	provider.mu.Lock()
	if provider.dispatches != 1 {
		provider.mu.Unlock()
		t.Fatalf("resume dispatch count=%d", provider.dispatches)
	}
	provider.dispatches = 0
	provider.mu.Unlock()
	if continued.Native == nil {
		t.Fatal("resume returned no native operation")
	}
	health := deployment.HealthReport{OrgID: f.org, DeploymentID: op.ID, Environment: "production", ConfigurationVersion: config.Version, SourceSHA: head, ArtifactDigest: artifact, RunID: continued.Native.ID, RunAttempt: 1, Revision: head, Healthy: true, Checks: map[string]bool{"smoke": true}, ObservedAt: time.Now(), Nonce: domain.NewID()}
	raw, _ = json.Marshal(health)
	if err = svc.ReceiveHealth(ctx, f.org, op.ID, health, base64.StdEncoding.EncodeToString(ed25519.Sign(hpriv, raw))); err != nil {
		t.Fatalf("receive resumed health: %v", err)
	}
	if observed, observeErr := svc.Observe(ctx, f.org, op.ID); observeErr != nil || observed.State != "healthy" {
		t.Fatalf("observe resumed operation: %+v %v", observed, observeErr)
	}
	gate2, err := svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "2", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "deployment-resume-concurrent")
	if err != nil || gate2.Decision.Outcome != "allow" {
		t.Fatalf("concurrent preview: %+v %v", gate2, err)
	}
	op2ID := seed(gate2, "resume-concurrent-key")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, callErr := svc.Continue(ctx, f.owner, f.org, op2ID, "concurrent-resume")
			errs <- callErr
		}()
	}
	wg.Wait()
	close(errs)
	for callErr := range errs {
		if callErr != nil && !errors.Is(callErr, auth.ErrConflict) {
			t.Fatalf("concurrent continue: %v", callErr)
		}
	}
	provider.mu.Lock()
	if provider.dispatches != 1 {
		provider.mu.Unlock()
		t.Fatalf("concurrent resume dispatch count=%d", provider.dispatches)
	}
	provider.mu.Unlock()
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE deployments SET state='reconciling',version=version+1 WHERE org_id=$1 AND id=$2`, f.org, op2ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	before := provider.dispatches
	provider.mu.Unlock()
	if _, err = svc.Continue(ctx, f.owner, f.org, op2ID, "no-retry"); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	if provider.dispatches != before {
		provider.mu.Unlock()
		t.Fatalf("reconciling operation redispatched: %d -> %d", before, provider.dispatches)
	}
	provider.mu.Unlock()
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE deployments SET state='dispatching',version=version+1 WHERE org_id=$1 AND id=$2`, f.org, op2ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Continue(ctx, f.owner, f.org, op2ID, "dispatching-no-retry"); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	if provider.dispatches != before {
		provider.mu.Unlock()
		t.Fatalf("dispatching operation redispatched: %d -> %d", before, provider.dispatches)
	}
	provider.mu.Unlock()
	detail, err := svc.Get(ctx, f.owner, f.org, op2ID)
	if err != nil || detail.Operation.Native == nil {
		t.Fatalf("concurrent operation detail: %+v %v", detail, err)
	}
	health2 := deployment.HealthReport{OrgID: f.org, DeploymentID: op2ID, Environment: "production", ConfigurationVersion: config.Version, SourceSHA: head, ArtifactDigest: artifact, RunID: detail.Operation.Native.ID, RunAttempt: 1, Revision: head, Healthy: true, Checks: map[string]bool{"smoke": true}, ObservedAt: time.Now(), Nonce: domain.NewID()}
	raw, _ = json.Marshal(health2)
	if err = svc.ReceiveHealth(ctx, f.org, op2ID, health2, base64.StdEncoding.EncodeToString(ed25519.Sign(hpriv, raw))); err != nil {
		t.Fatalf("receive concurrent health: %v", err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE deployments SET updated_at=clock_timestamp()-interval '2 minutes' WHERE org_id=$1 AND id=$2`, f.org, op2ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if observed, observeErr := svc.Observe(ctx, f.org, op2ID); observeErr != nil || observed.State != "healthy" {
		t.Fatalf("observe concurrent operation: %+v %v", observed, observeErr)
	}
	gate3, err := svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "3", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "deployment-resume-viewer")
	if err != nil || gate3.Decision.Outcome != "allow" {
		t.Fatalf("viewer preview: %+v %v", gate3, err)
	}
	op3ID := seed(gate3, "resume-viewer-key")
	var beforeState string
	var beforeVersion int64
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state,version FROM deployments WHERE org_id=$1 AND id=$2`, f.org, op3ID).Scan(&beforeState, &beforeVersion)
	}); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	viewerDispatches := provider.dispatches
	provider.mu.Unlock()
	if err = f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET role='viewer' WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Continue(ctx, f.owner, f.org, op3ID, "viewer"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer continue error=%v", err)
	}
	provider.mu.Lock()
	if provider.dispatches != viewerDispatches {
		provider.mu.Unlock()
		t.Fatalf("viewer continuation dispatched: %d -> %d", viewerDispatches, provider.dispatches)
	}
	provider.mu.Unlock()
	var afterState string
	var afterVersion int64
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state,version FROM deployments WHERE org_id=$1 AND id=$2`, f.org, op3ID).Scan(&afterState, &afterVersion)
	}); err != nil || afterState != beforeState || afterVersion != beforeVersion {
		t.Fatalf("viewer changed operation: before=%s/%d after=%s/%d err=%v", beforeState, beforeVersion, afterState, afterVersion, err)
	}
}
