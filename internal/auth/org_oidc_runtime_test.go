package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/jackc/pgx/v5"
	"reforge/internal/domain"
	"reforge/internal/secrets"
	"reforge/internal/store"
)

func TestOrgOIDCRuntimeLoginCallback(t *testing.T) {
	databaseURL := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REFORGE_TEST_DATABASE_URL must point to a migrated disposable PostgreSQL database using a restricted runtime role")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal("open test database")
	}
	defer db.Close()
	const clientSecret = "runtime-test-client-secret"
	issuer := ""
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("create local signing key")
	}
	var mu sync.Mutex
	var authorization struct{ state, nonce, challenge string }
	subject := "existing-subject"
	tokenCase := ""
	idp := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/tenant/.well-known/openid-configuration":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"issuer":"` + issuer + `","authorization_endpoint":"` + issuer + `/authorize","token_endpoint":"` + issuer + `/token","jwks_uri":"` + issuer + `/keys"}`))
		case "/tenant/authorize":
			_ = request.ParseForm()
			mu.Lock()
			authorization.state = request.Form.Get("state")
			authorization.nonce = request.Form.Get("nonce")
			authorization.challenge = request.Form.Get("code_challenge")
			mu.Unlock()
			writer.WriteHeader(http.StatusOK)
		case "/tenant/token":
			if err := request.ParseForm(); err != nil || request.Form.Get("client_secret") != clientSecret || request.Form.Get("code") != "authorization-code" {
				http.Error(writer, "invalid token request", http.StatusUnauthorized)
				return
			}
			mu.Lock()
			nonce := authorization.nonce
			challenge := authorization.challenge
			currentSubject := subject
			currentTokenCase := tokenCase
			mu.Unlock()
			verifier := request.Form.Get("code_verifier")
			digest := sha256.Sum256([]byte(verifier))
			if verifier == "" || base64.RawURLEncoding.EncodeToString(digest[:]) != challenge {
				http.Error(writer, "invalid PKCE verifier", http.StatusUnauthorized)
				return
			}
			tokenIssuer, audience, tokenNonce := issuer, "runtime-client", nonce
			switch currentTokenCase {
			case "wrong-issuer":
				tokenIssuer += "/other"
			case "wrong-audience":
				audience = "another-client"
			case "wrong-nonce":
				tokenNonce += "-mismatch"
			}
			token, signErr := signTestIDToken(privateKey, tokenIssuer, currentSubject, audience, tokenNonce)
			if signErr != nil {
				http.Error(writer, "token signing failed", http.StatusInternalServerError)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "test-access-token", "token_type": "Bearer", "expires_in": 300, "id_token": token})
		case "/tenant/keys":
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &privateKey.PublicKey, KeyID: "runtime-key", Algorithm: string(jose.RS256), Use: "sig"}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer idp.Close()
	issuer = idp.URL + "/tenant"

	const bootstrapToken = "local-test-bootstrap-token-with-over-32-bytes"
	cfg := Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, FixtureAuth: true, ListenAddress: "127.0.0.1:8080", BootstrapToken: bootstrapToken, BootstrapExpiresAt: time.Now().Add(time.Hour)}
	identity, err := New(ctx, db, cfg)
	if err != nil {
		t.Fatal("create fixture identity service")
	}
	vault, err := secrets.New("runtime-test", map[string]string{"runtime-test": base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal("create test vault")
	}
	identity.SetOrgOIDCVault(vault)
	loginResponse := httptest.NewRecorder()
	if _, err = identity.Login(ctx, loginResponse); err != nil {
		t.Fatal("create installation recovery session")
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("fixture session cookie missing")
	}
	owner, err := identity.Authenticate(ctx, cookies[0].Value)
	if err != nil || len(owner.Organisations) != 1 {
		t.Fatal("authenticate fixture owner")
	}
	orgID, otherOrgID := string(domain.NewID()), string(domain.NewID())
	user, err := identity.upsertUser(ctx, issuer, subject, "Known member", "known@example.test")
	if err != nil {
		t.Fatal("create already-enrolled OIDC identity")
	}
	for _, organization := range []struct{ id, name string }{{orgID, "OIDC runtime"}, {otherOrgID, "Other organisation"}} {
		if err = db.Tenant(ctx, organization.id, owner.User.ID, func(tx pgx.Tx) error {
			if _, insertErr := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,$2)`, organization.id, organization.name); insertErr != nil {
				return insertErr
			}
			if _, insertErr := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, organization.id, owner.User.ID); insertErr != nil {
				return insertErr
			}
			_, insertErr := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'viewer',true)`, organization.id, user.ID)
			return insertErr
		}); err != nil {
			t.Fatal("create isolated org and existing memberships")
		}
	}
	created, err := identity.PutOrgOIDC(ctx, owner, orgID, OrgOIDCInput{Issuer: issuer, ClientID: "runtime-client", ClientSecret: clientSecret}, 0, "runtime-configure")
	if err != nil {
		t.Fatal("configure local issuer")
	}
	verified, err := identity.ProbeOrgOIDC(ctx, owner, orgID, created.Version, "runtime-probe")
	if err != nil || !verified.Verified {
		t.Fatal("probe local issuer")
	}
	if err = identity.ActivateOrgOIDC(ctx, owner, orgID, verified.Version, "runtime-activate"); err != nil {
		t.Fatal("activate verified issuer")
	}

	startFor := func(loginOrgID, currentSubject string) (string, *http.Cookie) {
		t.Helper()
		mu.Lock()
		subject = currentSubject
		tokenCase = ""
		mu.Unlock()
		writer := httptest.NewRecorder()
		location, loginErr := identity.Login(ctx, writer, loginOrgID)
		if loginErr != nil {
			t.Fatalf("start org login: %v", loginErr)
		}
		authorizeURL, parseErr := url.Parse(location)
		if parseErr != nil {
			t.Fatal("parse authorization URL")
		}
		if authorizeURL.Scheme != "http" || authorizeURL.Host != strings.TrimPrefix(idp.URL, "http://") || authorizeURL.Path != "/tenant/authorize" || authorizeURL.Query().Get("code_challenge_method") != "S256" {
			t.Fatal("authorization request escaped validated issuer or omitted PKCE")
		}
		response, getErr := http.Get(location)
		if getErr != nil {
			t.Fatal("request local authorization endpoint")
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatal("local authorization endpoint rejected request")
		}
		var browser *http.Cookie
		for _, cookie := range writer.Result().Cookies() {
			if cookie.Name == identity.oidcCookieName() {
				browser = cookie
			}
		}
		if browser == nil {
			t.Fatal("OIDC browser binding cookie missing")
		}
		return authorizeURL.Query().Get("state"), browser
	}
	start := func(currentSubject string) (string, *http.Cookie) {
		t.Helper()
		return startFor(orgID, currentSubject)
	}
	callback := func(service *Service, state string, browser *http.Cookie) (*httptest.ResponseRecorder, error) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/auth/callback?state="+url.QueryEscape(state)+"&code=authorization-code", nil)
		request.AddCookie(browser)
		writer := httptest.NewRecorder()
		return writer, service.Callback(ctx, writer, request)
	}

	state, browser := start("existing-subject")
	restarted, err := New(ctx, db, cfg)
	if err != nil {
		t.Fatal("restart identity service")
	}
	restarted.SetOrgOIDCVault(vault)
	callbackResponse, err := callback(restarted, state, browser)
	if err != nil {
		t.Fatalf("complete org OIDC callback after restart: %v", err)
	}
	var sessionToken string
	for _, cookie := range callbackResponse.Result().Cookies() {
		if cookie.Name == restarted.CookieName() {
			sessionToken = cookie.Value
		}
	}
	if sessionToken == "" {
		t.Fatal("org-bound session cookie missing")
	}
	session, err := restarted.Authenticate(ctx, sessionToken)
	if err != nil || session.OrganizationID != orgID || len(session.Organisations) != 1 || session.Organisations[0].ID != orgID {
		t.Fatal("session was not bound to its authenticated organisation")
	}
	if _, err = restarted.ResolveActor(ctx, session, otherOrgID); !errors.Is(err, ErrForbidden) {
		t.Fatal("org-bound session switched to another existing membership")
	}
	if _, err = callback(restarted, state, browser); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("callback replay was accepted")
	}

	for _, invalidCase := range []string{"wrong-nonce", "wrong-audience", "wrong-issuer"} {
		state, browser = start("existing-subject")
		mu.Lock()
		tokenCase = invalidCase
		mu.Unlock()
		invalidResponse, callbackErr := callback(restarted, state, browser)
		if !errors.Is(callbackErr, ErrUnauthenticated) {
			t.Fatalf("ID token with %s was accepted: %v", invalidCase, callbackErr)
		}
		for _, cookie := range invalidResponse.Result().Cookies() {
			if cookie.Name == restarted.CookieName() {
				t.Fatalf("ID token with %s issued an application session", invalidCase)
			}
		}
	}

	state, browser = start("not-enrolled")
	if _, err = callback(restarted, state, browser); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("OIDC identity without existing membership was enrolled")
	}
	var unexpectedUsers int
	if err = identity.identity(ctx, "", map[string]string{"reforge.issuer": issuer, "reforge.subject": "not-enrolled"}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE issuer=$1 AND subject='not-enrolled'`, issuer).Scan(&unexpectedUsers)
	}); err != nil || unexpectedUsers != 0 {
		t.Fatal("uninvited OIDC identity created an account")
	}

	state, browser = start("existing-subject")
	if err = identity.identity(ctx, "", map[string]string{"reforge.login_hash": digest(state)}, func(tx pgx.Tx) error {
		_, updateErr := tx.Exec(ctx, `UPDATE oidc_logins SET expires_at=now()-interval '1 second' WHERE state_hash=$1`, digest(state))
		return updateErr
	}); err != nil {
		t.Fatal("expire pending OIDC state")
	}
	if _, err = callback(restarted, state, browser); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("expired OIDC state was accepted")
	}

	state, browser = start("existing-subject")
	latest, err := restarted.OrgOIDC(ctx, owner, orgID)
	if err != nil {
		t.Fatal("read current org identity configuration")
	}
	if _, err = restarted.PutOrgOIDC(ctx, owner, orgID, OrgOIDCInput{Issuer: issuer, ClientID: "rotated-client"}, latest.Version, "runtime-rotate"); err != nil {
		t.Fatal("rotate OIDC config while callback pending")
	}
	if _, err = callback(restarted, state, browser); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("callback used rotated configuration")
	}
	if _, err = restarted.Authenticate(ctx, sessionToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("session remained valid after its OIDC config rotated")
	}
	if _, err = restarted.ResolveActor(ctx, session, orgID); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("existing actor remained valid after its OIDC config rotated")
	}

	secondConfig, err := restarted.PutOrgOIDC(ctx, owner, otherOrgID, OrgOIDCInput{Issuer: issuer, ClientID: "runtime-client", ClientSecret: clientSecret}, 0, "runtime-second-configure")
	if err != nil {
		t.Fatal("configure second org issuer")
	}
	verified, err = restarted.ProbeOrgOIDC(ctx, owner, otherOrgID, secondConfig.Version, "runtime-second-probe")
	if err != nil || !verified.Verified {
		t.Fatal("verify second org issuer")
	}
	if err = restarted.ActivateOrgOIDC(ctx, owner, otherOrgID, verified.Version, "runtime-second-activate"); err != nil {
		t.Fatal("activate second org issuer")
	}
	state, browser = startFor(otherOrgID, "existing-subject")
	activeResponse, err := callback(restarted, state, browser)
	if err != nil {
		t.Fatal("complete second org login before disable")
	}
	var activeSessionToken string
	for _, cookie := range activeResponse.Result().Cookies() {
		if cookie.Name == restarted.CookieName() {
			activeSessionToken = cookie.Value
		}
	}
	if activeSessionToken == "" {
		t.Fatal("pre-disable org session cookie missing")
	}
	state, browser = startFor(otherOrgID, "existing-subject")
	secondConfig, err = restarted.OrgOIDC(ctx, owner, otherOrgID)
	if err != nil {
		t.Fatal("read second org config before disable")
	}
	if _, err = restarted.DisableOrgOIDC(ctx, owner, otherOrgID, secondConfig.Version, "runtime-second-disable"); err != nil {
		t.Fatal("disable active issuer")
	}
	if _, err = callback(restarted, state, browser); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("pending callback completed after provider disable")
	}
	if _, err = restarted.Authenticate(ctx, activeSessionToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("org session remained valid after provider disable")
	}

}

func signTestIDToken(key *rsa.PrivateKey, issuer, subject, audience, nonce string) (string, error) {
	claims, err := json.Marshal(map[string]any{
		"iss":   issuer,
		"sub":   subject,
		"aud":   audience,
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Add(-time.Second).Unix(),
		"nonce": nonce,
	})
	if err != nil {
		return "", err
	}
	options := (&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), "runtime-key")
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, options)
	if err != nil {
		return "", err
	}
	object, err := signer.Sign(claims)
	if err != nil {
		return "", err
	}
	return object.CompactSerialize()
}
