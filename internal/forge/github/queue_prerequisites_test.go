package github

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/forge"
)

func TestQueuePrerequisitesExcludeOnlyRequiredExecutionCheck(t *testing.T) {
	for _, mode := range []string{"eligible", "missing_gate", "wrong_publisher", "no_queue", "failed_other_check", "stale_review", "unknown_state", "grouped_queue"} {
		t.Run(mode, func(t *testing.T) {
			fixture := &nativeFixture{mode: "queue"}
			if mode == "no_queue" {
				fixture.mode = "eligible"
			}
			provider := fixtureProvider(t, func(request *http.Request) (*http.Response, error) {
				response, err := fixture.call(t, request)
				if err != nil {
					return response, err
				}
				var body any
				if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
					return nil, err
				}
				response.Body.Close()
				switch strings.TrimPrefix(request.URL.Path, "/api/v3") {
				case "/repos/acme/repo/branches/main/protection":
					if mode != "missing_gate" {
						status := body.(map[string]any)["required_status_checks"].(map[string]any)
						publisher := 42
						if mode == "wrong_publisher" {
							publisher = 99
						}
						status["checks"] = append(status["checks"].([]any), map[string]any{"context": forge.QueueExecutionCheckName, "app_id": publisher})
						status["contexts"] = append(status["contexts"].([]any), forge.QueueExecutionCheckName)
					}
				case "/repos/acme/repo/commits/" + testHead + "/check-runs":
					if mode == "failed_other_check" {
						body.(map[string]any)["check_runs"].([]any)[0].(map[string]any)["conclusion"] = "failure"
					}
				case "/repos/acme/repo/pulls/7/reviews":
					if mode == "stale_review" {
						body.([]any)[0].(map[string]any)["commit_id"] = testBase
					}
				case "/repos/acme/repo/pulls/7":
					if mode == "unknown_state" {
						body.(map[string]any)["mergeable_state"] = "unknown"
					}
				case "/repos/acme/repo/rules/branches/main":
					if mode == "grouped_queue" {
						body.([]any)[1].(map[string]any)["parameters"].(map[string]any)["max_entries_to_merge"] = 2
					}
				}
				return jsonResponse(response.StatusCode, body), nil
			}).WithMergeGuard(func(context.Context, forge.MergeRequest, forge.Change, forge.Rules) error { return nil })
			state, requirement, err := provider.EvaluateQueuePrerequisites(context.Background(), testRepo, "7")
			if err != nil || requirement != (forge.CheckRule{Name: forge.QueueExecutionCheckName, PublisherID: "42"}) {
				t.Fatalf("identity: %+v %v", requirement, err)
			}
			if (state.State == "eligible") != (mode == "eligible") {
				t.Fatalf("unsafe prerequisite decision: %+v", state)
			}
			if mode == "eligible" {
				ordinary, err := provider.EvaluateNativeEligibility(context.Background(), testRepo, "7")
				if err != nil || ordinary.State == "eligible" {
					t.Fatalf("ordinary merge ignored missing execution check: %+v %v", ordinary, err)
				}
			}
			if fixture.mutations != 0 {
				t.Fatal("prerequisite inspection mutated provider")
			}
		})
	}
}
