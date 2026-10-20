package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/artifact"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/control"
	"github.com/reforgeapp/reforge/pkg/customcmd"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/runner"
	"github.com/reforgeapp/reforge/pkg/runnerclient"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func TestCustomProfilePostgresGVisorWorkflow(t *testing.T) {
	if os.Getenv("REFORGE_CUSTOM_PROFILE_E2E_TEST") != "1" {
		t.Skip("set REFORGE_CUSTOM_PROFILE_E2E_TEST=1 for PostgreSQL and gVisor custom-profile acceptance")
	}
	if runtime.GOOS != "linux" {
		t.Skip("gVisor fixture requires Linux")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	runsc, tool := "/tmp/reforge-gvisor/bin/runsc", filepath.Join(root, "bin/reforge-sandbox-tool")
	for _, path := range []string{runsc, tool} {
		if _, err = os.Stat(path); err != nil {
			t.Fatalf("required local sandbox asset %s: %v", path, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	imageRoot := filepath.Join(t.TempDir(), "javascript")
	if output, buildErr := exec.CommandContext(ctx, "python3", filepath.Join(root, "scripts/build-runner-images.py"), "--stack", "javascript", "--output", imageRoot).CombinedOutput(); buildErr != nil {
		t.Fatalf("build local JavaScript image: %v: %s", buildErr, output)
	}
	image, err := sandbox.ImageDigest(imageRoot)
	if err != nil {
		t.Fatal(err)
	}

	f := newDiscoveryFixture(t)
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	authority := control.NewAuthority(policies)
	jobs := workflow.New(f.db, f.identity, authority.Check)
	artifactRoot := t.TempDir()
	if err = os.Chmod(artifactRoot, 0700); err != nil {
		t.Fatal(err)
	}
	artifacts, err := artifact.NewLocal(f.db, artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifacts.Close() })
	runners := runner.New(f.db, f.identity, jobs, artifacts)
	jobs.RegisterScopeCheck(runners.CheckScopeTx)
	f.connections.RegisterRunnerCheck(runners.CheckRunnerTx)
	pool, err := runners.PutPool(ctx, f.owner, f.org, "", runner.PoolInput{Name: "Custom integration", RepositoryIDs: []string{f.repo}}, 0, "custom-e2e")
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := runners.EnrollToken(ctx, f.owner, f.org, pool.ID, "custom-e2e")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := runners.Enroll(ctx, enrollment.Token, "custom-e2e")
	if err != nil {
		t.Fatal(err)
	}

	patched := "exports.value = function() { return 2 }"
	profileInput := customcmd.Profile{Name: "gVisor integration", ImageDigest: image, Executable: "/usr/local/bin/node", Argv: []string{"-e", "require('fs').writeFileSync('value.js', " + strconvQuote(patched) + "), console.log(JSON.stringify({type:'result',data:{outcome:'success'}}))"}, ProtocolVersion: customcmd.ProtocolVersion, MaxWallSeconds: 30, MaxOutputBytes: 1 << 20, MaxTurns: 2, Concurrency: 1}
	if err = customcmd.Validate(profileInput); err != nil {
		t.Fatalf("profile input rejected: %v; image=%q executable=%q argv=%q", err, image, profileInput.Executable, profileInput.Argv)
	}
	profiles := customcmd.New(f.db, f.identity)
	created, err := profiles.Create(ctx, f.owner, f.org, profileInput, "custom-e2e")
	if err != nil {
		t.Fatal(err)
	}
	approved, err := profiles.Approve(ctx, f.owner, f.org, created.ID, "local PostgreSQL and gVisor acceptance", created.Version, "custom-e2e")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "agent", Provider: "custom_command", Name: "gVisor integration", Endpoint: "https://runtime.example", Settings: connections.Settings{AuthKind: "official_runtime", BillingRoute: "subscription", Model: "integration-fixture"}}, "custom-e2e")
	if err != nil {
		t.Fatal(err)
	}
	budgets := budget.New(f.db, f.identity, func(ctx context.Context, tx pgx.Tx, lease budget.Lease) error {
		_, fenceErr := jobs.ValidateFenceTx(ctx, tx, workflow.Lease(lease), "budget")
		return fenceErr
	}, nil)
	route, err := budgets.PutRoute(ctx, f.owner, f.org, budget.Route{ConnectionID: agent.ID, Model: "integration-fixture", Name: "default", Mode: "quota", PricingVersion: "fixture", MaxInputTokens: 10000, MaxOutputTokens: 4096, MaxMilliseconds: 60000, MaxRequests: 1}, 0, "custom-e2e")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, qualifyErr := budgets.QualifyRouteTx(ctx, tx, f.org, agent.ID, "integration-fixture", "default", route.Version, "local integration fixture")
		return qualifyErr
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = budgets.PutLimit(ctx, f.owner, f.org, budget.Limit{Scope: budget.Scope{Kind: "organisation", ID: f.org}, Period: "daily", Caps: budget.Caps{Tokens: i64(100000), Milliseconds: i64(3600000), Requests: i64(20), Concurrency: i64(2)}}, 0, "custom-e2e"); err != nil {
		t.Fatal(err)
	}
	document := policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{Recipes: []string{"javascript"}, Models: []string{"integration-fixture"}, Routes: []string{agent.ID + "/default"}}}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, document, "custom", "custom-e2e")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Repair})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, sim.Hash, "custom", "custom-e2e"); err != nil {
		t.Fatal(err)
	}
	baseSHA, targetSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
	files := map[string][]byte{"value.js": []byte("exports.value = function() { return 1 }"), "value.test.js": []byte("const test = require('node:test'); const assert = require('node:assert/strict'); const { value } = require('./value.js'); test('value', () => assert.equal(value(), 2));")}
	reader := &repairContractReader{fixture: f, files: files}
	repairs := repair.New(f.db, f.identity, f.service, jobs, runners, policies, budgets, f.connections, profiles, reader, map[string]string{"javascript": image})
	for _, action := range []string{"repair.source", "repair.report", "repair.custom"} {
		authority.Register(action, func(context.Context, pgx.Tx, workflow.Task, policy.Resolved) error { return nil })
	}
	finding := f.observe(t, f.observation("custom-e2e", "main", baseSHA, nil))
	input := repair.Input{FindingID: finding.ID, FindingVersion: finding.Version, Recipe: "javascript", ModelConnectionID: agent.ID, ModelRoute: "default", RunnerPoolID: pool.ID, CustomProfileID: approved.ID, CustomProfileVersion: approved.Version, IdempotencyKey: domain.NewID()}
	preview, err := repairs.Preview(ctx, f.owner, f.org, input)
	if err != nil || len(preview.Blockers) != 0 {
		t.Fatalf("preview: %v blockers=%v", err, preview.Blockers)
	}
	input.PlanDigest = preview.Context.Plan.Digest
	if _, err = repairs.Enqueue(ctx, f.owner, f.org, input, "custom-e2e"); err != nil {
		t.Fatal(err)
	}
	claimed, err := runners.Claim(ctx, supervisor.Token)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []domain.TaskState{domain.TaskPlanning, domain.TaskRepairing} {
		if _, err = runners.Progress(ctx, claimed.Credential.Token, state); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := customcmd.NewDispatcher(f.db, runners, profiles, budgets)
	baseline := lifecycleSnapshot(t, baseSHA, files)
	target := lifecycleSnapshot(t, targetSHA, files)
	plan := preview.Context.Plan
	run := repair.Run{Task: claimed.Task, Context: preview.Context}
	candidateSHA := strings.Repeat("c", 40)
	candidate := lifecycleSnapshot(t, candidateSHA, map[string][]byte{"value.js": []byte(patched), "value.test.js": files["value.test.js"]})
	snapshots := map[string]sandbox.Snapshot{baseSHA: baseline, targetSHA: target, candidateSHA: candidate}
	var report repair.Report
	var publish repair.Publication
	stageCount, publishCount := 0, 0
	mux := http.NewServeMux()
	mux.HandleFunc("/runner/v1/repair/run", func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(run) })
	mux.HandleFunc("/runner/v1/repair/source/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(snapshots[strings.TrimPrefix(r.URL.Path, "/runner/v1/repair/source/")])
	})
	mux.HandleFunc("/runner/v1/progress", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			State domain.TaskState `json:"state"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		run.Task.State = in.State
		_ = json.NewEncoder(w).Encode(run.Task)
	})
	mux.HandleFunc("/runner/v1/repair/custom/authorize", func(w http.ResponseWriter, r *http.Request) {
		authorized, authErr := dispatcher.Authorize(r.Context(), claimed.Credential.Token)
		if authErr != nil {
			http.Error(w, authErr.Error(), http.StatusConflict)
			return
		}
		_ = json.NewEncoder(w).Encode(authorized)
	})
	mux.HandleFunc("/runner/v1/repair/custom/report", func(w http.ResponseWriter, r *http.Request) {
		var in customcmd.ReportInput
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "invalid report", http.StatusBadRequest)
			return
		}
		if _, reportErr := dispatcher.Report(r.Context(), claimed.Credential.Token, in); reportErr != nil {
			http.Error(w, reportErr.Error(), http.StatusConflict)
			return
		}
		_ = json.NewEncoder(w).Encode(customcmd.Authorized{})
	})
	mux.HandleFunc("/runner/v1/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "fixture-artifact"})
	})
	mux.HandleFunc("/runner/v1/repair/report", func(w http.ResponseWriter, r *http.Request) {
		if json.NewDecoder(r.Body).Decode(&report) != nil {
			http.Error(w, "invalid repair report", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(struct{}{})
	})
	mux.HandleFunc("/runner/v1/repair/stage", func(w http.ResponseWriter, _ *http.Request) {
		if report.State != "validated" {
			http.Error(w, "candidate not validated", http.StatusConflict)
			return
		}
		stageCount++
		run.CandidateSHA = candidateSHA
		_ = json.NewEncoder(w).Encode(run)
	})
	mux.HandleFunc("/runner/v1/repair/native-checks", func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(run) })
	mux.HandleFunc("/runner/v1/repair/publish", func(w http.ResponseWriter, r *http.Request) {
		if json.NewDecoder(r.Body).Decode(&publish) != nil || report.State != "validated" || stageCount != 1 || publish.HeadSHA != run.CandidateSHA || publish.PlanDigest != plan.Digest {
			http.Error(w, "publication evidence mismatch", http.StatusConflict)
			return
		}
		for _, check := range publish.Checks {
			if !check.Complete || check.ExitCode != 0 {
				http.Error(w, "native checks incomplete", http.StatusConflict)
				return
			}
		}
		publishCount++
		_ = json.NewEncoder(w).Encode(run)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := runnerclient.New(runnerclient.Config{Endpoint: server.URL, Development: true, Name: "custom-profile-e2e", CredentialFile: filepath.Join(t.TempDir(), "credential")})
	if err != nil {
		t.Fatal(err)
	}
	job := runnerclient.Job{Token: "fixture-job-token", Lease: claimed.Lease, Task: claimed.Task}
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	cfg := sandbox.RuntimeConfig{Runsc: runsc, RunscSHA256: testFileDigest(t, runsc), Tool: tool, ToolSHA256: testFileDigest(t, tool), StateRoot: runtimeRoot, Images: map[string]string{image: imageRoot}, Development: true, Rootless: true, MemoryBytes: 1 << 30, DiskBytes: 256 << 20, CPUs: 1, MaxProcesses: 128, Fetch: func(_ context.Context, request sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		if snapshot, ok := snapshots[request.CommitSHA]; ok {
			return snapshot, nil
		}
		return sandbox.Snapshot{}, sandbox.ErrBoundary
	}}
	completion, err := runnerclient.RepairProcessor(cfg)(ctx, client, job)
	if err != nil {
		t.Fatalf("processor: %v report=%+v", err, report)
	}
	if completion.Outcome != "completed" || report.State != "validated" || len(report.Patches) != 1 || string(report.Patches[0].Content) != patched || len(report.Baseline) == 0 || len(report.Candidate) == 0 || len(report.Target) == 0 || stageCount != 1 || publishCount != 1 || len(publish.Checks) == 0 {
		t.Fatalf("incomplete processor lifecycle: completion=%+v report=%+v stage=%d publish=%d", completion, report, stageCount, publishCount)
	}
	var runState, usageRaw string
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state,usage::text FROM custom_profile_runs WHERE org_id=$1 AND task_id=$2`, f.org, claimed.Task.ID).Scan(&runState, &usageRaw)
	}); err != nil {
		t.Fatal(err)
	}
	if runState != "completed_unverified" {
		t.Fatalf("persisted custom execution state=%q", runState)
	}
	var usage customcmd.Usage
	if json.Unmarshal([]byte(usageRaw), &usage) != nil || usage.Known {
		t.Fatalf("expected actual runtime to preserve unknown usage, got %s", usageRaw)
	}
	var reservationID string
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT reservation_id::text FROM custom_profile_runs WHERE org_id=$1 AND task_id=$2`, f.org, claimed.Task.ID).Scan(&reservationID)
	}); err != nil {
		t.Fatal(err)
	}
	var reservation budget.Reservation
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		var raw []byte
		if queryErr := tx.QueryRow(ctx, `SELECT record FROM budget_reservations WHERE org_id=$1 AND id=$2`, f.org, reservationID).Scan(&raw); queryErr != nil {
			return queryErr
		}
		return json.Unmarshal(raw, &reservation)
	}); err != nil {
		t.Fatal(err)
	}
	if reservation.State != "unknown" || reservation.Actual != nil {
		t.Fatalf("unknown usage reservation=%+v", reservation)
	}
	if _, err = profiles.Revoke(ctx, f.owner, f.org, approved.ID, approved.Version, "custom-e2e"); err != nil {
		t.Fatal(err)
	}
	if _, err = dispatcher.Authorize(ctx, claimed.Credential.Token); err == nil {
		t.Fatal("revoked profile dispatch unexpectedly succeeded")
	}
}

func strconvQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func testFileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestCustomProfileDispatcherTerminalReplay(t *testing.T) {
	fixture := newCustomProfileBudgetFixture(t)
	ctx := context.Background()
	input := customcmd.ReportInput{RunID: fixture.runID, State: "failed", Reason: "terminal replay acceptance", Usage: customcmd.Usage{Known: true, Tokens: 1, Milliseconds: 10, Requests: 1}}
	if _, err := fixture.dispatcher.Report(ctx, fixture.credential, input); err != nil {
		t.Fatal(err)
	}
	_, replayErr := fixture.dispatcher.Authorize(ctx, fixture.credential)
	var state string
	if err := fixture.fixture.db.Tenant(ctx, fixture.fixture.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state FROM custom_profile_runs WHERE org_id=$1 AND id=$2`, fixture.fixture.org, fixture.runID).Scan(&state)
	}); err != nil {
		t.Fatal(err)
	}
	if replayErr == nil || state != "failed" {
		t.Fatalf("terminal custom run replay: authorize_error=%v persisted_state=%q", replayErr, state)
	}
}

