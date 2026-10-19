package integration

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/config"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/githubapp"
	"github.com/reforgeapp/reforge/internal/httpapi"
	"github.com/reforgeapp/reforge/internal/inventory"
	"github.com/reforgeapp/reforge/internal/secrets"
	"github.com/reforgeapp/reforge/internal/store"
)

type fakeGitHub struct {
	mu         sync.Mutex
	pem        string
	challenges map[string]bool
	users      map[string]int64
	visible    map[string][]map[string]any
	admins     map[string]bool
	suspended  bool
	calls      int
	appID      int64
	hostedID   int64
	instID     int64
}

func rsaPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func install(id, app, account int64, login, kind string) map[string]any {
	return map[string]any{"id": id, "app_id": app, "account": map[string]any{"id": account, "login": login, "type": kind}}
}

func (f *fakeGitHub) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	user := func(r *http.Request) string { return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") }
	mux.HandleFunc("POST /app-manifests/{code}/conversions", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls++
		f.mu.Unlock()
		if r.PathValue("code") != "manifest-code" {
			http.Error(w, "gone", 404)
			return
		}
		write(w, map[string]any{"id": f.appID, "slug": "reforge-test", "client_id": "Iv1.test", "client_secret": "client-secret-value", "webhook_secret": "webhook-secret-value-123", "pem": f.pem, "owner": map[string]any{"id": 500, "login": "alice", "type": "User"}})
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		f.mu.Lock()
		ok := f.challenges[base64.RawURLEncoding.EncodeToString(sum[:])]
		f.mu.Unlock()
		if !ok || r.PostForm.Get("client_secret") == "" {
			write(w, map[string]any{"error": "bad_verification_code"})
			return
		}
		write(w, map[string]any{"access_token": "tok-" + r.PostForm.Get("code")})
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]any{"id": f.users[user(r)]})
	})
	mux.HandleFunc("GET /user/installations", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]any{"installations": f.visible[user(r)]})
	})
	mux.HandleFunc("GET /user/memberships/orgs/{org}", func(w http.ResponseWriter, r *http.Request) {
		role := "member"
		if f.admins[user(r)] {
			role = "admin"
		}
		write(w, map[string]any{"state": "active", "role": role, "organization": map[string]any{"id": 800}})
	})
	mux.HandleFunc("GET /app/installations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if strings.Count(user(r), ".") != 2 {
			http.Error(w, "jwt", 401)
			return
		}
		for _, list := range f.visible {
			for _, inst := range list {
				if strconv.FormatInt(inst["id"].(int64), 10) == r.PathValue("id") {
					out := map[string]any{}
					for k, v := range inst {
						out[k] = v
					}
					if f.suspended {
						out["suspended_at"] = time.Now()
					}
					write(w, out)
					return
				}
			}
		}
		http.Error(w, "missing", 404)
	})
	return mux
}

type ghHarness struct {
	t       *testing.T
	db      *store.Store
	server  *httpapi.Server
	svc     *githubapp.Service
	conns   *connections.Service
	inv     *inventory.Service
	vault   *secrets.Vault
	fake    *fakeGitHub
	origin  string
	org     string
	owner   auth.Session
	cookie  *http.Cookie
	webhook []byte
}

