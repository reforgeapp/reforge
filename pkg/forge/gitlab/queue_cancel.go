package gitlab

import (
	"context"
	"errors"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"net/http"
)

func (p *Provider) CancelNativeQueue(ctx context.Context, request forge.QueueCancelRequest) (forge.QueueState, error) {
	if !safeID(request.Repository.NativeID) || !safeID(request.ChangeID) || request.QueueID == "" || !validSHA(request.ExpectedHeadSHA) || !auth.ValidID(request.OperationID) {
		return forge.QueueState{}, failure("invalid", "Project, queue, change, head and UUID operation are required")
	}
	current, err := p.ReadQueueState(ctx, request.Repository, request.ChangeID)
	if err != nil {
		var providerErr *domain.ProviderError
		if errors.As(err, &providerErr) && providerErr.Kind == "not_found" {
			change, autoMerge, readErr := p.readCancellationMR(ctx, request)
			if readErr == nil && !autoMerge && change.HeadSHA == request.ExpectedHeadSHA && change.State == "opened" && !change.Draft {
				return forge.QueueState{State: "not_queued", HeadSHA: change.HeadSHA, TargetSHA: change.TargetSHA}, nil
			}
		}
		return forge.QueueState{}, err
	}
	if current.ID == "" && current.State == "not_queued" {
		change, autoMerge, err := p.readCancellationMR(ctx, request)
		if err != nil {
			return forge.QueueState{}, err
		}
		if !autoMerge && change.HeadSHA == request.ExpectedHeadSHA && change.State == "opened" && !change.Draft {
			return current, nil
		}
		return forge.QueueState{}, failure("conflict", "Absent train has unresolved auto-merge or changed source")
	}
	if current.ID != request.QueueID || current.HeadSHA != request.ExpectedHeadSHA {
		return forge.QueueState{}, failure("conflict", "Merge train admission or merge request head changed")
	}
	project, err := p.projectSegment(request.Repository)
	if err != nil {
		return forge.QueueState{}, err
	}
	status, headers, _, err := p.request(ctx, http.MethodPost, []string{"projects", project, "merge_requests", request.ChangeID, "cancel_merge_when_pipeline_succeeds"}, nil, nil)
	if err != nil {
		return forge.QueueState{}, err
	}
	if status < 200 || status >= 300 {
		return forge.QueueState{}, responseError(status, headers)
	}
	change, autoMerge, err := p.readCancellationMR(ctx, request)
	if err != nil || change.HeadSHA != request.ExpectedHeadSHA || (change.State != "open" && change.State != "opened") || change.Draft {
		return forge.QueueState{}, &domain.ProviderError{Kind: "uncertain", Message: "GitLab merge request changed during queue cancellation", Uncertain: true}
	}
	state, err := p.ReadQueueState(ctx, request.Repository, request.ChangeID)
	if err != nil {
		var providerErr *domain.ProviderError
		if errors.As(err, &providerErr) && providerErr.Kind == "not_found" && !autoMerge {
			return forge.QueueState{State: "not_queued", HeadSHA: change.HeadSHA, TargetSHA: change.TargetSHA}, nil
		}
		return forge.QueueState{}, &domain.ProviderError{Kind: "uncertain", Message: "GitLab queue cancellation outcome requires reconciliation", Uncertain: true}
	}
	if state.ID == "" && state.State == "not_queued" && state.HeadSHA == request.ExpectedHeadSHA && !autoMerge {
		return state, nil
	}
	if state.ID == request.QueueID || state.ID != "" {
		return state, &domain.ProviderError{Kind: "uncertain", Message: "GitLab queue admission changed during cancellation", Uncertain: true}
	}
	return state, &domain.ProviderError{Kind: "uncertain", Message: "GitLab queue cancellation outcome requires reconciliation", Uncertain: true}
}

func (p *Provider) readCancellationMR(ctx context.Context, request forge.QueueCancelRequest) (forge.Change, bool, error) {
	project, err := p.projectSegment(request.Repository)
	if err != nil {
		return forge.Change{}, false, err
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "merge_requests", request.ChangeID}, nil, nil)
	if err != nil {
		return forge.Change{}, false, err
	}
	if status < 200 || status >= 300 {
		return forge.Change{}, false, responseError(status, headers)
	}
	var raw struct {
		IID                       int64  `json:"iid"`
		ProjectID                 int64  `json:"project_id"`
		State                     string `json:"state"`
		Draft                     bool   `json:"draft"`
		SHA                       string `json:"sha"`
		MergeWhenPipelineSucceeds *bool  `json:"merge_when_pipeline_succeeds"`
	}
	if err := decode(body, &raw); err != nil {
		return forge.Change{}, false, err
	}
	if stringID(raw.IID) != request.ChangeID || stringID(raw.ProjectID) != request.Repository.NativeID {
		return forge.Change{}, false, failure("identity", "Merge request identity changed")
	}
	if raw.MergeWhenPipelineSucceeds == nil {
		return forge.Change{}, false, failure("uncertain", "GitLab auto-merge state is absent")
	}
	change, err := p.ReadChange(ctx, request.Repository, request.ChangeID)
	if err != nil {
		return forge.Change{}, false, err
	}
	if change.HeadSHA != raw.SHA || change.State != raw.State || change.Draft != raw.Draft {
		return forge.Change{}, false, failure("uncertain", "GitLab merge request changed during inspection")
	}
	return change, *raw.MergeWhenPipelineSucceeds, nil
}
