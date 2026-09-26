package integration

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/config"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/httpapi"
	"reforge/internal/model"
	"reforge/internal/modelbroker"
	"reforge/internal/privateconnector"
	"reforge/internal/providers"
	"reforge/internal/runner"
	"reforge/internal/workflow"
)

func TestModelBrokerPrivateOllamaBudgetAndReplay(t *testing.T) {
	if os.Getenv("REFORGE_LOCAL_OLLAMA_TEST") != "1" {
		t.Skip("requires explicit local Ollama fixture")
	}
	f := newInventoryFixture(t, 1)
	f.importAll(t)
	repo := f.repository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	policies := workflow.New(f.db, f.identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) { return "model-test-policy", nil })
	runners := runner.New(f.db, f.identity, policies, nil)
	policies.RegisterScopeCheck(runners.CheckScopeTx)
	f.connections.RegisterRunnerCheck(runners.CheckRunnerTx)
	pool, err := runners.PutPool(ctx, f.owner, f.org, "", runner.PoolInput{Name: "Model test pool", RepositoryIDs: []string{repo.ID}}, 0, "model-test")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := runners.EnrollToken(ctx, f.owner, f.org, pool.ID, "model-test")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := runners.Enroll(ctx, grant.Token, "model-test-worker")
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
	privateAPI := httpapi.New(config.Config{PublicURL: "http://" + transport.Listener.Addr().String(), Development: true}, f.db)
	privateAPI.RegisterPrivateConnector(connector)
	transport.Config.Handler = privateAPI.Router
	transport.Start()
	defer transport.Close()
	client, err := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: transport.URL, Credential: credential.Token, Target: privateconnector.Target{OrgID: f.org, RunnerID: credential.Runner.ID}, Development: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	modelConnection, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "model", Provider: "compatible", Name: "Local Ollama", Endpoint: "http://127.0.0.1:55435", Settings: connections.Settings{AuthKind: "api_key", BillingRoute: "direct_api", Model: "qwen3:0.6b", Profile: "ollama"}, PrivateRoute: &connections.Route{RunnerID: credential.Runner.ID, Host: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}}}, "model-test")
	if err != nil {
		t.Fatal(err)
	}
	runClient, stopClient := context.WithCancel(ctx)
	clientErr := make(chan error, 1)
	go func() {
		for runClient.Err() == nil {
			err := client.RunOnce(runClient)
			if err != nil && !errors.Is(err, privateconnector.ErrUnavailable) && !errors.Is(err, privateconnector.ErrConflict) {
				clientErr <- err
				return
			}
		}
		clientErr <- runClient.Err()
	}()
	checked, err := f.connections.Test(ctx, f.owner, f.org, modelConnection.ID, modelConnection.Version, "model-test")
	if err != nil || checked.State != "healthy" {
		stopClient()
		t.Fatalf("local Ollama probe: %v %s", err, checked.State)
	}
	limits := budget.Limit{Scope: budget.Scope{Kind: "organisation", ID: f.org}, Period: "daily", Caps: budget.Caps{MicroUSD: ptr(int64(0)), Tokens: ptr(int64(200000)), Milliseconds: ptr(int64(120000)), Requests: ptr(int64(2)), Concurrency: ptr(int64(1))}}
	budgets := budget.New(f.db, f.identity, func(ctx context.Context, tx pgx.Tx, l budget.Lease) error {
		_, err := policies.ValidateFenceTx(ctx, tx, workflow.Lease(l), "budget")
		return err
	}, nil)
	limits, err = budgets.PutLimit(ctx, f.owner, f.org, limits, 0, "model-test")
	if err != nil {
		stopClient()
		t.Fatal(err)
	}
	_, err = budgets.PutRoute(ctx, f.owner, f.org, budget.Route{ConnectionID: modelConnection.ID, Model: "qwen3:0.6b", Name: "default", Mode: "priced", PricingVersion: "local-zero", MaxInputTokens: 65536, MaxOutputTokens: 512, MaxMilliseconds: 60000, MaxRequests: 1}, 0, "model-test")
	if err != nil {
		stopClient()
		t.Fatal(err)
	}
	task, err := policies.Enqueue(ctx, f.owner, f.org, workflow.EnqueueInput{RepositoryID: repo.ID, Recipe: "model-test", RecipeVersion: "1", TargetBranch: "main", ModelConnectionID: modelConnection.ID, ModelRoute: "default", RunnerPoolID: pool.ID, IdempotencyKey: domain.NewID()}, "model-test")
	if err != nil {
		stopClient()
		t.Fatal(err)
	}
	assignment, err := runners.Claim(ctx, credential.Token)
	if err != nil {
		stopClient()
		t.Fatal(err)
	}
	jobCredential := assignment.Credential.Token
	broker := modelbroker.New(f.db, runners, f.connections, budgets, connector, f.vault, true)
	if _, err := broker.Turn(ctx, jobCredential, model.Turn{}); !errors.Is(err, modelbroker.ErrUnavailable) {
		t.Fatalf("missing authority permitted broker: %v", err)
	}
	broker.AuthorizeReservation = func(context.Context, pgx.Tx, workflow.Task, budget.Reservation) error { return nil }
	turn := model.Turn{OperationID: task.OperationID, Model: "qwen3:0.6b", Messages: []model.Message{{Role: "user", Text: "Reply with one short word. /no_think"}}, MaxOutputTokens: 512, TimeoutMS: 30000}
	result, err := broker.Turn(ctx, jobCredential, turn)
	if err != nil || result.Text == "" || !result.Usage.Known {
		stopClient()
		t.Fatalf("model turn: %v %+v", err, result)
	}
	replay, err := broker.Turn(ctx, jobCredential, turn)
	if err != nil || replay.Text != result.Text {
		t.Fatalf("cached replay: %v %+v", err, replay)
	}
	changed := turn
	changed.Messages = []model.Message{{Role: "user", Text: "Changed input."}}
	if _, err = broker.Turn(ctx, jobCredential, changed); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("changed input accepted: %v", err)
	}
	limits.Caps.Tokens = ptr(int64(1))
	if _, err = budgets.PutLimit(ctx, f.owner, f.org, limits, limits.Version, "model-test-tighten"); err != nil {
		t.Fatalf("tighten budget: %v", err)
	}
	over := turn
	over.OperationID = domain.NewID()
	if _, err = broker.Turn(ctx, jobCredential, over); !errors.Is(err, budget.ErrCapacity) {
		t.Fatalf("overbudget turn dispatched: %v", err)
	}
	stopClient()
	if err := <-clientErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("private client: %v", err)
	}
}

func ptr(v int64) *int64 { return &v }
