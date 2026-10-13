package budget

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/model"
)

func TestPublishedPresetAllowsBoundedFollowUpAndReservesCeiling(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := pricingPresets[0]
	if err := f.db.Tenant(ctx, f.lease.OrgID, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET provider=$3,endpoint='https://api.openai.com/v1',settings=jsonb_set(settings,'{model}',to_jsonb($4::text)) WHERE org_id=$1 AND id=$2`, f.lease.OrgID, f.connection, p.Provider, p.Model)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	route, err := f.s.ApplyPricingPreset(ctx, f.owner, f.lease.OrgID, f.connection, p.ID, 0, "published price")
	if err != nil {
		t.Fatal(err)
	}
	quote := Quote{InputTokens: model.MaxRequestBytes/2 + 4096, MaxOutputTokens: 4096, MaxMilliseconds: 600000, MaxRequests: 1}
	amount, err := estimate(route, quote)
	if err != nil || amount.MicroUSD <= 0 || amount.Tokens != quote.InputTokens+quote.MaxOutputTokens {
		t.Fatalf("bounded follow-up cannot reserve published ceiling: %+v %v", amount, err)
	}
	quote.InputTokens++
	if _, err = estimate(route, quote); err == nil {
		t.Fatal("larger than bounded model request accepted")
	}
}
