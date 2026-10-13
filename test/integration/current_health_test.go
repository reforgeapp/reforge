package integration

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/insights"
)

func TestCompletedRolloutAcceptsFreshHealthAndKeepsHistory(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	public, private, _ := ed25519.GenerateKey(nil)
	id, gateID := domain.NewID(), domain.NewID()
	head := strings.Repeat("a", 40)
	artifact := "sha256:" + strings.Repeat("b", 64)
	created := time.Now().Add(-2 * time.Hour)
	configuration := deployment.Configuration{Environment: "production", RepositoryID: f.repo, Version: 1, HealthPublicKey: base64.StdEncoding.EncodeToString(public), HealthChecks: []string{"smoke"}, MaxEvidenceAgeSeconds: 300}
	gate := deployment.Gate{ID: gateID, RepositoryID: f.repo, Environment: "production", ConnectionID: f.connection.ID, ConnectionVersion: f.connection.Version, ConfigurationVersion: 1, Request: deployment.PreviewRequest{SourceSHA: head, ArtifactDigest: artifact}}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		cfg, _ := json.Marshal(configuration)
		raw, _ := json.Marshal(gate)
		native, _ := json.Marshal(forge.DeploymentStatus{ID: "release-1", RunAttempt: 1, SourceSHA: head, ArtifactDigest: artifact})
		if _, err := tx.Exec(ctx, `INSERT INTO deployment_configurations(org_id,environment,repository_id,version,document) VALUES($1,'production',$2,1,$3)`, f.org, f.repo, cfg); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO deployment_gates(org_id,id,repository_id,environment,document) VALUES($1,$2,$3,'production',$4)`, f.org, gateID, f.repo, raw); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO deployments(org_id,id,repository_id,environment,gate_id,requested_by,idempotency_key,state,native_result,created_at,finished_at,last_health_at) VALUES($1,$2::uuid,$3,'production',$4,$5,$2::text,'healthy',$6,$7::timestamptz,$7::timestamptz+interval '5 minutes',$7::timestamptz+interval '5 minutes')`, f.org, id, f.repo, gateID, f.owner.User.ID, native, created)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := deployment.New(f.db, f.identity, f.connections, nil, nil)
	read := insights.New(f.identity)
	report := deployment.HealthReport{OrgID: f.org, DeploymentID: id, Environment: "production", ConfigurationVersion: 1, SourceSHA: head, ArtifactDigest: artifact, RunID: "release-1", RunAttempt: 1, Revision: head, Healthy: true, Checks: map[string]bool{"smoke": true}}
	send := func(healthy bool) {
		t.Helper()
		report.Healthy = healthy
		report.Checks["smoke"] = healthy
		report.Nonce = domain.NewID()
		report.ObservedAt = time.Now()
		raw, _ := json.Marshal(report)
		if err := service.ReceiveHealth(ctx, f.org, id, report, base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw))); err != nil {
			t.Fatal(err)
		}
	}
	state := func(want string) {
		t.Helper()
		page, err := read.Health(ctx, f.owner, f.org, insights.Filter{Limit: 50})
		if err != nil || len(page.Items) != 1 || page.Items[0].State != want {
			t.Fatalf("health: %+v %v", page, err)
		}
		var historical string
		var version int64
		if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT state,version FROM deployments WHERE org_id=$1 AND id=$2`, f.org, id).Scan(&historical, &version)
		}); err != nil {
			t.Fatal(err)
		}
		if historical != "healthy" || version != 1 {
			t.Fatalf("historical rollout changed: %s %d", historical, version)
		}
	}
	state("unknown")
	send(true)
	state("healthy")
	send(false)
	state("unhealthy")
	var incident string
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state FROM maintenance_findings WHERE org_id=$1 AND repository_id=$2 AND source='environment_health'`, f.org, f.repo).Scan(&incident)
	}); err != nil || incident != "open" {
		t.Fatalf("late failure lost: %s %v", incident, err)
	}
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE environment_health SET expires_at=now()-interval '1 second' WHERE org_id=$1`, f.org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state("unknown")
	send(true)
	state("healthy")
	wrong := report
	wrong.Nonce = domain.NewID()
	wrong.ObservedAt = time.Now().Add(-time.Hour)
	raw, _ := json.Marshal(wrong)
	if err := service.ReceiveHealth(ctx, f.org, id, wrong, base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw))); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale signed report accepted: %v", err)
	}
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		newID := domain.NewID()
		_, err := tx.Exec(ctx, `INSERT INTO deployments(org_id,id,repository_id,environment,gate_id,requested_by,idempotency_key,state,native_result,finished_at) SELECT org_id,$3::uuid,repository_id,environment,gate_id,requested_by,$3::text,'failed',jsonb_set(native_result,'{id}','"release-2"'),now() FROM deployments WHERE org_id=$1 AND id=$2`, f.org, id, newID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state("unknown")
	report.Nonce = domain.NewID()
	report.ObservedAt = time.Now()
	raw, _ = json.Marshal(report)
	if err := service.ReceiveHealth(ctx, f.org, id, report, base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw))); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("older release reported healthy after newer rollout: %v", err)
	}
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE deployment_configurations SET version=version+1 WHERE org_id=$1`, f.org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state("unknown")
}

func TestCurrentHealthScopesBeforePagination(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	hidden, visible := domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,'hidden','hidden'),($1,$3,'visible','visible')`, f.org, hidden, visible); err != nil {
			return err
		}
		for i := 0; i < 110; i++ {
			if _, err := tx.Exec(ctx, `INSERT INTO deployment_configurations(org_id,repository_id,environment,version,document) VALUES($1,$2,$3,1,'{}')`, f.org, hidden, fmt.Sprintf("hidden-%d", i)); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO deployment_configurations(org_id,repository_id,environment,version,document) VALUES($1,$2,'production',1,'{}'),($1,$2,'staging',1,'{}')`, f.org, visible); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO member_repositories(org_id,user_id,repository_id) VALUES($1,$2,$3)`, f.org, f.owner.User.ID, visible)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := insights.New(f.identity)
	first, err := service.Health(ctx, f.owner, f.org, insights.Filter{Limit: 1})
	if err != nil || len(first.Items) != 1 || first.Complete || first.NextCursor == "" || first.Items[0].RepositoryID != visible || first.Items[0].State != "unknown" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	second, err := service.Health(ctx, f.owner, f.org, insights.Filter{Limit: 1, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || !second.Complete || second.Items[0].RepositoryID != visible || second.Items[0].Environment == first.Items[0].Environment {
		t.Fatalf("second page: %+v %v", second, err)
	}
}
