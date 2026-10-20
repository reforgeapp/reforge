package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/inventory"
)

func TestStartScanQueuesRefreshAndDefersStaleExecution(t *testing.T) {
	f := newDiscoveryFixture(t)
	service, _ := f.scannerFixture(t, false)
	ctx := context.Background()
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE inventory_repository_state SET state='fresh',changes_observed_at=NULL,refresh_due=clock_timestamp() WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartScan(ctx, f.owner, f.org, f.repo, "refresh-before-scan"); err != nil {
		t.Fatal(err)
	}
	var refreshID, refreshState, scanState string
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id::text,state FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state IN ('queued','running') ORDER BY created_at DESC LIMIT 1`, f.org, f.repo).Scan(&refreshID, &refreshState); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT state FROM maintenance_scans WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo).Scan(&scanState)
	}); err != nil {
		t.Fatal(err)
	}
	if refreshID == "" || refreshState != "queued" || scanState != "queued" {
		t.Fatalf("queued refresh/scan = %s/%s/%s", refreshID, refreshState, scanState)
	}
	if worked, err := service.RunOrganisationOnce(ctx, f.org); err != nil {
		t.Fatalf("stale scan claim: %v", err)
	} else if worked {
		t.Fatal("stale scan executed")
	}
	status, err := service.ScanStatus(ctx, f.owner, f.org, f.repo)
	if err != nil || status.State != "queued" {
		t.Fatalf("stale scan executed: %v %+v", err, status)
	}
	var refreshDirty bool
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT coalesce((input->>'dirty')::boolean,false) FROM inventory_jobs WHERE org_id=$1 AND id=$2`, f.org, refreshID).Scan(&refreshDirty)
	}); err != nil || refreshDirty {
		t.Fatalf("existing refresh marked dirty: %v %v", err, refreshDirty)
	}
	f.drain(t, refreshID)
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE maintenance_scans SET available_at=clock_timestamp() WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if worked, err := service.RunOrganisationOnce(ctx, f.org); err != nil || !worked {
		t.Fatalf("refreshed scan: %v %v", err, worked)
	}
	status, err = service.ScanStatus(ctx, f.owner, f.org, f.repo)
	if err != nil || status.State != "complete" {
		t.Fatalf("refreshed scan did not complete: %v %+v", err, status)
	}
	var cadenceOK bool
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT available_at>clock_timestamp() AND available_at<=clock_timestamp()+interval '5 minutes' FROM maintenance_scans WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo).Scan(&cadenceOK)
	}); err != nil || !cadenceOK {
		t.Fatalf("successful scan cadence: %v %v", err, cadenceOK)
	}
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE inventory_repository_state SET changes_observed_at=NULL WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE maintenance_scans SET available_at=clock_timestamp() WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if worked, err := service.RunOrganisationOnce(ctx, f.org); err != nil || worked {
		t.Fatalf("stale periodic scan should queue refresh and defer: %v %v", err, worked)
	}
	var staleRefreshState string
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state IN ('queued','running') ORDER BY created_at DESC LIMIT 1`, f.org, f.repo).Scan(&staleRefreshState)
	}); err != nil || staleRefreshState != "queued" {
		t.Fatalf("stale periodic scan did not request refresh: %v %q", err, staleRefreshState)
	}
}

func TestPeriodicRefreshKeepsRecentSnapshotUsable(t *testing.T) {
	f := newDiscoveryFixture(t)
	_, _ = f.scannerFixture(t, false)
	ctx := context.Background()
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE inventory_repository_state SET refresh_due=clock_timestamp()-interval '1 minute' WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.inventoryFixture.service.Maintain(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	var refreshState string
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state IN ('queued','running') ORDER BY created_at DESC LIMIT 1`, f.org, f.repo).Scan(&refreshState)
	}); err != nil || refreshState != "queued" {
		t.Fatalf("periodic refresh was not queued: %v %q", err, refreshState)
	}
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error { return inventory.RequireFreshTx(ctx, tx, f.org, f.repo) }); err != nil {
		t.Fatalf("periodic refresh invalidated recent snapshot: %v", err)
	}
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE inventory_repository_state SET changes_observed_at=clock_timestamp()-interval '16 minutes' WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var freshnessErr error
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		freshnessErr = inventory.RequireFreshTx(ctx, tx, f.org, f.repo)
		return nil
	}); err != nil || !errors.Is(freshnessErr, inventory.ErrStale) {
		t.Fatalf("stale observation accepted: %v %v", err, freshnessErr)
	}
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE inventory_repository_state SET changes_observed_at=clock_timestamp() WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo); err != nil {
			return err
		}
		return inventory.RequestRefreshTx(ctx, tx, f.org, f.repo)
	}); err != nil {
		t.Fatal(err)
	}
	freshnessErr = nil
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		freshnessErr = inventory.RequireFreshTx(ctx, tx, f.org, f.repo)
		return nil
	}); err != nil || !errors.Is(freshnessErr, inventory.ErrStale) {
		t.Fatalf("manual refresh did not invalidate snapshot: %v %v", err, freshnessErr)
	}
}

func TestDirtyActiveRefreshInvalidatesAfterCompletion(t *testing.T) {
	f := newDiscoveryFixture(t)
	_, _ = f.scannerFixture(t, false)
	service := f.inventoryFixture.service
	ctx := context.Background()
	var refreshID string
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if err := inventory.RequestRefreshTx(ctx, tx, f.org, f.repo); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id::text FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state='queued' ORDER BY created_at DESC LIMIT 1`, f.org, f.repo).Scan(&refreshID)
	}); err != nil {
		t.Fatal(err)
	}
	lease, err := service.Claim(ctx, f.org, "refresh-repository")
	if err != nil || lease == nil || lease.ID != refreshID {
		t.Fatalf("claim repository refresh: %v %+v", err, lease)
	}
	if err = service.Step(ctx, *lease); err != nil {
		t.Fatalf("refresh repository step: %v", err)
	}
	lease, err = service.Claim(ctx, f.org, "refresh-changes")
	if err != nil || lease == nil || lease.ID != refreshID || lease.Phase != "changes" {
		t.Fatalf("claim changes refresh: %v %+v", err, lease)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return inventory.RequestRefreshTx(ctx, tx, f.org, f.repo)
	}); err != nil {
		t.Fatal(err)
	}
	if err = service.Step(ctx, *lease); err != nil {
		t.Fatalf("complete dirty refresh: %v", err)
	}
	var dirty bool
	var followupID string
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT coalesce((input->>'dirty')::boolean,false) FROM inventory_jobs WHERE org_id=$1 AND id=$2 AND state='complete'`, f.org, refreshID).Scan(&dirty); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id::text FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state='queued' AND id<>$3 ORDER BY created_at DESC LIMIT 1`, f.org, f.repo, refreshID).Scan(&followupID)
	}); err != nil || !dirty || followupID == "" {
		t.Fatalf("dirty refresh follow-up: %v dirty=%v followup=%q", err, dirty, followupID)
	}
	var freshnessErr error
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		freshnessErr = inventory.RequireFreshTx(ctx, tx, f.org, f.repo)
		return nil
	}); err != nil || !errors.Is(freshnessErr, inventory.ErrStale) {
		t.Fatalf("dirty refresh completion restored stale evidence: %v %v", err, freshnessErr)
	}
}
