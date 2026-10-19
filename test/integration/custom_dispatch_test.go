package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/artifact"
	"github.com/reforgeapp/reforge/internal/budget"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/control"
	"github.com/reforgeapp/reforge/internal/customcmd"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/maintenance/repair"
	"github.com/reforgeapp/reforge/internal/policy"
	"github.com/reforgeapp/reforge/internal/runner"
	"github.com/reforgeapp/reforge/internal/workflow"
)

func i64(value int64) *int64 { return &value }

func TestCustomProfileDispatchAuthorizesExecutesAndFencesRevocation(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
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
	pool, err := runners.PutPool(ctx, f.owner, f.org, "", runner.PoolInput{Name: "Custom dispatch", RepositoryIDs: []string{f.repo}}, 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := runners.EnrollToken(ctx, f.owner, f.org, pool.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := runners.Enroll(ctx, enrollment.Token, "custom")
	if err != nil {
		t.Fatal(err)
	}

	profiles := customcmd.New(f.db, f.identity)
	profile, err := profiles.Create(ctx, f.owner, f.org, customcmd.Profile{
		Name: "Reviewed command", ImageDigest: "sha256:" + strings.Repeat("b", 64), Executable: "/bin/review", Argv: []string{"--json"},
		ProtocolVersion: customcmd.ProtocolVersion, MaxWallSeconds: 60, MaxOutputBytes: 1 << 20, MaxTurns: 2, Concurrency: 1,
	}, "custom-test")
	if err != nil {
		t.Fatal(err)
	}
	approved, err := profiles.Approve(ctx, f.owner, f.org, profile.ID, "fixture approval", profile.Version, "custom-test")
	if err != nil {
		t.Fatal(err)
	}

	agent, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{
		Kind: "agent", Provider: "custom_command", Name: "Custom runtime", Endpoint: "https://runtime.example",
		Settings: connections.Settings{AuthKind: "official_runtime", BillingRoute: "subscription", Model: "custom-model"},
	}, "custom-test")
	if err != nil {
		t.Fatal(err)
	}

	budgets := budget.New(f.db, f.identity, func(ctx context.Context, tx pgx.Tx, l budget.Lease) error {
		_, err := jobs.ValidateFenceTx(ctx, tx, workflow.Lease(l), "budget")
		return err
	}, nil)
	route, err := budgets.PutRoute(ctx, f.owner, f.org, budget.Route{ConnectionID: agent.ID, Model: "custom-model", Name: "default", Mode: "quota", PricingVersion: "custom", MaxInputTokens: 100000, MaxOutputTokens: 4096, MaxMilliseconds: 60000, MaxRequests: 1}, 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, e := budgets.QualifyRouteTx(ctx, tx, f.org, agent.ID, "custom-model", "default", route.Version, "fixture quota qualification")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = budgets.PutLimit(ctx, f.owner, f.org, budget.Limit{Scope: budget.Scope{Kind: "organisation", ID: f.org}, Period: "daily", Caps: budget.Caps{Tokens: i64(1000000), Milliseconds: i64(3600000), Requests: i64(100), Concurrency: i64(4)}}, 0, "test"); err != nil {
		t.Fatal(err)
	}

	document := policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{Recipes: []string{"javascript"}, Models: []string{"custom-model"}, Routes: []string{agent.ID + "/default"}}}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, document, "custom", "test")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Repair})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, sim.Hash, "custom", "test"); err != nil {
		t.Fatal(err)
	}

	reader := &repairContractReader{fixture: f, files: map[string][]byte{"value.js": []byte("exports.value = () => 1"), "value.test.js": []byte("require('node:test')('value',()=>require('node:assert').equal(require('./value').value(),2))")}}
	service := repair.New(f.db, f.identity, f.service, jobs, runners, policies, budgets, f.connections, profiles, reader, map[string]string{"javascript": "sha256:" + strings.Repeat("a", 64)})
	for _, action := range []string{"repair.source", "repair.report", "repair.custom"} {
		authority.Register(action, func(context.Context, pgx.Tx, workflow.Task, policy.Resolved) error { return nil })
	}

	finding := f.observe(t, f.observation("custom-dispatch", "main", strings.Repeat("a", 40), nil))
	input := repair.Input{FindingID: finding.ID, FindingVersion: finding.Version, Recipe: "javascript", ModelConnectionID: agent.ID, ModelRoute: "default", RunnerPoolID: pool.ID, CustomProfileID: approved.ID, CustomProfileVersion: approved.Version, IdempotencyKey: domain.NewID()}
	preview, err := service.Preview(ctx, f.owner, f.org, input)
	if err != nil || len(preview.Blockers) > 0 {
		t.Fatalf("preview: %v %+v", err, preview.Blockers)
	}
	if preview.Context.CustomProfile == nil || preview.Context.CustomProfile.ID != approved.ID {
		t.Fatalf("custom profile was not bound into the execution context: %+v", preview.Context.CustomProfile)
	}
	input.PlanDigest = preview.Context.Plan.Digest
	run, err := service.Enqueue(ctx, f.owner, f.org, input, "custom")
	if err != nil {
		t.Fatal(err)
	}

	job, err := runners.Claim(ctx, supervisor.Token)
	if err != nil {
		t.Fatal(err)
	}
	credential := job.Credential.Token
	for _, state := range []domain.TaskState{domain.TaskPlanning, domain.TaskRepairing} {
		if _, err = runners.Progress(ctx, credential, state); err != nil {
			t.Fatal(err)
		}
	}

	dispatcher := customcmd.NewDispatcher(f.db, runners, profiles, budgets)
	authorized, err := dispatcher.Authorize(ctx, credential)
	if err != nil {
		t.Fatal(err)
	}
	if authorized.RunID == "" || authorized.Profile.ImageDigest != approved.ImageDigest {
		t.Fatalf("authorize returned unexpected binding: %+v", authorized)
	}
	if _, err = dispatcher.Report(ctx, credential, customcmd.ReportInput{RunID: authorized.RunID, State: "completed_unverified", Reason: "profile exited zero", Usage: customcmd.Usage{Known: false}}); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state FROM custom_profile_runs WHERE org_id=$1 AND id=$2`, f.org, authorized.RunID).Scan(&state)
	}); err != nil {
		t.Fatal(err)
	}
	if state != "completed_unverified" {
		t.Fatalf("run state = %q", state)
	}

	if _, err = profiles.Revoke(ctx, f.owner, f.org, approved.ID, approved.Version, "custom-test"); err != nil {
		t.Fatal(err)
	}
	if _, err = dispatcher.Authorize(ctx, credential); !errors.Is(err, customcmd.ErrNotApproved) {
		t.Fatalf("revoked profile dispatch was admitted: %v", err)
	}
	_ = run
}
