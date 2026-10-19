package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/network"
	"github.com/reforgeapp/reforge/internal/secrets"
	"github.com/reforgeapp/reforge/internal/store"
)

func TestNormalizeOrgOIDCIssuerBoundaries(t *testing.T) {
	cases := []struct {
		name        string
		issuer      string
		development bool
		valid       bool
	}{
		{name: "public HTTPS", issuer: "https://id.example.test/tenant", valid: true},
		{name: "loopback development HTTP", issuer: "http://127.0.0.1:8123/issuer", development: true, valid: true},
		{name: "public HTTP", issuer: "http://id.example.test", development: true},
		{name: "userinfo", issuer: "https://user@id.example.test"},
		{name: "query", issuer: "https://id.example.test?next=example"},
		{name: "empty query marker", issuer: "https://id.example.test?"},
		{name: "opaque URL", issuer: "https:id.example.test/tenant"},
		{name: "fragment", issuer: "https://id.example.test#issuer"},
		{name: "custom public port", issuer: "https://id.example.test:8443"},
		{name: "loopback HTTP outside development", issuer: "http://127.0.0.1"},
		{name: "path traversal", issuer: "https://id.example.test/tenant/../other"},
		{name: "encoded path traversal", issuer: "https://id.example.test/tenant/%2e%2e/other"},
		{name: "double encoded traversal", issuer: "https://id.example.test/tenant/%252e%252e/other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeIssuer(tc.issuer, tc.development)
			if tc.valid != (err == nil) {
				t.Fatalf("valid=%t, got %q, err=%v", tc.valid, got, err)
			}
		})
	}
}

func TestOrgOIDCIssuerUsesSharedPublicIPPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "168.63.129.16", "100.64.0.1", "192.0.2.1", "192.88.99.1", "198.18.0.1", "::1", "fe80::1", "2001:db8::1", "2002:a00:1::", "64:ff9b::a00:1"} {
		if network.IsPublicIP(netip.MustParseAddr(raw)) {
			t.Errorf("accepted non-public issuer address %q", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !network.IsPublicIP(netip.MustParseAddr(raw)) {
			t.Errorf("rejected public issuer address %q", raw)
		}
	}
}

func TestOrgOIDCMetadataEndpointsStayWithinIssuer(t *testing.T) {
	issuer, err := url.Parse("https://id.example.test/tenant")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		endpoint string
		valid    bool
	}{
		{"https://id.example.test/tenant/token", true},
		{"https://id.example.test/tenant/keys", true},
		{"https://id.example.test/tenant/%2e%2e/admin", false},
		{"https://id.example.test/tenant/%252e%252e/admin", false},
		{"https://id.example.test/tenant%2f..%2fadmin", false},
		{"https://id.example.test/other/token", false},
		{"https://attacker.example/tenant/token", false},
		{"http://id.example.test/tenant/token", false},
		{"https://user@id.example.test/tenant/token", false},
		{"https://id.example.test:443/tenant/token", false},
	} {
		if got := endpointInsideIssuer(tc.endpoint, issuer); got != tc.valid {
			t.Errorf("endpointInsideIssuer(%q) = %t, want %t", tc.endpoint, got, tc.valid)
		}
	}
}

