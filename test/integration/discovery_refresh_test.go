package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
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
}
