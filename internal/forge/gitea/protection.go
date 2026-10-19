package gitea

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

type protection struct {
	Name              string   `json:"rule_name"`
	Priority          int      `json:"priority"`
	Approvals         int      `json:"required_approvals"`
	Dismiss           bool     `json:"dismiss_stale_approvals"`
	IgnoreStale       bool     `json:"ignore_stale_approvals"`
	Strict            bool     `json:"block_on_outdated_branch"`
	Rejected          bool     `json:"block_on_rejected_reviews"`
	Requests          bool     `json:"block_on_official_review_requests"`
	BlockAdmin        bool     `json:"block_admin_merge_override"`
	Status            bool     `json:"enable_status_check"`
	Contexts          []string `json:"status_check_contexts"`
	Bypass            bool     `json:"enable_bypass_allowlist"`
	BypassUsers       []string `json:"bypass_allowlist_usernames"`
	BypassTeams       []string `json:"bypass_allowlist_teams"`
	ApprovalWhitelist bool     `json:"enable_approvals_whitelist"`
	ApprovalUsers     []string `json:"approvals_whitelist_username"`
	ApprovalTeams     []string `json:"approvals_whitelist_teams"`
}

func matchRule(pattern, branch string) (bool, bool) {
	if pattern == "*" {
		return true, true
	}
	if strings.ContainsAny(pattern, "*?[]{}\\") {
		return false, false
	}
	return pattern == branch, true
}
func (p *Provider) effective(ctx context.Context, r forge.RepoRef, branch string) (forge.Rules, protection, error) {
	out := forge.Rules{State: domain.Unknown, Reason: "Protection reader with repository administration visibility required", ObservedAt: time.Now().UTC(), CodeOwnersEnforced: domain.Unknown, StrictTargetEnforced: domain.Unknown}
	repo, e := p.repo(ctx, r)
	if e != nil {
		return out, protection{}, e
	}
	if repo.FastForward {
		out.AllowedMergeMethods = []string{"fast-forward-only"}
	}
	actor, e := p.actor(ctx)
	if e != nil {
		return out, protection{}, e
	}
	out.ActorCanBypass = actor.Admin || repo.Permissions["admin"]
	if p.inspector == nil {
		return hashRules(out, nil), protection{}, nil
	}
	route, _ := repoPath(r)
	var raw json.RawMessage
	e = p.inspector.request(ctx, "GET", route+"/branch_protections", nil, &raw)
	if e != nil {
		return out, protection{}, e
	}
	var rows []protection
	if json.Unmarshal(raw, &rows) != nil {
		return out, protection{}, failure("response", "Invalid branch protection response")
	}
	if len(rows) > 1000 {
		return out, protection{}, failure("response", "Too many branch protection rules")
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Priority < rows[j].Priority })
	var rule protection
	found := false
	for _, v := range rows {
		match, known := matchRule(v.Name, branch)
		if !known {
			out.Reason = "A higher-priority branch pattern cannot be evaluated safely"
			return hashRules(out, raw), rule, nil
		}
		if match {
			rule = v
			found = true
			break
		}
	}
	if !found {
		out.Reason = "Target branch has no native protection"
		return hashRules(out, raw), rule, nil
	}
	out.State = domain.Supported
	out.Reason = ""
	out.RequiredApprovals = rule.Approvals
	out.DismissStaleReviews = rule.Dismiss || rule.IgnoreStale
	out.RequireStrictTarget = rule.Strict
	if rule.Strict {
		var version struct {
			Version string `json:"version"`
		}
		if e := p.request(ctx, "GET", "/version", nil, &version); e != nil {
			return out, rule, e
		}
		if version.Version == "1.27.3" && repo.FastForward {
			out.StrictTargetEnforced = domain.Supported
		} else {
			out.State = domain.Unknown
			out.Reason = "Atomic target freshness requires certified Gitea 1.27.3 fast-forward-only merge"
		}
	}
	if rule.Bypass {
		for _, name := range rule.BypassUsers {
			if name == actor.Login {
				out.ActorCanBypass = true
			}
		}
		if len(rule.BypassTeams) > 0 {
			out.State = domain.Unknown
			out.Reason = "Bypass team membership cannot be proven absent"
		}
	}
	if rule.Status {
		out.State = domain.Unknown
		out.Reason = "Gitea required contexts do not atomically enforce trusted publisher identity"
		for _, name := range rule.Contexts {
			publisher := p.publishers[name]
			if name == "" || strings.ContainsAny(name, "*?[]{}\\") || !positive(publisher) {
				out.State = domain.Unknown
				out.Reason = "Required status needs an exact context and immutable trusted publisher"
			}
			out.RequiredChecks = append(out.RequiredChecks, forge.CheckRule{Name: name, PublisherID: publisher})
		}
		if len(rule.Contexts) == 0 {
			out.State = domain.Unknown
			out.Reason = "Native status enforcement has no contexts"
		}
	}
	if rule.ApprovalWhitelist && len(rule.ApprovalTeams) > 0 {
		out.State = domain.Unknown
		out.Reason = "Approval team enforcement has not been certified"
	}
	target, e := p.ResolveRef(ctx, r, branch)
	if e != nil {
		return out, rule, e
	}
	for _, name := range []string{"CODEOWNERS", "docs/CODEOWNERS", ".gitea/CODEOWNERS"} {
		_, e := p.ReadFileAtRef(ctx, r, name, target)
		if e == nil {
			out.RequireCodeOwners = true
			out.State = domain.Unknown
			out.Reason = "CODEOWNERS exists; native code-owner approval enforcement is uncertified"
			break
		}
		if pe, ok := e.(*domain.ProviderError); !ok || pe.Kind != "not_found" {
			return out, rule, e
		}
	}
	return hashRules(out, raw), rule, nil
}
func hashRules(r forge.Rules, raw []byte) forge.Rules {
	copy := r
	copy.Hash = ""
	copy.ObservedAt = time.Time{}
	b, _ := json.Marshal(struct {
		Rules  forge.Rules
		Native json.RawMessage
	}{copy, raw})
	sum := sha256.Sum256(b)
	r.Hash = hex.EncodeToString(sum[:])
	return r
}
func (p *Provider) ReadEffectiveRules(ctx context.Context, r forge.RepoRef, branch string) (forge.Rules, error) {
	out, _, e := p.effective(ctx, r, branch)
	return out, e
}

