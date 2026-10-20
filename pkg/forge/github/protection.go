package github

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

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

type statusRules struct {
	Strict   bool     `json:"strict"`
	Contexts []string `json:"contexts"`
	Checks   []struct {
		Context string `json:"context"`
		AppID   *int64 `json:"app_id"`
	} `json:"checks"`
}
type reviewRules struct {
	Count   int  `json:"required_approving_review_count"`
	Dismiss bool `json:"dismiss_stale_reviews"`
	Owners  bool `json:"require_code_owner_reviews"`
	Bypass  struct {
		Apps []struct {
			ID int64 `json:"id"`
		} `json:"apps"`
		Users []struct {
			ID int64 `json:"id"`
		} `json:"users"`
		Teams []json.RawMessage `json:"teams"`
	} `json:"bypass_pull_request_allowances"`
}
type classicProtection struct {
	Status  *statusRules `json:"required_status_checks"`
	Reviews *reviewRules `json:"required_pull_request_reviews"`
	Admins  *struct {
		Enabled bool `json:"enabled"`
	} `json:"enforce_admins"`
	Linear struct {
		Enabled bool `json:"enabled"`
	} `json:"required_linear_history"`
}
type branchRule struct {
	Type       string          `json:"type"`
	RulesetID  int64           `json:"ruleset_id"`
	Parameters json.RawMessage `json:"parameters"`
}

