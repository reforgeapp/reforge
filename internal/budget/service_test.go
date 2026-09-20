package budget

import (
	"context"
	"errors"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/store"
)

type fixture struct {
	db                                    *store.Store
	s                                     *Service
	identity                              *auth.Service
	owner                                 auth.Session
	lease                                 Lease
	quote                                 Quote
	campaign, connection, team, otherTeam string
}

func fixtureFence(ctx context.Context, tx pgx.Tx, l Lease) error {
	if err := lockOrg(ctx, tx, l.OrgID); err != nil {
		return err
	}
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT v.policy_hash=$3 AND NOT EXISTS(SELECT 1 FROM workflow_pauses p WHERE p.org_id=$1 AND p.paused AND (p.scope_kind='recipe' AND p.scope_id=t.recipe OR p.scope_kind='campaign' AND p.scope_id=t.campaign_id::text OR p.scope_kind='model' AND p.scope_id=t.model_connection_id::text)) FROM workflow_tasks t JOIN policy_bindings b ON b.org_id=t.org_id AND b.scope_kind='organisation' JOIN policy_versions v ON v.org_id=b.org_id AND v.id=b.policy_version_id WHERE t.org_id=$1 AND t.id=$2`, l.OrgID, l.TaskID, l.PolicyHash).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRevoked
	}
	return nil
}
func fixtureCampaign(ctx context.Context, tx pgx.Tx, org, id string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_tasks WHERE org_id=$1 AND campaign_id=$2)`, org, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrRevoked
	}
	return nil
}

