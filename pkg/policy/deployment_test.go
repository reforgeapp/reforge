package policy

import "testing"

func TestDeploymentAdmissionDoesNotSubstituteNativeApproval(t *testing.T) {
	resolved := Resolve(baseLayers(), "repo", "", false)
	in := trustedInput(resolved, Deploy)
	changeEvidence(&in, "native_approvals", func(e *Evidence) { e.State = "pending" })
	if got := Evaluate(resolved, in); got.Outcome == "allow" {
		t.Fatal("deployment execution admitted without native approval")
	}
	in.Stage = "deployment_admission"
	if got := Evaluate(resolved, in); got.Outcome == "allow" {
		t.Fatal("workflow admission lacks native enforcement")
	}
	in.Evidence = append(in.Evidence, Evidence{ID: "native_enforcement", State: "satisfied", Binding: in.Current, ObservedAt: in.Now, Reference: "protected-environment"})
	if got := Evaluate(resolved, in); got.Outcome != "allow" {
		t.Fatalf("native-enforced admission: %+v", got)
	}
	layers := baseLayers()
	layers[0].Policy.Required = []Requirement{{ID: "native_approvals", Actions: []Action{Deploy}}}
	strict := Resolve(layers, "repo", "", false)
	in.Current.PolicyHash = strict.Hash
	for i := range in.Evidence {
		in.Evidence[i].Binding = in.Current
	}
	if got := Evaluate(strict, in); got.Outcome == "allow" {
		t.Fatal("explicit ancestor preapproval was weakened")
	}
	in.Stage = "deployment_execution_bypass"
	if got := Evaluate(resolved, in); got.Outcome != "deny" {
		t.Fatal("unknown stage accepted")
	}
}
