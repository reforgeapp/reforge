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
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/attention"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/store"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func TestPlatformFailureParksAndReleasesOnce(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	if u, err := url.Parse(raw); err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	ctx := context.Background()
	migrationURL := os.Getenv("REFORGE_TEST_MIGRATION_DATABASE_URL")
	if migrationURL == "" {
		t.Fatal("requires disposable PostgreSQL migration URL")
	}
	if err := store.MigrateFS(ctx, migrationURL, store.Migrations()); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	owner := auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}}
	org, repo, otherRepo, finding, task := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	connection, model, pool, runner := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	unsupportedFinding, restoreFinding, dismissedFinding := domain.NewID(), domain.NewID(), domain.NewID()
	policies, err := policy.New(db, identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	budgets := budget.New(db, identity, nil, nil)
	service := New(db, identity, nil, nil, budgets, policies)
	route, _ := json.Marshal(budget.Route{ConnectionID: model, Model: "lifecycle-model", Name: "default", Mode: "priced", PricingVersion: "fixture-v1", InputMicroUSDPerMillion: 1, OutputMicroUSDPerMillion: 1, MaxInputTokens: 4096, MaxOutputTokens: 1024, MaxMilliseconds: 600_000, MaxRequests: 1, Version: 1})
	modelSettings, _ := json.Marshal(map[string]string{"model": "lifecycle-model"})
	if err = db.Identity(ctx, owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'lifecycle-test',$1::text,'Owner','lifecycle@example.test')`, owner.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, owner.ID, owner.User.ID, domain.NewID(), domain.NewID())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err = db.Tenant(ctx, org, owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Lifecycle')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, owner.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,'lifecycle','team/lifecycle')`, org, repo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,'lifecycle-other','team/lifecycle-other')`, org, otherRepo); err != nil {
			return err
		}
		if err := policies.ApplyMaintenanceTx(ctx, tx, org, repo, owner.User.ID, domain.NewID(), policy.Policy{Schema: "maintenance/v1"}, true); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'forge','github','GitHub','https://api.github.com','{}','healthy')`, org, connection); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE repositories SET connection_id=$3 WHERE org_id=$1 AND id=$2`, org, repo, connection); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runner_pools(org_id,id,name,state) VALUES($1,$2,'Lifecycle','active')`, org, pool); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id) VALUES($1,$2,$3)`, org, pool, repo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runners(org_id,id,pool_id,name,credential_hash,credential_expires_at,state) VALUES($1,$2,$3,'Lifecycle','hash',now()+interval '1 day','active')`, org, runner, pool); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO autopilot_settings(org_id,enabled,enabled_by) VALUES($1,false,$2)`, org, owner.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO autopilot_requests(org_id,repository_id,requested_by) VALUES($1,$2,$3)`, org, repo, owner.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','openai','Lifecycle model','https://api.openai.com',$3,'healthy')`, org, model, modelSettings); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO budget_routes(org_id,connection_id,model,name,config,version) VALUES($1,$2,'lifecycle-model','default',$3,1)`, org, model, route); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO budget_limits(org_id,scope_kind,scope_id,period,period_start,period_end,caps,version) VALUES($1,'organisation',$1,'daily',date_trunc('day',now()),date_trunc('day',now())+interval '1 day','{"micro_usd":10000000,"concurrency":2}',1)`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO budget_spend(org_id,scope_kind,scope_id,period_start,amount) VALUES($1,'organisation',$1,date_trunc('day',now()),'{"micro_usd":1234}')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'scanner','lifecycle','dependency','high','Lifecycle finding',$5,$6)`, org, finding, repo, repeatByte('a', 64), []byte(`{"complete":true,"blockers":[]}`), repeatByte('b', 64)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'scanner','lifecycle-unsupported','dependency','high','Unsupported finding',$5,$6),($1,$7,$3,$8,'scanner','lifecycle-dismissed','dependency','high','Dismissed finding',$5,$9),($1,$10,$3,$11,'scanner','lifecycle-restore','dependency','high','Runner finding',$5,$12)`, org, unsupportedFinding, repo, repeatByte('d', 64), []byte(`{"complete":true,"blockers":[]}`), repeatByte('e', 64), dismissedFinding, repeatByte('f', 64), repeatByte('1', 64), restoreFinding, repeatByte('2', 64), repeatByte('3', 64)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE maintenance_findings SET state='dismissed' WHERE org_id=$1 AND id=$2`, org, dismissedFinding); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE maintenance_findings SET last_seen=clock_timestamp()-interval '2 hours' WHERE org_id=$1 AND id=ANY($2::uuid[])`, org, []string{unsupportedFinding, restoreFinding}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO autopilot_attempts(org_id,finding_id,finding_version,outcome,runs,max_runs,capability,blocker_kind,reason) VALUES($1,$2,1,'blocked',0,6,'old','unsupported','raw model essay must not surface'),($1,$3,1,'blocked',0,6,'old','platform','dismissed internal state'),($1,$4,1,'blocked',0,6,'old','restore_runner','No runner pool includes this repository')`, org, unsupportedFinding, dismissedFinding, restoreFinding); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main','policy','policy','failed',3,$7)`, org, task, repo, domain.NewID(), domain.NewID(), repeatByte('c', 64), owner.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO autopilot_attempts(org_id,finding_id,finding_version,task_id,outcome,runs,max_runs,capability) VALUES($1,$2,1,$3,'queued',6,6,'old')`, org, finding, task); err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]any{"state": "failed", "platform_failure": true})
		return workflow.EmitTx(ctx, tx, domain.Event{OrgID: org, RepositoryID: repo, Type: "task.failed", AggregateType: "task", AggregateID: task, DataVersion: 1, Data: data})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Step(ctx, org); err != nil {
		t.Fatal(err)
	}
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var outcome, blocker string
		var maxRuns, tasks int
		if err := tx.QueryRow(ctx, `SELECT outcome,blocker_kind,max_runs FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2`, org, finding).Scan(&outcome, &blocker, &maxRuns); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM workflow_tasks WHERE org_id=$1 AND repository_id=$2`, org, repo).Scan(&tasks); err != nil {
			return err
		}
		if outcome != "blocked" || blocker != "platform" || maxRuns != 7 || tasks != 1 {
			t.Fatalf("Step ignored platform cooldown: outcome=%s blocker=%s max=%d tasks=%d", outcome, blocker, maxRuns, tasks)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		before, err := spendMicroUSD(ctx, tx, org)
		if err != nil {
			return err
		}
		if before != 1234 {
			t.Fatalf("fixture spend missing: %d microUSD", before)
		}
		if _, err = tx.Exec(ctx, `UPDATE runners SET last_seen_at=clock_timestamp()-interval '10 minutes' WHERE org_id=$1 AND id=$2`, org, runner); err != nil {
			return err
		}
		staleFingerprint, err := service.fingerprintTx(ctx, tx, org, finding, 1)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE runners SET last_seen_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, org, runner); err != nil {
			return err
		}
		fingerprint, err := service.fingerprintTx(ctx, tx, org, finding, 1)
		if err != nil {
			return err
		}
		if fingerprint == staleFingerprint {
			t.Fatal("runner recovery did not change capability")
		}
		if err = service.updatePlatformFailuresTx(ctx, tx, org); err != nil {
			return err
		}
		heartbeatFingerprint, err := service.fingerprintTx(ctx, tx, org, finding, 1)
		if err != nil {
			return err
		}
		if heartbeatFingerprint != fingerprint {
			t.Fatalf("runner heartbeat changed capability: %s -> %s", fingerprint, heartbeatFingerprint)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO budget_limits(org_id,scope_kind,scope_id,period,period_start,period_end,caps,version) VALUES($1,'repository',$2,'daily',date_trunc('day',now()),date_trunc('day',now())+interval '1 day','{"micro_usd":100}',1)`, org, otherRepo); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE budget_limits SET caps='{"micro_usd":200}' WHERE org_id=$1 AND scope_kind='repository' AND scope_id=$2`, org, otherRepo); err != nil {
			return err
		}
		otherBudgetFingerprint, err := service.fingerprintTx(ctx, tx, org, finding, 1)
		if err != nil {
			return err
		}
		if otherBudgetFingerprint != fingerprint {
			t.Fatal("another repository budget changed this finding capability")
		}
		var runs, maxRuns, retries int
		var outcome, blocker, capability string
		var retryAfter *time.Time
		if err = tx.QueryRow(ctx, `SELECT runs,max_runs,platform_retries,outcome,blocker_kind,capability,retry_after FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2 AND finding_version=1`, org, finding).Scan(&runs, &maxRuns, &retries, &outcome, &blocker, &capability, &retryAfter); err != nil {
			return err
		}
		if runs != 6 || maxRuns != 7 || retries != 1 || outcome != "blocked" || blocker != "platform" || capability != fingerprint || retryAfter == nil || !retryAfter.After(time.Now()) {
			t.Fatalf("platform park: runs=%d max=%d retries=%d outcome=%s blocker=%s retry=%v", runs, maxRuns, retries, outcome, blocker, retryAfter)
		}
		if err = service.updatePlatformFailuresTx(ctx, tx, org); err != nil {
			return err
		}
		var unchanged int
		if err = tx.QueryRow(ctx, `SELECT max_runs FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2`, org, finding).Scan(&unchanged); err != nil {
			return err
		}
		if unchanged != 7 {
			t.Fatalf("duplicate platform credit: max_runs=%d", unchanged)
		}
		a, err := identity.ActorTx(ctx, tx, owner, org)
		if err != nil {
			return err
		}
		needs, err := attention.ListTx(ctx, tx, a, 100, "")
		if err != nil {
			return err
		}
		for _, need := range needs.Items {
			if need.FindingID == finding || need.FindingID == unsupportedFinding || need.FindingID == restoreFinding {
				t.Fatalf("parked platform attempt surfaced in Needs: %+v", need)
			}
		}
		work, err := attention.WorkTx(ctx, tx, a, "blocked", 100, "")
		if err != nil {
			return err
		}
		foundPlatform, foundUnsupported, foundRestore := false, false, false
		for _, item := range work.Items {
			if item.ID == "parked:"+task && item.Kind == "repair" && item.State == "waiting" && item.Bucket == "blocked" && item.URL == "runs?run="+task {
				foundPlatform = true
			}
			if item.ID == "parked:"+unsupportedFinding+":1:unsupported" && item.URL == "repositories?repository="+repo && !strings.Contains(item.Reason, "raw model essay") {
				foundUnsupported = true
			}
			if item.ID == "parked:"+restoreFinding+":1:restore_runner" && item.URL == "repositories?repository="+repo {
				foundRestore = true
			}
			if item.ID == "parked:"+dismissedFinding+":1:platform" {
				t.Fatalf("dismissed finding surfaced in Work: %+v", item)
			}
		}
		if !foundPlatform || !foundUnsupported || !foundRestore {
			t.Fatalf("parked attempt missing from Work: %+v", work.Items)
		}
		after, err := spendMicroUSD(ctx, tx, org)
		if err != nil {
			return err
		}
		if after != before {
			t.Fatalf("platform recovery changed spend: %d -> %d microUSD", before, after)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for retry := 2; retry <= 3; retry++ {
		nextTask := domain.NewID()
		err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main','policy','policy','failed',3,$7)`, org, nextTask, repo, domain.NewID(), domain.NewID(), repeatByte('4', 64), owner.User.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE autopilot_attempts SET task_id=$3,outcome='queued',runs=runs+1,blocker_kind='',retry_after=NULL WHERE org_id=$1 AND finding_id=$2 AND finding_version=1`, org, finding, nextTask); err != nil {
				return err
			}
			data, _ := json.Marshal(map[string]any{"state": "failed", "platform_failure": true})
			if err := workflow.EmitTx(ctx, tx, domain.Event{OrgID: org, RepositoryID: repo, Type: "task.failed", AggregateType: "task", AggregateID: nextTask, DataVersion: 1, Data: data}); err != nil {
				return err
			}
			if err := service.updatePlatformFailuresTx(ctx, tx, org); err != nil {
				return err
			}
			var runs, maxRuns, platformRetries int
			var retryAfter *time.Time
			if err := tx.QueryRow(ctx, `SELECT runs,max_runs,platform_retries,retry_after FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2`, org, finding).Scan(&runs, &maxRuns, &platformRetries, &retryAfter); err != nil {
				return err
			}
			if runs != 5+retry || maxRuns != 6+retry || platformRetries != min(retry, 2) || (retry == 3 && retryAfter != nil) || (retry == 2 && (retryAfter == nil || !retryAfter.After(time.Now()))) {
				t.Fatalf("platform circuit retry %d: runs=%d max=%d retries=%d retry=%v", retry, runs, maxRuns, platformRetries, retryAfter)
			}
			return service.updatePlatformFailuresTx(ctx, tx, org)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var oldFingerprint string
		if err := tx.QueryRow(ctx, `SELECT capability FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2`, org, finding).Scan(&oldFingerprint); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE autopilot_attempts SET outcome='blocked',blocker_kind='exhausted',runs=max_runs WHERE org_id=$1 AND finding_id=$2`, org, finding); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE connections SET state='degraded' WHERE org_id=$1 AND id=$2`, org, connection); err != nil {
			return err
		}
		newFingerprint, err := service.fingerprintTx(ctx, tx, org, finding, 1)
		if err != nil {
			return err
		}
		if oldFingerprint == newFingerprint {
			t.Fatal("connection state change did not change capability")
		}
		if _, err := tx.Exec(ctx, `UPDATE autopilot_attempts SET capability=$3 WHERE org_id=$1 AND finding_id=$2`, org, finding, oldFingerprint); err != nil {
			return err
		}
		if err := service.releaseChangedAttemptsTx(ctx, tx, org); err != nil {
			return err
		}
		var runs, maxRuns int
		var outcome string
		if err := tx.QueryRow(ctx, `SELECT runs,max_runs,outcome FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2`, org, finding).Scan(&runs, &maxRuns, &outcome); err != nil {
			return err
		}
		if maxRuns != runs+1 || outcome != "retry" {
			t.Fatalf("changed capability did not release one attempt: runs=%d max=%d outcome=%s", runs, maxRuns, outcome)
		}
		if err := service.releaseChangedAttemptsTx(ctx, tx, org); err != nil {
			return err
		}
		var repeated int
		if err := tx.QueryRow(ctx, `SELECT max_runs FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2`, org, finding).Scan(&repeated); err != nil {
			return err
		}
		if repeated != maxRuns {
			t.Fatalf("same capability released another attempt: %d -> %d", maxRuns, repeated)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM runner_pool_repositories WHERE org_id=$1 AND pool_id=$2 AND repository_id=$3`, org, pool, repo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE connections SET state='healthy' WHERE org_id=$1 AND id=$2`, org, connection); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE autopilot_attempts SET retry_after=clock_timestamp() WHERE org_id=$1 AND finding_id=$2`, org, finding); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Step(ctx, org); err != nil {
		t.Fatal(err)
	}
	if err = service.Step(ctx, org); err != nil {
		t.Fatal(err)
	}
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var runs, maxRuns int
		var blocker string
		var retryAfter *time.Time
		if err := tx.QueryRow(ctx, `SELECT runs,max_runs,blocker_kind,retry_after FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2`, org, finding).Scan(&runs, &maxRuns, &blocker, &retryAfter); err != nil {
			return err
		}
		if runs != maxRuns-1 || maxRuns != 10 || blocker != "restore_runner" || retryAfter == nil || !retryAfter.After(time.Now()) {
			t.Fatalf("bounded Step retry: runs=%d max=%d blocker=%s retry=%v", runs, maxRuns, blocker, retryAfter)
		}
		var tasks int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM workflow_tasks WHERE org_id=$1 AND repository_id=$2`, org, repo).Scan(&tasks); err != nil {
			return err
		}
		if tasks != 3 {
			t.Fatalf("Step queued work without a runner: task count=%d", tasks)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	staleTask := domain.NewID()
	err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE maintenance_findings SET version=version+1 WHERE org_id=$1 AND id=$2`, org, finding); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main','policy','policy','failed',3,$7)`, org, staleTask, repo, domain.NewID(), domain.NewID(), repeatByte('5', 64), owner.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE autopilot_attempts SET task_id=$3,outcome='queued' WHERE org_id=$1 AND finding_id=$2 AND finding_version=1`, org, finding, staleTask); err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]any{"state": "failed", "platform_failure": true})
		return workflow.EmitTx(ctx, tx, domain.Event{OrgID: org, RepositoryID: repo, Type: "task.failed", AggregateType: "task", AggregateID: staleTask, DataVersion: 1, Data: data})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Step(ctx, org); err != nil {
		t.Fatalf("stale finding version blocked Step: %v", err)
	}
}

func repeatByte(b byte, n int) string {
	return strings.Repeat(string(b), n)
}

func spendMicroUSD(ctx context.Context, tx pgx.Tx, org string) (int64, error) {
	var total int64
	err := tx.QueryRow(ctx, `SELECT coalesce(sum((amount->>'micro_usd')::bigint),0) FROM budget_spend WHERE org_id=$1`, org).Scan(&total)
	return total, err
}