func newGitHubHarness(t *testing.T, edition string) *ghHarness {
	db := authDB(t)
	identity, server := identityServer(t, db, authConfig())
	cookie, owner := identityLogin(t, identity, server)
	vault, err := secrets.New("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("g", 32)))})
	if err != nil {
		t.Fatal(err)
	}
	h := &ghHarness{t: t, db: db, server: server, vault: vault, org: owner.Organisations[0].ID, owner: owner, cookie: cookie, fake: &fakeGitHub{pem: rsaPEM(t), challenges: map[string]bool{}, users: map[string]int64{"tok-alice": 500, "tok-mallory": 900}, visible: map[string][]map[string]any{}, admins: map[string]bool{"tok-alice": true}, appID: time.Now().UnixNano()%1e9 + 1, hostedID: time.Now().UnixNano()%1e9 + 2, instID: time.Now().UnixNano()%1e12 + 1}}
	gh := httptest.NewServer(h.fake.handler(t))
	t.Cleanup(gh.Close)
	h.origin = gh.URL
	h.conns = connections.New(db, identity, vault, true)
	server.RegisterConnections(h.conns)
	h.inv = inventory.New(db, identity, vault, nil, nil)
	server.RegisterInventory(h.inv)
	opt := githubapp.Options{PublicURL: "http://127.0.0.1:8080", Edition: edition, Development: true, Web: gh.URL, API: gh.URL}
	if edition == "hosted" {
		dir := t.TempDir()
		files := map[string]string{"client": "hosted-client-secret-value", "key": rsaPEM(t), "hook": "hosted-webhook-secret-value"}
		for name, value := range files {
			if err := writeFile(dir+"/"+name, value); err != nil {
				t.Fatal(err)
			}
		}
		hosted, err := githubapp.LoadHosted(configFiles(dir, h.fake.hostedID))
		if err != nil {
			t.Fatal(err)
		}
		opt.Hosted = hosted
		h.webhook = []byte(files["hook"])
	}
	h.svc = githubapp.New(db, identity, vault, h.conns, h.inv, opt)
	server.RegisterGitHubApp(h.svc)
	return h
}

func (h *ghHarness) get(path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	return identityRequest(h.server, "GET", path, "", cookie, nil)
}

func (h *ghHarness) start(cookie *http.Cookie, session auth.Session, org string) string {
	h.t.Helper()
	r := identityRequest(h.server, "POST", "/api/v1/orgs/"+org+"/github-app/setups", `{"name":"Reforge test"}`, cookie, map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8080", "X-CSRF-Token": session.CSRFToken})
	if r.Code != 201 {
		h.t.Fatalf("start: %d %s", r.Code, r.Body.String())
	}
	var out githubapp.Created
	_ = json.Unmarshal(r.Body.Bytes(), &out)
	return out.HandoffURL
}

func location(t *testing.T, r *httptest.ResponseRecorder) *url.URL {
	t.Helper()
	if r.Code != 303 {
		t.Fatalf("expected redirect: %d %s", r.Code, r.Body.String())
	}
	u, err := url.Parse(r.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (h *ghHarness) authorize(u *url.URL) string {
	h.fake.mu.Lock()
	h.fake.challenges[u.Query().Get("code_challenge")] = u.Query().Get("code_challenge_method") == "S256"
	h.fake.mu.Unlock()
	return u.Query().Get("state")
}

func (h *ghHarness) manifestToInstall(cookie *http.Cookie, session auth.Session, org string) (string, string) {
	h.t.Helper()
	page := h.get(h.start(cookie, session, org), cookie)
	if page.Code != 200 || !strings.Contains(page.Header().Get("Content-Security-Policy"), "form-action "+h.origin) || page.Header().Get("Referrer-Policy") != "no-referrer" {
		h.t.Fatalf("handoff: %d %v", page.Code, page.Header())
	}
	body := page.Body.String()
	if !strings.Contains(body, "&#34;active&#34;:false") || !strings.Contains(body, "&#34;setup_on_update&#34;:true") || strings.Contains(body, "<script") {
		h.t.Fatal("manifest must escape, disable loopback webhook and accept configuration updates")
	}
	start := strings.Index(body, "state=") + len("state=")
	state, _ := url.QueryUnescape(body[start : start+strings.IndexByte(body[start:], '"')])
	install := location(h.t, h.get("/auth/github/manifest/callback?code=manifest-code&state="+url.QueryEscape(state), cookie))
	if !strings.HasPrefix(install.String(), h.origin+"/apps/reforge-test/installations/new") {
		h.t.Fatalf("install redirect %s", install)
	}
	return state, install.Query().Get("state")
}

func result(t *testing.T, r *httptest.ResponseRecorder) url.Values {
	t.Helper()
	u := location(t, r)
	if !strings.HasSuffix(u.Path, "/connections") && (u.Path != "/" || u.Query().Get("github_result") != "expired") {
		t.Fatalf("final redirect %s", u)
	}
	return u.Query()
}

func TestGitHubManifestSetupEndToEnd(t *testing.T) {
	h := newGitHubHarness(t, "self-hosted")
	ctx := context.Background()
	inst := strconv.FormatInt(h.fake.instID, 10)
	h.fake.visible["tok-alice"] = []map[string]any{install(h.fake.instID, h.fake.appID, 500, "alice", "User")}
	h.fake.visible["tok-mallory"] = h.fake.visible["tok-alice"]

	manifestState, installState := h.manifestToInstall(h.cookie, h.owner, h.org)
	if got := result(t, h.get("/auth/github/manifest/callback?code=manifest-code&state="+url.QueryEscape(manifestState), h.cookie)); got.Get("github_result") != "expired" || h.fake.calls != 1 {
		t.Fatalf("manifest replay: %v calls=%d", got, h.fake.calls)
	}
	oauth := location(t, h.get("/auth/github/install/callback?installation_id="+inst+"&setup_action=install&state="+url.QueryEscape(installState), h.cookie))
	if oauth.Query().Get("code_challenge_method") != "S256" || oauth.Query().Get("client_id") != "Iv1.test" {
		t.Fatalf("authorize %s", oauth)
	}
	state := h.authorize(oauth)

	var wg sync.WaitGroup
	outcomes := make([]url.Values, 2)
	for i := range outcomes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outcomes[i] = result(t, h.get("/auth/github/oauth/callback?code=alice&state="+url.QueryEscape(state), h.cookie))
		}(i)
	}
	wg.Wait()
	connected := 0
	var id string
	for _, o := range outcomes {
		if o.Get("github_result") == "connected" {
			connected++
			id = o.Get("connection")
		}
	}
	if connected != 1 || !auth.ValidID(id) {
		t.Fatalf("concurrent callbacks: %v", outcomes)
	}
	detail := h.get("/api/v1/orgs/"+h.org+"/connections/"+id, h.cookie)
	if detail.Code != 200 || strings.Contains(detail.Body.String(), "PRIVATE KEY") || !strings.Contains(detail.Body.String(), `"managed":"github_manifest"`) || !strings.Contains(detail.Body.String(), "Webhook inactive") {
		t.Fatalf("connection detail: %s", detail.Body.String())
	}
	err := h.db.Tenant(ctx, h.org, "", func(tx pgx.Tx) error {
		r, err := h.conns.ResolveTx(ctx, tx, h.org, id, "")
		if err != nil {
			return err
		}
		if r.Secret != h.fake.pem || r.Connection.Settings.AppID != strconv.FormatInt(h.fake.appID, 10) || r.Connection.Settings.InstallationID != inst {
			t.Fatal("managed App credential not resolvable")
		}
		var setups, hooks int
		if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM github_app_setups),(SELECT count(*) FROM inventory_webhooks WHERE connection_id=$1)`, id).Scan(&setups, &hooks); err != nil {
			return err
		}
		if setups != 0 || hooks != 1 {
			t.Fatalf("setup wipe/webhook adoption: %d %d", setups, hooks)
		}
		var raw string
		return tx.QueryRow(ctx, `SELECT envelope::text FROM inventory_webhooks WHERE connection_id=$1`, id).Scan(&raw)
	})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := h.conns.Get(ctx, h.owner, h.org, id)
	if _, err := h.inv.ConfigureWebhook(ctx, h.owner, h.org, id, 1, "test"); !errors.Is(err, connections.ErrManaged) {
		t.Fatalf("managed webhook rotate: %v", err)
	}
	if err := h.inv.RevokeWebhook(ctx, h.owner, h.org, id, 1, "test"); !errors.Is(err, connections.ErrManaged) {
		t.Fatalf("managed webhook revoke: %v", err)
	}
	if _, err := h.conns.Rotate(ctx, h.owner, h.org, id, c.Version, "not a key", "test"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("managed rotate accepted invalid PEM: %v", err)
	}

	_, installState = h.manifestToInstall(h.cookie, h.owner, h.org)
	state = h.authorize(location(t, h.get("/auth/github/install/callback?installation_id="+inst+"&state="+url.QueryEscape(installState), h.cookie)))
	if got := result(t, h.get("/auth/github/oauth/callback?code=mallory&state="+url.QueryEscape(state), h.cookie)); got.Get("github_reason") != "app_created_not_owner" {
		t.Fatalf("non-owner GitHub user: %v", got)
	}

	_, installState = h.manifestToInstall(h.cookie, h.owner, h.org)
	if got := result(t, h.get("/auth/github/install/callback?setup_action=request&state="+url.QueryEscape(installState), h.cookie)); got.Get("github_result") != "pending_approval" {
		t.Fatalf("pending approval: %v", got)
	}
	status := h.get("/api/v1/orgs/"+h.org+"/github-app", h.cookie)
	if !strings.Contains(status.Body.String(), `"phase":"awaiting_install"`) || !strings.Contains(status.Body.String(), "resume_url") {
		t.Fatalf("status %s", status.Body.String())
	}
	_ = h.db.Tenant(ctx, h.org, h.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE github_app_setups SET expires_at=now()-interval '1 second'`)
		return err
	})
	var pending githubapp.Status
	_ = json.Unmarshal(status.Body.Bytes(), &pending)
	if got := result(t, h.get(pending.Pending.ResumeURL, h.cookie)); got.Get("github_result") != "expired" {
		t.Fatalf("expired resume: %v", got)
	}
	if n := h.setupCount(); n != 0 {
		t.Fatalf("expired setup must be wiped, found %d", n)
	}

	userID, otherCookie := fixtureIdentity(t, h.db)
	if _, err := identityServiceMember(h, userID, domain.Owner); err != nil {
		t.Fatal(err)
	}
	other, err := h.serverAuth().Authenticate(ctx, otherCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	_, installState = h.manifestToInstall(otherCookie, other, h.org)
	if got := result(t, h.get("/auth/github/install/callback?installation_id="+inst+"&state="+url.QueryEscape(installState), h.cookie)); got.Get("github_result") != "expired" {
		t.Fatalf("wrong user session accepted: %v", got)
	}
	state = h.authorize(location(t, h.get("/auth/github/install/callback?installation_id="+inst+"&state="+url.QueryEscape(installState), otherCookie)))
	if _, err := identityServiceMember(h, userID, domain.Viewer); err != nil {
		t.Fatal(err)
	}
	if got := result(t, h.get("/auth/github/oauth/callback?code=alice&state="+url.QueryEscape(state), otherCookie)); got.Get("github_reason") != "owner_required" {
		t.Fatalf("role loss: %v", got)
	}
	if n := h.setupCount(); n != 0 {
		t.Fatalf("role loss setup must be wiped, found %d", n)
	}
}

func TestGitHubHostedClaimWebhookAndPublicCreate(t *testing.T) {
	h := newGitHubHarness(t, "hosted")
	ctx := context.Background()
	inst := strconv.FormatInt(h.fake.instID, 10)
	h.fake.visible["tok-alice"] = []map[string]any{install(h.fake.instID, h.fake.hostedID, 800, "acme", "Organization")}
	h.fake.visible["tok-mallory"] = h.fake.visible["tok-alice"]
	connect := func(cookie *http.Cookie, session auth.Session, org, code string) url.Values {
		install := location(t, h.get(h.start(cookie, session, org), cookie))
		if !strings.HasPrefix(install.String(), h.origin+"/apps/reforge-hosted/installations/new") {
			t.Fatalf("hosted install %s", install)
		}
		spoof := location(t, h.get("/auth/github/install/callback?installation_id="+inst+"&state="+url.QueryEscape(install.Query().Get("state")), cookie))
		return result(t, h.get("/auth/github/oauth/callback?code="+code+"&state="+url.QueryEscape(h.authorize(spoof)), cookie))
	}
	if got := connect(h.cookie, h.owner, h.org, "mallory"); got.Get("github_reason") != "not_owner" {
		t.Fatalf("org member accepted: %v", got)
	}
	got := connect(h.cookie, h.owner, h.org, "alice")
	id := got.Get("connection")
	if got.Get("github_result") != "connected" {
		t.Fatalf("hosted connect: %v", got)
	}
	err := h.db.Tenant(ctx, h.org, "", func(tx pgx.Tx) error {
		r, err := h.conns.ResolveTx(ctx, tx, h.org, id, "")
		if err != nil {
			return err
		}
		if !strings.Contains(r.Secret, "PRIVATE KEY") || r.Connection.Settings.AuthKind != connections.PlatformAuthKind || r.Connection.SecretID != nil {
			t.Fatal("platform key not mounted at resolve")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if body := h.get("/api/v1/orgs/"+h.org+"/connections/"+id, h.cookie).Body.String(); strings.Contains(body, "PRIVATE KEY") {
		t.Fatal("platform key disclosed")
	}

	userID, otherCookie := fixtureIdentity(t, h.db)
	otherOrg := domain.NewID()
	err = h.db.Tenant(ctx, otherOrg, userID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Tenant B')`, otherOrg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, otherOrg, userID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := h.serverAuth().Authenticate(ctx, otherCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if got := connect(otherCookie, other, otherOrg, "alice"); got.Get("github_reason") != "already_connected" || got.Get("connection") != "" {
		t.Fatalf("cross-tenant claim: %v", got)
	}

	for _, input := range []string{
		`{"provider":"github","name":"x","endpoint":"https://api.github.com","settings":{"auth_kind":"github_app_platform","billing_route":"forge","app_id":"1","installation_id":"7002"}}`,
		`{"provider":"github","name":"x","endpoint":"https://api.github.com","settings":{"auth_kind":"token","billing_route":"forge","managed":"github_hosted"},"secret":"t"}`,
		`{"provider":"github","name":"x","endpoint":"https://api.github.com","settings":{"auth_kind":"github_app","billing_route":"forge","app_id":"1","installation_id":"2"},"secret":"-----BEGIN RSA PRIVATE KEY----- flattened -----END RSA PRIVATE KEY-----"}`,
		`{"provider":"github","name":"x","endpoint":"https://github.com/acme/repo","settings":{"auth_kind":"token","billing_route":"forge"},"secret":"t"}`,
		`{"provider":"github","name":"x","endpoint":"https://api.github.com","settings":{"auth_kind":"token","billing_route":"forge","webhook_pending":true},"secret":"t"}`,
	} {
		r := identityRequest(h.server, "POST", "/api/v1/orgs/"+h.org+"/connections/forges", input, h.cookie, map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8080", "X-CSRF-Token": h.owner.CSRFToken})
		if r.Code != 400 {
			t.Fatalf("public create accepted forged/invalid setup: %d %s", r.Code, input)
		}
		if strings.Contains(input, "acme/repo") && !strings.Contains(r.Body.String(), "repository_endpoint") {
			t.Fatalf("repository URL error not actionable: %s", r.Body.String())
		}
	}

	hook := func(event, body, secret string) int {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		return identityRequest(h.server, "POST", "/hooks/github/app", body, nil, map[string]string{"Content-Type": "application/json", "X-GitHub-Event": event, "X-Hub-Signature-256": "sha256=" + hex.EncodeToString(mac.Sum(nil))}).Code
	}
	deleted := `{"action":"deleted","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.fake.hostedID, 10) + `,"account":{"id":800}}}`
	if code := hook("installation", deleted, "wrong-secret-value-000"); code != 401 {
		t.Fatalf("unsigned webhook: %d", code)
	}
	if code := hook("installation", deleted, string(h.webhook)); code != 202 {
		t.Fatalf("webhook: %d", code)
	}
	c, err := h.conns.Get(ctx, h.owner, h.org, id)
	if err != nil || c.State != "revoked" {
		t.Fatalf("uninstall must revoke: %v %s", err, c.State)
	}
	if got := connect(otherCookie, other, otherOrg, "alice"); got.Get("github_result") != "connected" {
		t.Fatalf("released installation not claimable: %v", got)
	}
}

func (h *ghHarness) serverAuth() *auth.Service { return h.server.Auth }

func identityServiceMember(h *ghHarness, userID string, role domain.Role) (auth.Membership, error) {
	var version int64
	_ = h.db.Tenant(context.Background(), h.org, h.owner.User.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT version FROM memberships WHERE org_id=$1 AND user_id=$2`, h.org, userID).Scan(&version)
	})
	return h.server.Auth.PutMember(context.Background(), h.owner, h.org, userID, auth.Membership{Role: role, AllRepositories: true}, version, "test")
}

func writeFile(path, value string) error { return os.WriteFile(path, []byte(value), 0o600) }

func configFiles(dir string, id int64) config.GitHubAppFiles {
	return config.GitHubAppFiles{AppID: strconv.FormatInt(id, 10), Slug: "reforge-hosted", ClientID: "Iv1.hosted", ClientSecretFile: dir + "/client", PrivateKeyFile: dir + "/key", WebhookFile: dir + "/hook"}
}
