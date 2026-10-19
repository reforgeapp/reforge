package integration

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/artifact"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/budget"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/control"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/maintenance/repair"
	"github.com/reforgeapp/reforge/internal/policy"
	"github.com/reforgeapp/reforge/internal/privateconnector"
	"github.com/reforgeapp/reforge/internal/runner"
	"github.com/reforgeapp/reforge/internal/sandbox"
	"github.com/reforgeapp/reforge/internal/source"
	"github.com/reforgeapp/reforge/internal/workflow"
)

type repairContractReader struct {
	fixture                *discoveryFixture
	files                  map[string][]byte
	writes                 int
	branch, sha, operation string
	lose                   bool
	change                 *forge.Change
}

func (r *repairContractReader) authorize(ctx context.Context, org, id string, fn func(context.Context, pgx.Tx, connections.Connection) error) error {
	return r.fixture.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var lock string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&lock); err != nil {
			return err
		}
		c, err := r.fixture.connections.MetadataTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		return fn(ctx, tx, c)
	})
}
func (r *repairContractReader) SourceReader(org, id string, fn func(context.Context, pgx.Tx, connections.Connection) error) source.Reader {
	return source.Reader{Manifest: func(ctx context.Context, ref forge.RepoRef, sha string) (forge.SourceManifest, error) {
		if err := r.authorize(ctx, org, id, fn); err != nil {
			return forge.SourceManifest{}, err
		}
		files := r.files
		if sha == r.sha {
			files = map[string][]byte{}
			for path, body := range r.files {
				files[path] = body
			}
			files["value.js"] = []byte("exports.value = () => 2")
		}
		m := forge.SourceManifest{Repository: ref, CommitSHA: sha, ObjectFormat: "sha1", Proof: "immutable_ref_api", Complete: true}
		for path, body := range files {
			m.Entries = append(m.Entries, forge.SourceEntry{Path: path, SHA: repairBlob(body), Type: "blob", Mode: "100644"})
		}
		return m, nil
	}, File: func(ctx context.Context, ref forge.RepoRef, path, sha string) (forge.File, error) {
		if err := r.authorize(ctx, org, id, fn); err != nil {
			return forge.File{}, err
		}
		body := r.files[path]
		if sha == r.sha && path == "value.js" {
			body = []byte("exports.value = () => 2")
		}
		return forge.File{Path: path, SHA: repairBlob(body), Content: body}, nil
	}}
}
func repairBlob(body []byte) string {
	sum := sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(body))), body...))
	return hex.EncodeToString(sum[:])
}
func (r *repairContractReader) Read(ctx context.Context, org, id string, op privateconnector.Operation, fn func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if err := r.authorize(ctx, org, id, fn); err != nil {
		return privateconnector.Result{}, err
	}
	out := privateconnector.Result{OperationID: op.ID}
	switch op.Kind {
	case privateconnector.ForgeResolveRef:
		out.SHA = strings.Repeat("b", 40)
		if strings.HasPrefix(op.Ref.Ref, "reforge/") {
			out.SHA = r.sha
		}
	case privateconnector.ForgeCommitProof:
		out.Commit = &forge.CommitProof{SHA: r.sha, Parents: []string{strings.Repeat("b", 40)}, Message: "repair\n\n[reforge-operation:" + r.operation + "]"}
	case privateconnector.ForgeFindChange:
		out.Change = r.change
	default:
		return out, fmt.Errorf("unhandled repair fixture read %s", op.Kind)
	}
	return out, nil
}
func (r *repairContractReader) Write(ctx context.Context, org, id string, op privateconnector.Operation, fn func(context.Context, pgx.Tx, connections.Connection) (string, error), validate func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if err := r.authorize(ctx, org, id, func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		_, err := fn(ctx, tx, c)
		return err
	}); err != nil {
		return privateconnector.Result{}, err
	}
	if err := r.authorize(ctx, org, id, validate); err != nil {
		return privateconnector.Result{}, err
	}
	r.writes++
	out := privateconnector.Result{OperationID: op.ID}
	if op.Kind == privateconnector.ForgeUpdateBranch {
		r.sha = strings.Repeat("c", 40)
		r.branch = op.Branch.Branch
		r.operation = op.ID
		out.SHA = r.sha
	} else {
		r.change = &forge.Change{ID: "17", OperationID: op.ID, State: "open", HeadSHA: op.Create.ExpectedHeadSHA, HeadBranch: op.Create.HeadBranch, TargetBranch: op.Create.TargetBranch, HeadRepository: op.Create.Repository, TargetRepository: op.Create.Repository}
		out.Change = r.change
	}
	if r.lose {
		return privateconnector.Result{}, privateconnector.ErrUncertain
	}
	return out, nil
}
func TestRepairFrozenPreviewArtifactBindingAndLostPublication(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancel), func(t *testing.T) { repairFrozenPreviewAndPublication(t, cancel) })
	}
}

