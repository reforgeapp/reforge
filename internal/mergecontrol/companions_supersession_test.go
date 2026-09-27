package mergecontrol

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/privateconnector"
	"reforge/internal/providers"
	"reforge/internal/source"
	"reforge/internal/store"
)

func TestSupersessionRequiresExactPublishedReplacementBinding(t *testing.T) {
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
	org, repoID, user := domain.NewID(), domain.NewID(), domain.NewID()
	oldTask, newTask := domain.NewID(), domain.NewID()
	oldFinding, newFinding := domain.NewID(), domain.NewID()
	gateID, operationID := domain.NewID(), domain.NewID()
	oldID, newID := "120", "127"
	oldBranch, newBranch, target := "reforge/repair/old", "reforge/repair/new", "main"
	oldCandidate, oldObserved := strings.Repeat("a", 40), strings.Repeat("b", 40)
	newCandidate, newHead, merge := strings.Repeat("c", 40), strings.Repeat("d", 40), strings.Repeat("e", 40)
	repository := forge.RepoRef{NativeID: "supersession-repo", FullName: "org/repo"}
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'supersession-test',$2,'Owner',$3)`, user, user, user+"@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	gateDoc, _ := json.Marshal(map[string]any{
		"binding": map[string]any{"head": newHead}, "decision": map[string]any{"outcome": "allow"},
		"snapshot": map[string]any{"change": map[string]any{"id": newID, "head_sha": newHead, "head_branch": newBranch, "target_branch": target, "repository": repository, "head_repository": repository, "target_repository": repository}},
	})
	nativeResult, _ := json.Marshal(forge.MergeResult{State: "merged", HeadSHA: newHead, MergeSHA: merge})
	oldContext := `{"finding":{"evidence":{"change":{"id":"84","head_sha":"` + strings.Repeat("f", 40) + `","head_branch":"dependabot/pkg"}}}}`
	newContext := `{"finding":{"evidence":{"change":{"id":"120","head_sha":"` + oldObserved + `","head_branch":"` + oldBranch + `"}}},"request":{"owner":true},"plan":{"owner":true},"replaces_branch":"` + oldBranch + `"}`
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Supersession test')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,$3,$4)`, org, repoID, repository.NativeID, repository.FullName); err != nil {
			return err
		}
		for i, f := range []string{oldFinding, newFinding} {
			if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'native_ci',$5,'test_failure','high','Supersession','{}',$4)`, org, f, repoID, strings.Repeat(string(rune('a'+i)), 64), "supersession-"+f); err != nil {
				return err
			}
		}
		for _, task := range []string{oldTask, newTask} {
			if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main',$6,$6,'completed',1,$7)`, org, task, repoID, domain.NewID(), "supersession-"+task, strings.Repeat("b", 64), user); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,candidate_sha,branch,native_change) VALUES($1,$2,$3,$4,$5,1,$6,$7,'published',$8,$9,$10)`, org, oldTask, repoID, oldFinding, user, strings.Repeat("c", 64), oldContext, oldCandidate, oldBranch, `{"id":"120","target_branch":"main"}`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,candidate_sha,branch,native_change) VALUES($1,$2,$3,$4,$5,1,$6,$7,'published',$8,$9,$10)`, org, newTask, repoID, newFinding, user, strings.Repeat("d", 64), newContext, newCandidate, newBranch, `{"id":"127","target_branch":"main"}`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO merge_gates(org_id,id,repository_id,change_id,configuration_version,document) VALUES($1,$2,$3,$4,1,$5)`, org, gateID, repoID, newID, gateDoc); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO merge_operations(org_id,id,repository_id,gate_id,requested_gate_id,change_id,target_branch,idempotency_key,requested_by,state,native_result) VALUES($1,$2,$3,$4,$4,$5,$6,$8,$7,'merged',$9)`, org, operationID, repoID, gateID, newID, target, user, domain.NewID(), nativeResult)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	proof := Companion{TaskID: oldTask, FindingID: oldFinding, ChangeID: oldID, HeadSHA: oldCandidate, ObservedHeadSHA: oldObserved, Branch: oldBranch, TargetBranch: target, SourceChangeID: "84", SourceHeadSHA: strings.Repeat("f", 40), SourceBranch: "dependabot/pkg", State: "superseded", Supersession: []ReplacementProof{{TaskID: newTask, FindingID: newFinding, ChangeID: newID, CandidateSHA: newCandidate, Branch: newBranch, TargetBranch: target, SourceChangeID: oldID, SourceHeadSHA: oldObserved, SourceBranch: oldBranch, ObservedHeadSHA: newHead, MergeSHA: merge}}}
	gate := Gate{RepositoryID: repoID, Snapshot: Snapshot{Change: forge.Change{ID: "84", Repository: repository}}, Companions: []Companion{proof}}
	validate := func() bool {
		var valid bool
		if err := db.Tenant(ctx, org, "", func(tx pgx.Tx) error { var e error; valid, e = validateCompanionsTx(ctx, tx, org, gate); return e }); err != nil {
			t.Fatal(err)
		}
		return valid
	}
	if !validate() {
		t.Fatal("valid replacement with exact merge gate was rejected")
	}
	oldChange := &forge.Change{ID: oldID, Repository: repository, HeadRepository: repository, TargetRepository: repository, HeadSHA: oldObserved, HeadBranch: oldBranch, TargetBranch: target, State: "open"}
	terminal := &forge.Change{ID: newID, Repository: repository, HeadRepository: repository, TargetRepository: repository, HeadSHA: newHead, HeadBranch: newBranch, TargetBranch: target, State: "merged", MergeSHA: merge}
	provider := &supersessionProvider{changes: map[string]*forge.Change{newID: terminal}}
	service := &Service{db: db, providers: provider}
	resolve := func() ([]ReplacementProof, bool, error) {
		return service.resolveReplacementChain(ctx, org, repoID, "connection", proof, oldChange, repository, target, nil)
	}
	chain, terminalOK, err := resolve()
	if err != nil || !terminalOK || len(chain) != 1 || chain[0].ObservedHeadSHA != newHead {
		t.Fatalf("chain=%+v terminal=%t err=%v", chain, terminalOK, err)
	}
	foreign := *terminal
	foreign.Repository.NativeID = "foreign"
	provider.changes[newID] = &foreign
	if _, accepted, err := resolve(); err == nil || accepted {
		t.Fatal("cross-repository replacement accepted")
	}
	unmerged := *terminal
	unmerged.State = "open"
	provider.changes[newID] = &unmerged
	if _, accepted, err := resolve(); err != nil || accepted {
		t.Fatalf("unmerged terminal accepted=%t err=%v", accepted, err)
	}
	provider.changes[newID] = terminal
	duplicateTask, duplicateFinding := domain.NewID(), domain.NewID()
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'native_ci',$5,'test_failure','high','Supersession','{}',$4)`, org, duplicateFinding, repoID, strings.Repeat("z", 64), "supersession-"+duplicateFinding); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main',$6,$6,'completed',1,$7)`, org, duplicateTask, repoID, domain.NewID(), "supersession-"+duplicateTask, strings.Repeat("e", 64), user); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,candidate_sha,branch,native_change) VALUES($1,$2,$3,$4,$5,1,$6,$7,'published',$8,$9,$10)`, org, duplicateTask, repoID, duplicateFinding, user, strings.Repeat("f", 64), newContext, strings.Repeat("f", 40), "reforge/repair/duplicate", `{"id":"128","target_branch":"main"}`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, accepted, err := resolve(); err == nil || accepted {
		t.Fatal("ambiguous replacement candidate accepted")
	}
	if validate() {
		t.Fatal("execution validation accepted a newly ambiguous replacement")
	}
	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM repair_runs WHERE org_id=$1 AND task_id=$2`, org, duplicateTask)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err = db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE repair_runs SET context=jsonb_set(context,'{finding,evidence,change,head_sha}',to_jsonb($3::text)) WHERE org_id=$1 AND task_id=$2`, org, newTask, strings.Repeat("f", 40))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if validate() {
		t.Fatal("replacement linked to a stale predecessor head was accepted")
	}
	if _, accepted, err := resolve(); err != nil || accepted {
		t.Fatalf("stale source link accepted=%t err=%v", accepted, err)
	}
}

type supersessionProvider struct{ changes map[string]*forge.Change }

func (p *supersessionProvider) Read(_ context.Context, _, _ string, operation privateconnector.Operation, _ func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	return privateconnector.Result{Change: p.changes[operation.Change.ChangeID]}, nil
}
func (p *supersessionProvider) Write(context.Context, string, string, privateconnector.Operation, func(context.Context, pgx.Tx, connections.Connection) (string, error), func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	return privateconnector.Result{}, nil
}
func (p *supersessionProvider) SourceReader(string, string, func(context.Context, pgx.Tx, connections.Connection) error) source.Reader {
	return source.Reader{}
}
func (p *supersessionProvider) ForProtection(string, map[string]string) providers.Client { return p }
