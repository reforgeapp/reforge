package gitlab

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

type trainEntry struct {
	ID           int64           `json:"id"`
	Status       string          `json:"status"`
	TargetBranch string          `json:"target_branch"`
	Pipeline     *pipelineRecord `json:"pipeline"`
	MergeRequest struct {
		IID       int64 `json:"iid"`
		ProjectID int64 `json:"project_id"`
	} `json:"merge_request"`
	User struct {
		ID int64 `json:"id"`
	} `json:"user"`
}

func validTrain(entry trainEntry, r forge.RepoRef, id, branch string) bool {
	if entry.ID <= 0 || stringID(entry.MergeRequest.IID) != id || stringID(entry.MergeRequest.ProjectID) != r.NativeID || entry.TargetBranch != branch {
		return false
	}
	switch entry.Status {
	case "idle", "fresh", "stale", "merging", "merged", "skip_merged":
		return true
	}
	return false
}
func (p *Provider) ReadQueueState(ctx context.Context, r forge.RepoRef, id string) (forge.QueueState, error) {
	var out forge.QueueState
	if !safeID(r.NativeID) || !safeID(id) {
		return out, failure("invalid", "Immutable project and MR required")
	}
	change, err := p.ReadChange(ctx, r, id)
	if err != nil {
		return out, err
	}
	var entry trainEntry
	if err = p.read(ctx, []string{"projects", r.NativeID, "merge_trains", "merge_requests", id}, &entry); err != nil {
		var native *domain.ProviderError
		if errors.As(err, &native) && native.Kind == "not_found" {
			return forge.QueueState{State: "not_queued", HeadSHA: change.HeadSHA, TargetSHA: change.TargetSHA}, nil
		}
		return out, err
	}
	if !validTrain(entry, r, id, change.TargetBranch) {
		return out, failure("identity", "Merge train identity or state is incomplete")
	}
	out.ID = stringID(entry.ID)
	out.HeadSHA = change.HeadSHA
	out.State = entry.Status
	if entry.Pipeline != nil {
		if entry.Pipeline.ID <= 0 || stringID(entry.Pipeline.ProjectID) != r.NativeID || !validSHA(entry.Pipeline.SHA) {
			return out, failure("identity", "Merge train pipeline identity is incomplete")
		}
		out.TestedSHA = entry.Pipeline.SHA
		var commit struct {
			Parents []string `json:"parent_ids"`
		}
		if err = p.read(ctx, []string{"projects", r.NativeID, "repository", "commits", entry.Pipeline.SHA}, &commit); err != nil {
			return out, err
		}
		hasHead, hasTarget := false, false
		for _, parent := range commit.Parents {
			hasHead = hasHead || parent == change.HeadSHA
			hasTarget = hasTarget || parent == change.TargetSHA
		}
		if len(commit.Parents) == 2 && hasHead && hasTarget && out.TestedSHA != change.HeadSHA {
			out.HeadSHA, out.TargetSHA = change.HeadSHA, change.TargetSHA
		} else {
			out.TestedSHA = ""
			out.State = "stale"
		}

	}
	return out, nil
}
func (p *Provider) RequestNativeMergeOrQueue(ctx context.Context, in forge.MergeRequest) (forge.MergeResult, error) {
	var out forge.MergeResult
	if p.mergeGuard == nil {
		return out, failure("unsupported", "Persisted execution gate and provider qualification required")
	}
	if !safeID(in.Repository.NativeID) || !safeID(in.ChangeID) || !validSHA(in.ExpectedHeadSHA) || !validSHA(in.ExpectedTargetSHA) || in.GateID == "" || in.RulesHash == "" {
		return out, failure("invalid", "Exact source, target and persisted gate required")
	}
	if err := validOperationID(in.OperationID); err != nil {
		return out, err
	}
	actor, err := p.authenticatedActor(ctx)
	if err != nil {
		return out, err
	}
	change, err := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if err != nil {
		return out, err
	}
	if change.HeadSHA != in.ExpectedHeadSHA || change.TargetSHA != in.ExpectedTargetSHA {
		return out, failure("conflict", "Source or target changed")
	}
	var eligibility forge.NativeEligibility
	if in.Queue {
		eligibility, _, err = p.EvaluateQueuePrerequisites(ctx, in.Repository, in.ChangeID)
	} else {
		eligibility, err = p.EvaluateNativeEligibility(ctx, in.Repository, in.ChangeID)
	}
	if err != nil {
		return out, err
	}
	if eligibility.State != "eligible" || eligibility.HeadSHA != in.ExpectedHeadSHA || eligibility.TargetSHA != in.ExpectedTargetSHA {
		return out, failure("policy", "Native merge prerequisites are not satisfied")
	}
	rules, err := p.ReadEffectiveRules(ctx, in.Repository, change.TargetBranch)
	if err != nil {
		return out, err
	}
	if rules.State != domain.Supported || rules.ActorCanBypass || rules.Hash != in.RulesHash || rules.RequireQueue != in.Queue {
		return out, failure("conflict", "Protection or native merge path changed")
	}
	if !in.Queue && rules.StrictTargetEnforced != domain.Supported {
		return out, failure("unsupported", "Native target freshness is not enforceable")
	}
	policyCheck := false
	for _, check := range rules.RequiredChecks {
		if check.PublisherID == actor {
			policyCheck = true
		}
	}
	if !policyCheck {
		return out, failure("unsupported", "A trusted status publisher bound to the operational actor and a certified native execution gate are required")
	}
	allowed := false
	for _, method := range rules.AllowedMergeMethods {
		if method == in.Method {
			allowed = true
		}
	}
	if !allowed {
		return out, failure("policy", "Requested merge method is not permitted by project")
	}
	if err = p.mergeGuard(ctx, in, change, rules); err != nil {
		return out, err
	}
	head, err := p.ResolveRef(ctx, change.HeadRepository, "heads/"+change.HeadBranch)
	if err != nil {
		return out, err
	}
	target, err := p.ResolveRef(ctx, in.Repository, "heads/"+change.TargetBranch)
	if err != nil {
		return out, err
	}
	if head != in.ExpectedHeadSHA || target != in.ExpectedTargetSHA {
		return out, failure("conflict", "Source or target changed at execution boundary")
	}
	route := []string{"projects", in.Repository.NativeID, "merge_requests", in.ChangeID, "merge"}
	method := "PUT"
	if in.Queue {
		route = []string{"projects", in.Repository.NativeID, "merge_trains", "merge_requests", in.ChangeID}
		method = "POST"
	}
	body, _ := json.Marshal(map[string]any{"sha": in.ExpectedHeadSHA, "squash": in.Method == "squash", "auto_merge": false})
	fresh, err := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if err != nil {
		return out, err
	}
	if fresh.HeadSHA != in.ExpectedHeadSHA || fresh.TargetSHA != in.ExpectedTargetSHA || fresh.HeadBranch != change.HeadBranch || fresh.TargetBranch != change.TargetBranch || !sameRepo(fresh.HeadRepository, change.HeadRepository) || fresh.State != "opened" || fresh.Draft || fresh.MergeStatus != "mergeable" {
		return out, failure("conflict", "Merge request identity or state changed at execution boundary")
	}
	if err = p.mergeGuard(ctx, in, fresh, rules); err != nil {
		return out, err
	}
	status, headers, raw, err := p.request(ctx, method, route, nil, &requestBody{data: body})
	if err != nil {
		return out, err
	}
	if status < 200 || status >= 300 {
		return out, responseError(status, headers)
	}
	if in.Queue {
		var entry trainEntry
		if status != 201 || decode(raw, &entry) != nil || !validTrain(entry, in.Repository, in.ChangeID, change.TargetBranch) || stringID(entry.User.ID) != actor {
			return out, uncertain(failure("identity", "Train admission requires canonical reconciliation"))
		}
		if entry.Status != "idle" && entry.Status != "fresh" {
			return out, uncertain(failure("conflict", "Train admission state changed"))
		}
		return forge.MergeResult{State: "queued", NativeID: stringID(entry.ID), HeadSHA: in.ExpectedHeadSHA, URL: change.URL}, nil
	}
	var merged gitlabMergeRequest
	if decode(raw, &merged) != nil || stringID(merged.IID) != in.ChangeID || merged.State != "merged" || stringID(merged.TargetProjectID) != in.Repository.NativeID {
		return out, uncertain(failure("provider", "Native merge outcome requires reconciliation"))
	}
	result, err := p.ReadMergeResult(ctx, in.Repository, in.ChangeID)
	if err != nil || result.State != "merged" || result.HeadSHA != in.ExpectedHeadSHA || validSHA(merged.MergeCommitSHA) && result.MergeSHA != merged.MergeCommitSHA {
		return out, uncertain(failure("provider", "Native merge could not be reconciled"))
	}
	return result, nil
}
func (p *Provider) ReadMergeResult(ctx context.Context, r forge.RepoRef, id string) (forge.MergeResult, error) {
	change, err := p.ReadChange(ctx, r, id)
	if err != nil {
		return forge.MergeResult{}, err
	}
	if change.State == "merged" && !validSHA(change.MergeSHA) {
		return forge.MergeResult{}, failure("provider", "Merged MR commit identity unavailable")
	}
	if change.State == "opened" {
		queue, err := p.ReadQueueState(ctx, r, id)
		if err != nil {
			return forge.MergeResult{}, err
		}
		if queue.ID != "" {
			return forge.MergeResult{State: "queued", NativeID: queue.ID, HeadSHA: change.HeadSHA, URL: change.URL}, nil
		}
	}
	return forge.MergeResult{State: change.State, NativeID: change.ID, HeadSHA: change.HeadSHA, MergeSHA: change.MergeSHA, URL: change.URL}, nil
}

var _ forge.Provider = (*Provider)(nil)
