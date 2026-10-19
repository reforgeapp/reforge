package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/budget"
	"github.com/reforgeapp/reforge/internal/control"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/policy"
	"github.com/reforgeapp/reforge/internal/workflow"
)

func TestTaskBudgetHTTPAndCurrentPolicyDispatch(t *testing.T) {
	db := authDB(t)
	identity, server := identityServer(t, db, authConfig())
	cookie, owner := identityLogin(t, identity, server)
	ctx := context.Background()
	org, repo, connection := domain.NewID(), domain.NewID(), domain.NewID()
	err := db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO organisations(id,name) VALUES($1,'Integrated controls')`, []any{org}},
			{`INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, []any{org, owner.User.ID}},
			{`INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,'fixture','Control fixture')`, []any{org, repo}},
			{`INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','compatible','Contract fixture','https://model.example', '{"model":"fixture-model","auth_kind":"api_key","billing_route":"direct_api"}','healthy')`, []any{org, connection}},
		} {
			if _, err := tx.Exec(ctx, q.sql, q.args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(db, identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	authority := control.NewAuthority(policies)
	workflows := workflow.New(db, identity, authority.Check)
	budgets := budget.New(db, identity, func(ctx context.Context, tx pgx.Tx, l budget.Lease) error {
		_, err := workflows.ValidateFenceTx(ctx, tx, workflow.Lease(l), "budget")
		return err
	}, nil)
	server.RegisterWorkflow(workflows)
	server.RegisterBudget(budgets)
	base := "/api/v1/orgs/" + org
	headers := map[string]string{"Origin": "http://127.0.0.1:8080", "X-CSRF-Token": owner.CSRFToken, "Content-Type": "application/json", "If-Match": "\"0\""}
	input := workflow.EnqueueInput{RepositoryID: repo, Recipe: "go-upgrade", RecipeVersion: "1", TargetBranch: "main", ModelConnectionID: connection, ModelRoute: "default", IdempotencyKey: domain.NewID(), MaxAttempts: 2}
	body, _ := json.Marshal(input)
	response := identityRequest(server, "POST", base+"/tasks", string(body), cookie, headers)
	if response.Code != 409 {
		t.Fatalf("missing policy admitted task: %d", response.Code)
	}
	activate := func(document policy.Policy, expected int64) {
		t.Helper()
		version, err := policies.CreateVersion(ctx, owner, org, policy.Scope{Kind: "organisation", ID: org}, document, "Contract policy", "test")
		if err != nil {
			t.Fatal(err)
		}
		sim, err := policies.Simulate(ctx, owner, org, version.ID, repo, "", policy.Input{Action: policy.Repair})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = policies.Activate(ctx, owner, org, version.ID, repo, "", expected, sim.Hash, "Reviewed contract", "test"); err != nil {
			t.Fatal(err)
		}
	}
	document := policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{Recipes: []string{"go-upgrade"}, Models: []string{"fixture-model"}, Routes: []string{connection + "/default"}}}
	activate(document, 0)
	response = identityRequest(server, "POST", base+"/tasks", string(body), cookie, headers)
	var task workflow.Task
	if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &task) != nil {
		t.Fatalf("enqueue: %d %s", response.Code, response.Body.String())
	}
	input.ModelRoute = "unapproved"
	input.IdempotencyKey = domain.NewID()
	bad, _ := json.Marshal(input)
	if response = identityRequest(server, "POST", base+"/tasks", string(bad), cookie, headers); response.Code != 409 {
		t.Fatal("unapproved billing route admitted")
	}
	limitURL := base + "/budgets/organisation/" + org
	limitBody := `{"period":"daily","caps":{"micro_usd":10000,"concurrency":1},"paused":false}`
	if response = identityRequest(server, "PUT", limitURL, limitBody, cookie, headers); response.Code != 200 {
		t.Fatalf("budget: %d %s", response.Code, response.Body.String())
	}
	if response = identityRequest(server, "PUT", limitURL, limitBody, cookie, headers); response.Code != 409 {
		t.Fatal("stale budget overwrite accepted")
	}
	routeBody := `{"model":"fixture-model","name":"default","mode":"priced","pricing_version":"contract-1","input_micro_usd_per_million":1000000,"output_micro_usd_per_million":1000000,"max_input_tokens":1000,"max_output_tokens":100,"max_milliseconds":10000,"max_requests":1}`
	if response = identityRequest(server, "PUT", base+"/budget-routes/"+connection, routeBody, cookie, headers); response.Code != 200 {
		t.Fatalf("route: %d %s", response.Code, response.Body.String())
	}
	lease, err := workflows.Claim(ctx, "trusted-controller", []string{org}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var reservation budget.Reservation
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var err error
		reservation, err = budgets.ReserveTx(ctx, tx, budget.Lease(lease), budget.Quote{OperationID: domain.NewID(), Model: "fixture-model", Route: "default", RouteVersion: 1, InputTokens: 20, MaxOutputTokens: 10, MaxMilliseconds: 5000, MaxRequests: 1})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if response = identityRequest(server, "GET", base+"/usage/"+reservation.ID, "", cookie, nil); response.Code != 200 {
		t.Fatal("scoped reservation unavailable")
	}
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := workflows.PrepareIntentTx(ctx, tx, lease, "publish", domain.NewID(), json.RawMessage(`{}`))
		return err
	})
	if !errors.Is(err, workflow.ErrPolicy) {
		t.Fatal("unregistered publication authority admitted")
	}
	document.Deny = []policy.Action{policy.Repair}
	activate(document, 1)
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := budgets.MarkDispatchedTx(ctx, tx, budget.Lease(lease), reservation.ID)
		return err
	})
	if err == nil {
		t.Fatal("changed policy authorized spending")
	}
	var state string
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state FROM budget_reservations WHERE org_id=$1 AND id=$2`, org, reservation.ID).Scan(&state)
	}); err != nil || state != "reserved" {
		t.Fatalf("failed dispatch altered reservation: %s %v", state, err)
	}
	if response = identityRequest(server, "POST", fmt.Sprintf("%s/tasks/%s/cancel", base, task.ID), "", cookie, nil); response.Code != 403 {
		t.Fatal("cancel omitted CSRF")
	}
}
