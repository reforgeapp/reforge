package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/insights"
)

func TestOverviewMatchesAllPagedWorkKinds(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	repo := domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,'shop','shop')`, f.org, repo); err != nil {
			return err
		}
		for i := 0; i < 13; i++ {
			state := "queued"
			if i == 12 {
				state = "blocked"
			}
			hash := fmt.Sprintf("%064x", i+1)
			if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2::uuid,$3,$4,$2::text,$5,'repair','1','main',$5,$5,$6,1,$7)`, f.org, domain.NewID(), repo, domain.NewID(), hash, state, f.owner.User.ID); err != nil {
				return err
			}
		}
		for i, state := range []string{"queued", "blocked"} {
			gate, id := domain.NewID(), domain.NewID()
			change := fmt.Sprintf("change-%d", i)
			if _, err := tx.Exec(ctx, `INSERT INTO merge_gates(org_id,id,repository_id,change_id,configuration_version,document) VALUES($1,$2,$3,$4,1,'{}')`, f.org, gate, repo, change); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO merge_operations(org_id,id,repository_id,gate_id,requested_gate_id,change_id,target_branch,state,requested_by,idempotency_key) VALUES($1,$2::uuid,$3,$4,$4,$5,$5,$6,$7,$2::text)`, f.org, id, repo, gate, change, state, f.owner.User.ID); err != nil {
				return err
			}
		}
		for i, state := range []string{"verifying", "blocked"} {
			environment := fmt.Sprintf("native-%d", i)
			gate, id := domain.NewID(), domain.NewID()
			if _, err := tx.Exec(ctx, `INSERT INTO deployment_configurations(org_id,environment,repository_id,version,document) VALUES($1,$2,$3,1,'{}')`, f.org, environment, repo); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO deployment_gates(org_id,id,repository_id,environment,document) VALUES($1,$2,$3,$4,'{}')`, f.org, gate, repo, environment); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO deployments(org_id,id,repository_id,environment,gate_id,requested_by,idempotency_key,state) VALUES($1,$2::uuid,$3,$4,$5,$6,$2::text,$7)`, f.org, id, repo, environment, gate, f.owner.User.ID, state); err != nil {
				return err
			}
			environment = fmt.Sprintf("gitops-%d", i)
			gate, id = domain.NewID(), domain.NewID()
			if _, err := tx.Exec(ctx, `INSERT INTO gitops_configurations(org_id,environment,source_repository_id,delivery_repository_id,version,document) VALUES($1,$2,$3,$3,1,'{}')`, f.org, environment, repo); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO gitops_gates(org_id,id,environment,source_repository_id,delivery_repository_id,document) VALUES($1,$2,$3,$4,$4,'{}')`, f.org, gate, environment, repo); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO gitops_promotions(org_id,id,environment,source_repository_id,delivery_repository_id,gate_id,requested_by,idempotency_key,state,branch,target_branch,manifest_path,pointer) VALUES($1,$2::uuid,$3,$4,$4,$5,$6,$2::text,$7,'delivery','main',$3,'/image')`, f.org, id, environment, repo, gate, f.owner.User.ID, state); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := insights.New(f.identity)
	overview, err := service.Overview(ctx, f.owner, f.org)
	if err != nil || overview.Counts.Running != 15 || overview.Counts.Blocked != 4 || overview.Counts.NeedsDecision != 0 {
		t.Fatalf("count semantics: %+v %v", overview.Counts, err)
	}
	for bucket, want := range map[string]int{"active": 15, "blocked": 4} {
		seen := map[string]bool{}
		kinds := map[string]bool{}
		cursor := ""
		for page := 0; page < 10; page++ {
			result, err := service.Work(ctx, f.owner, f.org, insights.Filter{State: bucket, Limit: 3, Cursor: cursor})
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range result.Items {
				if seen[item.ID] || item.Bucket != bucket || item.RepositoryID != repo || item.URL == "" {
					t.Fatalf("mismatched work: %+v", item)
				}
				seen[item.ID] = true
				kinds[item.Kind] = true
			}
			if result.Complete {
				break
			}
			if result.NextCursor == "" {
				t.Fatal("work page lacks cursor")
			}
			cursor = result.NextCursor
		}
		if len(seen) != want || len(kinds) != 4 {
			t.Fatalf("work missing: %s %d items %v kinds", bucket, len(seen), kinds)
		}
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	overview, err = service.Overview(ctx, f.owner, f.org)
	if err != nil || overview.Counts.Running != 0 || overview.Counts.Blocked != 0 {
		t.Fatalf("work counts escaped scope: %+v %v", overview.Counts, err)
	}
	page, err := service.Work(ctx, f.owner, f.org, insights.Filter{State: "active", Limit: 3})
	if err != nil || len(page.Items) != 0 || !page.Complete {
		t.Fatalf("work list escaped scope: %+v %v", page, err)
	}
}
