package mergecontrol

import (
	"strings"
	"testing"
	"time"

	"reforge/internal/forge"
)

func TestQueuePolicySeparatesAdmissionAndCandidateExecution(t *testing.T) {
	for _, scenario := range []string{"admission", "execution", "missing_ci", "wrong_ci_sha", "wrong_gate_publisher", "target_drift", "no_candidate", "candidate_is_head", "unqualified", "unknown_state", "unmergeable", "locked"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Now().UTC()
			snapshot, resolved, authority := evaluationFixture(now)
			snapshot.Capabilities.Provider = "github"
			snapshot.Rules.RequireQueue = true
			snapshot.ExecutionCheck = forge.CheckRule{Name: forge.QueueExecutionCheckName, PublisherID: "app"}
			snapshot.Rules.RequiredChecks = append(snapshot.Rules.RequiredChecks, snapshot.ExecutionCheck)
			authority.ExecutionPublisher = "app"
			snapshot.Queue = forge.QueueState{State: "not_queued", HeadSHA: snapshot.Change.HeadSHA, TargetSHA: snapshot.Change.TargetSHA}
			if scenario != "admission" {
				snapshot.Queue.ID, snapshot.Queue.State = "queue", "awaiting_checks"
				snapshot.Queue.TestedSHA = strings.Repeat("c", 40)
				snapshot.Checks[0].HeadSHA = snapshot.Queue.TestedSHA
			}
			switch scenario {
			case "missing_ci":
				snapshot.Checks = nil
			case "wrong_ci_sha":
				snapshot.Checks[0].HeadSHA = snapshot.Change.HeadSHA
			case "wrong_gate_publisher":
				snapshot.ExecutionCheck.PublisherID = "other-app"
			case "target_drift":
				snapshot.Queue.TargetSHA = strings.Repeat("f", 40)
			case "no_candidate":
				snapshot.Queue.TestedSHA = ""
			case "candidate_is_head":
				snapshot.Queue.TestedSHA = snapshot.Change.HeadSHA
				snapshot.Checks[0].HeadSHA = snapshot.Change.HeadSHA
			case "unqualified":
				authority.Qualified = false
			case "unknown_state", "unmergeable", "locked":
				snapshot.Queue.State = scenario
			}
			gate := Evaluate(snapshot, resolved, "squash", authority, now)
			want := scenario == "admission" || scenario == "execution"
			if (gate.Decision.Outcome == "allow") != want {
				t.Fatalf("decision %+v", gate.Decision)
			}
			if want && gate.Phase != "queue_"+scenario {
				t.Fatalf("phase %q", gate.Phase)
			}
		})
	}
}
