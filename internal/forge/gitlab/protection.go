package gitlab

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

type projectPolicy struct {
	ID               int64  `json:"id"`
	Method           string `json:"merge_method"`
	Squash           string `json:"squash_option"`
	Pipelines        *bool  `json:"only_allow_merge_if_pipeline_succeeds"`
	Skipped          *bool  `json:"allow_merge_on_skipped_pipeline"`
	Discussions      *bool  `json:"only_allow_merge_if_all_discussions_are_resolved"`
	Trains           *bool  `json:"merge_trains_enabled"`
	MergePipelines   *bool  `json:"merge_pipelines_enabled"`
	SkipTrain        *bool  `json:"merge_trains_skip_train_allowed"`
	TrainEnforcement string `json:"merge_train_enforcement"`
	AutomaticRebase  *bool  `json:"automatic_rebase_enabled"`
	ExternalChecks   *bool  `json:"only_allow_merge_if_all_status_checks_passed"`
}
type protectedBranch struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CodeOwners *bool  `json:"code_owner_approval_required"`
	Force      *bool  `json:"allow_force_push"`
}
type approvalRule struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"rule_type"`
	Count        *int   `json:"approvals_required"`
	Approved     *bool  `json:"approved"`
	Hidden       *bool  `json:"contains_hidden_groups"`
	Overridden   *bool  `json:"overridden"`
	AllProtected *bool  `json:"applies_to_all_protected_branches"`
	Branches     []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"protected_branches"`
}

