package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/store"
)

func TestPlatformInvitationCreatesOrganisationOnce(t *testing.T) {
	databaseURL := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REFORGE_TEST_DATABASE_URL must point to a disposable PostgreSQL database with migrations applied")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	staffURL := os.Getenv("REFORGE_TEST_STAFF_DATABASE_URL")
	if staffURL == "" {
		t.Skip("REFORGE_TEST_STAFF_DATABASE_URL must point to the same database as the reforge_staff role")
	}
	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	staff, err := store.Open(ctx, staffURL)
	if err != nil {
		t.Fatal(err)
	}
	defer staff.Close()
	if _, err = CreatePlatformInvitation(ctx, db, "Runtime "+string(domain.NewID())[:8], "runtime@example.com"); err == nil {
		t.Fatal("runtime role created a tenant invitation")
	}
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
	acme, globex := "Acme "+string(domain.NewID())[:8], "Globex "+string(domain.NewID())[:8]
	if _, err = CreatePlatformInvitation(ctx, staff, acme, "not an email"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid email accepted: %v", err)
	}
	token, err := CreatePlatformInvitation(ctx, staff, acme, "Owner@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identity.RedeemPlatformInvitation(ctx, session, token, "request-self-hosted"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("self-hosted edition redeemed a platform invitation: %v", err)
	}
	identity.cfg.Edition = "hosted"
	org, err := identity.RedeemPlatformInvitation(ctx, session, token, "request-redeem")
	if err != nil || org.Name != acme {
		t.Fatalf("redeem: %+v %v", org, err)
	}
	if _, err = identity.RedeemPlatformInvitation(ctx, session, token, "request-reuse"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("invitation reused: %v", err)
	}
	if _, err = CreatePlatformInvitation(ctx, staff, strings.ToLower(acme), "owner@example.com"); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted invitation re-issued: %v", err)
	}
	first, err := CreatePlatformInvitation(ctx, staff, globex, "lead@example.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreatePlatformInvitation(ctx, staff, globex, "lead@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identity.RedeemPlatformInvitation(ctx, session, first, "request-stale"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("superseded token redeemed: %v", err)
	}
	pending, err := PlatformInvitations(ctx, staff)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, p := range pending {
		if p.OrgName == globex {
			count++
			if err = RevokePlatformInvitation(ctx, staff, p.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if count != 1 {
		t.Fatalf("pending Globex invitations: %d", count)
	}
	if _, err = identity.RedeemPlatformInvitation(ctx, session, second, "request-revoked"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked invitation redeemed: %v", err)
	}
	session, err = identity.Authenticate(ctx, cookie)
	if err != nil {
		t.Fatal(err)
	}
	memberToken, orgName, err := identity.CreateMemberInvitation(ctx, session, org.ID, "colleague@example.com", "maintainer", "request-member")
	if err != nil || orgName != acme {
		t.Fatalf("member invitation: %q %v", orgName, err)
	}
	listed, err := identity.MemberInvitations(ctx, session, org.ID)
	if err != nil || len(listed) != 1 || listed[0].Role != "maintainer" {
		t.Fatalf("member invitations: %+v %v", listed, err)
	}
	joined, err := identity.RedeemPlatformInvitation(ctx, session, memberToken, "request-join")
	if err != nil || joined.ID != org.ID {
		t.Fatalf("member join: %+v %v", joined, err)
	}
	tenants, err := PlatformTenants(ctx, staff)
	if err != nil {
		t.Fatal(err)
	}
	for _, tenant := range tenants {
		if tenant.ID == org.ID {
			if tenant.Name != acme || tenant.Members != 1 {
				t.Fatalf("tenant %+v", tenant)
			}
			return
		}
	}
	t.Fatal("tenant missing")
}
