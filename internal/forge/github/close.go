package github

import (
	"context"
	"encoding/json"
	"net/http"

	"reforge/internal/forge"
)

func (p *Provider) CloseChange(ctx context.Context, in forge.CloseChangeRequest) (forge.Change, error) {
	if !in.Valid() || !positive(in.ChangeID) {
		return forge.Change{}, failure("invalid", "Reforge pull request required")
	}
	actor, err := p.authenticatedBot(ctx)
	if err != nil {
		return forge.Change{}, err
	}
	change, err := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if err != nil {
		return forge.Change{}, err
	}
	if !forge.Closable(change, in, actor) {
		return forge.Change{}, failure("conflict", "Pull request is not an open Reforge pull request")
	}
	route, err := repositoryPath(in.Repository)
	if err != nil {
		return forge.Change{}, err
	}
	for _, step := range []struct {
		method   string
		segments []string
		body     any
	}{
		{http.MethodPost, append(append([]string{}, route...), "issues", in.ChangeID, "comments"), map[string]string{"body": in.Comment}},
		{http.MethodPatch, append(append([]string{}, route...), "pulls", in.ChangeID), map[string]string{"state": "closed"}},
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
