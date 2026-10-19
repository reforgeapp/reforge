package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/maintenance/detectors"
	"github.com/reforgeapp/reforge/internal/store"
)

func TestPublishedRepairOverlapAllowsOnlyOwnedFollowUp(t *testing.T) {
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
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'repair-overlap-test',$2,'Owner',$3)`, user, user, user+"@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Repair overlap')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,$3,'repo')`, org, repo, "native-"+repo); err != nil {
			return err
		}
		insertRepair := func(finding, task, branch, state string, dep string) error {
			evidence, err := json.Marshal(Evidence{
				TargetBranch: "main",
				Dependencies: []detectors.Dependency{{Ecosystem: "npm", Manifest: "web/package.json", Name: dep}},
			})
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'forge_change',$5,'dependency','medium','Repair',$6,$7)`, org, finding, repo, strings.Repeat(task, 2)[:64], "change-"+task, evidence, strings.Repeat("a", 64)); err != nil {
				return err
			}
			taskState := "completed"
			if state == "queued" {
				taskState = "queued"
			}
			if _, err = tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main',$6,$6,$8,1,$7)`, org, task, repo, domain.NewID(), "overlap-"+task, strings.Repeat("b", 64), user, taskState); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,branch) VALUES($1,$2,$3,$4,$5,1,$6,'{}',$7,$8)`, org, task, repo, finding, user, strings.Repeat("c", 64), state, branch); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO maintenance_repairs(org_id,finding_id,repository_id,task_id,evidence_digest,active) VALUES($1,$2,$3,$4,$5,true)`, org, finding, repo, task, strings.Repeat("d", 64))
			return err
		}
		if err := insertRepair(domain.NewID(), domain.NewID(), "reforge/repair/published-a", "published", "typescript"); err != nil {
			return err
		}
		if err := insertRepair(domain.NewID(), domain.NewID(), "reforge/repair/published-b", "published", "js-yaml"); err != nil {
			return err
		}
		incoming := func(branch, ownership, dep string) Finding {
			return Finding{ID: domain.NewID(), OrgID: org, RepositoryID: repo, Evidence: Evidence{
				Ownership: ownership, TargetBranch: "main", Change: &forge.Change{HeadBranch: branch},
				Dependencies: []detectors.Dependency{{Ecosystem: "npm", Manifest: "web/package.json", Name: dep}},
			}}
		}
		followUp := incoming("reforge/repair/published-a", "reforge", "typescript")
		followUp.Evidence.Dependencies = append(followUp.Evidence.Dependencies, detectors.Dependency{Ecosystem: "npm", Manifest: "web/package.json", Name: "js-yaml"})
		if err := CheckRepairOverlapTx(ctx, tx, followUp); err != nil {
			return err
		}
		for _, candidate := range []Finding{
			incoming("dependabot/npm_and_yarn/web/typescript-6", "bot", "typescript"),
			incoming("reforge/repair/unknown-owner", "unknown", "typescript"),
			incoming("reforge/repair/unverified", "reforge", "typescript"),
			incoming("reforge/repair/published-a", "unknown", "js-yaml"),
		} {
			if err := CheckRepairOverlapTx(ctx, tx, candidate); err != ErrDuplicate {
				return fmt.Errorf("unverified work should remain blocked, got %v", err)
			}
		}
		if err := insertRepair(domain.NewID(), domain.NewID(), "", "queued", "typescript"); err != nil {
			return err
		}
		if err := CheckRepairOverlapTx(ctx, tx, followUp); err != ErrDuplicate {
			return fmt.Errorf("active queued repair should block follow-up, got %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
