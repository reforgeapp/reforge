package budget

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestChatGPTQuotaReservationAndAccountBinding(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.limit(t, Scope{"campaign", f.campaign}, 0)
	if _, err := f.s.PutLimit(ctx, f.owner, f.lease.OrgID, Limit{Scope: Scope{"organisation", f.lease.OrgID}, Period: "monthly", Caps: ChatGPTCaps(1)}, 0, "request limit"); err != nil {
		t.Fatal(err)
	}
	if err := f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET provider='openai',endpoint='https://api.openai.com/v1',settings='{"model":"fixture","auth_kind":"chatgpt_oauth","billing_route":"subscription","profile":"chatgpt_plan"}' WHERE org_id=$1 AND id=$2`, f.lease.OrgID, f.connection)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.tx(func(tx pgx.Tx) error { return f.s.ModelHeadroomTx(ctx, tx, f.lease.OrgID, f.connection, "") }); err != nil {
		t.Fatal(err)
	}
	route := Route{ConnectionID: f.connection, Model: "fixture", Name: "api", Mode: "quota", MaxInputTokens: 100, MaxOutputTokens: 100, MaxMilliseconds: 10000, MaxRequests: 1}
	route, err := f.s.PutRoute(ctx, f.owner, f.lease.OrgID, route, 1, "subscription route")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.tx(func(tx pgx.Tx) error {
		var err error
		route, err = f.s.QualifyRouteTx(ctx, tx, f.lease.OrgID, f.connection, "fixture", "api", route.Version, "chatgpt-plan/v1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	quote := f.quote
	quote.RouteVersion = route.Version
	r, err := f.reserve(quote)
	if err != nil || r.Maximum.MicroUSD != 0 || r.Maximum.Requests != 1 {
		t.Fatalf("subscription reserve failed: %+v %v", r, err)
	}
	if err := f.tx(func(tx pgx.Tx) error {
		_, err := f.s.MarkDispatchedTx(ctx, tx, f.lease, r.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	actual, err := ObservedAmount(route, 12, 7, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.tx(func(tx pgx.Tx) error {
		_, err := f.s.SettleTx(ctx, tx, f.lease.OrgID, r.ID, Settlement{Known: true, Actual: actual, Reference: "completed subscription response"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	limit, err := f.s.GetLimit(ctx, f.owner, f.lease.OrgID, Scope{"organisation", f.lease.OrgID})
	if err != nil || limit.Spent.MicroUSD != 0 || limit.Spent.Requests != 1 || limit.Spent.Tokens != 19 || limit.Held.Requests != 0 {
		t.Fatalf("subscription settlement wrong: %+v %v", limit, err)
	}
	quote.OperationID = f.lease.OperationID
	if _, err := f.reserve(quote); !errors.Is(err, ErrCapacity) {
		t.Fatalf("request cap ignored: %v", err)
	}
	if err := f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET settings=jsonb_set(settings,'{billing_route}','"direct_api"') WHERE org_id=$1 AND id=$2`, f.lease.OrgID, f.connection)
		if err != nil {
			return err
		}
		_, err = currentConnection(ctx, tx, f.lease.OrgID, f.connection, route)
		return err
	}); !errors.Is(err, ErrRevoked) {
		t.Fatalf("quota used API billing identity: %v", err)
	}
}

func TestAutomaticSubscriptionEligibilityMatchesSQL(t *testing.T) {
	f := setup(t)
	base := Route{ConnectionID: f.connection, Model: "fixture", Name: "default", Mode: "quota", MaxInputTokens: 100, MaxOutputTokens: 100, MaxMilliseconds: 10000, MaxRequests: 1, Qualified: true, QualificationRef: "chatgpt-plan/v1"}
	cases := []Route{base, base, base, base, base, base}
	cases[1].Qualified = false
	cases[2].QualificationRef = "custom-command-evidence"
	cases[3].RequestMicroUSD = 1
	cases[4].Paused = true
	cases[5].MaxRequests = 2
	for i, route := range cases {
		raw, _ := json.Marshal(route)
		if err := f.tx(func(tx pgx.Tx) error {
			var sqlEligible bool
			err := tx.QueryRow(context.Background(), `SELECT `+AutomaticRouteSQL("r")+` FROM (SELECT $1::jsonb AS config,$2::uuid AS connection_id,'fixture'::text AS model,'default'::text AS name) r`, raw, f.connection).Scan(&sqlEligible)
			if err == nil && sqlEligible != AutomaticEligible(route) {
				t.Errorf("SQL eligibility diverged for case %d", i)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAPIHeadroomKeepsDollarCeilingWithQuotaCaps(t *testing.T) {
	f := setup(t)
	f.limit(t, Scope{"organisation", f.lease.OrgID}, 0)
	if err := f.tx(func(tx pgx.Tx) error {
		return f.s.ModelHeadroomTx(context.Background(), tx, f.lease.OrgID, f.connection, "")
	}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("API dollar ceiling bypassed by quota caps: %v", err)
	}
}
