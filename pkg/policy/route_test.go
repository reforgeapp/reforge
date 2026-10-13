package policy

import "testing"

func TestRouteNamesAndConnectionRestrictionsMatchExecution(t *testing.T) {
	layers := baseLayers()
	layers[0].Policy.Allow.Routes = []string{"default"}
	resolved := Resolve(layers, "repo", "", false)
	in := trustedInput(resolved, Repair)
	in.Model = "approved"
	in.Route = "approved-connection/default"
	if got := Evaluate(resolved, in); got.Outcome != "allow" {
		t.Fatalf("guide route cannot execute: %+v", got)
	}
	layers[0].Policy.Allow.Routes = []string{"approved-connection/default"}
	resolved = Resolve(layers, "repo", "", false)
	in.Current.PolicyHash = resolved.Hash
	for i := range in.Evidence {
		in.Evidence[i].Binding.PolicyHash = resolved.Hash
	}
	if got := Evaluate(resolved, in); got.Outcome != "allow" {
		t.Fatalf("explicit connection rejected: %+v", got)
	}
	in.Route = "other-connection/default"
	if got := Evaluate(resolved, in); got.Outcome == "allow" {
		t.Fatal("connection-specific approval bypassed")
	}
}
