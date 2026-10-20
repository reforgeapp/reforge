package mergecontrol

import (
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

func TestPendingChecksBlockMergeWithoutTriggeringRecreation(t *testing.T) {
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	for _, status := range []string{"queued", "in_progress", "running", "pending"} {
		t.Run(status, func(t *testing.T) {
			snapshot, resolved, authority := evaluationFixture(now)
			head := snapshot.Change.HeadSHA
			snapshot.Rules = forge.Rules{State: domain.Supported, Hash: snapshot.Rules.Hash, AllowedMergeMethods: []string{"squash"}, ActorCanBypass: true, Unprotected: true}
			snapshot.Checks = []forge.Check{{Name: "CI", HeadSHA: head, Status: status}}
			snapshot.TargetChecks = []forge.Check{{Name: "CI", Conclusion: "success"}}
			snapshot.UpToDate = true
			authority.ReforgeEnforced = true
			if StaleChecks(snapshot.Checks, head, now) {
				t.Fatal("pending check incorrectly requests bot recreation")
			}
			if gate := Evaluate(snapshot, resolved, "squash", authority, now); gate.Decision.Outcome == "allow" {
				t.Fatal("merge allowed while native check is pending")
			}
		})
	}
}

func TestStaleChecksStillRejectsOldOrUndatedTerminalEvidence(t *testing.T) {
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	head := strings.Repeat("a", 40)
	old := now.Add(-CheckFreshness - time.Second)
	for name, check := range map[string]forge.Check{
		"old completed":    {Name: "CI", HeadSHA: head, Status: "completed", Conclusion: "success", CompletedAt: &old},
		"undated terminal": {Name: "CI", HeadSHA: head, Status: "completed", Conclusion: "success"},
	} {
		t.Run(name, func(t *testing.T) {
			if !StaleChecks([]forge.Check{check}, head, now) {
				t.Fatal("stale terminal check was accepted")
			}
		})
	}
}
