package mergecontrol

import (
	"testing"
	"time"
)

func TestNativeStatusCannotProveUnverifiedProposalBehaviour(t *testing.T) {
	now := time.Now().UTC()
	snapshot, resolved, authority := evaluationFixture(now)
	authority.BehaviourRequired = true
	authority.ValidationReference = ""
	if gate := Evaluate(snapshot, resolved, "squash", authority, now); gate.Decision.Outcome == "allow" {
		t.Fatal("successful status alone verified behaviour")
	}
	authority.ValidationReference = "independent-counted-ci-proof"
	if gate := Evaluate(snapshot, resolved, "squash", authority, now); gate.Decision.Outcome != "allow" {
		t.Fatalf("exact-head behavioural proof blocked: %+v", gate.Decision)
	}
	authority.ValidationTarget = "old-target"
	if gate := Evaluate(snapshot, resolved, "squash", authority, now); gate.Decision.Outcome == "allow" {
		t.Fatal("stale target proof accepted")
	}
}
