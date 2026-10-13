package policy

import (
	"slices"
	"testing"
)

func TestQualifiedProposalCannotBypassMergeOrExplicitValidation(t *testing.T) {
	layers := baseLayers()
	layers[0].Policy.Defaults.BranchPrefix = "reforge/"
	resolved := Resolve(layers, "repo", "", false)
	in := trustedInput(resolved, Publish)
	in.Stage = "proposal_publication"
	in.Evidence = slices.DeleteFunc(in.Evidence, func(e Evidence) bool { return e.ID == "validation" })
	in.Evidence = append(in.Evidence, Evidence{ID: "proposal_bounds", State: "satisfied", Binding: in.Current, ObservedAt: in.Now, Reference: "bounded-unverified-proposal"})
	if result := Evaluate(resolved, in); result.Outcome != "allow" {
		t.Fatalf("bounded proposal blocked: %+v", result)
	}
	in.Action = Merge
	if result := Evaluate(resolved, in); result.Outcome == "allow" {
		t.Fatal("proposal authority bypassed merge validation")
	}
	in.Action = Publish
	layers[0].Policy.Required = []Requirement{{ID: "validation", Actions: []Action{Publish}}}
	resolved = Resolve(layers, "repo", "", false)
	in.Current.PolicyHash = resolved.Hash
	for i := range in.Evidence {
		in.Evidence[i].Binding.PolicyHash = resolved.Hash
	}
	if result := Evaluate(resolved, in); result.Outcome == "allow" {
		t.Fatal("explicit validation requirement bypassed")
	}
}
