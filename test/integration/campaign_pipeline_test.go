package integration

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/policy"
)

func TestCampaignPipelineUsesNativeDispatchHealthAndCancel(t *testing.T) {
	f := newCampaignFixture(t, 1)
	ctx := context.Background()
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	policyVersion, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{Environments: []string{f.input.Members[0].Environment}, Workflows: []string{"pipeline"}}}, "campaign native contract", "campaign-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	simulation, err := policies.Simulate(ctx, f.owner, f.org, policyVersion.ID, f.repo, "", policy.Input{Action: policy.Deploy, Environment: f.input.Members[0].Environment, Workflow: "pipeline"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, policyVersion.ID, f.repo, "", 1, simulation.Hash, "campaign native contract", "campaign-pipeline"); err != nil {
		t.Fatal(err)
	}
	provider := &deploymentContractProvider{queueContractProvider: &queueContractProvider{db: f.db, connections: f.connections, org: f.org, repo: forge.RepoRef{NativeID: "1", FullName: "acme/repository-0000"}, head: strings.Repeat("a", 40), target: strings.Repeat("b", 40), tested: strings.Repeat("a", 40), ciPassing: true, files: map[string]map[string][]byte{}}, native: map[string]forge.DeploymentStatus{}, failed: map[string]bool{}, running: map[string]bool{}}
	counts := func() (int, int) {
		provider.mu.Lock()
		defer provider.mu.Unlock()
		return provider.dispatches, provider.cancelDispatches
	}
	deployments := deployment.New(f.db, f.identity, f.connections, policies, provider)
	configs, err := deployments.Configurations(ctx, f.owner, f.org)
	if err != nil || len(configs) != 1 {
		t.Fatalf("deployment config: %v count=%d", err, len(configs))
	}
	config := configs[0]
	f.input.ObservationSeconds = 0
	healthPub, healthPriv, _ := ed25519.GenerateKey(nil)
	config.Enabled = true
	config.Mode = "pipeline"
	config.Workflow = deployment.Workflow{ID: "pipeline", Path: ".github/workflows/deploy.yml", Ref: "refs/heads/main", SHA: strings.Repeat("d", 40), ConfigSHA256: strings.Repeat("e", 64)}
	config.Qualification = deployment.Qualification{Provider: "github", ServerVersion: "3.0", ConnectionVersion: f.connection.Version, EvidenceReference: "campaign-pipeline", EvidenceSHA256: strings.Repeat("a", 64), VerifiedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour), PinnedInputs: true, NativeEnforcement: true, EnvironmentSerialization: true, NoBypass: true}
	config.HealthPublicKey = base64.StdEncoding.EncodeToString(healthPub)
	config.HealthChecks = []string{"smoke"}
	config.ObservationSeconds = 0
	config.MaxEvidenceAgeSeconds = 300
	config.DeadlineSeconds = 600
	config, err = deployments.PutConfiguration(ctx, f.owner, f.org, config, config.Version, "campaign-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	f.service.ConfigureExecution(nil, deployments, nil, nil)
	deployments.RegisterGateAuthority(f.service.GateAuthorityTx)
	preview, err := f.service.Preview(ctx, f.owner, f.org, f.input)
	if err != nil || len(preview.Blockers) != 0 {
		t.Fatalf("campaign preview: %+v %v", preview.Blockers, err)
	}
	c, err := f.service.Create(ctx, f.owner, f.org, preview.ID, "native-campaign-key", "campaign-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	c, err = f.service.Control(ctx, f.owner, f.org, c.ID, "start", "native pipeline contract", "", c.Version, "campaign-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Step(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	dispatches, _ := counts()
	if dispatches != 1 {
		current, getErr := f.service.Get(ctx, f.owner, f.org, c.ID)
		t.Fatalf("native dispatches=%d campaign=%+v get=%v", dispatches, current, getErr)
	}
	members, err := f.service.Members(ctx, f.owner, f.org, c.ID, "", 100)
	if err != nil || len(members.Items) != 1 || members.Items[0].ActionID == "" {
		t.Fatalf("campaign action persistence: %+v %v", members, err)
	}
	detail, err := deployments.Get(ctx, f.owner, f.org, members.Items[0].ActionID)
	if err != nil || detail.Operation.Native == nil {
		t.Fatalf("native detail: %+v %v", detail, err)
	}
	proof := f.input.Members[0].Pipeline
	health := deployment.HealthReport{OrgID: f.org, DeploymentID: detail.Operation.ID, Environment: f.input.Members[0].Environment, ConfigurationVersion: config.Version, SourceSHA: proof.SourceSHA, ArtifactDigest: proof.ArtifactDigest, RunID: detail.Operation.Native.ID, RunAttempt: 1, Revision: proof.SourceSHA, Healthy: true, Checks: map[string]bool{"smoke": true}, ObservedAt: time.Now(), Nonce: domain.NewID()}
	raw, _ := json.Marshal(health)
	if err = deployments.ReceiveHealth(ctx, f.org, detail.Operation.ID, health, base64.StdEncoding.EncodeToString(ed25519.Sign(healthPriv, raw))); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE deployments SET updated_at=clock_timestamp()-interval '2 minutes' WHERE org_id=$1 AND id=$2`, f.org, detail.Operation.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	observed, err := deployments.Observe(ctx, f.org, detail.Operation.ID)
	if err != nil || observed.State != "healthy" {
		t.Fatalf("native health: %+v %v", observed, err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE campaign_members SET action_id=NULL,state='pending' WHERE org_id=$1 AND campaign_id=$2`, f.org, c.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	c = f.step(t, c.ID)
	restored, err := f.service.Members(ctx, f.owner, f.org, c.ID, "", 100)
	if err != nil || restored.Items[0].ActionID != detail.Operation.ID {
		t.Fatalf("lost campaign persistence did not reconcile: %+v %v", restored, err)
	}
	recoveredDispatches, _ := counts()
	if recoveredDispatches != 1 {
		t.Fatalf("recovery repeated native dispatch: %d", recoveredDispatches)
	}
	c = f.step(t, c.ID)
	if c.State != "completed" || c.Counts.Succeeded != 1 {
		t.Fatalf("campaign completion: %+v", c)
	}
	preCancelGate, err := deployments.Preview(ctx, f.owner, f.org, f.input.Members[0].Environment, *f.input.Members[0].Pipeline, "campaign-pre-dispatch-cancel")
	if err != nil || preCancelGate.Decision.Outcome != "allow" {
		t.Fatalf("pre-dispatch cancel gate: %+v %v", preCancelGate, err)
	}
	preCancelID := preCancelGate.Pipeline.CorrelationID
	preCancelKey := "campaign-pre-dispatch-key"
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO deployments(org_id,id,repository_id,environment,gate_id,requested_by,idempotency_key,state,recovery_of,native_environment) VALUES($1,$2,$3,$4,$5,$6,$7,'requested',NULLIF($8,'')::uuid,$9)`, f.org, preCancelID, preCancelGate.RepositoryID, preCancelGate.Environment, preCancelGate.ID, f.owner.User.ID, preCancelKey, preCancelGate.Request.RecoveryOf, preCancelGate.Pipeline.Environment)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	_, cancelsBefore := counts()
	preCancelled, err := deployments.Cancel(ctx, f.owner, f.org, preCancelID, 1, "campaign-pre-dispatch-cancel")
	if err != nil || preCancelled.State != "cancelled" {
		t.Fatalf("pre-dispatch cancel: %+v %v", preCancelled, err)
	}
	_, cancelsAfter := counts()
	if cancelsAfter != cancelsBefore || preCancelled.CancelState != "confirmed" {
		t.Fatalf("pre-dispatch cancel wrote native request: state=%+v cancels=%d/%d", preCancelled, cancelsAfter, cancelsBefore)
	}
	f.input.Name = "Paused native campaign"
	preview, err = f.service.Preview(ctx, f.owner, f.org, f.input)
	if err != nil || len(preview.Blockers) != 0 {
		t.Fatalf("paused preview: %+v %v", preview.Blockers, err)
	}
	pausedCampaign, err := f.service.Create(ctx, f.owner, f.org, preview.ID, "native-paused-key", "campaign-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	pausedCampaign, err = f.service.Control(ctx, f.owner, f.org, pausedCampaign.ID, "start", "start native pause contract", "", pausedCampaign.Version, "campaign-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	provider.holdNext = true
	provider.mu.Unlock()
	_ = f.step(t, pausedCampaign.ID)
	pausedCampaign, err = f.service.Get(ctx, f.owner, f.org, pausedCampaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	pausedCampaign, err = f.service.Control(ctx, f.owner, f.org, pausedCampaign.ID, "pause", "pause native contract", "", pausedCampaign.Version, "campaign-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	pausedCampaign = f.step(t, pausedCampaign.ID)
	_, cancels := counts()
	if cancels != 1 || pausedCampaign.State != "paused" {
		t.Fatalf("native pause cancel: campaign=%+v cancels=%d", pausedCampaign, cancels)
	}
	f.input.Name = "Lost native campaign"
	preview, err = f.service.Preview(ctx, f.owner, f.org, f.input)
	if err != nil || len(preview.Blockers) != 0 {
		t.Fatalf("lost preview: %+v %v", preview.Blockers, err)
	}
	lostCampaign, err := f.service.Create(ctx, f.owner, f.org, preview.ID, "native-lost-key", "campaign-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	lostCampaign, err = f.service.Control(ctx, f.owner, f.org, lostCampaign.ID, "start", "start lost native contract", "", lostCampaign.Version, "campaign-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	provider.lose = true
	provider.mu.Unlock()
	_ = f.step(t, lostCampaign.ID)
	provider.mu.Lock()
	dispatches = provider.dispatches
	provider.lose = false
	provider.mu.Unlock()
	lostCampaign = f.step(t, lostCampaign.ID)
	finalDispatches, _ := counts()
	if finalDispatches != dispatches || lostCampaign.Counts.Unknown != 1 {
		t.Fatalf("lost native recovery repeated dispatch: campaign=%+v dispatches=%d/%d", lostCampaign, finalDispatches, dispatches)
	}
}
