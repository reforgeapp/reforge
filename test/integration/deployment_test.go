package integration

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/connections"
	"reforge/internal/deployment"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/providers"
)

type deploymentContractProvider struct {
	*queueContractProvider
	dispatches       int
	mu               sync.Mutex
	native           map[string]forge.DeploymentStatus
	lose             bool
	cancelDispatches int
	loseCancel       bool
	failed           map[string]bool
	failNext         bool
	running          map[string]bool
	holdNext         bool
	trackState       string
}

func (p *deploymentContractProvider) ForProtection(string, map[string]string) providers.Client {
	return p
}
func (p *deploymentContractProvider) Read(ctx context.Context, org, id string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if err := p.authorize(ctx, id, authorize); err != nil {
		return privateconnector.Result{}, err
	}
	switch op.Kind {
	case privateconnector.ForgeReadChange:
		return privateconnector.Result{Change: &forge.Change{ID: op.Change.ChangeID, Repository: p.repo, TargetRepository: p.repo, HeadRepository: p.repo, State: "merged", HeadSHA: p.head, TargetSHA: p.target, MergeSHA: p.head, TargetBranch: "main"}}, nil
	case privateconnector.ForgePipelineInspect:
		if op.Pipeline == nil || !op.Pipeline.ObserveOnly || op.Pipeline.Repository != p.repo || (op.Pipeline.WorkflowSHA != strings.Repeat("d", 40) && op.Pipeline.WorkflowSHA != strings.Repeat("f", 40)) || (op.Pipeline.ConfigSHA256 != strings.Repeat("e", 64) && op.Pipeline.ConfigSHA256 != strings.Repeat("9", 64)) {
			return privateconnector.Result{}, privateconnector.ErrInvalid
		}
		return privateconnector.Result{DeploymentGates: &forge.DeploymentGates{State: "configured", NativeEnforced: domain.Supported, Environment: op.Pipeline.Environment, RulesHash: strings.Repeat("c", 64)}}, nil
	case privateconnector.ForgeDeliveryGates:
		return privateconnector.Result{DeploymentGates: &forge.DeploymentGates{State: "configured", NativeEnforced: domain.Supported, Environment: op.Delivery.Environment, RulesHash: strings.Repeat("c", 64)}}, nil
	case privateconnector.ForgeDeliveryStatus:
		state := p.trackState
		if state == "" {
			state = "success"
		}
		return privateconnector.Result{Deployment: &forge.DeploymentStatus{ID: op.Delivery.RunID, State: state, SourceSHA: p.head, WorkflowSHA: strings.Repeat("d", 40), ArtifactDigest: "sha256:" + strings.Repeat("b", 64), Environment: "production", WorkflowID: "pipeline", WorkflowPath: ".gitlab-ci.yml", Ref: "refs/heads/main", Event: "api", RunAttempt: 1}}, nil
	case privateconnector.ForgePipelineObserve:
		p.mu.Lock()
		defer p.mu.Unlock()
		native, ok := p.native[op.Pipeline.CorrelationID]
		if !ok {
			native = forge.DeploymentStatus{ID: op.Pipeline.RunID, State: p.trackState, SourceSHA: p.head, WorkflowSHA: strings.Repeat("d", 40), ArtifactDigest: "sha256:" + strings.Repeat("b", 64), Environment: op.Pipeline.Environment, CorrelationID: op.Pipeline.CorrelationID, WorkflowID: op.Pipeline.WorkflowID, WorkflowPath: op.Pipeline.WorkflowPath, Ref: op.Pipeline.Ref, Event: "api", RunAttempt: 1}
			if native.State == "" {
				native.State = "success"
			}
		}
		if p.running[op.Pipeline.CorrelationID] {
			native.State = "running"
		} else if p.failed[op.Pipeline.CorrelationID] {
			native.State = "failed"
		} else {
			native.State = "success"
		}
		p.native[op.Pipeline.CorrelationID] = native
		return privateconnector.Result{Deployment: &native}, nil
	default:
		return privateconnector.Result{}, privateconnector.ErrUnsupported
	}
}
func (p *deploymentContractProvider) Write(ctx context.Context, org, id string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) (string, error), current func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if _, err := p.authorizeWrite(ctx, id, authorize); err != nil {
		return privateconnector.Result{}, err
	}
	if err := p.authorize(ctx, id, current); err != nil {
		return privateconnector.Result{}, err
	}
	p.mu.Lock()
	if op.Kind == privateconnector.ForgePipelineCancel {
		p.cancelDispatches++
		native, ok := p.native[op.Pipeline.CorrelationID]
		if !ok || native.ID != op.Pipeline.RunID {
			p.mu.Unlock()
			return privateconnector.Result{}, privateconnector.ErrConflict
		}
		native.State = "cancelled"
		p.native[op.Pipeline.CorrelationID] = native
		lose := p.loseCancel
		p.mu.Unlock()
		if lose {
			return privateconnector.Result{}, nil
		}
		return privateconnector.Result{Deployment: &native}, nil
	}
	p.dispatches++
	failed := p.failNext
	p.failNext = false
	native := forge.DeploymentStatus{ID: "pipeline-" + op.Pipeline.CorrelationID, State: "queued", SourceSHA: p.head, WorkflowSHA: op.Pipeline.WorkflowSHA, ArtifactDigest: "sha256:" + strings.Repeat("b", 64), Environment: "production", CorrelationID: op.Pipeline.CorrelationID, WorkflowID: op.Pipeline.WorkflowID, WorkflowPath: op.Pipeline.WorkflowPath, Ref: op.Pipeline.Ref, Event: "api", RunAttempt: 1}
	p.native[op.Pipeline.CorrelationID] = native
	p.failed[op.Pipeline.CorrelationID] = failed
	p.running[op.Pipeline.CorrelationID] = p.holdNext
	p.holdNext = false
	lose := p.lose
	p.mu.Unlock()
	if lose {
		return privateconnector.Result{}, nil
	}
	return privateconnector.Result{Deployment: &native}, nil
}

