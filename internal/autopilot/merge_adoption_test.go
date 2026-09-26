package autopilot

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"reforge/internal/domain"
	"reforge/internal/store"
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
