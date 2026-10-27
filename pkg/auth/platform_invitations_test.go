package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/store"
)

func TestPlatformInvitationCreatesOrganisationOnce(t *testing.T) {
	databaseURL := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REFORGE_TEST_DATABASE_URL must point to a disposable PostgreSQL database with migrations applied")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	identity, err := New(ctx, db, Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, FixtureAuth: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRecorder()
	if _, err = identity.Login(ctx, login); err != nil {
		t.Fatal(err)
	}
	cookie := login.Result().Cookies()[0].Value
	session, err := identity.Authenticate(ctx, cookie)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlatformInvitation(ctx, db, "Acme", "not an email"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid email accepted: %v", err)
	}
	token, err := CreatePlatformInvitation(ctx, db, "Acme", "Owner@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identity.RedeemPlatformInvitation(ctx, session, token, "request-self-hosted"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("self-hosted edition redeemed a platform invitation: %v", err)
	}
	identity.cfg.Edition = "hosted"
	org, err := identity.RedeemPlatformInvitation(ctx, session, token, "request-redeem")
	if err != nil || org.Name != "Acme" {
		t.Fatalf("redeem: %+v %v", org, err)
	}
	if _, err = identity.RedeemPlatformInvitation(ctx, session, token, "request-reuse"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("invitation reused: %v", err)
	}
	session, err = identity.Authenticate(ctx, cookie)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range session.Memberships {
		if m.OrgID == org.ID {
			if m.Role != "owner" {
				t.Fatalf("role %q", m.Role)
			}
			return
		}
	}
	t.Fatal("membership missing")
}
