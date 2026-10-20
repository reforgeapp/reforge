package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/artifact"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/runner"
	"github.com/reforgeapp/reforge/pkg/store"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

type recoveryProcess struct {
	command *exec.Cmd
	waited  bool
}

func startRecoveryProcess(t *testing.T, command *exec.Cmd) *recoveryProcess {
	t.Helper()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	process := &recoveryProcess{command: command}
	t.Cleanup(func() {
		if !process.waited {
			process.killAndWait()
		}
	})
	return process
}

func (p *recoveryProcess) wait() error {
	if p.waited {
		return errors.New("child process already waited")
	}
	p.waited = true
	return p.command.Wait()
}

func (p *recoveryProcess) killAndWait() {
	if p.waited {
		return
	}
	if p.command.Process != nil {
		_ = p.command.Process.Kill()
	}
	_ = p.wait()
}

func TestActiveJobLeaseHolderProcess(t *testing.T) {
	if os.Getenv("REFORGE_ACTIVE_JOB_HELPER") != "1" {
		t.Skip("subprocess fixture")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, os.Getenv("REFORGE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := workflow.New(db, nil, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) { return "policy-one", nil })
	lease, err := service.Claim(ctx, os.Getenv("REFORGE_ACTIVE_JOB_WORKER"), []string{os.Getenv("REFORGE_ACTIVE_JOB_ORG")}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(lease); err != nil {
		t.Fatal(err)
	}
	select {}
}

func TestRunnerLeaseHolderProcess(t *testing.T) {
	if os.Getenv("REFORGE_RUNNER_PROCESS_HELPER") != "1" {
		t.Skip("subprocess fixture")
	}
	var input struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db, err := store.Open(ctx, os.Getenv("REFORGE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	jobs := workflow.New(db, identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) { return "policy-one", nil })
	runners := runner.New(db, identity, jobs, (*artifact.Local)(nil))
	jobs.RegisterScopeCheck(runners.CheckScopeTx)
	assignment, err := runners.Claim(ctx, input.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(struct {
		Assignment runner.Assignment `json:"assignment"`
		Credential string            `json:"credential"`
	}{assignment, assignment.Credential.Token}); err != nil {
		t.Fatal(err)
	}
	select {}
}

func TestRunnerLeaseRecoveryAfterSupervisorProcessKill(t *testing.T) {
	f := newWorkflowFixture(t, 1)
	ctx := context.Background()
	runners := runner.New(f.db, f.identity, f.service, (*artifact.Local)(nil))
	f.service.RegisterScopeCheck(runners.CheckScopeTx)
	pool, err := runners.PutPool(ctx, f.owner, f.org, "", runner.PoolInput{Name: "Recovery fixture", RepositoryIDs: []string{f.repos[0]}}, 0, "process-recovery")
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := runners.EnrollToken(ctx, f.owner, f.org, pool.ID, "process-recovery")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := runners.Enroll(ctx, enrollment.Token, "interrupted-supervisor")
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.service.Enqueue(ctx, f.owner, f.org, workflow.EnqueueInput{RepositoryID: f.repos[0], RunnerPoolID: pool.ID, Recipe: "build-repair", RecipeVersion: "1", TargetBranch: "main", IdempotencyKey: domain.NewID(), MaxAttempts: 3}, "process-recovery")
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	processCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(processCtx, executable, "-test.run=^TestRunnerLeaseHolderProcess$", "-test.count=1")
	command.Env = []string{"PATH=/usr/bin:/bin", "REFORGE_RUNNER_PROCESS_HELPER=1", "REFORGE_TEST_DATABASE_URL=" + os.Getenv("REFORGE_TEST_DATABASE_URL")}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	process := startRecoveryProcess(t, command)
	if err = json.NewEncoder(stdin).Encode(map[string]string{"token": supervisor.Token}); err != nil {
		process.killAndWait()
		t.Fatal(err)
	}
	if err = stdin.Close(); err != nil {
		process.killAndWait()
		t.Fatal(err)
	}
	var result struct {
		Assignment runner.Assignment `json:"assignment"`
		Credential string            `json:"credential"`
	}
	if err = json.NewDecoder(bufio.NewReader(stdout)).Decode(&result); err != nil {
		process.killAndWait()
		t.Fatalf("supervisor process did not claim job: %v", err)
	}
	if result.Assignment.Task.ID != task.ID || result.Credential == "" {
		process.killAndWait()
		t.Fatal("supervisor did not return assigned task and credential")
	}
	if err = command.Process.Kill(); err != nil {
		process.killAndWait()
		t.Fatal(err)
	}
	if err = process.wait(); err == nil {
		t.Fatal("supervisor process survived forced termination")
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2`, f.org, result.Assignment.Lease.JobID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	restartedJobs := workflow.New(f.db, f.identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) { return "policy-one", nil })
	restartedRunners := runner.New(f.db, f.identity, restartedJobs, (*artifact.Local)(nil))
	restartedJobs.RegisterScopeCheck(restartedRunners.CheckScopeTx)
	if err = restartedJobs.Recover(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	replacement, err := restartedRunners.Claim(ctx, supervisor.Token)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Lease.JobID != result.Assignment.Lease.JobID || replacement.Lease.OperationID != result.Assignment.Lease.OperationID || replacement.Lease.Fence <= result.Assignment.Lease.Fence || replacement.Lease.AttemptID == result.Assignment.Lease.AttemptID {
		t.Fatalf("runner reassignment mismatch: old=%+v new=%+v", result.Assignment.Lease, replacement.Lease)
	}
	if _, err = runners.Heartbeat(ctx, result.Credential); !errors.Is(err, workflow.ErrFence) {
		t.Fatalf("killed supervisor credential retained authority: %v", err)
	}
	if _, err = restartedRunners.Heartbeat(ctx, replacement.Credential.Token); err != nil {
		t.Fatalf("replacement runner could not heartbeat: %v", err)
	}
}

func TestActiveJobRecoveryAfterWorkerProcessKill(t *testing.T) {
	f := newWorkflowFixture(t, 1)
	f.enqueue(t, f.repos[0], 3)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	processCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(processCtx, executable, "-test.run=^TestActiveJobLeaseHolderProcess$", "-test.count=1")
	command.Env = []string{
		"PATH=/usr/bin:/bin",
		"REFORGE_ACTIVE_JOB_HELPER=1",
		"REFORGE_ACTIVE_JOB_ORG=" + f.org,
		"REFORGE_ACTIVE_JOB_WORKER=interrupted-worker",
		"REFORGE_TEST_DATABASE_URL=" + os.Getenv("REFORGE_TEST_DATABASE_URL"),
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	process := startRecoveryProcess(t, command)
	reader := bufio.NewReader(stdout)
	var interrupted workflow.Lease
	var output strings.Builder
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			if decodeErr := json.Unmarshal([]byte(line), &interrupted); decodeErr == nil {
				break
			}
			output.WriteString(line)
		}
		if readErr != nil {
			process.wait()
			t.Fatalf("worker exited before publishing active lease: %v; output=%q", readErr, strings.TrimSpace(output.String()))
		}
	}
	if interrupted.WorkerID != "interrupted-worker" || interrupted.ExpiresAt.Before(time.Now()) {
		process.killAndWait()
		t.Fatalf("worker did not hold a live lease: %+v", interrupted)
	}
	if err = command.Process.Kill(); err != nil {
		process.killAndWait()
		t.Fatal(err)
	}
	if err = process.wait(); err == nil {
		t.Fatal("worker process survived forced termination")
	}
	if wait := time.Until(interrupted.ExpiresAt); wait > 0 {
		time.Sleep(wait + 50*time.Millisecond)
	}
	restarted := workflow.New(f.db, f.identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) {
		return "policy-one", nil
	})
	ctx := context.Background()
	if err = restarted.Recover(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := restarted.Claim(ctx, "replacement-worker", []string{f.org}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed.JobID != interrupted.JobID || reclaimed.OperationID != interrupted.OperationID || reclaimed.Fence <= interrupted.Fence || reclaimed.AttemptID == interrupted.AttemptID {
		t.Fatalf("recovered work identity/fence mismatch: old=%+v new=%+v", interrupted, reclaimed)
	}
	if _, err = f.service.Heartbeat(ctx, interrupted, time.Minute); !errors.Is(err, workflow.ErrFence) {
		t.Fatalf("interrupted process retained lease authority: %v", err)
	}
	if _, err = f.service.Complete(ctx, interrupted, workflow.Completion{Outcome: "succeeded"}); !errors.Is(err, workflow.ErrFence) {
		t.Fatalf("interrupted process result was accepted: %v", err)
	}
	if _, err = restarted.Heartbeat(ctx, reclaimed, time.Minute); err != nil {
		t.Fatalf("replacement worker could not heartbeat: %v", err)
	}
	var state string
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state FROM workflow_attempts WHERE org_id=$1 AND id=$2`, f.org, interrupted.AttemptID).Scan(&state)
	}); err != nil {
		t.Fatal(err)
	}
	if state != "lost" {
		t.Fatalf("interrupted attempt state=%q, want lost", state)
	}
}
