package policy

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/store"
)

func policyTestDB(t *testing.T) *store.Store {
	t.Helper()
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable reforge_test PostgreSQL")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path != "/reforge_test" || u.Fragment != "" {
		t.Fatal("test database must explicitly target reforge_test")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		t.Fatal("invalid test database parameters")
	}
	for key, values := range q {
		if (key != "host" && key != "port" && key != "sslmode") || len(values) != 1 {
			t.Fatal("unsupported test database parameter")
		}
	}
	db, err := store.Open(context.Background(), raw)
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func policyIdentity(t *testing.T, db *store.Store) auth.Session {
	t.Helper()
	session := auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}}
	err := db.Identity(context.Background(), session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'policy-test',$1::text,'Policy test','test@example.test')`, session.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, session.ID, session.User.ID, domain.NewID(), domain.NewID())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func TestPersistedActivationAndScope(t *testing.T) {
	db := policyTestDB(t)
	ctx := context.Background()
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(db, identity, Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	owner := policyIdentity(t, db)
	admin := policyIdentity(t, db)
	viewer := policyIdentity(t, db)
	org, repo, otherRepo, team := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	err = db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Policy integration')`, org); err != nil {
			return err
		}
		for _, m := range []struct {
			s    auth.Session
			role string
			all  bool
		}{{owner, "owner", true}, {admin, "admin", false}, {viewer, "viewer", false}} {
			if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,$3,$4)`, org, m.s.User.ID, m.role, m.all); err != nil {
				return err
			}
		}
		for _, id := range []string{repo, otherRepo} {
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Policy repository')`, org, id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO teams(org_id,id,name) VALUES($1,$2,'Policy team')`, org, team); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO team_repositories(org_id,team_id,repository_id) VALUES($1,$2,$3)`, org, team, repo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO team_memberships(org_id,team_id,user_id) VALUES($1,$2,$3)`, org, team, admin.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO member_repositories(org_id,user_id,repository_id) VALUES($1,$2,$3)`, org, viewer.User.ID, repo)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	p := baseLayers()[0].Policy
	orgVersion, err := s.CreateVersion(ctx, owner, org, Scope{"organisation", org}, p, "set organisation limits", "request")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateVersion(ctx, admin, org, Scope{"organisation", org}, p, "admin widening ancestor", "request"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("admin changed organisation: %v", err)
	}
	if _, err = s.CreateVersion(ctx, viewer, org, Scope{"repository", repo}, p, "viewer writes policy", "request"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("viewer changed policy: %v", err)
	}
	if _, err = s.CreateVersion(ctx, admin, org, Scope{"repository", otherRepo}, p, "cross-team policy", "request"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("cross-team policy accepted: %v", err)
	}
	resolved, err := s.Resolve(ctx, owner, org, repo)
	if err != nil || len(resolved.Problems) == 0 {
		t.Fatalf("proposal activated itself: %+v %v", resolved, err)
	}
	simulation, err := s.Simulate(ctx, owner, org, orgVersion.ID, "", "", Input{Action: Repair})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Activate(ctx, owner, org, orgVersion.ID, "", "", 0, simulation.Hash, "approve initial policy", "request"); err != nil {
		t.Fatal(err)
	}
	resolved, err = s.Resolve(ctx, owner, org, repo)
	if err != nil || len(resolved.Problems) > 0 {
		t.Fatalf("active policy unresolved: %+v %v", resolved, err)
	}
	oldInput := trustedInput(resolved, Merge)
	if decision := Evaluate(resolved, oldInput); decision.Outcome != "allow" {
		t.Fatalf("baseline blocked: %+v", decision)
	}
	repoVersion, err := s.CreateVersion(ctx, admin, org, Scope{"repository", repo}, Policy{Schema: "maintenance/v1", Deny: []Action{Merge}}, "disable automatic merges", "request")
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := s.Simulate(ctx, admin, org, repoVersion.ID, repo, "", oldInput)
	if err != nil {
		t.Fatal(err)
	}
	still, err := s.Resolve(ctx, owner, org, repo)
	if err != nil || still.Hash != resolved.Hash {
		t.Fatal("simulation changed active binding")
	}
	if _, err = s.Activate(ctx, admin, org, repoVersion.ID, repo, "", 0, proposal.Hash, "activate repository denial", "request"); err != nil {
		t.Fatal(err)
	}
	current, err := s.Resolve(ctx, owner, org, repo)
	if err != nil {
		t.Fatal(err)
	}
	decision := Evaluate(current, oldInput)
	if current.Hash == resolved.Hash || decision.Outcome != "deny" || !strings.Contains(strings.Join(decision.Blockers, ";"), "policy binding") {
		t.Fatalf("old task retained authority: %+v", decision)
	}
	if _, err = s.Activate(ctx, admin, org, repoVersion.ID, repo, "", 0, proposal.Hash, "replay stale approval", "request"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale activation accepted: %v", err)
	}
	if _, err = s.GetVersion(ctx, viewer, org, repoVersion.ID); err != nil {
		t.Fatalf("scoped policy read denied: %v", err)
	}
	if _, err = s.GetVersion(ctx, admin, domain.NewID(), repoVersion.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("cross-tenant version visible: %v", err)
	}
	err = db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE policy_versions SET reason='mutated' WHERE org_id=$1 AND id=$2`, org, repoVersion.ID)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 0 {
			t.Fatal("immutable version updated")
		}
		result, err = tx.Exec(ctx, `DELETE FROM policy_versions WHERE org_id=$1 AND id=$2`, org, repoVersion.ID)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 0 {
			t.Fatal("immutable version deleted")
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE org_id=$1 AND action='policy.changed'`, org).Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Fatalf("expected atomic activation audit: %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.Logout(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateVersion(ctx, admin, org, Scope{"repository", repo}, p, "revoked identity", "request"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("revoked identity wrote policy: %v", err)
	}
}
