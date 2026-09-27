package gitlab

import (
	"context"
	"net/http"
	"net/url"
	"reforge/internal/forge"
)

func (p *Provider) Behind(ctx context.Context, repo forge.RepoRef, base, head string) (int, error) {
	if !validSHA(base) || !validSHA(head) {
		return 0, failure("invalid", "Exact commits required")
	}
	project, err := p.projectSegment(repo)
	if err != nil {
		return 0, err
	}
	query := url.Values{"from": {head}, "to": {base}, "straight": {"true"}}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "repository", "compare"}, query, nil)
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK {
		return 0, responseError(status, headers)
	}
	var result struct {
		Commits *[]struct {
			ID string `json:"id"`
		} `json:"commits"`
		Timeout *bool `json:"compare_timeout"`
	}
	if err = decode(body, &result); err != nil {
		return 0, err
	}
	if result.Commits == nil || result.Timeout == nil || *result.Timeout {
		return 0, failure("provider", "Missing or incomplete comparison result")
	}
	for _, commit := range *result.Commits {
		if !validSHA(commit.ID) {
			return 0, failure("provider", "Invalid comparison commit")
		}
	}
	return len(*result.Commits), nil
}
