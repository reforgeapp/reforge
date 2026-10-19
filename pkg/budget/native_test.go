package budget

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func TestNativeSubscriptionBudgetAtZeroUSD(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	nativeConnection := domain.NewID()
	modelName := "codex-test-model"
	if err := f.tx(func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,verified_at) VALUES($1,$2,'agent','codex','Codex subscription','https://native.invalid',$3,'healthy',now())`, f.lease.OrgID, nativeConnection, []byte(`{"model":"`+modelName+`","managed":"native_runtime","auth_kind":"official_runtime","billing_route":"subscription","namespace":"agent-runtime"}`)); err != nil {
			return err
		}
		if err := ConfigureNativeTx(ctx, tx, f.lease.OrgID, nativeConnection, modelName, domain.NewID()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE workflow_tasks SET model_connection_id=$3,model_route='default' WHERE org_id=$1 AND id=$2`, f.lease.OrgID, f.lease.TaskID, nativeConnection)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.limit(t, Scope{Kind: "repository", ID: f.lease.RepositoryID}, 0)
	f.limit(t, Scope{Kind: "campaign", ID: f.campaign}, 0)
	if err := f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE budget_limits SET period='daily',caps='{"micro_usd":0,"tokens":100000,"milliseconds":1000000,"requests":1000,"concurrency":2}'::jsonb,version=version+1 WHERE org_id=$1 AND scope_kind='organisation' AND scope_id=$1`, f.lease.OrgID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	eligible := func(id string) bool {
		t.Helper()
		var result bool
		err := f.tx(func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT `+AutomaticConnectionSQL("c", "r")+` FROM connections c JOIN budget_routes r ON r.org_id=c.org_id AND r.connection_id=c.id AND r.model=c.settings->>'model' WHERE c.org_id=$1 AND c.id=$2`, f.lease.OrgID, id).Scan(&result)
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if !eligible(nativeConnection) {
		t.Fatal("verified native subscription row was not eligible for automatic use")
	}
	if err := f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET settings=jsonb_set(settings,'{auth_kind}','"api_key"') WHERE org_id=$1 AND id=$2`, f.lease.OrgID, nativeConnection)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if eligible(nativeConnection) {
		t.Fatal("native runtime without official auth was eligible for automatic use")
	}
	if err := f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET settings=jsonb_set(settings,'{auth_kind}','"official_runtime"') WHERE org_id=$1 AND id=$2`, f.lease.OrgID, nativeConnection)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.tx(func(tx pgx.Tx) error {
		if err := f.s.ModelHeadroomTx(ctx, tx, f.lease.OrgID, nativeConnection); err != nil {
			return err
		}
		return f.s.RepositoryModelHeadroomTx(ctx, tx, f.lease.OrgID, f.lease.RepositoryID, nativeConnection)
	}); err != nil {
		t.Fatalf("zero-dollar native headroom rejected: %v", err)
	}
	q := Quote{OperationID: domain.NewID(), Model: modelName, Route: "default", RouteVersion: 1, InputTokens: 10, MaxOutputTokens: 10, MaxMilliseconds: 1000, MaxRequests: 1}
	reservation, err := f.reserve(q)
	if err != nil || reservation.Maximum.MicroUSD != 0 || reservation.State != "reserved" {
		t.Fatalf("zero-dollar native reservation = %+v, %v", reservation, err)
	}
	if err := f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_tasks SET model_connection_id=$3,model_route='api' WHERE org_id=$1 AND id=$2`, f.lease.OrgID, f.lease.TaskID, f.connection)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	apiQuote := f.quote
	apiQuote.OperationID = domain.NewID()
	if _, err := f.reserve(apiQuote); !errors.Is(err, ErrCapacity) {
		t.Fatalf("priced API reservation at zero USD returned %v", err)
	}
	if err := f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE budget_limits SET paused=true WHERE org_id=$1 AND scope_kind='organisation' AND scope_id=$1`, f.lease.OrgID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.tx(func(tx pgx.Tx) error { return f.s.ModelHeadroomTx(ctx, tx, f.lease.OrgID, nativeConnection) }); !errors.Is(err, ErrRevoked) {
		t.Fatalf("paused native org budget returned %v", err)
	}
	if err := f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE budget_limits SET paused=false,caps=jsonb_set(caps,'{tokens}','1'::jsonb) WHERE org_id=$1 AND scope_kind='organisation' AND scope_id=$1`, f.lease.OrgID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.tx(func(tx pgx.Tx) error { return f.s.ModelHeadroomTx(ctx, tx, f.lease.OrgID, nativeConnection) }); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capped native org budget returned %v", err)
	}
}
