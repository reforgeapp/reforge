package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/store"
	"github.com/reforgeapp/reforge/internal/workflow"
)

type workflowFixture struct {
	db       *store.Store
	identity *auth.Service
	service  *workflow.Service
	owner    auth.Session
	org      string
	repos    []string
	policy   atomic.Value
}

func newWorkflowFixture(t *testing.T, count int) *workflowFixture {
	t.Helper()
	db := authDB(t)
	identity, server := identityServer(t, db, authConfig())
	_, owner := identityLogin(t, identity, server)
	f := &workflowFixture{db: db, identity: identity, owner: owner, org: domain.NewID()}
	f.policy.Store("policy-one")
	ctx := context.Background()
	err := db.Tenant(ctx, f.org, owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Workflow fixture')`, f.org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, f.org, owner.User.ID); err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			id := domain.NewID()
			f.repos = append(f.repos, id)
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Workflow repository')`, f.org, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.service = workflow.New(db, identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) {
		return f.policy.Load().(string), nil
	})
	return f
}
func (f *workflowFixture) enqueue(t *testing.T, repo string, max int) workflow.Task {
	t.Helper()
	task, err := f.service.Enqueue(context.Background(), f.owner, f.org, workflow.EnqueueInput{RepositoryID: repo, Recipe: "build-repair", RecipeVersion: "1", TargetBranch: "main", IdempotencyKey: domain.NewID(), MaxAttempts: max}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return task
}
func (f *workflowFixture) claim(t *testing.T) workflow.Lease {
	t.Helper()
	l, err := f.service.Claim(context.Background(), "worker-fixture", []string{f.org}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func expireWorkflowLease(t *testing.T, f *workflowFixture, l workflow.Lease) {
	t.Helper()
	if err := f.db.Tenant(context.Background(), f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE workflow_jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2`, f.org, l.JobID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowConcurrentClaimRestartFenceAndIdempotency(t *testing.T) {
	f := newWorkflowFixture(t, 1)
	ctx := context.Background()
	input := workflow.EnqueueInput{RepositoryID: f.repos[0], Recipe: "build-repair", RecipeVersion: "1", TargetBranch: "main", IdempotencyKey: domain.NewID(), MaxAttempts: 3}
	task, err := f.service.Enqueue(ctx, f.owner, f.org, input, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := f.service.Enqueue(ctx, f.owner, f.org, input, "fixture")
	if err != nil || duplicate.ID != task.ID {
		t.Fatal("duplicate task intent created")
	}
	if task.ModelRoute != "default" || duplicate.ModelRoute != "default" {
		t.Fatal("default model route was not persisted")
	}
	input.ModelRoute = "higher-cost"
	if _, err = f.service.Enqueue(ctx, f.owner, f.org, input, "fixture"); !errors.Is(err, auth.ErrConflict) {
		t.Fatal("idempotency allowed model route replacement")
	}
	input.ModelRoute = "default"
	if same, sameErr := f.service.Enqueue(ctx, f.owner, f.org, input, "fixture"); sameErr != nil || same.ID != task.ID {
		t.Fatal("explicit default changed normalized input identity")
	}
	input.RecipeVersion = "2"
	if _, err = f.service.Enqueue(ctx, f.owner, f.org, input, "fixture"); !errors.Is(err, auth.ErrConflict) {
		t.Fatal("idempotency conflict ignored")
	}
	results := make(chan workflow.Lease, 100)
	failures := make(chan error, 100)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l, err := f.service.Claim(ctx, fmt.Sprintf("worker-%d", i), []string{f.org}, time.Minute)
			if err == nil {
				results <- l
			} else if !errors.Is(err, workflow.ErrNoWork) {
				failures <- err
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if len(results) != 1 {
		t.Fatalf("100 competing claims produced %d leases", len(results))
	}
	first := <-results
	expireWorkflowLease(t, f, first)
	restarted := workflow.New(f.db, f.identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) {
		return f.policy.Load().(string), nil
	})
	if err = restarted.Recover(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	next, err := restarted.Claim(ctx, "replacement", []string{f.org}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if next.Fence <= first.Fence || next.AttemptID == first.AttemptID || next.OperationID != first.OperationID {
		t.Fatal("restart reused fence/attempt or changed logical operation")
	}
	if _, err = restarted.Heartbeat(ctx, first, time.Minute); !errors.Is(err, workflow.ErrFence) {
		t.Fatalf("stale heartbeat: %v", err)
	}
	if _, err = restarted.Complete(ctx, first, workflow.Completion{Outcome: "failed"}); !errors.Is(err, workflow.ErrFence) {
		t.Fatal("stale result accepted")
	}
	altered := next
	altered.RepositoryID = domain.NewID()
	if _, err = restarted.Advance(ctx, altered, domain.TaskPlanning); !errors.Is(err, workflow.ErrFence) {
		t.Fatal("forged repository binding accepted")
	}
	if _, err = restarted.Heartbeat(ctx, next, time.Minute); err != nil {
		t.Fatal(err)
	}
	f.policy.Store("policy-two")
	if _, err = restarted.Advance(ctx, next, domain.TaskPlanning); !errors.Is(err, workflow.ErrPolicy) {
		t.Fatalf("changed policy accepted: %v", err)
	}
	blocked, err := restarted.Get(ctx, f.owner, f.org, task.ID)
	if err != nil || blocked.State != domain.TaskBlocked {
		t.Fatalf("policy block not durable: %+v %v", blocked, err)
	}
	resumed, err := restarted.Resume(ctx, f.owner, f.org, task.ID, blocked.Version, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.PolicyHash != "policy-two" || resumed.StartingPolicyHash != "policy-one" || resumed.ModelRoute != "default" {
		t.Fatal("resume lost policy history")
	}
	var auditCount int
	if err = f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE org_id=$1 AND actor_id=$2 AND object_id=$3 AND request_id='fixture'`, f.org, f.owner.User.ID, task.ID).Scan(&auditCount)
	}); err != nil || auditCount < 2 {
		t.Fatalf("workflow user actions lack audit: %d %v", auditCount, err)
	}

	resumedLease, err := restarted.Claim(ctx, "replacement", []string{f.org}, time.Minute)
	if err != nil || resumedLease.AttemptID == next.AttemptID {
		t.Fatalf("resume did not create new attempt: %v", err)
	}
}

func TestWorkflowOutboxRecoveryRetryAndCancellation(t *testing.T) {
	f := newWorkflowFixture(t, 1)
	ctx := context.Background()
	task := f.enqueue(t, f.repos[0], 3)
	lease := f.claim(t)
	var intent workflow.Intent
	prepare := func(payload string) error {
		return f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
			var err error
			intent, err = f.service.PrepareIntentTx(ctx, tx, lease, "publish", "publish-main", json.RawMessage(payload))
			return err
		})
	}
	if err := prepare(`{"head":"fixture-sha","native_id":9007199254740993}`); err != nil {
		t.Fatal(err)
	}
	operation := intent.OperationID
	if !bytes.Contains(intent.Payload, []byte("9007199254740993")) {
		t.Fatal("outbox canonicalization changed provider identifier")
	}
	if _, err := f.service.SetPause(ctx, f.owner, f.org, workflow.Pause{Kind: "model", ID: domain.NewID(), Paused: true}, 0, "fixture"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("cross-tenant or missing model scope accepted")
	}
	if _, err := f.service.SetPause(ctx, f.owner, f.org, workflow.Pause{Kind: "runner_pool", ID: domain.NewID(), Paused: true}, 0, "fixture"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatal("unenrolled runner pool scope accepted")
	}

	if err := prepare(`{"head":"fixture-sha","native_id":9007199254740993}`); err != nil || intent.OperationID != operation {
		t.Fatal("outbox intent duplicated")
	}
	if err := prepare(`{"head":"different"}`); !errors.Is(err, auth.ErrConflict) {
		t.Fatal("outbox idempotency changed payload")
	}
	if err := prepare(`{"head":"fixture-sha","native_id":9007199254740993}`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.BeginIntent(ctx, lease, intent.ID); err != nil {
		t.Fatal(err)
	}
	expireWorkflowLease(t, f, lease)
	if err := f.service.Recover(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	recovered, err := f.service.Get(ctx, f.owner, f.org, task.ID)
	if err != nil || recovered.State != domain.TaskReconciling {
		t.Fatalf("uncertain dispatch retried: %+v %v", recovered, err)
	}

	f.enqueue(t, f.repos[0], 1)
	if _, claimErr := f.service.Claim(ctx, "competing-writer", []string{f.org}, time.Minute); !errors.Is(claimErr, workflow.ErrNoWork) {
		t.Fatalf("uncertain writer lost repository serialization: %v", claimErr)
	}
	if _, err = f.service.Resume(ctx, f.owner, f.org, task.ID, recovered.Version, "fixture"); !errors.Is(err, workflow.ErrReconciliation) {
		t.Fatal("unknown external outcome resumed")
	}
	cancelled, err := f.service.Cancel(ctx, f.owner, f.org, task.ID, recovered.Version, "fixture")
	if err != nil || cancelled.State != domain.TaskReconciling {
		t.Fatalf("unknown effect falsely cancelled: %+v %v", cancelled, err)
	}
	if _, err = f.service.ReconcileIntent(ctx, f.org, intent.ID, "absent", "provider operation lookup proved absent"); err != nil {
		t.Fatal(err)
	}
	resumed, err := f.service.Resume(ctx, f.owner, f.org, task.ID, cancelled.Version, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	_ = resumed
	lease = f.claim(t)
	second, err := f.service.BeginIntent(ctx, lease, intent.ID)
	if err != nil || second.OperationID != operation || second.DispatchCount != 2 {
		t.Fatalf("reconciliation lost stable operation: %+v %v", second, err)
	}
	if _, err = f.service.CompleteIntent(ctx, lease, intent.ID, "succeeded", "provider request fixture-123"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.BeginIntent(ctx, lease, intent.ID); !errors.Is(err, workflow.ErrReconciliation) {
		t.Fatal("successful operation redispatched")
	}
	current, err := f.service.Get(ctx, f.owner, f.org, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	stopping, err := f.service.Cancel(ctx, f.owner, f.org, task.ID, current.Version, "fixture")
	if err != nil || stopping.State != domain.TaskCancelling {
		t.Fatal("active cancellation claimed stopped")
	}
	if _, err = f.service.Advance(ctx, lease, domain.TaskPlanning); !errors.Is(err, workflow.ErrPaused) {
		t.Fatal("cancelled worker can mutate")
	}
	stopped, err := f.service.Complete(ctx, lease, workflow.Completion{Outcome: "cancelled"})
	if err != nil || stopped.State != domain.TaskCancelled {
		t.Fatalf("confirmed stop failed: %+v %v", stopped, err)
	}
}

func TestWorkflowPauseFairnessAndRetryBound(t *testing.T) {
	f := newWorkflowFixture(t, 2)
	ctx := context.Background()
	f.enqueue(t, f.repos[0], 2)
	f.enqueue(t, f.repos[1], 2)
	pause, err := f.service.SetPause(ctx, f.owner, f.org, workflow.Pause{Kind: "repository", ID: f.repos[0], Paused: true}, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	lease := f.claim(t)
	if lease.RepositoryID != f.repos[1] {
		t.Fatal("paused repository dispatched")
	}
	if _, err = f.service.Complete(ctx, lease, workflow.Completion{Outcome: "failed", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_jobs SET available_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, f.org, lease.JobID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	retry := f.claim(t)
	if retry.Fence <= lease.Fence {
		t.Fatal("retry fence not advanced")
	}
	exhausted, err := f.service.Complete(ctx, retry, workflow.Completion{Outcome: "failed", Retryable: true})
	if err != nil || exhausted.State != domain.TaskFailed {
		t.Fatal("attempt ceiling ignored")
	}
	if _, err = f.service.Resume(ctx, f.owner, f.org, exhausted.ID, exhausted.Version, "fixture"); !errors.Is(err, auth.ErrConflict) {
		t.Fatal("resume bypassed attempt ceiling")
	}
	if _, err = f.service.SetPause(ctx, f.owner, f.org, workflow.Pause{Kind: "repository", ID: f.repos[0], Paused: false}, pause.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	if resumed := f.claim(t); resumed.RepositoryID != f.repos[0] {
		t.Fatal("healthy repository did not resume")
	}
	other := newWorkflowFixture(t, 1)
	f.enqueue(t, f.repos[1], 1)
	other.enqueue(t, other.repos[0], 1)
	first, err := f.service.Claim(ctx, "fair-worker", []string{f.org, other.org}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Claim(ctx, "fair-worker", []string{f.org, other.org}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if first.OrgID == second.OrgID {
		t.Fatal("organisation queue starved")
	}
}

func TestWorkflowReplayScopesAndSessionRevocation(t *testing.T) {
	f := newWorkflowFixture(t, 2)
	ctx := context.Background()
	userID, cookie := fixtureIdentity(t, f.db)
	member, err := f.identity.PutMember(ctx, f.owner, f.org, userID, auth.Membership{Role: domain.Viewer, RepositoryIDs: []string{f.repos[0]}}, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	session, err := f.identity.Authenticate(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	first := f.enqueue(t, f.repos[0], 1)
	f.enqueue(t, f.repos[1], 1)
	if _, err = f.service.SetPause(ctx, f.owner, f.org, workflow.Pause{Kind: "recipe", ID: "unrelated", Paused: true}, 0, "fixture"); err != nil {
		t.Fatal(err)
	}
	page, err := f.service.Replay(ctx, session, f.org, 0, 200)
	if err != nil || len(page.Items) != 1 || page.Items[0].RepositoryID != f.repos[0] {
		t.Fatalf("event scope leak: %+v %v", page, err)
	}
	if page.ScanAfter <= page.Cursor {
		t.Fatal("hidden events did not advance internal scan")
	}
	wire, _ := json.Marshal(page)
	if stringsContainsJSONField(wire, "ScanAfter") || stringsContainsJSONField(wire, "scan_after") {
		t.Fatal("hidden cursor exposed")
	}
	if _, err = f.service.Get(ctx, session, f.org, first.ID); err != nil {
		t.Fatal(err)
	}
	all, err := f.service.Replay(ctx, f.owner, f.org, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(all.Items); i++ {
		if all.Items[i].ID <= all.Items[i-1].ID {
			t.Fatal("event order violated")
		}
	}

	other := newWorkflowFixture(t, 1)
	other.enqueue(t, other.repos[0], 1)
	if _, err = f.service.Replay(ctx, session, other.org, 0, 200); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("cross-tenant replay accepted")
	}
	if err = f.service.PruneEvents(ctx, f.org, page.Cursor); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Replay(ctx, f.owner, f.org, 0, 200); !errors.Is(err, workflow.ErrCursor) {
		t.Fatal("expired cursor silently replayed incomplete history")
	}
	if err = f.identity.DeleteMember(ctx, f.owner, f.org, userID, member.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Replay(ctx, session, f.org, page.ScanAfter, 200); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("SSE session kept revoked organisation membership")
	}
	if err = f.identity.Logout(ctx, f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Replay(ctx, f.owner, f.org, 0, 200); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("SSE session kept revoked login")
	}
	var count int
	for _, table := range []string{"workflow_tasks", "workflow_jobs", "workflow_attempts", "workflow_events", "workflow_outbox", "workflow_scheduler"} {
		if err = f.db.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unscoped %s leaked: %d %v", table, count, err)
		}
	}
}
func stringsContainsJSONField(raw []byte, key string) bool {
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	_, ok := fields[key]
	return ok
}

func TestWorkflowPauseCannotReviveOldLease(t *testing.T) {
	f := newWorkflowFixture(t, 1)
	ctx := context.Background()
	task := f.enqueue(t, f.repos[0], 3)
	lease := f.claim(t)
	pause, err := f.service.SetPause(ctx, f.owner, f.org, workflow.Pause{Kind: "recipe", ID: "build-repair", Paused: true}, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.SetPause(ctx, f.owner, f.org, workflow.Pause{Kind: "recipe", ID: "build-repair", Paused: false}, pause.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Advance(ctx, lease, domain.TaskPlanning); !errors.Is(err, workflow.ErrFence) {
		t.Fatalf("unpause revived stale lease: %v", err)
	}
	blocked, err := f.service.Get(ctx, f.owner, f.org, task.ID)
	if err != nil || blocked.State != domain.TaskBlocked {
		t.Fatalf("pause failed to persist revoked lease: %+v %v", blocked, err)
	}
	if _, err = f.service.Resume(ctx, f.owner, f.org, task.ID, blocked.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	renewed := f.claim(t)
	if renewed.AttemptID == lease.AttemptID || renewed.Fence <= lease.Fence {
		t.Fatal("pause resume reused attempt or fence")
	}
}

func TestWorkflowRecoveryBatchRetainsWriterExclusion(t *testing.T) {
	f := newWorkflowFixture(t, 1)
	ctx := context.Background()
	var target string
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) SELECT $1,gen_random_uuid(),$2,gen_random_uuid(),'batch-'||n,'fixture','build-repair','1','batch-'||n,'policy-one','policy-one','publishing',3,$3 FROM generate_series(1,201) n`, f.org, f.repos[0], f.owner.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_jobs(org_id,id,task_id,operation_id,state,fence,attempts,lease_owner,lease_expires_at) SELECT org_id,gen_random_uuid(),id,operation_id,'running',1,1,'expired-fixture',clock_timestamp()-interval '1 second' FROM workflow_tasks WHERE org_id=$1`, f.org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_attempts(org_id,id,task_id,job_id,number,fence,lease_owner,state) SELECT org_id,gen_random_uuid(),task_id,id,1,1,lease_owner,'running' FROM workflow_jobs WHERE org_id=$1`, f.org); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT t.target_branch FROM workflow_jobs j JOIN workflow_tasks t ON t.org_id=j.org_id AND t.id=j.task_id WHERE j.org_id=$1 ORDER BY j.id DESC LIMIT 1`, f.org).Scan(&target)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Enqueue(ctx, f.owner, f.org, workflow.EnqueueInput{RepositoryID: f.repos[0], Recipe: "build-repair", RecipeVersion: "1", TargetBranch: target, IdempotencyKey: domain.NewID()}, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Claim(ctx, "new-writer", []string{f.org}, time.Minute); !errors.Is(err, workflow.ErrNoWork) {
		t.Fatalf("unrecovered expired writer bypassed: %v", err)
	}
	var running, reconciling int
	count := func() error {
		return f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='running'),count(*) FILTER(WHERE state='reconciling') FROM workflow_jobs WHERE org_id=$1`, f.org).Scan(&running, &reconciling)
		})
	}
	if err := count(); err != nil || running != 1 || reconciling != 200 {
		t.Fatalf("recovery batch was not bounded: running=%d reconciling=%d err=%v", running, reconciling, err)
	}
	if err := f.service.Recover(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	if err := count(); err != nil || running != 0 || reconciling != 201 {
		t.Fatalf("remaining recovery failed: running=%d reconciling=%d err=%v", running, reconciling, err)
	}
}
