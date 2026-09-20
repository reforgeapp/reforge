package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
)

func TestScopedAutomationAuthority(t *testing.T) {
	db := authDB(t)
	ctx := context.Background()
	identity, _ := identityServer(t, db, authConfig())
	user, cookie := fixtureIdentity(t, db)
	org, repo, hidden := domain.NewID(), domain.NewID(), domain.NewID()
	e := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Automation authority')`, org); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'admin',true)`, org, user); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Allowed'),($1,$3::uuid,$3::text,'Outside grant')`, org, repo, hidden)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	web, e := identity.Authenticate(ctx, cookie.Value)
	if e != nil {
		t.Fatal(e)
	}
	revoked := false
	called := false
	grantID := domain.NewID()
	repos := []string{repo}
	grant, e := auth.NewAutomationSession(org, user, grantID, repos, func(context.Context, pgx.Tx) error {
		called = true
		if revoked {
			return auth.ErrForbidden
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	repos[0] = hidden
	a, e := identity.ResolveActor(ctx, grant, org)
	if e != nil {
		t.Fatal(e)
	}
	if !called || a.Role != domain.Maintainer || a.AllRepositories || len(a.RepositoryIDs) != 1 || a.RepositoryIDs[0] != repo || grant.AutomationID() != grantID {
		t.Fatalf("incorrect scoped actor: %+v", a)
	}
	a.RepositoryIDs[0] = hidden
	if _, e = identity.RequireRepository(ctx, grant, org, repo); e != nil {
		t.Fatal(e)
	}
	if _, e = identity.RequireRepository(ctx, grant, org, hidden); !errors.Is(e, auth.ErrForbidden) {
		t.Fatalf("out-of-scope repository: %v", e)
	}
	if _, e = identity.ResolveActor(ctx, grant, domain.NewID()); !errors.Is(e, auth.ErrForbidden) {
		t.Fatalf("cross-tenant grant: %v", e)
	}
	tampered := grant
	tampered.User.ID = domain.NewID()
	if _, e = identity.ResolveActor(ctx, tampered, org); !errors.Is(e, auth.ErrForbidden) {
		t.Fatalf("changed originator: %v", e)
	}
	raw, e := json.Marshal(grant)
	if e != nil {
		t.Fatal(e)
	}
	var decoded auth.Session
	if e = json.Unmarshal(raw, &decoded); e != nil {
		t.Fatal(e)
	}
	if decoded.AutomationID() != "" {
		t.Fatal("automation authority serialized")
	}
	if _, e = identity.ResolveActor(ctx, decoded, org); e == nil {
		t.Fatal("wire JSON fabricated automation authority")
	}
	if e = identity.Logout(ctx, web); e != nil {
		t.Fatal(e)
	}
	if _, e = identity.ResolveActor(ctx, grant, org); e != nil {
		t.Fatalf("approved automation wrongly bound to web login: %v", e)
	}
	mutated := false
	if e = identity.WithMutation(ctx, grant, org, func(pgx.Tx, domain.Actor) error { mutated = true; return nil }); e != nil || !mutated {
		t.Fatalf("grant transaction: %v", e)
	}
	revoked = true
	mutated = false
	if e = identity.WithMutation(ctx, grant, org, func(pgx.Tx, domain.Actor) error { mutated = true; return nil }); !errors.Is(e, auth.ErrForbidden) || mutated {
		t.Fatalf("revoked grant mutation: %v", e)
	}
	revoked = false
	e = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, org, user)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = identity.ResolveActor(ctx, grant, org); !errors.Is(e, auth.ErrForbidden) {
		t.Fatalf("repository binding removed: %v", e)
	}
	e = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO member_repositories(org_id,user_id,repository_id) VALUES($1,$2,$3)`, org, user, repo)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = identity.ResolveActor(ctx, grant, org); e != nil {
		t.Fatalf("explicit current binding: %v", e)
	}
	e = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE memberships SET role='viewer' WHERE org_id=$1 AND user_id=$2`, org, user)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = identity.ResolveActor(ctx, grant, org); !errors.Is(e, auth.ErrForbidden) {
		t.Fatalf("demoted originator: %v", e)
	}
}