func repairFrozenPreviewAndPublication(t *testing.T, cancel bool) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	authority := control.NewAuthority(policies)
	jobs := workflow.New(f.db, f.identity, authority.Check)
	artifactRoot := t.TempDir()
	if err := os.Chmod(artifactRoot, 0700); err != nil {
		t.Fatal(err)
	}
	artifacts, err := artifact.NewLocal(f.db, artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer artifacts.Close()
	runners := runner.New(f.db, f.identity, jobs, artifacts)
	jobs.RegisterScopeCheck(runners.CheckScopeTx)
	f.connections.RegisterRunnerCheck(runners.CheckRunnerTx)
	pool, err := runners.PutPool(ctx, f.owner, f.org, "", runner.PoolInput{Name: "Repair contract", RepositoryIDs: []string{f.repo}}, 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := runners.EnrollToken(ctx, f.owner, f.org, pool.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := runners.Enroll(ctx, enrollment.Token, "contract")
	if err != nil {
		t.Fatal(err)
	}
	modelID := domain.NewID()
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'model','compatible','Repair fixture','https://model.example','{"model":"fixture-model","auth_kind":"api_key","billing_route":"direct_api"}','healthy')`, f.org, modelID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	document := policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{Recipes: []string{"javascript"}, Models: []string{"fixture-model"}, Routes: []string{modelID + "/default"}}}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, document, "contract", "test")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Repair})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, sim.Hash, "contract", "test"); err != nil {
		t.Fatal(err)
	}
	budgets := budget.New(f.db, f.identity, func(ctx context.Context, tx pgx.Tx, l budget.Lease) error {
		_, err := jobs.ValidateFenceTx(ctx, tx, workflow.Lease(l), "budget")
		return err
	}, nil)
	route, err := budgets.PutRoute(ctx, f.owner, f.org, budget.Route{ConnectionID: modelID, Model: "fixture-model", Name: "default", Mode: "priced", PricingVersion: "fixture-zero", MaxInputTokens: 100000, MaxOutputTokens: 4096, MaxMilliseconds: 60000, MaxRequests: 1}, 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	reader := &repairContractReader{fixture: f, files: map[string][]byte{"value.js": []byte("exports.value = () => 1"), "value.test.js": []byte("require('node:test')('value',()=>require('node:assert').equal(require('./value').value(),2))")}}
	service := repair.New(f.db, f.identity, f.service, jobs, runners, policies, budgets, f.connections, nil, reader, map[string]string{"javascript": "sha256:" + strings.Repeat("a", 64)})
	authority.Register("repair.stage", service.CheckStage)
	authority.Register("stage", service.CheckStage)
	authority.Register("publish", service.CheckPublish)
	authority.Register("outbox.dispatch", func(ctx context.Context, tx pgx.Tx, t workflow.Task, p policy.Resolved) error {
		if t.State == domain.TaskValidating {
			return service.CheckStage(ctx, tx, t, p)
		}
		return service.CheckPublish(ctx, tx, t, p)
	})
	for _, action := range []string{"repair.source", "repair.report"} {
		authority.Register(action, func(context.Context, pgx.Tx, workflow.Task, policy.Resolved) error { return nil })
	}
	runners.CompletionCheck = service.CheckCompletion
	finding := f.observe(t, f.observation("repair-contract", "main", strings.Repeat("a", 40), nil))
	input := repair.Input{FindingID: finding.ID, FindingVersion: finding.Version, Recipe: "javascript", ModelConnectionID: modelID, ModelRoute: "default", RunnerPoolID: pool.ID, IdempotencyKey: domain.NewID()}
	preview, err := service.Preview(ctx, f.owner, f.org, input)
	if err != nil || len(preview.Blockers) > 0 {
		t.Fatalf("preview: %v %+v", err, preview.Blockers)
	}
	input.PlanDigest = preview.Context.Plan.Digest
	route.MaxOutputTokens = 2048
	route, err = budgets.PutRoute(ctx, f.owner, f.org, route, route.Version, "changed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Enqueue(ctx, f.owner, f.org, input, "stale"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale pricing preview admitted: %v", err)
	}
	preview, err = service.Preview(ctx, f.owner, f.org, input)
	if err != nil {
		t.Fatal(err)
	}
	input.PlanDigest = preview.Context.Plan.Digest
	run, err := service.Enqueue(ctx, f.owner, f.org, input, "repair")
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Enqueue(ctx, f.owner, f.org, input, "retry")
	if err != nil || again.Task.ID != run.Task.ID {
		t.Fatalf("enqueue idempotency: %v", err)
	}
	changed := input
	changed.FindingVersion++
	if _, err = service.Enqueue(ctx, f.owner, f.org, changed, "conflict"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("changed idempotency body: %v", err)
	}
	job, err := runners.Claim(ctx, supervisor.Token)
	if err != nil {
		t.Fatal(err)
	}
	credential := job.Credential.Token
	handoff := repair.Report{PlanDigest: run.Context.Plan.Digest, State: "handoff", Reason: "previous runner stopped", Baseline: []repair.CheckResult{}, Candidate: []repair.CheckResult{}, Target: []repair.CheckResult{}, Patches: []sandbox.Patch{}, Artifacts: []string{}}
	if _, err = service.SaveReport(ctx, credential, handoff); err != nil {
		t.Fatalf("save handoff: %v", err)
	}
	firstReservation := budget.Reservation{ID: domain.NewID(), Lease: budget.Lease(job.Lease), Quote: budget.Quote{MaxOutputTokens: 10, MaxMilliseconds: 1000}}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return service.CheckModelTx(ctx, tx, job.Task, firstReservation)
	}); !errors.Is(err, workflow.ErrPolicy) {
		t.Fatalf("same attempt resumed saved handoff: %v", err)
	}
	saved, err := service.Get(ctx, f.owner, f.org, run.Task.ID)
	if err != nil || saved.State != "handoff" || saved.Report == nil {
		t.Fatalf("same-attempt rejection changed saved handoff: state=%s report=%+v err=%v", saved.State, saved.Report, err)
	}
	if _, err = runners.Complete(ctx, credential, workflow.Completion{Outcome: "failed", Retryable: true}); err != nil {
		t.Fatalf("complete handoff attempt: %v", err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_jobs SET available_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2`, f.org, run.Task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	retried, err := runners.Claim(ctx, supervisor.Token)
	if err != nil {
		t.Fatalf("claim retry: %v", err)
	}
	reservation := budget.Reservation{ID: domain.NewID(), Lease: budget.Lease(retried.Lease), Quote: budget.Quote{MaxOutputTokens: 10, MaxMilliseconds: 1000}}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return service.CheckModelTx(ctx, tx, retried.Task, reservation)
	}); err != nil {
		t.Fatalf("admit fresh attempt after handoff: %v", err)
	}
	restarted, err := service.Get(ctx, f.owner, f.org, run.Task.ID)
	var active bool
	if err == nil {
		err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT active FROM maintenance_repairs WHERE org_id=$1 AND task_id=$2`, f.org, run.Task.ID).Scan(&active)
		})
	}
	if err != nil || restarted.State != "queued" || restarted.Report != nil || !active {
		t.Fatalf("handoff retry did not reacquire repair: state=%s report=%+v active=%t err=%v", restarted.State, restarted.Report, active, err)
	}
	job = retried
	credential = retried.Credential.Token
	for _, state := range []domain.TaskState{domain.TaskPlanning, domain.TaskRepairing, domain.TaskValidating} {
		if _, err = runners.Progress(ctx, credential, state); err != nil {
			t.Fatal(err)
		}
	}
	command := run.Context.Plan.Recipe.Commands[0]
	failure := []byte("not ok 1 - value\n")
	success := []byte("ok 1 - value\n")
	baseline := repair.Interpret(command, sandbox.CommandResult{ExitCode: 1, Output: failure})
	passed := repair.Interpret(command, sandbox.CommandResult{Output: success})
	upload := func(name string, raw []byte) string {
		t.Helper()
		a, e := runners.Upload(ctx, credential, name, "text/plain", bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		return a.ID
	}
	failedID := upload("baseline.log", failure)
	passedID := upload("candidate.log", success)
	report := repair.Report{PlanDigest: run.Context.Plan.Digest, State: "validated", Baseline: []repair.CheckResult{baseline}, Candidate: []repair.CheckResult{passed}, Target: []repair.CheckResult{passed}, Patches: []sandbox.Patch{{Path: "value.js", Content: []byte("exports.value = () => 2")}}, Artifacts: []string{passedID}}
	if _, err = service.SaveReport(ctx, credential, report); !errors.Is(err, repair.ErrValidation) {
		t.Fatalf("unbound baseline evidence accepted: %v", err)
	}
	report.Artifacts = append(report.Artifacts, failedID)
	if _, err = service.SaveReport(ctx, credential, report); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveReport(ctx, credential, report); err != nil {
		t.Fatalf("report replay: %v", err)
	}
	if _, err = runners.Complete(ctx, credential, workflow.Completion{Outcome: "completed"}); !errors.Is(err, repair.ErrValidation) {
		t.Fatalf("unpublished completion admitted: %v", err)
	}
	staged, err := service.Stage(ctx, credential)
	if err != nil || staged.CandidateSHA == "" {
		t.Fatalf("stage: %v", err)
	}
	failedNative := repair.Publication{HeadSHA: staged.CandidateSHA, PlanDigest: report.PlanDigest, Checks: []repair.CheckResult{baseline}, ArtifactIDs: []string{failedID}}
	if _, err = service.Publish(ctx, credential, failedNative); !errors.Is(err, repair.ErrValidation) {
		t.Fatalf("failed native validation published: %v", err)
	}
	recorded, err := service.Get(ctx, f.owner, f.org, run.Task.ID)
	if err != nil || recorded.Report == nil || !strings.Contains(recorded.Report.Diff, "-exports.value = () => 1") || !strings.Contains(recorded.Report.Diff, "+exports.value = () => 2") || len(recorded.CandidateChecks) != 1 || recorded.CandidateChecks[0].ExitCode != 1 || reader.writes != 1 {
		t.Fatalf("native failure evidence missing or publication dispatched: %v", err)
	}
	latest, err := service.Baseline(ctx, f.owner, f.org, f.repo)
	if err != nil || latest == nil || latest.Task.ID != run.Task.ID {
		t.Fatalf("repository baseline binding: %v", err)
	}
	if _, err = service.Baseline(ctx, f.owner, f.org, domain.NewID()); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("absent repository baseline enumerated: %v", err)
	}
	reader.lose = true
	publication := repair.Publication{HeadSHA: staged.CandidateSHA, PlanDigest: report.PlanDigest, Checks: []repair.CheckResult{passed}, ArtifactIDs: []string{passedID}}
	if _, err = service.Publish(ctx, credential, publication); !errors.Is(err, workflow.ErrReconciliation) {
		t.Fatalf("lost publication response: %v", err)
	}
	if _, err = service.Publish(ctx, credential, publication); !errors.Is(err, workflow.ErrReconciliation) {
		t.Fatalf("uncertain replay: %v", err)
	}
	if reader.writes != 2 {
		t.Fatalf("duplicate external writes: %d", reader.writes)
	}
	if _, err = runners.Complete(ctx, credential, workflow.Completion{Outcome: "uncertain"}); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_outbox SET updated_at=clock_timestamp()-interval '3 minutes' WHERE org_id=$1 AND task_id=$2`, f.org, run.Task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	current, err := service.Get(ctx, f.owner, f.org, run.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedState := domain.TaskCompleted
	if cancel {
		if _, err = jobs.Cancel(ctx, f.owner, f.org, run.Task.ID, current.Task.Version, "cancel"); err != nil {
			t.Fatal(err)
		}
		expectedState = domain.TaskCancelled
	}
	reconciled, err := service.Reconcile(ctx, f.owner, f.org, run.Task.ID, current.Version, "reconcile")
	if err != nil || reconciled.State != "published" || reconciled.Task.State != expectedState || reader.writes != 2 {
		t.Fatalf("lost response recovery: state=%s task=%s writes=%d err=%v", reconciled.State, reconciled.Task.State, reader.writes, err)
	}
}
