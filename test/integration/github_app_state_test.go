package integration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/githubapp"
	"reforge/internal/secrets"
)

func (h *ghHarness) secondSession(userID string) (*http.Cookie, auth.Session) {
	h.t.Helper()
	ctx := context.Background()
	sessionID := domain.NewID()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		h.t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	if err := h.db.Identity(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,'second-session',now()+interval '1 hour')`, sessionID, userID, hex.EncodeToString(hash[:]))
		return err
	}); err != nil {
		h.t.Fatal(err)
	}
	session, err := h.serverAuth().Authenticate(ctx, token)
	if err != nil {
		h.t.Fatal(err)
	}
	return &http.Cookie{Name: h.serverAuth().CookieName(), Value: token}, session
}

func (h *ghHarness) connectManifest(inst string) string {
	h.t.Helper()
	_, installState := h.manifestToInstall(h.cookie, h.owner, h.org)
	oauth := location(h.t, h.get("/auth/github/install/callback?installation_id="+inst+"&setup_action=install&state="+url.QueryEscape(installState), h.cookie))
	state := h.authorize(oauth)
	got := result(h.t, h.get("/auth/github/oauth/callback?code=alice&state="+url.QueryEscape(state), h.cookie))
	if got.Get("github_result") != "connected" {
		h.t.Fatalf("manifest connect: %v", got)
	}
	return got.Get("connection")
}

func TestGitHubAppHandoffStylesheet(t *testing.T) {
	h := newGitHubHarness(t, "self-hosted")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(`<link rel="stylesheet" href="/assets/site.css">`), 0o600); err != nil {
		t.Fatal(err)
	}
	h.server.Config.WebDir = dir
	page := h.get(h.start(h.cookie, h.owner, h.org), h.cookie)
	body := page.Body.String()
	if page.Code != 200 || !strings.Contains(body, `href="/assets/site.css"`) || strings.Contains(body, "<script") || strings.Contains(body, "style=") {
		t.Fatalf("handoff stylesheet: %d %s", page.Code, body)
	}
}

func TestGitHubAppNamespaceInstallation(t *testing.T) {
	h := newGitHubHarness(t, "self-hosted")
	ctx := context.Background()
	inst := strconv.FormatInt(h.fake.instID, 10)
	h.fake.visible["tok-alice"] = []map[string]any{install(h.fake.instID, h.fake.appID, 500, "alice", "User")}
	id := h.connectManifest(inst)
	c, err := h.conns.Get(ctx, h.owner, h.org, id)
	if err != nil || c.Settings.Namespace != "installation" || !c.Settings.WebhookPending {
		t.Fatalf("managed namespace/webhook pending: %+v %v", c.Settings, err)
	}
	manual := fmt.Sprintf(`{"provider":"github","name":"manual app","endpoint":"https://api.github.com","settings":{"auth_kind":"github_app","billing_route":"forge","app_id":"7","installation_id":"9"},"secret":%q}`, rsaPEM(t))
	r := identityRequest(h.server, "POST", "/api/v1/orgs/"+h.org+"/connections/forges", manual, h.cookie, map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8080", "X-CSRF-Token": h.owner.CSRFToken})
	if r.Code != 201 {
		t.Fatalf("manual create: %d %s", r.Code, r.Body.String())
	}
	var created connections.Connection
	if err := json.Unmarshal(r.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Settings.Namespace != "installation" {
		t.Fatalf("manual namespace: %+v", created.Settings)
	}
	bad := fmt.Sprintf(`{"provider":"github","name":"bad namespace","endpoint":"https://api.github.com","settings":{"auth_kind":"github_app","billing_route":"forge","namespace":"user","app_id":"7","installation_id":"9"},"secret":%q}`, rsaPEM(t))
	if r := identityRequest(h.server, "POST", "/api/v1/orgs/"+h.org+"/connections/forges", bad, h.cookie, map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8080", "X-CSRF-Token": h.owner.CSRFToken}); r.Code != 400 {
		t.Fatalf("incompatible namespace accepted: %d %s", r.Code, r.Body.String())
	}
}

func TestGitHubAppWrongSessionKeepsPending(t *testing.T) {
	h := newGitHubHarness(t, "self-hosted")
	inst := strconv.FormatInt(h.fake.instID, 10)
	h.fake.visible["tok-alice"] = []map[string]any{install(h.fake.instID, h.fake.appID, 500, "alice", "User")}
	_, installState := h.manifestToInstall(h.cookie, h.owner, h.org)
	otherCookie, otherSession := h.secondSession(h.owner.User.ID)
	if otherSession.ID == h.owner.ID {
		t.Fatal("expected a distinct session")
	}
	wrong := result(t, h.get("/auth/github/install/callback?installation_id="+inst+"&state="+url.QueryEscape(installState), otherCookie))
	if wrong.Get("github_reason") != "session_mismatch" {
		t.Fatalf("wrong session consumed setup: %v", wrong)
	}
	if body := h.get("/api/v1/orgs/"+h.org+"/github-app", otherCookie).Body.String(); strings.Contains(body, `"pending"`) {
		t.Fatalf("wrong session sees pending setup: %s", body)
	}
	oauth := location(t, h.get("/auth/github/install/callback?installation_id="+inst+"&state="+url.QueryEscape(installState), h.cookie))
	state := h.authorize(oauth)
	got := result(t, h.get("/auth/github/oauth/callback?code=alice&state="+url.QueryEscape(state), h.cookie))
	if got.Get("github_result") != "connected" {
		t.Fatalf("legitimate callback after wrong session: %v", got)
	}
}

func TestGitHubAppHostedConfigRemovedRestart(t *testing.T) {
	h := newGitHubHarness(t, "hosted")
	ctx := context.Background()
	setupID := strings.TrimPrefix(h.start(h.cookie, h.owner, h.org), "/auth/github/setup/")
	stripped := githubapp.New(h.db, h.serverAuth(), h.vault, h.conns, h.inv, githubapp.Options{PublicURL: "http://127.0.0.1:8080", Edition: "hosted", Development: true})
	if _, err := stripped.Handoff(ctx, h.owner, setupID); !errors.Is(err, githubapp.ErrUnavailable) {
		t.Fatalf("handoff without hosted config: %v", err)
	}
	state := "restart-state-" + domain.NewID()
	sum := sha256.Sum256([]byte(state))
	err := h.db.Tenant(ctx, h.org, h.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO github_app_setups(org_id,id,user_id,session_id,mode,phase,name,state_hash,connection_id,webhook_id,app_id,expires_at) VALUES($1,$2,$3,$4,'hosted','authorizing','restart',$5,$6,$7,$8,now()+interval '10 minutes')`, h.org, domain.NewID(), h.owner.User.ID, h.owner.ID, hex.EncodeToString(sum[:]), domain.NewID(), domain.NewID(), h.fake.hostedID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	res := stripped.OAuthCallback(ctx, h.owner, "code", state, "")
	if res.Outcome != "failed" || res.Reason != "provider_unavailable" {
		t.Fatalf("oauth without hosted config: %+v", res)
	}
}