func matches(pattern, branch string) bool {
	if pattern == "" {
		return false
	}
	expression := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
	ok, _ := regexp.MatchString(expression, branch)
	return ok
}
func (p *Provider) ReadEffectiveRules(ctx context.Context, r forge.RepoRef, branch string) (forge.Rules, error) {
	out := forge.Rules{State: domain.Unknown, Reason: "Complete project protection, approval and operational authority evidence required", ObservedAt: time.Now().UTC(), CodeOwnersEnforced: domain.Unknown, StrictTargetEnforced: domain.Unknown}
	if !safeID(r.NativeID) || !validBranch(branch) {
		return out, failure("invalid", "Immutable project and branch required")
	}
	reader := p.protectionReader()
	route := []string{"projects", r.NativeID}
	var config projectPolicy
	if err := reader.read(ctx, route, &config); err != nil {
		return out, err
	}
	if stringID(config.ID) != r.NativeID {
		return out, failure("identity", "Protection project identity changed")
	}
	var actor struct {
		ID    int64 `json:"id"`
		Admin *bool `json:"is_admin"`
	}
	if err := p.read(ctx, []string{"user"}, &actor); err != nil {
		return out, err
	}
	var liveBranch struct {
		Name      string `json:"name"`
		Protected *bool  `json:"protected"`
		CanPush   *bool  `json:"can_push"`
	}
	if err := p.read(ctx, append(route, "repository", "branches", branch), &liveBranch); err != nil {
		return out, err
	}
	if actor.ID <= 0 || actor.Admin == nil || liveBranch.Name != branch || liveBranch.Protected == nil || liveBranch.CanPush == nil {
		return out, failure("provider", "Operational authority or protected branch evidence incomplete")
	}
	var membership struct {
		Access int    `json:"access_level"`
		RoleID *int64 `json:"member_role_id"`
	}
	if err := p.read(ctx, append(route, "members", "all", stringID(actor.ID)), &membership); err != nil {
		return out, err
	}
	if membership.Access <= 0 || membership.RoleID != nil {
		return out, failure("unsupported", "Operational membership role is unavailable or custom")
	}
	out.ActorCanBypass = *actor.Admin || *liveBranch.CanPush || membership.Access >= 40
	if !*liveBranch.Protected {
		out.Reason = "Target branch is not protected"
		return out, nil
	}
	rows, err := reader.pages(ctx, append(route, "protected_branches"), nil)
	if err != nil {
		return out, err
	}
	matched := []json.RawMessage{}
	owners := false
	for _, raw := range rows {
		var rule protectedBranch
		if json.Unmarshal(raw, &rule) != nil || rule.ID <= 0 || rule.Name == "" {
			return out, failure("provider", "Invalid protected branch rule")
		}
		if !matches(rule.Name, branch) {
			continue
		}
		if rule.CodeOwners == nil || rule.Force == nil {
			return out, failure("provider", "Protected branch requirements incomplete")
		}
		matched = append(matched, raw)
		owners = owners || *rule.CodeOwners
	}
	if len(matched) == 0 {
		out.Reason = "No visible rule explains effective protection"
		return out, nil
	}
	var approvalSettings struct {
		Reset      *bool `json:"reset_approvals_on_push"`
		NoOverride *bool `json:"disable_overriding_approvers_per_merge_request"`
	}
	if err = reader.read(ctx, append(route, "approvals"), &approvalSettings); err != nil {
		return out, err
	}
	if approvalSettings.Reset == nil || approvalSettings.NoOverride == nil {
		return out, failure("provider", "Approval reset and override enforcement unavailable")
	}
	approvalRows, err := reader.pages(ctx, append(route, "approval_rules"), nil)
	if err != nil {
		return out, err
	}
	out.State = domain.Supported
	out.Reason = "Native protection and project requirements observed"
	out.RequireCodeOwners = owners
	out.CodeOwnersEnforced = domain.Supported
	out.DismissStaleReviews = *approvalSettings.Reset
	for _, raw := range approvalRows {
		var rule approvalRule
		if json.Unmarshal(raw, &rule) != nil || rule.ID <= 0 || rule.Count == nil || *rule.Count < 0 || rule.Hidden == nil {
			return out, failure("provider", "Approval rule visibility incomplete")
		}
		if *rule.Hidden {
			out.State = domain.Unknown
			out.Reason = "Approval rule contains hidden groups"
		}
		applies := len(rule.Branches) == 0 || rule.AllProtected != nil && *rule.AllProtected
		for _, v := range rule.Branches {
			if matches(v.Name, branch) {
				applies = true
			}
		}
		switch rule.Type {
		case "regular", "any_approver", "code_owner", "report_approver":
		default:
			out.State = domain.Unknown
			out.Reason = "Unknown approval rule type"
		}
		if applies && *rule.Count > out.RequiredApprovals {
			out.RequiredApprovals = *rule.Count
		}
	}
	if !*approvalSettings.NoOverride {
		out.State = domain.Unknown
		out.Reason = "Merge request approval requirements can be overridden"
	}
	if config.Pipelines == nil || config.Skipped == nil || config.Discussions == nil || config.Trains == nil || config.ExternalChecks == nil {
		out.State = domain.Unknown
		out.Reason = "Project merge requirements are incomplete"
	}
	out.RequireQueue = config.Trains != nil && *config.Trains
	out.RequireStrictTarget = config.Method == "ff" || config.Method == "rebase_merge"
	if out.RequireStrictTarget && config.AutomaticRebase != nil && !*config.AutomaticRebase && config.Pipelines != nil && *config.Pipelines && config.Skipped != nil && !*config.Skipped {
		out.StrictTargetEnforced = domain.Supported
	}
	if out.RequireQueue && (config.MergePipelines == nil || !*config.MergePipelines || config.SkipTrain == nil || *config.SkipTrain || (config.TrainEnforcement != "enforce_for_all_users" && config.TrainEnforcement != "enforce_with_owner_override")) {
		out.State = domain.Unknown
		out.Reason = "Merge train enforcement, bypass or merged-results pipeline configuration is unqualified"
	}
	switch config.Method {
	case "merge", "rebase_merge", "ff":
	default:
		out.State = domain.Unknown
		out.Reason = "Unknown native merge method"
	}
	switch config.Squash {
	case "never":
		out.AllowedMergeMethods = []string{config.Method}
	case "always":
		out.AllowedMergeMethods = []string{"squash"}
	case "default_on", "default_off":
		out.AllowedMergeMethods = []string{config.Method, "squash"}
	default:
		out.State = domain.Unknown
		out.Reason = "Unknown squash policy"
	}
	for name, id := range p.publishers {
		if name == "" || !safeID(id) {
			out.State = domain.Unknown
			out.Reason = "Required status publisher identity missing"
		}
		out.RequiredChecks = append(out.RequiredChecks, forge.CheckRule{Name: name, PublisherID: id})
	}
	sort.Slice(out.RequiredChecks, func(i, j int) bool { return out.RequiredChecks[i].Name < out.RequiredChecks[j].Name })
	material, _ := json.Marshal(struct {
		Project   projectPolicy
		Branches  []json.RawMessage
		Approvals any
		Rules     []json.RawMessage
		Actor     int64
		Bypass    bool
		Checks    []forge.CheckRule
	}{config, matched, approvalSettings, approvalRows, actor.ID, out.ActorCanBypass, out.RequiredChecks})
	sum := sha256.Sum256(material)
	out.Hash = hex.EncodeToString(sum[:])
	return out, nil
}
func (p *Provider) ReadApprovals(ctx context.Context, r forge.RepoRef, id string) ([]forge.Approval, error) {
	if !safeID(r.NativeID) || !safeID(id) {
		return nil, failure("invalid", "Immutable project and MR required")
	}
	var response struct {
		IID        int64 `json:"iid"`
		ProjectID  int64 `json:"project_id"`
		ApprovedBy []struct {
			User struct {
				ID int64 `json:"id"`
			} `json:"user"`
		} `json:"approved_by"`
	}
	if err := p.read(ctx, []string{"projects", r.NativeID, "merge_requests", id, "approvals"}, &response); err != nil {
		return nil, err
	}
	if stringID(response.IID) != id || stringID(response.ProjectID) != r.NativeID || response.ApprovedBy == nil {
		return nil, failure("identity", "Approval response identity incomplete")
	}
	out := []forge.Approval{}
	for _, v := range response.ApprovedBy {
		if v.User.ID <= 0 {
			return nil, failure("identity", "Approver identity missing")
		}
		out = append(out, forge.Approval{ActorID: stringID(v.User.ID), State: "approved"})
	}
	return out, nil
}
func (p *Provider) EvaluateNativeEligibility(ctx context.Context, r forge.RepoRef, id string) (forge.NativeEligibility, error) {
	out := forge.NativeEligibility{State: "blocked", Blockers: []string{}}
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
		out.Blockers = append(out.Blockers, rules.Reason)
	}
	if rules.ActorCanBypass {
		out.Blockers = append(out.Blockers, "Operational identity can bypass target protection")
	}
	if change.State != "opened" || change.Draft || change.MergeStatus != "mergeable" {
		out.Blockers = append(out.Blockers, "Native merge state is not mergeable; calculating, syncing and unknown states require refresh")
	}
	route := []string{"projects", r.NativeID, "merge_requests", id}
	var approvals struct {
		Overwritten *bool          `json:"approval_rules_overwritten"`
		Rules       []approvalRule `json:"rules"`
	}
	if err = p.protectionReader().read(ctx, append(route, "approval_state"), &approvals); err != nil {
		return out, err
	}
	if approvals.Overwritten == nil || *approvals.Overwritten || approvals.Rules == nil {
		return out, failure("provider", "Effective MR approvals are incomplete or overridden")
	}
	for _, rule := range approvals.Rules {
		if rule.ID <= 0 || rule.Count == nil || rule.Approved == nil || rule.Hidden == nil {
			return out, failure("provider", "Effective approval rule is incomplete")
		}
		if *rule.Hidden || !*rule.Approved || rule.Overridden != nil && *rule.Overridden {
			out.Blockers = append(out.Blockers, "Applicable native approval rule is not satisfied")
		}
		if *rule.Count > 0 && !rules.DismissStaleReviews {
			out.Blockers = append(out.Blockers, "Current-head approval enforcement is unavailable")
		}
	}
	var details gitlabMergeRequest
	if err = p.read(ctx, route, &details); err != nil {
		return out, err
	}
	if stringID(details.IID) != id || stringID(details.TargetProjectID) != r.NativeID || details.SHA != change.HeadSHA || details.DetailedMergeStatus != "mergeable" {
		out.Blockers = append(out.Blockers, "Merge request changed while checking approvals")
	}
	pipeline := details.HeadPipeline
	testedSHA, pipelineErr := p.verifiedPipeline(ctx, r, pipeline, change)
	if pipelineErr != nil {
		return out, pipelineErr
	}
	if testedSHA == "" {
		out.Blockers = append(out.Blockers, "Successful source or exact merged-result pipeline evidence missing")
	}
	checks, err := p.ListChecks(ctx, r, pipelineSHA(pipeline, change.HeadSHA))
	if err != nil {
		return out, err
	}
	for _, required := range rules.RequiredChecks {
		var latest forge.Check
		for _, check := range checks {
			if check.Name == required.Name && check.PublisherID == required.PublisherID && check.HeadSHA == testedSHA {
				a, _ := parseID(check.ID)
				b, _ := parseID(latest.ID)
				if a > b {
					latest = check
				}
			}
		}
		if latest.Status != "success" {
			out.Blockers = append(out.Blockers, "Required status missing, stale, failing or wrong publisher: "+required.Name)
		}
	}
	var settings projectPolicy
	if err = p.protectionReader().read(ctx, []string{"projects", r.NativeID}, &settings); err != nil {
		return out, err
	}
	if settings.ExternalChecks != nil && *settings.ExternalChecks {
		rows, err := p.pages(ctx, append(route, "status_checks"), nil)
		if err != nil {
			return out, err
		}
		for _, raw := range rows {
			var check struct {
				ID     int64  `json:"id"`
				Status string `json:"status"`
			}
			if json.Unmarshal(raw, &check) != nil || check.ID <= 0 || check.Status != "passed" {
				out.Blockers = append(out.Blockers, "Required external status check not passed")
			}
		}
	}
	if !rules.RequireQueue && rules.StrictTargetEnforced != domain.Supported {
		out.Blockers = append(out.Blockers, "Target freshness is not enforceable by the observed native configuration")
	}
	if p.mergeGuard == nil {
		out.Blockers = append(out.Blockers, "Persisted execution gate and provider qualification required")
	}
	if len(out.Blockers) == 0 {
		out.State = "eligible"
	}
	return out, nil
}