func TestCustomProfileConcurrentAuthorizeHonorsProfileLimit(t *testing.T) {
	base := newInventoryFixture(t, 2)
	base.importAll(t)
	repositoryPage, err := base.service.Repositories(context.Background(), base.owner, base.org, 10, "")
	if err != nil || len(repositoryPage.Items) != 2 {
		t.Fatalf("two repository fixture: %v count=%d", err, len(repositoryPage.Items))
	}
	repoIDs := []string{repositoryPage.Items[0].ID, repositoryPage.Items[1].ID}
	ctx := context.Background()
	if err = base.service.Maintain(ctx, base.org); err != nil {
		t.Fatal(err)
	}
	discoveryService := discovery.New(base.db, base.identity, nil)
	base.server.RegisterDiscovery(discoveryService)
	for attempt := 0; attempt < 8; attempt++ {
		progressed := false
		for _, worker := range []string{"discovery-fixture", "discovery-changes"} {
			lease, claimErr := base.service.Claim(ctx, base.org, worker)
			if claimErr != nil {
				t.Fatal(claimErr)
			}
			if lease != nil {
				if err = base.service.Step(ctx, *lease); err != nil {
					t.Fatal(err)
				}
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	f := &discoveryFixture{inventoryFixture: base, service: discoveryService, repo: repoIDs[0]}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	authority := control.NewAuthority(policies)
	jobs := workflow.New(f.db, f.identity, authority.Check)
	artifactRoot := t.TempDir()
	if err = os.Chmod(artifactRoot, 0700); err != nil {
		t.Fatal(err)
	}
	artifacts, err := artifact.NewLocal(f.db, artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifacts.Close() })
	runners := runner.New(f.db, f.identity, jobs, artifacts)
	jobs.RegisterScopeCheck(runners.CheckScopeTx)
	f.connections.RegisterRunnerCheck(runners.CheckRunnerTx)
	pool, err := runners.PutPool(ctx, f.owner, f.org, "", runner.PoolInput{Name: "Concurrent custom dispatch", RepositoryIDs: repoIDs}, 0, "custom-concurrency")
	if err != nil {
		t.Fatal(err)
	}
	var supervisorTokens []string
	for _, name := range []string{"custom-concurrency-a", "custom-concurrency-b"} {
		enrollment, enrollErr := runners.EnrollToken(ctx, f.owner, f.org, pool.ID, "custom-concurrency")
		if enrollErr != nil {
			t.Fatal(enrollErr)
		}
		supervisor, enrollErr := runners.Enroll(ctx, enrollment.Token, name)
		if enrollErr != nil {
			t.Fatal(enrollErr)
		}
		supervisorTokens = append(supervisorTokens, supervisor.Token)
	}
	profiles := customcmd.New(f.db, f.identity)
	created, err := profiles.Create(ctx, f.owner, f.org, customProfileInput(), "custom-concurrency")
	if err != nil {
		t.Fatal(err)
	}
	approved, err := profiles.Approve(ctx, f.owner, f.org, created.ID, "concurrent authorization cap", created.Version, "custom-concurrency")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "agent", Provider: "custom_command", Name: "Concurrency fixture", Endpoint: "https://runtime.example", Settings: connections.Settings{AuthKind: "official_runtime", BillingRoute: "subscription", Model: "concurrency-fixture"}}, "custom-concurrency")
	if err != nil {
		t.Fatal(err)
	}
	budgets := budget.New(f.db, f.identity, func(ctx context.Context, tx pgx.Tx, lease budget.Lease) error {
		_, fenceErr := jobs.ValidateFenceTx(ctx, tx, workflow.Lease(lease), "budget")
		return fenceErr
	}, nil)
	route, err := budgets.PutRoute(ctx, f.owner, f.org, budget.Route{ConnectionID: agent.ID, Model: "concurrency-fixture", Name: "default", Mode: "quota", PricingVersion: "fixture", MaxInputTokens: 1000, MaxOutputTokens: 100, MaxMilliseconds: 60000, MaxRequests: 1}, 0, "custom-concurrency")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, qualifyErr := budgets.QualifyRouteTx(ctx, tx, f.org, agent.ID, "concurrency-fixture", "default", route.Version, "local concurrency fixture")
		return qualifyErr
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = budgets.PutLimit(ctx, f.owner, f.org, budget.Limit{Scope: budget.Scope{Kind: "organisation", ID: f.org}, Period: "daily", Caps: budget.Caps{Tokens: i64(100000), Milliseconds: i64(3600000), Requests: i64(20), Concurrency: i64(4)}}, 0, "custom-concurrency"); err != nil {
		t.Fatal(err)
	}
	document := policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{Recipes: []string{"javascript"}, Models: []string{"concurrency-fixture"}, Routes: []string{agent.ID + "/default"}}}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, document, "custom", "custom-concurrency")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Repair})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, sim.Hash, "custom", "custom-concurrency"); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"value.js": []byte("exports.value = () => 1"), "value.test.js": []byte("require('node:test')('value',()=>require('node:assert').equal(require('./value').value(),2))")}
	reader := &repairContractReader{fixture: f, files: files}
	repairs := repair.New(f.db, f.identity, f.service, jobs, runners, policies, budgets, f.connections, profiles, reader, map[string]string{"javascript": "sha256:" + strings.Repeat("a", 64)})
	for _, action := range []string{"repair.source", "repair.report", "repair.custom"} {
		authority.Register(action, func(context.Context, pgx.Tx, workflow.Task, policy.Resolved) error { return nil })
	}
	credentials := make([]string, 0, 2)
	for index, name := range []string{"custom-concurrency-a", "custom-concurrency-b"} {
		head := strings.Repeat("a", 40)
		if index == 1 {
			head = strings.Repeat("d", 40)
		}
		observation := f.observation(name, "main", head, nil)
		observation.RepositoryID = repoIDs[index]
		finding := f.observe(t, observation)
		input := repair.Input{FindingID: finding.ID, FindingVersion: finding.Version, Recipe: "javascript", ModelConnectionID: agent.ID, ModelRoute: "default", RunnerPoolID: pool.ID, CustomProfileID: approved.ID, CustomProfileVersion: approved.Version, IdempotencyKey: domain.NewID()}
		preview, previewErr := repairs.Preview(ctx, f.owner, f.org, input)
		if previewErr != nil || len(preview.Blockers) > 0 {
			t.Fatalf("preview %d: %v blockers=%v", index, previewErr, preview.Blockers)
		}
		input.PlanDigest = preview.Context.Plan.Digest
		if _, err = repairs.Enqueue(ctx, f.owner, f.org, input, "custom-concurrency"); err != nil {
			t.Fatal(err)
		}
		assignment, claimErr := runners.Claim(ctx, supervisorTokens[index])
		if claimErr != nil {
			t.Fatal(claimErr)
		}
		for _, state := range []domain.TaskState{domain.TaskPlanning, domain.TaskRepairing} {
			if _, err = runners.Progress(ctx, assignment.Credential.Token, state); err != nil {
				t.Fatal(err)
			}
		}
		credentials = append(credentials, assignment.Credential.Token)
	}
	dispatcher := customcmd.NewDispatcher(f.db, runners, profiles, budgets)
	start := make(chan struct{})
	type result struct {
		authorized customcmd.Authorized
		err        error
	}
	results := make(chan result, len(credentials))
	for _, credential := range credentials {
		go func(token string) {
			<-start
			authorized, authorizeErr := dispatcher.Authorize(ctx, token)
			results <- result{authorized: authorized, err: authorizeErr}
		}(credential)
	}
	close(start)
	successes, capacityErrors := 0, 0
	for range credentials {
		got := <-results
		if got.err == nil && got.authorized.RunID != "" {
			successes++
		} else if errors.Is(got.err, customcmd.ErrCapacity) {
			capacityErrors++
		} else {
			t.Fatalf("unexpected concurrent authorize result: run=%q err=%v", got.authorized.RunID, got.err)
		}
	}
	if successes != 1 || capacityErrors != 1 {
		t.Fatalf("profile concurrency admitted %d jobs and denied %d; want 1 each", successes, capacityErrors)
	}
	var active int
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM custom_profile_runs WHERE org_id=$1 AND profile_id=$2 AND state IN ('queued','running')`, f.org, approved.ID).Scan(&active)
	}); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("persisted active profile runs=%d, want 1", active)
	}
}
