package insights_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/config"
	"reforge/internal/domain"
	"reforge/internal/httpapi"
	"reforge/internal/insights"
	"reforge/internal/policy"
	"reforge/internal/store"
)

func TestScopedLedgerAndAuditExport(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path != "/reforge_test" || u.Fragment != "" {
		t.Fatal("requires disposable reforge_test")
	}
	for k, v := range u.Query() {
		if (k != "host" && k != "port" && k != "sslmode") || len(v) != 1 {
			t.Fatal("unsupported database parameters")
		}
	}
	ctx := context.Background()
	db, e := store.Open(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	identity, e := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if e != nil {
		t.Fatal(e)
	}
	service := insights.New(identity)
	owner := auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}}
	viewer := auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}}
	token := domain.NewID() + "fixture"
	hash := sha256.Sum256([]byte(token))
	org, other, repo, hidden, connection, team := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	for _, s := range []auth.Session{owner, viewer} {
		e = db.Identity(ctx, s.User.ID, func(tx pgx.Tx) error {
			if _, e := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'insights-test',$1::text,'Insights','test@example.test')`, s.User.ID); e != nil {
				return e
			}
			h := domain.NewID()
			if s.ID == viewer.ID {
				h = hex.EncodeToString(hash[:])
			}
			_, e := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, s.ID, s.User.ID, h, domain.NewID())
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	e = db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		queries := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO organisations(id,name) VALUES($1,'Insights')`, []any{org}},
			{`INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true),($1,$3,'viewer',false)`, []any{org, owner.User.ID, viewer.User.ID}},
			{`INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Visible'),($1,$3::uuid,$3::text,'Hidden')`, []any{org, repo, hidden}},
			{`INSERT INTO member_repositories(org_id,user_id,repository_id) VALUES($1,$2,$3)`, []any{org, viewer.User.ID, repo}},
			{`INSERT INTO teams(org_id,id,name) VALUES($1,$2,'Scope')`, []any{org, team}},
			{`INSERT INTO team_repositories(org_id,team_id,repository_id) VALUES($1,$2,$3)`, []any{org, team, repo}},
			{`INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','compatible','Fixture','https://model.example','{}','healthy')`, []any{org, connection}},
		}
		for _, q := range queries {
			if _, e := tx.Exec(ctx, q.sql, q.args...); e != nil {
				return e
			}
		}
		for i, state := range []string{"settled", "unknown", "reserved", "dispatched", "cancelled", "settled"} {
			rid := repo
			if i == 5 {
				rid = hidden
			}
			task, job, attempt, operation := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
			if _, e := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,model_connection_id,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4::uuid,$4::text,'hash','repair','1','main',$5,'hash','hash','repairing',3,$6)`, org, task, rid, operation, connection, owner.User.ID); e != nil {
				return e
			}
			if _, e := tx.Exec(ctx, `INSERT INTO workflow_jobs(org_id,id,task_id,operation_id,state,fence,attempts) VALUES($1,$2,$3,$4,'running',1,1)`, org, job, task, operation); e != nil {
				return e
			}
			if _, e := tx.Exec(ctx, `INSERT INTO workflow_attempts(org_id,id,task_id,job_id,number,fence,lease_owner,state) VALUES($1,$2,$3,$4,1,1,'test','running')`, org, attempt, task, job); e != nil {
				return e
			}
			r := budget.Reservation{ID: domain.NewID(), Lease: budget.Lease{OrgID: org, RepositoryID: rid, TaskID: task, JobID: job, AttemptID: attempt, OperationID: operation}, ConnectionID: connection, State: state, CreatedAt: at, Maximum: budget.Amount{MicroUSD: 100, Tokens: 200, Requests: 1, Concurrency: 1, Milliseconds: 1000}}
			if state == "settled" {
				r.Actual = &budget.Amount{MicroUSD: 15, Tokens: 30}
			}
			record, e := json.Marshal(r)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO budget_reservations(org_id,id,operation_id,repository_id,task_id,job_id,attempt_id,fingerprint,record,state,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,'fixture',$8,$9,$10)`, org, r.ID, operation, rid, task, job, attempt, record, state, at); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,repository_id,actor_id,action,object_id,request_id,data,occurred_at) VALUES($1,$2,$3,'system','usage.recorded',$4,'test','{"escaped":"<script>"}',$5)`, domain.NewID(), org, rid, r.ID, at); e != nil {
				return e
			}
		}
		_, e := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id) VALUES($1,$2::uuid,'system','organisation.updated',$2::text,'test')`, domain.NewID(), org)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	f := insights.Filter{Limit: 2}
	seen := map[string]bool{}
	for {
		page, e := service.Usage(ctx, viewer, org, f)
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range page.Items {
			if seen[item.Reservation.ID] || item.Reservation.Lease.RepositoryID != repo {
				t.Fatal("duplicate or leaked reservation")
			}
			seen[item.Reservation.ID] = true
		}
		if page.Complete {
			break
		}
		if page.NextCursor == "" {
			t.Fatal("missing cursor")
		}
		f.Cursor = page.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("visible usage count %d", len(seen))
	}
	f = insights.Filter{Limit: 100}
	summary, e := service.UsageSummary(ctx, viewer, org, f)
	if e != nil {
		t.Fatal(e)
	}
	if summary.Records != 5 || summary.Settled != 1 || summary.EstimatedCostMicroUSD != 15 || summary.KnownTokens != 30 || summary.UnknownMaximum.MicroUSD != 100 || summary.Held.MicroUSD != 300 || summary.Held.Requests != 3 {
		t.Fatalf("wrong summary: %+v", summary)
	}
	series, e := service.UsageSeries(ctx, viewer, org, f)
	if e != nil || len(series.Days) < 30 || series.Days[len(series.Days)-1].MicroUSD != 15 || series.Days[len(series.Days)-1].Records != 5 || len(series.Providers) != 1 || series.Providers[0].Tokens != 30 {
		t.Fatalf("usage series: %+v %v", series, e)
	}
	overview, e := service.Overview(ctx, viewer, org)
	if e != nil || len(overview.Trend) != 14 {
		t.Fatalf("overview trend: %d %v", len(overview.Trend), e)
	}
	resolve := func(context.Context, pgx.Tx, string, string) (policy.Resolved, error) {
		return policy.Resolved{Hash: "h"}, nil
	}
	setup, e := service.Setup(ctx, viewer, org, resolve, false, false)
	if e != nil || len(setup.Steps) != 10 || setup.Steps[0].Done || !setup.Steps[3].Done || !setup.Steps[8].Done || setup.Steps[9].Done {
		t.Fatalf("setup: %+v %v", setup, e)
	}
	if setup, e = service.Setup(ctx, viewer, org, resolve, true, true); e != nil || len(setup.Steps) != 8 {
		t.Fatalf("built-in setup: %+v %v", setup, e)
	}
	f.State = "unknown"
	f.Provider = "compatible"
	f.Recipe = "repair"
	f.ConnectionID = connection
	page, e := service.Usage(ctx, viewer, org, f)
	if e != nil || len(page.Items) != 1 {
		t.Fatalf("usage filters: %d %v", len(page.Items), e)
	}
	f.Until = &at
	page, e = service.Usage(ctx, viewer, org, f)
	if e != nil || len(page.Items) != 0 {
		t.Fatalf("exclusive upper bound: %v", e)
	}
	f = insights.Filter{Limit: 100, RepositoryID: hidden}
	if _, e = service.UsageSummary(ctx, viewer, org, f); !errors.Is(e, auth.ErrForbidden) {
		t.Fatalf("hidden repository: %v", e)
	}
	f = insights.Filter{Limit: 100, TeamID: team}
	if _, e = service.Usage(ctx, viewer, org, f); !errors.Is(e, auth.ErrForbidden) {
		t.Fatalf("hidden team: %v", e)
	}
	if page, e = service.Usage(ctx, owner, org, f); e != nil || len(page.Items) != 5 {
		t.Fatalf("owner team filter: %v", e)
	}
	f = insights.Filter{Limit: 100}
	if _, e = service.Usage(ctx, viewer, other, f); !errors.Is(e, auth.ErrForbidden) {
		t.Fatalf("tenant isolation: %v", e)
	}
	audit, e := service.Audit(ctx, viewer, org, f)
	if e != nil || len(audit.Items) != 5 {
		t.Fatalf("audit scope: %d %v", len(audit.Items), e)
	}
	audit, e = service.Audit(ctx, owner, org, f)
	if e != nil || len(audit.Items) != 7 {
		t.Fatalf("admin audit: %d %v", len(audit.Items), e)
	}
	f.ActorID = "system"
	f.Action = "usage.recorded"
	audit, e = service.Audit(ctx, owner, org, f)
	if e != nil || len(audit.Items) != 6 {
		t.Fatalf("audit filters: %v", e)
	}
	f.Cursor = "malformed"
	if _, e = service.Audit(ctx, viewer, org, f); !errors.Is(e, auth.ErrInvalid) {
		t.Fatalf("bad cursor: %v", e)
	}
	app := httpapi.New(config.Config{Development: true}, db)
	app.RegisterIdentity(identity)
	app.RegisterBudget(nil)
	app.RegisterInsights(service)
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/orgs/"+org+path, nil)
		req.AddCookie(&http.Cookie{Name: identity.CookieName(), Value: token})
		w := httptest.NewRecorder()
		app.Router.ServeHTTP(w, req)
		return w
	}
	w := get("/audit-events/export?limit=2")
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/x-ndjson" || w.Header().Get("X-Export-Complete") != "false" || len(strings.Split(strings.TrimSpace(w.Body.String()), "\n")) != 2 {
		t.Fatalf("export: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Next-Cursor") == "" || strings.Contains(w.Body.String(), "<script>") {
		t.Fatal("export cursor or escaping")
	}
	w = get("/usage/summary")
	if w.Code != 200 {
		t.Fatalf("summary route: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/usage/series?since=2020-01-01T00:00:00Z", "/audit-events?limit=101", "/usage?since=yesterday", "/usage?repository_id=invalid"} {
		if w = get(path); w.Code != 400 {
			t.Fatalf("invalid filter %s: %d", path, w.Code)
		}
	}
	e = db.Identity(ctx, viewer.User.ID, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE id=$1`, viewer.ID)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.Usage(ctx, viewer, org, insights.Filter{Limit: 100}); !errors.Is(e, auth.ErrUnauthenticated) {
		t.Fatalf("revoked session: %v", e)
	}
	if w = get("/audit-events/export"); w.Code != 401 {
		t.Fatalf("revoked export: %d", w.Code)
	}
}
