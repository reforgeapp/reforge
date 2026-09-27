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

func TestOwnerRepairTargetCoordinationIsRepositoryAndTargetScoped(t *testing.T) {
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
	org, user, repo, otherRepo := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	finding, task := domain.NewID(), domain.NewID()
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'autopilot-merge-coordination',$2,'Owner',$3)`, user, user, user+"@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Autopilot merge coordination')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, user); err != nil {
			return err
		}
		for _, id := range []string{repo, otherRepo} {
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,$3,$4)`, org, id, "native-"+id, "repo-"+id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'native_ci','coordination','test_failure','high','Repair candidate',$5,$4)`, org, finding, repo, strings.Repeat("a", 64), `{"target_branch":"main"}`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main',$6,$6,'planning',1,$7)`, org, task, repo, domain.NewID(), "coordination-"+task, strings.Repeat("b", 64), user); err != nil {
			return err
		}
		context := `{"request":{"owner":true},"plan":{"owner":true},"follow_up_branch":"","finding":{"evidence":{"target_branch":"main"}}}`
		if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state) VALUES($1,$2,$3,$4,$5,1,$6,$7,'queued')`, org, task, repo, finding, user, strings.Repeat("c", 64), context); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO maintenance_repairs(org_id,finding_id,repository_id,task_id,evidence_digest,active) VALUES($1,$2,$3,$4,$5,true)`, org, finding, repo, task, strings.Repeat("c", 64))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := &Service{db: db}
	assert := func(repository, target string, want bool) {
		t.Helper()
		got, err := service.ownerRepairPinsTarget(ctx, org, repository, target)
		if err != nil || got != want {
			t.Fatalf("repo=%s target=%s active=%t err=%v; want active=%t", repository, target, got, err, want)
		}
	}
	assert(repo, "main", true)
	assert(otherRepo, "main", false)
	assert(repo, "release", false)
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE workflow_tasks SET state='queued' WHERE org_id=$1 AND id=$2`, org, task); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE repair_runs SET state='handoff' WHERE org_id=$1 AND task_id=$2`, org, task); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE maintenance_repairs SET active=false WHERE org_id=$1 AND task_id=$2`, org, task)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assert(repo, "main", true)
	for _, state := range []struct{ task, run string }{
		{task: "completed", run: "published"},
		{task: "completed", run: "queued"},
		{task: "failed", run: "queued"},
		{task: "cancelled", run: "queued"},
	} {
		if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE workflow_tasks SET state=$3 WHERE org_id=$1 AND id=$2`, org, task, state.task); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE repair_runs SET state=$3 WHERE org_id=$1 AND task_id=$2`, org, task, state.run)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		assert(repo, "main", false)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_tasks SET state='planning' WHERE org_id=$1 AND id=$2`, org, task)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE repair_runs SET state='queued',context=jsonb_set(context,'{follow_up_branch}','"feature/dep"') WHERE org_id=$1 AND task_id=$2`, org, task)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assert(repo, "main", false)
}
