package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/artifact"
	"reforge/internal/budget"
	"reforge/internal/config"
	"reforge/internal/connections"
	"reforge/internal/control"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/httpapi"
	"reforge/internal/inventory"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/maintenance/repair"
	"reforge/internal/model"
	"reforge/internal/modelbroker"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/providers"
	"reforge/internal/runner"
	"reforge/internal/runnerclient"
	"reforge/internal/sandbox"
	"reforge/internal/workflow"
)

func TestRepairLiveControllerRunnerAndGitea(t *testing.T) { runLiveRepair(t, false) }
func TestRepairLiveDependencyUpgrade(t *testing.T)        { runLiveRepair(t, true) }
func TestRepairLiveProtectedMerge(t *testing.T) {
	t.Setenv("REFORGE_LIVE_REPAIR_MERGE", "1")
	runLiveRepair(t, false)
}

func runLiveRepair(t *testing.T, upgrade bool) {
	if os.Getenv("REFORGE_LIVE_REPAIR_TEST") != "1" {
		t.Skip("requires disposable PostgreSQL, Gitea, Ollama and gVisor")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	t.Setenv("REFORGE_TEST_ROOT", root)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	f := newInventoryFixture(t, 0)
	repo := localPrivateRepository(t)
	seed := seedRepairFixture(t, ctx, repo, root, upgrade)
	native, token := seed.Native, seed.Token
	modelName := os.Getenv("REFORGE_REPAIR_TEST_MODEL")
	if modelName == "" {
		modelName = "qwen3:1.7b"
	}
	imageRoot := filepath.Join(t.TempDir(), "image")
	if output, err := exec.CommandContext(ctx, "python3", filepath.Join(root, "scripts/build-runner-images.py"), "--stack", "javascript", "--output", imageRoot).CombinedOutput(); err != nil {
		t.Fatalf("image preparation: %v %s", err, output)
	}
	digest, err := sandbox.ImageDigest(imageRoot)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	authority := control.NewAuthority(policies)
	jobs := workflow.New(f.db, f.identity, authority.Check)
	artifactRoot := t.TempDir()
	if err := os.Chmod(artifactRoot, 0700); err != nil {
		t.Fatal(err)
	}
	artifacts, err := artifact.NewLocal(f.db, artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer artifacts.Close()
	runners := runner.New(f.db, f.identity, jobs, artifacts)
	jobs.RegisterScopeCheck(runners.CheckScopeTx)
	f.connections.RegisterRunnerCheck(runners.CheckRunnerTx)
	budgets := budget.New(f.db, f.identity, func(ctx context.Context, tx pgx.Tx, lease budget.Lease) error {
		_, err := jobs.ValidateFenceTx(ctx, tx, workflow.Lease(lease), "budget")
		return err
	}, nil)
	connector, err := privateconnector.New(privateconnector.Config{Authenticate: runners.AuthenticateSupervisor, Development: true, TTL: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	providers.Factory{Development: true}.Register(f.connections)
	providers.RegisterPrivate(f.connections, connector, runners)
	reader := providers.New(f.db, f.connections, connector, runners, true)
	portfolio := inventory.New(f.db, f.identity, f.vault, reader, providers.DecodeWebhook)
	discoveries := discovery.New(f.db, f.identity, reader)
	repairs := repair.New(f.db, f.identity, discoveries, jobs, runners, policies, budgets, f.connections, reader.ForExecution(), map[string]string{"javascript": digest})
	broker := modelbroker.New(f.db, runners, f.connections, budgets, connector, f.vault, true)
	broker.AuthorizeReservation = repairs.CheckModelTx
	runners.CompletionCheck = repairs.CheckCompletion
	authority.Register("repair.stage", repairs.CheckStage)
	authority.Register("stage", repairs.CheckStage)
	authority.Register("publish", repairs.CheckPublish)
	authority.Register("outbox.dispatch", func(ctx context.Context, tx pgx.Tx, task workflow.Task, p policy.Resolved) error {
		if task.State == domain.TaskValidating {
			return repairs.CheckStage(ctx, tx, task, p)
		}
		return repairs.CheckPublish(ctx, tx, task, p)
	})
	for _, action := range []string{"repair.source", "repair.report", "model.turn"} {
		authority.Register(action, func(_ context.Context, _ pgx.Tx, task workflow.Task, _ policy.Resolved) error {
			if action == "model.turn" && task.State != domain.TaskPlanning && task.State != domain.TaskRepairing {
				return workflow.ErrPolicy
			}
			if task.State != domain.TaskReproducing && task.State != domain.TaskPlanning && task.State != domain.TaskRepairing && task.State != domain.TaskValidating && task.State != domain.TaskPublishing {
				return workflow.ErrPolicy
			}
			return nil
		})
	}
	transport := httptest.NewUnstartedServer(nil)
	api := httpapi.New(config.Config{PublicURL: "http://" + transport.Listener.Addr().String(), Development: true}, f.db)
	api.Auth = f.identity
	api.RegisterRunner(runners)
	api.RegisterPrivateConnector(connector)
	api.RegisterModelBroker(broker)
	api.RegisterRepair(repairs)
	transport.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runner/v1/private/results" {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var result struct {
				Result privateconnector.Result `json:"result"`
			}
			if json.Unmarshal(raw, &result) == nil && result.Result.Failure != nil {
				t.Logf("local fixture private failure code=%s uncertain=%t", result.Result.Failure.Code, result.Result.Failure.Uncertain)
			}
		}
		if r.URL.Path != "/runner/v1/model-turns" {
			api.Router.ServeHTTP(w, r)
			return
		}
		response := httptest.NewRecorder()
		api.Router.ServeHTTP(response, r)
		var turn model.TurnResult
		if response.Code == 200 && json.Unmarshal(response.Body.Bytes(), &turn) == nil {
			names := []string{}
			for _, call := range turn.ToolCalls {
				names = append(names, call.Name)
				t.Logf("local fixture tool=%s arguments=%s", call.Name, call.Arguments[:min(len(call.Arguments), 1800)])
			}
			t.Logf("local fixture model tools=%v text=%q", names, turn.Text[:min(len(turn.Text), 1800)])
		}
		for name, values := range response.Header() {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	})
	transport.Start()
	defer transport.Close()
	pool, err := runners.PutPool(ctx, f.owner, f.org, "", runner.PoolInput{Name: "Live repair", RepositoryIDs: []string{}}, 0, "live")
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := runners.EnrollToken(ctx, f.owner, f.org, pool.ID, "live")
	if err != nil {
		t.Fatal(err)
	}
	credentialRoot := t.TempDir()
	if err = os.Chmod(credentialRoot, 0700); err != nil {
		t.Fatal(err)
	}
	client, err := runnerclient.New(runnerclient.Config{Endpoint: transport.URL, CredentialFile: filepath.Join(credentialRoot, "credential"), Name: "live", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Enroll(ctx, enrollment.Token); err != nil {
		t.Fatal(err)
	}
	supervisor, credential := client.Supervisor()
	private, err := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: transport.URL, Credential: credential, Target: privateconnector.Target{OrgID: f.org, RunnerID: supervisor.ID}, Development: true})
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	privateCtx, stopPrivate := context.WithCancel(ctx)
	privateDone := make(chan error, 1)
	go func() {
		for privateCtx.Err() == nil {
			if err := private.RunOnce(privateCtx); err != nil && !errors.Is(err, privateconnector.ErrUnavailable) && !errors.Is(err, privateconnector.ErrConflict) {
				privateDone <- err
				return
			}
			select {
			case <-privateCtx.Done():
			case <-time.After(25 * time.Millisecond):
			}
		}
		privateDone <- privateCtx.Err()
	}()
	defer func() { stopPrivate(); <-privateDone }()
	route := &connections.Route{RunnerID: supervisor.ID, Host: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}}
	forgeConnection, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "forge", Provider: "gitea", Name: "Live forge", Endpoint: "http://127.0.0.1:53000", Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge"}, Secret: token, PrivateRoute: route}, "live")
	if err != nil {
		t.Fatal(err)
	}
	forgeConnection, err = f.connections.Test(ctx, f.owner, f.org, forgeConnection.ID, forgeConnection.Version, "live")
	if err != nil || forgeConnection.State != "healthy" {
		t.Fatalf("forge connection: %v %s", err, forgeConnection.State)
	}
	modelConnection, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "model", Provider: "compatible", Name: "Live Ollama", Endpoint: captureLocalModel(t, root), Settings: connections.Settings{AuthKind: "api_key", BillingRoute: "direct_api", Model: modelName, Profile: "ollama"}, PrivateRoute: route}, "live")
	if err != nil {
		t.Fatal(err)
	}
	modelConnection, err = f.connections.Test(ctx, f.owner, f.org, modelConnection.ID, modelConnection.Version, "live")
	if err != nil || modelConnection.State != "healthy" {
		t.Fatalf("model connection: %v %s", err, modelConnection.State)
	}
	drain := func(id string) inventory.Job {
		for i := 0; i < 30; i++ {
			job, err := portfolio.Job(ctx, f.owner, f.org, id)
			if err != nil {
				t.Fatal(err)
			}
			if job.State == "complete" {
				return job
			}
			lease, err := portfolio.Claim(ctx, f.org, "live")
			if err != nil || lease == nil {
				t.Fatalf("inventory claim: %v", err)
			}
			if err = portfolio.Step(ctx, *lease); err != nil {
				t.Fatalf("inventory step: %v", err)
			}
		}
		t.Fatal("inventory did not converge")
		return inventory.Job{}
	}
	scan, err := portfolio.StartSync(ctx, f.owner, f.org, inventory.SyncInput{ConnectionID: forgeConnection.ID}, "live")
	if err != nil {
		t.Fatal(err)
	}
	scan = drain(scan.ID)
	imported, err := portfolio.StartImport(ctx, f.owner, f.org, scan.ID, inventory.ImportInput{NativeIDs: []string{repo.NativeID}}, scan.Version, "live")
	if err != nil {
		t.Fatal(err)
	}
	drain(imported.ID)
	if err = portfolio.Maintain(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	lease, err := portfolio.Claim(ctx, f.org, "live")
	if err != nil || lease == nil || lease.Kind != "refresh" {
		t.Fatalf("refresh claim: %v", err)
	}
	if err = portfolio.Step(ctx, *lease); err != nil {
		t.Fatal(err)
	}
	drain(lease.ID)
	repositoryID := lease.RepositoryID
	pool, err = runners.PutPool(ctx, f.owner, f.org, pool.ID, runner.PoolInput{Name: pool.Name, RepositoryIDs: []string{repositoryID}}, pool.Version, "live")
	if err != nil {
		t.Fatal(err)
	}
	if upgrade {
		if _, err = discoveries.PutConfig(ctx, f.owner, f.org, repositoryID, discovery.Config{TrustedBots: []discovery.BotIdentity{{Kind: "renovate", ActorID: seed.AuthorID}}, MergeAuthority: "observe"}, 0, "live"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = discoveries.StartScan(ctx, f.owner, f.org, repositoryID, "live"); err != nil {
		t.Fatal(err)
	}
	if worked, err := discoveries.RunOrganisationOnce(ctx, f.org); err != nil || !worked {
		t.Fatalf("discovery: %v %v", worked, err)
	}
	category := "ci_failure"
	if upgrade {
		category = "dependency_update"
	}
	findings, err := discoveries.List(ctx, f.owner, f.org, 20, "", discovery.Filter{RepositoryID: repositoryID, Category: category})
	if err != nil || len(findings.Items) != 1 {
		t.Fatalf("native failing finding: count=%d err=%v", len(findings.Items), err)
	}
	if upgrade && (findings.Items[0].Evidence.Ownership != "bot" || findings.Items[0].Evidence.Bot != "renovate" || findings.Items[0].Evidence.HeadSHA == findings.Items[0].Evidence.TargetSHA) {
		t.Fatal("upgrade not identified from immutable native bot/head evidence")
	}
	document := policy.Policy{Schema: "maintenance/v1", ForbiddenPaths: []string{"vendor/**"}, Allow: policy.Lists{Recipes: []string{"javascript"}, Models: []string{modelName}, Routes: []string{modelConnection.ID + "/default"}}}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, document, "live", "live")
	if err != nil {
		t.Fatal(err)
	}
	simulation, err := policies.Simulate(ctx, f.owner, f.org, version.ID, repositoryID, "", policy.Input{Action: policy.Repair})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, repositoryID, "", 0, simulation.Hash, "live", "live"); err != nil {
		t.Fatal(err)
	}
	if _, err = budgets.PutRoute(ctx, f.owner, f.org, budget.Route{ConnectionID: modelConnection.ID, Model: modelName, Name: "default", Mode: "priced", PricingVersion: "local-zero-cost", MaxInputTokens: 100000, MaxOutputTokens: 1024, MaxMilliseconds: 180000, MaxRequests: 1}, 0, "live"); err != nil {
		t.Fatal(err)
	}
	if _, err = budgets.PutLimit(ctx, f.owner, f.org, budget.Limit{Scope: budget.Scope{Kind: "organisation", ID: f.org}, Period: "daily", Caps: budget.Caps{MicroUSD: ptr(0), Tokens: ptr(1000000), Milliseconds: ptr(900000), Requests: ptr(16), Concurrency: ptr(1)}}, 0, "live"); err != nil {
		t.Fatal(err)
	}
	input := repair.Input{FindingID: findings.Items[0].ID, FindingVersion: findings.Items[0].Version, Recipe: "javascript", ModelConnectionID: modelConnection.ID, ModelRoute: "default", RunnerPoolID: pool.ID, IdempotencyKey: domain.NewID()}
	preview, err := repairs.Preview(ctx, f.owner, f.org, input)
	if err != nil || len(preview.Blockers) > 0 {
		t.Fatalf("preview: %v %v", err, preview.Blockers)
	}
	input.PlanDigest = preview.Context.Plan.Digest
	run, err := repairs.Enqueue(ctx, f.owner, f.org, input, "live")
	if err != nil {
		t.Fatal(err)
	}
	runsc, tool := "/tmp/reforge-gvisor/bin/runsc", filepath.Join(root, "bin/reforge-sandbox-tool")
	stateRoot := filepath.Join(t.TempDir(), "runtime")
	cfg := sandbox.RuntimeConfig{Runsc: runsc, RunscSHA256: fileDigest(t, runsc), Tool: tool, ToolSHA256: fileDigest(t, tool), StateRoot: stateRoot, Images: map[string]string{digest: imageRoot}, Development: true, Rootless: true, MemoryBytes: 2 << 30, DiskBytes: 512 << 20, CPUs: 1, MaxProcesses: 128}
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() { workerDone <- client.Run(workerCtx, runnerclient.RepairProcessor(cfg)) }()
	defer func() { stopWorker(); <-workerDone }()
	for {
		run, err = repairs.Get(ctx, f.owner, f.org, run.Task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Task.State == domain.TaskCompleted {
			break
		}
		if run.Task.State == domain.TaskFailed || run.Task.State == domain.TaskBlocked || run.Task.State == domain.TaskReconciling {
			reason := ""
			if run.Report != nil {
				reason = fmt.Sprintf("%s turns=%d candidate=%s", run.Report.Reason, run.Report.Turns, fmt.Sprintf("%+v", run.Report.Candidate))
			}
			t.Fatalf("live repair stopped: task=%s run=%s reason=%s report=%s", run.Task.State, run.State, run.Task.Reason, reason)
		}
		select {
		case <-ctx.Done():
			t.Fatal("live repair deadline")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if run.Change == nil || run.Report == nil || !repair.Reproduced(run.Report.Baseline) || !repair.Verified(run.Context.Plan, run.Report.Baseline, run.CandidateChecks) || run.Report.Diff == "" || len(run.CandidateArtifacts) == 0 {
		t.Fatal("completed run lacks native validation evidence")
	}
	var pulls []struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	native("GET", "/pulls?state=all", nil, &pulls)
	expectedPulls := 1
	if upgrade {
		expectedPulls = 2
	}
	candidateCount := 0
	for _, pull := range pulls {
		if pull.Head.SHA == run.CandidateSHA {
			candidateCount++
		}
	}
	if len(pulls) != expectedPulls || candidateCount != 1 {
		t.Fatalf("native publication count/head: count=%d", len(pulls))
	}
	if upgrade {
		if len(run.Report.Patches) != 1 || run.Report.Patches[0].Path != "value.js" || run.Context.Plan.BaselineSHA != seed.Head || run.Context.Plan.TargetSHA != seed.Target {
			t.Fatal("companion changed bot dependency, tests or pinned revisions")
		}
	}
	t.Log(fmt.Sprintf("real native PR=%s baseline=%s candidate=%s turns=%d", run.Change.ID, run.Context.Plan.BaselineSHA, run.CandidateSHA, run.Report.Turns))
	if os.Getenv("REFORGE_LIVE_REPAIR_MERGE") == "1" {
		verifyLiveProtectedMerge(t, ctx, f, policies, discoveries, reader, route, forgeConnection, repositoryID, repo, run, document, native, root)
	}
}

type repairFixtureSeed struct {
	Native   func(string, string, any, any)
	Token    string
	Head     string
	Target   string
	ChangeID string
	AuthorID string
}

func seedRepairFixture(t *testing.T, ctx context.Context, repo forge.RepoRef, root string, upgrade bool) repairFixtureSeed {
	t.Helper()
	secret, err := os.ReadFile(filepath.Join(root, ".local/gitea/reforge-bot.token"))
	if err != nil {
		t.Fatal("local bot fixture credential missing")
	}
	token := strings.TrimSpace(string(secret))
	clear(secret)
	native := func(method, suffix string, in, out any) {
		t.Helper()
		body, _ := json.Marshal(in)
		r, _ := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:53000/api/v1/repos/"+repo.FullName+suffix, bytes.NewReader(body))
		r.Header.Set("Authorization", "token "+token)
		r.Header.Set("Content-Type", "application/json")
		response, err := (&http.Client{Timeout: 10 * time.Second}).Do(r)
		if err != nil {
			t.Fatal("local fixture request failed")
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("local fixture %s %s HTTP%d", method, suffix, response.StatusCode)
		}
		if out != nil && json.NewDecoder(response.Body).Decode(out) != nil {
			t.Fatal("local fixture response invalid")
		}
	}
	files := map[string]string{"value.js": "export function value(a, b) { return a - b; }\n", "value.test.js": "import { strict as assert } from 'node:assert';\nimport { value } from './value.js';\nimport test from 'node:test';\ntest('value', () => { assert.equal(value(2, 3), 5); assert.equal(value(-4, 2), -2); });\n"}
	if upgrade {
		files = map[string]string{
			"package.json":           "{\n  \"type\": \"module\",\n  \"imports\": {\"#dependency\": \"./vendor/v1.js\"},\n  \"dependencies\": {\"fixture-value\": \"file:vendor/v1\"}\n}\n",
			"value.js":               "import { readValue } from '#dependency';\nexport function value(input) { return readValue(input); }\n",
			"value.test.js":          "import { strict as assert } from 'node:assert';\nimport { value } from './value.js';\nimport test from 'node:test';\ntest('value', () => { for (const input of [2, 7, -1]) assert.equal(value(input), input); });\n",
			"vendor/v1.js":           "export function readValue(input) { return input; }\n",
			"vendor/v1/package.json": "{\n  \"name\": \"fixture-value\",\n  \"version\": \"1.0.0\"\n}\n",
			"vendor/v2.js":           "export function readValue(input) { return { value: input }; }\n",
			"vendor/v2/package.json": "{\n  \"name\": \"fixture-value\",\n  \"version\": \"2.0.0\"\n}\n",
		}
	}
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	for _, name := range paths {
		body := files[name]
		native("POST", "/contents/"+name, map[string]any{"content": base64.StdEncoding.EncodeToString([]byte(body)), "message": "Add broken maintenance fixture", "branch": "main"}, nil)
	}
	var branch struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	native("GET", "/branches/main", nil, &branch)
	if upgrade {
		var packageFile struct {
			SHA string `json:"sha"`
		}
		native("GET", "/contents/package.json?ref=main", nil, &packageFile)
		branchName := "renovate/fixture-value"
		native("POST", "/branches", map[string]any{"new_branch_name": branchName, "old_ref_name": branch.Commit.ID}, nil)
		updated := "{\n  \"type\": \"module\",\n  \"imports\": {\"#dependency\": \"./vendor/v2.js\"},\n  \"dependencies\": {\"fixture-value\": \"file:vendor/v2\"}\n}\n"
		native("PUT", "/contents/package.json", map[string]any{"branch": branchName, "sha": packageFile.SHA, "content": base64.StdEncoding.EncodeToString([]byte(updated)), "message": "Upgrade fixture-value"}, nil)
		var pull struct {
			Number int64 `json:"number"`
			Head   struct {
				SHA string `json:"sha"`
			} `json:"head"`
			User struct {
				ID int64 `json:"id"`
			} `json:"user"`
		}
		native("POST", "/pulls", map[string]any{"head": branchName, "base": "main", "title": "Upgrade fixture-value", "body": "Dependency fixture upgrade"}, &pull)
		if pull.Number <= 0 || len(pull.Head.SHA) != 40 || pull.User.ID <= 0 {
			t.Fatal("native dependency upgrade PR identity incomplete")
		}
		native("POST", "/statuses/"+pull.Head.SHA, map[string]any{"state": "failure", "context": "fixture/node-test", "description": "value test fails on upgraded dependency"}, nil)
		return repairFixtureSeed{Native: native, Token: token, Head: pull.Head.SHA, Target: branch.Commit.ID, ChangeID: fmt.Sprint(pull.Number), AuthorID: fmt.Sprint(pull.User.ID)}
	}
	native("POST", "/statuses/"+branch.Commit.ID, map[string]any{"state": "failure", "context": "fixture/node-test", "description": "value test fails on the original fixture"}, nil)
	return repairFixtureSeed{Native: native, Token: token, Head: branch.Commit.ID}
}
