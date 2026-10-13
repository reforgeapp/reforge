package autopilot

import (
	"context"
	"errors"
	"net/url"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/store"
)

func TestContinuousMaintenanceScopeRotationAndRevocation(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	if u, err := url.Parse(raw); err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	org := domain.NewID()
	users := []string{domain.NewID(), domain.NewID()}
	repos := []string{domain.NewID(), domain.NewID(), domain.NewID()}
	for _, user := range users {
		if err := db.Identity(ctx, user, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'maintenance-scope-test',$1::text,'Owner',$1::text||'@example.test')`, user)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Tenant(ctx, org, users[0], func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Scoped maintenance')`, org); err != nil {
			return err
		}
		for _, user := range users {
			if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, user); err != nil {
				return err
			}
		}
		for i, repo := range repos {
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,$2::text)`, org, repo); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO autopilot_requests(org_id,repository_id,requested_by,continuous,requested_at) VALUES($1,$2,$3,$4,now()-make_interval(days=>$5))`, org, repo, users[min(i, 1)], i < 2, i+2); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := &Service{db: db, auth: identity}
	first, scope, err := service.session(ctx, org)
	if err != nil || !scope.enabled || scope.portfolio || len(scope.repositories) != 1 || slices.Contains(scope.repositories, repos[2]) {
		t.Fatalf("scope: %+v %v", scope, err)
	}
	if err := identity.WithActor(ctx, first, org, func(_ pgx.Tx, a domain.Actor) error {
		if a.AllRepositories || len(a.RepositoryIDs) != 1 || !slices.Equal(a.RepositoryIDs, scope.repositories) {
			t.Fatalf("grant widened: %+v", a)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	second, next, err := service.session(ctx, org)
	if err != nil || second.User.ID == first.User.ID || slices.Equal(next.repositories, scope.repositories) {
		t.Fatalf("actor rotation starved: %+v %v", next, err)
	}
	if err := db.Tenant(ctx, org, first.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM autopilot_requests WHERE org_id=$1 AND repository_id=ANY($2::uuid[])`, org, scope.repositories)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := identity.WithActor(ctx, first, org, func(pgx.Tx, domain.Actor) error { return nil }); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("revoked scope accepted: %v", err)
	}
}
