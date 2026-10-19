package github

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/reforgeapp/reforge/internal/forge"
)

func (p *Provider) CommentChange(ctx context.Context, in forge.CommentChangeRequest) (forge.Change, error) {
	if !in.Valid() || !positive(in.ChangeID) {
		return forge.Change{}, failure("invalid", "Pull request comment required")
	}
	change, err := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if err != nil {
		return forge.Change{}, err
	}
	if change.State != "open" || change.HeadSHA != in.HeadSHA {
		return forge.Change{}, failure("conflict", "Pull request changed before Reforge could comment")
	}
	route, err := repositoryPath(in.Repository)
	if err != nil {
		return forge.Change{}, err
	}
	data, _ := json.Marshal(map[string]string{"body": in.Comment})
	status, headers, _, err := p.request(ctx, http.MethodPost, append(append([]string{}, route...), "issues", in.ChangeID, "comments"), nil, &requestBody{data: data})
	if err != nil {
		return forge.Change{}, err
	}
	if status < 200 || status >= 300 {
		return forge.Change{}, responseError(status, headers)
	}
	return change, nil
}
