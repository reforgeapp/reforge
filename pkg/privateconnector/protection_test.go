package privateconnector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/forge"
)

type protectionHTTPFixture struct{ calls int }

func (f *protectionHTTPFixture) Do(*http.Request) (*http.Response, error) {
	f.calls++
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

func TestProtectionHTTPAllowsOnlyBranchProtectionsReads(t *testing.T) {
	backend := &protectionHTTPFixture{}
	client := ProtectionHTTP(backend)
	allowed, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://forge.example/api/v1/repos/org/repo/branch_protections", nil)
	if _, err := client.Do(allowed); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/repos/org/repo/branch_protections"},
		{http.MethodGet, "/api/v1/repos/org/repo/hooks"},
		{http.MethodGet, "/api/v1/repos/org/repo/branch_protections?branch=main"},
	} {
		req, _ := http.NewRequestWithContext(context.Background(), tc.method, "https://forge.example"+tc.path, nil)
		if _, err := client.Do(req); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("%s %s error=%v", tc.method, tc.path, err)
		}
	}
	if backend.calls != 1 {
		t.Fatalf("backend calls=%d", backend.calls)
	}
}

func TestProtectionWireRoundTripAndSecretRedaction(t *testing.T) {
	grant := Grant{ID: "grant", Connection: Connection{
		ID: "target", OrgID: "org", Provider: "gitea", Endpoint: "https://forge.example", Secret: "target-secret",
		Protection: &ProtectionCredential{ID: "protection", Secret: "protection-secret"},
	}, Operation: Operation{ID: "operation", Kind: ForgeProbe}}
	for _, value := range []string{fmt.Sprintf("%+v", grant), mustJSON(t, grant)} {
		if strings.Contains(value, "target-secret") || strings.Contains(value, "protection-secret") {
			t.Fatalf("secret leaked: %s", value)
		}
	}
	wire, err := grant.MarshalWire()
	if err != nil || !strings.Contains(string(wire), "protection-secret") {
		t.Fatalf("wire err=%v", err)
	}
	decoded, err := DecodeGrant(wire)
	if err != nil || decoded.Connection.Protection == nil || decoded.Connection.Protection.Secret != "protection-secret" {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
	if _, err := DecodeGrant([]byte(`{"protection_secret":"orphan"}`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("orphan secret error=%v", err)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var _ forge.HTTPClient = (*protectionHTTPFixture)(nil)