type review struct {
	ID        int64  `json:"id"`
	User      user   `json:"user"`
	Commit    string `json:"commit_id"`
	State     string `json:"state"`
	Dismissed bool   `json:"dismissed"`
	Stale     bool   `json:"stale"`
	Official  bool   `json:"official"`
}

func (p *Provider) reviews(ctx context.Context, r forge.RepoRef, id string) ([]review, error) {
	if _, e := p.loadPull(ctx, r, id); e != nil {
		return nil, e
	}
	route, _ := repoPath(r)
	var out []review
	for n := 1; n <= maxPages; n++ {
		var rows []review
		if e := p.request(ctx, "GET", route+"/pulls/"+id+"/reviews?limit=100&page="+strconv.Itoa(n), nil, &rows); e != nil {
			return nil, e
		}
		out = append(out, rows...)
		if len(rows) < 100 {
			return out, nil
		}
	}
	return nil, failure("pagination", "Review inventory exceeds limit")
}
func (p *Provider) ReadApprovals(ctx context.Context, r forge.RepoRef, id string) ([]forge.Approval, error) {
	rows, e := p.reviews(ctx, r, id)
	if e != nil {
		return nil, e
	}
	out := make([]forge.Approval, 0, len(rows))
	for _, v := range rows {
		if v.User.ID <= 0 || v.ID <= 0 {
			return nil, failure("identity", "Review actor identity missing")
		}
		state := strings.ToLower(v.State)
		out = append(out, forge.Approval{ID: strconv.FormatInt(v.ID, 10), ActorID: strconv.FormatInt(v.User.ID, 10), HeadSHA: v.Commit, State: state, Dismissed: v.Dismissed || v.Stale || !v.Official})
	}
	return out, nil
}
func (p *Provider) EvaluateNativeEligibility(ctx context.Context, r forge.RepoRef, id string) (forge.NativeEligibility, error) {
	change, e := p.ReadChange(ctx, r, id)
	if e != nil {
		return forge.NativeEligibility{}, e
	}
	out := forge.NativeEligibility{State: "blocked", HeadSHA: change.HeadSHA, TargetSHA: change.TargetSHA}
	rules, rule, e := p.effective(ctx, r, change.TargetBranch)
	if e != nil {
		return out, e
	}
	if rules.State != domain.Supported {
		out.Blockers = append(out.Blockers, rules.Reason)
	}
	if rules.ActorCanBypass {
		out.Blockers = append(out.Blockers, "Operational actor can bypass branch protections")
	}
	if !rules.RequireStrictTarget || rules.StrictTargetEnforced != domain.Supported {
		out.Blockers = append(out.Blockers, "Native strict-target enforcement is required")
	}
	if change.State != "open" || change.Draft || change.MergeStatus != "mergeable" {
		out.Blockers = append(out.Blockers, "Pull request is not open, ready and mergeable")
	}
	checks, e := p.ListChecks(ctx, change.HeadRepository, change.HeadSHA)
	if e != nil {
		return out, e
	}
	for _, required := range rules.RequiredChecks {
		ok := false
		for _, check := range checks {
			if check.Name == required.Name && check.PublisherID == required.PublisherID && check.HeadSHA == change.HeadSHA && check.Conclusion == "success" {
				ok = true
			}
		}
		if !ok {
			out.Blockers = append(out.Blockers, "Required trusted status is missing or unsuccessful: "+required.Name)
		}
	}
	reviews, e := p.reviews(ctx, r, id)
	if e != nil {
		return out, e
	}
	latest := map[int64]review{}
	for _, v := range reviews {
		if v.ID <= 0 || v.User.ID <= 0 {
			return out, failure("identity", "Review identity is incomplete")
		}
		switch v.State {
		case "APPROVED", "PENDING", "COMMENT", "REQUEST_CHANGES", "REQUEST_REVIEW":
		default:
			return out, failure("response", "Review state is unsupported")
		}
		if v.ID > latest[v.User.ID].ID {
			latest[v.User.ID] = v
		}
	}
	approvals := 0
	for _, v := range latest {
		if !v.Official || v.Dismissed || v.Stale {
			continue
		}
		if rule.Rejected && v.State == "REQUEST_CHANGES" {
			out.Blockers = append(out.Blockers, "Official review requests changes")
		}
		if rule.Requests && v.State == "REQUEST_REVIEW" {
			out.Blockers = append(out.Blockers, "Official requested review remains outstanding")
		}
		if v.State == "APPROVED" && v.Commit == change.HeadSHA && strconv.FormatInt(v.User.ID, 10) != change.AuthorID {
			if rule.ApprovalWhitelist {
				allowed := false
				for _, name := range rule.ApprovalUsers {
					if name == v.User.Login {
						allowed = true
					}
				}
				if !allowed {
					continue
				}
			}
			approvals++
		}
	}
	if approvals < rules.RequiredApprovals {
		out.Blockers = append(out.Blockers, "Current-head official approvals are insufficient")
	}
	if len(out.Blockers) == 0 {
		out.State = "eligible"
	}
	return out, nil
}
func (p *Provider) ReadQueueState(context.Context, forge.RepoRef, string) (forge.QueueState, error) {
	return forge.QueueState{State: "unsupported"}, failure("unsupported", "Gitea native merge queue has not been certified")
}
func (p *Provider) RequestNativeMergeOrQueue(ctx context.Context, in forge.MergeRequest) (forge.MergeResult, error) {
	if in.Queue {
		return forge.MergeResult{}, failure("unsupported", "Gitea native merge queue has not been certified")
	}
	if !sha(in.ExpectedHeadSHA) || !sha(in.ExpectedTargetSHA) || in.RulesHash == "" || in.GateID == "" || !validOperation(in.OperationID) {
		return forge.MergeResult{}, failure("invalid", "Merge requires immutable head, target, rules and persisted gate identity")
	}
	change, e := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if e != nil {
		return forge.MergeResult{}, e
	}
	if change.HeadSHA != in.ExpectedHeadSHA || change.TargetSHA != in.ExpectedTargetSHA {
		return forge.MergeResult{}, failure("conflict", "Merge head or target moved")
	}
	rules, e := p.ReadEffectiveRules(ctx, in.Repository, change.TargetBranch)
	if e != nil {
		return forge.MergeResult{}, e
	}
	if rules.Hash != in.RulesHash {
		return forge.MergeResult{}, failure("conflict", "Effective merge rules changed")
	}
	allowed := false
	for _, method := range rules.AllowedMergeMethods {
		if method == in.Method {
			allowed = true
		}
	}
	if !allowed {
		return forge.MergeResult{}, failure("forbidden", "Merge method is not allowed by repository settings")
	}
	eligible, e := p.EvaluateNativeEligibility(ctx, in.Repository, in.ChangeID)
	if e != nil {
		return forge.MergeResult{}, e
	}
	if eligible.State != "eligible" || eligible.HeadSHA != in.ExpectedHeadSHA || eligible.TargetSHA != in.ExpectedTargetSHA {
		return forge.MergeResult{}, failure("forbidden", "Native merge requirements are not satisfied")
	}
	route, _ := repoPath(in.Repository)
	e = p.request(ctx, "POST", route+"/pulls/"+url.PathEscape(in.ChangeID)+"/merge", map[string]any{"do": in.Method, "head_commit_id": in.ExpectedHeadSHA, "force_merge": false, "merge_when_checks_succeed": false, "delete_branch_after_merge": false}, nil)
	if e != nil {
		return forge.MergeResult{}, e
	}
	out, e := p.ReadMergeResult(ctx, in.Repository, in.ChangeID)
	if e != nil {
		return out, &domain.ProviderError{Kind: "uncertain", Message: "Merge response could not be reconciled", Uncertain: true}
	}
	if out.State != "merged" || !sha(out.MergeSHA) || out.MergeSHA != in.ExpectedHeadSHA || out.HeadSHA != in.ExpectedHeadSHA {
		return out, &domain.ProviderError{Kind: "uncertain", Message: "Merge completion is not yet observable", Uncertain: true}
	}
	return out, nil
}
func (p *Provider) ReadMergeResult(ctx context.Context, r forge.RepoRef, id string) (forge.MergeResult, error) {
	v, e := p.ReadChange(ctx, r, id)
	if e != nil {
		return forge.MergeResult{}, e
	}
	return forge.MergeResult{State: v.State, NativeID: v.ID, MergeSHA: v.MergeSHA, HeadSHA: v.HeadSHA, URL: v.URL}, nil
}
