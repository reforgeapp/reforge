package mergecontrol

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

func TestCompanionsExcludePublishedFollowupOnSameChange(t *testing.T) {
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
	org, user, repo := domain.NewID(), domain.NewID(), domain.NewID()
	tasks := []string{domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()}
	findings := []string{domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()}
	const targetChange = "target-change"
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'companion-test',$2,'Owner',$3)`, user, user, user+"@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Companion test')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, user); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,'companion-repo','companion/repo')`, org, repo); err != nil {
			return err
		}
		states := []string{"published", "published", "publishing", "published"}
		changes := []string{targetChange, "separate-change", targetChange, ""}
		for i := range tasks {
			if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'native_ci',$5,'test_failure','high','Companion repair','{}',$4)`, org, findings[i], repo, strings.Repeat(string(rune('a'+i)), 64), "finding-"+findings[i]); err != nil {
				return err
			}
			taskState := "completed"
			if states[i] == "publishing" {
				taskState = "publishing"
			}
			if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main',$6,$6,$7,1,$8)`, org, tasks[i], repo, domain.NewID(), "companion-"+tasks[i], "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", taskState, user); err != nil {
				return err
			}
			contextJSON := `{"finding":{"evidence":{"change":{"id":"` + targetChange + `"}}}}`
			nativeJSON := `{"id":"` + changes[i] + `","head_sha":"0123456789012345678901234567890123456789","state":"open"}`
			if i == 3 {
				nativeJSON = "{}"
			}
			if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,candidate_sha,native_change) VALUES($1,$2,$3,$4,$5,1,'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc',$6,$7,'0123456789012345678901234567890123456789',$8)`, org, tasks[i], repo, findings[i], user, contextJSON, states[i], nativeJSON); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		got, err := companionsTx(ctx, tx, org, repo, targetChange)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, item := range got {
			seen[item.TaskID] = true
		}
		if len(got) != 3 || seen[tasks[0]] || !seen[tasks[1]] || !seen[tasks[2]] || !seen[tasks[3]] {
			t.Fatalf("companions %+v; want separate published PR, unfinished same-PR repair and missing native identity", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
