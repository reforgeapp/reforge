package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/artifact"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/control"
	"github.com/reforgeapp/reforge/pkg/customcmd"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/runner"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

type customProfileBudgetFixture struct {
	fixture    *discoveryFixture
	profiles   *customcmd.Service
	dispatcher *customcmd.Dispatcher
	credential string
	profile    customcmd.Profile
	runID      string
	reserveID  string
}

func newCustomProfileBudgetFixture(t *testing.T) *customProfileBudgetFixture {
	t.Helper()
	f := newDiscoveryFixture(t)
	ctx := context.Background()
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
	pool, err := runners.PutPool(ctx, f.owner, f.org, "", runner.PoolInput{Name: "Custom budget", RepositoryIDs: []string{f.repo}}, 0, "budget-test")
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := runners.EnrollToken(ctx, f.owner, f.org, pool.ID, "budget-test")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := runners.Enroll(ctx, enrollment.Token, "custom-budget")
	if err != nil {
		t.Fatal(err)
	}
	profiles := customcmd.New(f.db, f.identity)
	profileInput := customProfileInput()
	profileInput.Name = "Budget fixture"
	profileInput.ImageDigest = "sha256:" + strings.Repeat("c", 64)
	profileInput.MaxTurns = 2
	profileInput.Concurrency = 1
	profile, err := profiles.Create(ctx, f.owner, f.org, profileInput, "budget-test")
	if err != nil {
		t.Fatal(err)
	}
	approved, err := profiles.Approve(ctx, f.owner, f.org, profile.ID, "accounting boundary fixture", profile.Version, "budget-test")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "agent", Provider: "custom_command", Name: "Budget runtime", Endpoint: "https://runtime.example", Settings: connections.Settings{AuthKind: "official_runtime", BillingRoute: "subscription", Model: "accounting-fixture"}}, "budget-test")
	if err != nil {
		t.Fatal(err)
	}
	budgets := budget.New(f.db, f.identity, func(ctx context.Context, tx pgx.Tx, lease budget.Lease) error {
		_, err := jobs.ValidateFenceTx(ctx, tx, workflow.Lease(lease), "budget")
		return err
	}, nil)
	route, err := budgets.PutRoute(ctx, f.owner, f.org, budget.Route{ConnectionID: agent.ID, Model: "accounting-fixture", Name: "default", Mode: "quota", PricingVersion: "fixture", MaxInputTokens: 10000, MaxOutputTokens: 4096, MaxMilliseconds: 60000, MaxRequests: 1}, 0, "budget-test")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := budgets.QualifyRouteTx(ctx, tx, f.org, agent.ID, "accounting-fixture", "default", route.Version, "local accounting fixture")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = budgets.PutLimit(ctx, f.owner, f.org, budget.Limit{Scope: budget.Scope{Kind: "organisation", ID: f.org}, Period: "daily", Caps: budget.Caps{Tokens: i64(1000000), Milliseconds: i64(3600000), Requests: i64(100), Concurrency: i64(4)}}, 0, "budget-test"); err != nil {
		t.Fatal(err)
	}
	document := policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{Recipes: []string{"javascript"}, Models: []string{"accounting-fixture"}, Routes: []string{agent.ID + "/default"}}}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, document, "custom", "budget-test")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Repair})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, sim.Hash, "custom", "budget-test"); err != nil {
		t.Fatal(err)
	}
	reader := &repairContractReader{fixture: f, files: map[string][]byte{"value.js": []byte("exports.value = () => 1"), "value.test.js": []byte("require('node:test')('value',()=>require('node:assert').equal(require('./value').value(),2))")}}
	repairs := repair.New(f.db, f.identity, f.service, jobs, runners, policies, budgets, f.connections, profiles, reader, map[string]string{"javascript": "sha256:" + strings.Repeat("a", 64)})
	for _, action := range []string{"repair.source", "repair.report", "repair.custom"} {
		authority.Register(action, func(context.Context, pgx.Tx, workflow.Task, policy.Resolved) error { return nil })
	}
	finding := f.observe(t, f.observation("custom-budget", "main", strings.Repeat("a", 40), nil))
	input := repair.Input{FindingID: finding.ID, FindingVersion: finding.Version, Recipe: "javascript", ModelConnectionID: agent.ID, ModelRoute: "default", RunnerPoolID: pool.ID, CustomProfileID: approved.ID, CustomProfileVersion: approved.Version, IdempotencyKey: domain.NewID()}
	preview, err := repairs.Preview(ctx, f.owner, f.org, input)
	if err != nil || len(preview.Blockers) > 0 {
		t.Fatalf("preview: %v blockers=%v", err, preview.Blockers)
	}
	input.PlanDigest = preview.Context.Plan.Digest
	if _, err = repairs.Enqueue(ctx, f.owner, f.org, input, "budget-test"); err != nil {
		t.Fatal(err)
	}
	job, err := runners.Claim(ctx, supervisor.Token)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []domain.TaskState{domain.TaskPlanning, domain.TaskRepairing} {
		if _, err = runners.Progress(ctx, job.Credential.Token, state); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := customcmd.NewDispatcher(f.db, runners, profiles, budgets)
	authorized, err := dispatcher.Authorize(ctx, job.Credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	var reservationID string
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT reservation_id::text FROM custom_profile_runs WHERE org_id=$1 AND id=$2`, f.org, authorized.RunID).Scan(&reservationID)
	}); err != nil {
		t.Fatal(err)
	}
	return &customProfileBudgetFixture{fixture: f, profiles: profiles, dispatcher: dispatcher, credential: job.Credential.Token, profile: approved, runID: authorized.RunID, reserveID: reservationID}
}

func TestCustomProfileReportBudgetAccounting(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage customcmd.Usage
		state string
	}{
		{name: "known usage settles once", usage: customcmd.Usage{Known: true, Tokens: 2, Milliseconds: 1500, Requests: 1}, state: "settled"},
		{name: "unknown usage stays held", usage: customcmd.Usage{Known: false}, state: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCustomProfileBudgetFixture(t)
			ctx := context.Background()
			if _, err := fixture.dispatcher.Authorize(ctx, fixture.credential); !errors.Is(err, customcmd.ErrCapacity) {
				t.Fatalf("second active dispatch must hit profile concurrency cap, got %v", err)
			}
			if count := readCustomTaskReservationCount(t, fixture); count != 1 {
				t.Fatalf("capacity denial created %d reservations for active task", count)
			}
			input := customcmd.ReportInput{RunID: fixture.runID, State: "failed", Reason: "accounting fixture; no process result asserted", Usage: tc.usage}
			if _, err := fixture.dispatcher.Report(ctx, fixture.credential, input); err != nil {
				t.Fatal(err)
			}
			reservation := readCustomBudgetReservation(t, fixture, fixture.reserveID)
			if reservation.State != tc.state {
				t.Fatalf("reservation state=%q want=%q", reservation.State, tc.state)
			}
			if tc.usage.Known {
				if reservation.Actual == nil || *reservation.Actual != (budget.Amount{Tokens: 2, Milliseconds: 1500, Requests: 1}) {
					t.Fatalf("settled usage=%+v", reservation.Actual)
				}
				if held := readCustomOrgHeld(t, fixture); held != (budget.Amount{}) {
					t.Fatalf("settled reservation retained hold: %+v", held)
				}
				before := readCustomOrgSpend(t, fixture)
				if _, err := fixture.dispatcher.Report(ctx, fixture.credential, input); err != nil {
					t.Fatalf("identical report replay: %v", err)
				}
				after := readCustomOrgSpend(t, fixture)
				if before != after || after != (budget.Amount{Tokens: 2, Milliseconds: 1500, Requests: 1}) {
					t.Fatalf("duplicate report changed spend: before=%+v after=%+v", before, after)
				}
			} else {
				if reservation.Actual != nil || reservation.Reference != "custom-profile:"+fixture.runID {
					t.Fatalf("unknown usage was treated as actual: %+v", reservation)
				}
				held := readCustomOrgHeld(t, fixture)
				if held != reservation.Maximum {
					t.Fatalf("unknown reservation hold=%+v maximum=%+v", held, reservation.Maximum)
				}
				if _, err := fixture.dispatcher.Report(ctx, fixture.credential, input); err != nil {
					t.Fatalf("identical unknown report replay: %v", err)
				}
				if again := readCustomOrgHeld(t, fixture); again != held {
					t.Fatalf("duplicate unknown report changed hold: before=%+v after=%+v", held, again)
				}
			}
			if _, err := fixture.profiles.Revoke(ctx, fixture.fixture.owner, fixture.fixture.org, fixture.profile.ID, fixture.profile.Version, "budget-test"); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.dispatcher.Authorize(ctx, fixture.credential); !errors.Is(err, customcmd.ErrNotApproved) {
				t.Fatalf("revoked profile authorize error=%v", err)
			}
		})
	}
}

func readCustomBudgetReservation(t *testing.T, fixture *customProfileBudgetFixture, id string) budget.Reservation {
	t.Helper()
	var reservation budget.Reservation
	err := fixture.fixture.db.Tenant(context.Background(), fixture.fixture.org, "", func(tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(context.Background(), `SELECT record FROM budget_reservations WHERE org_id=$1 AND id=$2`, fixture.fixture.org, id).Scan(&raw); err != nil {
			return err
		}
		return json.Unmarshal(raw, &reservation)
	})
	if err != nil {
		t.Fatal(err)
	}
	return reservation
}

func readCustomOrgSpend(t *testing.T, fixture *customProfileBudgetFixture) budget.Amount {
	t.Helper()
	var amount budget.Amount
	err := fixture.fixture.db.Tenant(context.Background(), fixture.fixture.org, "", func(tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(context.Background(), `SELECT amount FROM budget_spend WHERE org_id=$1 AND scope_kind='organisation' AND scope_id=$1`, fixture.fixture.org).Scan(&raw); err != nil {
			return err
		}
		return json.Unmarshal(raw, &amount)
	})
	if err != nil {
		t.Fatal(err)
	}
	return amount
}

func readCustomOrgHeld(t *testing.T, fixture *customProfileBudgetFixture) budget.Amount {
	t.Helper()
	var amount budget.Amount
	err := fixture.fixture.db.Tenant(context.Background(), fixture.fixture.org, "", func(tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(context.Background(), `SELECT held FROM budget_limits WHERE org_id=$1 AND scope_kind='organisation' AND scope_id=$1`, fixture.fixture.org).Scan(&raw); err != nil {
			return err
		}
		return json.Unmarshal(raw, &amount)
	})
	if err != nil {
		t.Fatal(err)
	}
	return amount
}

func readCustomTaskReservationCount(t *testing.T, fixture *customProfileBudgetFixture) int {
	t.Helper()
	var count int
	err := fixture.fixture.db.Tenant(context.Background(), fixture.fixture.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM budget_reservations WHERE org_id=$1 AND task_id=(SELECT task_id FROM custom_profile_runs WHERE org_id=$1 AND id=$2)`, fixture.fixture.org, fixture.runID).Scan(&count)
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}
