package github

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

type graphPull struct {
	ID    string `json:"id"`
	Head  string `json:"headRefOid"`
	Base  string `json:"baseRefOid"`
	Queue *struct {
		ID    string `json:"id"`
		State string `json:"state"`
		Base  *struct {
			OID string `json:"oid"`
		} `json:"baseCommit"`
	} `json:"mergeQueueEntry"`
}

func (p *Provider) queuePull(ctx context.Context, r forge.RepoRef, id string) (graphPull, error) {
	var out struct {
		Repository struct {
			ID   int64     `json:"databaseId"`
			Pull graphPull `json:"pullRequest"`
		} `json:"repository"`
	}
	parts := strings.Split(r.FullName, "/")
	n, err := strconv.Atoi(id)
	if len(parts) != 2 || err != nil || n <= 0 || !positive(r.NativeID) {
		return graphPull{}, failure("invalid", "Immutable repository and pull request required")
	}
	err = p.graphql(ctx, `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){databaseId pullRequest(number:$number){id headRefOid baseRefOid mergeQueueEntry{id state baseCommit{oid}}}}}`, map[string]any{"owner": parts[0], "name": parts[1], "number": n}, &out, false)
	if err != nil {
		return graphPull{}, err
	}
	if strconv.FormatInt(out.Repository.ID, 10) != r.NativeID || out.Repository.Pull.ID == "" || !validSHA(out.Repository.Pull.Head) || !validSHA(out.Repository.Pull.Base) {
		return graphPull{}, failure("identity", "GraphQL pull request identity is incomplete")
	}
	return out.Repository.Pull, nil
}
func (p *Provider) ReadQueueState(ctx context.Context, r forge.RepoRef, id string) (forge.QueueState, error) {
	pull, err := p.queuePull(ctx, r, id)
	if err != nil {
		return forge.QueueState{}, err
	}
	out := forge.QueueState{State: "not_queued", HeadSHA: pull.Head, TargetSHA: pull.Base}
	if pull.Queue == nil {
		return out, nil
	}
	if pull.Queue.ID == "" {
		return out, failure("provider", "Queue identity missing")
	}
	switch pull.Queue.State {
	case "AWAITING_CHECKS", "LOCKED", "MERGEABLE", "QUEUED", "UNMERGEABLE":
	default:
		return out, failure("provider", "Unknown merge queue state")
	}
	out.ID = pull.Queue.ID
	out.State = strings.ToLower(pull.Queue.State)
	out.TargetSHA = ""
	if pull.Queue.Base != nil && validSHA(pull.Queue.Base.OID) {
		out.TargetSHA = pull.Queue.Base.OID
	}
	return out, nil
}
func (p *Provider) RequestNativeMergeOrQueue(ctx context.Context, in forge.MergeRequest) (forge.MergeResult, error) {
	var out forge.MergeResult
	if p.mergeGuard == nil {
		return out, failure("unsupported", "Persisted execution authorization and live native enforcement qualification required")
	}
	if !positive(in.Repository.NativeID) || !positive(in.ChangeID) || !validSHA(in.ExpectedHeadSHA) || !validSHA(in.ExpectedTargetSHA) || in.RulesHash == "" || in.GateID == "" {
		return out, failure("invalid", "Exact head, target, rules and persisted gate are required")
	}
	if err := validOperationID(in.OperationID); err != nil {
		return out, err
	}
	if _, err := p.authenticatedBot(ctx); err != nil {
		return out, err
	}
	change, err := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if err != nil {
		return out, err
	}
	if change.HeadSHA != in.ExpectedHeadSHA || change.TargetSHA != in.ExpectedTargetSHA {
		return out, failure("conflict", "Pull request head or current target changed")
	}
	eligibility, err := p.EvaluateNativeEligibility(ctx, in.Repository, in.ChangeID)
	if err != nil {
		return out, err
	}
	if eligibility.HeadSHA != in.ExpectedHeadSHA || eligibility.TargetSHA != in.ExpectedTargetSHA {
		return out, failure("conflict", "Native eligibility is for a different candidate")
	}
	if eligibility.State != "eligible" {
		return out, failure("policy", "Native merge prerequisites are not satisfied")
	}
	rules, err := p.ReadEffectiveRules(ctx, in.Repository, change.TargetBranch)
	if err != nil {
		return out, err
	}
	if rules.State != domain.Supported || rules.Hash != in.RulesHash || rules.ActorCanBypass || rules.RequireQueue != in.Queue {
		return out, failure("conflict", "Native protections or merge path changed")
	}
	appGate := false
	for _, check := range rules.RequiredChecks {
		if check.PublisherID == p.app.appID {
			appGate = true
		}
	}
	if !appGate {
		return out, failure("unsupported", "A provider-required check bound to this App must enforce the execution policy")
	}
	if !in.Queue && rules.StrictTargetEnforced != domain.Supported {
		return out, failure("unsupported", "Direct merge requires provider-enforced strict target freshness")
	}
	allowed := false
	for _, method := range rules.AllowedMergeMethods {
		if method == in.Method {
			allowed = true
		}
	}
	if !allowed {
		return out, failure("policy", "Requested merge method is not permitted")
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
		return out, failure("conflict", "Head or target changed at execution boundary")
	}
	confirm := func() error {
		current, err := p.ReadChange(ctx, in.Repository, in.ChangeID)
		if err != nil {
			return err
		}
		if current.HeadRepository != change.HeadRepository || current.TargetRepository != change.TargetRepository || current.Repository != change.Repository || current.HeadBranch != change.HeadBranch || current.TargetBranch != change.TargetBranch || current.HeadSHA != in.ExpectedHeadSHA || current.TargetSHA != in.ExpectedTargetSHA || current.State != "open" || current.Draft {
			return failure("conflict", "Pull request identity or target changed at execution boundary")
		}
		change = current
		return nil
	}
	if in.Queue {
		pull, err := p.queuePull(ctx, in.Repository, in.ChangeID)
		if err != nil {
			return out, err
		}
		if pull.Head != in.ExpectedHeadSHA || pull.Base != in.ExpectedTargetSHA {
			return out, failure("conflict", "Queue candidate changed")
		}
		if pull.Queue != nil {
			if pull.Queue.ID == "" {
				return out, failure("provider", "Queue identity missing")
			}
			switch pull.Queue.State {
			case "AWAITING_CHECKS", "LOCKED", "MERGEABLE", "QUEUED":
			default:
				return out, failure("conflict", "Existing queue admission requires reconciliation")
			}
			return forge.MergeResult{State: "queued", NativeID: pull.Queue.ID, HeadSHA: pull.Head, URL: change.URL}, nil
		}
		if err = confirm(); err != nil {
			return out, err
		}
		if err = p.mergeGuard(ctx, in, change, rules); err != nil {
			return out, err
		}
		var result struct {
			Enqueue struct {
				Entry *struct {
					ID string `json:"id"`
				} `json:"mergeQueueEntry"`
			} `json:"enqueuePullRequest"`
		}
		err = p.graphql(ctx, `mutation($input:EnqueuePullRequestInput!){enqueuePullRequest(input:$input){mergeQueueEntry{id}}}`, map[string]any{"input": map[string]any{"pullRequestId": pull.ID, "expectedHeadOid": in.ExpectedHeadSHA, "jump": false, "clientMutationId": in.OperationID}}, &result, true)
		if err != nil {
			return out, err
		}
		if result.Enqueue.Entry == nil || result.Enqueue.Entry.ID == "" {
			return out, transportFailure("POST", "Queue admission requires reconciliation")
		}
		return forge.MergeResult{State: "queued", NativeID: result.Enqueue.Entry.ID, HeadSHA: in.ExpectedHeadSHA, URL: change.URL}, nil
	}
	if err = confirm(); err != nil {
		return out, err
	}
	if err = p.mergeGuard(ctx, in, change, rules); err != nil {
		return out, err
	}
	route, _ := repositoryPath(in.Repository)
	body, _ := json.Marshal(map[string]string{"sha": in.ExpectedHeadSHA, "merge_method": in.Method})
	status, headers, raw, err := p.request(ctx, "PUT", append(route, "pulls", in.ChangeID, "merge"), nil, &requestBody{data: body})
	if err != nil {
		return out, err
	}
	if status != 200 {
		return out, responseError(status, headers)
	}
	var result struct {
		Merged bool   `json:"merged"`
		SHA    string `json:"sha"`
	}
	if decode(raw, &result) != nil || !result.Merged || !validSHA(result.SHA) {
		return out, transportFailure("PUT", "Native merge result requires canonical reconciliation")
	}
	canonical, err := p.ReadMergeResult(ctx, in.Repository, in.ChangeID)
	if err != nil || canonical.State != "merged" || canonical.MergeSHA != result.SHA {
		return out, transportFailure("PUT", "Native merge could not be reconciled")
	}
	return canonical, nil
}
func (p *Provider) ReadMergeResult(ctx context.Context, r forge.RepoRef, id string) (forge.MergeResult, error) {
	change, err := p.ReadChange(ctx, r, id)
	if err != nil {
		return forge.MergeResult{}, err
	}
	out := forge.MergeResult{State: change.State, NativeID: change.ID, HeadSHA: change.HeadSHA, MergeSHA: change.MergeSHA, URL: change.URL}
	if out.State == "merged" && !validSHA(out.MergeSHA) {
		return forge.MergeResult{}, failure("provider", "Merged pull request lacks merge commit identity")
	}
	return out, nil
}
func (p *Provider) WithReviewAuthorizer(fn func(context.Context, forge.RepoRef, string, []string) error) *Provider {
	q := *p
	q.authorizeReview = fn
	return &q
}

