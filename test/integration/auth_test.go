package integration

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/config"
	"reforge/internal/domain"
	"reforge/internal/httpapi"
	"reforge/internal/store"
)

func authDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(context.Background(), testDatabase(t, "REFORGE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db
}
func authConfig() auth.Config {
	return auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, FixtureAuth: true, ListenAddress: "127.0.0.1:8080"}
}
func identityServer(t *testing.T, db *store.Store, cfg auth.Config) (*auth.Service, *httpapi.Server) {
	t.Helper()
	svc, err := auth.New(context.Background(), db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	server := httpapi.New(config.Config{PublicURL: cfg.PublicURL, Development: cfg.Development}, db)
	server.RegisterIdentity(svc)
	return svc, server
}
func identityRequest(server *httpapi.Server, method, path, body string, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(body))
	if cookie != nil {
		request.AddCookie(cookie)
	}
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	response := httptest.NewRecorder()
	server.Router.ServeHTTP(response, request)
	return response
}
func identityLogin(t *testing.T, svc *auth.Service, server *httpapi.Server) (*http.Cookie, auth.Session) {
	t.Helper()
	response := identityRequest(server, "GET", "/auth/login", "", nil, nil)
	if response.Code != 302 {
		t.Fatalf("login: %d %s", response.Code, response.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range response.Result().Cookies() {
		if c.Name == svc.CookieName() {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("session cookie absent")
	}
	session, err := svc.Authenticate(context.Background(), cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	return cookie, session
}
func fixtureIdentity(t *testing.T, db *store.Store) (string, *http.Cookie) {
	t.Helper()
	ctx := context.Background()
	userID, sessionID := domain.NewID(), domain.NewID()
	raw := make([]byte, 32)
	rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	err := db.Identity(ctx, userID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'fixture',$1::text,'Scoped user','scoped@example.test')`, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,'fixture-csrf',now()+interval '1 hour')`, sessionID, userID, hex.EncodeToString(hash[:]))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return userID, &http.Cookie{Name: "reforge_development_session", Value: token}
}

func TestIdentityScopeCSRFAndRevocation(t *testing.T) {
	db := authDB(t)
	ctx := context.Background()
	svc, server := identityServer(t, db, authConfig())
	cookie, owner := identityLogin(t, svc, server)
	orgID := owner.Organisations[0].ID
	if got := identityRequest(server, "GET", "/api/v1/session", "", nil, nil); got.Code != 401 {
		t.Fatalf("unauthenticated session: %d", got.Code)
	}
	badHost := httptest.NewRequest("GET", "http://attacker.test/auth/login", nil)
	response := httptest.NewRecorder()
	server.Router.ServeHTTP(response, badHost)
	if response.Code != 403 {
		t.Fatal("Host bypass")
	}
	for _, headers := range []map[string]string{nil, {"Origin": "http://attacker.test", "X-CSRF-Token": owner.CSRFToken}, {"Origin": "http://127.0.0.1:8080", "X-CSRF-Token": "wrong"}} {
		if got := identityRequest(server, "POST", "/auth/logout", "", cookie, headers); got.Code != 403 {
			t.Fatalf("CSRF bypass: %d", got.Code)
		}
	}
	userID, scopedCookie := fixtureIdentity(t, db)
	repoA, repoB, otherOrg, otherRepo := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	for _, item := range []struct {
		org string
		ids []string
	}{{orgID, []string{repoA, repoB}}, {otherOrg, []string{otherRepo}}} {
		err := db.Tenant(ctx, item.org, owner.User.ID, func(tx pgx.Tx) error {
			if item.org != orgID {
				if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Other tenant')`, item.org); err != nil {
					return err
				}
			}
			for _, id := range item.ids {
				if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Private repository')`, item.org, id); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	teamID := domain.NewID()
	team, err := svc.PutTeam(ctx, owner, orgID, teamID, auth.Team{Name: "Scoped " + teamID, RepositoryIDs: []string{repoA}}, 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	member, err := svc.PutMember(ctx, owner, orgID, userID, auth.Membership{Role: domain.Viewer, TeamIDs: []string{teamID}}, 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := svc.Authenticate(ctx, scopedCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.RequireRepository(ctx, scoped, orgID, repoA); err != nil {
		t.Fatal(err)
	}
	for _, item := range [][2]string{{orgID, repoB}, {otherOrg, otherRepo}, {orgID, otherRepo}} {
		if _, err = svc.RequireRepository(ctx, scoped, item[0], item[1]); err != auth.ErrForbidden {
			t.Fatalf("scope bypass: %v", err)
		}
	}
	if _, err = svc.ResolveRepository(ctx, scoped, orgID, domain.NewID(), func(ctx context.Context, tx pgx.Tx, org, id string) (string, error) { return repoB, nil }); err != auth.ErrForbidden {
		t.Fatal("child scope bypass")
	}
	if _, err = svc.PutMember(ctx, owner, orgID, userID, auth.Membership{Role: domain.Viewer, RepositoryIDs: []string{otherRepo}}, member.Version, "test"); err != auth.ErrInvalid {
		t.Fatalf("cross tenant binding: %v", err)
	}
	if _, err = svc.PutTeam(ctx, owner, orgID, teamID, team, 0, "test"); err != auth.ErrConflict {
		t.Fatal("stale team version accepted")
	}
	if _, err = svc.PutMember(ctx, owner, orgID, owner.User.ID, auth.Membership{Role: domain.Viewer}, owner.Memberships[0].Version, "test"); err != auth.ErrConflict {
		t.Fatalf("last-owner demotion: %v", err)
	}
	if err = svc.DeleteMember(ctx, owner, orgID, owner.User.ID, owner.Memberships[0].Version, "test"); err != auth.ErrConflict {
		t.Fatal("last-owner deletion accepted")
	}
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8080", "X-CSRF-Token": owner.CSRFToken, "If-Match": "\"0\""}
	teamBody := fmt.Sprintf(`{"name":"API team %s","repository_ids":[%q]}`, domain.NewID(), repoB)
	if got := identityRequest(server, "PUT", "/api/v1/orgs/"+orgID+"/teams/"+domain.NewID(), teamBody, cookie, headers); got.Code != 200 {
		t.Fatalf("team API: %d %s", got.Code, got.Body.String())
	}
	if got := identityRequest(server, "GET", "/api/v1/orgs/"+otherOrg+"/teams", "", scopedCookie, nil); got.Code != 403 {
		t.Fatal("team tenant enumeration")
	}
	teams, err := svc.Teams(ctx, scoped, orgID, 50, "")
	if err != nil || len(teams.Items) != 1 || teams.Items[0].ID != teamID {
		t.Fatalf("team enumeration: %+v %v", teams, err)
	}
	members, err := svc.Members(ctx, owner, orgID, 200, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range members.Items {
		if m.UserID == userID {
			if len(m.RepositoryIDs) != 0 {
				t.Fatal("inherited grants leaked into editable direct bindings")
			}
			m.Role = domain.Reviewer
			member, err = svc.PutMember(ctx, owner, orgID, userID, m, m.Version, "roundtrip")
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = svc.DeleteTeam(ctx, owner, orgID, teamID, team.Version, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.RequireRepository(ctx, scoped, orgID, repoA); err != auth.ErrForbidden {
		t.Fatal("membership roundtrip preserved removed team grant")
	}
	if err = svc.DeleteMember(ctx, owner, orgID, userID, member.Version, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.RequireRepository(ctx, scoped, orgID, repoA); err != auth.ErrForbidden {
		t.Fatal("membership revocation not observed")
	}
	var unscoped int
	for _, table := range []string{"users", "sessions", "memberships", "teams", "repositories", "oidc_logins", "bootstrap"} {
		if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&unscoped); err != nil || unscoped != 0 {
			t.Fatalf("global identity/tenant leak %s: %d %v", table, unscoped, err)
		}
	}
	if got := identityRequest(server, "POST", "/auth/logout", "", cookie, headers); got.Code != 204 {
		t.Fatal("logout failed")
	}
	if _, err = svc.Authenticate(ctx, cookie.Value); err != auth.ErrUnauthenticated {
		t.Fatal("revoked cookie accepted")
	}
	if _, err = svc.ResolveActor(ctx, owner, orgID); err != auth.ErrUnauthenticated {
		t.Fatal("saved session bypasses revocation")
	}
}

func TestIdentityConcurrentLastOwner(t *testing.T) {
	db := authDB(t)
	ctx := context.Background()
	svc, server := identityServer(t, db, authConfig())
	_, owner := identityLogin(t, svc, server)
	orgID := domain.NewID()
	userID, cookie := fixtureIdentity(t, db)
	if err := db.Tenant(ctx, orgID, owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Concurrent owners')`, orgID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true),($1,$3,'owner',true)`, orgID, owner.User.ID, userID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	second, err := svc.Authenticate(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, session := range []auth.Session{owner, second} {
		wg.Add(1)
		go func(session auth.Session) {
			defer wg.Done()
			results <- svc.DeleteMember(ctx, session, orgID, session.User.ID, 1, "concurrent")
		}(session)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if err == auth.ErrConflict {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("last owner concurrency: %d successes %d conflicts", success, conflict)
	}
}

func TestIdentityOIDCAndBootstrap(t *testing.T) {
	db := authDB(t)
	ctx := context.Background()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer, nonce, challenge string
	var wrongNonce bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "fixture", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
		case "/token":
			r.ParseForm()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(hash[:]) != challenge || r.Form.Get("code") != "valid-code" {
				w.WriteHeader(400)
				return
			}
			tokenNonce := nonce
			if wrongNonce {
				tokenNonce = "wrong"
			}
			payload, _ := json.Marshal(map[string]any{"iss": issuer, "sub": "fixture-subject", "aud": "reforge-test", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": tokenNonce, "name": "OIDC owner", "email": "owner@example.test"})
			header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"fixture"}`))
			unsigned := header + "." + base64.RawURLEncoding.EncodeToString(payload)
			sum := sha256.Sum256([]byte(unsigned))
			signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-only", "token_type": "Bearer", "expires_in": 3600, "id_token": unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)})
		default:
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	issuer = provider.URL
	cfg := authConfig()
	cfg.FixtureAuth = false
	cfg.OIDCIssuer = issuer
	cfg.OIDCClientID = "reforge-test"
	svc, server := identityServer(t, db, cfg)
	begin := func() (string, *http.Cookie) {
		response := identityRequest(server, "GET", "/auth/login", "", nil, nil)
		if response.Code != 302 {
			t.Fatalf("OIDC begin: %d", response.Code)
		}
		location, err := url.Parse(response.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		nonce = location.Query().Get("nonce")
		challenge = location.Query().Get("code_challenge")
		if nonce == "" || challenge == "" || location.Query().Get("code_challenge_method") != "S256" {
			t.Fatal("OIDC binding absent")
		}
		return "/auth/callback?code=valid-code&state=" + location.Query().Get("state"), response.Result().Cookies()[0]
	}
	path, browser := begin()
	if response := identityRequest(server, "GET", path, "", nil, nil); response.Code != 401 {
		t.Fatal("unbound callback accepted")
	}
	svc, server = identityServer(t, db, cfg)
	response := identityRequest(server, "GET", path, "", browser, nil)
	if response.Code != 302 {
		t.Fatalf("OIDC callback: %d %s", response.Code, response.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range response.Result().Cookies() {
		if c.Name == svc.CookieName() {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("OIDC session missing")
	}
	if response := identityRequest(server, "GET", path, "", browser, nil); response.Code != 401 {
		t.Fatal("state replay accepted")
	}
	session, err := svc.Authenticate(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	wrongNonce = true
	path, browser = begin()
	if response := identityRequest(server, "GET", path, "", browser, nil); response.Code != 401 {
		t.Fatal("nonce mismatch accepted")
	}
	token := strings.Repeat("b", 43)
	hash := sha256.Sum256([]byte(token))
	bootstrapHash := hex.EncodeToString(hash[:])
	bootstrapSQL := func(query string, args ...any) error {
		return db.Identity(ctx, session.User.ID, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('reforge.bootstrap_hash',$1,true)`, bootstrapHash); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, query, args...)
			return err
		})
	}
	if err = bootstrapSQL(`DELETE FROM bootstrap`); err != nil {
		t.Fatal(err)
	}
	defer bootstrapSQL(`DELETE FROM bootstrap`)
	cfg.BootstrapToken = token
	cfg.BootstrapExpiresAt = time.Now().Add(time.Hour)
	svc, server = identityServer(t, db, cfg)
	headers := map[string]string{"Content-Type": "application/json", "Origin": cfg.PublicURL, "X-CSRF-Token": session.CSRFToken}
	if response := identityRequest(server, "POST", "/auth/bootstrap", `{"token":"wrong","name":"Bootstrap organisation"}`, cookie, headers); response.Code != 403 {
		t.Fatal("wrong bootstrap accepted")
	}
	body := fmt.Sprintf(`{"token":%q,"name":"Bootstrap organisation"}`, token)
	if response := identityRequest(server, "POST", "/auth/bootstrap", body, cookie, headers); response.Code != 201 {
		t.Fatalf("bootstrap: %d %s", response.Code, response.Body.String())
	}
	if response := identityRequest(server, "POST", "/auth/bootstrap", body, cookie, headers); response.Code != 403 {
		t.Fatal("bootstrap reuse accepted")
	}
	if err = bootstrapSQL(`UPDATE bootstrap SET consumed_at=NULL,expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if response := identityRequest(server, "POST", "/auth/bootstrap", body, cookie, headers); response.Code != 403 {
		t.Fatal("expired bootstrap accepted")
	}
}

func TestIdentityQueuedMutationRechecksMembership(t *testing.T) {
	db := authDB(t)
	ctx := context.Background()
	svc, server := identityServer(t, db, authConfig())
	_, owner := identityLogin(t, svc, server)
	orgID := owner.Organisations[0].ID
	userID, cookie := fixtureIdentity(t, db)
	if _, err := svc.PutMember(ctx, owner, orgID, userID, auth.Membership{Role: domain.Admin, AllRepositories: true}, 0, "test"); err != nil {
		t.Fatal(err)
	}
	session, err := svc.Authenticate(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	teamID := domain.NewID()
	team, err := svc.PutTeam(ctx, owner, orgID, teamID, auth.Team{Name: "Queued " + teamID}, 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	err = db.Tenant(ctx, orgID, owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM organisations WHERE id=$1 FOR UPDATE`, orgID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM teams WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, teamID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM memberships WHERE org_id=$1 AND user_id=$2`, orgID, userID); err != nil {
			return err
		}
		go func() {
			_, err := svc.PutTeam(ctx, session, orgID, teamID, team, team.Version, "queued")
			result <- err
		}()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			var waiting bool
			if err := db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND usename=current_user AND wait_event_type='Lock' AND pid<>pg_backend_pid())`).Scan(&waiting); err != nil {
				return err
			}
			if waiting {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
		return fmt.Errorf("mutation did not wait for revocation transaction")
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != auth.ErrForbidden {
			t.Fatalf("queued mutation retained revoked role: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("queued mutation did not complete")
	}
}