func TestOrgOIDCPostgresContract(t *testing.T) {
	databaseURL := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REFORGE_TEST_DATABASE_URL must point to an empty disposable PostgreSQL database with migrations applied")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal("open test database")
	}
	defer db.Close()
	const bootstrapToken = "local-test-bootstrap-token-with-over-32-bytes"
	identity, err := New(ctx, db, Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, FixtureAuth: true, ListenAddress: "127.0.0.1:8080", BootstrapToken: bootstrapToken, BootstrapExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal("create test identity service")
	}
	vault, err := secrets.New("test", map[string]string{"test": base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal("create test vault")
	}
	identity.SetOrgOIDCVault(vault)
	loginResponse := httptest.NewRecorder()
	if _, err = identity.Login(ctx, loginResponse); err != nil {
		t.Fatal("create fixture session")
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("fixture session cookie missing")
	}
	owner, err := identity.Authenticate(ctx, cookies[0].Value)
	if err != nil || len(owner.Organisations) == 0 {
		t.Fatal("authenticate fixture owner")
	}
	orgID := "00000000-0000-4000-8000-000000000001"
	foundDevelopmentOrg := false
	for _, organisation := range owner.Organisations {
		if organisation.ID == orgID {
			foundDevelopmentOrg = true
			break
		}
	}
	if !foundDevelopmentOrg {
		t.Fatal("development organisation missing")
	}
	var healthy atomic.Bool
	healthy.Store(true)
	var issuer string
	metadataServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/tenant/.well-known/openid-configuration" {
			http.NotFound(writer, request)
			return
		}
		if !healthy.Load() {
			http.Error(writer, "unavailable", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		base := strings.TrimRight(issuer, "/")
		_, _ = writer.Write([]byte(`{"issuer":"` + issuer + `","authorization_endpoint":"` + base + `/authorize","token_endpoint":"` + base + `/token","jwks_uri":"` + base + `/keys"}`))
	}))
	defer metadataServer.Close()
	issuer = metadataServer.URL + "/tenant/"
	const clientSecret = "test-only-oidc-client-secret-value"
	created, err := identity.PutOrgOIDC(ctx, owner, orgID, OrgOIDCInput{Issuer: issuer, ClientID: "client-test", ClientSecret: clientSecret}, 0, "request-configure")
	if err != nil || !created.Configured || created.Version != 1 || created.Status != "draft" || !created.SecretPresent || created.ActivationAvailable {
		t.Fatal("owner could not save draft identity config")
	}
	encoded, err := json.Marshal(created)
	if err != nil || strings.Contains(string(encoded), clientSecret) {
		t.Fatal("identity response exposed client secret")
	}
	var stored string
	if err = db.Tenant(ctx, orgID, owner.User.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT envelope::text FROM org_oidc_secrets WHERE org_id=$1`, orgID).Scan(&stored)
	}); err != nil || strings.Contains(stored, clientSecret) {
		t.Fatal("identity secret was not encrypted at rest")
	}
	if _, err = identity.PutOrgOIDC(ctx, owner, orgID, OrgOIDCInput{Issuer: issuer, ClientID: "client-test"}, 0, "request-stale"); !errors.Is(err, ErrConflict) {
		t.Fatal("stale OIDC config version accepted")
	}
	updated, err := identity.PutOrgOIDC(ctx, owner, orgID, OrgOIDCInput{Issuer: issuer, ClientID: "client-test"}, created.Version, "request-update")
	if err != nil || updated.Version != 2 {
		t.Fatal("secret-preserving config update failed")
	}
	var persisted orgOIDCRecord
	if err = identity.WithActor(ctx, owner, orgID, func(tx pgx.Tx, _ domain.Actor) error {
		var loadErr error
		persisted, loadErr = loadOrgOIDC(ctx, tx, orgID)
		return loadErr
	}); err != nil {
		t.Fatal("read persisted OIDC config")
	}
	var envelope secrets.Envelope
	if err = json.Unmarshal(persisted.SecretEnvelope, &envelope); err != nil {
		t.Fatal("decode encrypted secret envelope")
	}
	opened, err := vault.OpenContext(ctx, secrets.Binding{OrgID: orgID, ConnectionID: persisted.ID, Version: persisted.SecretVersion}, envelope)
	if err != nil || string(opened) != clientSecret {
		t.Fatal("secret-preserving update changed the stored client secret")
	}
	clear(opened)
	verified, err := identity.ProbeOrgOIDC(ctx, owner, orgID, updated.Version, "request-probe-success")
	if err != nil || verified.Status != "probe_verified" || !verified.Verified || !verified.SecretPresent || !verified.ActivationAvailable {
		t.Fatal("successful metadata probe was not represented truthfully")
	}
	if _, err = identity.ProbeOrgOIDC(ctx, owner, orgID, updated.Version, "request-probe-too-soon"); err == nil {
		t.Fatal("successful probe cooldown was not enforced")
	} else {
		var cooldown *ProbeCooldownError
		if !errors.As(err, &cooldown) || cooldown.RetryAfter <= 0 {
			t.Fatal("successful probe did not return a retry delay")
		}
	}
	if err = identity.ActivateOrgOIDC(ctx, owner, orgID, updated.Version, "request-activate"); err != nil {
		t.Fatal("verified organization OIDC configuration could not be activated")
	}
	active, err := identity.OrgOIDC(ctx, owner, orgID)
	if err != nil || active.Status != "active" || active.ActivationAvailable || active.ActivationBlocked != "" {
		t.Fatal("active organization OIDC status was not represented truthfully")
	}
	if err = db.Tenant(ctx, orgID, owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE org_oidc_configs SET probe_attempt_at=now()-interval '31 seconds' WHERE org_id=$1`, orgID)
		return err
	}); err != nil {
		t.Fatal("expire successful-probe cooldown")
	}
	healthy.Store(false)
	if _, err = identity.ProbeOrgOIDC(ctx, owner, orgID, updated.Version, "request-probe-failure"); !errors.Is(err, ErrOIDCProbe) {
		t.Fatal("unavailable issuer probe was accepted")
	}
	failed, err := identity.OrgOIDC(ctx, owner, orgID)
	if err != nil || failed.Status != "draft" || failed.Verified {
		t.Fatal("failed probe left stale verification active")
	}
	if _, err = identity.ProbeOrgOIDC(ctx, owner, orgID, updated.Version, "request-failed-probe-too-soon"); err == nil {
		t.Fatal("failed probe cooldown was not enforced")
	} else {
		var cooldown *ProbeCooldownError
		if !errors.As(err, &cooldown) || cooldown.RetryAfter <= 0 {
			t.Fatal("failed probe did not return a retry delay")
		}
	}
	adminUser, err := identity.upsertUser(ctx, "test:idp", "admin-subject", "Admin", "admin@example.test")
	if err != nil {
		t.Fatal("create admin fixture identity")
	}
	if _, err = identity.PutMember(ctx, owner, orgID, adminUser.ID, Membership{Role: domain.Admin}, 0, "request-admin"); err != nil {
		t.Fatal("create admin membership")
	}
	adminToken, err := identity.createSession(ctx, adminUser.ID)
	if err != nil {
		t.Fatal("create admin session")
	}
	admin, err := identity.Authenticate(ctx, adminToken)
	if err != nil {
		t.Fatal("authenticate admin")
	}
	if _, err = identity.OrgOIDC(ctx, admin, orgID); !errors.Is(err, ErrForbidden) {
		t.Fatal("admin could read owner-only OIDC config")
	}
	viewerUser, err := identity.upsertUser(ctx, "test:idp", "viewer-subject", "Viewer", "viewer@example.test")
	if err != nil {
		t.Fatal("create viewer fixture identity")
	}
	if _, err = identity.PutMember(ctx, owner, orgID, viewerUser.ID, Membership{Role: domain.Viewer}, 0, "request-viewer"); err != nil {
		t.Fatal("create viewer membership")
	}
	viewerToken, err := identity.createSession(ctx, viewerUser.ID)
	if err != nil {
		t.Fatal("create viewer session")
	}
	viewer, err := identity.Authenticate(ctx, viewerToken)
	if err != nil {
		t.Fatal("authenticate viewer")
	}
	if _, err = identity.OrgOIDC(ctx, viewer, orgID); !errors.Is(err, ErrForbidden) {
		t.Fatal("viewer could read owner-only OIDC config")
	}
	org2, err := identity.Bootstrap(ctx, owner, bootstrapToken, "Isolated organisation", "request-org2")
	if err != nil {
		t.Fatal("create second organisation")
	}
	var visible int
	err = db.Tenant(ctx, org2.ID, owner.User.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM org_oidc_configs`).Scan(&visible)
	})
	if err != nil {
		t.Fatal("cross-tenant policy query")
	}
	if visible != 0 {
		t.Fatal("OIDC configuration crossed tenant boundary")
	}
	var affected int64
	err = db.Tenant(ctx, org2.ID, owner.User.ID, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx, `UPDATE org_oidc_configs SET client_id='cross-tenant-write' WHERE org_id=$1`, orgID)
		affected = tag.RowsAffected()
		return execErr
	})
	if err != nil || affected != 0 {
		t.Fatal("cross-tenant identity config write was visible")
	}
	disabled, err := identity.DisableOrgOIDC(ctx, owner, orgID, updated.Version, "request-disable")
	if err != nil || disabled.Status != "disabled" || !disabled.SecretPresent {
		t.Fatal("owner could not disable draft OIDC config")
	}
}

func TestOrgOIDCProbeConcurrentCapacity(t *testing.T) {
	acquired := 0
	for acquired < cap(orgOIDCProbeSlots) && acquireOrgOIDCProbe() {
		acquired++
	}
	defer func() {
		for i := 0; i < acquired; i++ {
			releaseOrgOIDCProbe()
		}
	}()
	if acquired != cap(orgOIDCProbeSlots) || acquireOrgOIDCProbe() {
		t.Fatal("per-process OIDC probe capacity was not enforced")
	}
}

func TestOrgOIDCMetadataProbeSuccessAndRequestBoundary(t *testing.T) {
	var issuer string
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/tenant/.well-known/openid-configuration" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"issuer":"` + issuer + `","authorization_endpoint":"` + issuer + `/tenant/authorize","token_endpoint":"` + issuer + `/tenant/token","jwks_uri":"` + issuer + `/tenant/keys"}`))
	}))
	defer server.Close()
	issuer = server.URL + "/tenant"
	metadata, err := fetchOIDCMetadata(context.Background(), issuer, true)
	if err != nil || metadata.Issuer != issuer {
		t.Fatal("local development issuer metadata rejected")
	}
	parsed, err := url.Parse(issuer)
	if err != nil {
		t.Fatal(err)
	}
	client, err := issuerHTTPClient(context.Background(), parsed, true)
	if err != nil {
		t.Fatal("local issuer client unavailable")
	}
	for _, target := range []string{server.URL + "/outside/token", "https://attacker.example/tenant/token", server.URL + "/tenant/%2e%2e/outside"} {
		request, requestErr := http.NewRequest(http.MethodGet, target, nil)
		if requestErr != nil {
			t.Fatal("create bounded request")
		}
		if _, requestErr = client.Do(request); requestErr == nil {
			t.Errorf("issuer client admitted outside request target %s", target)
		}
	}
	if requests.Load() != 1 {
		t.Fatal("out-of-bound request reached the local issuer")
	}
}

func TestOrgOIDCMetadataRedirectRejected(t *testing.T) {
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/outside", http.StatusFound)
	}))
	defer server.Close()
	issuer = server.URL + "/tenant"
	if _, err := fetchOIDCMetadata(context.Background(), issuer, true); !errors.Is(err, ErrOIDCProbe) {
		t.Fatal("issuer metadata redirect was accepted")
	}
}

func TestOrgOIDCResponseBodyBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("x", (1<<20)+1)))
	}))
	defer server.Close()
	issuer, err := url.Parse(server.URL + "/tenant")
	if err != nil {
		t.Fatal(err)
	}
	client, err := issuerHTTPClient(context.Background(), issuer, true)
	if err != nil {
		t.Fatal("create bounded issuer client")
	}
	defer client.CloseIdleConnections()
	response, err := client.Get(server.URL + "/tenant/body")
	if err != nil {
		t.Fatal("request bounded issuer body")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err == nil || len(body) > 1<<20 {
		t.Fatal("oversized issuer response body was not rejected")
	}
}
