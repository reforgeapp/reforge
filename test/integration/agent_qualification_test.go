package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/agent"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func TestAgentQualificationGatesCapabilities(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	connection, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{
		Kind: "agent", Provider: "codex", Name: "Codex runtime", Endpoint: "https://codex.example",
		Settings: connections.Settings{AuthKind: "official_runtime", BillingRoute: "subscription", RuntimeVersion: "0.150.1", Namespace: "workspace-1", Model: "gpt-5"},
	}, "agent-test")
	if err != nil {
		t.Fatal(err)
	}
	service := agent.NewQualificationService(f.db, f.identity)
	binding := agent.Binding{OrgID: f.org, ConnectionID: connection.ID, ConnectionVersion: connection.Version, CredentialVersion: connection.CredentialVersion, AccountID: "workspace-1", Model: "gpt-5", RuntimeDigest: "0.150.1", Deployment: ""}

	if _, found, err := service.Get(ctx, f.owner, f.org, connection.ID); err != nil || found {
		t.Fatalf("unexpected initial qualification found=%v err=%v", found, err)
	}
	caps := agent.Capabilities(agent.Qualification{}, binding, "codex", false)
	if caps["runtime"].State != domain.Unsupported {
		t.Fatalf("unqualified runtime must be unsupported, got %q", caps["runtime"].State)
	}
	if _, ok := caps["entitlement"]; ok {
		t.Fatal("unqualified runtime must not expose feature capabilities")
	}

	now := time.Now().UTC().Truncate(time.Second)
	q := agent.Qualification{Binding: binding, EvidenceID: domain.NewID(), CheckedAt: now, ExpiresAt: now.Add(60 * 24 * time.Hour), AuthCustody: true, NativeToolContainment: true, Terms: true, Topology: true, Entitlement: true, Quota: true, NoPaidOverage: true}
	if _, err := service.Put(ctx, f.owner, f.org, connection.ID, q); err != nil {
		t.Fatal(err)
	}
	stored, found, err := service.Get(ctx, f.owner, f.org, connection.ID)
	if err != nil || !found || stored.Binding != binding {
		t.Fatalf("stored qualification mismatch found=%v err=%v %+v", found, err, stored)
	}
	caps = agent.Capabilities(stored, binding, "codex", true)
	if caps["auth_custody"].State != domain.Supported || caps["no_paid_overage"].State != domain.Supported {
		t.Fatalf("qualified features must be supported: %+v", caps)
	}
	if caps["runtime"].State != domain.Unsupported {
		t.Fatal("feature qualification must not certify the runtime itself")
	}

	expired := q
	expired.CheckedAt = now.Add(-2 * time.Hour)
	expired.ExpiresAt = now.Add(-time.Hour)
	if _, err := service.Put(ctx, f.owner, f.org, connection.ID, expired); !errors.Is(err, agent.ErrInvalidQualification) {
		t.Fatalf("expired qualification must be rejected, got %v", err)
	}
	mismatch := q
	mismatch.Binding.ConnectionID = domain.NewID()
	if _, err := service.Put(ctx, f.owner, f.org, connection.ID, mismatch); !errors.Is(err, agent.ErrInvalidQualification) {
		t.Fatalf("binding mismatch must be rejected, got %v", err)
	}
	grant, err := auth.NewAutomationSession(f.org, f.owner.User.ID, domain.NewID(), []string{domain.NewID()}, func(context.Context, pgx.Tx) error { return nil })
	if err == nil {
		if _, err = service.Put(ctx, grant, f.org, connection.ID, q); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("automation grant must not record qualification, got %v", err)
		}
	}
	if err := service.Delete(ctx, f.owner, f.org, connection.ID); err != nil {
		t.Fatal(err)
	}
	if _, found, err := service.Get(ctx, f.owner, f.org, connection.ID); err != nil || found {
		t.Fatalf("qualification must be cleared found=%v err=%v", found, err)
	}
}

func TestAgentAuthStaysDisabledUntilRuntimeAndQualificationExist(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	connection, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{
		Kind: "agent", Provider: "codex", Name: "Codex runtime", Endpoint: "https://codex.example",
		Settings: connections.Settings{AuthKind: "official_runtime", BillingRoute: "subscription", RuntimeVersion: "0.150.1", Namespace: "workspace-1", Model: "gpt-5", Profile: "customer-runner"},
	}, "agent-test")
	if err != nil {
		t.Fatal(err)
	}
	qualifications := agent.NewQualificationService(f.db, f.identity)
	unconfigured := agent.NewAuthService(f.db, f.identity, f.connections, qualifications, agent.NewFactory(nil, nil))
	if _, err := unconfigured.Login(ctx, f.owner, f.org, connection.ID); !errors.Is(err, agent.ErrDisabled) {
		t.Fatalf("unconfigured runtime login must be disabled, got %v", err)
	}
	launcher := agent.NewFactory(func(context.Context, agent.Binding) (agent.Runtime, error) {
		return agent.Runtime{}, errors.New("no runtime")
	}, nil)
	service := agent.NewAuthService(f.db, f.identity, f.connections, qualifications, launcher)
	if _, err := service.Login(ctx, f.owner, f.org, connection.ID); !errors.Is(err, agent.ErrDisabled) {
		t.Fatalf("unqualified runtime login must be disabled, got %v", err)
	}
	binding := agent.BindingFor(connection)
	now := time.Now().UTC().Truncate(time.Second)
	q := agent.Qualification{Binding: binding, EvidenceID: domain.NewID(), CheckedAt: now, ExpiresAt: now.Add(24 * time.Hour), AuthCustody: true, Topology: true}
	if _, err := qualifications.Put(ctx, f.owner, f.org, connection.ID, q); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(ctx, f.owner, f.org, connection.ID); !errors.Is(err, agent.ErrDisabled) {
		t.Fatalf("login must fail closed when the launcher cannot start the runtime, got %v", err)
	}
}
