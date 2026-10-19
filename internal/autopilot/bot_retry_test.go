package autopilot

import (
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/mergecontrol"
)

func TestBotMergeRetryChecksPendingCIWithinOneMinute(t *testing.T) {
	gate := mergecontrol.Gate{
		Snapshot: mergecontrol.Snapshot{
			TargetChecks: []forge.Check{{Name: "Validate", Conclusion: "success"}},
			Checks:       []forge.Check{{Name: "Validate"}},
		},
	}
	gate.Decision.Blockers = []string{"requirement not proven: native_checks"}
	reason, delay := botMergeRetry(gate)
	if reason != "Waiting for merge checks" || delay != time.Minute {
		t.Fatalf("retry=%q after %s", reason, delay)
	}
	gate.Snapshot.Checks[0].Conclusion = "failure"
	reason, delay = botMergeRetry(gate)
	if reason != "requirement not proven: native_checks" || delay != 10*time.Minute {
		t.Fatalf("blocked retry=%q after %s", reason, delay)
	}
}
