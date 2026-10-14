package integration

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/autopilot"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/config"
	deliverycmd "github.com/reforgeapp/reforge/pkg/delivery"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/environmenthealth"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/httpapi"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/mergecontrol"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/release"
	"github.com/reforgeapp/reforge/pkg/runner"
	"github.com/reforgeapp/reforge/pkg/workflow"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/gitops"
	"github.com/reforgeapp/reforge/pkg/inventory"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/providers"
)

func TestGitOpsLiveProtectedPromotion(t *testing.T) {
	if os.Getenv("REFORGE_LIVE_GITOPS_TEST") != "1" {
		t.Skip("requires disposable PostgreSQL and Gitea")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root := os.Getenv("REFORGE_TEST_ROOT")
	if root == "" {
		t.Fatal("REFORGE_TEST_ROOT is required")
	}
	f := newInventoryFixture(t, 0)
	source := localPrivateRepository(t)
	delivery := localPrivateRepository(t)

	native := func(repo forge.RepoRef, actor, method, suffix string, in, out any) {
		t.Helper()
		body, _ := json.Marshal(in)
		var res *http.Response
		for attempt := 0; attempt < 50; attempt++ {
			req, _ := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:53000/api/v1/repos/"+repo.FullName+suffix, bytes.NewReader(body))
			req.Header.Set("Authorization", "token "+readGiteaToken(t, root, actor))
			req.Header.Set("Content-Type", "application/json")
			var err error
			res, err = (&http.Client{Timeout: 10 * time.Second}).Do(req)
			if err != nil {
				t.Fatal("local native request failed")
			}
			if res.StatusCode >= 200 && res.StatusCode < 300 {
				break
			}
			var failure struct{ Message string }
			_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&failure)
			_ = res.Body.Close()
			if attempt < 49 && method == "POST" && strings.HasSuffix(suffix, "/merge") && res.StatusCode == 405 && failure.Message == "Please try again later" {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			t.Fatalf("native %s %s HTTP%d: %s", method, suffix, res.StatusCode, failure.Message)
		}
		defer res.Body.Close()
		if out != nil && json.NewDecoder(res.Body).Decode(out) != nil {
			t.Fatal("native response invalid")
		}
	}
	for _, repo := range []forge.RepoRef{source, delivery} {
		native(repo, "bot", "PUT", "/collaborators/reforge-reviewer", map[string]string{"permission": "write"}, nil)
	}
	native(source, "bot", "POST", "/branches", map[string]string{"new_branch_name": "release", "old_ref_name": "main"}, nil)
	for path, content := range map[string]string{"app.cjs": liveCheckoutApplication, "app.test.cjs": liveCheckoutTest, ".gitea/workflows/checks.yml": liveCheckoutWorkflow} {
		native(source, "bot", "POST", "/contents/"+path, map[string]string{"branch": "release", "message": "Add qualified checkout checks", "content": base64.StdEncoding.EncodeToString([]byte(content))}, nil)
	}
	var sourcePR struct {
		Number   int64  `json:"number"`
		MergeSHA string `json:"merge_commit_sha"`
		Head     struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	native(source, "bot", "POST", "/pulls", map[string]string{"head": "release", "base": "main", "title": "Release fixture"}, &sourcePR)
	for i := 0; i < 60; i++ {
		var state struct {
			Mergeable bool `json:"mergeable"`
		}
		native(source, "bot", "GET", fmt.Sprintf("/pulls/%d", sourcePR.Number), nil, &state)
		if state.Mergeable {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	native(source, "bot", "POST", fmt.Sprintf("/pulls/%d/merge", sourcePR.Number), map[string]any{"Do": "merge", "head_commit_id": sourcePR.Head.SHA}, nil)
	native(source, "bot", "GET", fmt.Sprintf("/pulls/%d", sourcePR.Number), nil, &sourcePR)
	native(delivery, "bot", "POST", "/contents/deploy.yaml", map[string]string{"branch": "main", "message": "Initial delivery manifest", "content": base64.StdEncoding.EncodeToString([]byte("image: registry.example/app@sha256:" + strings.Repeat("0", 64) + "\n"))}, nil)
	native(delivery, "bot", "PATCH", "", map[string]any{"allow_fast_forward_only_merge": true}, nil)
	native(delivery, "bot", "POST", "/branch_protections", map[string]any{"rule_name": "main", "enable_push": false, "required_approvals": 1, "dismiss_stale_approvals": true, "block_on_outdated_branch": true, "block_on_rejected_reviews": true, "block_on_official_review_requests": true, "block_admin_merge_override": true, "enable_status_check": false}, nil)
	jobs := workflow.New(f.db, f.identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) {
		return "", errors.New("no repair authority")
	})
	runners := runner.New(f.db, f.identity, jobs, nil)
	f.connections.RegisterRunnerCheck(runners.CheckRunnerTx)
	pool, err := runners.PutPool(ctx, f.owner, f.org, "", runner.PoolInput{Name: "GitOps private connector", RepositoryIDs: []string{}}, 0, "gitops-live")
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := runners.EnrollToken(ctx, f.owner, f.org, pool.ID, "gitops-live")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := runners.Enroll(ctx, enrollment.Token, "gitops-live")
	if err != nil {
		t.Fatal(err)
	}
	connector, err := privateconnector.New(privateconnector.Config{Authenticate: runners.AuthenticateSupervisor, Development: true, TTL: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	providers.Factory{Development: true}.Register(f.connections)
	providers.RegisterPrivate(f.connections, connector, runners)
	transport := httptest.NewUnstartedServer(nil)
	api := httpapi.New(config.Config{PublicURL: "http://" + transport.Listener.Addr().String(), Development: true}, f.db)
	api.RegisterPrivateConnector(connector)
	transport.Config.Handler = api.Router
	transport.Start()
	defer transport.Close()
	client, err := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: transport.URL, Credential: supervisor.Token, Target: privateconnector.Target{OrgID: f.org, RunnerID: supervisor.Runner.ID}, Development: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	privateCtx, stopPrivate := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for privateCtx.Err() == nil {
			_ = client.RunOnce(privateCtx)
			select {
			case <-privateCtx.Done():
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()
	defer func() { stopPrivate(); <-done }()
	route := &connections.Route{RunnerID: supervisor.Runner.ID, Host: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}}
	secret := readGiteaToken(t, root, "reviewer")
	realConnection, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "forge", Provider: "gitea", Name: "GitOps live forge", Endpoint: "http://127.0.0.1:53000", Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge"}, Secret: secret, PrivateRoute: route}, "gitops-live")
	if err != nil {
		t.Fatal(err)
	}
	realConnection, err = f.connections.Test(ctx, f.owner, f.org, realConnection.ID, realConnection.Version, "gitops-live")
	if err != nil || realConnection.State != "healthy" {
		t.Fatalf("Gitea connection: %v state=%s", err, realConnection.State)
	}
	reader := providers.New(f.db, f.connections, connector, runners, true)
	portfolio := inventory.New(f.db, f.identity, f.vault, reader, providers.DecodeWebhook)
	scan, err := portfolio.StartSync(ctx, f.owner, f.org, inventory.SyncInput{ConnectionID: realConnection.ID}, "gitops-live")
	if err != nil {
		t.Fatal(err)
	}
	scan = drainLiveInventory(t, ctx, portfolio, f, scan.ID)
	imported, err := portfolio.StartImport(ctx, f.owner, f.org, scan.ID, inventory.ImportInput{NativeIDs: []string{source.NativeID, delivery.NativeID}}, scan.Version, "gitops-live")
	if err != nil {
		t.Fatal(err)
	}
	drainLiveInventory(t, ctx, portfolio, f, imported.ID)
	var sourceID, deliveryID string
	page, err := portfolio.Repositories(ctx, f.owner, f.org, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, repo := range page.Items {
		if repo.NativeID == source.NativeID {
			sourceID = repo.ID
		}
		if repo.NativeID == delivery.NativeID {
			deliveryID = repo.ID
		}
	}
	if sourceID == "" || deliveryID == "" || sourceID == deliveryID {
		t.Fatalf("imported source/delivery repositories: %q %q", sourceID, deliveryID)
	}
	provenance, provenanceKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	health, healthKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	document := policy.Policy{Schema: "maintenance/v1", Defaults: policy.Defaults{BranchPrefix: "reforge/"}, Allow: policy.Lists{MergeMethods: []string{"fast-forward-only"}, Environments: []string{"production"}, Workflows: []string{"gitops:production"}}, Limits: policy.Limits{Concurrency: ptrLive(10), Attempts: ptrLive(3), ChangedFiles: ptrLive(10), ChangedLines: ptrLive(1000), OpenChanges: ptrLive(10)}}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}

	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, document, "GitOps fixture", "gitops-live")
	if err != nil {
		t.Fatal(err)
	}
	simulation, err := policies.Simulate(ctx, f.owner, f.org, version.ID, deliveryID, "", policy.Input{Action: policy.Publish})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, deliveryID, "", 0, simulation.Hash, "GitOps fixture", "gitops-live"); err != nil {
		t.Fatal(err)
	}
	inspector, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "forge", Provider: "gitea", Name: "GitOps protection reader", Endpoint: realConnection.Endpoint, Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge"}, Secret: readGiteaToken(t, root, "inspector"), PrivateRoute: route}, "gitops-live")
	if err != nil {
		t.Fatal(err)
	}
	inspector, err = f.connections.Test(ctx, f.owner, f.org, inspector.ID, inspector.Version, "gitops-live")
	if err != nil || inspector.State != "healthy" {
		t.Fatalf("inspector: %v", err)
	}
	discoveries := discovery.New(f.db, f.identity, reader)
	for _, repo := range []string{sourceID, deliveryID} {
		if _, err = discoveries.PutConfig(ctx, f.owner, f.org, repo, discovery.Config{MergeAuthority: "reforge"}, 0, "gitops-live"); err != nil {
			t.Fatal(err)
		}
	}
	merges := mergecontrol.New(f.db, f.identity, f.connections, policies, reader)
	qualification := mergecontrol.Qualification{Provider: "gitea", ServerVersion: realConnection.ServerVersion, ConnectionVersion: realConnection.Version, InspectorVersion: inspector.Version, EvidenceReference: "local-gitea-protected-gitops", EvidenceSHA256: fileDigest(t, filepath.Join(root, "test/forge/gitea/contract_test.go")), VerifiedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour), ExactHead: true, StrictTarget: true}
	if _, err = merges.PutConfig(ctx, f.owner, f.org, deliveryID, mergecontrol.Configuration{Enabled: true, InspectorConnectionID: inspector.ID, CheckPublishers: map[string]string{}, CooperationReference: "Disposable GitOps fixture has no other automerge actor", Qualification: qualification}, 0, "gitops-live"); err != nil {
		t.Fatal(err)
	}
	native(source, "bot", "PATCH", "", map[string]any{"allow_fast_forward_only_merge": true}, nil)
	native(source, "bot", "POST", "/branch_protections", map[string]any{"rule_name": "main", "enable_push": false, "required_approvals": 1, "dismiss_stale_approvals": true, "block_on_outdated_branch": true, "block_on_rejected_reviews": true, "block_on_official_review_requests": true, "block_admin_merge_override": true, "enable_status_check": false}, nil)
	if _, err = merges.PutConfig(ctx, f.owner, f.org, sourceID, mergecontrol.Configuration{Enabled: true, InspectorConnectionID: inspector.ID, CheckPublishers: map[string]string{}, CooperationReference: "Disposable source fixture has no other automerge actor", Qualification: qualification}, 0, "gitops-live"); err != nil {
		t.Fatal(err)
	}
	workflowDigest := sha256.Sum256([]byte(liveCheckoutWorkflow))
	faults := &gitopsLostResponses{Client: reader, stage: true, publish: true, stageEntered: make(chan struct{}), stageRelease: make(chan struct{}), firstClaimed: make(chan struct{}), duplicateFinished: make(chan struct{})}
	service := gitops.New(f.db, f.identity, f.connections, policies, faults, merges)
	config := gitops.Configuration{BuildWorkflowPath: ".gitea/workflows/checks.yml", BuildWorkflowSHA256: hex.EncodeToString(workflowDigest[:]), Environment: "production", SourceRepositoryID: sourceID, DeliveryRepositoryID: deliveryID, Enabled: true, TargetBranch: "main", ManifestPath: "deploy.yaml", Pointer: "/image", ImageRepository: "registry.example/app", ProvenancePublicKey: base64.StdEncoding.EncodeToString(provenance), HealthPublicKey: base64.StdEncoding.EncodeToString(health), HealthChecks: []string{"smoke"}, ObservationSeconds: 1, MaxEvidenceAgeSeconds: 300, DeadlineSeconds: 600, RecoveryAllowed: true}
	stored, err := service.PutConfiguration(ctx, f.owner, f.org, config, 0, "gitops-live")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != 1 || stored.SourceRepositoryID != sourceID || stored.DeliveryRepositoryID != deliveryID {
		t.Fatalf("stored config: %+v", stored)
	}

	sign := func(value any, key ed25519.PrivateKey) string {
		raw, _ := json.Marshal(value)
		return base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw))
	}
	digest, validation := buildLiveCheckout(t, []byte(liveCheckoutApplication), sourcePR.MergeSHA)
	artifacts := map[string][]byte{digest: []byte(liveCheckoutApplication)}
	var application *liveCheckoutProcess
	reconcile := func(p gitops.Promotion) bool {
		t.Helper()
		var file struct{ Content string }
		native(delivery, "bot", "GET", "/contents/deploy.yaml?ref="+p.MergeSHA, nil, &file)
		manifest, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			t.Fatal("read actual merged delivery manifest", err)
		}
		image, err := gitops.ReadManifestField(manifest, "/image")
		if err != nil {
			t.Fatal("read merged image reference", err)
		}
		artifact := strings.TrimPrefix(image, "registry.example/app@")
		content, exists := artifacts[artifact]
		if !exists {
			t.Fatal("reconciler cannot find immutable application artifact")
		}
		sum := sha256.Sum256(content)
		if artifact != "sha256:"+hex.EncodeToString(sum[:]) {
			t.Fatal("reconciler artifact digest mismatch")
		}
		if application != nil {
			application.stop()
		}
		application = startLiveCheckout(t, content)
		return application.smoke()
	}
	proof := deployment.Provenance{Kind: "gitops", Environment: "production", ConfigurationVersion: stored.Version, WorkflowSHA256: stored.BuildWorkflowSHA256, Recovery: &deployment.RecoverySafety{State: "stateless"}, OrgID: f.org, RepositoryID: sourceID, SourceSHA: sourcePR.MergeSHA, ArtifactDigest: digest, Validation: validation, BuildID: "disposable-build", IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour)}
	input := gitops.PreviewRequest{ChangeID: fmt.Sprint(sourcePR.Number), SourceSHA: sourcePR.MergeSHA, ArtifactDigest: digest, Provenance: deployment.SignedProvenance{Document: proof, Signature: sign(proof, provenanceKey)}}
	releases := &release.Service{DB: f.db, Auth: f.identity}
	if _, err = releases.Receive(ctx, f.org, sourceID, input.Provenance); err != nil {
		t.Fatal("actual signed CI reporter rejected", err)
	}
	deliveryCommands := &deliverycmd.Service{Releases: releases, GitOps: service, Providers: reader}
	connected, err := deliveryCommands.Discover(ctx, f.owner, f.org, sourceID)
	if err != nil || len(connected.Paths) != 1 || !connected.Paths[0].Ready {
		t.Fatalf("actual CI and protected delivery setup not connected: %+v %v", connected, err)
	}

	gate, err := service.Preview(ctx, f.owner, f.org, "production", input, "gitops-live")
	if err != nil || gate.Decision.Outcome != "allow" {
		t.Fatalf("preview: %v %+v", err, gate.Decision)
	}
	if gate.Before != "registry.example/app@sha256:"+strings.Repeat("0", 64) || gate.After != "registry.example/app@"+digest {
		t.Fatal("incorrect deterministic manifest change")
	}
	key := domain.NewID()
	type requested struct {
		p   gitops.Promotion
		err error
	}
	result := make(chan requested, 1)
	go func() {
		p, e := service.Request(ctx, f.owner, f.org, gate.ID, key, "gitops-live")
		result <- requested{p, e}
	}()
	select {
	case <-faults.stageEntered:
	case <-ctx.Done():
		t.Fatal("concurrent stage did not start")
	}
	if _, err = service.Continue(ctx, f.owner, f.org, gate.OperationID, "gitops-concurrent"); err != nil {
		t.Fatal(err)
	}
	close(faults.duplicateFinished)
	first := <-result
	if first.err != nil {
		t.Fatal(first.err)
	}
	detail, err := service.Get(ctx, f.owner, f.org, gate.OperationID)
	promotion := detail.Promotion
	if err != nil || promotion.State != "stage_uncertain" {
		t.Fatalf("lost staging response: %v %+v", err, promotion)
	}
	service = gitops.New(f.db, f.identity, f.connections, policies, faults, merges)
	for attempt := 0; attempt < 3; attempt++ {
		promotion, err = service.Observe(ctx, f.org, promotion.ID)
		if !errors.Is(err, privateconnector.ErrUnavailable) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil || promotion.State != "staged" {
		t.Fatalf("stage recovery: %v %+v", err, promotion)
	}
	promotion, err = service.Continue(ctx, f.owner, f.org, promotion.ID, "gitops-live")
	if err != nil || promotion.State != "publish_uncertain" {
		t.Fatalf("lost publication response: %v %+v", err, promotion)
	}
	service = gitops.New(f.db, f.identity, f.connections, policies, faults, merges)
	promotion, err = service.Observe(ctx, f.org, promotion.ID)
	if err != nil || promotion.State != "awaiting_merge" || promotion.Change == nil {
		t.Fatalf("publication recovery: %v %+v", err, promotion)
	}
	if faults.writes != 2 {
		t.Fatalf("ambiguous writes repeated: %d", faults.writes)
	}
	replay, err := service.Request(ctx, f.owner, f.org, gate.ID, key, "gitops-live")
	if err != nil || replay.ID != promotion.ID {
		t.Fatal("publication idempotency lost")
	}
	blocked, err := service.MergePreview(ctx, f.owner, f.org, promotion.ID, "fast-forward-only", "gitops-live")
	if err != nil {
		t.Fatalf("blocked merge preview: %v", err)
	}
	if blocked.Decision.Outcome == "allow" {
		t.Fatal("missing native approval allowed")
	}
	if _, err = service.RequestMerge(ctx, f.owner, f.org, promotion.ID, blocked.ID, domain.NewID(), "gitops-live"); err == nil {
		t.Fatal("blocked native merge allowed")
	}
	native(delivery, "bot", "POST", "/pulls/"+promotion.Change.ID+"/reviews", map[string]string{"commit_id": promotion.CandidateSHA, "event": "APPROVED", "body": "Disposable protected GitOps approval"}, nil)
	mergeGate, err := service.MergePreview(ctx, f.owner, f.org, promotion.ID, "fast-forward-only", "gitops-live")
	if err != nil || mergeGate.Decision.Outcome != "allow" {
		t.Fatalf("approved merge preview: %v %+v native=%+v", err, mergeGate.Decision, mergeGate.Snapshot.Native)
	}

	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO member_repositories(org_id,user_id,repository_id) VALUES($1,$2,$3)`, f.org, f.owner.User.ID, deliveryID)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = merges.Request(ctx, f.owner, f.org, mergeGate.ID, domain.NewID(), "source-revoked"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("direct merge bypassed source revocation: %v", err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE memberships SET all_repositories=true WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	merged, err := service.RequestMerge(ctx, f.owner, f.org, promotion.ID, mergeGate.ID, domain.NewID(), "gitops-live")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20 && merged.State != "merged"; i++ {
		time.Sleep(100 * time.Millisecond)
		merged, err = merges.Observe(ctx, f.org, merged.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if merged.State != "merged" {
		t.Fatalf("native merge incomplete: %+v", merged)
	}
	service = gitops.New(f.db, f.identity, f.connections, policies, reader, merges)
	promotion, err = service.Observe(ctx, f.org, promotion.ID)
	if err != nil || promotion.State != "completed_unverified" {
		t.Fatalf("restart observation: %v %+v", err, promotion)
	}
	if !reconcile(promotion) {
		t.Fatal("actual checkout application is unhealthy after protected rollout")
	}
	h := gitops.HealthReport{OrgID: f.org, PromotionID: promotion.ID, Environment: "production", ConfigurationVersion: stored.Version, SourceSHA: input.SourceSHA, ArtifactDigest: digest, DeliveryRevision: promotion.MergeSHA, Healthy: true, Checks: map[string]bool{"smoke": true}, ObservedAt: time.Now(), Nonce: domain.NewID()}
	bad := h
	bad.DeliveryRevision = strings.Repeat("f", 40)
	if err = service.ReceiveHealth(ctx, f.org, promotion.ID, bad, sign(bad, healthKey)); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("wrong reconciler revision accepted: %v", err)
	}
	if err = service.ReceiveHealth(ctx, f.org, promotion.ID, h, sign(h, healthKey)); err != nil {
		t.Fatal(err)
	}
	promotion, err = service.Observe(ctx, f.org, promotion.ID)
	if err != nil || promotion.State != "verifying" {
		t.Fatalf("unelapsed window: %v %s", err, promotion.State)
	}
	time.Sleep(1100 * time.Millisecond)
	h.ObservedAt = time.Now()
	h.Nonce = domain.NewID()
	if err = service.ReceiveHealth(ctx, f.org, promotion.ID, h, sign(h, healthKey)); err != nil {
		t.Fatal(err)
	}
	promotion, err = service.Observe(ctx, f.org, promotion.ID)
	if err != nil || promotion.State != "healthy" {
		t.Fatalf("attributed health: %v %+v", err, promotion)
	}

	known := promotion
	completePromotion := func(in gitops.PreviewRequest, p gitops.Promotion, healthy bool) gitops.Promotion {
		t.Helper()
		var e error
		native(delivery, "bot", "POST", "/pulls/"+p.Change.ID+"/reviews", map[string]string{"commit_id": p.CandidateSHA, "event": "APPROVED", "body": "Disposable recovery approval"}, nil)
		mg, e := service.MergePreview(ctx, f.owner, f.org, p.ID, "fast-forward-only", "gitops-recovery")
		if e != nil || mg.Decision.Outcome != "allow" {
			t.Fatalf("recovery merge preview: %v %+v", e, mg.Decision)
		}
		op, e := service.RequestMerge(ctx, f.owner, f.org, p.ID, mg.ID, domain.NewID(), "gitops-recovery")
		if e != nil {
			t.Fatal(e)
		}
		for i := 0; i < 20 && op.State != "merged"; i++ {
			time.Sleep(100 * time.Millisecond)
			op, e = merges.Observe(ctx, f.org, op.ID)
			if e != nil {
				t.Fatal(e)
			}
		}
		if op.State != "merged" {
			t.Fatalf("recovery native merge: %+v", op)
		}
		p, e = service.Observe(ctx, f.org, p.ID)
		if e != nil || p.State != "completed_unverified" {
			t.Fatalf("recovery observe: %v %+v", e, p)
		}
		if !reconcile(p) {
			t.Fatal("actual application failed before runtime fault injection")
		}
		if !healthy {
			if e = os.WriteFile(filepath.Join(application.root, "outage"), []byte("runtime outage"), 0600); e != nil {
				t.Fatal(e)
			}
		}
		observed := application.smoke()
		if observed != healthy {
			t.Fatal("actual checkout outcome disagrees with expected fault or recovery")
		}
		h := gitops.HealthReport{OrgID: f.org, PromotionID: p.ID, Environment: "production", ConfigurationVersion: stored.Version, SourceSHA: in.SourceSHA, ArtifactDigest: in.ArtifactDigest, DeliveryRevision: p.MergeSHA, Healthy: observed, Checks: map[string]bool{"smoke": observed}, ObservedAt: time.Now(), Nonce: domain.NewID()}
		if e = service.ReceiveHealth(ctx, f.org, p.ID, h, sign(h, healthKey)); e != nil {
			t.Fatal(e)
		}
		if healthy {
			time.Sleep(1100 * time.Millisecond)
			h.ObservedAt = time.Now()
			h.Nonce = domain.NewID()
			if e = service.ReceiveHealth(ctx, f.org, p.ID, h, sign(h, healthKey)); e != nil {
				t.Fatal(e)
			}
		}
		p, e = service.Observe(ctx, f.org, p.ID)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	next := input
	native(source, "reviewer", "POST", "/branches", map[string]string{"new_branch_name": "release-two", "old_ref_name": "main"}, nil)
	var currentFile struct{ SHA string }
	native(source, "bot", "GET", "/contents/app.cjs?ref=main", nil, &currentFile)
	nextApp := []byte(strings.Replace(liveCheckoutApplication, "const version = 'one'", "const version = 'two'", 1))
	native(source, "reviewer", "PUT", "/contents/app.cjs", map[string]string{"branch": "release-two", "sha": currentFile.SHA, "message": "Release second checkout version", "content": base64.StdEncoding.EncodeToString(nextApp)}, nil)
	var nextPR struct {
		Number   int64
		Head     struct{ SHA string }
		MergeSHA string `json:"merge_commit_sha"`
	}
	native(source, "reviewer", "POST", "/pulls", map[string]string{"head": "release-two", "base": "main", "title": "Second checkout release"}, &nextPR)
	var releaseBranch struct{ Commit struct{ ID string } }
	native(source, "bot", "GET", "/branches/release-two", nil, &releaseBranch)
	nextPR.Head.SHA = releaseBranch.Commit.ID
	_, sourceValidation := buildLiveCheckout(t, nextApp, nextPR.Head.SHA)
	var status struct{ Creator struct{ ID int64 } }
	native(source, "admin", "POST", "/statuses/"+nextPR.Head.SHA, map[string]string{"state": "success", "context": "ci/checkout", "description": "Actual build and three named checkout cases passed"}, &status)
	if status.Creator.ID < 1 {
		t.Fatal("actual CI publisher identity missing")
	}
	for i := 0; i < 60; i++ {
		var state struct{ Mergeable bool }
		native(source, "bot", "GET", fmt.Sprintf("/pulls/%d", nextPR.Number), nil, &state)
		if state.Mergeable {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	var approval struct {
		State    string
		CommitID string `json:"commit_id"`
	}
	native(source, "bot", "POST", fmt.Sprintf("/pulls/%d/reviews", nextPR.Number), map[string]string{"commit_id": nextPR.Head.SHA, "event": "APPROVED", "body": "Approve disposable checkout release"}, &approval)
	if approval.State != "APPROVED" || approval.CommitID != nextPR.Head.SHA {
		t.Fatalf("exact source review not recorded: %+v head=%s", approval, nextPR.Head.SHA)
	}
	merges.RegisterChangeAuthority(func(ctx context.Context, tx pgx.Tx, org, repo string, snapshot forge.MergeEvidence, actor domain.Actor) (string, error) {
		if org == f.org && repo == sourceID && snapshot.Change.HeadSHA == nextPR.Head.SHA && snapshot.Change.TargetSHA == input.SourceSHA && sourceValidation != nil {
			return "actual-checkout-ci:" + nextPR.Head.SHA, nil
		}
		return service.CheckMergeTx(ctx, tx, org, repo, snapshot, actor)
	})
	time.Sleep(500 * time.Millisecond)
	sourceGate, err := merges.Inspect(ctx, f.owner, f.org, sourceID, fmt.Sprint(nextPR.Number), "fast-forward-only", "source-release")
	for i := 0; i < 40 && err == nil && sourceGate.Decision.Outcome != "allow" && len(sourceGate.Snapshot.Native.Blockers) == 1 && sourceGate.Snapshot.Native.Blockers[0] == "Pull request is not open, ready and mergeable" && sourceGate.Snapshot.Change.State == "open" && sourceGate.Snapshot.Change.HeadSHA == nextPR.Head.SHA && sourceGate.Snapshot.Change.TargetSHA == input.SourceSHA; i++ {
		time.Sleep(200 * time.Millisecond)
		sourceGate, err = merges.Inspect(ctx, f.owner, f.org, sourceID, fmt.Sprint(nextPR.Number), "fast-forward-only", "source-release")
	}
	if err != nil || sourceGate.Decision.Outcome != "allow" {
		var reviews []struct {
			ID                         int64
			Official, Stale, Dismissed bool
			CommitID                   string `json:"commit_id"`
		}
		native(source, "bot", "GET", fmt.Sprintf("/pulls/%d/reviews", nextPR.Number), nil, &reviews)
		t.Logf("source native review flags: %+v", reviews)
		t.Fatalf("protected source preview: blockers=%v native=%+v reviews=%+v err=%v", sourceGate.Decision.Blockers, sourceGate.Snapshot.Native, sourceGate.Snapshot.Approvals, err)
	}
	sourceMerge, err := merges.Request(ctx, f.owner, f.org, sourceGate.ID, domain.NewID(), "source-release")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20 && sourceMerge.State != "merged"; i++ {
		time.Sleep(100 * time.Millisecond)
		sourceMerge, err = merges.Observe(ctx, f.org, sourceMerge.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if sourceMerge.State != "merged" {
		t.Fatalf("protected source merge incomplete: %+v", sourceMerge)
	}
	native(source, "bot", "GET", fmt.Sprintf("/pulls/%d", nextPR.Number), nil, &nextPR)
	var refreshID string
	if err = f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if err := inventory.RequestRefreshTx(ctx, tx, f.org, sourceID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id::text FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state='queued'`, f.org, sourceID).Scan(&refreshID)
	}); err != nil {
		t.Fatal(err)
	}
	drainLiveInventory(t, ctx, portfolio, f, refreshID)
	nextDigest, nextValidation := buildLiveCheckout(t, nextApp, nextPR.MergeSHA)
	artifacts[nextDigest] = nextApp
	next.ChangeID, next.SourceSHA, next.ArtifactDigest = fmt.Sprint(nextPR.Number), nextPR.MergeSHA, nextDigest
	next.Provenance.Document.SourceSHA = next.SourceSHA
	next.Provenance.Document.BuildID = "disposable-build-two"
	next.Provenance.Document.IssuedAt = time.Now()
	next.Provenance.Document.Validation = nextValidation
	next.Provenance.Document.ArtifactDigest = next.ArtifactDigest
	next.Provenance.Signature = sign(next.Provenance.Document, provenanceKey)
	if _, err = releases.Receive(ctx, f.org, sourceID, next.Provenance); err != nil {
		t.Fatal(err)
	}
	budgets := budget.New(f.db, f.identity, nil, nil)
	repairs := repair.New(f.db, f.identity, discoveries, jobs, runners, policies, budgets, f.connections, nil, reader, nil)
	newAutopilot := func() *autopilot.Service {
		auto := autopilot.New(f.db, f.identity, repairs, merges, budgets, policies)
		auto.Delivery = &deliverycmd.Service{Releases: releases, GitOps: service, Providers: reader}
		auto.Promotions = service
		return auto
	}
	auto := newAutopilot()
	if _, err = auto.Put(ctx, f.owner, f.org, true, 0, "autopilot-release"); err != nil {
		t.Fatal(err)
	}
	if err = auto.Step(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	promotions, err := service.List(ctx, f.owner, f.org, "production", "", 100)
	if err != nil || len(promotions.Items) != 2 {
		t.Fatalf("merged receipt did not create one delivery: %+v %v", promotions, err)
	}
	var dispatched gitops.Promotion
	for _, item := range promotions.Items {
		if item.ID != known.ID {
			dispatched = item
		}
	}
	dispatchDetails, err := service.Get(ctx, f.owner, f.org, dispatched.ID)
	if err != nil || dispatched.Change == nil || dispatchDetails.Gate.Request.SourceSHA != next.SourceSHA || dispatchDetails.Gate.Request.ArtifactDigest != next.ArtifactDigest {
		t.Fatal("automatic delivery lost merged source or artifact attribution", err)
	}
	var beforeRestart []json.RawMessage
	native(delivery, "bot", "GET", "/pulls?state=all", nil, &beforeRestart)
	if err = newAutopilot().Step(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	promotions, err = service.List(ctx, f.owner, f.org, "production", "", 100)
	var afterRestart []json.RawMessage
	native(delivery, "bot", "GET", "/pulls?state=all", nil, &afterRestart)
	if err != nil || len(promotions.Items) != 2 || len(beforeRestart) != len(afterRestart) {
		t.Fatal("controller restart repeated delivery operation or protected pull request", err)
	}
	if restarted, err := service.Get(ctx, f.owner, f.org, dispatched.ID); err != nil || restarted.Promotion.Change == nil || restarted.Promotion.Change.ID != dispatched.Change.ID {
		t.Fatal("controller restart lost protected delivery attribution", err)
	}
	failed := completePromotion(next, dispatched, true)
	if err = os.WriteFile(filepath.Join(application.root, "outage"), []byte("post-rollout checkout outage"), 0600); err != nil {
		t.Fatal(err)
	}
	late := gitops.HealthReport{OrgID: f.org, PromotionID: failed.ID, Environment: "production", ConfigurationVersion: stored.Version, SourceSHA: next.SourceSHA, ArtifactDigest: next.ArtifactDigest, DeliveryRevision: failed.MergeSHA, Healthy: application.smoke(), Checks: map[string]bool{"smoke": application.smoke()}, ObservedAt: time.Now(), Nonce: domain.NewID()}
	if late.Healthy || err != nil {
		t.Fatal("runtime fault did not break actual checkout")
	}
	if err = service.ReceiveHealth(ctx, f.org, failed.ID, late, sign(late, healthKey)); err != nil {
		t.Fatal(err)
	}
	lateDetail, err := service.Get(ctx, f.owner, f.org, failed.ID)
	if err != nil || lateDetail.Promotion.State != "healthy" || lateDetail.Health == nil || lateDetail.Health.Healthy {
		t.Fatalf("late runtime incident lost historical rollout or current failure: %+v %v", lateDetail, err)
	}
	var unhealthy bool
	if err = f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		var e error
		unhealthy, e = environmenthealth.UnhealthyTx(ctx, tx, f.org, "gitops", failed.ID)
		return e
	}); err != nil || !unhealthy {
		t.Fatal("current environment lost real unhealthy observation", err)
	}
	failed = lateDetail.Promotion
	restore := input
	restore.RecoveryOf, restore.RestorePromotionID = failed.ID, known.ID
	commands := &deliverycmd.Service{Releases: &release.Service{DB: f.db, Auth: f.identity}, GitOps: service}
	plan, err := commands.PreviewRestore(ctx, f.owner, f.org, "gitops", failed.ID, failed.Version, "owner-restore")
	if err != nil || plan.PreviousAt.IsZero() || len(plan.Limits) == 0 || len(plan.Blockers) != 0 {
		t.Fatalf("owner recovery plan: %+v %v", plan, err)
	}
	restoredRelease, err := commands.ConfirmRestore(ctx, f.owner, f.org, "gitops", failed.ID, plan.GateID, failed.Version, "owner-restore")
	if err != nil {
		t.Fatal(err)
	}
	pendingRestore, err := service.Get(ctx, f.owner, f.org, restoredRelease.ID)
	if err != nil {
		t.Fatal(err)
	}
	recovered := completePromotion(restore, pendingRestore.Promotion, true)
	if recovered.ID == known.ID || recovered.ID == failed.ID || recovered.RecoveryOf != failed.ID || recovered.State != "recovered" {
		t.Fatalf("separate known-good revert missing: %+v", recovered)
	}
	retained, e := service.Get(ctx, f.owner, f.org, failed.ID)
	if e != nil || retained.Promotion.State != "healthy" {
		t.Fatal("recovery erased failed history")
	}
	t.Logf("real protected GitOps promotion=%s source=%s delivery=%s", promotion.ID, input.SourceSHA, promotion.MergeSHA)
	t.Logf("real Gitea GitOps source=%s delivery=%s config_version=%d", source.FullName, delivery.FullName, stored.Version)
}

