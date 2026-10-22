package repair

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/source"
	"github.com/reforgeapp/reforge/pkg/store"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

type replacementSourceReader struct {
	change     *forge.Change
	connection string
	db         *store.Store
	org        string
	reads      int
	writes     int
}

func (r *replacementSourceReader) Read(ctx context.Context, org, connection string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if r.change == nil || connection != r.connection || op.Kind != privateconnector.ForgeReadChange {
		return privateconnector.Result{}, privateconnector.ErrUnsupported
	}
	if err := r.db.Tenant(ctx, r.org, "", func(tx pgx.Tx) error {
		return authorize(ctx, tx, connections.Connection{ID: connection, Version: 1, State: "healthy"})
	}); err != nil {
		return privateconnector.Result{}, err
	}
	r.reads++
	return privateconnector.Result{Change: r.change}, nil
}

func (r *replacementSourceReader) Write(context.Context, string, string, privateconnector.Operation, func(context.Context, pgx.Tx, connections.Connection) (string, error), func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	r.writes++
	return privateconnector.Result{}, privateconnector.ErrUnsupported
}

func (*replacementSourceReader) SourceReader(string, string, func(context.Context, pgx.Tx, connections.Connection) error) source.Reader {
	files := map[string][]byte{
		"go.mod":         []byte("module example.test/repo\n\ngo 1.22\n"),
		"sample_test.go": []byte("package example\n\nimport \"testing\"\n\nfunc TestFixture(t *testing.T) {}\n"),
	}
	blobs := map[string]string{}
	entries := make([]forge.SourceEntry, 0, len(files))
	for path, content := range files {
		hash := sha1.New()
		_, _ = fmt.Fprintf(hash, "blob %d\x00", len(content))
		_, _ = hash.Write(content)
		blobs[path] = hex.EncodeToString(hash.Sum(nil))
		entries = append(entries, forge.SourceEntry{Path: path, SHA: blobs[path], Mode: "100644", Type: "blob"})
	}
	return source.Reader{
		Manifest: func(_ context.Context, repo forge.RepoRef, commit string) (forge.SourceManifest, error) {
			return forge.SourceManifest{Repository: repo, CommitSHA: commit, ObjectFormat: "sha1", Proof: "immutable_ref_api", Complete: true, Entries: entries}, nil
		},
		File: func(_ context.Context, _ forge.RepoRef, path, _ string) (forge.File, error) {
			return forge.File{Path: path, SHA: blobs[path], Content: files[path]}, nil
		},
	}
}

