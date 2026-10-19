package autopilot

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/store"
)

func TestBlockedMergeOperationAllowsFreshCandidateOnly(t *testing.T) {
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
	changes := []string{"blocked-change", "inflight-change", "merged-change"}
	states := []string{"blocked", "dispatching", "merged"}
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'autopilot-merge-retry-test',$2,'Owner',$3)`, user, user, user+"@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Autopilot merge retry')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, user); err != nil {
			return err
		}
		for i, repo := range repos {
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,$3,$4)`, org, repo, "native-"+repo, "repo-"+repo); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'native_ci',$5,'test_failure','high','Repair candidate','{}',$4)`, org, findings[i], repo, strings.Repeat(string(rune('a'+i)), 64), "finding-"+findings[i]); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main',$6,$6,'completed',1,$7)`, org, tasks[i], repo, domain.NewID(), "merge-retry-"+tasks[i], strings.Repeat("f", 64), user); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,candidate_sha,native_change) VALUES($1,$2,$3,$4,$5,1,$6,'{}','published','head',$7)`, org, tasks[i], repo, findings[i], user, strings.Repeat("e", 64), `{"id":"`+changes[i]+`","head_sha":"head","state":"open"}`); err != nil {
				return err
			}
			gate := domain.NewID()
			if _, err := tx.Exec(ctx, `INSERT INTO merge_gates(org_id,id,repository_id,change_id,configuration_version,document) VALUES($1,$2,$3,$4,1,'{}')`, org, gate, repo, changes[i]); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO merge_operations(org_id,id,repository_id,gate_id,requested_gate_id,change_id,target_branch,idempotency_key,requested_by,state) VALUES($1,$2,$3,$4,$4,$5,'main',$6,$7,$8)`, org, domain.NewID(), repo, gate, changes[i], "merge-retry-"+changes[i], user, states[i]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	selectCandidate := func() (mergeCandidate, error) {
		t.Helper()
		var got mergeCandidate
		err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
			var err error
			got, err = selectMergeCandidate(ctx, tx, org)
			return err
		})
		return got, err
	}
	got, err := selectCandidate()
	if err != nil || got.change != changes[0] || got.task != tasks[0] {
		t.Fatalf("candidate %+v, error %v; want blocked pre-dispatch operation retried", got, err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_repairs(org_id,finding_id,repository_id,task_id,evidence_digest,active,supersession) VALUES($1,$2,$3,$4,$5,false,'{}')`, org, findings[0], repos[0], tasks[0], strings.Repeat("d", 64)); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, err = selectCandidate(); err != pgx.ErrNoRows {
		t.Fatalf("superseded published repair was selected: %+v, error %v", got, err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE maintenance_repairs SET supersession=NULL WHERE org_id=$1 AND task_id=$2`, org, tasks[0]); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE merge_operations SET state='requested' WHERE org_id=$1 AND change_id=$2`, org, changes[0])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, err = selectCandidate(); err != pgx.ErrNoRows {
		t.Fatalf("in-flight and merged operations must remain excluded; got %+v, error %v", got, err)
	}
}
