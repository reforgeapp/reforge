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

func TestModelPricingSourcesEligibilityAndOverrides(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	official, custom, agent := domain.NewID(), domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','openai','Approved API','https://api.openai.com/v1','{"model":"gpt-6-luna"}','healthy'),($1,$3,'model','compatible','Local','http://127.0.0.1:11434/v1','{"model":"gpt-6-luna"}','healthy'),($1,$4,'agent','codex','Managed','https://runtime.example','{"model":"gpt-6-luna"}','disabled')`, f.org, official, custom, agent)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := budget.New(f.db, f.identity, nil, nil)
	options, err := service.Pricing(ctx, f.owner, f.org, official)
	if err != nil || options.Manual || options.Automatic || len(options.Presets) != 1 {
		t.Fatalf("unpriced route marked usable: %+v %v", options, err)
	}
	preset := options.Presets[0]
	if preset.Source == "" || preset.CheckedAt == "" {
		t.Fatal("published rates lack dated source")
	}
	if _, err = service.ApplyPricingPreset(ctx, f.owner, f.org, custom, preset.ID, 0, "custom-preset"); !errors.Is(err, budget.ErrInvalid) {
		t.Fatalf("official pricing attributed to custom endpoint: %v", err)
	}
	route, err := service.ApplyPricingPreset(ctx, f.owner, f.org, official, preset.ID, 0, "apply")
	if err != nil || route.CostBasis != "published_ceiling" || route.PricingSource != preset.Source || route.PricingCheckedAt != preset.CheckedAt {
		t.Fatalf("source lost: %+v %v", route, err)
	}
	if _, err = service.ApplyPricingPreset(ctx, f.owner, f.org, official, preset.ID, 0, "stale"); !errors.Is(err, budget.ErrConflict) {
		t.Fatalf("stale pricing update accepted: %v", err)
	}
	options, err = service.Pricing(ctx, f.owner, f.org, official)
	if err != nil || !options.Manual || !options.Automatic {
		t.Fatalf("usable priced model unavailable: %+v %v", options, err)
	}
	forged := route
	forged.InputMicroUSDPerMillion = 0
	if _, err = service.PutRoute(ctx, f.owner, f.org, forged, route.Version, "forged-source"); !errors.Is(err, budget.ErrInvalid) {
		t.Fatalf("changed rates kept published attribution: %v", err)
	}
	override := forged
	override.PricingPresetID = ""
	override.PricingVersion = "operator-2026-10-13"
	overridden, err := service.PutRoute(ctx, f.owner, f.org, override, route.Version, "override")
	if err != nil || overridden.PricingSource != "" || overridden.PricingCheckedAt != "" || overridden.CostBasis != "operator_override" {
		t.Fatalf("override masquerades as source rate: %+v %v", overridden, err)
	}
	options, err = service.Pricing(ctx, f.owner, f.org, agent)
	if err != nil || options.Manual || options.Automatic || options.Reason != "Runtime account, custody and quota qualification missing; no paid fallback" {
		t.Fatalf("uncertified runtime eligibility: %+v %v", options, err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET state='degraded' WHERE org_id=$1 AND id=$2`, f.org, official)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	options, err = service.Pricing(ctx, f.owner, f.org, official)
	if err != nil || options.Manual || options.Automatic {
		t.Fatalf("degraded model eligible: %+v %v", options, err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET role='viewer' WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ApplyPricingPreset(ctx, f.owner, f.org, official, preset.ID, overridden.Version, "viewer"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer changed shared pricing: %v", err)
	}
}

func seedPricedFixtureRoute(t *testing.T, f *inventoryFixture, connection, model string) {
	t.Helper()
	service := budget.New(f.db, f.identity, nil, nil)
	r := budget.Route{ConnectionID: connection, Model: model, Name: "default", Mode: "priced", PricingVersion: "fixture-v1", MaxInputTokens: 64_000, MaxOutputTokens: 4096, MaxMilliseconds: 600_000, MaxRequests: 1}
	if _, err := service.PutRoute(context.Background(), f.owner, f.org, r, 0, "fixture"); err != nil {
		t.Fatal(err)
	}
}