func TestOwnerDirtyRepairPreviewAndEnqueueReplaceBranchAgainstDefault(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable PostgreSQL reforge_test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(db, identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	user, org, repo, forgeID, modelID, poolID, findingID, oldTask, priorTask, wrongDigestTask := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	session := auth.Session{ID: domain.NewID(), User: auth.User{ID: user}}
	branch := "reforge/repair/" + oldTask
	headSHA, targetSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
	repoRef := forge.RepoRef{NativeID: "native-repo", FullName: "team/repo"}
	change := forge.Change{ID: "42", Repository: repoRef, HeadRepository: repoRef, TargetRepository: repoRef, HeadBranch: branch, TargetBranch: "main", HeadSHA: headSHA, TargetSHA: targetSHA, State: "open", MergeStatus: "dirty"}
	evidence := discovery.Evidence{Provenance: "canonical provider reads at pinned commits", ConnectionID: forgeID, ConnectionVersion: 1, ConfigVersion: 1, HeadSHA: headSHA, TargetSHA: targetSHA, TargetBranch: "main", Change: &change, Checks: []forge.Check{}, Ownership: "reforge", HeadOwnership: "reforge", MergeBlockers: []string{}, Complete: true, Blockers: []string{}}
	evidenceBody, _ := json.Marshal(evidence)
	policyBody, _ := json.Marshal(policy.Policy{Schema: "maintenance/v1"})
	modelSettings := `{"billing_route":"direct_api","model":"fixture-model"}`
	routeBody, _ := json.Marshal(budget.Route{ConnectionID: modelID, Model: "fixture-model", Name: "default", Mode: "priced", PricingVersion: "fixture-v1", InputMicroUSDPerMillion: 1000, OutputMicroUSDPerMillion: 1000, MaxInputTokens: 10000, MaxOutputTokens: 1000, MaxMilliseconds: 60000, MaxRequests: 1})
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'repair-replacement-preview-test',$2,'Owner',$3)`, user, user, user+"@example.test"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, session.ID, user, domain.NewID(), domain.NewID())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Repair replacement preview')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, user); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'forge','gitea','Forge','http://127.0.0.1:53000','{}','healthy'),($1,$3,'model','test','Model','https://model.invalid',$4,'healthy')`, org, forgeID, modelID, modelSettings); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,connection_id,native_id,name,default_branch,last_synced_at) VALUES($1,$2,$3,$4,$5,'main',clock_timestamp())`, org, repo, forgeID, repoRef.NativeID, repoRef.FullName); err != nil {
			return err
		}
		jobID := domain.NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO inventory_jobs(org_id,id,connection_id,connection_version,kind,repository_id,state) VALUES($1,$2,$3,1,'refresh',$4,'complete')`, org, jobID, forgeID, repo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO inventory_repository_state(org_id,repository_id,connection_id,namespace,connection_version,scan_id,state,changes_observed_at) VALUES($1,$2,$3,'',1,$4,'fresh',clock_timestamp())`, org, repo, forgeID, jobID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_configs(org_id,repository_id,merge_authority,version) VALUES($1,$2,'reforge',1)`, org, repo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO autopilot_settings(org_id,enabled,enabled_by) VALUES($1,true,$2)`, org, user); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO policy_versions(org_id,id,scope_kind,scope_id,document,policy_hash,actor_id,reason) VALUES($1,$1,'organisation',$1,$2,$3,$4,'test')`, org, policyBody, strings.Repeat("c", 64), user); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO policy_bindings(org_id,scope_kind,scope_id,policy_version_id,version,simulation_hash) VALUES($1,'organisation',$1,$1,1,$2)`, org, strings.Repeat("d", 64)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO budget_routes(org_id,connection_id,model,name,config,version) VALUES($1,$2,'fixture-model','default',$3,1)`, org, modelID, routeBody); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runner_pools(org_id,id,name,state) VALUES($1,$2,'Fixture','active')`, org, poolID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id) VALUES($1,$2,$3)`, org, poolID, repo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'forge_change','42','ci_failure','high','Conflicting repair',$5,$6)`, org, findingID, repo, strings.Repeat("e", 64), evidenceBody, strings.Repeat("f", 64)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'go','1','main',$6,$6,'completed',1,$7)`, org, oldTask, repo, domain.NewID(), "prior-"+oldTask, strings.Repeat("1", 64), user); err != nil {
			return err
		}
		oldContext, _ := json.Marshal(ExecutionContext{FollowUpBranch: branch})
		oldChange, _ := json.Marshal(change)
		if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,branch,candidate_sha,native_change,report,candidate_artifacts) VALUES($1,$2,$3,$4,$5,1,$6,$7,'published',$8,$9,$10,'{"patches":[]}','[]')`, org, oldTask, repo, findingID, user, strings.Repeat("f", 64), oldContext, branch, headSHA, oldChange); err != nil {
			return err
		}
		priorReport, _ := json.Marshal(Report{State: "handoff", Reason: strings.Repeat("prior model summary ", 300), Candidate: []CheckResult{{CommandID: "go-test", ExitCode: 1, Complete: true, Cases: map[string]string{"TestFixture": "fail"}, Reason: "TestFixture still fails"}}, Patches: []sandbox.Patch{{Path: "secret.go", Content: []byte("DO_NOT_FORWARD_SOURCE")}}})
		for _, item := range []struct {
			taskID  string
			digest  string
			created string
			report  []byte
		}{{priorTask, strings.Repeat("f", 64), "clock_timestamp()", priorReport}, {wrongDigestTask, strings.Repeat("9", 64), "clock_timestamp()+interval '1 hour'", []byte(`{"state":"handoff","reason":"WRONG_DIGEST_FEEDBACK"}`)}} {
			if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,'go','1','main',$6,$6,'failed',1,$7,`+item.created+`)`, org, item.taskID, repo, domain.NewID(), "prior-"+item.taskID, strings.Repeat("1", 64), user); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,report,candidate_artifacts) VALUES($1,$2,$3,$4,$5,1,$6,$7,'handoff',$8,'[]')`, org, item.taskID, repo, findingID, user, item.digest, oldContext, item.report); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	workflowService := workflow.New(db, identity, func(ctx context.Context, tx pgx.Tx, task workflow.Task, _ string) (string, error) {
		resolved, err := policies.ResolveTx(ctx, tx, task.OrgID, task.RepositoryID)
		return resolved.Hash, err
	})
	workflowService.RegisterScopeCheck(func(ctx context.Context, tx pgx.Tx, scopeOrg, kind, id string) error {
		if kind != "runner_pool" {
			return auth.ErrForbidden
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runner_pools WHERE org_id=$1 AND id=$2 AND state='active')`, scopeOrg, id).Scan(&active); err != nil {
			return err
		}
		if !active {
			return auth.ErrForbidden
		}
		return nil
	})
	reader := &replacementSourceReader{connection: forgeID, db: db, org: org}
	service := New(db, identity, discovery.New(db, identity, reader), workflowService, nil, policies, budget.New(db, identity, nil, nil), connections.New(db, identity, nil, true), nil, reader, map[string]string{"go": "sha256:" + strings.Repeat("a", 64)})
	in := Input{FindingID: findingID, FindingVersion: 1, Recipe: "go", ModelConnectionID: modelID, ModelRoute: "default", RunnerPoolID: poolID, Owner: true}
	if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE autopilot_settings SET enabled=false WHERE org_id=$1`, org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ownerPreview, err := service.Preview(ctx, session, org, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(ownerPreview.Blockers) != 0 || ownerPreview.Context.ReplacesBranch != branch || ownerPreview.Context.FollowUpBranch != "" || ownerPreview.Context.Plan.BaselineSHA != headSHA || ownerPreview.Context.Plan.TargetSHA != targetSHA {
		t.Fatalf("dirty owned replacement preview=%+v", ownerPreview)
	}
	feedback := ownerPreview.Context.PriorFeedback
	if len(feedback) >= 2048 || !strings.Contains(feedback, "Previous attempt task state: failed") || !strings.Contains(feedback, "go-test") || !strings.Contains(feedback, "TestFixture") || !strings.Contains(feedback, "TestFixture still fails") || strings.Contains(feedback, "WRONG_DIGEST_FEEDBACK") || strings.Contains(feedback, "DO_NOT_FORWARD_SOURCE") {
		t.Fatalf("prior feedback was not scoped and bounded: %q", feedback)
	}
	nonOwner := in
	nonOwner.Owner = false
	nonOwnerPreview, err := service.Preview(ctx, session, org, nonOwner)
	if err != nil || len(nonOwnerPreview.Blockers) == 0 {
		t.Fatalf("non-owner conflict preview was not blocked: blockers=%v err=%v", nonOwnerPreview.Blockers, err)
	}
	normalChange := change
	normalChange.MergeStatus = ""
	normalEvidence := evidence
	normalEvidence.Change = &normalChange
	normalBody, _ := json.Marshal(normalEvidence)
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE maintenance_findings SET evidence=$3 WHERE org_id=$1 AND id=$2`, org, findingID, normalBody)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ordinaryPreview, err := service.Preview(ctx, session, org, nonOwner)
	if err != nil {
		t.Fatal(err)
	}
	if ordinaryPreview.Context.ReplacesBranch != "" || ordinaryPreview.Context.FollowUpBranch != branch || ordinaryPreview.Context.Plan.BaselineSHA != headSHA || ordinaryPreview.Context.Plan.TargetSHA != headSHA {
		t.Fatalf("ordinary follow-up preview=%+v", ordinaryPreview)
	}
	botEvidence := normalEvidence
	botChange := normalChange
	botChange.HeadBranch = "renovate/example"
	botEvidence.Change, botEvidence.Bot, botEvidence.Ownership = &botChange, "renovate", "bot"
	botBody, _ := json.Marshal(botEvidence)
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE maintenance_findings SET evidence=$3 WHERE org_id=$1 AND id=$2`, org, findingID, botBody)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	botPreview, err := service.Preview(ctx, session, org, in)
	if err != nil {
		t.Fatal(err)
	}
	if botPreview.Context.Plan.Owner || botPreview.Context.Plan.BaselineSHA != headSHA || botPreview.Context.Plan.TargetSHA != targetSHA || len(botPreview.Context.Plan.ProtectedHashes) == 0 {
		t.Fatalf("trusted bot repair lost frozen update checks: %+v", botPreview.Context.Plan)
	}
	if CheckPatch(botPreview.Context.Plan, map[string][]byte{"go.mod": []byte("module example.test/repo\n\ngo 1.22\n")}, []sandbox.Patch{{Path: "go.mod", Content: []byte("module example.test/repo\n\ngo 1.21\n")}}) == nil {
		t.Fatal("trusted bot repair could revert dependency metadata")
	}
	foreignEvidence := evidence
	foreignChange := change
	foreignChange.HeadRepository = forge.RepoRef{NativeID: "fork-repo", FullName: "fork/repo"}
	foreignEvidence.Change = &foreignChange
	foreignBody, _ := json.Marshal(foreignEvidence)
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE maintenance_findings SET evidence=$3 WHERE org_id=$1 AND id=$2`, org, findingID, foreignBody)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	foreignPreview, err := service.Preview(ctx, session, org, in)
	if err != nil {
		t.Fatal(err)
	}
	if foreignPreview.Context.ReplacesBranch != "" || foreignPreview.Context.FollowUpBranch != branch {
		t.Fatalf("foreign branch incorrectly selected as replacement: %+v", foreignPreview.Context)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE maintenance_findings SET evidence=$3 WHERE org_id=$1 AND id=$2`, org, findingID, evidenceBody)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	in.PlanDigest = ownerPreview.Context.Plan.Digest
	in.IdempotencyKey = "replacement-" + domain.NewID()
	if repeated, repeatErr := service.Preview(ctx, session, org, in); repeatErr != nil || len(repeated.Blockers) != 0 || repeated.Context.Plan.Digest != in.PlanDigest {
		t.Fatalf("replacement re-preview=%+v err=%v", repeated, repeatErr)
	}
	run, err := service.Enqueue(ctx, session, org, in, "replacement-preview-test")
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if run.Context.ReplacesBranch != branch || run.Context.FollowUpBranch != "" || run.Context.Plan.BaselineSHA != headSHA || run.Context.Plan.TargetSHA != targetSHA {
		t.Fatalf("enqueued replacement context=%+v", run.Context)
	}
	newHead, newTarget := strings.Repeat("c", 40), strings.Repeat("d", 40)
	currentChange := change
	currentChange.HeadSHA, currentChange.TargetSHA = newHead, newTarget
	reader.change = &currentChange
	baselineChecks := make([]CheckResult, 0, len(run.Context.Plan.Recipe.Commands))
	for _, check := range run.Context.Plan.Recipe.Commands {
		baselineChecks = append(baselineChecks, CheckResult{CommandID: check.ID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestFixture": "pass"}})
	}
	priorReport, _ := json.Marshal(Report{State: "unverified", Mode: "owner", PlanDigest: run.Context.Plan.Digest, Reason: "Saved owner proposal", Baseline: baselineChecks, Candidate: baselineChecks, Patches: []sandbox.Patch{{Path: "value.go", Content: []byte("package example\n")}}})
	currentBody, _ := json.Marshal(currentChange)
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_tasks SET state='completed' WHERE org_id=$1 AND id=$2`, org, run.Task.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE repair_runs SET state='published',branch=$3,candidate_sha=$4,native_change=$5,report=$6 WHERE org_id=$1 AND task_id=$2`, org, run.Task.ID, branch, headSHA, currentBody, priorReport)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	reason, err := service.RevalidateFix(ctx, session, org, run.Task.ID)
	if err != nil || !strings.Contains(reason, "Rechecking saved repair") {
		t.Fatalf("revalidation enqueue reason=%q err=%v", reason, err)
	}
	var childID, childState string
	var childMaxAttempts int
	var child Run
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id::text,state,max_attempts FROM workflow_tasks WHERE org_id=$1 AND repository_id=$2 AND idempotency_key LIKE 'repair-validation/%'`, org, repo).Scan(&childID, &childState, &childMaxAttempts); err != nil {
			return err
		}
		var loadErr error
		child, loadErr = loadRun(ctx, tx, org, childID)
		return loadErr
	}); err != nil {
		t.Fatal(err)
	}
	if childState != "queued" || childMaxAttempts != 2 || child.Context.ValidationTaskID != run.Task.ID || child.Context.ValidationTargetSHA != newTarget || child.Context.Plan.BaselineSHA != newHead || child.Context.Plan.TargetSHA != newHead || !reflect.DeepEqual(child.Context.Plan.Recipe.Commands, run.Context.Plan.Recipe.Commands) || child.Report == nil || child.Report.PlanDigest != child.Context.Plan.Digest || len(child.Report.Patches) != 0 || len(child.Report.Diff) != 0 {
		t.Fatalf("prepared validation child=%+v task_state=%s max_attempts=%d", child, childState, childMaxAttempts)
	}
	for _, name := range []string{"repeated head does not duplicate task", "failed child remains parked"} {
		if name == "failed child remains parked" {
			if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE workflow_tasks SET state='failed' WHERE org_id=$1 AND id=$2`, org, childID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
		message, revalidateErr := service.RevalidateFix(ctx, session, org, run.Task.ID)
		if revalidateErr != nil {
			t.Fatalf("%s: %v", name, revalidateErr)
		}
		want := "Rechecking saved repair"
		if name == "failed child remains parked" {
			want = "waiting for updated code or validation tools"
		}
		if !strings.Contains(message, want) {
			t.Fatalf("%s returned %q", name, message)
		}
	}
	defectContext := run.Context
	defectContext.Finding = discovery.Finding{Observation: discovery.Observation{Source: "user_report", Category: "code_defect"}}
	defectContextBody, _ := json.Marshal(defectContext)
	defectReport, _ := json.Marshal(Report{State: "unverified", Mode: "owner", PlanDigest: defectContext.Plan.Digest, Baseline: baselineChecks, Candidate: baselineChecks})
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE repair_runs SET context=$3,report=$4 WHERE org_id=$1 AND task_id=$2`, org, run.Task.ID, defectContextBody, defectReport)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	reader.change.HeadSHA, reader.change.TargetSHA = strings.Repeat("e", 40), strings.Repeat("f", 40)
	readsBeforeUnreproduced := reader.reads
	message, err := service.RevalidateFix(ctx, session, org, run.Task.ID)
	if err != nil || message != "" || reader.reads != readsBeforeUnreproduced {
		t.Fatalf("unreproduced report triggered validation: message=%q err=%v reads=%d before=%d", message, err, reader.reads, readsBeforeUnreproduced)
	}
	var taskCount int
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM workflow_tasks WHERE org_id=$1 AND repository_id=$2 AND idempotency_key LIKE 'repair-validation/%'`, org, repo).Scan(&taskCount)
	}); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 || reader.writes != 0 {
		t.Fatalf("validation tasks=%d source writes=%d", taskCount, reader.writes)
	}
}