func drainLiveInventory(t *testing.T, ctx context.Context, service *inventory.Service, f *inventoryFixture, id string) inventory.Job {
	t.Helper()
	for i := 0; i < 100; i++ {
		job, err := service.Job(ctx, f.owner, f.org, id)
		if err != nil {
			t.Fatal(err)
		}
		if job.State == "complete" {
			return job
		}
		var lease *inventory.Job
		claimContext, cancel := context.WithTimeout(ctx, 5*time.Second)
		for lease == nil && claimContext.Err() == nil {
			lease, err = service.Claim(claimContext, f.org, "gitops-live")
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			if lease == nil {
				time.Sleep(20 * time.Millisecond)
			}
		}
		cancel()
		if lease == nil {
			t.Fatal("live inventory claim timed out")
		}
		if err = service.Step(ctx, *lease); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("live inventory did not converge")
	return inventory.Job{}
}

func ptrLive(value int64) *int64 { return &value }

func readGiteaToken(t *testing.T, root, actor string) string {
	raw, err := os.ReadFile(filepath.Join(root, ".local/gitea/reforge-"+actor+".token"))
	if err != nil {
		t.Fatal("local Gitea token unavailable")
	}
	defer clear(raw)
	return strings.TrimSpace(string(raw))
}

type gitopsLostResponses struct {
	providers.Client
	mu                              sync.Mutex
	stageEntered, stageRelease      chan struct{}
	firstClaimed, duplicateFinished chan struct{}
	entered                         int
	stage, publish                  bool
	writes                          int
}

func (f *gitopsLostResponses) Write(ctx context.Context, org, id string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) (string, error), check func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	first := false
	if op.Kind == privateconnector.ForgeUpdateBranch && f.stageEntered != nil {
		f.mu.Lock()
		f.entered++
		first = f.entered == 1
		if f.entered == 1 {
			close(f.stageEntered)
		}
		if f.entered == 2 {
			close(f.stageRelease)
		}
		f.mu.Unlock()
		select {
		case <-f.stageRelease:
		case <-ctx.Done():
			return privateconnector.Result{}, ctx.Err()
		}
	}
	if op.Kind == privateconnector.ForgeUpdateBranch && f.stageEntered != nil {
		if first {
			original := authorize
			authorize = func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
				id, e := original(ctx, tx, c)
				close(f.firstClaimed)
				return id, e
			}
		} else {
			select {
			case <-f.firstClaimed:
			case <-ctx.Done():
				return privateconnector.Result{}, ctx.Err()
			}
		}
	}
	result, err := f.Client.Write(ctx, org, id, op, authorize, check)
	if first && err == nil {
		select {
		case <-f.duplicateFinished:
		case <-ctx.Done():
			return privateconnector.Result{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		f.writes++
	}
	if err == nil && op.Kind == privateconnector.ForgeUpdateBranch && f.stage {
		f.stage = false
		return privateconnector.Result{}, errors.New("injected lost stage response")
	}
	if err == nil && op.Kind == privateconnector.ForgeCreateChange && f.publish {
		f.publish = false
		return privateconnector.Result{}, errors.New("injected lost publication response")
	}
	return result, err
}