func (p *Provider) get(ctx context.Context, route []string, query url.Values, out any) error {
	status, headers, body, err := p.request(ctx, "GET", route, query, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return responseError(status, headers)
	}
	return decode(body, out)
}
func (p *Provider) ReadEffectiveRules(ctx context.Context, r forge.RepoRef, branch string) (forge.Rules, error) {
	out := forge.Rules{State: domain.Unknown, Reason: "Complete native protection and App bypass visibility required", ObservedAt: time.Now().UTC(), CodeOwnersEnforced: domain.Unknown, StrictTargetEnforced: domain.Unknown}
	if !positive(r.NativeID) || !validBranch(branch) {
		return out, failure("invalid", "Immutable repository and branch required")
	}
	actor, err := p.authenticatedBot(ctx)
	if err != nil {
		return out, err
	}
	repo, err := p.GetRepository(ctx, r)
	if err != nil {
		return out, err
	}
	route, _ := repositoryPath(r)
	var branchInfo struct {
		Protected *bool `json:"protected"`
	}
	if err = p.get(ctx, append(route, "branches", branch), nil, &branchInfo); err != nil {
		return out, err
	}
	if branchInfo.Protected == nil {
		return out, failure("provider", "Missing branch protection indicator")
	}
	var classic classicProtection
	var classicRaw json.RawMessage
	status, headers, raw, err := p.request(ctx, "GET", append(route, "branches", branch, "protection"), nil, nil)
	if err != nil {
		return out, err
	}
	if status == 200 {
		classicRaw = raw
		if err = decode(raw, &classic); err != nil {
			return out, err
		}
		var sections map[string]json.RawMessage
		_ = json.Unmarshal(raw, &sections)
		if classic.Status != nil && !fieldsPresent(sections["required_status_checks"], "strict", "contexts", "checks") {
			return out, failure("provider", "Incomplete classic status requirements")
		}
		if classic.Reviews != nil && !fieldsPresent(sections["required_pull_request_reviews"], "required_approving_review_count", "dismiss_stale_reviews", "require_code_owner_reviews", "bypass_pull_request_allowances") {
			return out, failure("provider", "Incomplete classic review or bypass requirements")
		}
		if classic.Admins == nil {
			return out, failure("provider", "Incomplete classic protection document")
		}
	} else if status != 404 && (status != 403 || *branchInfo.Protected) {
		return out, responseError(status, headers)
	}
	out.State = domain.Supported
	out.Reason = "Native branch protection and applied rulesets observed"
	out.CodeOwnersEnforced = domain.Supported
	out.StrictTargetEnforced = domain.Unsupported
	for _, permission := range repo.Permissions {
		if permission == "admin" {
			out.ActorCanBypass = true
		}
	}
	var applied []branchRule
	var rawRules []json.RawMessage
	for page := 1; page <= maxPages; page++ {
		status, headers, body, err := p.request(ctx, "GET", append(route, "rules", "branches", branch), url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}}, nil)
		if err != nil {
			return out, err
		}
		if status == 403 && !*branchInfo.Protected {
			break
		}
		if status != 200 {
			return out, responseError(status, headers)
		}
		var rows []branchRule
		if err = decode(body, &rows); err != nil {
			return out, err
		}
		if rows == nil || len(rows) > maxPageSize {
			return out, failure("provider", "Invalid applied-rules page")
		}
		applied = append(applied, rows...)
		rawRules = append(rawRules, body)
		next, err := p.nextCursor(headers, page)
		if err != nil {
			return out, err
		}
		if next == "" {
			break
		}
	}
	addCheck := func(name string, id int64) {
		if name == "" || id <= 0 {
			out.State = domain.Unknown
			out.Reason = "Required check publisher is not bound to an immutable App ID"
			return
		}
		out.RequiredChecks = append(out.RequiredChecks, forge.CheckRule{Name: name, PublisherID: strconv.FormatInt(id, 10)})
	}
	if classic.Status != nil {
		out.RequireStrictTarget = classic.Status.Strict
		for _, c := range classic.Status.Checks {
			if c.AppID == nil {
				addCheck(c.Context, 0)
			} else {
				addCheck(c.Context, *c.AppID)
			}
		}
		for _, name := range classic.Status.Contexts {
			found := false
			for _, c := range classic.Status.Checks {
				if c.Context == name {
					found = true
				}
			}
			if !found {
				addCheck(name, 0)
			}
		}
	}
	if classic.Reviews != nil {
		v := classic.Reviews
		out.RequiredApprovals = v.Count
		out.DismissStaleReviews = v.Dismiss
		out.RequireCodeOwners = v.Owners
		for _, app := range v.Bypass.Apps {
			if p.app != nil && strconv.FormatInt(app.ID, 10) == p.appID() {
				out.ActorCanBypass = true
			}
		}
		for _, user := range v.Bypass.Users {
			if strconv.FormatInt(user.ID, 10) == actor {
				out.ActorCanBypass = true
			}
		}
		if len(v.Bypass.Teams) > 0 {
			out.State = domain.Unknown
			out.Reason = "Classic team bypass applicability is not qualified"
		}
	}
	linear := classic.Linear.Enabled
	details := map[string]json.RawMessage{}
	for _, rule := range applied {
		if rule.RulesetID <= 0 {
			return out, failure("provider", "Applied rule has no ruleset identity")
		}
		key := strconv.FormatInt(rule.RulesetID, 10)
		if _, seen := details[key]; !seen {
			var detailRaw json.RawMessage
			if err = p.get(ctx, append(route, "rulesets", key), url.Values{"includes_parents": {"true"}}, &detailRaw); err != nil {
				return out, err
			}
			var detail struct {
				ID          int64  `json:"id"`
				Enforcement string `json:"enforcement"`
				Bypass      *[]struct {
					ActorID   *int64 `json:"actor_id"`
					ActorType string `json:"actor_type"`
					Mode      string `json:"bypass_mode"`
				} `json:"bypass_actors"`
			}
			if err = decode(detailRaw, &detail); err != nil {
				return out, err
			}
			if detail.ID != rule.RulesetID || detail.Enforcement != "active" || detail.Bypass == nil {
				out.State = domain.Unknown
				out.Reason = "Applied ruleset identity, enforcement or bypass visibility is incomplete"
			} else {
				for _, b := range *detail.Bypass {
					if b.Mode != "always" && b.Mode != "pull_request" && b.Mode != "exempt" {
						out.State = domain.Unknown
						out.Reason = "Unknown ruleset bypass mode"
					}
					switch b.ActorType {
					case "Integration":
						if b.ActorID == nil {
							out.State = domain.Unknown
						} else if p.app != nil && strconv.FormatInt(*b.ActorID, 10) == p.appID() {
							out.ActorCanBypass = true
						}
					case "User":
						if b.ActorID == nil {
							out.State = domain.Unknown
						} else if strconv.FormatInt(*b.ActorID, 10) == actor {
							out.ActorCanBypass = true
						}
					default:
						out.State = domain.Unknown
						out.Reason = "Ruleset bypass actor applicability is not qualified"
					}
				}
			}
			details[key] = detailRaw
		}
		switch rule.Type {
		case "required_status_checks":
			if !fieldsPresent(rule.Parameters, "strict_required_status_checks_policy", "required_status_checks") {
				return out, failure("provider", "Incomplete status check rule")
			}
			var v struct {
				Strict bool `json:"strict_required_status_checks_policy"`
				Checks []struct {
					Name string `json:"context"`
					ID   *int64 `json:"integration_id"`
				} `json:"required_status_checks"`
			}
			if err = json.Unmarshal(rule.Parameters, &v); err != nil {
				return out, failure("provider", "Invalid required status checks")
			}
			out.RequireStrictTarget = out.RequireStrictTarget || v.Strict
			if len(v.Checks) == 0 {
				out.State = domain.Unknown
				out.Reason = "Empty required-check rule"
			}
			for _, c := range v.Checks {
				if c.ID == nil {
					addCheck(c.Name, 0)
				} else {
					addCheck(c.Name, *c.ID)
				}
			}
		case "pull_request":
			if !fieldsPresent(rule.Parameters, "required_approving_review_count", "dismiss_stale_reviews_on_push", "require_code_owner_review") {
				return out, failure("provider", "Incomplete pull request rule")
			}
			var v struct {
				Count   int      `json:"required_approving_review_count"`
				Dismiss bool     `json:"dismiss_stale_reviews_on_push"`
				Owners  bool     `json:"require_code_owner_review"`
				Methods []string `json:"allowed_merge_methods"`
			}
			if err = json.Unmarshal(rule.Parameters, &v); err != nil {
				return out, failure("provider", "Invalid pull request rule")
			}
			if v.Count > out.RequiredApprovals {
				out.RequiredApprovals = v.Count
			}
			out.DismissStaleReviews = out.DismissStaleReviews || v.Dismiss
			out.RequireCodeOwners = out.RequireCodeOwners || v.Owners
			if len(v.Methods) > 0 {
				if out.AllowedMergeMethods == nil {
					out.AllowedMergeMethods = v.Methods
				} else {
					out.AllowedMergeMethods = intersection(out.AllowedMergeMethods, v.Methods)
				}
			}
		case "merge_queue":
			out.RequireQueue = true
			var queue struct {
				Method   string `json:"merge_method"`
				Grouping string `json:"grouping_strategy"`
				Maximum  int    `json:"max_entries_to_merge"`
			}
			if json.Unmarshal(rule.Parameters, &queue) != nil {
				return out, failure("provider", "Invalid merge queue rule")
			}
			if queue.Grouping != "ALLGREEN" || queue.Maximum != 1 {
				out.State = domain.Unknown
				out.Reason = "Queue qualification requires ALLGREEN and one entry per merge group"
			}
			method := strings.ToLower(queue.Method)
			if method != "merge" && method != "squash" && method != "rebase" {
				out.State = domain.Unknown
				out.Reason = "Native queue merge method is unavailable"
			} else if out.AllowedMergeMethods == nil {
				out.AllowedMergeMethods = []string{method}
			} else {
				out.AllowedMergeMethods = intersection(out.AllowedMergeMethods, []string{method})
			}
		case "required_linear_history":
			linear = true
		case "deletion", "non_fast_forward":
		default:
			out.State = domain.Unknown
			out.Reason = "An applied ruleset requirement is not qualified by this adapter"
		}
	}
	if out.RequiredApprovals < 0 || out.RequiredApprovals > 100 {
		out.State = domain.Unknown
		out.Reason = "Invalid review requirement"
	}
	if out.RequireStrictTarget && len(out.RequiredChecks) > 0 {
		out.StrictTargetEnforced = domain.Supported
	}
	var repositoryOptions struct {
		ID          int64           `json:"id"`
		Permissions map[string]bool `json:"permissions"`
		Merge       *bool           `json:"allow_merge_commit"`
		Squash      *bool           `json:"allow_squash_merge"`
		Rebase      *bool           `json:"allow_rebase_merge"`
	}
	if err = p.get(ctx, route, nil, &repositoryOptions); err != nil {
		return out, err
	}
	if strconv.FormatInt(repositoryOptions.ID, 10) != r.NativeID {
		return out, failure("identity", "Repository identity changed while reading protection")
	}
	if admin, present := repositoryOptions.Permissions["admin"]; !present {
		out.State = domain.Unknown
		out.Reason = "Operational actor administrative authority is unavailable"
	} else if admin {
		out.ActorCanBypass = true
	}
	methods := []string{}
	if repositoryOptions.Merge != nil && *repositoryOptions.Merge && !linear {
		methods = append(methods, "merge")
	}
	if repositoryOptions.Squash != nil && *repositoryOptions.Squash {
		methods = append(methods, "squash")
	}
	if repositoryOptions.Rebase != nil && *repositoryOptions.Rebase {
		methods = append(methods, "rebase")
	}
	if out.AllowedMergeMethods == nil {
		out.AllowedMergeMethods = methods
	} else {
		out.AllowedMergeMethods = intersection(out.AllowedMergeMethods, methods)
	}
	sort.Slice(out.RequiredChecks, func(i, j int) bool {
		a, b := out.RequiredChecks[i], out.RequiredChecks[j]
		if a.Name == b.Name {
			return a.PublisherID < b.PublisherID
		}
		return a.Name < b.Name
	})
	sort.Strings(out.AllowedMergeMethods)
	out.Unprotected = !*branchInfo.Protected && classicRaw == nil && len(applied) == 0
	material, _ := json.Marshal(struct {
		Classic json.RawMessage
		Applied []json.RawMessage
		Details map[string]json.RawMessage
		Methods []string
		Actor   string
		Bypass  bool
	}{classicRaw, rawRules, details, out.AllowedMergeMethods, actor, out.ActorCanBypass})
	sum := sha256.Sum256(material)
	out.Hash = hex.EncodeToString(sum[:])
	return out, nil
}
func intersection(a, b []string) []string {
	out := []string{}
	for _, x := range a {
		for _, y := range b {
			if x == y {
				out = append(out, x)
				break
			}
		}
	}
	return out
}
func (p *Provider) ReadApprovals(ctx context.Context, r forge.RepoRef, id string) ([]forge.Approval, error) {
	if !positive(id) {
		return nil, failure("invalid", "Pull request number required")
	}
	route, err := repositoryPath(r)
	if err != nil {
		return nil, err
	}
	out := []forge.Approval{}
	for page := 1; page <= maxPages; page++ {
		status, headers, raw, err := p.request(ctx, "GET", append(route, "pulls", id, "reviews"), url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}}, nil)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, responseError(status, headers)
		}
		var rows []struct {
			ID   int64 `json:"id"`
			User struct {
				ID int64 `json:"id"`
			} `json:"user"`
			Commit string `json:"commit_id"`
			State  string `json:"state"`
		}
		if err = decode(raw, &rows); err != nil {
			return nil, err
		}
		if rows == nil || len(rows) > maxPageSize {
			return nil, failure("provider", "Invalid approval page")
		}
		for _, v := range rows {
			if v.ID <= 0 || v.User.ID <= 0 {
				return nil, failure("provider", "Missing review identity")
			}
			switch v.State {
			case "APPROVED", "CHANGES_REQUESTED", "COMMENTED", "DISMISSED", "PENDING":
			default:
				return nil, failure("provider", "Unknown review state")
			}
			out = append(out, forge.Approval{ID: strconv.FormatInt(v.ID, 10), ActorID: strconv.FormatInt(v.User.ID, 10), HeadSHA: v.Commit, State: v.State, Dismissed: v.State == "DISMISSED"})
		}
		next, err := p.nextCursor(headers, page)
		if err != nil {
			return nil, err
		}
		if next == "" {
			return out, nil
		}
	}
	return nil, failure("provider", "Review pagination exceeded bound")
}
func (p *Provider) EvaluateNativeEligibility(ctx context.Context, r forge.RepoRef, id string) (forge.NativeEligibility, error) {
	return p.evaluateEligibility(ctx, r, id, forge.CheckRule{})
}

