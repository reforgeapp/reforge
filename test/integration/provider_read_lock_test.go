package integration

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/config"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/httpapi"
	"github.com/reforgeapp/reforge/internal/privateconnector"
	"github.com/reforgeapp/reforge/internal/providers"
	"github.com/reforgeapp/reforge/internal/runner"
	"github.com/reforgeapp/reforge/internal/secrets"
	"github.com/reforgeapp/reforge/internal/workflow"
)

func TestProviderReadReleasesTenantLockAndRechecksAuthorization(t *testing.T) {
	db := authDB(t)
	identity, server := identityServer(t, db, authConfig())
	_, owner := identityLogin(t, identity, server)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	org := domain.NewID()
	if err := db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Provider read lock fixture')`, org); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	vault, err := secrets.New("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))})
	if err != nil {
		t.Fatal(err)
	}
	connService := connections.New(db, identity, vault, true)
	jobs := workflow.New(db, identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) {
		return "", errors.New("no repair authority")
	})
	runners := runner.New(db, identity, jobs, nil)
	connService.RegisterRunnerCheck(runners.CheckRunnerTx)
	pool, err := runners.PutPool(ctx, owner, org, "", runner.PoolInput{Name: "Read lock " + domain.NewID(), RepositoryIDs: []string{}}, 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := runners.EnrollToken(ctx, owner, org, pool.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := runners.Enroll(ctx, enrollment.Token, "provider-read-lock")
	if err != nil {
		t.Fatal(err)
	}
	connector, err := privateconnector.New(privateconnector.Config{Authenticate: runners.AuthenticateSupervisor, Development: true, TTL: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	transport := httptest.NewUnstartedServer(nil)
	privateAPI := httpapi.New(config.Config{PublicURL: "http://" + transport.Listener.Addr().String(), Development: true}, db)
	privateAPI.RegisterPrivateConnector(connector)
	transport.Config.Handler = privateAPI.Router
	transport.Start()
	defer transport.Close()
	client, err := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: transport.URL, Credential: supervisor.Token, Target: privateconnector.Target{OrgID: org, RunnerID: supervisor.Runner.ID}, Development: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce, releaseOnce sync.Once
	providerServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		select {
		case <-release:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		case <-r.Context().Done():
		}
	}))
	defer providerServer.Close()
	defer func() { releaseOnce.Do(func() { close(release) }) }()
	connection, err := connService.Create(ctx, owner, org, connections.CreateRequest{Kind: "forge", Provider: "github", Name: "Read lock", Endpoint: providerServer.URL + "/api/v3", Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge", Namespace: "acme", CAPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: providerServer.Certificate().Raw}))}, Secret: "provider-read-lock-secret", PrivateRoute: &connections.Route{RunnerID: supervisor.Runner.ID, Host: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET state='healthy' WHERE org_id=$1 AND id=$2`, org, connection.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	reader := providers.New(db, connService, connector, runners, true)
	var revoked atomic.Bool
	readDone := make(chan error, 1)
	clientDone := make(chan error, 1)
	go func() { clientDone <- client.RunOnce(ctx) }()
	go func() {
		_, err := reader.Read(ctx, org, connection.ID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeInventory, Inventory: &privateconnector.InventoryArgs{Limit: 10}}, func(context.Context, pgx.Tx, connections.Connection) error {
			if revoked.Load() {
				return auth.ErrForbidden
			}
			return nil
		})
		readDone <- err
	}()
	select {
	case <-started:
	case err := <-readDone:
		t.Fatalf("provider read ended before request: %v", err)
	case <-ctx.Done():
		t.Fatal("provider request did not start")
	}
	lockCtx, lockCancel := context.WithTimeout(ctx, time.Second)
	defer lockCancel()
	lockDone := make(chan error, 1)
	go func() {
		lockDone <- db.Tenant(lockCtx, org, "", func(tx pgx.Tx) error {
			var id string
			return tx.QueryRow(lockCtx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&id)
		})
	}()
	select {
	case err := <-lockDone:
		if err != nil {
			t.Fatalf("tenant lock remained held during provider read: %v", err)
		}
	case <-lockCtx.Done():
		t.Fatal("tenant lock remained held during provider read")
	}
	revoked.Store(true)
	releaseOnce.Do(func() { close(release) })
	if err := <-readDone; err == nil {
		t.Fatal("revoked read returned a result")
	}
	if err := <-clientDone; err != nil {
		t.Fatalf("private runner read failed: %v", err)
	}
}
