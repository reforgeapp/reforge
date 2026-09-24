package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/secrets"
)

func TestModelCatalogAuthorizationBoundary(t *testing.T) {
	db := authDB(t)
	ctx := context.Background()
	identity, server := identityServer(t, db, authConfig())
	cookie, owner := identityLogin(t, identity, server)
	org := owner.Organisations[0].ID
	vault, _ := secrets.New("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))})
	svc := connections.New(db, identity, vault, true)
	server.RegisterConnections(svc)
	const secret = "sk-catalog-never-return"
	var calls atomic.Int32
	fail := atomic.Bool{}
	svc.RegisterCatalog(func(_ context.Context, r connections.Resolved) ([]connections.CatalogItem, error) {
		calls.Add(1)
		if r.Secret != secret || r.Client == nil || r.Connection.ID != "" {
			t.Error("catalog received unexpected connection state")
		}
		if fail.Load() {
			return nil, errors.New("upstream said " + secret)
		}
		return []connections.CatalogItem{{ID: "gpt-x", Name: "GPT X"}, {ID: "leak-" + secret}, {ID: "gpt-x"}, {ID: "bare"}}, nil
	})
	body := func(endpoint, extra string) string {
		return `{"provider":"openai","endpoint":"` + endpoint + `","secret":"` + secret + `","settings":{"auth_kind":"api_key","billing_route":"direct_api"}` + extra + `}`
	}
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8080", "X-CSRF-Token": owner.CSRFToken}
	path := "/api/v1/orgs/" + org + "/connections/model-catalog"

	userID, scopedCookie := fixtureIdentity(t, db)
	if _, err := identity.PutMember(ctx, owner, org, userID, auth.Membership{Role: domain.Viewer}, 0, "test"); err != nil {
		t.Fatal(err)
	}
	denied := []struct {
		path, body string
		cookie     bool
		headers    map[string]string
		status     int
	}{
		{path, body("https://api.openai.example", ""), true, map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8080"}, 403},
		{path, body("https://api.openai.example", ""), true, map[string]string{"Content-Type": "application/json", "Origin": "http://attacker.test", "X-CSRF-Token": owner.CSRFToken}, 403},
		{"/api/v1/orgs/" + domain.NewID() + "/connections/model-catalog", body("https://api.openai.example", ""), true, headers, 403},
		{path, body("https://api.openai.example", ""), false, map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8080", "X-CSRF-Token": "fixture-csrf"}, 403},
		{path, body("https://api.openai.example", `,"private_route":{"runner_id":"`+domain.NewID()+`","host":"10.0.0.5","cidrs":["10.0.0.0/8"]}`), true, headers, 400},
		{path, body("https://10.0.0.5", ""), true, headers, 400},
		{path, body("https://169.254.169.254", ""), true, headers, 400},
		{path, body("http://api.openai.example", ""), true, headers, 400},
		{path, `{"provider":"openai","endpoint":"https://api.openai.example","secret":"eyJsubscription","settings":{"auth_kind":"api_key","billing_route":"direct_api"}}`, true, headers, 400},
		{path, `{"provider":"compatible","endpoint":"https://opencode.ai/zen/v1","secret":"","settings":{"profile":"opencode_zen","auth_kind":"api_key","billing_route":"direct_api"}}`, true, headers, 400},
		{path, `{"provider":"compatible","endpoint":"https://opencode.example/zen/v1","secret":"k","settings":{"profile":"opencode_zen","auth_kind":"api_key","billing_route":"direct_api"}}`, true, headers, 400},
	}
	for i, tc := range denied {
		c := cookie
		if !tc.cookie {
			c = scopedCookie
		}
		got := identityRequest(server, "POST", tc.path, tc.body, c, tc.headers)
		if got.Code != tc.status || strings.Contains(got.Body.String(), secret) {
			t.Fatalf("case %d: %d %s", i, got.Code, got.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatal("provider traffic before authorization and destination checks")
	}

	got := identityRequest(server, "POST", path, body("https://api.openai.example", ""), cookie, headers)
	var out struct {
		Items []connections.CatalogItem `json:"items"`
	}
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &out) != nil || strings.Contains(got.Body.String(), secret) {
		t.Fatalf("catalog: %d %s", got.Code, got.Body.String())
	}
	if len(out.Items) != 2 || out.Items[0] != (connections.CatalogItem{ID: "gpt-x", Name: "GPT X"}) || out.Items[1].Name != "bare" {
		t.Fatalf("catalog items not sanitized: %+v", out.Items)
	}
	fail.Store(true)
	got = identityRequest(server, "POST", path, body("https://api.openai.example", ""), cookie, headers)
	if got.Code != 502 || strings.Contains(got.Body.String(), secret) || strings.Contains(got.Body.String(), "upstream") {
		t.Fatalf("catalog failure leaked: %d %s", got.Code, got.Body.String())
	}
	if _, err := svc.Create(ctx, owner, org, connections.CreateRequest{Kind: "model", Provider: "openai", Name: "No model", Endpoint: "https://api.openai.example", Settings: connections.Settings{AuthKind: "api_key", BillingRoute: "direct_api"}, Secret: secret}, "test"); err != auth.ErrInvalid {
		t.Fatalf("model-less connection saved: %v", err)
	}
	err := db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM connections WHERE org_id=$1 AND kind='model'`, org).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("catalog persisted a connection")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
