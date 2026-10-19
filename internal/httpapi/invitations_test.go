package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/config"
	"github.com/reforgeapp/reforge/internal/store"
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
	form := "application/x-www-form-urlencoded;charset=UTF-8"
	for _, tc := range []struct {
		name        string
		origin      string
		fetchSite   string
		contentType string
		query       string
		body        string
		status      int
	}{
		{name: "missing origin", contentType: form, body: "token=" + token, status: http.StatusForbidden},
		{name: "cross-site origin", origin: "https://attacker.example", contentType: form, body: "token=" + token, status: http.StatusForbidden},
		{name: "cross-site fetch metadata", origin: publicURL, fetchSite: "cross-site", contentType: form, body: "token=" + token, status: http.StatusForbidden},
		{name: "token in query", origin: publicURL, contentType: form, query: "?token=" + token, body: "token=" + token, status: http.StatusBadRequest},
		{name: "json body", origin: publicURL, contentType: "application/json", body: `{"token":"` + token + `"}`, status: http.StatusBadRequest},
		{name: "oversized body", origin: publicURL, contentType: form, body: "token=" + token + "&pad=" + strings.Repeat("a", 9<<10), status: http.StatusBadRequest},
		{name: "duplicate token", origin: publicURL, contentType: form, body: "token=" + token + "&token=" + token, status: http.StatusUnauthorized},
		{name: "unknown token", origin: publicURL, fetchSite: "same-origin", contentType: form, body: "token=" + token, status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, publicURL+"/auth/invitations/redeem"+tc.query, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", tc.contentType)
			if tc.origin != "" {
				request.Header.Set("Origin", tc.origin)
			}
			if tc.fetchSite != "" {
				request.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}
			writer := httptest.NewRecorder()
			server.Router.ServeHTTP(writer, request)
			if writer.Code != tc.status {
				t.Fatalf("status = %d, want %d", writer.Code, tc.status)
			}
			if writer.Header().Get("Referrer-Policy") != "no-referrer" || writer.Header().Get("Cache-Control") != "no-store" || writer.Header().Get("Location") != "" || strings.Contains(writer.Body.String(), token) {
				t.Fatal("redemption error leaked token, redirected or omitted no-referrer/no-store")
			}
			if !strings.HasPrefix(writer.Header().Get("Content-Type"), "application/json") || strings.Contains(writer.Body.String(), "authorization_url") {
				t.Fatal("redemption error must be a JSON API error without an authorization URL")
			}
		})
	}
}
