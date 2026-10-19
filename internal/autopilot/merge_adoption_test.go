package autopilot

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/budget"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/maintenance/discovery"
	"github.com/reforgeapp/reforge/internal/store"
)

func TestPublishedRepairAdoptionScopeCooldownAndAttemptProvenance(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	if u, err := url.Parse(raw); err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	org, user := domain.NewID(), domain.NewID()
	eligibleRepo, pausedRepo, inaccessibleRepo := domain.NewID(), domain.NewID(), domain.NewID()
	findings := []string{domain.NewID(), domain.NewID(), domain.NewID()}
	tasks := []string{domain.NewID(), domain.NewID(), domain.NewID()}
	oldTask := domain.NewID()
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'autopilot-adoption-test',$2,'Owner',$3)`, user, user, user+"@example.test"); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Autopilot adoption')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, user); err != nil {
			return err
		}
		for i, repo := range []string{eligibleRepo, pausedRepo, inaccessibleRepo} {
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name,paused,accessible) VALUES($1,$2,$3,$4,$5,$6)`, org, repo, "native-"+repo, "repo-"+repo, i == 1, i != 2); err != nil {
				return err
			}
		}
		for i, repo := range []string{eligibleRepo, pausedRepo, inaccessibleRepo} {
			if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'native_ci',$5,'test_failure','high','Repair candidate','{}',$4)`, org, findings[i], repo, strings.Repeat(string(rune(97+i)), 64), "finding-"+findings[i]); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main',$6,$6,'completed',1,$7)`, org, tasks[i], repo, domain.NewID(), "adoption-"+tasks[i], strings.Repeat("b", 64), user); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,candidate_sha,native_change) VALUES($1,$2,$3,$4,$5,1,$6,'{}','published',$7,$8)`, org, tasks[i], repo, findings[i], user, strings.Repeat("c", 64), "head", `{"id":"change-`+tasks[i]+`","head_sha":"head","state":"open"}`); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO autopilot_attempts(org_id,finding_id,finding_version,task_id,outcome,reason) VALUES($1,$2,1,$3,'retry','prior failed attempt')`, org, findings[0], oldTask)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var picked mergeCandidate
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		var err error
		picked, err = selectMergeCandidate(ctx, tx, org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if picked.task != tasks[0] || !picked.adopted || picked.repository != eligibleRepo {
		t.Fatalf("candidate %+v; want accessible, unpaused manual repair %s", picked, tasks[0])
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO autopilot_bot_merges(org_id,repository_id,change_id,head_sha,reason,merge_after) VALUES($1,$2,$3,'head','Adopted existing Reforge repair: CI failing',clock_timestamp()+interval '1 hour')`, org, eligibleRepo, "change-"+tasks[0])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := selectMergeCandidate(ctx, tx, org)
		if err != pgx.ErrNoRows {
			t.Fatalf("cooldown did not suppress adopted PR: %v", err)
		}
		var task string
		if err = tx.QueryRow(ctx, `SELECT task_id::text FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2 AND finding_version=1`, org, findings[0]).Scan(&task); err != nil {
			return err
		}
		if task != oldTask {
			t.Fatalf("adoption changed existing attempt task from %s to %s", oldTask, task)
		}
		if _, err = tx.Exec(ctx, `UPDATE autopilot_attempts SET task_id=$3,merge_after=NULL WHERE org_id=$1 AND finding_id=$2 AND finding_version=1`, org, findings[0], tasks[0]); err != nil {
			return err
		}
		picked, err = selectMergeCandidate(ctx, tx, org)
		if err != nil || picked.task != tasks[0] || picked.adopted {
			t.Fatalf("attempt cooldown NULL was overridden by per-PR cooldown: %+v %v", picked, err)
		}
		if _, err = tx.Exec(ctx, `UPDATE autopilot_attempts SET merge_after=clock_timestamp()+interval '1 hour' WHERE org_id=$1 AND finding_id=$2 AND finding_version=1`, org, findings[0]); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE autopilot_bot_merges SET merge_after=clock_timestamp()-interval '1 minute' WHERE org_id=$1 AND repository_id=$2 AND change_id=$3`, org, eligibleRepo, "change-"+tasks[0]); err != nil {
			return err
		}
		_, err = selectMergeCandidate(ctx, tx, org)
		if err != pgx.ErrNoRows {
			t.Fatalf("future attempt cooldown was ignored: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRepairIdempotencyKeyScopesManualRequests(t *testing.T) {
	candidate := candidate{finding: "finding", version: 1, runs: 0, requestGeneration: "20260927090000000000"}
	first := repairIdempotencyKey(candidate, strings.Repeat("a", 64))
	if retry := repairIdempotencyKey(candidate, strings.Repeat("a", 64)); retry != first {
		t.Fatalf("same request produced unstable key: %q != %q", retry, first)
	}
	candidate.requestGeneration = "20260927090100000000"
	if resubmission := repairIdempotencyKey(candidate, strings.Repeat("a", 64)); resubmission == first {
		t.Fatal("new manual request reused terminal task idempotency key")
	}
	candidate.requestGeneration = "20260927090000000000"
	candidate.version++
	if refreshed := repairIdempotencyKey(candidate, strings.Repeat("a", 64)); refreshed == first {
		t.Fatal("new finding version reused previous task idempotency key")
	}
}

func TestRebaseCooldownRetriesSameFindingAndCaps(t *testing.T) {
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
	org, user, repo, finding := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'autopilot-rebase-test',$1::text,'Owner',$2)`, user, user+"@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	evidence, _ := json.Marshal(map[string]any{"bot": "dependabot", "blockers": []string{discovery.WaitingForRebase}})
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Autopilot rebase')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name,accessible) VALUES($1,$2,$3,'team/repo',true)`, org, repo, domain.NewID()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'forge_change','change-1','ci_failure','medium','Dependabot PR',$5,$6)`, org, finding, repo, strings.Repeat("a", 64), evidence, strings.Repeat("b", 64)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO autopilot_attempts(org_id,finding_id,finding_version,outcome,reason,retry_after,runs) VALUES($1,$2,1,'rebase','Dependabot rebase: request accepted',clock_timestamp()+interval '1 hour',1)`, org, finding)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	load := func() []rebaseCandidate {
		t.Helper()
		var selected []rebaseCandidate
		if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
			var err error
			selected, err = loadRebaseCandidates(ctx, tx, org)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return selected
	}
	if got := load(); len(got) != 0 {
		t.Fatalf("cooling finding selected: %+v", got)
	}
	sibling, otherRepo, otherFinding := domain.NewID(), domain.NewID(), domain.NewID()
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name,accessible) VALUES($1,$2,$3,'team/other',true)`, org, otherRepo, domain.NewID()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'forge_change','change-2','ci_failure','medium','Sibling Dependabot PR',$5,$6),($1,$7,$8,$9,'forge_change','change-3','ci_failure','medium','Other Dependabot PR',$5,$10)`, org, sibling, repo, strings.Repeat("c", 64), evidence, strings.Repeat("d", 64), otherFinding, otherRepo, strings.Repeat("e", 64), strings.Repeat("f", 64)); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := load(); len(got) != 1 || got[0].finding.ID != otherFinding {
		t.Fatalf("repository cooldown did not serialize rebases: %+v", got)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE maintenance_findings SET state='resolved' WHERE org_id=$1 AND id=ANY($2::uuid[])`, org, []string{sibling, otherFinding})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE autopilot_attempts SET retry_after=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND finding_id=$2 AND finding_version=1`, org, finding)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got := load()
	if len(got) != 1 || got[0].finding.ID != finding || got[0].finding.Version != 1 || got[0].runs != 1 {
		t.Fatalf("unchanged finding did not become eligible after cooldown: %+v", got)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE autopilot_attempts SET runs=$3 WHERE org_id=$1 AND finding_id=$2 AND finding_version=1`, org, finding, maxRebases)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := &Service{db: db}
	if err = service.rebase(ctx, org); err != nil {
		t.Fatal(err)
	}
	if got := load(); len(got) != 0 {
		t.Fatalf("exhausted finding selected again: %+v", got)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		var outcome, reason string
		if err := tx.QueryRow(ctx, `SELECT outcome,reason FROM autopilot_attempts WHERE org_id=$1 AND finding_id=$2 AND finding_version=1`, org, finding).Scan(&outcome, &reason); err != nil {
			return err
		}
		if outcome != "skipped" || !strings.Contains(reason, "manually rebase or close") {
			t.Fatalf("retry cap not surfaced: outcome=%q reason=%q", outcome, reason)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEscalatedModelPrefersEligibleAlternativeAndFallsBackToPrior(t *testing.T) {
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
	org, user := domain.NewID(), domain.NewID()
	priorModel, alternateModel := domain.NewID(), domain.NewID()
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'autopilot-failover-test',$1::text,'Owner',$2)`, user, user+"@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Autopilot failover')`, org); err != nil {
			return err
		}
		for i, model := range []string{priorModel, alternateModel} {
			if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,reason) VALUES($1,$2,'model','test',$3,'https://model.invalid','{}','healthy','')`, org, model, "Model "+strconv.Itoa(i)); err != nil {
				return err
			}
			price := 1000000
			if i == 1 {
				price = 500000
			}
			config, _ := json.Marshal(map[string]any{"mode": "priced", "input_micro_usd_per_million": price})
			if _, err := tx.Exec(ctx, `INSERT INTO budget_routes(org_id,connection_id,model,name,config,version) VALUES($1,$2,$3,'default',$4,1)`, org, model, "model-"+strconv.Itoa(i), config); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO budget_limits(org_id,scope_kind,scope_id,period,period_start,period_end,caps,version) VALUES($1,'connection',$2,'daily',clock_timestamp()-interval '1 day',clock_timestamp()+interval '1 day','{"micro_usd":0}',1)`, org, alternateModel)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := &Service{budgets: budget.New(db, nil, nil, nil)}
	selectModel := func(want, why string) {
		t.Helper()
		if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
			model, route, _, err := service.pickModel(ctx, tx, org, true, priorModel)
			if err != nil {
				return err
			}
			if model != want {
				t.Fatalf("%s: selected model %s, want %s (route %s)", why, model, want, route)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	selectModel(priorModel, "only prior model has budget headroom")
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM budget_limits WHERE org_id=$1 AND scope_kind='connection' AND scope_id=$2`, org, alternateModel)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	selectModel(alternateModel, "eligible alternative should be preferred")
}

func TestTerminalRepairCleanupRequiresObservedMerge(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	if u, err := url.Parse(raw); err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	org, user := domain.NewID(), domain.NewID()
	repos := []string{domain.NewID(), domain.NewID(), domain.NewID()}
	findings := []string{domain.NewID(), domain.NewID(), domain.NewID()}
	tasks := []string{domain.NewID(), domain.NewID(), domain.NewID()}
	changes := []string{"merged-by-observation", "merge-requested", "merge-blocked"}
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'terminal-repair-test',$2,'Owner',$3)`, user, user, user+"@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Terminal repair cleanup')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, user); err != nil {
			return err
		}
		for i, repo := range repos {
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,$3,$4)`, org, repo, "native-"+repo, "repo-"+repo); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'native_ci',$5,'test_failure','high','Repair candidate','{}',$4)`, org, findings[i], repo, strings.Repeat("d", 64), "finding-"+findings[i]); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main',$6,$6,'completed',1,$7)`, org, tasks[i], repo, domain.NewID(), "terminal-repair-"+tasks[i], strings.Repeat("e", 64), user); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,native_change) VALUES($1,$2,$3,$4,$5,1,$6,'{}','published',$7)`, org, tasks[i], repo, findings[i], user, strings.Repeat("f", 64), `{"id":"`+changes[i]+`","head_sha":"head","state":"open"}`); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO maintenance_repairs(org_id,finding_id,repository_id,task_id,evidence_digest,active) VALUES($1,$2,$3,$4,$5,true)`, org, findings[i], repo, tasks[i], strings.Repeat("a", 64)); err != nil {
				return err
			}
			gate := domain.NewID()
			if _, err := tx.Exec(ctx, `INSERT INTO merge_gates(org_id,id,repository_id,change_id,configuration_version,document) VALUES($1,$2,$3,$4,1,'{}')`, org, gate, repo, changes[i]); err != nil {
				return err
			}
			state := []string{"merged", "requested", "blocked"}[i]
			if _, err := tx.Exec(ctx, `INSERT INTO merge_operations(org_id,id,repository_id,gate_id,requested_gate_id,change_id,target_branch,idempotency_key,requested_by,state) VALUES($1,$2,$3,$4,$4,$5,'main',$6,$7,$8)`, org, domain.NewID(), repo, gate, changes[i], "merge-"+changes[i], user, state); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		return releaseTerminalRepairsTx(ctx, tx, org)
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT m.task_id::text,m.active,rr.native_change->>'state' FROM maintenance_repairs m JOIN repair_runs rr ON rr.org_id=m.org_id AND rr.task_id=m.task_id WHERE m.org_id=$1`, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		got, states := map[string]bool{}, map[string]string{}
		for rows.Next() {
			var task, state string
			var active bool
			if err = rows.Scan(&task, &active, &state); err != nil {
				return err
			}
			got[task], states[task] = active, state
		}
		if err = rows.Err(); err != nil {
			return err
		}
		if got[tasks[0]] || !got[tasks[1]] || !got[tasks[2]] {
			t.Fatalf("active repair ownership after cleanup: merged=%t requested=%t blocked=%t", got[tasks[0]], got[tasks[1]], got[tasks[2]])
		}
		if states[tasks[0]] != "merged" || states[tasks[1]] != "open" || states[tasks[2]] != "open" {
			t.Fatalf("run change states after cleanup: %v", states)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
