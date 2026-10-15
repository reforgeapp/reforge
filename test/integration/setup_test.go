package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/insights"
	"github.com/reforgeapp/reforge/pkg/policy"
)

func TestSetupRequiresOneRepositoryExecutionPath(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	repo, other, pool, model, unpriced := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		queries := []struct {
			sql  string
			args []any
		}{
			{`UPDATE connections SET state='healthy' WHERE org_id=$1 AND id=$2`, []any{f.org, f.connection.ID}},
			{`INSERT INTO repositories(org_id,id,connection_id,native_id,name) VALUES($1,$2,$4,'main','a-main'),($1,$3,$4,'other','z-other')`, []any{f.org, repo, other, f.connection.ID}},
			{`INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,created_at) VALUES($1,$2,'model','compatible','Unpriced','https://model.example','{"model":"unpriced"}','healthy',now()-interval '1 day'),($1,$3,'model','compatible','Priced','https://model.example','{"model":"priced"}','healthy',now())`, []any{f.org, unpriced, model}},
			{`INSERT INTO runner_pools(org_id,id,name,state) VALUES($1,$2,'Other repo runner','active')`, []any{f.org, pool}},
			{`INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id) VALUES($1,$2,$3)`, []any{f.org, pool, other}},
			{`INSERT INTO runners(org_id,id,pool_id,name,credential_hash,credential_expires_at) VALUES($1,$2,$3,'Runner','fixture',now()+interval '1 hour')`, []any{f.org, domain.NewID(), pool}},
			{`INSERT INTO maintenance_scans(org_id,repository_id,state,observed_at) VALUES($1,$2,'complete',now())`, []any{f.org, other}},
		}
		for _, query := range queries {
			if _, err := tx.Exec(ctx, query.sql, query.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	seedPricedFixtureRoute(t, f, model, "priced")
	service := insights.New(f.identity)
	resolve := func(context.Context, pgx.Tx, string, string) (policy.Resolved, error) {
		return policy.Resolved{Hash: "valid", Policy: policy.Policy{Schema: "maintenance/v1"}}, nil
	}
	setup, err := service.Setup(ctx, f.owner, f.org, resolve, true, false, repo)
	if err != nil {
		t.Fatal(err)
	}
	steps := map[string]insights.SetupStep{}
	for _, step := range setup.Steps {
		steps[step.ID] = step
	}
	if _, exists := steps["delivery"]; exists {
		t.Fatal("optional deployment blocks maintenance onboarding")
	}
	if steps["runner"].Done || steps["assignment"].Done || steps["scan"].Done || !steps["policy"].Done || !steps["pricing"].Done || steps["model"].ConnectionID != model {
		t.Fatalf("unrelated resources satisfy setup: %+v", steps)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id) VALUES($1,$2,$3)`, f.org, pool, repo); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE runners SET credential_expires_at=now()-interval '1 second' WHERE org_id=$1`, f.org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	setup, err = service.Setup(ctx, f.owner, f.org, resolve, true, false, repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range setup.Steps {
		if step.ID == "runner" && step.Done {
			t.Fatal("expired runner credential marked ready")
		}
	}
	resolve = func(context.Context, pgx.Tx, string, string) (policy.Resolved, error) {
		return policy.Resolved{Hash: "valid", Policy: policy.Policy{Schema: "maintenance/v1", Deny: []policy.Action{policy.Repair}}}, nil
	}
	setup, err = service.Setup(ctx, f.owner, f.org, resolve, false, false, repo)
	if err != nil || len(setup.Steps) != 4 {
		t.Fatalf("scan-only setup requires repair infrastructure: %+v %v", setup, err)
	}
}
