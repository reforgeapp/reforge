package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"strconv"
)

func (p *Provider) CancelNativeQueue(ctx context.Context, request forge.QueueCancelRequest) (forge.QueueState, error) {
	if !positive(request.Repository.NativeID) || request.QueueID == "" || request.ChangeID == "" || !validSHA(request.ExpectedHeadSHA) || !auth.ValidID(request.OperationID) {
		return forge.QueueState{}, failure("invalid", "Repository, queue, change, head and UUID operation are required")
	}
	current, err := p.queuePull(ctx, request.Repository, request.ChangeID)
	if err != nil {
		return forge.QueueState{}, err
	}
	if current.Head != request.ExpectedHeadSHA {
		return forge.QueueState{}, failure("conflict", "Queue admission or pull request head changed")
	}
	if current.Queue == nil {
		return p.convergeAbsentQueue(ctx, request)
	}
	if current.Queue.ID != request.QueueID {
		return forge.QueueState{}, failure("conflict", "Queue admission or pull request head changed")
	}
	var response struct {
		Dequeue struct {
			Entry *struct {
				ID string `json:"id"`
			} `json:"mergeQueueEntry"`
		} `json:"dequeuePullRequest"`
	}
	query := `mutation($input:DequeuePullRequestInput!){dequeuePullRequest(input:$input){mergeQueueEntry{id}}}`
	if err := p.graphql(ctx, query, map[string]any{"input": map[string]any{"id": current.ID, "clientMutationId": request.OperationID}}, &response, true); err != nil {
		return forge.QueueState{}, err
	}
	if response.Dequeue.Entry == nil || response.Dequeue.Entry.ID != request.QueueID {
		return forge.QueueState{}, &domain.ProviderError{Kind: "uncertain", Message: "GitHub queue cancellation response requires reconciliation", Uncertain: true}
	}
	state, err := p.ReadQueueState(ctx, request.Repository, request.ChangeID)
	if err != nil {
		return forge.QueueState{}, &domain.ProviderError{Kind: "uncertain", Message: "GitHub queue cancellation outcome requires reconciliation", Uncertain: true}
	}
	change, err := p.ReadChange(ctx, request.Repository, request.ChangeID)
	if err != nil || change.HeadSHA != request.ExpectedHeadSHA || (change.State != "open" && change.State != "opened") || change.Draft {
		return state, &domain.ProviderError{Kind: "uncertain", Message: "GitHub pull request changed during queue cancellation", Uncertain: true}
	}
	if state.ID != "" {
		return state, &domain.ProviderError{Kind: "uncertain", Message: "GitHub queue cancellation was not observed", Uncertain: true}
	}
	return state, nil
}

func (p *Provider) convergeAbsentQueue(ctx context.Context, request forge.QueueCancelRequest) (forge.QueueState, error) {
	segments, err := repositoryPath(request.Repository)
	if err != nil {
		return forge.QueueState{}, err
	}
	segments = append(segments, "pulls", request.ChangeID)
	status, headers, body, err := p.request(ctx, http.MethodGet, segments, nil, nil)
	if err != nil {
		return forge.QueueState{}, err
	}
	if status < 200 || status >= 300 {
		return forge.QueueState{}, responseError(status, headers)
	}
	var raw struct {
		Number    int64           `json:"number"`
		State     string          `json:"state"`
		Draft     bool            `json:"draft"`
		Merged    bool            `json:"merged"`
		AutoMerge json.RawMessage `json:"auto_merge"`
		Head      struct {
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			SHA  string `json:"sha"`
			Repo struct {
				ID       int64  `json:"id"`
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
	}
	if err := decode(body, &raw); err != nil || strconv.FormatInt(raw.Number, 10) != request.ChangeID || strconv.FormatInt(raw.Base.Repo.ID, 10) != request.Repository.NativeID || raw.Base.Repo.FullName != request.Repository.FullName || raw.Head.SHA != request.ExpectedHeadSHA || (raw.State != "open" && raw.State != "opened") || raw.Draft || raw.Merged || len(raw.AutoMerge) == 0 || !bytes.Equal(bytes.TrimSpace(raw.AutoMerge), []byte("null")) {
		return forge.QueueState{}, &domain.ProviderError{Kind: "uncertain", Message: "GitHub queue cancellation requires canonical open pull request state", Uncertain: true}
	}
	final, err := p.queuePull(ctx, request.Repository, request.ChangeID)
	if err != nil || final.Queue != nil || final.Head != raw.Head.SHA || final.Base != raw.Base.SHA {
		return forge.QueueState{}, &domain.ProviderError{Kind: "uncertain", Message: "GitHub queue admission changed during cancellation observation", Uncertain: true}
	}
	return forge.QueueState{State: "not_queued", HeadSHA: raw.Head.SHA, TargetSHA: raw.Base.SHA}, nil
}
