package repair

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/runner"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/store"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func TestCheckpointShapeAllowsOwnerRetargetingProtectedFiles(t *testing.T) {
	p, _ := testPlan(t)
	p.Owner = true
	p.Digest = planDigest(p)
	checkpoint := Checkpoint{PlanDigest: p.Digest, Patches: []sandbox.Patch{{Path: "package.json", Content: []byte(`{"scripts":{"test":"node --test"}}`)}}}
	if !checkpoint.ValidFor(p) {
		t.Fatal("owner checkpoint rejected a protected manifest that owner validation may retarget")
	}
	checkpoint.Patches = []sandbox.Patch{{Path: "private/credentials.json", Content: []byte("{}")}}
	if checkpoint.ValidFor(p) {
		t.Fatal("checkpoint accepted a forbidden owner path")
	}
	checkpoint.Patches = []sandbox.Patch{{Path: ".env", Content: []byte(strings.Repeat("x", 4))}}
	if checkpoint.ValidFor(p) {
		t.Fatal("checkpoint accepted a secret file")
	}
}

func TestSaveCheckpointFencesPlanTaskAndRunnerLease(t *testing.T) {
	ctx := context.Background()
	db := checkpointTestDB(t)
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	session := auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}, CSRFToken: "checkpoint-test"}
	org, repo := domain.NewID(), domain.NewID()
	if err = db.Identity(ctx, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'checkpoint-test',$1::text,'Owner','checkpoint@example.test')`, session.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,clock_timestamp()+interval '1 hour')`, session.ID, session.User.ID, domain.NewID(), session.CSRFToken)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Checkpoint test')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, session.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'checkpoint/repo')`, org, repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	policyHash := strings.Repeat("a", 64)
	jobs := workflow.New(db, identity, func(_ context.Context, _ pgx.Tx, _ workflow.Task, _ string) (string, error) {
		return policyHash, nil
	})
	runners := runner.New(db, identity, jobs, nil)
	jobs.RegisterScopeCheck(runners.CheckScopeTx)
	supervisor, err := runners.EnrollBuiltin(ctx, org, "checkpoint-worker", 1)
	if err != nil {
		t.Fatal(err)
	}
	task, err := jobs.Enqueue(ctx, session, org, workflow.EnqueueInput{RepositoryID: repo, Recipe: "javascript", RecipeVersion: "v4", TargetBranch: "main", RunnerPoolID: supervisor.Runner.PoolID, PolicyHash: policyHash, IdempotencyKey: domain.NewID(), MaxAttempts: 2}, "checkpoint-test")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"value.js": []byte("exports.add=(a,b)=>a+b"), "value.test.js": []byte("const test=require('node:test'); test('adds',()=>{});"), "package.json": []byte(`{"scripts":{"test":"node --test"}}`)}
	plan, err := freeze("javascript", "sha256:"+strings.Repeat("b", 64), strings.Repeat("c", 40), strings.Repeat("d", 40), files, []string{"private/**"}, true)
	if err != nil {
		t.Fatal(err)
	}
	plan.Owner = true
	plan.MaxChangedLines = 1 << 20
	plan.Recipe.MaxFiles = 20
	plan.Recipe.MaxPatchBytes = 768 << 10
	plan.Recipe.MaxTurns = 200
	plan.Digest = planDigest(plan)
	findingID := domain.NewID()
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'forge_change','checkpoint','dependency','medium','checkpoint','{}',$5)`, org, findingID, repo, strings.Repeat("e", 64), strings.Repeat("f", 64)); err != nil {
			return err
		}
		body, _ := json.Marshal(ExecutionContext{Request: Input{Owner: true, Recipe: "javascript"}, Plan: plan, Repository: forge.RepoRef{NativeID: repo, FullName: "checkpoint/repo"}, PolicyHash: policyHash, Finding: discovery.Finding{RepositoryID: repo}})
		if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state) VALUES($1,$2,$3,$4,$5,1,$6,$7,'queued')`, org, task.ID, repo, findingID, session.User.ID, strings.Repeat("1", 64), body); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO maintenance_repairs(org_id,finding_id,repository_id,task_id,evidence_digest,active) VALUES($1,$2,$3,$4,$5,true)`, org, findingID, repo, task.ID, strings.Repeat("1", 64))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assignment, err := runners.Claim(ctx, supervisor.Token)
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Task.ID != task.ID {
		t.Fatalf("claimed task %s, want %s", assignment.Task.ID, task.ID)
	}
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_tasks SET state='planning' WHERE org_id=$1 AND id=$2`, org, task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := &Service{runners: runners}
	if _, err = service.SaveReport(ctx, assignment.Credential.Token, Report{PlanDigest: plan.Digest, State: "handoff", Mode: "owner"}); err != nil {
		t.Fatalf("save handoff before first checkpoint: %v", err)
	}
	var active bool
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT active FROM maintenance_repairs WHERE org_id=$1 AND task_id=$2`, org, task.ID).Scan(&active)
	}); err != nil || !active {
		t.Fatalf("checkpointless handoff lost repair ownership: active=%t error=%v", active, err)
	}
	checkpoint := Checkpoint{PlanDigest: plan.Digest, Patches: []sandbox.Patch{{Path: "value.js", Content: []byte("exports.add=(a,b)=>a-b")}}, Dependencies: []DependencyUpdate{{Ecosystem: "npm", Directory: ".", Package: "js-yaml", Version: "5.2.2"}}, Recent: "last completed batch", Turns: 7}
	if err = service.SaveCheckpoint(ctx, assignment.Credential.Token, checkpoint); err != nil {
		t.Fatalf("save valid checkpoint: %v", err)
	}
	var stored Run
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var err error
		stored, err = loadRun(ctx, tx, org, task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if stored.Checkpoint == nil || stored.Checkpoint.PlanDigest != checkpoint.PlanDigest || stored.Checkpoint.Turns != checkpoint.Turns || stored.Checkpoint.Recent != checkpoint.Recent || len(stored.Checkpoint.Patches) != 1 || len(stored.Checkpoint.Dependencies) != 1 {
		t.Fatalf("checkpoint did not persist: %+v", stored.Checkpoint)
	}
	if _, err = service.SaveReport(ctx, assignment.Credential.Token, Report{PlanDigest: plan.Digest, State: "handoff", Mode: "owner", Turns: checkpoint.Turns}); err != nil {
		t.Fatalf("save handoff with checkpoint: %v", err)
	}
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var err error
		stored, err = loadRun(ctx, tx, org, task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT active FROM maintenance_repairs WHERE org_id=$1 AND task_id=$2`, org, task.ID).Scan(&active)
	}); err != nil {
		t.Fatal(err)
	}
	if stored.Checkpoint == nil || stored.State != "handoff" || !active {
		t.Fatalf("handoff did not retain its resume checkpoint and repair ownership: state=%s checkpoint=%t active=%t", stored.State, stored.Checkpoint != nil, active)
	}
	stalePlan := checkpoint
	stalePlan.PlanDigest = strings.Repeat("9", 64)
	if err = service.SaveCheckpoint(ctx, assignment.Credential.Token, stalePlan); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("stale plan accepted: %v", err)
	}
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_tasks SET recipe='go' WHERE org_id=$1 AND id=$2`, org, task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = service.SaveCheckpoint(ctx, assignment.Credential.Token, checkpoint); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("task recipe edit accepted: %v", err)
	}
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_tasks SET recipe='javascript' WHERE org_id=$1 AND id=$2`, org, task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = runners.Complete(ctx, assignment.Credential.Token, workflow.Completion{Outcome: "failed", Retryable: true}); err != nil {
		t.Fatalf("schedule same-task retry: %v", err)
	}
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_jobs SET available_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, org, assignment.Lease.JobID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	retried, err := runners.Claim(ctx, supervisor.Token)
	if err != nil || retried.Task.ID != task.ID || retried.Lease.AttemptID == assignment.Lease.AttemptID {
		t.Fatalf("same task did not receive a fresh attempt: task=%s err=%v", retried.Task.ID, err)
	}
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var err error
		stored, err = loadRun(ctx, tx, org, task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if stored.Checkpoint == nil || stored.Checkpoint.Turns != checkpoint.Turns || stored.Checkpoint.Recent != checkpoint.Recent || len(stored.Checkpoint.Patches) != 1 || string(stored.Checkpoint.Patches[0].Content) != string(checkpoint.Patches[0].Content) {
		t.Fatal("retry discarded staged repair progress")
	}
	if err = service.SaveCheckpoint(ctx, assignment.Credential.Token, checkpoint); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("previous attempt credential accepted: %v", err)
	}
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_jobs SET fence=fence+1 WHERE org_id=$1 AND id=$2`, org, retried.Lease.JobID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = service.SaveCheckpoint(ctx, retried.Credential.Token, checkpoint); !errors.Is(err, workflow.ErrFence) {
		t.Fatalf("stale runner fence accepted: %v", err)
	}
}

func checkpointTestDB(t *testing.T) *store.Store {
	t.Helper()
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	db, err := store.Open(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db
}
