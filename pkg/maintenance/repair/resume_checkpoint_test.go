package repair

import (
	"testing"

	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

func TestResumeCheckpointFromFailedOwnerJob(t *testing.T) {
	plan, _ := testPlan(t)
	plan.Owner = true
	plan.Digest = planDigest(plan)
	current := ExecutionContext{
		Request:           Input{Owner: true, Recipe: plan.Recipe.Name, ModelConnectionID: "connection", ModelRoute: "default"},
		Plan:              plan,
		ConnectionID:      "connection",
		ConnectionVersion: 3,
		Model:             "fixture-model",
		PolicyHash:        "current-policy",
		Finding:           discovery.Finding{ID: "finding", RepositoryID: "repo"},
	}
	checkpoint := Checkpoint{
		PlanDigest:   plan.Digest,
		Patches:      []sandbox.Patch{{Path: "value.js", Content: []byte("exports.add = (a,b) => a+b\n")}},
		Dependencies: []DependencyUpdate{{Ecosystem: "npm", Directory: ".", Package: "fixture", Version: "1.2.3"}},
		Recent:       "last saved owner action",
		Turns:        5,
	}
	previous := Run{
		State:      "handoff",
		Context:    current,
		Checkpoint: &checkpoint,
	}
	resumed := resumeCheckpoint(current, previous)
	if resumed == nil {
		t.Fatal("failed owner checkpoint was not resumed")
	}
	if resumed.Turns != 0 || resumed.Recent != checkpoint.Recent || len(resumed.Patches) != 1 || string(resumed.Patches[0].Content) != string(checkpoint.Patches[0].Content) || len(resumed.Dependencies) != 1 || resumed.Dependencies[0] != checkpoint.Dependencies[0] {
		t.Fatalf("resumed checkpoint lost saved progress or turn budget: %+v", resumed)
	}

	for _, tc := range []struct {
		name   string
		mutate func(*Run)
	}{
		{"target changed", func(run *Run) {
			run.Context.Plan.TargetSHA = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
			run.Context.Plan.Digest = planDigest(run.Context.Plan)
		}},
		{"recipe changed", func(run *Run) {
			run.Context.Plan.Recipe.Name = "python"
			run.Context.Plan.Digest = planDigest(run.Context.Plan)
		}},
		{"published branch", func(run *Run) { run.Branch = "reforge/repair/fixture" }},
		{"external outcome uncertain", func(run *Run) { run.CandidateSHA = "candidate-sha" }},
		{"external change exists", func(run *Run) { run.Change = &forge.Change{} }},
		{"candidate checks exist", func(run *Run) { run.CandidateChecks = []CheckResult{{CommandID: "check", Complete: true}} }},
		{"candidate artifacts exist", func(run *Run) { run.CandidateArtifacts = []string{"artifact"} }},
		{"invalid checkpoint", func(run *Run) { run.Checkpoint.Patches = []sandbox.Patch{{Path: ".env", Content: []byte("secret")}} }},
		{"unsupported repair state", func(run *Run) { run.State = "failed" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := previous
			candidate.Checkpoint = &Checkpoint{PlanDigest: checkpoint.PlanDigest, Patches: append([]sandbox.Patch(nil), checkpoint.Patches...), Dependencies: append([]DependencyUpdate(nil), checkpoint.Dependencies...), Recent: checkpoint.Recent, Turns: checkpoint.Turns}
			tc.mutate(&candidate)
			if got := resumeCheckpoint(current, candidate); got != nil {
				t.Fatalf("unsafe checkpoint resumed: %+v", got)
			}
		})
	}
}

func TestCheckpointProgressDigestIgnoresNonProgressMetadata(t *testing.T) {
	checkpoint := Checkpoint{PlanDigest: "plan", Patches: []sandbox.Patch{{Path: "value.js", Content: []byte("one")}}, Turns: 2, Recent: "first"}
	digest := checkpoint.ProgressDigest()
	checkpoint.Turns = 7
	checkpoint.Recent = "different continuation summary"
	if checkpoint.ProgressDigest() != digest {
		t.Fatal("turn count or recent summary changed progress digest")
	}
	checkpoint.Patches[0].Content = []byte("two")
	if checkpoint.ProgressDigest() == digest {
		t.Fatal("changed staged patch did not change progress digest")
	}
}
