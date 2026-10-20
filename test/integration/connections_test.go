package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/secrets"
)

func TestConnectionPersistenceRotationAndRevocation(t *testing.T) {
	db := authDB(t)
	ctx := context.Background()
	identity, server := identityServer(t, db, authConfig())
	cookie, session := identityLogin(t, identity, server)
	org := session.Organisations[0].ID
	vault, err := secrets.New("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))})
	if err != nil {
		t.Fatal(err)
	}
	svc := connections.New(db, identity, vault, true)
	server.RegisterConnections(svc)
	input := connections.CreateRequest{Kind: "forge", Provider: "gitea", Name: "Contract", Endpoint: "https://forge.example.test/api/v1", Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge"}, Secret: "never-store-this-plaintext"}
	c, err := svc.Create(ctx, session, org, input, "test")
	if err != nil {
		t.Fatal(err)
	}
	var ciphertext []byte
	err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT envelope::text FROM connection_secrets WHERE org_id=$1 AND connection_id=$2`, org, c.ID).Scan(&ciphertext)
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ciphertext), input.Secret) {
		t.Fatal("plaintext credential persisted")
	}
	restarted := connections.New(db, identity, vault, true)
	err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		resolved, err := restarted.ResolveTx(ctx, tx, org, c.ID, "")
		if err != nil {
			return err
		}
		if resolved.Secret != input.Secret {
			t.Fatal("credential lost across service restart")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	response := identityRequest(server, "GET", "/api/v1/orgs/"+org+"/connections/"+c.ID, "", cookie, nil)
	if response.Code != 200 || strings.Contains(response.Body.String(), input.Secret) || strings.Contains(response.Body.String(), "secret_id") {
		t.Fatalf("public connection response: %d", response.Code)
	}
	rotatedKeys, _ := secrets.New("next", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))), "next": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("n", 32)))})
	svc = connections.New(db, identity, rotatedKeys, true)
	c, err = svc.Rewrap(ctx, session, org, c.ID, c.Version, "test")
	if err != nil {
		t.Fatal(err)
	}
	newOnly, _ := secrets.New("next", map[string]string{"next": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("n", 32)))})
	svc = connections.New(db, identity, newOnly, true)
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		r, err := svc.ResolveTx(ctx, tx, org, c.ID, "")
		if err != nil {
			return err
		}
		if r.Secret != input.Secret {
			t.Fatal("wrapping rotation lost credential")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	other := domain.NewID()
	if _, err = svc.Get(ctx, session, other, c.ID); err != auth.ErrForbidden {
		t.Fatalf("cross tenant lookup: %v", err)
	}
	tested, err := svc.Test(ctx, session, org, c.ID, c.Version, "test")
	if err != nil || tested.State != "disabled" {
		t.Fatalf("unregistered provider falsely passed: %v %s", err, tested.State)
	}
	rotated, err := svc.Rotate(ctx, session, org, c.ID, tested.Version, "replacement-credential", "test")
	if err != nil {
		t.Fatal(err)
	}
	if rotated.VerifiedAt != nil || rotated.CredentialVersion != 2 || rotated.State != "unverified" {
		t.Fatal("rotation retained old capability evidence")
	}
	if _, err = svc.Rotate(ctx, session, org, c.ID, tested.Version, "stale", "test"); err != auth.ErrConflict {
		t.Fatal("stale rotation accepted")
	}
	svc.Register("forge", "gitea", func(_ context.Context, r connections.Resolved) (connections.ProbeResult, error) {
		if r.Secret != "replacement-credential" {
			t.Error("rotated credential not used")
		}
		return connections.ProbeResult{}, errors.New("remote echoed " + r.Secret)
	})
	tested, err = svc.Test(ctx, session, org, c.ID, rotated.Version, "test")
	if err != nil || tested.State != "degraded" {
		t.Fatal("failed probe mishandled")
	}
	encoded, _ := json.Marshal(tested)
	if strings.Contains(string(encoded), "replacement-credential") {
		t.Fatal("provider error leaked secret")
	}
	edited, err := svc.Update(ctx, session, org, c.ID, tested.Version, "Renamed", "acme/app", "test")
	if err != nil || edited.Name != "Renamed" || edited.Settings.Namespace != "acme/app" || edited.CredentialVersion != tested.CredentialVersion {
		t.Fatalf("update: %+v %v", edited, err)
	}
	tested = edited
	revoked, err := svc.Revoke(ctx, session, org, c.ID, tested.Version, "test")
	if err != nil || revoked.State != "revoked" {
		t.Fatal("revoke failed", err)
	}
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error { _, err := svc.ResolveTx(ctx, tx, org, c.ID, ""); return err })
	if err != connections.ErrRevoked {
		t.Fatalf("revoked credential resolved: %v", err)
	}
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM connection_secrets WHERE org_id=$1 AND connection_id=$2`, org, c.ID).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("revoked encrypted credential retained")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Delete(ctx, session, org, c.ID, revoked.Version, "test"); err != nil {
		t.Fatal("delete unused connection", err)
	}
	if _, err = svc.Get(ctx, session, org, c.ID); err == nil {
		t.Fatal("deleted connection still readable")
	}
	input.PrivateRoute = &connections.Route{RunnerID: domain.NewID(), Host: "different.test", CIDRs: []string{"bad"}, RevokedAt: new(time.Time)}
	if _, err = svc.Create(ctx, session, org, input, "test"); err != auth.ErrInvalid {
		t.Fatalf("route timestamp bypass: %v", err)
	}
	input.PrivateRoute = &connections.Route{RunnerID: domain.NewID(), Host: "forge.example.test", CIDRs: []string{"10.0.0.0/8"}}
	if _, err = svc.Create(ctx, session, org, input, "test"); err != connections.ErrRunnerRequired {
		t.Fatalf("unenrolled route accepted: %v", err)
	}
}

func TestConnectionQueuedProbeCannotUseRevokedAuthority(t *testing.T) {
	db := authDB(t)
	ctx := context.Background()
	identity, server := identityServer(t, db, authConfig())
	_, session := identityLogin(t, identity, server)
	org := session.Organisations[0].ID
	vault, _ := secrets.New("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))})
	svc := connections.New(db, identity, vault, true)
	c, err := svc.Create(ctx, session, org, connections.CreateRequest{Kind: "forge", Provider: "gitea", Name: "Probe race", Endpoint: "https://forge.example.test", Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge"}, Secret: "private"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	svc.Register("forge", "gitea", func(context.Context, connections.Resolved) (connections.ProbeResult, error) {
		calls.Add(1)
		return connections.ProbeResult{State: "healthy"}, nil
	})
	result := make(chan error, 1)
	err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM organisations WHERE id=$1 FOR UPDATE`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE connections SET state='revoked',version=version+1 WHERE org_id=$1 AND id=$2`, org, c.ID); err != nil {
			return err
		}
		go func() { _, err := svc.Test(ctx, session, org, c.ID, c.Version, "test"); result <- err }()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			var waiting bool
			if err := db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND usename=current_user AND wait_event_type='Lock' AND pid<>pg_backend_pid())`).Scan(&waiting); err != nil {
				return err
			}
			if waiting {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
		return errors.New("probe did not wait for revocation lock")
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if err != auth.ErrConflict && err != connections.ErrRevoked {
			t.Fatalf("revoked probe: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("probe stalled")
	}
	if calls.Load() != 0 {
		t.Fatal("provider called after committed revocation")
	}
}
