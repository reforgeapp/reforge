package repair

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/model"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

func TestOwnerWithoutTestsPublishesOnlyUnverifiedProposal(t *testing.T) {
	files := map[string][]byte{"package.json": []byte(`{"name":"shop"}`), "checkout.js": []byte("exports.checkout=()=>false")}
	plan, err := freeze("javascript", "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 40), strings.Repeat("c", 40), files, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	plan.Owner = true
	plan.Digest = planDigest(plan)
	runtime := &passRuntime{retryRuntime: retryRuntime{files: files, patches: map[string][]sandbox.Patch{}}, target: files, targetSHA: plan.TargetSHA}
	engine := Engine{Runtime: runtime, Model: "fixture", MaxOutputTokens: 256, TurnTimeout: time.Second, JobID: "proposal", AttemptID: "proposal", Trust: "fixture", Progress: func(context.Context, string) error { return nil }, Turn: func(context.Context, model.Turn) (model.TurnResult, error) {
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "fix", Name: "write_file", Arguments: []byte(`{"path":"checkout.js","content":"exports.checkout=()=>true"}`)}, {ID: "checks", Name: "run_checks", Arguments: []byte(`{}`)}, {ID: "finish", Name: "finish", Arguments: []byte(`{"summary":"Proposed checkout fix"}`)}}}, nil
	}}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
	if err != nil || report.State != "unverified" || Verified(plan, report.Baseline, report.Candidate) || !publicationReady(Run{Context: ExecutionContext{Plan: plan}, Report: &report}, report.Candidate) {
		t.Fatalf("proposal truth/permission changed: %+v %v", report, err)
	}
	report.State = "validated"
	if publicationReady(Run{Context: ExecutionContext{Plan: plan}, Report: &report}, report.Candidate) {
		t.Fatal("no-tests result accepted as validated")
	}
}

func TestProposalCannotHideFailedLostTimedOutBehaviour(t *testing.T) {
	plan, _ := testPlan(t)
	plan.Owner = true
	plan.Digest = planDigest(plan)
	command := plan.Recipe.Commands[0]
	before := Interpret(command, sandbox.CommandResult{Output: []byte("ok 1 - checkout\n")})
	for _, raw := range []sandbox.CommandResult{{ExitCode: 1, Output: []byte("not ok 1 - checkout\n")}, {TimedOut: true}, {Truncated: true}, {Output: []byte("ok 1 - different\n")}, {Output: []byte("ok 1 - checkout # SKIP\n")}} {
		if ProposalReady(plan, []CheckResult{before}, []CheckResult{Interpret(command, raw)}) {
			t.Fatalf("bad result published as proposal: %+v", raw)
		}
	}
	failed := Interpret(command, sandbox.CommandResult{ExitCode: 1, Output: []byte("not ok 1 - checkout\n")})
	different := Interpret(command, sandbox.CommandResult{Output: []byte("ok 1 - unrelated\n")})
	if BehaviourFixed(plan, []CheckResult{failed}, []CheckResult{different}) {
		t.Fatal("different passing case accepted as regression fix")
	}
}
