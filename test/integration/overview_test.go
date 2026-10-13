package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/insights"
)

func TestOverviewCountsOnlyOpenReviewCandidates(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	repo, job := domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,connection_id,native_id,name) VALUES($1,$2,$3,'native','repo')`, f.org, repo, f.connection.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO inventory_jobs(org_id,id,connection_id,connection_version,kind,repository_id,requested_by,state) VALUES($1,$2,$3,$4,'refresh',$5,$6,'complete')`, f.org, job, f.connection.ID, f.connection.Version, repo, f.owner.User.ID); err != nil {
			return err
		}
		for i, state := range []string{"open", "closed", "merged", "open", "open"} {
			task, finding, change := domain.NewID(), domain.NewID(), fmt.Sprintf("change-%d", i)
			hash := fmt.Sprintf("%064x", i+1)
			if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'repository_review',$5,'code_defect','high','Repair candidate','{}',$4)`, f.org, finding, repo, hash, change); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2::uuid,$3,$4,$2::text,$5,'repair','1','main',$5,$5,'completed',1,$6)`, f.org, task, repo, domain.NewID(), hash, f.owner.User.ID); err != nil {
				return err
			}
			native, err := json.Marshal(map[string]string{"id": change, "state": state})
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,native_change) VALUES($1,$2,$3,$4,$5,1,$6,'{}','published',$7)`, f.org, task, repo, finding, f.owner.User.ID, hash, native); err != nil {
				return err
			}
			if i == 3 {
				snapshot, err := json.Marshal(map[string]string{"id": change, "state": "closed"})
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO inventory_changes(org_id,repository_id,native_id,snapshot,job_id) VALUES($1,$2,$3,$4,$5)`, f.org, repo, change, snapshot, job); err != nil {
					return err
				}
			}
			if i == 4 {
				gate := domain.NewID()
				if _, err := tx.Exec(ctx, `INSERT INTO merge_gates(org_id,id,repository_id,change_id,configuration_version,document) VALUES($1,$2,$3,$4,1,'{}')`, f.org, gate, repo, change); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO merge_operations(org_id,id,repository_id,gate_id,requested_gate_id,change_id,target_branch,state,requested_by,idempotency_key) VALUES($1,$2::uuid,$3,$4,$4,$5,'main','merged',$6,$2::text)`, f.org, domain.NewID(), repo, gate, change, f.owner.User.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := insights.New(f.identity)
	value, err := service.Overview(ctx, f.owner, f.org)
	if err != nil || value.Counts.ReadyForReview != 1 {
		t.Fatalf("finished changes counted as ready for review: %+v %v", value.Counts, err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	value, err = service.Overview(ctx, f.owner, f.org)
	if err != nil || value.Counts.ReadyForReview != 0 {
		t.Fatalf("review count leaked inaccessible repository: %+v %v", value.Counts, err)
	}
}