func TestDeploymentSignedProvenanceAndHealthContract(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET state='healthy',server_version='3.0',verified_at=now(),settings=jsonb_build_object('auth_kind','github_app','app_id','42','billing_route','forge') WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PutConfig(ctx, f.owner, f.org, f.repo, discovery.Config{MergeAuthority: "reforge"}, 0, "deploy-contract"); err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{Environments: []string{"production"}, Workflows: []string{"pipeline", "recovery"}}}, "deploy contract", "deploy-contract")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Deploy, Environment: "production", Workflow: "pipeline"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, sim.Hash, "deploy contract", "deploy-contract"); err != nil {
		t.Fatal(err)
	}
	pub, priv, _ := ed25519.GenerateKey(nil)
	hpub, hpriv, _ := ed25519.GenerateKey(nil)
	head, target := strings.Repeat("a", 40), strings.Repeat("b", 40)
	artifact := "sha256:" + strings.Repeat("b", 64)
	workflowSHA := strings.Repeat("d", 40)
	configHash := strings.Repeat("e", 64)
	repo := forge.RepoRef{NativeID: "1", FullName: "acme/repository-0000"}
	provider := &deploymentContractProvider{queueContractProvider: &queueContractProvider{db: f.db, connections: f.connections, org: f.org, repo: repo, head: head, target: target, tested: head, ciPassing: true, files: map[string]map[string][]byte{target: {"base": []byte("base")}, head: {"base": []byte("head")}}}, native: map[string]forge.DeploymentStatus{}, failed: map[string]bool{}, running: map[string]bool{}}
	svc := deployment.New(f.db, f.identity, f.connections, policies, provider)
	config := deployment.Configuration{Environment: "production", RepositoryID: f.repo, Enabled: true, Mode: "pipeline", Workflow: deployment.Workflow{ID: "pipeline", Path: ".gitlab-ci.yml", Ref: "refs/heads/main", SHA: workflowSHA, ConfigSHA256: configHash}, ProvenancePublicKey: base64.StdEncoding.EncodeToString(pub), HealthPublicKey: base64.StdEncoding.EncodeToString(hpub), HealthChecks: []string{"smoke"}, ObservationSeconds: 1, MaxEvidenceAgeSeconds: 300, DeadlineSeconds: 600, Qualification: deployment.Qualification{Provider: "github", ServerVersion: "3.0", ConnectionVersion: f.connection.Version, EvidenceReference: "deploy-contract", EvidenceSHA256: strings.Repeat("a", 64), VerifiedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour), PinnedInputs: true, NativeEnforcement: true, EnvironmentSerialization: true, NoBypass: true}}
	config, err = svc.PutConfiguration(ctx, f.owner, f.org, config, 0, "deploy-contract")
	if err != nil {
		t.Fatal(err)
	}
	provenance := deployment.Provenance{OrgID: f.org, RepositoryID: f.repo, SourceSHA: head, ArtifactDigest: artifact, BuildID: "build-1", IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour)}
	raw, _ := json.Marshal(provenance)
	signed := deployment.SignedProvenance{Document: provenance, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))}
	gate, err := svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "deploy-contract")
	if err != nil || gate.Decision.Outcome != "allow" {
		t.Fatalf("preview: %+v %v", gate, err)
	}
	paused := config
	paused.Enabled = false
	paused, err = svc.PutConfiguration(ctx, f.owner, f.org, paused, config.Version, "pause")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Request(ctx, f.owner, f.org, gate.ID, "paused-key", "paused"); err == nil {
		t.Fatal("paused configuration accepted request")
	}
	config.Enabled = true
	config, err = svc.PutConfiguration(ctx, f.owner, f.org, config, paused.Version, "resume")
	if err != nil {
		t.Fatal(err)
	}
	gate, err = svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "deploy-contract-2")
	if err != nil || gate.Decision.Outcome != "allow" {
		t.Fatalf("preview after resume: %+v %v", gate, err)
	}
	key := "deploy-key"
	op, err := svc.Request(ctx, f.owner, f.org, gate.ID, key, "deploy-contract")
	if err != nil || provider.dispatches != 1 {
		t.Fatalf("request: %+v %v dispatches=%d", op, err, provider.dispatches)
	}
	contentionGate, err := svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "contention-preview")
	if err != nil || contentionGate.Decision.Outcome != "allow" {
		t.Fatalf("contention preview: %+v %v", contentionGate, err)
	}
	if _, err = svc.Request(ctx, f.owner, f.org, contentionGate.ID, "contention-key", "contention"); err == nil {
		t.Fatal("second active deployment accepted for same environment")
	}
	cancelled, err := svc.Cancel(ctx, f.owner, f.org, op.ID, op.Version, "cancel")
	if err != nil || cancelled.State != "cancelled" || provider.cancelDispatches != 1 {
		t.Fatalf("explicit cancellation: %+v %v cancels=%d", cancelled, err, provider.cancelDispatches)
	}
	gate, err = svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "post-cancel-preview")
	if err != nil || gate.Decision.Outcome != "allow" {
		t.Fatalf("post-cancel preview: %+v %v", gate, err)
	}
	key = "deploy-key-2"
	op, err = svc.Request(ctx, f.owner, f.org, gate.ID, key, "deploy-contract-2")
	if err != nil || provider.dispatches != 2 {
		t.Fatalf("post-cancel request: %+v %v dispatches=%d", op, err, provider.dispatches)
	}
	replay, err := svc.Request(ctx, f.owner, f.org, gate.ID, key, "replay")
	if err != nil || replay.ID != op.ID || provider.dispatches != 2 {
		t.Fatalf("replay: %+v %v dispatches=%d", replay, err, provider.dispatches)
	}
	observed, err := svc.Observe(ctx, f.org, op.ID)
	if err != nil || observed.State != "completed_unverified" {
		t.Fatalf("native completion: %+v %v", observed, err)
	}
	runID := "pipeline-unknown"
	if op.Native != nil {
		runID = op.Native.ID
	}
	bad := deployment.HealthReport{OrgID: f.org, DeploymentID: op.ID, Environment: "production", ConfigurationVersion: config.Version, SourceSHA: head, ArtifactDigest: artifact, RunID: "wrong", RunAttempt: 1, Revision: head, Healthy: true, Checks: map[string]bool{"smoke": true}, ObservedAt: time.Now(), Nonce: domain.NewID()}
	raw, _ = json.Marshal(bad)
	if err = svc.ReceiveHealth(ctx, f.org, op.ID, bad, base64.StdEncoding.EncodeToString(ed25519.Sign(hpriv, raw))); err == nil {
		t.Fatal("mismatched native run accepted")
	}
	bad.RunID = runID
	if err = svc.ReceiveHealth(ctx, f.org, op.ID, bad, "bad"); err == nil {
		t.Fatal("bad health signature accepted")
	}
	bad.Nonce = domain.NewID()
	bad.ArtifactDigest = "sha256:" + strings.Repeat("c", 64)
	raw, _ = json.Marshal(bad)
	if err = svc.ReceiveHealth(ctx, f.org, op.ID, bad, base64.StdEncoding.EncodeToString(ed25519.Sign(hpriv, raw))); err == nil {
		t.Fatal("mismatched artifact digest accepted")
	}
	first := bad
	first.ArtifactDigest = artifact
	raw, _ = json.Marshal(first)
	if err = svc.ReceiveHealth(ctx, f.org, op.ID, first, base64.StdEncoding.EncodeToString(ed25519.Sign(hpriv, raw))); err != nil {
		t.Fatal(err)
	}
	if err = svc.ReceiveHealth(ctx, f.org, op.ID, first, base64.StdEncoding.EncodeToString(ed25519.Sign(hpriv, raw))); err != nil {
		t.Fatalf("health nonce replay not idempotent: %v", err)
	}
	verifying, err := svc.Observe(ctx, f.org, op.ID)
	if err != nil || verifying.State != "verifying" {
		t.Fatalf("health window: %+v %v", verifying, err)
	}
	second := first
	second.Nonce = domain.NewID()
	time.Sleep(1100 * time.Millisecond)
	second.ObservedAt = time.Now()
	raw, _ = json.Marshal(second)
	if err = svc.ReceiveHealth(ctx, f.org, op.ID, second, base64.StdEncoding.EncodeToString(ed25519.Sign(hpriv, raw))); err != nil {
		t.Fatal(err)
	}
	healthy, err := svc.Observe(ctx, f.org, op.ID)
	if err != nil || healthy.State != "healthy" {
		t.Fatalf("healthy completion: %+v %v", healthy, err)
	}
	config.RecoveryWorkflow = &deployment.Workflow{ID: "recovery", Path: ".gitlab-ci.yml", Ref: "refs/heads/main", SHA: strings.Repeat("f", 40), ConfigSHA256: strings.Repeat("9", 64)}
	config, err = svc.PutConfiguration(ctx, f.owner, f.org, config, config.Version, "configure-recovery")
	if err != nil {
		t.Fatalf("configure recovery: %T %v", err, err)
	}
	provider.mu.Lock()
	provider.failNext = true
	provider.mu.Unlock()
	failedGate, err := svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "failed-preview")
	if err != nil || failedGate.Decision.Outcome != "allow" {
		t.Fatalf("failed deployment preview: %+v %v", failedGate, err)
	}
	failedOp, err := svc.Request(ctx, f.owner, f.org, failedGate.ID, "failed-key", "failed-deployment")
	if err != nil {
		t.Fatal(err)
	}
	failedOp, err = svc.Observe(ctx, f.org, failedOp.ID)
	if err != nil || failedOp.State != "failed" {
		t.Fatalf("failed deployment: %+v %v", failedOp, err)
	}
	recoveryGate, err := svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed, RecoveryOf: failedOp.ID, RestoreDeploymentID: healthy.ID}, "recovery-preview")
	if err != nil || recoveryGate.Decision.Outcome != "allow" {
		t.Fatalf("recovery preview: %+v %v", recoveryGate, err)
	}
	recovery, err := svc.Request(ctx, f.owner, f.org, recoveryGate.ID, "recovery-key", "recovery")
	if err != nil {
		t.Fatal(err)
	}
	recovery, err = svc.Observe(ctx, f.org, recovery.ID)
	if err != nil || recovery.State != "completed_unverified" {
		t.Fatalf("recovery native completion: %+v %v", recovery, err)
	}
	recoveryHealth := deployment.HealthReport{OrgID: f.org, DeploymentID: recovery.ID, Environment: "production", ConfigurationVersion: config.Version, SourceSHA: head, ArtifactDigest: artifact, RunID: recovery.Native.ID, RunAttempt: 1, Revision: head, Healthy: true, Checks: map[string]bool{"smoke": true}, ObservedAt: time.Now(), Nonce: domain.NewID()}
	raw, _ = json.Marshal(recoveryHealth)
	if err = svc.ReceiveHealth(ctx, f.org, recovery.ID, recoveryHealth, base64.StdEncoding.EncodeToString(ed25519.Sign(hpriv, raw))); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Observe(ctx, f.org, recovery.ID); err != nil {
		t.Fatal(err)
	}
	recoveryHealth.Nonce = domain.NewID()
	time.Sleep(1100 * time.Millisecond)
	recoveryHealth.ObservedAt = time.Now()
	raw, _ = json.Marshal(recoveryHealth)
	if err = svc.ReceiveHealth(ctx, f.org, recovery.ID, recoveryHealth, base64.StdEncoding.EncodeToString(ed25519.Sign(hpriv, raw))); err != nil {
		t.Fatal(err)
	}
	recovered, err := svc.Observe(ctx, f.org, recovery.ID)
	if err != nil || recovered.State != "recovered" {
		t.Fatalf("recovered deployment: %+v %v", recovered, err)
	}
	autoGate, err := svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "auto-cancel-preview")
	if err != nil || autoGate.Decision.Outcome != "allow" {
		t.Fatalf("auto-cancel preview: %+v %v", autoGate, err)
	}
	provider.mu.Lock()
	provider.holdNext = true
	provider.mu.Unlock()
	autoOp, err := svc.Request(ctx, f.owner, f.org, autoGate.ID, "auto-cancel-key", "auto-cancel")
	if err != nil {
		t.Fatal(err)
	}
	pausedConfig := config
	pausedConfig.Enabled = false
	pausedConfig, err = svc.PutConfiguration(ctx, f.owner, f.org, pausedConfig, config.Version, "pause-active")
	if err != nil {
		t.Fatal(err)
	}
	autoCanceled, err := svc.Observe(ctx, f.org, autoOp.ID)
	if err != nil || autoCanceled.State != "cancelled" || provider.cancelDispatches != 2 {
		t.Fatalf("pause auto-cancel: %+v %v cancels=%d", autoCanceled, err, provider.cancelDispatches)
	}
	config.Enabled = true
	config, err = svc.PutConfiguration(ctx, f.owner, f.org, config, pausedConfig.Version, "resume-active")
	if err != nil {
		t.Fatal(err)
	}
	revokeGate, err := svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "revoke-preview")
	if err != nil || revokeGate.Decision.Outcome != "allow" {
		t.Fatalf("revoke preview: %+v %v", revokeGate, err)
	}
	provider.mu.Lock()
	provider.holdNext = true
	provider.mu.Unlock()
	revokeOp, err := svc.Request(ctx, f.owner, f.org, revokeGate.ID, "revoke-key", "revoke")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET role='viewer' WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	revoked, err := svc.Observe(ctx, f.org, revokeOp.ID)
	if err != nil || revoked.State != "cancelled" || provider.cancelDispatches != 3 {
		t.Fatalf("requester revocation auto-cancel: %+v %v cancels=%d", revoked, err, provider.cancelDispatches)
	}
	if err = f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET role='owner' WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	otherOrg := domain.NewID()
	if err = f.db.Tenant(ctx, otherOrg, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Other deployment org')`, otherOrg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, otherOrg, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Get(ctx, f.owner, otherOrg, op.ID); err == nil {
		t.Fatal("cross-org operation leaked")
	}

	provider.mu.Lock()
	provider.loseCancel = true
	provider.mu.Unlock()
	provider.mu.Lock()
	provider.lose = false
	provider.mu.Unlock()
	lostGate, err := svc.Preview(ctx, f.owner, f.org, "production", deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, "lost-cancel-preview")
	if err != nil || lostGate.Decision.Outcome != "allow" {
		t.Fatalf("lost response preview: %+v %v", lostGate, err)
	}
	lost, err := svc.Request(ctx, f.owner, f.org, lostGate.ID, "lost-cancel-key", "lost-cancel")
	if err != nil || provider.dispatches != 7 {
		t.Fatalf("lost response request: %+v %v dispatches=%d", lost, err, provider.dispatches)
	}
	emptyRevision := deployment.HealthReport{OrgID: f.org, DeploymentID: lost.ID, Environment: "production", ConfigurationVersion: config.Version, SourceSHA: head, ArtifactDigest: artifact, RunID: lost.Native.ID, RunAttempt: 1, Revision: "", Healthy: true, Checks: map[string]bool{"smoke": true}, ObservedAt: time.Now(), Nonce: domain.NewID()}
	raw, _ = json.Marshal(emptyRevision)
	if err = svc.ReceiveHealth(ctx, f.org, lost.ID, emptyRevision, base64.StdEncoding.EncodeToString(ed25519.Sign(hpriv, raw))); err == nil {
		t.Fatal("empty health revision accepted")
	}
	provider.mu.Lock()
	cancelCount := provider.cancelDispatches
	provider.mu.Unlock()
	uncertain, err := svc.Cancel(ctx, f.owner, f.org, lost.ID, lost.Version, "lost-cancel-response")
	if err != nil || uncertain.CancelState != "uncertain" || cancelCount+1 != provider.cancelDispatches {
		t.Fatalf("lost cancellation response: %+v %v cancels=%d", uncertain, err, provider.cancelDispatches)
	}
	restarted := deployment.New(f.db, f.identity, f.connections, policies, provider)
	recoveredLost, err := restarted.Observe(ctx, f.org, lost.ID)
	if err != nil || recoveredLost.State != "completed_unverified" || provider.dispatches != 7 || provider.cancelDispatches != cancelCount+1 {
		t.Fatalf("restart cancellation recovery: %+v %v dispatches=%d cancels=%d", recoveredLost, err, provider.dispatches, provider.cancelDispatches)
	}
}

