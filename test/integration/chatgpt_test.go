package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/chatgpt"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/secrets"
)

func TestChatGPTConnectionCustodyAndSetupIsolation(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	status, err := f.connections.ChatGPTStatus(ctx, f.owner, f.org)
	if err != nil || status.Available {
		t.Fatalf("unconfigured hosted client enabled: %+v %v", status, err)
	}
	f.connections.SetChatGPT(chatgpt.Config{ClientID: "oaiapp_fixture", RedirectURL: "https://app.example/auth/openai/callback", PlanUsageEnabled: true})
	const access = "oauth-test-access-never-return"
	f.connections.RegisterCatalog(func(_ context.Context, r connections.Resolved) ([]connections.CatalogItem, error) {
		if r.Secret != access || r.Connection.Settings.BillingRoute != "subscription" || r.Connection.Endpoint != chatgpt.Resource {
			t.Fatal("catalog used wrong identity or billing")
		}
		return []connections.CatalogItem{{ID: "eligible-model", Name: "Eligible model"}}, nil
	})
	id, other := domain.NewID(), domain.NewID()
	browser, state := chatgpt.Random(), chatgpt.Random()
	raw, _ := json.Marshal(map[string]any{"name": "ChatGPT fixture", "nonce": "nonce", "verifier": "verifier", "credentials": chatgpt.Credentials{ClientID: "oaiapp_fixture", Subject: "verified-subject", Email: "owner@example.test", AccessToken: access, RefreshToken: "oauth-test-refresh", IDToken: "retained-id", Scopes: "resource.invoke chatgpt.tokens.use.direct", Expiry: time.Now().Add(time.Hour)}})
	envelope, err := f.vault.SealContext(ctx, secrets.Binding{OrgID: f.org, ConnectionID: id, Version: 1}, raw)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, _ := json.Marshal(envelope)
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO chatgpt_setups(org_id,id,user_id,session_hash,browser_hash,state_hash,envelope,phase) VALUES($1,$2,$3,$4,$5,$6,$7,'pending')`, f.org, id, f.owner.User.ID, chatgpt.Digest(f.owner.ID), chatgpt.Digest(browser), chatgpt.Digest(state), encrypted); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tenant(ctx, other, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Other ChatGPT tenant')`, other); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, other, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.connections.CleanupChatGPT(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM chatgpt_setups WHERE org_id=$1 AND id=$2`, f.org, id).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Fatal("cleanup removed active sign-in")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	callbackState := f.org + "." + id + "." + state
	if _, _, err := f.connections.CompleteChatGPT(ctx, f.owner, callbackState, chatgpt.Random(), "code", ""); !errors.Is(err, chatgpt.ErrAuthorization) {
		t.Fatalf("different browser accepted: %v", err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE chatgpt_setups SET phase='authenticated' WHERE org_id=$1 AND id=$2`, f.org, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.connections.CompleteChatGPT(ctx, f.owner, callbackState, browser, "code", ""); !errors.Is(err, chatgpt.ErrAuthorization) {
		t.Fatalf("authorization callback replay accepted: %v", err)
	}
	if _, err := f.connections.ChatGPTSetup(ctx, f.owner, other, id); !errors.Is(err, chatgpt.ErrAuthorization) {
		t.Fatalf("cross-tenant setup readable: %v", err)
	}
	_, otherSession := identityLogin(t, f.identity, f.server)
	if _, err := f.connections.ChatGPTSetup(ctx, otherSession, f.org, id); !errors.Is(err, chatgpt.ErrAuthorization) {
		t.Fatalf("another browser session took setup: %v", err)
	}
	if _, err := f.connections.SaveChatGPT(ctx, f.owner, f.org, id, "hidden-model", "save"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("uncatalogued model accepted: %v", err)
	}
	connection, err := f.connections.SaveChatGPT(ctx, f.owner, f.org, id, "eligible-model", "save")
	if err != nil || !connection.ModelReady() || connection.Settings.BillingRoute != "subscription" {
		t.Fatalf("saved connection unusable: %+v %v", connection, err)
	}
	if _, err := f.connections.SaveChatGPT(ctx, f.owner, f.org, id, "eligible-model", "replay"); !errors.Is(err, chatgpt.ErrAuthorization) {
		t.Fatalf("setup reused: %v", err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		resolved, err := f.connections.ResolveTx(ctx, tx, f.org, connection.ID, "")
		if err != nil {
			return err
		}
		if resolved.Secret != access {
			t.Fatal("broker did not receive current access token")
		}
		if resolved.Client != nil {
			resolved.Client.CloseIdleConnections()
		}
		var stored []byte
		if err := tx.QueryRow(ctx, `SELECT envelope FROM connection_secrets WHERE org_id=$1 AND connection_id=$2`, f.org, connection.ID).Scan(&stored); err != nil {
			return err
		}
		if strings.Contains(string(stored), access) || strings.Contains(string(stored), "oauth-test-refresh") {
			t.Fatal("tokens stored unencrypted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	limits := budget.New(f.db, f.identity, nil, nil)
	pricing, err := limits.Pricing(ctx, f.owner, f.org, connection.ID)
	if err != nil || !pricing.Manual || !pricing.Automatic || len(pricing.Presets) != 0 {
		t.Fatalf("subscription routed through API pricing: %+v %v", pricing, err)
	}
	if _, err := f.connections.Rotate(ctx, f.owner, f.org, connection.ID, connection.Version, "sk-api", "rotate"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("API key replaced subscription credentials: %v", err)
	}
	if _, err := f.connections.SetRoute(ctx, f.owner, f.org, connection.ID, connection.Version, &connections.Route{RunnerID: domain.NewID()}, "route"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("OAuth token routed to private connector: %v", err)
	}
	f.server.RegisterChatGPT(f.connections)
	response := identityRequest(f.server, http.MethodGet, "/api/v1/orgs/"+f.org+"/chatgpt/setups/"+id, "", f.cookie, nil)
	if strings.Contains(response.Body.String(), access) || strings.Contains(response.Body.String(), "oauth-test-refresh") {
		t.Fatal("token leaked through API")
	}
	expired := domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO chatgpt_setups(org_id,id,user_id,session_hash,browser_hash,state_hash,envelope,phase,expires_at) VALUES($1,$2,$3,'expired','expired','expired',$4,'pending',now()-interval '1 minute')`, f.org, expired, f.owner.User.ID, encrypted)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.connections.CleanupChatGPT(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM chatgpt_setups WHERE org_id=$1 AND id=$2`, f.org, expired).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("expired credentials retained")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

}