func (p *Provider) EvaluateQueuePrerequisites(ctx context.Context, r forge.RepoRef, id string) (forge.NativeEligibility, forge.CheckRule, error) {
	if p.app == nil {
		return forge.NativeEligibility{}, forge.CheckRule{}, failure("unsupported", "GitHub merge queue gating requires the GitHub App")
	}
	if _, err := p.authenticatedBot(ctx); err != nil {
		return forge.NativeEligibility{}, forge.CheckRule{}, err
	}
	check := forge.CheckRule{Name: forge.QueueExecutionCheckName, PublisherID: p.appID()}
	state, err := p.evaluateEligibility(ctx, r, id, check)
	return state, check, err
}

func (p *Provider) evaluateEligibility(ctx context.Context, r forge.RepoRef, id string, executionCheck forge.CheckRule) (forge.NativeEligibility, error) {
	out := forge.NativeEligibility{State: "blocked", Blockers: []string{}}
	protection := func(reason string) {
		out.Blockers = append(out.Blockers, reason)
		out.Protection = append(out.Protection, reason)
	}
	change, err := p.ReadChange(ctx, r, id)
	if err != nil {
		return out, err
	}
	out.HeadSHA, out.TargetSHA = change.HeadSHA, change.TargetSHA
	rules, err := p.ReadEffectiveRules(ctx, r, change.TargetBranch)
	if err != nil {
		return out, err
	}
	if rules.State != domain.Supported {
		protection(rules.Reason)
	}
	if rules.ActorCanBypass {
		protection("Operational App can bypass protection")
	}
	if executionCheck.Name != "" {
		bound := false
		for _, required := range rules.RequiredChecks {
			bound = bound || required == executionCheck
		}
		if !rules.RequireQueue || !bound {
			out.Blockers = append(out.Blockers, "Native queue must require the execution check from this App")
		}
	}
	switch {
	case change.State != "open" || change.Draft || change.MergeStatus == "dirty" || change.MergeStatus == "unknown" || change.MergeStatus == "draft":
		out.Blockers = append(out.Blockers, "Native pull request state is not clean and open")
	case change.MergeStatus != "clean" && !(rules.RequireQueue && (change.MergeStatus == "blocked" || change.MergeStatus == "behind")):
		protection("Native pull request state is not clean and open")
	}
	head, err := p.ResolveRef(ctx, change.HeadRepository, "heads/"+change.HeadBranch)
	if err != nil {
		return out, err
	}
	if head != change.HeadSHA {
		out.Blockers = append(out.Blockers, "Pull request head is being refreshed")
	}
	checks, err := p.ListChecks(ctx, r, change.HeadSHA)
	if err != nil {
		return out, err
	}
	for _, required := range rules.RequiredChecks {
		if executionCheck.Name != "" && required == executionCheck {
			continue
		}
		var newest forge.Check
		var newestID int64
		for _, check := range checks {
			if check.Name != required.Name || check.PublisherID != required.PublisherID || check.HeadSHA != change.HeadSHA {
				continue
			}
			n, _ := strconv.ParseInt(check.ID, 10, 64)
			if n > newestID {
				newest, newestID = check, n
			}
		}
		if newestID == 0 || newest.Status != "completed" || newest.Conclusion != "success" {
			out.Blockers = append(out.Blockers, "Required check missing, stale, failing or wrong publisher: "+required.Name)
		}
	}
	approvals, err := p.ReadApprovals(ctx, r, id)
	if err != nil {
		return out, err
	}
	latest := map[string]forge.Approval{}
	for _, review := range approvals {
		if review.State == "COMMENTED" || review.State == "PENDING" {
			continue
		}
		old, found := latest[review.ActorID]
		n, _ := strconv.ParseInt(review.ID, 10, 64)
		oldN, _ := strconv.ParseInt(old.ID, 10, 64)
		if !found || n > oldN {
			latest[review.ActorID] = review
		}
	}
	approved := 0
	for _, review := range latest {
		if review.State == "CHANGES_REQUESTED" {
			out.Blockers = append(out.Blockers, "Native review requested changes")
		}
		if review.State == "APPROVED" && !review.Dismissed && review.HeadSHA == change.HeadSHA {
			approved++
		}
	}
	if approved < rules.RequiredApprovals {
		protection("Current-head approvals are missing")
	}
	if !rules.RequireQueue && rules.StrictTargetEnforced != domain.Supported {
		protection("Native target freshness enforcement is not established")
	}
	if p.mergeGuard == nil {
		out.Blockers = append(out.Blockers, "Persisted execution gate and live provider qualification are required")
	}
	if len(out.Blockers) == 0 {
		out.State = "eligible"
	}
	return out, nil
}

func fieldsPresent(raw json.RawMessage, names ...string) bool {
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return false
	}
	for _, name := range names {
		if len(values[name]) == 0 || string(values[name]) == "null" {
			return false
		}
	}
	return true
}
