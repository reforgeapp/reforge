package providers

import (
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/connections"
)

func protectionConnection() connections.Connection {
	return connections.Connection{
		ID: "target", OrgID: "org", Provider: "gitea", Kind: "forge", Endpoint: "https://forge.example",
		Settings: connections.Settings{CAPEM: "ca"}, State: "healthy",
		Route: &connections.Route{RunnerID: "runner", Host: "10.0.0.2", CIDRs: []string{"10.0.0.0/24"}},
	}
}

func TestSameProtectionRouteRejectsIdentityAndRouteMismatches(t *testing.T) {
	base := protectionConnection()
	valid := base
	valid.ID = "protection"
	cases := []struct {
		name   string
		mutate func(*connections.Connection)
	}{
		{"tenant", func(c *connections.Connection) { c.OrgID = "other" }},
		{"endpoint", func(c *connections.Connection) { c.Endpoint = "https://other.example" }},
		{"route", func(c *connections.Connection) {
			c.Route = &connections.Route{RunnerID: "other", Host: "10.0.0.2", CIDRs: []string{"10.0.0.0/24"}}
		}},
		{"revoked", func(c *connections.Connection) { now := time.Now(); c.Route.RevokedAt = &now }},
		{"self", func(c *connections.Connection) { c.ID = base.ID }},
	}
	if !sameProtectionRoute(base, valid) {
		t.Fatal("matching protection route rejected")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := valid
			route := *valid.Route
			candidate.Route = &route
			tc.mutate(&candidate)
			if sameProtectionRoute(base, candidate) {
				t.Fatal("invalid protection route accepted")
			}
		})
	}
}
