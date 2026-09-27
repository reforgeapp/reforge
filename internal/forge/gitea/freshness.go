package gitea

import (
	"context"
	"net/http"
	"reforge/internal/forge"
)

func (p *Provider) Behind(ctx context.Context, repo forge.RepoRef, base, head string) (int, error) {
	if !sha(base) || !sha(head) {
		return 0, failure("invalid", "Exact commits required")
	}
	route, err := repoPath(repo)
	if err != nil {
		return 0, err
	}
	var result struct {
		Total *int `json:"total_commits"`
	}
	if err = p.request(ctx, http.MethodGet, route+"/compare/"+head+".."+base, nil, &result); err != nil {
		return 0, err
	}
	if result.Total == nil || *result.Total < 0 {
		return 0, failure("provider", "Missing or invalid comparison result")
	}
	return *result.Total, nil
}
