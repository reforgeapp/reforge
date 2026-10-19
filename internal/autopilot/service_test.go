package autopilot_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/autopilot"
	"github.com/reforgeapp/reforge/internal/budget"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/policy"
	"github.com/reforgeapp/reforge/internal/store"
)

func TestAutopilotReportsWhyItWaits(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	if u, e := url.Parse(raw); e != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	ctx := context.Background()
	db, e := store.Open(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	identity, e := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if e != nil {
		t.Fatal(e)
	}
	policies, e := policy.New(db, identity, policy.Policy{Schema: "maintenance/v1"})
	if e != nil {
		t.Fatal(e)
	}
	budgets := budget.New(db, identity, nil, nil)
	service := autopilot.New(db, identity, nil, nil, budgets, policies)
	owner := auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}}
	org := domain.NewID()
	if e = db.Identity(ctx, owner.User.ID, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'autopilot-test',$1::text,'Owner','owner@example.test')`, owner.User.ID); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, owner.ID, owner.User.ID, domain.NewID(), domain.NewID())
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if e = db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		for _, q := range []string{`INSERT INTO organisations(id,name) VALUES($1,'Autopilot')`, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,gen_random_uuid(),'r','team/repo')`} {
			args := []any{org}
			if q[12:23] == "memberships" {
				args = append(args, owner.User.ID)
			}
			if _, e := tx.Exec(ctx, q, args...); e != nil {
				return e
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	settings, e := service.Put(ctx, owner, org, true, 0, "test")
	if e != nil || !settings.Enabled || settings.Version != 1 {
		t.Fatalf("enable: %+v %v", settings, e)
	}
	if _, e = service.Put(ctx, owner, org, false, 0, "test"); !errors.Is(e, auth.ErrConflict) {
		t.Fatalf("stale version accepted: %v", e)
	}
	status := func(want string) {
		t.Helper()
		if e := service.Step(ctx, org); e != nil {
			t.Fatal(e)
		}
		if got, _ := service.Get(ctx, owner, org); got.Status != want || got.CheckedAt == nil {
			t.Fatalf("status %q, want %q", got.Status, want)
		}
	}
	status("Set a spend limit in Usage → Budgets")
	usd := int64(5_000_000)
	if _, e = budgets.PutLimit(ctx, owner, org, budget.Limit{Scope: budget.Scope{Kind: "organisation", ID: org}, Period: "daily", Caps: budget.Caps{MicroUSD: &usd}}, 0, "test"); e != nil {
		t.Fatal(e)
	}
	status("Add an AI model with pricing")
	if _, e = service.Put(ctx, owner, org, false, 1, "test"); e != nil {
		t.Fatal(e)
	}
	var repo string
	if e = db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text FROM repositories WHERE org_id=$1`, org).Scan(&repo)
	}); e != nil {
		t.Fatal(e)
	}
	if e = service.Request(ctx, owner, org, repo, "test"); e != nil {
		t.Fatal(e)
	}
	status("Add an AI model with pricing")
	if e = service.Request(ctx, owner, org, domain.NewID(), "test"); !errors.Is(e, auth.ErrForbidden) {
		t.Fatalf("unknown repository accepted: %v", e)
	}
}