func pipelineSHA(pipeline *pipelineRecord, fallback string) string {
	if pipeline != nil && validSHA(pipeline.SHA) {
		return pipeline.SHA
	}
	return fallback
}
func (p *Provider) verifiedPipeline(ctx context.Context, r forge.RepoRef, pipeline *pipelineRecord, change forge.Change) (string, error) {
	if pipeline == nil || pipeline.ID <= 0 || stringID(pipeline.ProjectID) != r.NativeID || !validSHA(pipeline.SHA) || pipeline.Status != "success" {
		return "", nil
	}
	if pipeline.SHA == change.HeadSHA {
		return pipeline.SHA, nil
	}
	if pipeline.Source != "merge_request_event" {
		return "", nil
	}
	var commit struct {
		Parents []string `json:"parent_ids"`
	}
	if err := p.read(ctx, []string{"projects", r.NativeID, "repository", "commits", pipeline.SHA}, &commit); err != nil {
		return "", err
	}
	if len(commit.Parents) != 2 {
		return "", nil
	}
	hasHead, hasTarget := false, false
	for _, parent := range commit.Parents {
		hasHead = hasHead || parent == change.HeadSHA
		hasTarget = hasTarget || parent == change.TargetSHA
	}
	if !hasHead || !hasTarget {
		return "", nil
	}
	return pipeline.SHA, nil
}
