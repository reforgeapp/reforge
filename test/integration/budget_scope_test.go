package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func TestScopedOwnerCannotChangeSharedSpendingAuthority(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	repo, model := domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,'owned','owned')`, f.org, repo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','compatible','Local','https://model.example','{"model":"local"}','healthy')`, f.org, model); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO member_repositories(org_id,user_id,repository_id) VALUES($1,$2,$3)`, f.org, f.owner.User.ID, repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := budget.New(f.db, f.identity, nil, nil)
	cap := int64(5_000_000)
	for _, scope := range []budget.Scope{{Kind: "organisation", ID: f.org}, {Kind: "connection", ID: model}} {
		if _, err := service.PutLimit(ctx, f.owner, f.org, budget.Limit{Scope: scope, Period: "monthly", Caps: budget.Caps{MicroUSD: &cap}}, 0, "shared"); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("scoped owner changed shared ceiling %s: %v", scope.Kind, err)
		}
	}
	r := budget.Route{ConnectionID: model, Model: "local", Name: "default", Mode: "priced", PricingVersion: "fixture-v1", MaxInputTokens: 64_000, MaxOutputTokens: 4096, MaxMilliseconds: 600_000, MaxRequests: 1}
	if _, err := service.PutRoute(ctx, f.owner, f.org, r, 0, "shared-price"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("scoped owner changed shared pricing: %v", err)
	}
	if _, err := service.PutLimit(ctx, f.owner, f.org, budget.Limit{Scope: budget.Scope{Kind: "repository", ID: repo}, Period: "monthly", Caps: budget.Caps{MicroUSD: &cap}}, 0, "owned-ceiling"); err != nil {
		t.Fatalf("owned repository ceiling blocked: %v", err)
	}
}
