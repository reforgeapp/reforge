package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"reforge/internal/auth"
	"reforge/internal/config"
	"reforge/internal/store"
)

func TestOrgInvitationRoutesAreRegistered(t *testing.T) {
	server := New(config.Config{}, nil)
	server.RegisterIdentity(nil)
	routes := map[string]bool{}
	for _, route := range server.Router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/v1/orgs/:orgID/identity/oidc/invitations",
		"POST /api/v1/orgs/:orgID/identity/oidc/invitations",
		"DELETE /api/v1/orgs/:orgID/identity/oidc/invitations/:invitationID",
		"POST /auth/invitations/redeem",
	} {
		if !routes[route] {
			t.Fatalf("missing invitation route %s", route)
		}
	}
}

func TestInvitationRedeemRequiresSameOriginAndDoesNotEchoToken(t *testing.T) {
	databaseURL := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REFORGE_TEST_DATABASE_URL must point to a disposable PostgreSQL database using a restricted runtime role")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal("open restricted test database")
	}
	defer db.Close()
	const publicURL = "https://reforge.example.test"
	service, err := auth.New(ctx, db, auth.Config{PublicURL: publicURL, Edition: "self-hosted", ListenAddress: "0.0.0.0:8080"})
	if err != nil {
		t.Fatal("create identity service")
	}
	server := New(config.Config{PublicURL: publicURL}, db)
	server.RegisterIdentity(service)
	const token = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	for _, tc := range []struct {
		name   string
		origin string
		query  string
	}{
		{name: "missing origin"},
		{name: "cross-site origin", origin: "https://attacker.example"},
		{name: "token in query", origin: publicURL, query: "?token=" + token},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, publicURL+"/auth/invitations/redeem"+tc.query, strings.NewReader("token="+token))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.origin != "" {
				request.Header.Set("Origin", tc.origin)
			}
			writer := httptest.NewRecorder()
			server.Router.ServeHTTP(writer, request)
			if tc.query != "" && writer.Code != http.StatusBadRequest {
				t.Fatalf("query token status = %d", writer.Code)
			}
			if tc.query == "" && writer.Code != http.StatusForbidden {
				t.Fatalf("untrusted Origin status = %d", writer.Code)
			}
			if writer.Header().Get("Referrer-Policy") != "no-referrer" || writer.Header().Get("Cache-Control") != "no-store" || strings.Contains(writer.Body.String(), token) {
				t.Fatal("redemption error leaked token or omitted no-referrer/no-store")
			}
		})
	}
}
