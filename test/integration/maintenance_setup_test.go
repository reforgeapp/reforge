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
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/mergecontrol"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/providers"
)

type unprotectedMergeSetupProvider struct {
	providers.Client
	db interface {
		Tenant(context.Context, string, string, func(pgx.Tx) error) error
	}
	connections *connections.Service
	rules       forge.Rules
}

func (p *unprotectedMergeSetupProvider) ForProtection(string, map[string]string) providers.Client {
	return p
}

func (p *unprotectedMergeSetupProvider) Read(ctx context.Context, org, id string, op privateconnector.Operation, check func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if op.Kind != privateconnector.ForgeReadRules {
		return privateconnector.Result{}, privateconnector.ErrUnsupported
	}
	err := p.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		c, err := p.connections.MetadataTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		return check(ctx, tx, c)
	})
	if err != nil {
		return privateconnector.Result{}, err
	}
	rules := p.rules
	return privateconnector.Result{Rules: &rules}, nil
}

func TestMaintenanceSetupAcceptsFreshUnprotectedOrBypassMerge(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	repo, model, pool := domain.NewID(), domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE connections SET state='healthy',server_version='1.0' WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,connection_id,native_id,name) VALUES($1,$2,$3,'maintained','shop')`, f.org, repo, f.connection.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','compatible','Approved','https://model.example','{"model":"approved"}','healthy')`, f.org, model); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runner_pools(org_id,id,name,state,builtin) VALUES($1,$2,'Built in','active',true)`, f.org, pool); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO runners(org_id,id,pool_id,name,credential_hash,credential_expires_at) VALUES($1,$2,$3,'Runner','fixture',now()+interval '1 hour')`, f.org, domain.NewID(), pool)
		return err
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
	provider := &unprotectedMergeSetupProvider{db: f.db, connections: f.connections, rules: forge.Rules{State: domain.Supported, Hash: strings.Repeat("b", 64), AllowedMergeMethods: []string{"squash"}, Unprotected: true}}
	merges := mergecontrol.New(f.db, f.identity, f.connections, policies, provider)
	service := autopilot.New(f.db, f.identity, repairs, merges, budgets, policies)
	command := autopilot.MaintenanceCommand{Mode: "merge", MicroUSD: 10_000_000, Continuous: true}
	plan, err := service.PreviewMaintenance(ctx, f.owner, f.org, repo, command)
	if err != nil || !plan.Ready || plan.MergeSetup == nil || !plan.MergeSetup.Ready || slices.Contains(plan.Blockers, "Operator: qualify this repository's protected merge path") {
		t.Fatalf("fresh unprotected merge preview: %+v %v", plan, err)
	}
	command.Proof = plan.Proof
	configured, err := service.ConfigureMaintenance(ctx, f.owner, f.org, repo, command, "unprotected-merge-setup")
	if err != nil || !configured.Ready || configured.MergeSetup == nil || !configured.MergeSetup.Ready || !slices.Contains(configured.Actions, policy.Merge) || slices.Contains(configured.Actions, policy.Deploy) {
		t.Fatalf("fresh unprotected merge setup: %+v %v", configured, err)
	}
	status, err := service.MaintenanceStatus(ctx, f.owner, f.org, repo)
	if err != nil || status.Mode != "merge" || !status.Continuous || status.MicroUSD != command.MicroUSD {
		t.Fatalf("saved merge setup scope: %+v %v", status, err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		var continuous bool
		var cap int64
		if err := tx.QueryRow(ctx, `SELECT q.continuous,(b.caps->>'micro_usd')::bigint FROM autopilot_requests q JOIN budget_limits b ON b.org_id=q.org_id AND b.scope_kind='repository' AND b.scope_id=q.repository_id WHERE q.org_id=$1 AND q.repository_id=$2`, f.org, repo).Scan(&continuous, &cap); err != nil {
			return err
		}
		if !continuous || cap != command.MicroUSD {
			t.Fatalf("saved scope: continuous=%v cap=%d", continuous, cap)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	provider.rules.Unprotected = false
	provider.rules.ActorCanBypass = false
	provider.rules.RequiredApprovals = 1
	plan, err = service.PreviewMaintenance(ctx, f.owner, f.org, repo, autopilot.MaintenanceCommand{Mode: "merge", MicroUSD: command.MicroUSD, Continuous: true})
	if err != nil || plan.Ready || plan.MergeSetup == nil || plan.MergeSetup.Ready || !strings.Contains(plan.MergeSetup.Reason, "requires a person's approval") {
		t.Fatalf("required review marked ready: %+v %v", plan, err)
	}
	command.Proof = plan.Proof
	if _, err := service.ConfigureMaintenance(ctx, f.owner, f.org, repo, command, "required-review"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("required review configuration accepted: %v", err)
	}
	provider.rules.RequiredApprovals = 0
	provider.rules.Unprotected = false
	provider.rules.ActorCanBypass = true
	bypassCommand := autopilot.MaintenanceCommand{Mode: "merge", MicroUSD: command.MicroUSD, Continuous: true}
	bypassPlan, err := service.PreviewMaintenance(ctx, f.owner, f.org, repo, bypassCommand)
	if err != nil || !bypassPlan.Ready || bypassPlan.MergeSetup == nil || !bypassPlan.MergeSetup.Ready {
		t.Fatalf("fresh actor-bypass merge preview: %+v %v", bypassPlan, err)
	}
	bypassCommand.Proof = bypassPlan.Proof
	bypassConfigured, err := service.ConfigureMaintenance(ctx, f.owner, f.org, repo, bypassCommand, "actor-bypass-merge-setup")
	if err != nil || !bypassConfigured.Ready || bypassConfigured.MergeSetup == nil || !bypassConfigured.MergeSetup.Ready {
		t.Fatalf("fresh actor-bypass merge setup: %+v %v", bypassConfigured, err)
	}
	deliveryPlan, err := service.PreviewMaintenance(ctx, f.owner, f.org, repo, autopilot.MaintenanceCommand{Mode: "deliver", MicroUSD: command.MicroUSD, Continuous: true})
	if err != nil || deliveryPlan.Ready || !slices.Contains(deliveryPlan.Blockers, "Operator: connect and qualify a CI release reporter and permitted deployment path") {
		t.Fatalf("delivery setup bypassed: %+v %v", deliveryPlan, err)
	}
}

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
		blocked := "Resume or raise " + scope.Kind + " spending limit"
		plan, err := service.PreviewMaintenance(ctx, f.owner, f.org, repo, command)
		if err != nil || plan.Ready || !slices.Contains(plan.Blockers, blocked) {
			t.Fatalf("paused setup budget marked ready: %+v %v", plan, err)
		}
		defaults, err := repairs.Defaults(ctx, f.owner, f.org, repo)
		if err != nil || !slices.Contains(defaults.Blockers, blocked) {
			t.Fatalf("paused repair defaults marked ready: %+v %v", defaults, err)
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
