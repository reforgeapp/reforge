package delivery

import (
	"context"
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/release"
	"github.com/reforgeapp/reforge/pkg/store"
)

func TestLinkedGitOpsScopeRequiresCurrentRequestConfigurationAndGrants(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	if u, err := url.Parse(raw); err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", ListenAddress: "127.0.0.1:8080", Edition: "self-hosted", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	org, user := domain.NewID(), domain.NewID()
	source, target, sibling := domain.NewID(), domain.NewID(), domain.NewID()
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'linked-delivery-test',$1::text,'Owner','owner@example.test')`, user)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	mutation := func(query string, args ...any) {
		t.Helper()
		if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, query, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	mutation(`INSERT INTO organisations(id,name) VALUES($1,'Linked delivery')`, org)
	mutation(`INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, user)
	for _, repo := range []string{source, target, sibling} {
		mutation(`INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,$2::text)`, org, repo)
	}
	mutation(`INSERT INTO autopilot_requests(org_id,repository_id,requested_by,continuous) VALUES($1,$2,$3,true)`, org, source, user)
	mutation(`INSERT INTO gitops_configurations(org_id,environment,source_repository_id,delivery_repository_id,version,document) VALUES($1,'production',$2,$3,1,'{"enabled":true}')`, org, source, target)
	parent, err := auth.NewAutomationSession(org, user, domain.NewID(), []string{source}, func(ctx context.Context, tx pgx.Tx) error {
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM autopilot_requests WHERE org_id=$1 AND repository_id=$2 AND requested_by=$3 AND continuous)`, org, source, user).Scan(&active); err != nil {
			return err
		}
		if !active {
			return auth.ErrForbidden
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Releases: &release.Service{DB: db, Auth: identity}}
	child, err := service.gitopsSession(ctx, parent, org, source, "production")
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.WithActor(ctx, child, org, func(_ pgx.Tx, a domain.Actor) error {
		if a.AllRepositories || !auth.CanReadRepository(a, source) || !auth.CanReadRepository(a, target) || auth.CanReadRepository(a, sibling) {
			t.Fatalf("linked path widened beyond two repositories: %+v", a)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reject := func() {
		t.Helper()
		if err := identity.WithActor(ctx, child, org, func(pgx.Tx, domain.Actor) error { return nil }); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("revoked linked path accepted: %v", err)
		}
	}
	mutation(`UPDATE gitops_configurations SET version=2 WHERE org_id=$1`, org)
	reject()
	mutation(`UPDATE gitops_configurations SET version=1 WHERE org_id=$1`, org)
	mutation(`DELETE FROM autopilot_requests WHERE org_id=$1 AND repository_id=$2`, org, source)
	reject()
	mutation(`INSERT INTO autopilot_requests(org_id,repository_id,requested_by,continuous) VALUES($1,$2,$3,true)`, org, source, user)
	mutation(`UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, org, user)
	mutation(`INSERT INTO member_repositories(org_id,user_id,repository_id) VALUES($1,$2,$3)`, org, user, source)
	reject()
}