func (p *Provider) ListAllowedWorkflows(context.Context, forge.RepoRef) ([]forge.Workflow, error) {
	return nil, failure("unsupported", "Delivery workflow qualification belongs to the delivery controller")
}
func (p *Provider) TriggerOrObservePipeline(context.Context, forge.PipelineRequest) (forge.DeploymentStatus, error) {
	return forge.DeploymentStatus{}, failure("unsupported", "Native deployment approval and attribution are not qualified")
}
func (p *Provider) ReadDeploymentGates(context.Context, forge.RepoRef, string) (forge.DeploymentGates, error) {
	return forge.DeploymentGates{State: "unknown", NativeEnforced: domain.Unknown}, failure("unsupported", "Native deployment gate qualification is required")
}
func (p *Provider) ReadDeploymentStatus(context.Context, forge.RepoRef, string) (forge.DeploymentStatus, error) {
	return forge.DeploymentStatus{}, failure("unsupported", "Deployment attribution is not implemented")
}
func (p *Provider) RequestAllowedRecovery(context.Context, forge.PipelineRequest) (forge.DeploymentStatus, error) {
	return forge.DeploymentStatus{}, failure("unsupported", "Recovery authorization and native deployment qualification are required")
}

var _ forge.Provider = (*Provider)(nil)
