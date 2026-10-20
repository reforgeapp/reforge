package mergecontrol

import (
	"strings"
	"testing"
)

func TestCompanionOrderingBindsCanonicalMergeAndCurrentRepair(t *testing.T) {
	head, merged := strings.Repeat("a", 40), strings.Repeat("b", 40)
	current := []Companion{{TaskID: "task", ChangeID: "7", HeadSHA: head, Branch: "reforge/repair/task", TargetBranch: "main", State: "published"}}
	proof := []Companion{{TaskID: "task", ChangeID: "7", HeadSHA: head, ObservedHeadSHA: head, Branch: "reforge/repair/task", TargetBranch: "main", MergeSHA: merged, State: "merged"}}
	if !companionsCurrent(current, proof) || !companionsCurrent(nil, nil) {
		t.Fatal("current canonical merge or absent dependency blocked")
	}
	for _, state := range []string{"open", "merge_pending", "repair_pending", "closed", "unknown"} {
		candidate := append([]Companion(nil), proof...)
		candidate[0].State = state
		if companionsCurrent(current, candidate) {
			t.Fatalf("nonmerged companion %s allowed original update", state)
		}
	}
	for _, mutate := range []func(*Companion){
		func(c *Companion) { c.TaskID = "replacement" },
		func(c *Companion) { c.ChangeID = "8" },
		func(c *Companion) { c.HeadSHA = strings.Repeat("c", 40) },
		func(c *Companion) { c.State = "publishing" },
	} {
		candidate := append([]Companion(nil), current...)
		mutate(&candidate[0])
		if companionsCurrent(candidate, proof) {
			t.Fatal("changed repair retained stale merge authority")
		}
	}
	if companionsCurrent(append(current, current[0]), proof) || companionsCurrent(current, nil) {
		t.Fatal("unobserved repair dependency allowed original update")
	}
}
