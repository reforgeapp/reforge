package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/mergecontrol"
	"reforge/internal/policy"
	"reforge/internal/providers"
)

func TestMergeObserverRestartTenantAndVersionGuards(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	provider := providers.New(f.db, f.connections, nil, nil, false)
	gateID, operationID := domain.NewID(), domain.NewID()
	gate := mergecontrol.Gate{
		ID: gateID, RepositoryID: f.repo, ConnectionID: f.connection.ID,
		ConnectionVersion: f.connection.Version, ConfigurationVersion: 0,
		ExpiresAt: time.Now().Add(time.Hour),
		Snapshot:  forge.MergeEvidence{Change: forge.Change{ID: "7", Repository: forge.RepoRef{NativeID: "1", FullName: "acme/repository-0000"}, TargetBranch: "main"}},
	}
	rawGate, _ := json.Marshal(gate)
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO merge_gates(org_id,id,repository_id,change_id,configuration_version,document) VALUES($1,$2,$3,'7',0,$4)`, f.org, gateID, f.repo, rawGate); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO merge_operations(org_id,id,repository_id,gate_id,requested_gate_id,change_id,target_branch,idempotency_key,requested_by,state,updated_at) VALUES($1,$2,$3,$4,$4,'7','main',$5,$6,'requested',now())`, f.org, operationID, f.repo, gateID, domain.NewID(), f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	restarted := mergecontrol.New(f.db, f.identity, f.connections, policies, provider)
	op, err := restarted.Observe(ctx, f.org, operationID)
	if err != nil || op.State != "requested" || op.Version != 1 {
		t.Fatalf("restart observer changed recent operation: %+v %v", op, err)
	}
	if _, err = restarted.Observe(ctx, domain.NewID(), operationID); err == nil {
		t.Fatal("cross-tenant observer read succeeded")
	}
	if _, err = restarted.Reconcile(ctx, f.owner, f.org, operationID, 0, "stale-version"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("invalid optimistic version accepted: %v", err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE merge_operations SET version=version+1 WHERE org_id=$1 AND id=$2`, f.org, operationID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Reconcile(ctx, f.owner, f.org, operationID, 1, "stale-version"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale optimistic version accepted: %v", err)
	}

	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE merge_operations SET state='queued',native_queue_id='queue-fixture',updated_at=now()-interval '3 minutes' WHERE org_id=$1 AND id=$2`, f.org, operationID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	observed, observeErr := restarted.Observe(ctx, f.org, operationID)
	if observeErr == nil {
		t.Fatalf("unsupported fixture transport unexpectedly completed cancellation: %+v", observed)
	}
	var cancelRequested bool
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT cancel_requested FROM merge_operations WHERE org_id=$1 AND id=$2`, f.org, operationID).Scan(&cancelRequested)
	}); err != nil {
		t.Fatal(err)
	}
	if !cancelRequested {
		t.Fatal("authority revocation did not persist cancellation intent")
	}
}
