package repair

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/store"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func TestRepositoryReviewPreservesOmittedFindings(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	if u, err := url.Parse(raw); err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable PostgreSQL reforge_test")
	}
	for _, count := range []int{0, maxReviewFindings} {
		t.Run(fmt.Sprintf("reported=%d", count), func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(ctx, raw)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(db.Close)
			org, user, repo, task, connection := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
			if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'review-retention-test',$2,'Owner',$3)`, user, user, user+"@example.test")
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Review retention')`, org); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,'native','repo')`, org, repo); err != nil {
					return err
				}
				in := discovery.Observation{RepositoryID: repo, Source: "repository_review", SourceID: "review:original", Category: "code_defect", Severity: "high", Title: "Reproduced checkout defect", Evidence: discovery.Evidence{ConnectionID: connection, ConnectionVersion: 1, ConfigVersion: 1, HeadSHA: strings.Repeat("b", 40), TargetSHA: strings.Repeat("b", 40), TargetBranch: "main", Complete: true, Review: &discovery.ReviewEvidence{Path: "checkout.js", Line: 12, Objective: "Repair checkout"}}}
				original, err := discovery.ObserveTx(ctx, tx, org, in, user, task)
				if err != nil {
					return err
				}
				in.Source, in.SourceID, in.Category = "repository", "repository-review", "repository_review"
				in.Evidence.HeadSHA, in.Evidence.TargetSHA = strings.Repeat("a", 40), strings.Repeat("a", 40)
				in.Evidence.Review = nil
				trigger, err := discovery.ObserveTx(ctx, tx, org, in, user, task)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2::uuid,$3,$4,$2::text,$5,'review','1','main',$5,$5,'completed',1,$6)`, org, task, repo, domain.NewID(), strings.Repeat("c", 64), user); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state) VALUES($1,$2,$3,$4,$5,$6,$7,'{}','handoff')`, org, task, repo, trigger.ID, user, trigger.Version, trigger.EvidenceDigest); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO maintenance_repairs(org_id,finding_id,repository_id,task_id,evidence_digest) VALUES($1,$2,$3,$4,$5)`, org, trigger.ID, repo, task, trigger.EvidenceDigest); err != nil {
					return err
				}
				report := Report{Reason: "Bounded review finished"}
				for i := 0; i < count; i++ {
					report.Findings = append(report.Findings, ReviewFinding{Category: "code_defect", Severity: "high", Title: fmt.Sprintf("Another verified defect %d", i), Path: "other.js", Confidence: "high", Objective: "Repair verified defect"})
				}
				service := &Service{}
				if err := service.recordReview(ctx, tx, workflow.Lease{OrgID: org, RepositoryID: repo, TaskID: task}, Run{Context: ExecutionContext{Finding: trigger}}, report); err != nil {
					return err
				}
				var state, digest, head string
				var version int64
				if err := tx.QueryRow(ctx, `SELECT state,evidence_digest,evidence->>'head_sha',version FROM maintenance_findings WHERE org_id=$1 AND id=$2`, org, original.ID).Scan(&state, &digest, &head, &version); err != nil {
					return err
				}
				if state != "open" || version != original.Version || digest != original.EvidenceDigest || head != original.Evidence.HeadSHA {
					t.Fatalf("bounded review changed omitted finding: %s/%d/%s/%s", state, version, digest, head)
				}
				var active bool
				if err := tx.QueryRow(ctx, `SELECT f.state,m.active FROM maintenance_findings f JOIN maintenance_repairs m ON m.org_id=f.org_id AND m.finding_id=f.id WHERE f.org_id=$1 AND f.id=$2`, org, trigger.ID).Scan(&state, &active); err != nil {
					return err
				}
				if state != "resolved" || active {
					t.Fatalf("review trigger did not complete: state=%s active=%v", state, active)
				}
				var reported int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM maintenance_findings WHERE org_id=$1 AND source='repository_review' AND id<>$2 AND state='open'`, org, original.ID).Scan(&reported); err != nil {
					return err
				}
				if reported != count {
					t.Fatalf("reported findings=%d, want %d", reported, count)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
