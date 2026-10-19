package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

const testHead = "1111111111111111111111111111111111111111"
const testBase = "2222222222222222222222222222222222222222"
const testCommit = "3333333333333333333333333333333333333333"

var testRepo = forge.RepoRef{NativeID: "17", FullName: "group/repo"}

func jsonResponse(status int, value any) *http.Response {
	raw, _ := json.Marshal(value)
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}
}
func fixtureProvider(t *testing.T, fn roundTripFunc) *Provider {
	t.Helper()
	p, err := New(forge.Config{OrgID: "org", ConnectionID: "connection", BaseURL: "https://gitlab.example", Token: "operational", Client: fn})
	if err != nil {
		t.Fatal(err)
	}
	return authorizedFixture(p)
}
func TestFreshBranchPublicationVerifiesWholeTreeBeforeUniqueRef(t *testing.T) {
	for _, mode := range []string{"publish", "wrong_parent", "wrong_tree", "revoked", "collision", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			guards, mutations, finalAttempts := 0, 0, 0
			p := fixtureProvider(t, func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/api/v4/user":
					return jsonResponse(200, map[string]any{"id": 81}), nil
				case "/api/v4/projects/17":
					return jsonResponse(200, map[string]any{"id": 17, "path_with_namespace": "group/repo"}), nil
				case "/api/v4/projects/17/repository/tree":
					entry := treeEntry{ID: blobSHA([]byte("before")), Type: "blob", Mode: "100644", Path: "a.txt"}
					if mode == "symlink" {
						entry.Mode = "120000"
					}
					if req.URL.Query().Get("ref") == testCommit {
						entry.ID = blobSHA([]byte("after"))
					}
					rows := []treeEntry{entry}
					if mode == "wrong_tree" && req.URL.Query().Get("ref") == testCommit {
						rows = append(rows, treeEntry{ID: testHead, Type: "blob", Mode: "100644", Path: "unexpected"})
					}
					return jsonResponse(200, rows), nil
				case "/api/v4/projects/17/repository/branches":
					mutations++
					var body map[string]string
					_ = json.NewDecoder(req.Body).Decode(&body)
					if body["branch"] == "reforge/fix" {
						finalAttempts++
						if guards != 3 || body["ref"] != testCommit {
							t.Errorf("unverified final publication %v guards=%d", body, guards)
						}
						if mode == "collision" {
							return jsonResponse(400, map[string]string{"message": "already exists"}), nil
						}
					} else if !strings.HasPrefix(body["branch"], "reforge/staging/") || body["ref"] != testBase || guards != 1 {
						t.Errorf("unsafe staging ref %v", body)
					}
					return jsonResponse(201, map[string]any{"name": body["branch"], "commit": map[string]string{"id": body["ref"]}}), nil
				case "/api/v4/projects/17/repository/commits":
					mutations++
					var body map[string]any
					_ = json.NewDecoder(req.Body).Decode(&body)
					if guards != 2 || body["force"] != false || !strings.HasPrefix(body["branch"].(string), "reforge/staging/") {
						t.Errorf("unsafe staging mutation %v", body)
					}
					parent := testBase
					if mode == "wrong_parent" {
						parent = testHead
					}
					return jsonResponse(201, map[string]any{"id": testCommit, "parent_ids": []string{parent}}), nil
				case "/api/v4/projects/17/repository/branches/reforge/fix":
					return jsonResponse(200, map[string]any{"commit": map[string]string{"id": testCommit}}), nil
				default:
					t.Errorf("unexpected path %s", req.URL.Path)
					return jsonResponse(404, nil), nil
				}
			}).WithBranchAuthorizer(func(context.Context, forge.UpdateBranchRequest) error {
				guards++
				if mode == "revoked" && guards == 3 {
					return errors.New("revoked")
				}
				return nil
			})
			got, err := p.UpdateAppBranch(context.Background(), forge.UpdateBranchRequest{Repository: testRepo, Branch: "reforge/fix", BaseSHA: testBase, Message: "Repair", OperationID: "op:1", Edits: []forge.FileEdit{{Path: "a.txt", Content: []byte("after")}}})
			if mode == "publish" {
				if err != nil || got != testCommit || mutations != 3 {
					t.Fatalf("publication %s %v mutations=%d", got, err, mutations)
				}
			} else if err == nil {
				t.Fatal("unsafe branch publication succeeded")
			}
			if mode != "publish" && mode != "collision" && finalAttempts != 0 {
				t.Fatal("unverified final ref published")
			}
		})
	}
}

