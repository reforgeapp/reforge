package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/autopilot"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/policy"
)

func TestNeedsKeepExhaustionAndBoundExplicitRetry(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	repo, finding, model := domain.NewID(), domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,connection_id,native_id,name) VALUES($1,$2,$3,'repo','repo')`, f.org, repo, f.connection.ID); err != nil {
			return err
		}
		for i := 0; i < 12; i++ {
			id := domain.NewID()
			if i == 0 {
				id = finding
			}
			hash := fmt.Sprintf("%064x", i+1)
			if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2::uuid,$3,$4,'repository_review',$2::text,'code_defect','high','Fix behaviour','{"complete":true,"blockers":[]}',$4)`, f.org, id, repo, hash); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO autopilot_attempts(org_id,finding_id,finding_version,outcome,reason,runs,blocker_kind,updated_at) VALUES($1,$2,1,'blocked','Six verified attempts failed',6,'exhausted',now()-interval '2 days')`, f.org, finding); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','compatible','Approved','https://model.example','{"model":"approved"}','healthy')`, f.org, model)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO budget_routes(org_id,connection_id,model,name,config,version) VALUES($1,$2,'approved','default','{"mode":"priced"}',1)`, f.org, model)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	budgets := budget.New(f.db, f.identity, nil, nil)
	cap := int64(20_000_000)
	if _, err = budgets.PutLimit(ctx, f.owner, f.org, budget.Limit{Scope: budget.Scope{Kind: "organisation", ID: f.org}, Period: "monthly", Caps: budget.Caps{MicroUSD: &cap}}, 0, "test"); err != nil {
		t.Fatal(err)
	}
	repairs := repair.New(f.db, f.identity, nil, nil, nil, policies, budgets, nil, nil, nil, nil)
	service := autopilot.New(f.db, f.identity, repairs, nil, budgets, policies)
	first, err := service.NeedsPage(ctx, f.owner, f.org, 1, "")
	if err != nil || first.Total != 1 || len(first.Items) != 1 || first.Items[0].Kind != "exhausted" || !first.Complete {
		t.Fatalf("automatic findings counted as decisions: %+v %v", first, err)
	}
	if err = service.Request(ctx, f.owner, f.org, repo, "check-again"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE maintenance_findings SET state='dismissed' WHERE org_id=$1 AND id<>$2`, f.org, finding)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = service.Step(ctx, f.org); err != nil {
		t.Fatalf("terminal attempt observation: %v", err)
	}
	var runs, max int
	var kind string
	state := func() {
		t.Helper()
		if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT runs,max_runs,blocker_kind FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2`, f.org, finding).Scan(&runs, &max, &kind)
		}); err != nil {
			t.Fatal(err)
		}
	}
	state()
	if runs != 6 || max != 6 || kind != "exhausted" {
		t.Fatalf("Check again erased exhaustion: %d %d %s", runs, max, kind)
	}
	command := autopilot.RetryCommand{FindingVersion: 1, Reason: "Reviewed prior failures; runner now supports checks"}
	if err = service.RetryFinding(ctx, f.owner, f.org, finding, command, "retry"); err != nil {
		t.Fatal(err)
	}
	state()
	if runs != 6 || max != 7 || kind != "" {
		t.Fatalf("retry allowance: %d %d %s", runs, max, kind)
	}
	if err = service.RetryFinding(ctx, f.owner, f.org, finding, command, "replay"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("replayed retry widened allowance: %v", err)
	}
	if err = f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE autopilot_attempts SET runs=7,outcome='blocked',blocker_kind='exhausted',updated_at=now()-interval '2 days' WHERE org_id=$1 AND finding_id=$2`, f.org, finding); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = service.RetryFinding(ctx, f.owner, f.org, finding, command, "removed-access"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("retry escaped repository scope: %v", err)
	}
}