func TestGitHubAppWebhookPendingSurvivesTest(t *testing.T) {
	h := newGitHubHarness(t, "self-hosted")
	ctx := context.Background()
	inst := strconv.FormatInt(h.fake.instID, 10)
	h.fake.visible["tok-alice"] = []map[string]any{install(h.fake.instID, h.fake.appID, 500, "alice", "User")}
	id := h.connectManifest(inst)
	h.conns.Register("forge", "github", func(context.Context, connections.Resolved) (connections.ProbeResult, error) {
		return connections.ProbeResult{State: "healthy", Reason: "capability check passed", Capabilities: map[string]domain.Capability{}}, nil
	})
	c, err := h.conns.Get(ctx, h.owner, h.org, id)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := h.conns.Test(ctx, h.owner, h.org, id, c.Version, "test")
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != "healthy" || !updated.Settings.WebhookPending || !strings.Contains(updated.Reason, "webhook inactive") {
		t.Fatalf("webhook pending lost after test: %+v", updated)
	}
}

func TestGitHubAppExpiryCleanup(t *testing.T) {
	h := newGitHubHarness(t, "self-hosted")
	ctx := context.Background()
	expired, active := domain.NewID(), domain.NewID()
	err := h.db.Tenant(ctx, h.org, h.owner.User.ID, func(tx pgx.Tx) error {
		for _, item := range []struct{ id, ttl string }{{expired, "-1 second"}, {active, "1 hour"}} {
			if _, err := tx.Exec(ctx, `INSERT INTO github_app_setups(org_id,id,user_id,session_id,mode,phase,name,connection_id,webhook_id,expires_at) VALUES($1,$2,$3,$4,'manifest','created','cleanup',$5,$6,now()+$7::interval)`, h.org, item.id, h.owner.User.ID, h.owner.ID, domain.NewID(), domain.NewID(), item.ttl); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	err = h.db.Tenant(ctx, h.org, h.owner.User.ID, func(tx pgx.Tx) error {
		var gone, kept bool
		if err := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM github_app_setups WHERE id=$1)`, expired).Scan(&gone); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM github_app_setups WHERE id=$1)`, active).Scan(&kept); err != nil {
			return err
		}
		if !gone || !kept {
			t.Fatalf("cleanup expired=%v kept=%v", gone, kept)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (h *ghHarness) setupCount() int {
	h.t.Helper()
	var n int
	if err := h.db.Tenant(context.Background(), h.org, h.owner.User.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM github_app_setups WHERE org_id=$1`, h.org).Scan(&n)
	}); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func TestGitHubAppExpiryCleanupRLS(t *testing.T) {
	h := newGitHubHarness(t, "self-hosted")
	ctx := context.Background()
	orgB := domain.NewID()
	err := h.db.Tenant(ctx, orgB, h.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Tenant B')`, orgB); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, orgB, h.owner.User.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	insert := func(org, id, ttl string) {
		t.Helper()
		err := h.db.Tenant(ctx, org, h.owner.User.ID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO github_app_setups(org_id,id,user_id,session_id,mode,phase,name,connection_id,webhook_id,expires_at) VALUES($1,$2,$3,$4,'manifest','created','cleanup',$5,$6,now()+$7::interval)`, org, id, h.owner.User.ID, h.owner.ID, domain.NewID(), domain.NewID(), ttl)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	aExpired, aActive := domain.NewID(), domain.NewID()
	bExpired, bActive := domain.NewID(), domain.NewID()
	insert(h.org, aExpired, "-1 second")
	insert(h.org, aActive, "1 hour")
	insert(orgB, bExpired, "-1 second")
	insert(orgB, bActive, "1 hour")

	var leaked bool
	if err := h.db.Tenant(ctx, orgB, h.owner.User.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM github_app_setups WHERE id=$1)`, aExpired).Scan(&leaked)
	}); err != nil {
		t.Fatal(err)
	}
	if leaked {
		t.Fatal("tenant B observed tenant A expired setup")
	}
	var unscoped int
	if err := h.db.Pool.QueryRow(ctx, `SELECT count(*) FROM github_app_setups`).Scan(&unscoped); err != nil {
		t.Fatal(err)
	}
	if unscoped != 0 {
		t.Fatalf("unauthenticated context observed %d setups", unscoped)
	}

	if err := h.svc.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}

	check := func(org, active, expired string) {
		t.Helper()
		var hasActive, hasExpired bool
		if err := h.db.Tenant(ctx, org, h.owner.User.ID, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM github_app_setups WHERE id=$1)`, active).Scan(&hasActive); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM github_app_setups WHERE id=$1)`, expired).Scan(&hasExpired)
		}); err != nil {
			t.Fatal(err)
		}
		if !hasActive || hasExpired {
			t.Fatalf("cleanup org %s active=%v expired=%v", org, hasActive, hasExpired)
		}
	}
	check(h.org, aActive, aExpired)
	check(orgB, bActive, bExpired)
	if err := h.db.Pool.QueryRow(ctx, `SELECT count(*) FROM github_app_setups`).Scan(&unscoped); err != nil {
		t.Fatal(err)
	}
	if unscoped != 0 {
		t.Fatalf("unauthenticated context observed %d setups after cleanup", unscoped)
	}
}

