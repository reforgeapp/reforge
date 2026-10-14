package integration

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/autopilot"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/policy"
)

func TestMaintenanceSetupScopesAndFencesDelegation(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	repo, other, model, pool := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		queries := []struct {
			sql  string
			args []any
		}{
			{`UPDATE connections SET state='healthy' WHERE org_id=$1 AND id=$2`, []any{f.org, f.connection.ID}},
			{`INSERT INTO repositories(org_id,id,connection_id,native_id,name) VALUES($1,$2,$4,'maintained','maintained'),($1,$3,$4,'untouched','untouched')`, []any{f.org, repo, other, f.connection.ID}},
			{`INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','compatible','Approved','https://model.example','{"model":"approved"}','healthy')`, []any{f.org, model}},
			{`INSERT INTO runner_pools(org_id,id,name,state,builtin) VALUES($1,$2,'Built in','active',true)`, []any{f.org, pool}},
			{`INSERT INTO runners(org_id,id,pool_id,name,credential_hash,credential_expires_at) VALUES($1,$2,$3,'Runner','fixture',now()+interval '1 hour')`, []any{f.org, domain.NewID(), pool}},
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
	seedPricedFixtureRoute(t, f, model, "approved")
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	budgets := budget.New(f.db, f.identity, nil, nil)
	repairs := repair.New(f.db, f.identity, nil, nil, nil, policies, budgets, nil, nil, nil, map[string]string{"javascript": "sha256:" + strings.Repeat("a", 64)})
	service := autopilot.New(f.db, f.identity, repairs, nil, budgets, policies)
	command := autopilot.MaintenanceCommand{Mode: "propose", MicroUSD: 10_000_000, Continuous: true}
	plan, err := service.PreviewMaintenance(ctx, f.owner, f.org, repo, command)
	if err != nil || !plan.Ready || !slices.Contains(plan.Actions, policy.Repair) || slices.Contains(plan.Actions, policy.Merge) {
		t.Fatalf("preview: %+v %v", plan, err)
	}
	command.Proof = "stale"
	if _, err := service.ConfigureMaintenance(ctx, f.owner, f.org, repo, command, "test"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale proof accepted: %v", err)
	}
	command.Proof = plan.Proof
	if _, err := service.ConfigureMaintenance(ctx, f.owner, f.org, repo, command, "test"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		var enabled, continuous, otherBinding, assigned bool
		var cap int64
		if err := tx.QueryRow(ctx, `SELECT (SELECT enabled FROM autopilot_settings WHERE org_id=$1),(SELECT continuous FROM autopilot_requests WHERE org_id=$1 AND repository_id=$2),EXISTS(SELECT 1 FROM policy_bindings WHERE org_id=$1 AND scope_kind='repository' AND scope_id=$3),EXISTS(SELECT 1 FROM runner_pool_repositories WHERE org_id=$1 AND pool_id=$4 AND repository_id=$2),(SELECT (caps->>'micro_usd')::bigint FROM budget_limits WHERE org_id=$1 AND scope_kind='repository' AND scope_id=$2)`, f.org, repo, other, pool).Scan(&enabled, &continuous, &otherBinding, &assigned, &cap); err != nil {
			return err
		}
		if enabled || !continuous || otherBinding || !assigned || cap != command.MicroUSD {
			t.Fatalf("scope changed: enabled=%v continuous=%v other=%v assigned=%v cap=%d", enabled, continuous, otherBinding, assigned, cap)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	otherPolicy, err := policies.Resolve(ctx, f.owner, f.org, other)
	if err != nil || !slices.Contains(otherPolicy.Policy.Deny, policy.Repair) || !slices.Contains(otherPolicy.Policy.Deny, policy.Deploy) {
		t.Fatalf("first setup widened unconfigured repository permissions: %+v %v", otherPolicy, err)
	}
	defaults, err := repairs.Defaults(ctx, f.owner, f.org, repo)
	if err != nil || len(defaults.Blockers) != 0 || defaults.ModelConnectionID != model || defaults.RunnerPoolID != pool || defaults.Ceiling == nil || *defaults.Ceiling != command.MicroUSD {
		t.Fatalf("manual approved defaults: %+v %v", defaults, err)
	}
	unconfigured, err := repairs.Defaults(ctx, f.owner, f.org, other)
	if err != nil || len(unconfigured.Blockers) == 0 || unconfigured.ModelConnectionID != "" {
		t.Fatalf("unconfigured repository gained repair defaults: %+v %v", unconfigured, err)
	}
	if _, err := service.ConfigureMaintenance(ctx, f.owner, f.org, repo, command, "repeat"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("replayed proof accepted: %v", err)
	}
	status, err := service.MaintenanceStatus(ctx, f.owner, f.org, repo)
	if err != nil || status.Mode != "propose" || !status.Continuous || status.MicroUSD != command.MicroUSD {
		t.Fatalf("status: %+v %v", status, err)
	}
	for _, scope := range []budget.Scope{{Kind: "organisation", ID: f.org}, {Kind: "repository", ID: repo}} {
		limit, err := budgets.GetLimit(ctx, f.owner, f.org, scope)
		if err != nil || !limit.Start.IsZero() || !limit.End.IsZero() {
			t.Fatalf("setup budget dates: %+v %v", limit, err)
		}
		limit.Paused = true
		limit, err = budgets.PutLimit(ctx, f.owner, f.org, limit, limit.Version, "pause-setup-budget")
		if err != nil || !limit.Paused {
			t.Fatalf("setup budget cannot pause: %+v %v", limit, err)
		}
		limit.Paused = false
		limit, err = budgets.PutLimit(ctx, f.owner, f.org, limit, limit.Version, "resume-setup-budget")
		if err != nil || limit.Paused || limit.Caps.MicroUSD == nil || *limit.Caps.MicroUSD != command.MicroUSD {
			t.Fatalf("setup budget cannot resume with its ceiling: %+v %v", limit, err)
		}
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE budget_limits SET caps=caps||'{"concurrency":1,"tokens":6000}',period='daily',version=version+1 WHERE org_id=$1 AND scope_kind='repository' AND scope_id=$2`, f.org, repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	command.MicroUSD = 7_500_000
	plan, err = service.PreviewMaintenance(ctx, f.owner, f.org, repo, command)
	if err != nil || plan.BudgetPeriod != "daily" || plan.OrgMicroUSD == nil || *plan.OrgMicroUSD != 10_000_000 || plan.CreatesOrgLimit {
		t.Fatalf("budget preview: %+v %v", plan, err)
	}
	command.Proof = plan.Proof
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE budget_limits SET caps=jsonb_set(caps,'{micro_usd}','5000000'),version=version+1 WHERE org_id=$1 AND scope_kind='organisation'`, f.org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfigureMaintenance(ctx, f.owner, f.org, repo, command, "changed-org-budget"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale budget preview accepted: %v", err)
	}
	plan, err = service.PreviewMaintenance(ctx, f.owner, f.org, repo, command)
	if err != nil {
		t.Fatal(err)
	}
	command.Proof = plan.Proof
	if _, err = service.ConfigureMaintenance(ctx, f.owner, f.org, repo, command, "preserve-caps"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		var concurrency, tokens, usd int64
		var period string
		if err := tx.QueryRow(ctx, `SELECT (caps->>'concurrency')::bigint,(caps->>'tokens')::bigint,(caps->>'micro_usd')::bigint,period FROM budget_limits WHERE org_id=$1 AND scope_kind='repository' AND scope_id=$2`, f.org, repo).Scan(&concurrency, &tokens, &usd, &period); err != nil {
			return err
		}
		if concurrency != 1 || tokens != 6000 || usd != command.MicroUSD || period != "daily" {
			t.Fatalf("setup erased existing limits: %d %d %d %s", concurrency, tokens, usd, period)
		}
		_, err := tx.Exec(ctx, `UPDATE budget_limits SET caps='{ "concurrency":2 }',version=version+1 WHERE org_id=$1 AND scope_kind='organisation'`, f.org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	plan, err = service.PreviewMaintenance(ctx, f.owner, f.org, repo, command)
	if err != nil || plan.Ready || plan.CreatesOrgLimit || !slices.Contains(plan.Blockers, "Operator: set an organisation dollar ceiling") {
		t.Fatalf("missing existing dollar ceiling marked ready: %+v %v", plan, err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE budget_limits SET caps='{ "micro_usd":5000000 }',version=version+1 WHERE org_id=$1 AND scope_kind='organisation'`, f.org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err := policies.Resolve(ctx, f.owner, f.org, repo)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Policy.Defaults.ConnectionID != model {
		t.Fatalf("approved connection not pinned: %+v", resolved.Policy.Defaults)
	}
	replacement, finding := domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE connections SET state='degraded' WHERE org_id=$1 AND id=$2`, f.org, model); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','compatible','Paid mirror','https://paid.example','{"model":"approved"}','healthy')`, f.org, replacement); err != nil {
			return err
		}

		_, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'repository_review','checkout','code_defect','high','Checkout failed','{"complete":true,"blockers":[]}',$4)`, f.org, finding, repo, strings.Repeat("f", 64))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	seedPricedFixtureRoute(t, f, replacement, "approved")
	command.ConnectionID = ""
	plan, err = service.PreviewMaintenance(ctx, f.owner, f.org, repo, command)
	if err != nil || plan.Ready || plan.ConnectionID != "" {
		t.Fatalf("setup silently replaced unavailable approved endpoint: %+v %v", plan, err)
	}
	if err = service.Step(ctx, f.org); err != nil {
		t.Fatalf("unavailable approved endpoint observation: %v", err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		var kind string
		if err := tx.QueryRow(ctx, `SELECT blocker_kind FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2`, f.org, finding).Scan(&kind); err != nil {
			return err
		}
		if kind != "reconnect" {
			t.Fatalf("automatic paid fallback: %s", kind)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	command.ConnectionID = replacement
	plan, err = service.PreviewMaintenance(ctx, f.owner, f.org, repo, command)
	if err != nil || !plan.Ready || plan.ConnectionID != replacement {
		t.Fatalf("explicit replacement unavailable: %+v %v", plan, err)
	}
	command.Proof = plan.Proof
	if _, err = service.ConfigureMaintenance(ctx, f.owner, f.org, repo, command, "replace-approved-endpoint"); err != nil {
		t.Fatal(err)
	}
	resolved, err = policies.Resolve(ctx, f.owner, f.org, repo)
	if err != nil || resolved.Policy.Defaults.ConnectionID != replacement {
		t.Fatalf("replacement not bound: %+v %v", resolved.Policy.Defaults, err)
	}
	var orgVersion int64
	for _, layer := range resolved.Layers {
		if layer.Scope.Kind == "organisation" {
			orgVersion = layer.BindingVersion
		}
	}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, policy.Policy{Schema: "maintenance/v1", Deny: []policy.Action{policy.Merge}}, "Parent restriction", "test")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := policies.Simulate(ctx, f.owner, f.org, version.ID, repo, "", policy.Input{Action: policy.Merge})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policies.Activate(ctx, f.owner, f.org, version.ID, repo, "", orgVersion, sim.Hash, "Parent restriction", "test"); err != nil {
		t.Fatal(err)
	}
	command.Mode, command.Proof = "recover", ""
	plan, err = service.PreviewMaintenance(ctx, f.owner, f.org, repo, command)
	if err != nil || slices.Contains(plan.Actions, policy.Merge) {
		t.Fatalf("parent restriction lost: %+v %v", plan, err)
	}
	command.Mode = "observe"
	plan, err = service.PreviewMaintenance(ctx, f.owner, f.org, repo, command)
	if err != nil {
		t.Fatal(err)
	}
	command.Proof = plan.Proof
	if _, err := service.ConfigureMaintenance(ctx, f.owner, f.org, repo, command, "scan-only"); err != nil {
		t.Fatal(err)
	}
	status, err = service.MaintenanceStatus(ctx, f.owner, f.org, repo)
	if err != nil || status.Mode != "observe" || status.Continuous {
		t.Fatalf("scan-only retains delegation: %+v %v", status, err)
	}
}
