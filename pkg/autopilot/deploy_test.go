package autopilot

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/store"
)

func TestDeploymentIncidentRequiresVerifiedRecovery(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	if u, err := url.Parse(raw); err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable PostgreSQL reforge_test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	org, user, repo, otherRepo := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	connection := domain.NewID()
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'deployment-incident-test',$2,'Owner',$3)`, user, user, user+"@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Deployment incidents')`, org); err != nil {
			return err
		}
		for _, id := range []string{repo, otherRepo} {
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name,default_branch) VALUES($1,$2::uuid,$2::text,$2::text,'main')`, org, id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	add := func(repository, environment, state string, created time.Time) string {
		t.Helper()
		id, gateID := domain.NewID(), domain.NewID()
		gate, err := json.Marshal(deployment.Gate{ConnectionID: connection, ConnectionVersion: 1, Request: deployment.PreviewRequest{SourceSHA: strings.Repeat("a", 40)}})
		if err != nil {
			t.Fatal(err)
		}
		if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO deployment_configurations(org_id,environment,repository_id,version,document) VALUES($1,$2,$3,1,'{}') ON CONFLICT DO NOTHING`, org, environment, repository); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO deployment_gates(org_id,id,repository_id,environment,document) VALUES($1,$2,$3,$4,$5)`, org, gateID, repository, environment, gate); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO deployments(org_id,id,repository_id,environment,gate_id,requested_by,idempotency_key,state,created_at,finished_at) VALUES($1,$2::uuid,$3,$4,$5,$6,$2::text,$7,$8::timestamptz,$8::timestamptz+interval '1 hour')`, org, id, repository, environment, gateID, user, state, created)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	now := time.Now()
	failed := add(repo, "production", "failed", now.Add(-9*24*time.Hour))
	service := &Service{db: db}
	session := auth.Session{User: auth.User{ID: user}}
	var recovery string
	observe := func(wantState, wantDeployment string) {
		t.Helper()
		if err := service.observeDeployments(ctx, session, org); err != nil {
			t.Fatal(err)
		}
		var state, deploymentID, reason string
		if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT state,evidence->>'deployment',reason FROM maintenance_findings WHERE org_id=$1 AND repository_id=$2 AND source='deployment' AND source_id='deploy:production'`, org, repo).Scan(&state, &deploymentID, &reason)
		}); err != nil {
			t.Fatal(err)
		}
		if state != wantState || deploymentID != wantDeployment {
			t.Fatalf("incident state=%s deployment=%s; want %s/%s", state, deploymentID, wantState, wantDeployment)
		}
		if state == "resolved" && !strings.Contains(reason, recovery) {
			t.Fatalf("recovery missing from resolution: %s", reason)
		}
	}
	observe("open", failed)
	add(repo, "staging", "healthy", now.Add(-24*time.Hour))
	add(otherRepo, "production", "healthy", now.Add(-24*time.Hour))
	add(repo, "production", "healthy", now.Add(-10*24*time.Hour))
	add(repo, "production", "completed_unverified", now.Add(-24*time.Hour))
	observe("open", failed)
	recovery = add(repo, "production", "recovered", now.Add(-12*time.Hour))
	observe("resolved", failed)
	newFailure := add(repo, "production", "recovery_failed", now.Add(-time.Hour))
	observe("open", newFailure)
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE deployment_gates g SET document='[]' FROM deployments d WHERE d.org_id=g.org_id AND d.gate_id=g.id AND d.org_id=$1 AND d.id=$2`, org, newFailure)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	observe("open", newFailure)
}
