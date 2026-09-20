package gitea_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/forge/gitea"
)

type fixture struct {
	t       *testing.T
	base    string
	tokens  map[string]string
	repo    forge.RepoRef
	bot     *gitea.Provider
	client  *intercept
	initial string
}
type intercept struct {
	mu     sync.Mutex
	before func(*http.Request)
	client *http.Client
}

func (c *intercept) Do(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	f := c.before
	c.mu.Unlock()
	if f != nil {
		f(r)
	}
	return c.client.Do(r)
}
func (c *intercept) set(f func(*http.Request)) { c.mu.Lock(); defer c.mu.Unlock(); c.before = f }
func (f *fixture) call(actor, method, route string, in, out any) int {
	f.t.Helper()
	var b []byte
	if in != nil {
		b, _ = json.Marshal(in)
	}
	req, e := http.NewRequest(method, f.base+"/api/v1"+route, bytes.NewReader(b))
	if e != nil {
		f.t.Fatal(e)
	}
	req.Header.Set("Authorization", "token "+f.tokens[actor])
	req.Header.Set("Content-Type", "application/json")
	response, e := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if e != nil {
		f.t.Fatal(e)
	}
	defer response.Body.Close()
	body, e := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if e != nil {
		f.t.Fatal(e)
	}
	if out != nil && response.StatusCode < 300 && len(body) > 0 {
		if e := json.Unmarshal(body, out); e != nil {
			f.t.Fatalf("decode HTTP %d: %v", response.StatusCode, e)
		}
	}
	return response.StatusCode
}
func (f *fixture) ok(actor, method, route string, in, out any) {
	f.t.Helper()
	status := f.call(actor, method, route, in, out)
	if status < 200 || status >= 300 {
		f.t.Fatalf("%s %s HTTP %d", method, route, status)
	}
}
func (f *fixture) route() string { return "/repos/" + f.repo.FullName }
func newFixture(t *testing.T) *fixture {
	t.Helper()
	if os.Getenv("REFORGE_GITEA_TEST") != "1" {
		t.Skip("set REFORGE_GITEA_TEST=1 for disposable pinned local Gitea contracts")
	}
	root := os.Getenv("REFORGE_TEST_ROOT")
	if root == "" {
		t.Fatal("REFORGE_TEST_ROOT required")
	}
	f := &fixture{t: t, base: "http://127.0.0.1:53000", tokens: map[string]string{}, client: &intercept{client: &http.Client{Timeout: 15 * time.Second}}}
	for _, actor := range []string{"admin", "bot", "reviewer", "inspector"} {
		b, e := os.ReadFile(filepath.Join(root, ".local/gitea/reforge-"+actor+".token"))
		if e != nil {
			t.Fatal(e)
		}
		f.tokens[actor] = strings.TrimSpace(string(b))
	}
	var version struct{ Version string }
	f.ok("bot", "GET", "/version", nil, &version)
	if version.Version != "1.27.3" {
		t.Fatalf("test requires pinned Gitea 1.27.3, found %s", version.Version)
	}
	name := "t10-contract-" + domain.NewID()
	var repo struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	f.ok("admin", "POST", "/user/repos", map[string]any{"name": name, "default_branch": "main", "private": false, "auto_init": false}, &repo)
	f.repo = forge.RepoRef{NativeID: fmt.Sprint(repo.ID), FullName: repo.FullName}
	t.Cleanup(func() {
		status := f.call("admin", "DELETE", f.route(), nil, nil)
		if status != 204 {
			t.Errorf("disposable fixture cleanup HTTP %d", status)
		}
	})
	for _, actor := range []string{"bot", "reviewer"} {
		f.ok("admin", "PUT", f.route()+"/collaborators/reforge-"+actor, map[string]string{"permission": "write"}, nil)
	}
	var file struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	f.ok("admin", "POST", f.route()+"/contents/proof.txt", map[string]any{"branch": "main", "content": base64.StdEncoding.EncodeToString([]byte("baseline\n")), "message": "Initial fixture"}, &file)
	f.initial = file.Commit.SHA
	f.ok("admin", "PATCH", f.route(), map[string]any{"allow_fast_forward_only_merge": true}, nil)
	var e error
	f.bot, e = gitea.New(forge.Config{BaseURL: f.base, Client: f.client, Token: f.tokens["bot"]})
	if e != nil {
		t.Fatal(e)
	}
	inspector, e := gitea.New(forge.Config{BaseURL: f.base, Client: &http.Client{Timeout: 15 * time.Second}, Token: f.tokens["inspector"]})
	if e != nil {
		t.Fatal(e)
	}
	f.bot, e = f.bot.WithProtectionReader(inspector)
	if e != nil {
		t.Fatal(e)
	}
	f.bot = f.bot.WithBranchAuthorizer(func(_ context.Context, in forge.UpdateBranchRequest) error {
		if in.Repository != f.repo || !strings.HasPrefix(in.Branch, "reforge/") {
			return fmt.Errorf("unauthorized fixture branch")
		}
		return nil
	}).WithCheckPublishers(map[string]string{"ci/test": "3"})
	return f
}
func (f *fixture) protect(checks bool) {
	f.t.Helper()
	in := map[string]any{"rule_name": "main", "enable_push": true, "enable_push_whitelist": true, "push_whitelist_usernames": []string{"reforge-admin"}, "required_approvals": 1, "dismiss_stale_approvals": true, "block_on_outdated_branch": true, "block_on_rejected_reviews": true, "block_on_official_review_requests": true, "block_admin_merge_override": true, "enable_status_check": checks, "status_check_contexts": []string{"ci/test"}}
	f.ok("admin", "POST", f.route()+"/branch_protections", in, nil)
}
func (f *fixture) change(branch string) (forge.Change, string) {
	f.t.Helper()
	ctx := context.Background()
	op := domain.NewID()
	head, e := f.bot.UpdateAppBranch(ctx, forge.UpdateBranchRequest{Repository: f.repo, Branch: branch, BaseSHA: f.initial, Message: "Candidate fix", OperationID: op, Edits: []forge.FileEdit{{Path: "candidate.txt", Content: []byte("candidate\n")}}})
	if e != nil {
		f.t.Fatal(e)
	}
	v, e := f.bot.CreateChange(ctx, forge.CreateChangeRequest{Repository: f.repo, Title: "Candidate", Body: "Local contract fixture", HeadBranch: branch, TargetBranch: "main", ExpectedHeadSHA: head, OperationID: op})
	if e != nil {
		f.t.Fatal(e)
	}
	return v, head
}
func (f *fixture) approve(id, head string) {
	f.t.Helper()
	f.ok("reviewer", "POST", f.route()+"/pulls/"+id+"/reviews", map[string]any{"commit_id": head, "event": "APPROVED", "body": "Local contract approval"}, nil)
}
func (f *fixture) wait(id string) forge.Change {
	f.t.Helper()
	for n := 0; n < 30; n++ {
		v, e := f.bot.ReadChange(context.Background(), f.repo, id)
		if e == nil && v.MergeStatus == "mergeable" {
			return v
		}
		time.Sleep(100 * time.Millisecond)
	}
	f.t.Fatal("pull request never became mergeable")
	return forge.Change{}
}
func (f *fixture) mergeInput(id, head string) forge.MergeRequest {
	f.t.Helper()
	rules, e := f.bot.ReadEffectiveRules(context.Background(), f.repo, "main")
	if e != nil {
		f.t.Fatal(e)
	}
	target, e := f.bot.ResolveRef(context.Background(), f.repo, "main")
	if e != nil {
		f.t.Fatal(e)
	}
	return forge.MergeRequest{Repository: f.repo, ChangeID: id, ExpectedHeadSHA: head, ExpectedTargetSHA: target, RulesHash: rules.Hash, GateID: "fixture-gate", Method: "fast-forward-only", OperationID: domain.NewID()}
}
func TestGuardedUpdatesAndNativeMerge(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.protect(false)
	v, head := f.change("reforge/success")
	if e := f.bot.RequestReview(ctx, f.repo, v.ID, []string{"reforge-reviewer"}); e != nil {
		t.Fatal(e)
	}
	found, e := f.bot.FindChangeByOperation(ctx, f.repo, v.OperationID, v.HeadBranch, v.TargetBranch)
	if e != nil || found == nil || found.ID != v.ID {
		t.Fatalf("operation reconciliation failed: %v", e)
	}
	if found, e := f.bot.FindChangeByOperation(ctx, f.repo, v.OperationID, v.HeadBranch, "different-target"); e != nil || found != nil {
		t.Fatalf("operation adopted a different target: %v", e)
	}
	f.approve(v.ID, head)
	f.wait(v.ID)
	eligible, e := f.bot.EvaluateNativeEligibility(ctx, f.repo, v.ID)
	if e != nil || eligible.State != "eligible" {
		t.Fatalf("eligibility=%+v error=%v", eligible, e)
	}
	f.client.set(func(r *http.Request) {
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/merge") {
			b, e := io.ReadAll(r.Body)
			if e != nil {
				t.Fatal(e)
			}
			r.Body = io.NopCloser(bytes.NewReader(b))
			var body map[string]any
			json.Unmarshal(b, &body)
			if body["head_commit_id"] != head || body["force_merge"] != false || body["merge_when_checks_succeed"] != false {
				t.Errorf("unsafe native merge body")
			}
		}
	})
	merged, e := f.bot.RequestNativeMergeOrQueue(ctx, f.mergeInput(v.ID, head))
	if e != nil || merged.State != "merged" || merged.MergeSHA != head {
		t.Fatalf("merge=%+v error=%v", merged, e)
	}
	current, e := f.bot.ResolveRef(ctx, f.repo, "main")
	if e != nil || current != merged.MergeSHA {
		t.Fatalf("merge not on target: %v", e)
	}
}
func TestStaleHeadAndReview(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.protect(false)
	v, head := f.change("reforge/stale")
	f.approve(v.ID, head)
	f.wait(v.ID)
	in := f.mergeInput(v.ID, head)
	next, e := f.bot.UpdateAppBranch(ctx, forge.UpdateBranchRequest{Repository: f.repo, Branch: v.HeadBranch, BaseSHA: head, ExpectedOldSHA: head, Message: "New head", OperationID: domain.NewID(), Edits: []forge.FileEdit{{Path: "candidate.txt", Content: []byte("new candidate\n")}}})
	if e != nil {
		t.Fatal(e)
	}
	if next == head {
		t.Fatal("head unchanged")
	}
	f.wait(v.ID)
	if _, e := f.bot.RequestNativeMergeOrQueue(ctx, in); e == nil {
		t.Fatal("stale head merged")
	}
	eligible, e := f.bot.EvaluateNativeEligibility(ctx, f.repo, v.ID)
	if e != nil || eligible.State == "eligible" {
		t.Fatalf("stale approval eligible %+v %v", eligible, e)
	}
	status := f.call("bot", "POST", f.route()+"/pulls/"+v.ID+"/merge", map[string]any{"do": "merge", "head_commit_id": next, "force_merge": false}, nil)
	if status >= 200 && status < 300 {
		t.Fatal("native merge accepted stale approval")
	}
	status = f.call("bot", "POST", f.route()+"/pulls/"+v.ID+"/merge", map[string]any{"do": "merge", "head_commit_id": head, "force_merge": false}, nil)
	if status >= 200 && status < 300 {
		t.Fatal("native merge accepted stale head")
	}
}
func TestTargetMovesImmediatelyBeforeNativeMerge(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.protect(false)
	v, head := f.change("reforge/target-race")
	f.approve(v.ID, head)
	f.wait(v.ID)
	in := f.mergeInput(v.ID, head)
	in.Method = "fast-forward-only"
	triggered := false
	f.client.set(func(r *http.Request) {
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/merge") {
			if triggered {
				return
			}
			triggered = true
			f.ok("admin", "POST", f.route()+"/contents/target.txt", map[string]any{"branch": "main", "content": base64.StdEncoding.EncodeToString([]byte("target advanced\n")), "message": "Advance target"}, nil)
		}
	})
	if _, e := f.bot.RequestNativeMergeOrQueue(ctx, in); e == nil {
		t.Fatal("native merge accepted outdated branch after final preflight")
	}
	if !triggered {
		t.Fatal("test did not reach native merge boundary")
	}
	f.client.set(nil)
	result, e := f.bot.ReadMergeResult(ctx, f.repo, v.ID)
	if e != nil || result.State == "merged" {
		t.Fatalf("outdated pull merged %+v %v", result, e)
	}
}
func TestPublisherSpoofAndNativeRequiredChecks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.protect(true)
	v, head := f.change("reforge/checks")
	f.approve(v.ID, head)
	f.wait(v.ID)
	native := func() int {
		return f.call("bot", "POST", f.route()+"/pulls/"+v.ID+"/merge", map[string]any{"do": "merge", "head_commit_id": head, "force_merge": false}, nil)
	}
	if status := native(); status >= 200 && status < 300 {
		t.Fatal("native missing required check merged")
	}
	f.ok("reviewer", "POST", f.route()+"/statuses/"+head, map[string]string{"state": "failure", "context": "ci/test"}, nil)
	f.ok("bot", "POST", f.route()+"/statuses/"+head, map[string]string{"state": "success", "context": "ci/test"}, nil)
	checks, e := f.bot.ListChecks(ctx, f.repo, head)
	if e != nil {
		t.Fatal(e)
	}
	trustedFailure, spoofSuccess := false, false
	for _, check := range checks {
		trustedFailure = trustedFailure || check.PublisherID == "3" && check.Conclusion == "failure"
		spoofSuccess = spoofSuccess || check.PublisherID == "2" && check.Conclusion == "success"
	}
	if !trustedFailure || !spoofSuccess {
		t.Fatalf("publisher provenance lost %+v", checks)
	}
	rules, e := f.bot.ReadEffectiveRules(ctx, f.repo, "main")
	if e != nil || rules.State != domain.Unknown {
		t.Fatalf("publisher enforcement must be unknown %+v %v", rules, e)
	}
	if _, e := f.bot.RequestNativeMergeOrQueue(ctx, f.mergeInput(v.ID, head)); e == nil {
		t.Fatal("adapter accepted spoofed publisher")
	}
	status := native()
	if status < 200 || status >= 300 {
		t.Fatalf("expected native source-unbound same-name spoof proof, HTTP %d", status)
	}
}
func TestNativeCASRejectsMovedBranch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, head := f.change("reforge/cas")
	triggered := false
	f.client.set(func(r *http.Request) {
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/branches/") {
			if triggered {
				return
			}
			triggered = true
			f.ok("bot", "POST", f.route()+"/contents/racer.txt", map[string]any{"branch": "reforge/cas", "content": base64.StdEncoding.EncodeToString([]byte("racing change\n")), "message": "Race branch"}, nil)
		}
	})
	_, e := f.bot.UpdateAppBranch(ctx, forge.UpdateBranchRequest{Repository: f.repo, Branch: "reforge/cas", BaseSHA: head, ExpectedOldSHA: head, Message: "Candidate append", OperationID: domain.NewID(), Edits: []forge.FileEdit{{Path: "candidate.txt", Content: []byte("candidate revised\n")}}})
	if e == nil || !triggered {
		t.Fatalf("CAS race not rejected: reached=%v error=%v", triggered, e)
	}
	f.client.set(nil)
	actual, e := f.bot.ResolveRef(ctx, f.repo, "reforge/cas")
	if e != nil {
		t.Fatal(e)
	}
	file, e := f.bot.ReadFileAtRef(ctx, f.repo, "racer.txt", actual)
	if e != nil || string(file.Content) != "racing change\n" {
		t.Fatalf("racing commit lost: %v", e)
	}
}

