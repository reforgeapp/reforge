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
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/secrets"
	"github.com/reforgeapp/reforge/internal/store"
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
	tokenEmail := "invite@example.test"
	tokenEmailVerified := true
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
			currentEmail := tokenEmail
			currentEmailVerified := tokenEmailVerified
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
			token, signErr := signTestIDToken(privateKey, tokenIssuer, currentSubject, audience, tokenNonce, currentEmail, currentEmailVerified)
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

	startInvitation := func(service *Service, token, currentSubject, currentEmail string, verified bool) (string, *http.Cookie) {
		t.Helper()
		mu.Lock()
		subject = currentSubject
		tokenEmail = currentEmail
		tokenEmailVerified = verified
		tokenCase = ""
		mu.Unlock()
		writer := httptest.NewRecorder()
		location, beginErr := service.BeginOrgOIDCInvitation(ctx, writer, token)
		if beginErr != nil {
			t.Fatalf("begin invited login: %v", beginErr)
		}
		authorizeURL, parseErr := url.Parse(location)
		if parseErr != nil || authorizeURL.Query().Get("code_challenge_method") != "S256" {
			t.Fatal("invitation authorization omitted PKCE")
		}
		response, getErr := http.Get(location)
		if getErr != nil {
			t.Fatal("request local invitation authorization endpoint")
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatal("local invitation authorization endpoint rejected request")
		}
		var browser *http.Cookie
		for _, cookie := range writer.Result().Cookies() {
			if cookie.Name == service.oidcCookieName() {
				browser = cookie
			}
		}
		if browser == nil {
			t.Fatal("invitation browser binding cookie missing")
		}
		return authorizeURL.Query().Get("state"), browser
	}

	viewerState, viewerBrowser := start("existing-subject")
	viewerResponse, viewerErr := callback(identity, viewerState, viewerBrowser)
	if viewerErr != nil {
		t.Fatal("authenticate existing viewer")
	}
	var viewerToken string
	for _, cookie := range viewerResponse.Result().Cookies() {
		if cookie.Name == identity.CookieName() {
			viewerToken = cookie.Value
		}
	}
	viewerSession, err := identity.Authenticate(ctx, viewerToken)
	if err != nil {
		t.Fatal("load viewer session")
	}
	if _, err = identity.CreateOrgOIDCInvitation(ctx, viewerSession, orgID, OrgOIDCInvitationInput{Email: "forbidden@example.test", Role: domain.Viewer, ExpiresAt: time.Now().Add(24 * time.Hour)}, "runtime-invite-forbidden"); !errors.Is(err, ErrForbidden) {
		t.Fatal("non-owner created organisation invitation")
	}

	invitation, err := identity.CreateOrgOIDCInvitation(ctx, owner, orgID, OrgOIDCInvitationInput{
		Email: " New.Member@Example.Test ", Role: domain.Reviewer, ExpiresAt: time.Now().Add(24 * time.Hour),
	}, "runtime-invite-create")
	if err != nil || invitation.Email != "new.member@example.test" || invitation.RedemptionURL == "" || invitation.CreatedAt.IsZero() {
		t.Fatal("create normalized invitation with database timestamp")
	}
	inviteURL, err := url.Parse(invitation.RedemptionURL)
	if err != nil || inviteURL.RawQuery != "" {
		t.Fatal("invitation token appeared in HTTP URL")
	}
	inviteToken := mustInviteToken(t, invitation.RedemptionURL)
	if len(inviteToken) != 43 {
		t.Fatal("invitation link did not contain a high-entropy token")
	}
	if err = db.Tenant(ctx, orgID, owner.User.ID, func(tx pgx.Tx) error {
		var stored string
		if scanErr := tx.QueryRow(ctx, "SELECT token_hash FROM org_oidc_invitations WHERE org_id=$1 AND id=$2", orgID, invitation.ID).Scan(&stored); scanErr != nil {
			return scanErr
		}
		if stored == inviteToken || stored != digest(inviteToken) {
			t.Fatal("invitation token was not stored as a hash")
		}
		return nil
	}); err != nil {
		t.Fatal("read invitation through tenant role")
	}
	inviteState, inviteBrowser := startInvitation(identity, inviteToken, "first-time-subject", invitation.Email, true)
	inviteResponse, err := callback(identity, inviteState, inviteBrowser)
	if err != nil {
		t.Fatalf("redeem verified invitation: %v", err)
	}
	var inviteSessionToken string
	for _, cookie := range inviteResponse.Result().Cookies() {
		if cookie.Name == identity.CookieName() {
			inviteSessionToken = cookie.Value
		}
	}
	inviteSession, err := identity.Authenticate(ctx, inviteSessionToken)
	if err != nil || inviteSession.OrganizationID != orgID || len(inviteSession.Memberships) != 1 || inviteSession.Memberships[0].Role != domain.Reviewer || inviteSession.Memberships[0].AllRepositories {
		t.Fatal("invited login did not create a tenant-bound session with invited role")
	}
	ownerInvitation, err := identity.CreateOrgOIDCInvitation(ctx, owner, orgID, OrgOIDCInvitationInput{Email: "invited-owner@example.test", Role: domain.Owner, ExpiresAt: time.Now().Add(24 * time.Hour)}, "runtime-invite-owner")
	if err != nil {
		t.Fatal("create owner invitation")
	}
	firstPage, err := identity.OrgOIDCInvitations(ctx, owner, orgID, 1, "")
	if err != nil || firstPage.Complete || len(firstPage.Items) != 1 || firstPage.NextCursor != firstPage.Items[0].ID {
		t.Fatal("first invitation page omitted its continuation")
	}
	secondPage, err := identity.OrgOIDCInvitations(ctx, owner, orgID, 1, firstPage.NextCursor)
	if err != nil || !secondPage.Complete || len(secondPage.Items) != 1 || secondPage.Items[0].ID == firstPage.Items[0].ID {
		t.Fatal("second invitation page did not continue")
	}
	ownerState, ownerBrowser := startInvitation(identity, mustInviteToken(t, ownerInvitation.RedemptionURL), "invited-owner-subject", "invited-owner@example.test", true)
	ownerResponse, err := callback(identity, ownerState, ownerBrowser)
	if err != nil {
		t.Fatal("redeem owner invitation")
	}
	var ownerSessionToken string
	for _, cookie := range ownerResponse.Result().Cookies() {
		if cookie.Name == identity.CookieName() {
			ownerSessionToken = cookie.Value
		}
	}
	invitedOwnerSession, err := identity.Authenticate(ctx, ownerSessionToken)
	if err != nil || len(invitedOwnerSession.Memberships) != 1 || invitedOwnerSession.Memberships[0].Role != domain.Owner || !invitedOwnerSession.Memberships[0].AllRepositories {
		t.Fatal("invited owner did not receive owner repository scope")
	}
	invitedOwnerActor, err := identity.ResolveActor(ctx, invitedOwnerSession, orgID)
	if err != nil || !CanReadRepository(invitedOwnerActor, string(domain.NewID())) {
		t.Fatal("invited owner could not read organisation repositories")
	}
	if _, err = identity.BeginOrgOIDCInvitation(ctx, httptest.NewRecorder(), inviteToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("redeemed invite could be started again")
	}

	mismatch, err := identity.CreateOrgOIDCInvitation(ctx, owner, orgID, OrgOIDCInvitationInput{Email: "expected@example.test", Role: domain.Viewer, ExpiresAt: time.Now().Add(24 * time.Hour)}, "runtime-invite-mismatch")
	if err != nil {
		t.Fatal("create email-mismatch invitation")
	}
	mismatchToken := mustInviteToken(t, mismatch.RedemptionURL)
	mismatchState, mismatchBrowser := startInvitation(identity, mismatchToken, "mismatched-subject", "other@example.test", true)
	var visibleInvitations int
	if err = identity.identity(ctx, "", map[string]string{"reforge.org_id": orgID, "reforge.user_id": owner.User.ID, "reforge.invitation_hash": digest(mismatchToken)}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM org_oidc_invitations WHERE org_id=$1", orgID).Scan(&visibleInvitations)
	}); err != nil || visibleInvitations != 1 {
		t.Fatal("pre-auth invite capability exposed another invitation")
	}
	if _, err = callback(identity, mismatchState, mismatchBrowser); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("verified but mismatched email redeemed invitation")
	}
	assertNoInviteUser(t, db, orgID, issuer, "mismatched-subject")

	unverified, err := identity.CreateOrgOIDCInvitation(ctx, owner, orgID, OrgOIDCInvitationInput{Email: "unverified@example.test", Role: domain.Viewer, ExpiresAt: time.Now().Add(24 * time.Hour)}, "runtime-invite-unverified")
	if err != nil {
		t.Fatal("create unverified-email invitation")
	}
	unverifiedToken := mustInviteToken(t, unverified.RedemptionURL)
	unverifiedState, unverifiedBrowser := startInvitation(identity, unverifiedToken, "unverified-subject", "unverified@example.test", false)
	if _, err = callback(identity, unverifiedState, unverifiedBrowser); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("unverified email redeemed invitation")
	}
	assertNoInviteUser(t, db, orgID, issuer, "unverified-subject")

	expiredInvite, err := identity.CreateOrgOIDCInvitation(ctx, owner, orgID, OrgOIDCInvitationInput{Email: "expired@example.test", Role: domain.Viewer, ExpiresAt: time.Now().Add(24 * time.Hour)}, "runtime-invite-expired")
	if err != nil {
		t.Fatal("create expiring invitation")
	}
	if err = db.Tenant(ctx, orgID, owner.User.ID, func(tx pgx.Tx) error {
		_, updateErr := tx.Exec(ctx, "UPDATE org_oidc_invitations SET expires_at=now()-interval '1 second' WHERE org_id=$1 AND id=$2", orgID, expiredInvite.ID)
		return updateErr
	}); err != nil {
		t.Fatal("expire invitation")
	}
	if _, err = identity.BeginOrgOIDCInvitation(ctx, httptest.NewRecorder(), mustInviteToken(t, expiredInvite.RedemptionURL)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("expired invitation could start login")
	}

	concurrentInvite, err := identity.CreateOrgOIDCInvitation(ctx, owner, orgID, OrgOIDCInvitationInput{Email: "concurrent@example.test", Role: domain.Reviewer, ExpiresAt: time.Now().Add(24 * time.Hour)}, "runtime-invite-concurrent")
	if err != nil {
		t.Fatal("create concurrent invitation")
	}
	var configID string
	if err = identity.identity(ctx, "", map[string]string{"reforge.login_org_id": orgID}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT id::text FROM org_oidc_configs WHERE org_id=$1", orgID).Scan(&configID)
	}); err != nil {
		t.Fatal("read invitation issuer config")
	}
	concurrentAttempt := oidcLoginAttempt{OrgID: orgID, ConfigID: configID, ConfigVersion: verified.Version, Issuer: issuer, InvitationID: concurrentInvite.ID, InvitationHash: digest(mustInviteToken(t, concurrentInvite.RedemptionURL))}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, completeErr := identity.completeOrgOIDCInvitation(ctx, concurrentAttempt, "concurrent-subject", "concurrent@example.test", "")
			results <- completeErr
		}()
	}
	successes := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent invite redemptions succeeded %d times", successes)
	}

	crossOrgInvite, err := identity.CreateOrgOIDCInvitation(ctx, owner, orgID, OrgOIDCInvitationInput{Email: "cross@example.test", Role: domain.Viewer, ExpiresAt: time.Now().Add(24 * time.Hour)}, "runtime-invite-cross-org")
	if err != nil {
		t.Fatal("create cross-tenant invitation")
	}
	crossOrgAttempt := concurrentAttempt
	crossOrgAttempt.OrgID = otherOrgID
	crossOrgAttempt.InvitationID = crossOrgInvite.ID
	crossOrgAttempt.InvitationHash = digest(mustInviteToken(t, crossOrgInvite.RedemptionURL))
	if _, err = identity.completeOrgOIDCInvitation(ctx, crossOrgAttempt, "cross-org-subject", "cross@example.test", ""); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("invitation created membership in another tenant")
	}
	assertNoInviteUser(t, db, otherOrgID, issuer, "cross-org-subject")

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

	rotationInvite, err := restarted.CreateOrgOIDCInvitation(ctx, owner, orgID, OrgOIDCInvitationInput{Email: "rotation@example.test", Role: domain.Viewer, ExpiresAt: time.Now().Add(24 * time.Hour)}, "runtime-invite-rotation")
	if err != nil {
		t.Fatal("create rotation-bound invitation")
	}
	state, browser = startInvitation(restarted, mustInviteToken(t, rotationInvite.RedemptionURL), "rotation-subject", "rotation@example.test", true)
	latest, err := restarted.OrgOIDC(ctx, owner, orgID)
	if err != nil {
		t.Fatal("read current org identity configuration")
	}
	if _, err = restarted.PutOrgOIDC(ctx, owner, orgID, OrgOIDCInput{Issuer: issuer, ClientID: "rotated-client"}, latest.Version, "runtime-rotate"); err != nil {
		t.Fatal("rotate OIDC config while callback pending")
	}
	rotated, err := restarted.OrgOIDC(ctx, owner, orgID)
	if err != nil {
		t.Fatal("read rotated OIDC config")
	}
	if err = db.Tenant(ctx, orgID, owner.User.ID, func(tx pgx.Tx) error {
		_, updateErr := tx.Exec(ctx, "UPDATE org_oidc_configs SET probe_attempt_at=now()-interval '31 seconds' WHERE org_id=$1", orgID)
		return updateErr
	}); err != nil {
		t.Fatal("expire OIDC probe cooldown")
	}
	rotated, err = restarted.ProbeOrgOIDC(ctx, owner, orgID, rotated.Version, "runtime-rotated-probe")
	if err != nil || !rotated.Verified {
		t.Fatal("verify rotated OIDC config")
	}
	if err = restarted.ActivateOrgOIDC(ctx, owner, orgID, rotated.Version, "runtime-rotated-activate"); err != nil {
		t.Fatal("activate rotated OIDC config")
	}
	if _, err = callback(restarted, state, browser); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("callback used rotated configuration")
	}
	if _, err = restarted.BeginOrgOIDCInvitation(ctx, httptest.NewRecorder(), mustInviteToken(t, rotationInvite.RedemptionURL)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("old invitation started login after provider rotation")
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
	disableInvite, err := restarted.CreateOrgOIDCInvitation(ctx, owner, otherOrgID, OrgOIDCInvitationInput{Email: "disabled@example.test", Role: domain.Viewer, ExpiresAt: time.Now().Add(24 * time.Hour)}, "runtime-invite-disable")
	if err != nil {
		t.Fatal("create disable-bound invitation")
	}
	state, browser = startInvitation(restarted, mustInviteToken(t, disableInvite.RedemptionURL), "disabled-subject", "disabled@example.test", true)
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

func signTestIDToken(key *rsa.PrivateKey, issuer, subject, audience, nonce, email string, emailVerified bool) (string, error) {
	claims, err := json.Marshal(map[string]any{
		"iss":            issuer,
		"sub":            subject,
		"aud":            audience,
		"exp":            time.Now().Add(5 * time.Minute).Unix(),
		"iat":            time.Now().Add(-time.Second).Unix(),
		"nonce":          nonce,
		"email":          email,
		"email_verified": emailVerified,
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

func mustInviteToken(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil || parsed.RawQuery != "" || !strings.HasPrefix(parsed.Fragment, "token=") {
		t.Fatal("invitation link exposed token in request URL")
	}
	values, err := url.ParseQuery(parsed.Fragment)
	if err != nil || len(values["token"]) != 1 {
		t.Fatal("invitation fragment token missing")
	}
	return values.Get("token")
}

func assertNoInviteUser(t *testing.T, db *store.Store, orgID, issuer, subject string) {
	t.Helper()
	err := db.Identity(context.Background(), "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), "SELECT set_config('reforge.org_id',$1,true),set_config('reforge.issuer',$2,true),set_config('reforge.subject',$3,true)", orgID, issuer, subject); err != nil {
			return err
		}
		var users, members int
		if err := tx.QueryRow(context.Background(), "SELECT count(*) FROM users WHERE issuer=$1 AND subject=$2", issuer, subject).Scan(&users); err != nil {
			return err
		}
		if err := tx.QueryRow(context.Background(), "SELECT count(*) FROM memberships m JOIN users u ON u.id=m.user_id WHERE m.org_id=$1 AND u.issuer=$2 AND u.subject=$3", orgID, issuer, subject).Scan(&members); err != nil {
			return err
		}
		if users != 0 || members != 0 {
			t.Fatal("failed invitation created identity or membership")
		}
		return nil
	})
	if err != nil {
		t.Fatal("verify failed invitation left no account")
	}
}
