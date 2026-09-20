package mergecontrol

import (
	"reforge/internal/forge"
	"strings"
	"testing"
	"time"
)

func TestTrainExecutionRequiresExactImmutableBlockingJob(t *testing.T) {
	for _, mode := range []string{"ready", "changed_config", "wrong_sha", "wrong_queue", "missing_pipeline", "released", "ci_pending"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Now().UTC()
			snapshot, resolved, authority := evaluationFixture(now)
			snapshot.Capabilities.Provider = "gitlab"
			snapshot.Rules.RequireQueue = true
			snapshot.ExecutionCheck = forge.CheckRule{Name: forge.QueueExecutionCheckName, PublisherID: "actor"}
			snapshot.Rules.RequiredChecks = []forge.CheckRule{snapshot.ExecutionCheck}
			authority.ExecutionPublisher = "actor"
			authority.CIConfigSHA256 = strings.Repeat("a", 64)
			snapshot.Queue = forge.QueueState{ID: "queue", State: "fresh", HeadSHA: snapshot.Change.HeadSHA, TargetSHA: snapshot.Change.TargetSHA, TestedSHA: strings.Repeat("c", 40)}
			snapshot.TrainGate = &forge.TrainGate{QueueID: "queue", PipelineID: "10", JobID: "11", HeadSHA: snapshot.Change.HeadSHA, TargetSHA: snapshot.Change.TargetSHA, SHA: snapshot.Queue.TestedSHA, CIConfigSHA256: authority.CIConfigSHA256, Name: forge.QueueExecutionCheckName, PublisherID: "actor", State: "manual", ChecksReady: true}
			switch mode {
			case "changed_config":
				snapshot.TrainGate.CIConfigSHA256 = strings.Repeat("b", 64)
			case "wrong_sha":
				snapshot.TrainGate.SHA = snapshot.Change.HeadSHA
			case "wrong_queue":
				snapshot.TrainGate.QueueID = "other"
			case "missing_pipeline":
				snapshot.TrainGate.PipelineID = ""
			case "released":
				snapshot.TrainGate.State = "pending"
			case "ci_pending":
				snapshot.TrainGate.ChecksReady = false
			}
			gate := Evaluate(snapshot, resolved, "squash", authority, now)
			if (gate.Decision.Outcome == "allow") != (mode == "ready") {
				t.Fatalf("unsafe train gate: %+v", gate.Decision)
			}
		})
	}
}