func TestOrdinaryMergeDoesNotEnforceAtomicTargetFreshness(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.protect(false)
	v, head := f.change("reforge/native-limitation")
	f.approve(v.ID, head)
	f.wait(v.ID)
	rules, e := f.bot.ReadEffectiveRules(ctx, f.repo, "main")
	if e != nil {
		t.Fatal(e)
	}
	if len(rules.AllowedMergeMethods) != 1 || rules.AllowedMergeMethods[0] != "fast-forward-only" {
		t.Fatalf("unsafe automated methods: %v", rules.AllowedMergeMethods)
	}
	input := f.mergeInput(v.ID, head)
	input.Method = "merge"
	if _, e := f.bot.RequestNativeMergeOrQueue(ctx, input); e == nil {
		t.Fatal("adapter allowed ordinary merge")
	}
	f.ok("admin", "POST", f.route()+"/contents/target.txt", map[string]any{"branch": "main", "content": base64.StdEncoding.EncodeToString([]byte("target advanced\n")), "message": "Advance target"}, nil)
	target, e := f.bot.ResolveRef(ctx, f.repo, "main")
	if e != nil || target == f.initial {
		t.Fatalf("target did not advance: %v", e)
	}
	status := f.call("bot", "POST", f.route()+"/pulls/"+v.ID+"/merge", map[string]any{"do": "merge", "head_commit_id": head, "force_merge": false}, nil)
	if status >= 200 && status < 300 {
		merged, e := f.bot.ReadMergeResult(ctx, f.repo, v.ID)
		if e != nil || merged.State != "merged" {
			t.Fatalf("missing native result: %v", e)
		}
		t.Log("native ordinary merge accepted untested target advancement; adapter rejects this method")
	} else {
		t.Logf("native updater caught this race on this run (HTTP %d); atomic target guarantee remains uncertified", status)
	}
}

