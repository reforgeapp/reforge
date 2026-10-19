package privateconnector

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
)

func TestEveryNativeMutationRechecksAuthority(t *testing.T) {
	requests, checks := 0, 0
	revoked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(204) }))
	defer server.Close()
	client := GuardHTTP(server.Client(), func(context.Context) error {
		checks++
		if revoked {
			return auth.ErrForbidden
		}
		return nil
	})
	call := func(method string) error {
		r, _ := http.NewRequest(method, server.URL, nil)
		response, err := client.Do(r)
		if response != nil {
			response.Body.Close()
		}
		return err
	}
	if err := call("POST"); err != nil {
		t.Fatal(err)
	}
	revoked = true
	if err := call("PATCH"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("revoked mutation sent: %v", err)
	}
	if err := call("GET"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || checks != 2 {
		t.Fatalf("requests=%d checks=%d", requests, checks)
	}
}
func TestPrivateMutationStatusRechecksRecordedAuthority(t *testing.T) {
	id := domain.NewID()
	credential, capability := "credential", "capability"
	called := 0
	c := &Connector{pending: map[string]*pending{id: {grant: Grant{ExpiresAt: time.Now().Add(time.Minute)}, credentialHash: sha256.Sum256([]byte(credential)), capabilityHash: sha256.Sum256([]byte(capability)), check: func(context.Context) error { called++; return auth.ErrForbidden }}}}
	if err := c.Active(context.Background(), credential, id, capability); !errors.Is(err, auth.ErrForbidden) || called != 1 {
		t.Fatalf("revoked grant remained active: %v", err)
	}
	if err := c.Active(context.Background(), credential, id, "wrong"); !errors.Is(err, auth.ErrUnauthenticated) || called != 1 {
		t.Fatal("invalid capability reached authority")
	}
}
