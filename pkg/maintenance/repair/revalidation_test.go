package repair

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func TestRevalidationProofGuardsAndCompletion(t *testing.T) {
	ctx := context.Background()
	db := checkpointTestDB(t)
	org, repo, user := domain.NewID(), domain.NewID(), domain.NewID()
	parent, badParent, baselineParent := domain.NewID(), domain.NewID(), domain.NewID()
	goodTask, badTask, baselineTask := domain.NewID(), domain.NewID(), domain.NewID()
	finding := domain.NewID()
	parentDigest := strings.Repeat("1", 64)
	head, target := strings.Repeat("a", 40), strings.Repeat("b", 40)
	if err := db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'revalidation-test',$1::text,'Owner','revalidation@example.test')`, user)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	plan, _ := testPlan(t)
	plan.BaselineSHA, plan.TargetSHA = head, head
	plan.Digest = planDigest(plan)
	command := plan.Recipe.Commands[0]
	passing := []CheckResult{{CommandID: command.ID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestAdd": "pass"}}}
	skipped := []CheckResult{{CommandID: command.ID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestAdd": "skip"}}}
	insertRun := func(tx pgx.Tx, taskID, idempotency string, execution ExecutionContext, checks []CheckResult) error {
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'javascript','v1','main','policy','policy','completed',2,$7)`, org, taskID, repo, domain.NewID(), idempotency, strings.Repeat("c", 64), user); err != nil {
			return err
		}
		contextBody, err := json.Marshal(execution)
		if err != nil {
			return err
		}
		reportBody, err := json.Marshal(Report{State: "validated", PlanDigest: execution.Plan.Digest, Baseline: passing})
		if err != nil {
			return err
		}
		checkBody, err := json.Marshal(checks)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,report,state,candidate_sha,candidate_checks) VALUES($1,$2,$3,$4,$5,1,$6,$7,$8,'validated',$9,$10)`, org, taskID, repo, finding, user, strings.Repeat("d", 64), contextBody, reportBody, head, checkBody)
		return err
	}
	if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Revalidation test')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,$3,'revalidation/repo')`, org, repo, repo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'forge_change','revalidation','dependency','medium','revalidation','{}',$5)`, org, finding, repo, strings.Repeat("e", 64), strings.Repeat("f", 64)); err != nil {
			return err
		}
		good := ExecutionContext{ValidationTaskID: parent, ValidationTargetSHA: target, Request: Input{PlanDigest: parentDigest}, Plan: plan}
		if err := insertRun(tx, goodTask, "revalidation-good", good, passing); err != nil {
			return err
		}
		bad := good
		bad.ValidationTaskID = badParent
		badTaskPlan := plan
		badTaskPlan.BaselineSHA = strings.Repeat("9", 40)
		badTaskPlan.Digest = planDigest(badTaskPlan)
		bad.Plan = badTaskPlan
		if err := insertRun(tx, badTask, "revalidation-bad-baseline", bad, skipped); err != nil {
			return err
		}
		baseline := good
		baseline.ValidationTaskID = baselineParent
		baselineTaskPlan := plan
		baselineTaskPlan.BaselineSHA = strings.Repeat("8", 40)
		baselineTaskPlan.Digest = planDigest(baselineTaskPlan)
		baseline.Plan = baselineTaskPlan
		return insertRun(tx, baselineTask, "revalidation-wrong-baseline", baseline, passing)
	}); err != nil {
		t.Fatal(err)
	}
	var goodRef, badRef, baselineRef string
	if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		var err error
		goodRef, err = RevalidationReferenceTx(ctx, tx, org, repo, parent, parentDigest, head, target)
		if err != nil {
			return err
		}
		badRef, err = RevalidationReferenceTx(ctx, tx, org, repo, badParent, parentDigest, head, target)
		if err != nil {
			return err
		}
		baselineRef, err = RevalidationReferenceTx(ctx, tx, org, repo, baselineParent, parentDigest, head, target)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if goodRef != "repair-revalidation:"+goodTask || badRef != "" || baselineRef != "" {
		t.Fatalf("references good=%q bad=%q baseline=%q", goodRef, badRef, baselineRef)
	}
	for _, tc := range []struct {
		name, task, digest, candidate, target string
	}{
		{"different original task", domain.NewID(), parentDigest, head, target},
		{"different original plan", parent, strings.Repeat("2", 64), head, target},
		{"different head", parent, parentDigest, strings.Repeat("3", 40), target},
		{"different target", parent, parentDigest, head, strings.Repeat("4", 40)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
				ref, err := RevalidationReferenceTx(ctx, tx, org, repo, tc.task, tc.digest, tc.candidate, tc.target)
				if err == nil && ref != "" {
					t.Errorf("unexpected reference %q", ref)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
	service := &Service{}
	if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		task := workflow.Task{OrgID: org, ID: goodTask, State: domain.TaskValidating}
		if err := service.CheckCompletion(ctx, tx, workflow.Lease{OrgID: org, TaskID: goodTask}, task, workflow.Completion{Outcome: "completed"}); err != nil {
			return err
		}
		if err := service.CheckModelTx(ctx, tx, task, budget.Reservation{}); !errors.Is(err, workflow.ErrPolicy) {
			return errors.New("validation task was not denied model execution")
		}
		if err := service.CheckStage(ctx, tx, task, policy.Resolved{}); !errors.Is(err, workflow.ErrPolicy) {
			return errors.New("validation task was not denied staging")
		}
		if err := service.CheckPublish(ctx, tx, task, policy.Resolved{}); !errors.Is(err, workflow.ErrPolicy) {
			return errors.New("validation task was not denied publication")
		}
		badTaskRow := workflow.Task{OrgID: org, ID: badTask, State: domain.TaskValidating}
		returnErr := service.CheckCompletion(ctx, tx, workflow.Lease{OrgID: org, TaskID: badTask}, badTaskRow, workflow.Completion{Outcome: "completed"})
		if !errors.Is(returnErr, ErrValidation) {
			return errors.New("unverified native checks were accepted at completion")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