type nativeFixture struct {
	mode              string
	guards, mutations int
	merged            bool
}

func (f *nativeFixture) call(t *testing.T, req *http.Request) (*http.Response, error) {
	t.Helper()
	path := strings.TrimPrefix(req.URL.Path, "/api/v4")
	switch path {
	case "/user":
		return jsonResponse(200, map[string]any{"id": 81, "is_admin": false}), nil
	case "/projects/17":
		train := strings.HasPrefix(f.mode, "train")
		method := "ff"
		if train {
			method = "merge"
		}
		auto := f.mode == "automatic_rebase"
		out := map[string]any{"id": 17, "path_with_namespace": "group/repo", "merge_method": method, "squash_option": "default_off", "only_allow_merge_if_pipeline_succeeds": true, "allow_merge_on_skipped_pipeline": false, "only_allow_merge_if_all_discussions_are_resolved": true, "merge_trains_enabled": train, "merge_pipelines_enabled": train, "merge_trains_skip_train_allowed": false, "merge_train_enforcement": "enforce_for_all_users", "automatic_rebase_enabled": auto, "only_allow_merge_if_all_status_checks_passed": false}
		if f.mode == "rebase_unknown" {
			delete(out, "automatic_rebase_enabled")
		}
		if f.mode == "train_bypass" {
			out["merge_trains_skip_train_allowed"] = true
		}
		return jsonResponse(200, out), nil
	case "/projects/17/members/all/81":
		level := 30
		if f.mode == "maintainer" {
			level = 40
		}
		return jsonResponse(200, map[string]int{"access_level": level}), nil
	case "/projects/17/repository/branches/main":
		sha := testBase
		if f.mode == "target_drift" && f.guards > 0 {
			sha = testCommit
		}
		return jsonResponse(200, map[string]any{"name": "main", "protected": true, "can_push": f.mode == "can_push", "commit": map[string]string{"id": sha}}), nil
	case "/projects/17/repository/branches/other":
		return jsonResponse(200, map[string]any{"name": "other", "commit": map[string]string{"id": testBase}}), nil
	case "/projects/17/repository/branches/reforge/fix":
		return jsonResponse(200, map[string]any{"commit": map[string]string{"id": testHead}}), nil
	case "/projects/17/protected_branches":
		if f.mode == "partial_rules" {
			if req.URL.Query().Get("page") == "2" {
				return jsonResponse(403, nil), nil
			}
			response := jsonResponse(200, []any{})
			response.Header.Set("X-Next-Page", "2")
			return response, nil
		}
		return jsonResponse(200, []any{map[string]any{"id": 1, "name": "*", "allow_force_push": false, "code_owner_approval_required": false}}), nil
	case "/projects/17/approvals":
		return jsonResponse(200, map[string]any{"reset_approvals_on_push": f.mode != "stale_approval", "disable_overriding_approvers_per_merge_request": true}), nil
	case "/projects/17/approval_rules":
		return jsonResponse(200, []any{map[string]any{"id": 1, "name": "review", "rule_type": "regular", "approvals_required": 1, "contains_hidden_groups": false, "protected_branches": []any{}}}), nil
	case "/projects/17/merge_requests/9":
		state := "opened"
		if f.merged {
			state = "merged"
		}
		status := "mergeable"
		if f.mode == "calculating" {
			status = "approvals_syncing"
		}
		sha := testHead
		if f.mode == "merged_result" {
			sha = testCommit
		}
		targetBranch := "main"
		if f.mode == "retargeted" && f.guards > 0 {
			targetBranch = "other"
		}
		return jsonResponse(200, map[string]any{"iid": 9, "project_id": 17, "source_project_id": 17, "target_project_id": 17, "source_branch": "reforge/fix", "target_branch": targetBranch, "sha": testHead, "state": state, "detailed_merge_status": status, "merge_commit_sha": testCommit, "head_pipeline": map[string]any{"id": 55, "project_id": 17, "sha": sha, "source": "merge_request_event", "status": "success"}}), nil
	case "/projects/17/merge_requests/9/approval_state":
		if f.mode == "missing_tier" {
			return jsonResponse(403, nil), nil
		}
		return jsonResponse(200, map[string]any{"approval_rules_overwritten": false, "rules": []any{map[string]any{"id": 1, "rule_type": "regular", "approvals_required": 1, "approved": f.mode != "unapproved", "contains_hidden_groups": f.mode == "hidden_group", "overridden": false}}}), nil
	case "/projects/17/repository/commits/" + testHead + "/statuses", "/projects/17/repository/commits/" + testCommit + "/statuses":
		actor := 81
		if f.mode == "wrong_publisher" {
			actor = 99
		}
		sha := testHead
		if f.mode == "merged_result" {
			sha = testCommit
		}
		row := func(id int, status string) any {
			return map[string]any{"id": id, "name": "reforge/gate", "sha": sha, "status": status, "author": map[string]int{"id": actor}}
		}
		rows := []any{row(1, "success")}
		if f.mode == "newer_failure" {
			rows = append(rows, row(2, "failed"))
		}
		return jsonResponse(200, rows), nil
	case "/projects/17/repository/commits/" + testCommit:
		return jsonResponse(200, map[string]any{"parent_ids": []string{testBase, testHead}}), nil
	case "/projects/17/merge_requests/9/merge", "/projects/17/merge_trains/merge_requests/9":
		if req.Method == "GET" {
			return jsonResponse(200, map[string]any{"id": 7, "status": "fresh", "target_branch": "main", "merge_request": map[string]int{"iid": 9, "project_id": 17}, "pipeline": map[string]any{"id": 56, "project_id": 17, "sha": testCommit, "source": "merge_request_event"}}), nil
		}
		f.mutations++
		var body map[string]any
		_ = json.NewDecoder(req.Body).Decode(&body)
		if body["sha"] != testHead || body["auto_merge"] != false || body["squash"] != false || f.guards != 2 {
			t.Errorf("unguarded merge mutation %v guards=%d", body, f.guards)
		}
		if strings.HasPrefix(f.mode, "train") {
			if !strings.Contains(path, "/merge_trains/") || req.Method != "POST" {
				t.Error("train used generic merge API")
			}
			return jsonResponse(201, map[string]any{"id": 7, "status": "idle", "target_branch": "main", "merge_request": map[string]int{"iid": 9, "project_id": 17}, "user": map[string]int{"id": 81}}), nil
		}
		f.merged = true
		return jsonResponse(200, map[string]any{"iid": 9, "state": "merged", "source_project_id": 17, "target_project_id": 17, "sha": testHead, "merge_commit_sha": testCommit}), nil
	default:
		t.Errorf("unexpected native path %s", path)
		return jsonResponse(404, nil), nil
	}
}
func guardedNative(t *testing.T, f *nativeFixture) *Provider {
	return fixtureProvider(t, func(req *http.Request) (*http.Response, error) { return f.call(t, req) }).WithCheckPublishers(map[string]string{"reforge/gate": "81"}).WithMergeGuard(func(_ context.Context, in forge.MergeRequest, change forge.Change, rules forge.Rules) error {
		f.guards++
		if in.ExpectedHeadSHA != change.HeadSHA || in.ExpectedTargetSHA != change.TargetSHA || in.RulesHash != rules.Hash {
			t.Error("guard evidence differs")
		}
		if f.mode == "revoked" && f.guards == 2 {
			return errors.New("revoked")
		}
		return nil
	})
}
func TestProtectionAndNativeEligibilityFailClosed(t *testing.T) {
	for _, mode := range []string{"eligible", "merged_result", "calculating", "automatic_rebase", "rebase_unknown", "train_bypass", "maintainer", "can_push", "unapproved", "stale_approval", "hidden_group", "wrong_publisher", "newer_failure", "missing_tier", "partial_rules"} {
		t.Run(mode, func(t *testing.T) {
			f := &nativeFixture{mode: mode}
			p := guardedNative(t, f)
			eligibility, err := p.EvaluateNativeEligibility(context.Background(), testRepo, "9")
			if mode == "missing_tier" || mode == "partial_rules" {
				if err == nil {
					t.Fatal("incomplete scope/tier evidence accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "eligible" || mode == "merged_result" {
				if eligibility.State != "eligible" {
					t.Fatalf("eligibility %+v", eligibility)
				}
			} else if eligibility.State != "blocked" {
				t.Fatalf("unsafe eligibility %+v", eligibility)
			}
		})
	}
}
func TestNativeMergeTrainAndRevocationBoundaries(t *testing.T) {
	for _, mode := range []string{"merge", "train", "target_drift", "retargeted", "revoked"} {
		t.Run(mode, func(t *testing.T) {
			f := &nativeFixture{mode: mode}
			p := guardedNative(t, f)
			rules, err := p.ReadEffectiveRules(context.Background(), testRepo, "main")
			if err != nil {
				t.Fatal(err)
			}
			method := "ff"
			if mode == "train" {
				method = "merge"
			}
			result, err := p.RequestNativeMergeOrQueue(context.Background(), forge.MergeRequest{Repository: testRepo, ChangeID: "9", ExpectedHeadSHA: testHead, ExpectedTargetSHA: testBase, RulesHash: rules.Hash, GateID: "persisted-gate", Method: method, OperationID: "op-1", Queue: mode == "train"})
			if mode == "merge" {
				if err != nil || f.mutations != 1 {
					t.Fatalf("result %+v err=%v writes=%d", result, err, f.mutations)
				}
				if mode == "merge" && result.State != "merged" || mode == "train" && result.State != "queued" {
					t.Fatalf("result %+v", result)
				}
			} else if err == nil || f.mutations != 0 {
				t.Fatalf("unsafe mutation %+v %v writes=%d", result, err, f.mutations)
			}
		})
	}
}
func TestTrainProvesExactHeadAndTargetParents(t *testing.T) {
	f := &nativeFixture{mode: "train"}
	p := guardedNative(t, f)
	state, err := p.ReadQueueState(context.Background(), testRepo, "9")
	if err != nil || state.TestedSHA != testCommit || state.HeadSHA != testHead || state.TargetSHA != testBase {
		t.Fatalf("queue %+v %v", state, err)
	}
}
func TestUnfencedWritesAndCrossTenantInspectionFailClosed(t *testing.T) {
	calls := 0
	p := fixtureProvider(t, func(*http.Request) (*http.Response, error) { calls++; return jsonResponse(500, nil), nil })
	p = p.WithBranchAuthorizer(nil).WithChangeAuthorizer(nil).WithReviewAuthorizer(nil)
	if _, err := p.UpdateAppBranch(context.Background(), forge.UpdateBranchRequest{}); err == nil {
		t.Fatal("unguarded branch")
	}
	if _, err := p.CreateChange(context.Background(), forge.CreateChangeRequest{}); err == nil {
		t.Fatal("unguarded MR")
	}
	if err := p.RequestReview(context.Background(), testRepo, "9", []string{"81"}); err == nil {
		t.Fatal("unguarded review")
	}
	if _, err := p.RequestNativeMergeOrQueue(context.Background(), forge.MergeRequest{}); err == nil {
		t.Fatal("unguarded merge")
	}
	if calls != 0 {
		t.Fatal("unfenced writes made requests")
	}
	other := *p
	other.config.OrgID = "other"
	if _, err := p.WithProtectionReader(&other); err == nil {
		t.Fatal("cross-tenant inspector")
	}
	p = p.WithBranchAuthorizer(func(context.Context, forge.UpdateBranchRequest) error { return nil })
	_, err := p.UpdateAppBranch(context.Background(), forge.UpdateBranchRequest{ExpectedOldSHA: testHead})
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "unsupported" || calls != 0 {
		t.Fatalf("uncertified CAS attempted %v", err)
	}
}

func TestProtectionInspectorRemainsReadOnlyAndKeepsOperationalActor(t *testing.T) {
	f := &nativeFixture{mode: "eligible"}
	inspected := 0
	operational := 0
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		path := req.URL.Path
		inspection := strings.HasSuffix(path, "/projects/17") || strings.Contains(path, "/protected_branches") || strings.HasSuffix(path, "/approvals") || strings.HasSuffix(path, "/approval_rules")
		if inspection {
			inspected++
			if req.Header.Get("PRIVATE-TOKEN") != "inspector" || req.Method != "GET" {
				t.Error("inspection authority used outside GET inspection")
			}
		} else {
			operational++
			if req.Header.Get("PRIVATE-TOKEN") != "operational" {
				t.Error("operational identity replaced by inspector")
			}
		}
		return f.call(t, req)
	})
	p := fixtureProvider(t, client)
	reader, err := New(forge.Config{OrgID: "org", ConnectionID: "inspection", BaseURL: "https://gitlab.example", Token: "inspector", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.WithProtectionReader(reader)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := p.ReadEffectiveRules(context.Background(), testRepo, "main")
	if err != nil || rules.ActorCanBypass || inspected < 3 || operational < 3 {
		t.Fatalf("inspection state %+v %v inspected=%d operational=%d", rules, err, inspected, operational)
	}
}
