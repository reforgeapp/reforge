package runner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/agent"
	"github.com/reforgeapp/reforge/pkg/agentcontrol"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/autopilot"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func TestNativeAgentConnectionIsBoundToCreatingOwner(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	builtin, err := f.service.EnrollBuiltin(ctx, f.org, "built-in", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Claim(ctx, builtin.Token); !errors.Is(err, workflow.ErrNoWork) {
		t.Fatalf("heartbeat claim: %v", err)
	}
	connectionService := connections.New(f.db, f.identity, nil, true)
	control := agentcontrol.New(f.db, f.identity, connectionService, f.service, nil, nil, true)
	options, err := control.Options(ctx, f.session, f.org)
	if err != nil || !options.Available {
		t.Fatalf("native agent options %+v: %v", options, err)
	}
	created, err := control.Create(ctx, f.session, f.org, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	var metadata connections.Connection
	if err = f.db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		var err error
		metadata, err = connectionService.MetadataTx(ctx, tx, f.org, created.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if metadata.Settings.RuntimeUserID != f.session.User.ID || metadata.Settings.RuntimeRunnerID != builtin.Runner.ID || !metadata.NativeAgent() {
		t.Fatalf("native connection binding: %+v", metadata.Settings)
	}
	modelName := "codex-subscription-model"
	accountID := domain.NewID()
	forgeID := domain.NewID()
	if err = f.db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'forge','github','Fixture forge','https://api.github.com','{}','healthy')`, f.org, forgeID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE repositories SET connection_id=$3,default_branch='main' WHERE org_id=$1 AND id=$2`, f.org, f.repos[0], forgeID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE connections SET settings=settings||jsonb_build_object('model',$3::text,'namespace',$4::text),state='healthy',verified_at=clock_timestamp(),version=version+1 WHERE org_id=$1 AND id=$2`, f.org, created.ID, modelName, accountID); err != nil {
			return err
		}
		return budget.ConfigureNativeTx(ctx, tx, f.org, created.ID, modelName, domain.NewID())
	}); err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	budgets := budget.New(f.db, f.identity, nil, nil)
	repairs := repair.New(f.db, f.identity, nil, nil, f.service, policies, budgets, connectionService, nil, nil, map[string]string{"go": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	maintenance := autopilot.New(f.db, f.identity, repairs, nil, budgets, policies)
	preview, err := maintenance.PreviewMaintenance(ctx, f.session, f.org, f.repos[0], autopilot.MaintenanceCommand{ConnectionID: created.ID, Mode: "propose", MicroUSD: 10_000_000})
	if err != nil {
		t.Fatal(err)
	}
	zero, err := maintenance.PreviewMaintenance(ctx, f.session, f.org, f.repos[0], autopilot.MaintenanceCommand{ConnectionID: created.ID, Mode: "propose", MicroUSD: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Subscription || !zero.Subscription || preview.MicroUSD != 0 || zero.MicroUSD != 0 || preview.Proof != zero.Proof {
		t.Fatalf("native subscription preview differs by submitted cap: preview=%+v zero=%+v", preview, zero)
	}
	configured, err := maintenance.ConfigureMaintenance(ctx, f.session, f.org, f.repos[0], autopilot.MaintenanceCommand{ConnectionID: created.ID, Mode: "propose", MicroUSD: 0, Proof: preview.Proof}, domain.NewID())
	if err != nil {
		t.Fatalf("configure using equivalent zero-dollar proof: %v", err)
	}
	if configured.MicroUSD != 0 {
		t.Fatalf("configured native subscription cap=%d", configured.MicroUSD)
	}
	var repositoryCap int64
	if err = f.db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT (caps->>'micro_usd')::bigint FROM budget_limits WHERE org_id=$1 AND scope_kind='repository' AND scope_id=$2`, f.org, f.repos[0]).Scan(&repositoryCap)
	}); err != nil {
		t.Fatal(err)
	}
	if repositoryCap != 0 {
		t.Fatalf("native repository spending cap=%d", repositoryCap)
	}

	other := auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}, CSRFToken: "other-csrf"}
	if err = f.db.Identity(ctx, other.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'runner-test',$2,'Other owner',$3)`, other.User.ID, other.User.ID, other.User.ID+"@example.test"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,clock_timestamp()+interval '1 hour')`, other.ID, other.User.ID, domain.NewID(), other.CSRFToken)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'admin',true)`, f.org, other.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = control.Action(ctx, other, f.org, created.ID, agent.NativeAction{Kind: "status"}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("other org admin accessed native account: %v", err)
	}
}
