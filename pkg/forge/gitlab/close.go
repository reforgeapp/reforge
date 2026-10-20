package gitlab

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

func (p *Provider) CloseChange(ctx context.Context, in forge.CloseChangeRequest) (forge.Change, error) {
	if !in.Valid() || !safeID(in.ChangeID) {
		return forge.Change{}, &domain.ProviderError{Kind: "invalid", Message: "Reforge merge request required"}
	}
	actor, err := p.authenticatedActor(ctx)
	if err != nil {
		return forge.Change{}, err
	}
	change, err := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if err != nil {
		return forge.Change{}, err
	}
	if !forge.Closable(change, in, actor) {
		return forge.Change{}, &domain.ProviderError{Kind: "conflict", Message: "Merge request is not an open Reforge merge request"}
	}
	project, err := p.projectSegment(in.Repository)
	if err != nil {
		return forge.Change{}, err
	}
	for _, step := range []struct {
		method   string
		segments []string
		body     any
	}{
		{http.MethodPost, []string{"projects", project, "merge_requests", in.ChangeID, "notes"}, map[string]string{"body": in.Comment}},
		{http.MethodPut, []string{"projects", project, "merge_requests", in.ChangeID}, map[string]string{"state_event": "close"}},
	} {
		data, _ := json.Marshal(step.body)
		status, headers, _, err := p.request(ctx, step.method, step.segments, nil, &requestBody{data: data})
		if err != nil {
			return forge.Change{}, err
		}
		if status < 200 || status >= 300 {
			return forge.Change{}, responseError(status, headers)
		}
	}
	return p.ReadChange(ctx, in.Repository, in.ChangeID)
}