func TestDeploymentTrackObserveOnlyContract(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET state='healthy',server_version='3.0',verified_at=now(),settings=jsonb_build_object('auth_kind','github_app','app_id','42') WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PutConfig(ctx, f.owner, f.org, f.repo, discovery.Config{MergeAuthority: "reforge"}, 0, "track-contract"); err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, policy.Policy{Schema: "maintenance/v1"}, "track policy", "track-policy")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Read})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, sim.Hash, "track policy", "track-policy"); err != nil {
		t.Fatal(err)
	}
	pub, priv, _ := ed25519.GenerateKey(nil)
	head := strings.Repeat("a", 40)
	artifact := "sha256:" + strings.Repeat("b", 64)
	repo := forge.RepoRef{NativeID: "1", FullName: "acme/repository-0000"}
	provider := &deploymentContractProvider{queueContractProvider: &queueContractProvider{db: f.db, connections: f.connections, org: f.org, repo: repo, head: head, target: strings.Repeat("b", 40), tested: head, ciPassing: true, files: map[string]map[string][]byte{}}, native: map[string]forge.DeploymentStatus{}, failed: map[string]bool{}, running: map[string]bool{}, trackState: "running"}
	svc := deployment.New(f.db, f.identity, f.connections, policies, provider)
	config := deployment.Configuration{Environment: "production", RepositoryID: f.repo, Enabled: true, Mode: "observe", Workflow: deployment.Workflow{ID: "pipeline", Path: ".gitlab-ci.yml", Ref: "refs/heads/main", SHA: strings.Repeat("d", 40), ConfigSHA256: strings.Repeat("e", 64)}, ProvenancePublicKey: base64.StdEncoding.EncodeToString(pub), HealthPublicKey: base64.StdEncoding.EncodeToString(pub), HealthChecks: []string{"smoke"}, ObservationSeconds: 1, MaxEvidenceAgeSeconds: 300, DeadlineSeconds: 600}
	config, err = svc.PutConfiguration(ctx, f.owner, f.org, config, 0, "track-config")
	if err != nil {
		t.Fatal(err)
	}
	provenance := deployment.Provenance{OrgID: f.org, RepositoryID: f.repo, SourceSHA: head, ArtifactDigest: artifact, BuildID: "track-build", IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour)}
	raw, _ := json.Marshal(provenance)
	signed := deployment.SignedProvenance{Document: provenance, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))}
	in := deployment.TrackRequest{PreviewRequest: deployment.PreviewRequest{ChangeID: "1", SourceSHA: head, ArtifactDigest: artifact, Provenance: signed}, RunID: "123", IdempotencyKey: "track-key"}
	op, err := svc.Track(ctx, f.owner, f.org, "production", in, "track")
	if err != nil || op.State != "running" || provider.dispatches != 0 {
		t.Fatalf("track: %+v %v writes=%d", op, err, provider.dispatches)
	}
	replay, err := svc.Track(ctx, f.owner, f.org, "production", in, "track-replay")
	if err != nil || replay.ID != op.ID || provider.dispatches != 0 {
		t.Fatalf("track replay: %+v %v writes=%d", replay, err, provider.dispatches)
	}
	paused := config
	paused.Enabled = false
	paused, err = svc.PutConfiguration(ctx, f.owner, f.org, paused, config.Version, "track-pause")
	if err != nil {
		t.Fatal(err)
	}
	observed, err := svc.Observe(ctx, f.org, op.ID)
	if err != nil || provider.cancelDispatches != 0 || observed.State != "running" {
		t.Fatalf("observe-only pause: %+v %v cancels=%d", observed, err, provider.cancelDispatches)
	}
	if _, err = svc.Cancel(ctx, f.owner, f.org, op.ID, observed.Version, "public-cancel"); err == nil {
		t.Fatal("public cancel accepted observe-only operation")
	}
	config.Enabled = true
	config, err = svc.PutConfiguration(ctx, f.owner, f.org, config, paused.Version, "track-resume")
	if err != nil {
		t.Fatal(err)
	}
	bad := in
	bad.IdempotencyKey = "bad-proof"
	bad.Provenance.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte("forged")))
	if _, err = svc.Track(ctx, f.owner, f.org, "production", bad, "bad-proof"); err == nil {
		t.Fatal("forged tracking proof accepted")
	}
	otherOrg := domain.NewID()
	if err = f.db.Tenant(ctx, otherOrg, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Track other org')`, otherOrg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, otherOrg, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Get(ctx, f.owner, otherOrg, op.ID); err == nil {
		t.Fatal("cross-org tracked operation leaked")
	}
}
