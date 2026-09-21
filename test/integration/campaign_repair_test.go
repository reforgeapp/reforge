package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/campaign"
	"reforge/internal/control"
	"reforge/internal/domain"
	"reforge/internal/maintenance/repair"
	"reforge/internal/policy"
	"reforge/internal/runner"
	"reforge/internal/workflow"
)

func TestCampaignRepairAdmissionBudgetAndResume(t *testing.T) {
	f := newCampaignFixture(t, 1)
	ctx := context.Background()
	p, e := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if e != nil {
		t.Fatal(e)
	}
	authority := control.NewAuthority(p)
	jobs := workflow.New(f.db, f.identity, authority.Check)
	runners := runner.New(f.db, f.identity, jobs, nil)
	pool, e := runners.PutPool(ctx, f.owner, f.org, "", runner.PoolInput{Name: "Campaign repair contract", RepositoryIDs: []string{f.repo}}, 0, "test")
	if e != nil {
		t.Fatal(e)
	}
	model := domain.NewID()
	e = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','compatible','Campaign fixture','https://model.invalid','{"model":"fixture-model","auth_kind":"api_key","billing_route":"direct_api"}','healthy')`, f.org, model)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	images := map[string]string{"javascript": "sha256:" + strings.Repeat("a", 64)}
	service := campaign.New(f.db, f.identity, p, jobs, images, nil)
	f.service = service
	jobs.RegisterScopeCheck(func(ctx context.Context, tx pgx.Tx, org, kind, id string) error {
		if kind == "campaign" {
			return service.CheckScopeTx(ctx, tx, org, kind, id)
		}
		return runners.CheckScopeTx(ctx, tx, org, kind, id)
	})
	jobs.RegisterCampaignAuthority(service.TaskAuthorityTx)
	budgets := budget.New(f.db, f.identity, func(ctx context.Context, tx pgx.Tx, l budget.Lease) error {
		_, e := jobs.ValidateFenceTx(ctx, tx, workflow.Lease(l), "budget")
		return e
	}, func(ctx context.Context, tx pgx.Tx, org, id string) error {
		return service.CheckScopeTx(ctx, tx, org, "campaign", id)
	})
	route, e := budgets.PutRoute(ctx, f.owner, f.org, budget.Route{ConnectionID: model, Model: "fixture-model", Name: "default", Mode: "priced", PricingVersion: "fixture-zero", MaxInputTokens: 10000, MaxOutputTokens: 4096, MaxMilliseconds: 60000, MaxRequests: 1}, 0, "test")
	if e != nil {
		t.Fatal(e)
	}
	reader := &repairContractReader{fixture: f.discoveryFixture, files: map[string][]byte{"value.js": []byte("exports.value = () => 1"), "value.test.js": []byte("require('node:test')('value',()=>require('node:assert').equal(require('./value').value(),2))")}}
	repairs := repair.New(f.db, f.identity, f.discoveryFixture.service, jobs, runners, p, budgets, f.connections, reader, images)
	service.ConfigureExecution(repairs, nil, nil, nil)
	finding := f.observe(t, f.observation("campaign-repair", "main", strings.Repeat("a", 40), nil))
	f.input = campaign.Input{Name: "Repair campaign", Kind: "repair", Members: []campaign.MemberInput{{RepositoryID: f.repo, Repair: &repair.Input{FindingID: finding.ID, FindingVersion: finding.Version, Recipe: "javascript", ModelConnectionID: model, ModelRoute: "default", RunnerPoolID: pool.ID}}}, CanarySize: 1, BatchSize: 1, Concurrency: 1, Success: "published"}
	c := f.create(t)
	if _, e = service.Control(ctx, f.owner, f.org, c.ID, "start", "reviewed", "", c.Version, "test"); !errors.Is(e, budget.ErrUnknown) {
		t.Fatalf("unbudgeted campaign started: %v", e)
	}
	money, concurrency, requests := int64(1000000), int64(2), int64(1)
	for _, scope := range []budget.Scope{{Kind: "organisation", ID: f.org}, {Kind: "campaign", ID: c.ID}} {
		caps := budget.Caps{MicroUSD: &money, Concurrency: &concurrency}
		if scope.Kind == "campaign" {
			caps.Requests = &requests
		}
		if _, e = budgets.PutLimit(ctx, f.owner, f.org, budget.Limit{Scope: scope, Period: "daily", Caps: caps}, 0, "test"); e != nil {
			t.Fatal(e)
		}
	}
	c = f.control(t, c, "start")
	c = f.step(t, c.ID)
	page, e := service.Members(ctx, f.owner, f.org, c.ID, "", 100)
	if e != nil || len(page.Items) != 1 || page.Items[0].ActionID == "" {
		t.Fatalf("real repair admission: %+v %v campaign=%+v", page, e, c)
	}
	member := page.Items[0]
	run, e := repairs.Get(ctx, f.owner, f.org, member.ActionID)
	if e != nil || run.Task.CampaignID != c.ID {
		t.Fatalf("campaign task binding %+v %v", run.Task, e)
	}
	c = f.step(t, c.ID)
	var count int
	e = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM workflow_tasks WHERE org_id=$1 AND campaign_id=$2`, f.org, c.ID).Scan(&count)
	})
	if e != nil || count != 1 {
		t.Fatalf("duplicate admission %d %v", count, e)
	}
	lease, e := jobs.ClaimScoped(ctx, "campaign-contract", f.org, pool.ID, []string{f.repo}, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	quote := budget.Quote{OperationID: domain.NewID(), Model: "fixture-model", Route: "default", RouteVersion: route.Version, InputTokens: 10, MaxOutputTokens: 10, MaxMilliseconds: 1000, MaxRequests: 1}
	var reservation budget.Reservation
	e = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		var e error
		reservation, e = budgets.ReserveTx(ctx, tx, budget.Lease(lease), quote)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	quote.OperationID = domain.NewID()
	e = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error { _, e := budgets.ReserveTx(ctx, tx, budget.Lease(lease), quote); return e })
	if !errors.Is(e, budget.ErrCapacity) {
		t.Fatalf("campaign overspend accepted: %v", e)
	}
	e = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, e := budgets.MarkDispatchedTx(ctx, tx, budget.Lease(lease), reservation.ID); e != nil {
			return e
		}
		_, e := budgets.MarkUnknownTx(ctx, tx, f.org, reservation.ID, "simulated lost billing response")
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	c = f.step(t, c.ID)
	if c.State != "paused" {
		t.Fatalf("unknown usage did not pause: %+v", c)
	}
	if _, e = service.Control(ctx, f.owner, f.org, c.ID, "resume", "reviewed", "continue_current_stage", c.Version, "test"); !errors.Is(e, budget.ErrCapacity) {
		t.Fatalf("unknown spend resumed: %v", e)
	}
	e = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, e := budgets.SettleTx(ctx, tx, f.org, reservation.ID, budget.Settlement{Known: true, Actual: budget.Amount{Requests: 1}, Reference: "contract usage receipt"})
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	c = f.control(t, c, "resume")
	c = f.step(t, c.ID)
	task, e := jobs.Get(ctx, f.owner, f.org, member.ActionID)
	if e != nil || task.State != domain.TaskQueued {
		t.Fatalf("reviewed repair resume %+v %v campaign=%+v", task, e, c)
	}
	if _, e = jobs.Enqueue(ctx, f.owner, f.org, workflow.EnqueueInput{CampaignID: c.ID}, "inject"); !errors.Is(e, auth.ErrInvalid) {
		t.Fatalf("caller injected campaign authority: %v", e)
	}
	f.server.RegisterCampaigns(service)
	body, _ := json.Marshal(map[string]string{"reason": "HTTP contract"})
	path := "/api/v1/orgs/" + f.org + "/campaigns/" + c.ID + "/pause"
	response := identityRequest(f.server, "POST", path, string(body), f.cookie, map[string]string{"Content-Type": "application/json", "If-Match": "\"1\""})
	if response.Code != 403 {
		t.Fatalf("missing CSRF: %d %s", response.Code, response.Body.String())
	}
	response = identityRequest(f.server, "GET", "/api/v1/orgs/"+domain.NewID()+"/campaigns/"+c.ID, "", f.cookie, nil)
	if response.Code != 403 {
		t.Fatalf("cross-tenant campaign read: %d", response.Code)
	}
}
