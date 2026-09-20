package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

type nativeFixture struct {
	mode      string
	mutations int
	guards    int
	merged    bool
}

func (f *nativeFixture) call(t *testing.T, req *http.Request) (*http.Response, error) {
	t.Helper()
	path := strings.TrimPrefix(req.URL.Path, "/api/v3")
	switch path {
	case "/repos/acme/repo":
		return jsonResponse(200, map[string]any{"id": 1, "full_name": "acme/repo", "permissions": map[string]bool{"push": true, "admin": false}, "allow_merge_commit": true, "allow_squash_merge": true, "allow_rebase_merge": true}), nil
	case "/repos/acme/repo/branches/main":
		return jsonResponse(200, map[string]bool{"protected": true}), nil
	case "/repos/acme/repo/branches/main/protection":
		app := any(42)
		if f.mode == "unbound_check" {
			app = nil
		}
		strict := f.mode != "loose"
		return jsonResponse(200, map[string]any{"enforce_admins": map[string]bool{"enabled": true}, "required_status_checks": map[string]any{"strict": strict, "contexts": []string{"reforge/gate"}, "checks": []any{map[string]any{"context": "reforge/gate", "app_id": app}}}, "required_pull_request_reviews": map[string]any{"required_approving_review_count": 1, "dismiss_stale_reviews": true, "require_code_owner_reviews": false, "bypass_pull_request_allowances": map[string]any{"apps": []any{}, "users": []any{}, "teams": []any{}}}}), nil
	case "/repos/acme/repo/rules/branches/main":
		if f.mode == "partial_pagination" {
			if req.URL.Query().Get("page") == "2" {
				return jsonResponse(403, nil), nil
			}
			response := jsonResponse(200, []any{})
			response.Header.Set("Link", `<https://github.example/api/v3/repos/acme/repo/rules/branches/main?page=2>; rel="next"`)
			return response, nil
		}
		if f.mode == "null_rules" {
			return jsonResponse(200, nil), nil
		}
		rules := []any{map[string]any{"type": "non_fast_forward", "ruleset_id": 7}}
		if f.mode == "unknown_rule" {
			rules = append(rules, map[string]any{"type": "future_unknown", "ruleset_id": 7})
		}
		if strings.HasPrefix(f.mode, "queue") {
			rules = append(rules, map[string]any{"type": "merge_queue", "ruleset_id": 7, "parameters": map[string]any{"merge_method": "SQUASH", "grouping_strategy": "ALLGREEN", "max_entries_to_merge": 1}})
		}
		if f.mode == "partial_rule" {
			rules = append(rules, map[string]any{"type": "pull_request", "ruleset_id": 7, "parameters": map[string]any{"required_approving_review_count": 0}})
		}
		return jsonResponse(200, rules), nil
	case "/repos/acme/repo/rulesets/7":
		out := map[string]any{"id": 7, "enforcement": "active", "bypass_actors": []any{}}
		if f.mode == "hidden_bypass" {
			delete(out, "bypass_actors")
		}
		if f.mode == "app_bypass" {
			out["bypass_actors"] = []any{map[string]any{"actor_type": "Integration", "actor_id": 42, "bypass_mode": "pull_request"}}
		}
		return jsonResponse(200, out), nil
	case "/repos/acme/repo/pulls/7":
		pull := pullFixture(2, 7)
		if strings.Contains(f.mode, "retarget") && f.guards > 0 {
			pull["base"].(map[string]any)["ref"] = "other"
		}
		if f.merged {
			pull["merged"] = true
			pull["state"] = "closed"
			pull["merge_commit_sha"] = testCommit
		}
		return jsonResponse(200, pull), nil
	case "/repos/acme/repo/git/ref/heads/main", "/repos/acme/repo/git/ref/heads/other":
		sha := testBase
		if f.mode == "target_drift" && f.guards > 0 {
			sha = testCommit
		}
		return jsonResponse(200, map[string]any{"object": map[string]string{"sha": sha}}), nil
	case "/repos/acme/repo/git/ref/heads/reforge/fix":
		return jsonResponse(200, map[string]any{"object": map[string]string{"sha": testHead}}), nil
	case "/repos/acme/repo/commits/" + testHead + "/check-runs":
		app := 42
		if f.mode == "wrong_publisher" {
			app = 99
		}
		check := func(id int, conclusion string) any {
			return map[string]any{"id": id, "name": "reforge/gate", "head_sha": testHead, "status": "completed", "conclusion": conclusion, "app": map[string]int{"id": app}}
		}
		rows := []any{check(1, "success")}
		if f.mode == "newer_failure" {
			rows = append(rows, check(2, "failure"))
		}
		return jsonResponse(200, map[string]any{"check_runs": rows}), nil
	case "/repos/acme/repo/pulls/7/reviews":
		sha := testHead
		if f.mode == "stale_approval" {
			sha = testBase
		}
		return jsonResponse(200, []any{map[string]any{"id": 1, "user": map[string]int{"id": 10}, "state": "APPROVED", "commit_id": sha}}), nil
	case "/repos/acme/repo/pulls/7/merge":
		f.mutations++
		var payload map[string]any
		_ = json.NewDecoder(req.Body).Decode(&payload)
		if req.Method != "PUT" || payload["sha"] != testHead || payload["merge_method"] != "squash" || len(payload) != 2 || f.guards != 2 {
			t.Errorf("unguarded native mutation %v guards=%d", payload, f.guards)
		}
		f.merged = true
		return jsonResponse(200, map[string]any{"merged": true, "sha": testCommit}), nil
	case "/api/graphql":
		var body struct {
			Query     string                     `json:"query"`
			Variables map[string]json.RawMessage `json:"variables"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		if strings.HasPrefix(body.Query, "query") {
			base := testBase
			if f.mode == "queue_base_drift" {
				base = testCommit
			}
			var entry any
			if f.mode == "queue_observe" {
				entry = map[string]any{"id": "Q1", "state": "AWAITING_CHECKS", "baseCommit": map[string]string{"oid": testBase}}
			}
			return jsonResponse(200, map[string]any{"data": map[string]any{"repository": map[string]any{"databaseId": 1, "pullRequest": map[string]any{"id": "PR1", "headRefOid": testHead, "baseRefOid": base, "mergeQueueEntry": entry}}}}), nil
		}
		f.mutations++
		var input map[string]any
		_ = json.Unmarshal(body.Variables["input"], &input)
		if input["pullRequestId"] != "PR1" || input["expectedHeadOid"] != testHead || input["jump"] != false || f.guards != 2 {
			t.Errorf("unsafe queue input %v guards=%d", input, f.guards)
		}
		return jsonResponse(200, map[string]any{"data": map[string]any{"enqueuePullRequest": map[string]any{"mergeQueueEntry": map[string]string{"id": "Q1"}}}}), nil
	default:
		t.Errorf("unexpected native request %s", path)
		return jsonResponse(404, nil), nil
	}
}
func TestProtectionAndEligibilityFailClosed(t *testing.T) {
	for _, mode := range []string{"eligible", "hidden_bypass", "app_bypass", "unknown_rule", "partial_rule", "partial_pagination", "null_rules", "unbound_check", "wrong_publisher", "newer_failure", "stale_approval", "loose"} {
		t.Run(mode, func(t *testing.T) {
			f := &nativeFixture{mode: mode}
			p := fixtureProvider(t, func(req *http.Request) (*http.Response, error) { return f.call(t, req) }).WithMergeGuard(func(context.Context, forge.MergeRequest, forge.Change, forge.Rules) error { return nil })
			rules, err := p.ReadEffectiveRules(context.Background(), testRepo, "main")
			if mode == "partial_rule" || mode == "partial_pagination" || mode == "null_rules" {
				if err == nil {
					t.Fatal("partial rule accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "hidden_bypass" || mode == "unknown_rule" || mode == "unbound_check" {
				if rules.State != domain.Unknown {
					t.Fatalf("unknown rule became supported: %+v", rules)
				}
			}
			if mode == "app_bypass" && !rules.ActorCanBypass {
				t.Fatal("App bypass missed")
			}
			eligibility, err := p.EvaluateNativeEligibility(context.Background(), testRepo, "7")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "eligible" {
				if eligibility.State != "eligible" {
					t.Fatalf("unexpected blockers %+v", eligibility)
				}
			} else if eligibility.State != "blocked" {
				t.Fatalf("unsafe eligibility %+v", eligibility)
			}
		})
	}
}
func TestNativeMergeAndQueueRequireFreshAuthorizedEvidence(t *testing.T) {
	for _, mode := range []string{"merge", "target_drift", "revoked", "retarget", "queue", "queue_retarget", "queue_base_drift"} {
		t.Run(mode, func(t *testing.T) {
			f := &nativeFixture{mode: mode}
			p := fixtureProvider(t, func(req *http.Request) (*http.Response, error) { return f.call(t, req) }).WithMergeGuard(func(_ context.Context, in forge.MergeRequest, change forge.Change, rules forge.Rules) error {
				f.guards++
				if in.ExpectedHeadSHA != change.HeadSHA || in.ExpectedTargetSHA != change.TargetSHA || in.RulesHash != rules.Hash {
					t.Error("guard evidence differs from intent")
				}
				if mode == "revoked" && f.guards == 2 {
					return errors.New("revoked")
				}
				return nil
			})
			rules, err := p.ReadEffectiveRules(context.Background(), testRepo, "main")
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.RequestNativeMergeOrQueue(context.Background(), forge.MergeRequest{Repository: testRepo, ChangeID: "7", ExpectedHeadSHA: testHead, ExpectedTargetSHA: testBase, RulesHash: rules.Hash, GateID: "persisted-gate", Method: "squash", OperationID: "op-1", Queue: strings.HasPrefix(mode, "queue")})
			if mode == "merge" || mode == "queue" {
				if err != nil || f.mutations != 1 {
					t.Fatalf("native result %+v %v mutations=%d", result, err, f.mutations)
				}
				if mode == "merge" && result.State != "merged" || mode == "queue" && (result.State != "queued" || result.NativeID != "Q1") {
					t.Fatalf("result %+v", result)
				}
			} else if err == nil || f.mutations != 0 {
				t.Fatalf("unsafe mutation %+v %v mutations=%d", result, err, f.mutations)
			}
		})
	}
}
func TestQueueObservationDoesNotInventTestedCommit(t *testing.T) {
	f := &nativeFixture{mode: "queue_observe"}
	p := fixtureProvider(t, func(req *http.Request) (*http.Response, error) { return f.call(t, req) })
	queue, err := p.ReadQueueState(context.Background(), testRepo, "7")
	if err != nil || queue.ID != "Q1" || queue.HeadSHA != testHead || queue.TargetSHA != testBase || queue.TestedSHA != "" {
		t.Fatalf("queue %+v %v", queue, err)
	}
}