func TestGitHubHostedAdoptsWebhookSecret(t *testing.T) {
	h := newGitHubHarness(t, "hosted")
	ctx := context.Background()
	inst := strconv.FormatInt(h.fake.instID, 10)
	h.fake.visible["tok-alice"] = []map[string]any{install(h.fake.instID, h.fake.hostedID, 800, "acme", "Organization")}
	installURL := location(t, h.get(h.start(h.cookie, h.owner, h.org), h.cookie))
	spoof := location(t, h.get("/auth/github/install/callback?installation_id="+inst+"&state="+url.QueryEscape(installURL.Query().Get("state")), h.cookie))
	got := result(t, h.get("/auth/github/oauth/callback?code=alice&state="+url.QueryEscape(h.authorize(spoof)), h.cookie))
	id := got.Get("connection")
	if got.Get("github_result") != "connected" {
		t.Fatalf("hosted connect: %v", got)
	}
	c, err := h.conns.Get(ctx, h.owner, h.org, id)
	if err != nil || c.Settings.Namespace != "installation" || c.Settings.Managed != "github_hosted" {
		t.Fatalf("hosted settings: %+v %v", c.Settings, err)
	}
	err = h.db.Tenant(ctx, h.org, "", func(tx pgx.Tx) error {
		var webhookID string
		var envelope []byte
		if err := tx.QueryRow(ctx, `SELECT id::text,envelope FROM inventory_webhooks WHERE connection_id=$1`, id).Scan(&webhookID, &envelope); err != nil {
			return err
		}
		var e secrets.Envelope
		if err := json.Unmarshal(envelope, &e); err != nil {
			return err
		}
		secret, err := h.vault.OpenContext(ctx, secrets.Binding{OrgID: h.org, ConnectionID: webhookID, Version: 1}, e)
		if err != nil {
			return err
		}
		defer clear(secret)
		if string(secret) != string(h.webhook) {
			t.Fatal("hosted webhook secret not adopted from operator config")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
