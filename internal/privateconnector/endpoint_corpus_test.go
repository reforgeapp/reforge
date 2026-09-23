package privateconnector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"reforge/internal/domain"
	"reforge/internal/network"
)

func TestEndpointHostileCorpusRejectedAtGrantBoundary(t *testing.T) {
	orgID := domain.NewID()
	runnerID := domain.NewID()
	connectionID := domain.NewID()
	base := Connection{ID: connectionID, OrgID: orgID, Kind: "forge", Provider: "gitea", Endpoint: "https://forge.example", Version: 1, CredentialVersion: 1, Secret: "fixture-provider-secret-0123456789", Route: network.PrivateRoute{OrgID: orgID, ConnectionID: connectionID, RunnerID: runnerID, Host: "forge.example", CIDRs: []string{"10.20.0.0/16"}}}
	for _, endpoint := range []string{
		"https://user:pass@forge.example",
		"https://forge.example?token=secret",
		"https://forge.example#fragment",
		"https://forge.example/api/%252e%252e/admin",
		"https://forge.example./api",
		"https://forge.example:0",
		"http://forge.example",
		"https://169.254.169.254/latest/meta-data",
		"https://168.63.129.16",
		"https://100.100.100.200/latest/meta-data",
		"https://[fd00:ec2::254]/latest/meta-data",
		"https://127.0.0.1",
		"https://[::1]",
		"https://10.20.1.7",
	} {
		t.Run(endpoint, func(t *testing.T) {
			connection := base
			connection.Endpoint = endpoint
			if err := validateConnection(connection, Target{OrgID: orgID, RunnerID: runnerID}, false); err == nil {
				t.Fatalf("hostile endpoint accepted: %s", endpoint)
			}
		})
	}
}

func TestExecutorDoesNotFollowApprovedRouteRedirectToMetadata(t *testing.T) {
	var redirected atomic.Int32
	metadata := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer metadata.Close()
	metadataURL, err := url.Parse(metadata.URL)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	forgeFixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/api/v1/version" || r.Header.Get("Authorization") != "token fixture-provider-secret-0123456789" {
			t.Errorf("unexpected connector request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Location", metadata.URL+"/latest/meta-data")
		w.WriteHeader(http.StatusFound)
	}))
	defer forgeFixture.Close()
	endpoint, err := url.Parse(forgeFixture.URL)
	if err != nil {
		t.Fatal(err)
	}
	orgID := domain.NewID()
	runnerID := domain.NewID()
	connectionID := domain.NewID()
	grant := Grant{
		ID: domain.NewID(), RunnerVersion: 1,
		Target:      Target{OrgID: orgID, RunnerID: runnerID},
		Operation:   Operation{ID: domain.NewID(), Kind: ForgeProbe},
		AuthorityID: domain.NewID(),
		Connection:  Connection{ID: connectionID, OrgID: orgID, Kind: "forge", Provider: "gitea", Endpoint: forgeFixture.URL, Version: 1, CredentialVersion: 1, Secret: "fixture-provider-secret-0123456789", Route: network.PrivateRoute{OrgID: orgID, ConnectionID: connectionID, RunnerID: runnerID, Host: endpoint.Hostname(), CIDRs: []string{"127.0.0.1/32"}}},
		TimeoutMS:   2000, ExpiresAt: time.Now().Add(2 * time.Second), ResultCapability: "0123456789012345678901234567890123456789012",
		executionDeadline: time.Now().Add(2 * time.Second),
	}
	result := (Executor{Target: grant.Target, Development: true}).Execute(context.Background(), grant)
	if result.Failure == nil || result.Failure.Code != "provider" {
		t.Fatalf("redirect response did not fail closed: %+v", result)
	}
	if requests.Load() != 1 || redirected.Load() != 0 || metadataURL.Host == endpoint.Host {
		t.Fatalf("redirect crossed approved route: forge requests=%d metadata requests=%d", requests.Load(), redirected.Load())
	}
}
