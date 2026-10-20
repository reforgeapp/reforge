package integration

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/mergecontrol"
	"github.com/reforgeapp/reforge/pkg/policy"
)

func TestMergeConfigurationIsolationAndDurableCancellation(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	service := mergecontrol.New(f.db, f.identity, f.connections, policies, nil)
	cfg, err := service.Config(ctx, f.owner, f.org, f.repo)
	if err != nil || cfg.Enabled || cfg.Version != 0 {
		t.Fatalf("initial merge authority: %+v %v", cfg, err)
	}
	cfg, err = service.PutConfig(ctx, f.owner, f.org, f.repo, cfg, 0, "merge-contract")
	if err != nil || cfg.Version != 1 {
		t.Fatalf("configuration: %+v %v", cfg, err)
	}
	if _, err = service.PutConfig(ctx, f.owner, f.org, f.repo, cfg, 0, "merge-contract"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale config accepted: %v", err)
	}
	cfg.Enabled = true
	if _, err = service.PutConfig(ctx, f.owner, f.org, f.repo, cfg, 1, "merge-contract"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("unqualified automation enabled: %v", err)
	}
	id, gateID := domain.NewID(), domain.NewID()
	gate := mergecontrol.Gate{ID: gateID, RepositoryID: f.repo, ConnectionID: f.connection.ID, ConnectionVersion: f.connection.Version, ExpiresAt: time.Now().Add(-time.Minute), Snapshot: forge.MergeEvidence{Change: forge.Change{ID: "7", TargetBranch: "main"}}, Decision: policy.Result{Decision: domain.Decision{Outcome: "allow"}}}
	raw, _ := json.Marshal(gate)
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO merge_gates(org_id,id,repository_id,change_id,configuration_version,document) VALUES($1,$2,$3,'7',1,$4)`, f.org, gateID, f.repo, raw); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO merge_operations(org_id,id,repository_id,gate_id,requested_gate_id,change_id,target_branch,idempotency_key,requested_by,state) VALUES($1,$2,$3,$4,$4,'7','main',$5,$6,'requested')`, f.org, id, f.repo, gateID, domain.NewID(), f.owner.User.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Request(ctx, f.owner, f.org, gateID, domain.NewID(), "merge-contract"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("expired gate accepted: %v", err)
	}
	if err = f.db.Tenant(ctx, domain.NewID(), "", func(tx pgx.Tx) error {
		var visible int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM merge_operations WHERE id=$1`, id).Scan(&visible); err != nil {
			return err
		}
		if visible != 0 {
			t.Fatal("cross-tenant merge operation visible")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Cancel(ctx, f.owner, f.org, id, 1, "cancel-contract")
			outcomes <- err
		}()
	}
	wg.Wait()
	close(outcomes)
	winners := 0
	for err := range outcomes {
		if err == nil {
			winners++
		} else if !errors.Is(err, auth.ErrConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("cancellation winners=%d", winners)
	}
	restarted := mergecontrol.New(f.db, f.identity, f.connections, policies, nil)
	op, err := restarted.Get(ctx, f.owner, f.org, id)
	if err != nil || op.State != "cancelled" || !op.CancelRequested || op.Version != 2 {
		t.Fatalf("lost durable cancellation: %+v %v", op, err)
	}
	page, err := restarted.List(ctx, f.owner, f.org, f.repo, "7", "", 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != id || page.Items[0].RequestedGateID != gateID || !page.Complete {
		t.Fatalf("operation history lost request binding: %+v %v", page, err)
	}
	page, err = restarted.List(ctx, f.owner, f.org, f.repo, "other-change", "", 1)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("change-scoped history leaked: %+v %v", page, err)
	}
	page, err = restarted.List(ctx, f.owner, f.org, f.repo, "7", id, 1)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("history cursor replayed operation: %+v %v", page, err)
	}
	if _, err = restarted.Get(ctx, f.owner, domain.NewID(), id); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("cross-tenant service read: %v", err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE merge_operations SET state='dispatching',cancel_requested=false,version=3 WHERE org_id=$1 AND id=$2`, f.org, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	op, err = restarted.Cancel(ctx, f.owner, f.org, id, 3, "cancel-dispatched")
	if err != nil || op.State != "reconciling" || !op.CancelRequested {
		t.Fatalf("dispatched cancellation falsely confirmed: %+v %v", op, err)
	}
}