func setup(t *testing.T) *fixture {
	t.Helper()
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path != "/reforge_test" || u.Fragment != "" {
		t.Fatal("test requires disposable reforge_test database")
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
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	t.Cleanup(db.Close)
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{db: db, identity: identity, owner: auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}}, connection: domain.NewID(), campaign: domain.NewID(), team: domain.NewID(), otherTeam: domain.NewID()}
	f.s = New(db, identity, fixtureFence, fixtureCampaign)
	f.lease = Lease{OrgID: domain.NewID(), RepositoryID: domain.NewID(), TaskID: domain.NewID(), JobID: domain.NewID(), AttemptID: domain.NewID(), OperationID: domain.NewID(), WorkerID: "budget-fixture", Fence: 1, PolicyHash: "fixture-policy", ExpiresAt: time.Now().Add(time.Hour)}
	err = db.Identity(ctx, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'budget-test',$1::text,'Budget test','test@example.test')`, f.owner.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, f.owner.ID, f.owner.User.ID, domain.NewID(), domain.NewID())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Tenant(ctx, f.lease.OrgID, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Budget integration')`, f.lease.OrgID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, f.lease.OrgID, f.owner.User.ID); err != nil {
			return err
		}
		policyVersion := domain.NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO policy_versions(org_id,id,scope_kind,scope_id,document,policy_hash,actor_id,reason) VALUES($1,$2,'organisation',$1,'{"schema":"maintenance/v1"}',$3,$4,'fixture')`, f.lease.OrgID, policyVersion, f.lease.PolicyHash, f.owner.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO policy_bindings(org_id,scope_kind,scope_id,policy_version_id,version,simulation_hash) VALUES($1,'organisation',$1,$2,1,'fixture')`, f.lease.OrgID, policyVersion); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,verified_at) VALUES($1,$2,'model','compatible','Budget model','https://model.example','{"billing_route":"direct_api","model":"fixture"}','healthy',now())`, f.lease.OrgID, f.connection); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Budget repository')`, f.lease.OrgID, f.lease.RepositoryID); err != nil {
			return err
		}
		for _, id := range []string{f.team, f.otherTeam} {
			if _, err := tx.Exec(ctx, `INSERT INTO teams(org_id,id,name) VALUES($1,$2::uuid,$2::text)`, f.lease.OrgID, id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO team_repositories(org_id,team_id,repository_id) VALUES($1,$2,$3)`, f.lease.OrgID, id, f.lease.RepositoryID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,model_route,model_connection_id,campaign_id,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,'hash','repair','1','main','api',$6,$7,$8,$8,'repairing',3,$9)`, f.lease.OrgID, f.lease.TaskID, f.lease.RepositoryID, domain.NewID(), domain.NewID(), f.connection, f.campaign, f.lease.PolicyHash, f.owner.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_jobs(org_id,id,task_id,operation_id,state,fence,attempts,lease_owner,lease_expires_at) VALUES($1,$2,$3,$4,'running',1,1,$5,clock_timestamp()+interval '1 hour')`, f.lease.OrgID, f.lease.JobID, f.lease.TaskID, f.lease.OperationID, f.lease.WorkerID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO workflow_attempts(org_id,id,task_id,job_id,number,fence,lease_owner,state) VALUES($1,$2,$3,$4,1,1,$5,'running')`, f.lease.OrgID, f.lease.AttemptID, f.lease.TaskID, f.lease.JobID, f.lease.WorkerID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	route, err := f.s.PutRoute(ctx, f.owner, f.lease.OrgID, Route{ConnectionID: f.connection, Model: "fixture", Name: "api", Mode: "priced", PricingVersion: "fixture-pricing", RequestMicroUSD: 1, MaxInputTokens: 100, MaxOutputTokens: 100, MaxMilliseconds: 10000, MaxRequests: 1}, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	f.quote = Quote{OperationID: domain.NewID(), Model: route.Model, Route: route.Name, RouteVersion: route.Version, InputTokens: 10, MaxOutputTokens: 10, MaxMilliseconds: 1000, MaxRequests: 1}
	return f
}
func (f *fixture) limit(t *testing.T, scope Scope, money int64) Limit {
	t.Helper()
	l, err := f.s.PutLimit(context.Background(), f.owner, f.lease.OrgID, Limit{Scope: scope, Period: "daily", Caps: Caps{MicroUSD: n(money), Tokens: n(100000), Milliseconds: n(1000000), Requests: n(1000), Concurrency: n(1000)}}, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func (f *fixture) reserve(q Quote) (Reservation, error) {
	var r Reservation
	err := f.db.Tenant(context.Background(), f.lease.OrgID, f.owner.User.ID, func(tx pgx.Tx) error {
		var err error
		r, err = f.s.ReserveTx(context.Background(), tx, f.lease, q)
		return err
	})
	return r, err
}
func (f *fixture) tx(fn func(pgx.Tx) error) error {
	return f.db.Tenant(context.Background(), f.lease.OrgID, f.owner.User.ID, fn)
}

func TestHundredContendingHierarchicalReservations(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for _, scope := range []struct {
		s   Scope
		cap int64
	}{{Scope{"organisation", f.lease.OrgID}, 50}, {Scope{"team", f.team}, 30}, {Scope{"team", f.otherTeam}, 45}, {Scope{"repository", f.lease.RepositoryID}, 40}, {Scope{"connection", f.connection}, 60}, {Scope{"campaign", f.campaign}, 35}} {
		f.limit(t, scope.s, scope.cap)
	}
	var wg sync.WaitGroup
	results := make(chan Reservation, 100)
	failures := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q := f.quote
			q.OperationID = domain.NewID()
			r, err := f.reserve(q)
			if err == nil {
				results <- r
			} else {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	if len(results) != 30 {
		t.Fatalf("reserved %d, expected tightest team ceiling 30", len(results))
	}
	for err := range failures {
		if !errors.Is(err, ErrCapacity) {
			t.Fatalf("unexpected contention error: %v", err)
		}
	}
	var first Reservation
	for r := range results {
		first = r
		if len(r.Scopes) != 6 {
			t.Fatalf("ancestor omitted: %+v", r.Scopes)
		}
	}
	org, err := f.s.GetLimit(ctx, f.owner, f.lease.OrgID, Scope{"organisation", f.lease.OrgID})
	if err != nil || org.Held.MicroUSD != 30 || org.Held.Concurrency != 30 {
		t.Fatalf("incorrect held totals: %+v %v", org, err)
	}
	f.s = New(f.db, f.identity, fixtureFence, fixtureCampaign)
	again, err := f.reserve(first.Quote)
	if err != nil || again.ID != first.ID {
		t.Fatalf("restart duplicate not idempotent: %v", err)
	}
	bad := first.Quote
	bad.MaxOutputTokens++
	if _, err = f.reserve(bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("operation changed quote: %v", err)
	}
	err = f.tx(func(tx pgx.Tx) error { _, err := f.s.CancelTx(ctx, tx, f.lease.OrgID, first.ID, ""); return err })
	if err != nil {
		t.Fatal(err)
	}
	q := f.quote
	q.OperationID = domain.NewID()
	if _, err = f.reserve(q); err != nil {
		t.Fatalf("provably unused reservation not released: %v", err)
	}
}

func TestUnknownRolloverSettlementAndDebt(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.limit(t, Scope{"organisation", f.lease.OrgID}, 1)
	f.limit(t, Scope{"campaign", f.campaign}, 10)
	r, err := f.reserve(f.quote)
	if err != nil {
		t.Fatal(err)
	}
	err = f.tx(func(tx pgx.Tx) error {
		var err error
		r, err = f.s.MarkDispatchedTx(ctx, tx, f.lease, r.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.tx(func(tx pgx.Tx) error { _, err := f.s.MarkDispatchedTx(ctx, tx, f.lease, r.ID); return err })
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate dispatch accepted: %v", err)
	}
	err = f.tx(func(tx pgx.Tx) error {
		var err error
		r, err = f.s.MarkUnknownTx(ctx, tx, f.lease.OrgID, r.ID, "provider timeout")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.tx(func(tx pgx.Tx) error {
		for i := range r.Scopes {
			r.Scopes[i].PeriodStart = r.Scopes[i].PeriodStart.Add(-24 * time.Hour)
		}
		return saveReservation(ctx, tx, r)
	})
	if err != nil {
		t.Fatal(err)
	}
	q := f.quote
	q.OperationID = domain.NewID()
	if _, err = f.reserve(q); !errors.Is(err, ErrCapacity) {
		t.Fatalf("rollover forgot unknown hold: %v", err)
	}
	err = f.tx(func(tx pgx.Tx) error { _, err := f.s.CancelTx(ctx, tx, f.lease.OrgID, r.ID, ""); return err })
	if !errors.Is(err, ErrUnknown) {
		t.Fatalf("uncertain charge released by cancellation: %v", err)
	}
	settlement := Settlement{Known: true, Actual: Amount{MicroUSD: 2, Tokens: 10, Milliseconds: 500, Requests: 1}, Reference: "signed-provider-charge"}
	err = f.tx(func(tx pgx.Tx) error {
		var err error
		r, err = f.s.SettleTx(ctx, tx, f.lease.OrgID, r.ID, settlement)
		return err
	})
	if err != nil || r.Debt.MicroUSD != 1 {
		t.Fatalf("observed overcharge lost: %+v %v", r, err)
	}
	err = f.tx(func(tx pgx.Tx) error { _, err := f.s.SettleTx(ctx, tx, f.lease.OrgID, r.ID, settlement); return err })
	if err != nil {
		t.Fatalf("duplicate settlement failed: %v", err)
	}
	if _, err = f.reserve(q); !errors.Is(err, ErrRevoked) {
		t.Fatalf("old-period debt failed to block new dispatch: %v", err)
	}
	org, err := f.s.GetLimit(ctx, f.owner, f.lease.OrgID, Scope{"organisation", f.lease.OrgID})
	if err != nil || !org.Paused || org.Held != (Amount{}) {
		t.Fatalf("debt pause or concurrency release failed: %+v %v", org, err)
	}
}

func TestBudgetRevalidationAndUnknownRoutes(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	org := f.limit(t, Scope{"organisation", f.lease.OrgID}, 10)
	f.limit(t, Scope{"campaign", f.campaign}, 10)
	r, err := f.reserve(f.quote)
	if err != nil {
		t.Fatal(err)
	}
	org.Caps.MicroUSD = n(5)
	if _, err = f.s.PutLimit(ctx, f.owner, f.lease.OrgID, org, org.Version, "ceiling changed"); err != nil {
		t.Fatal(err)
	}
	err = f.tx(func(tx pgx.Tx) error { _, err := f.s.MarkDispatchedTx(ctx, tx, f.lease, r.ID); return err })
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("changed ceiling version accepted: %v", err)
	}
	org.Period = "monthly"
	if _, err = f.s.PutLimit(ctx, f.owner, f.lease.OrgID, org, 2, "reset accounting"); !errors.Is(err, ErrConflict) {
		t.Fatalf("period change reset spend: %v", err)
	}
	q := f.quote
	q.OperationID = domain.NewID()
	q.Model = "missing"
	if _, err = f.reserve(q); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown pricing accepted: %v", err)
	}
	q = f.quote
	q.OperationID = domain.NewID()
	q.Route = "alternate"
	if _, err = f.reserve(q); !errors.Is(err, ErrRevoked) {
		t.Fatalf("unbound model route accepted: %v", err)
	}
	wrong := f.lease
	wrong.Fence++
	err = f.tx(func(tx pgx.Tx) error { _, err := f.s.ReserveTx(ctx, tx, wrong, f.quote); return err })
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("stale fence accepted: %v", err)
	}
	err = f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET state='revoked',revoked_at=now(),version=version+1 WHERE org_id=$1 AND id=$2`, f.lease.OrgID, f.connection)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	q = f.quote
	q.OperationID = domain.NewID()
	if _, err = f.reserve(q); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked connection accepted: %v", err)
	}
}

func TestSubscriptionQuotaAndChangedScope(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.limit(t, Scope{"organisation", f.lease.OrgID}, 0)
	f.limit(t, Scope{"campaign", f.campaign}, 0)
	err := f.tx(func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE connections SET kind='agent',settings='{"billing_route":"subscription"}',version=version+1 WHERE org_id=$1 AND id=$2`, f.lease.OrgID, f.connection); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE workflow_tasks SET model_route='subscription' WHERE org_id=$1 AND id=$2`, f.lease.OrgID, f.lease.TaskID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	route := Route{ConnectionID: f.connection, Model: "fixture", Name: "subscription", Mode: "quota", MaxInputTokens: 100, MaxOutputTokens: 100, MaxMilliseconds: 10000, MaxRequests: 1}
	route, err = f.s.PutRoute(ctx, f.owner, f.lease.OrgID, route, 0, "quota")
	if err != nil {
		t.Fatal(err)
	}
	q := f.quote
	q.Route = route.Name
	q.RouteVersion = route.Version
	if _, err = f.reserve(q); !errors.Is(err, ErrRevoked) {
		t.Fatalf("unqualified subscription route accepted: %v", err)
	}
	asserted := route
	asserted.Qualified = true
	asserted.QualificationRef = "browser assertion"
	if _, err = f.s.PutRoute(ctx, f.owner, f.lease.OrgID, asserted, route.Version, "forged qualification"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("browser qualified runtime: %v", err)
	}
	err = f.tx(func(tx pgx.Tx) error {
		var err error
		route, err = f.s.QualifyRouteTx(ctx, tx, f.lease.OrgID, f.connection, route.Model, route.Name, route.Version, "trusted-T16-fixture")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	q.RouteVersion = route.Version
	r, err := f.reserve(q)
	if err != nil || r.Maximum.MicroUSD != 0 || r.Route.Mode != "quota" {
		t.Fatalf("qualified quota route failed: %+v %v", r, err)
	}
	f.limit(t, Scope{"team", f.team}, 0)
	err = f.tx(func(tx pgx.Tx) error { _, err := f.s.MarkDispatchedTx(ctx, tx, f.lease, r.ID); return err })
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("newly configured ancestor omitted: %v", err)
	}
}

func TestInjectedAuthorityChecksAndRetainedSpend(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	org := f.limit(t, Scope{"organisation", f.lease.OrgID}, 10)
	f.limit(t, Scope{"campaign", f.campaign}, 10)
	disabled := New(f.db, f.identity, nil, nil)
	err := f.tx(func(tx pgx.Tx) error { _, err := disabled.ReserveTx(ctx, tx, f.lease, f.quote); return err })
	if !errors.Is(err, ErrUnknown) {
		t.Fatalf("missing authority callback accepted: %v", err)
	}
	if _, err = disabled.PutLimit(ctx, f.owner, f.lease.OrgID, Limit{Scope: Scope{"campaign", domain.NewID()}, Period: "daily", Caps: Caps{MicroUSD: n(1)}}, 0, "campaign unavailable"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("missing campaign authority accepted: %v", err)
	}
	r, err := f.reserve(f.quote)
	if err != nil {
		t.Fatal(err)
	}
	err = f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO workflow_pauses(org_id,scope_kind,scope_id,paused) VALUES($1,'recipe','repair',true)`, f.lease.OrgID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.tx(func(tx pgx.Tx) error { _, err := f.s.MarkDispatchedTx(ctx, tx, f.lease, r.ID); return err })
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("paused recipe bypassed callback: %v", err)
	}
	err = f.tx(func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_pauses SET paused=false WHERE org_id=$1`, f.lease.OrgID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.tx(func(tx pgx.Tx) error { _, err := f.s.MarkDispatchedTx(ctx, tx, f.lease, r.ID); return err })
	if err != nil {
		t.Fatal(err)
	}
	err = f.tx(func(tx pgx.Tx) error {
		_, err := f.s.SettleTx(ctx, tx, f.lease.OrgID, r.ID, Settlement{Known: true, Actual: Amount{MicroUSD: 1, Tokens: 10, Milliseconds: 500, Requests: 1}, Reference: "known-provider-usage"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := f.s.PutLimit(ctx, f.owner, f.lease.OrgID, org, org.Version, "unchanged limits")
	if err != nil || updated.Spent.MicroUSD != 1 {
		t.Fatalf("limit edit falsely erased displayed spend: %+v %v", updated, err)
	}
	q := f.quote
	q.OperationID = domain.NewID()
	r, err = f.reserve(q)
	if err != nil {
		t.Fatal(err)
	}
	err = f.tx(func(tx pgx.Tx) error {
		version := domain.NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO policy_versions(org_id,id,scope_kind,scope_id,document,policy_hash,actor_id,reason) VALUES($1,$2,'organisation',$1,'{"schema":"maintenance/v1"}','new-policy',$3,'changed policy')`, f.lease.OrgID, version, f.owner.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE policy_bindings SET policy_version_id=$2,version=version+1 WHERE org_id=$1 AND scope_kind='organisation'`, f.lease.OrgID, version)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.tx(func(tx pgx.Tx) error { _, err := f.s.MarkDispatchedTx(ctx, tx, f.lease, r.ID); return err })
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("changed activated policy bypassed callback: %v", err)
	}
}