func TestMissingInspectorAndCodeownersFailClosed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.protect(false)
	bare, e := gitea.New(forge.Config{BaseURL: f.base, Client: &http.Client{}, Token: f.tokens["bot"]})
	if e != nil {
		t.Fatal(e)
	}
	rules, e := bare.ReadEffectiveRules(ctx, f.repo, "main")
	if e != nil || rules.State != domain.Unknown {
		t.Fatalf("missing inspector should be unknown: %+v %v", rules, e)
	}
	f.ok("admin", "POST", f.route()+"/contents/CODEOWNERS", map[string]any{"branch": "main", "content": base64.StdEncoding.EncodeToString([]byte(".* @reforge-reviewer\n")), "message": "Code owners"}, nil)
	rules, e = f.bot.ReadEffectiveRules(ctx, f.repo, "main")
	if e != nil || rules.State != domain.Unknown || !rules.RequireCodeOwners || rules.CodeOwnersEnforced != domain.Unknown {
		t.Fatalf("uncertified owners incorrectly supported %+v %v", rules, e)
	}
}

func TestForkHeadRepositoryIdentity(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var fork struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	f.ok("bot", "POST", f.route()+"/forks", map[string]any{}, &fork)
	forkRoute := "/repos/" + fork.FullName
	t.Cleanup(func() {
		if status := f.call("bot", "DELETE", forkRoute, nil, nil); status != 204 {
			t.Errorf("fork cleanup HTTP %d", status)
		}
	})
	var changed struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	f.ok("bot", "POST", forkRoute+"/contents/fork.txt", map[string]any{"branch": "main", "content": base64.StdEncoding.EncodeToString([]byte("fork contribution\n")), "message": "Fork candidate"}, &changed)
	var pull struct {
		Number int `json:"number"`
	}
	f.ok("bot", "POST", f.route()+"/pulls", map[string]any{"head": "reforge-bot:main", "base": "main", "title": "Fork source identity"}, &pull)
	change, e := f.bot.ReadChange(ctx, f.repo, fmt.Sprint(pull.Number))
	if e != nil {
		t.Fatal(e)
	}
	expected := forge.RepoRef{NativeID: fmt.Sprint(fork.ID), FullName: fork.FullName}
	if change.HeadRepository != expected || change.TargetRepository != f.repo || change.HeadSHA != changed.Commit.SHA || change.TargetSHA != f.initial {
		t.Fatalf("fork provenance lost: %+v", change)
	}
	f.ok("bot", "POST", forkRoute+"/statuses/"+changed.Commit.SHA, map[string]string{"state": "success", "context": "fork/check"}, nil)
	checks, e := f.bot.ListChecks(ctx, change.HeadRepository, change.HeadSHA)
	if e != nil || len(checks) != 1 || checks[0].PublisherID != "2" {
		t.Fatalf("fork checks missing: %+v %v", checks, e)
	}
}
