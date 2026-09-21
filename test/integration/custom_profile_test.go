package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/customcmd"
	"reforge/internal/domain"
)

func customProfileFixture(t *testing.T) (*inventoryFixture, *customcmd.Service) {
	t.Helper()
	f := newInventoryFixture(t, 0)
	return f, customcmd.New(f.db, f.identity)
}

func customProfileInput() customcmd.Profile {
	return customcmd.Profile{
		Name:            "reviewer",
		ImageDigest:     "sha256:" + strings.Repeat("a", 64),
		Executable:      "/bin/review",
		Argv:            []string{"--mode", "review"},
		ProtocolVersion: customcmd.ProtocolVersion,
		MaxWallSeconds:  30,
		MaxOutputBytes:  1 << 20,
		MaxTurns:        4,
		Concurrency:     1,
	}
}

func TestCustomProfileApprovalRevocationAndBind(t *testing.T) {
	f, service := customProfileFixture(t)
	ctx := context.Background()
	created, err := service.Create(ctx, f.owner, f.org, customProfileInput(), "custom-test")
	if err != nil {
		t.Fatal(err)
	}
	if created.State() != "draft" {
		t.Fatalf("new profile must be draft, got %q", created.State())
	}
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, e := service.Bind(ctx, tx, f.org, created.ID, created.Version); !errors.Is(e, customcmd.ErrNotApproved) {
			t.Fatalf("draft profile must not bind, got %v", e)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := service.Approve(ctx, f.owner, f.org, created.ID, "ticket-1", created.Version, "custom-test")
	if err != nil {
		t.Fatal(err)
	}
	if approved.State() != "approved" || approved.Version != created.Version+1 {
		t.Fatalf("unexpected approval %+v", approved)
	}
	if _, err = service.Approve(ctx, f.owner, f.org, created.ID, "ticket-1", created.Version, "custom-test"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale approval must conflict, got %v", err)
	}
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		bound, e := service.Bind(ctx, tx, f.org, created.ID, approved.Version)
		if e != nil {
			return e
		}
		if bound.ImageDigest != approved.ImageDigest || bound.Version != approved.Version {
			t.Fatalf("bind returned unexpected profile %+v", bound)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := service.Revoke(ctx, f.owner, f.org, created.ID, approved.Version, "custom-test")
	if err != nil {
		t.Fatal(err)
	}
	if revoked.State() != "revoked" {
		t.Fatalf("expected revoked, got %q", revoked.State())
	}
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, e := service.Bind(ctx, tx, f.org, created.ID, revoked.Version); !errors.Is(e, customcmd.ErrNotApproved) {
			t.Fatalf("revoked profile must not bind, got %v", e)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCustomProfileValidationAndAutomation(t *testing.T) {
	f, service := customProfileFixture(t)
	ctx := context.Background()
	bad := customProfileInput()
	bad.ImageDigest = "alpine:3.21"
	if _, err := service.Create(ctx, f.owner, f.org, bad, "custom-test"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("tag digest must be rejected, got %v", err)
	}
	bad = customProfileInput()
	bad.Argv = []string{"a;rm -rf /"}
	if _, err := service.Create(ctx, f.owner, f.org, bad, "custom-test"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("shell metacharacters must be rejected, got %v", err)
	}
	grant, err := auth.NewAutomationSession(f.org, f.owner.User.ID, domain.NewID(), []string{domain.NewID()}, func(context.Context, pgx.Tx) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Create(ctx, grant, f.org, customProfileInput(), "custom-test"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("automation grant must not manage profiles, got %v", err)
	}
}

func TestCustomProfileTenantIsolation(t *testing.T) {
	f, service := customProfileFixture(t)
	ctx := context.Background()
	created, err := service.Create(ctx, f.owner, f.org, customProfileInput(), "custom-test")
	if err != nil {
		t.Fatal(err)
	}
	other := domain.NewID()
	if err = f.db.Tenant(ctx, other, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Other tenant')`, other); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, other, f.owner.User.ID)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Get(ctx, f.owner, other, created.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("cross-tenant get must be forbidden, got %v", err)
	}
	page, err := service.List(ctx, f.owner, other, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("cross-tenant list must be empty, got %d", len(page.Items))
	}
}

func TestCustomProfileExpiredApprovalDoesNotBind(t *testing.T) {
	f, service := customProfileFixture(t)
	ctx := context.Background()
	created, err := service.Create(ctx, f.owner, f.org, customProfileInput(), "custom-test")
	if err != nil {
		t.Fatal(err)
	}
	approved, err := service.Approve(ctx, f.owner, f.org, created.ID, "ticket-1", created.Version, "custom-test")
	if err != nil {
		t.Fatal(err)
	}
	if customcmd.Approved(approved, time.Now().Add(-time.Minute)) {
		t.Fatal("approval must not be valid before it was recorded")
	}
}