func TestNeedsPagesAllScopedDecisions(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	repo := domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,'repo','repo')`, f.org, repo); err != nil {
			return err
		}
		for i := 0; i < 120; i++ {
			hash := fmt.Sprintf("%064x", i+1)
			if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'repository','permission','provider_access','high','Grant access','{"blockers":["Provider permission required"]}',$4)`, f.org, domain.NewID(), repo, hash); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := autopilot.New(f.db, f.identity, nil, nil, nil, nil)
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 4; page++ {
		result, err := service.NeedsPage(ctx, f.owner, f.org, 50, cursor)
		if err != nil || result.Total != 120 {
			t.Fatalf("page: %+v %v", result, err)
		}
		for _, item := range result.Items {
			if seen[item.ID] {
				t.Fatal("duplicate decision")
			}
			seen[item.ID] = true
		}
		if result.Complete {
			break
		}
		if result.NextCursor == "" {
			t.Fatal("missing cursor")
		}
		cursor = result.NextCursor
	}
	if len(seen) != 120 {
		t.Fatalf("decisions truncated: %d", len(seen))
	}
}

func TestNeedsResolveInfrastructureHoldsFromCurrentState(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	repo, model, pool := domain.NewID(), domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		queries := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO repositories(org_id,id,connection_id,native_id,name) VALUES($1,$2,$3,'shop','shop')`, []any{f.org, repo, f.connection.ID}},
			{`INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'repository_review','bug','code_defect','high','Checkout failed','{"complete":true}',$4)`, []any{f.org, domain.NewID(), repo, fmt.Sprintf("%064x", 1)}},
			{`INSERT INTO autopilot_requests(org_id,repository_id,requested_by,continuous) VALUES($1,$2,$3,true)`, []any{f.org, repo, f.owner.User.ID}},
			{`INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','compatible','Approved','https://model.example','{"model":"approved"}','degraded')`, []any{f.org, model}},
			{`UPDATE connections SET state='degraded' WHERE org_id=$1 AND id=$2`, []any{f.org, f.connection.ID}},
			{`INSERT INTO budget_routes(org_id,connection_id,model,name,config,version) VALUES($1,$2,'approved','default','{"mode":"priced"}',1)`, []any{f.org, model}},
			{`INSERT INTO budget_limits(org_id,scope_kind,scope_id,period,period_start,period_end,caps,paused,version) VALUES($1,'organisation',$1,'monthly',date_trunc('month',now()),date_trunc('month',now())+interval '1 month','{"micro_usd":5000000}',true,1)`, []any{f.org}},
		}
		for _, q := range queries {
			if _, err := tx.Exec(ctx, q.sql, q.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := autopilot.New(f.db, f.identity, nil, nil, nil, nil)
	result, err := service.NeedsPage(ctx, f.owner, f.org, 50, "")
	if err != nil || result.Total != 4 {
		t.Fatalf("missing infrastructure handoff: %+v %v", result, err)
	}
	for _, item := range result.Items {
		if item.RepositoryID != repo || item.URL == "" || item.Owner == "" || item.Label == "" {
			t.Fatalf("handoff lacks command/scope/owner: %+v", item)
		}
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE connections SET state='healthy' WHERE org_id=$1`, f.org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE budget_limits SET paused=false WHERE org_id=$1`, f.org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runner_pools(org_id,id,name,state) VALUES($1,$2,'Local','active')`, f.org, pool); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id) VALUES($1,$2,$3)`, f.org, pool, repo); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO runners(org_id,id,pool_id,name,credential_hash,credential_expires_at) VALUES($1,$2,$3,'Local','fixture',now()+interval '1 hour')`, f.org, domain.NewID(), pool)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	result, err = service.NeedsPage(ctx, f.owner, f.org, 50, "")
	if err != nil || result.Total != 0 {
		t.Fatalf("resolved holds still ask for decisions: %+v %v", result, err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO budget_spend(org_id,scope_kind,scope_id,period_start,amount) VALUES($1,'organisation',$1,date_trunc('month',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC','{"micro_usd":5000000}')`, f.org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	result, err = service.NeedsPage(ctx, f.owner, f.org, 50, "")
	if err != nil || result.Total != 1 || result.Items[0].Kind != "raise_limit" {
		t.Fatalf("exhausted allowance not actionable: %+v %v", result, err)
	}
}
