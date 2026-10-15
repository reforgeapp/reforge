package gitea

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/reforgeapp/reforge/pkg/forge"
)

func (p *Provider) actionsCheck(ctx context.Context, repository forge.RepoRef, status status) (forge.Check, error) {
	u, err := url.Parse(status.URL)
	base, _ := url.Parse(p.base)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.IsAbs() || u.Host != "") && (u.Host != base.Host || u.Scheme != base.Scheme) {
		return forge.Check{}, failure("identity", "Native Actions status location is invalid")
	}
	prefix := strings.TrimSuffix(base.Path, "/api/v1") + "/" + repository.FullName + "/actions/runs/"
	parts := strings.Split(strings.TrimPrefix(u.Path, prefix), "/")
	if !strings.HasPrefix(u.Path, prefix) || len(parts) != 3 || parts[1] != "jobs" || !positive(parts[0]) || !positive(parts[2]) {
		return forge.Check{}, failure("identity", "Native Actions status identity is missing")
	}
	route, _ := repoPath(repository)
	var job struct {
		ID         int64  `json:"id"`
		RunID      int64  `json:"run_id"`
		HeadSHA    string `json:"head_sha"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}
	if err := p.request(ctx, "GET", route+"/actions/jobs/"+parts[2], nil, &job); err != nil {
		return forge.Check{}, err
	}
	if strconv.FormatInt(job.ID, 10) != parts[2] || strconv.FormatInt(job.RunID, 10) != parts[0] || !sha(job.HeadSHA) || job.Status == "" {
		return forge.Check{}, failure("identity", "Native Actions job does not match the pinned status")
	}
	u.Scheme, u.Host = base.Scheme, base.Host
	return forge.Check{ID: "actions:" + parts[2], Name: status.Context, PublisherID: "gitea-actions", HeadSHA: job.HeadSHA, Status: job.Status, Conclusion: job.Conclusion, URL: u.String()}, nil
}

func (p *Provider) ReadCheckLog(ctx context.Context, repository forge.RepoRef, checkID string) (string, error) {
	id, ok := strings.CutPrefix(checkID, "actions:")
	if !ok || !positive(id) {
		return "", failure("unsupported", "Only native Actions job logs are available")
	}
	if _, err := p.repo(ctx, repository); err != nil {
		return "", err
	}
	route, _ := repoPath(repository)
	var body []byte
	if err := p.request(ctx, "GET", route+"/actions/jobs/"+id+"/logs", nil, &body); err != nil {
		return "", err
	}
	return forge.LogTail(body), nil
}
