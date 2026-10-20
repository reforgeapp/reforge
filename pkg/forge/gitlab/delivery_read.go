package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/forge"
)

func (p *Provider) ListAllowedWorkflows(ctx context.Context, reference forge.RepoRef) ([]forge.Workflow, error) {
	project, err := p.projectSegment(reference)
	if err != nil {
		return nil, err
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project}, nil, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, responseError(status, headers)
	}
	var raw struct {
		ID                int64           `json:"id"`
		PathWithNamespace string          `json:"path_with_namespace"`
		DefaultBranch     string          `json:"default_branch"`
		WebURL            string          `json:"web_url"`
		CIConfigPath      json.RawMessage `json:"ci_config_path"`
	}
	if err = decode(body, &raw); err != nil {
		return nil, err
	}
	if !safeID(strconv.FormatInt(raw.ID, 10)) || raw.PathWithNamespace == "" || reference.NativeID != "" && reference.NativeID != stringID(raw.ID) || reference.FullName != "" && reference.FullName != raw.PathWithNamespace || raw.DefaultBranch == "" || !validHTTPURL(raw.WebURL) || raw.CIConfigPath == nil {
		return nil, failure("protocol", "GitLab workflow project identity is incomplete or mismatched")
	}
	path := ""
	if string(raw.CIConfigPath) != "null" {
		if err := json.Unmarshal(raw.CIConfigPath, &path); err != nil {
			return nil, failure("protocol", "GitLab CI configuration path is invalid")
		}
	}
	if path == "" {
		path = ".gitlab-ci.yml"
	}
	if !validWorkflowPath(path) {
		return nil, failure("protocol", "GitLab CI configuration path is invalid")
	}
	return []forge.Workflow{{ID: "pipeline", Name: "GitLab pipeline", Ref: raw.DefaultBranch, Path: path, URL: strings.TrimRight(raw.WebURL, "/") + "/-/pipelines"}}, nil
}

func (p *Provider) ReadDeploymentStatus(ctx context.Context, reference forge.RepoRef, runID string) (forge.DeploymentStatus, error) {
	var out forge.DeploymentStatus
	project, err := p.projectSegment(reference)
	if err != nil || !safeID(runID) {
		return out, failure("invalid", "GitLab pipeline identity is invalid")
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "pipelines", runID}, nil, nil)
	if err != nil {
		return out, err
	}
	if status < 200 || status >= 300 {
		return out, responseError(status, headers)
	}
	var pipeline struct {
		pipelineRecord
		WebURL    string    `json:"web_url"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err = decode(body, &pipeline); err != nil {
		return out, err
	}
	if pipeline.ID <= 0 || stringID(pipeline.ID) != runID || reference.NativeID != stringID(pipeline.ProjectID) || !validSHA(pipeline.SHA) || pipeline.Ref == "" || pipeline.Source == "" || !validHTTPURL(pipeline.WebURL) || pipeline.CreatedAt.IsZero() || pipeline.UpdatedAt.IsZero() {
		return out, failure("protocol", "GitLab pipeline identity is incomplete or mismatched")
	}
	if err = p.pipelineBridges(ctx, project, runID); err != nil {
		return out, err
	}
	attempt, err := p.pipelineAttempt(ctx, project, runID, pipeline.ID, pipeline.ProjectID, pipeline.SHA)
	if err != nil {
		return out, err
	}
	correlation, err := p.pipelineCorrelation(ctx, project, runID)
	if err != nil {
		return out, err
	}
	workflows, err := p.ListAllowedWorkflows(ctx, reference)
	if err != nil || len(workflows) != 1 {
		if err == nil {
			err = failure("identity", "Pipeline workflow path is unknown")
		}
		return out, err
	}
	out = forge.DeploymentStatus{ID: runID, WorkflowSHA: pipeline.SHA, CorrelationID: correlation, WorkflowID: "pipeline", WorkflowPath: workflows[0].Path, Ref: pipeline.Ref, Event: pipeline.Source, RunAttempt: attempt, URL: pipeline.WebURL, State: mapPipelineState(pipeline.Status), CreatedAt: pipeline.CreatedAt, UpdatedAt: pipeline.UpdatedAt, ObservedAt: time.Now().UTC()}
	return out, nil
}

func (p *Provider) pipelineBridges(ctx context.Context, project, runID string) error {
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "pipelines", runID, "bridges"}, url.Values{"page": {"1"}, "per_page": {"100"}}, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return responseError(status, headers)
	}
	if strings.TrimSpace(string(body)) == "null" {
		return failure("protocol", "GitLab pipeline bridges collection is missing")
	}
	var bridges []json.RawMessage
	if err = decode(body, &bridges); err != nil {
		return err
	}
	if len(bridges) > 0 {
		return failure("unsupported", "GitLab child pipelines are not supported")
	}
	return nil
}

func (p *Provider) pipelineAttempt(ctx context.Context, project, runID string, pipelineID, projectID int64, sha string) (int64, error) {
	type job struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Stage    string `json:"stage"`
		Status   string `json:"status"`
		Retried  bool   `json:"retried"`
		Type     string `json:"type"`
		Pipeline *struct {
			ID        int64  `json:"id"`
			ProjectID int64  `json:"project_id"`
			SHA       string `json:"sha"`
		} `json:"pipeline"`
		Commit *struct {
			ID string `json:"id"`
		} `json:"commit"`
		Downstream json.RawMessage `json:"downstream_pipeline"`
	}
	seen := map[string]int64{}
	var attempt int64 = 1
	count := 0
	for page := 1; page <= 10; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "per_page": {"100"}, "include_retried": {"true"}}
		status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "pipelines", runID, "jobs"}, query, nil)
		if err != nil {
			return 0, err
		}
		if status < 200 || status >= 300 {
			return 0, responseError(status, headers)
		}
		var jobs []job
		if err = decode(body, &jobs); err != nil {
			return 0, err
		}
		if strings.TrimSpace(string(body)) == "null" {
			return 0, failure("protocol", "GitLab pipeline jobs collection is missing")
		}
		count += len(jobs)
		if count > 1000 {
			return 0, failure("protocol", "GitLab pipeline job list exceeds bounded limit")
		}
		for _, item := range jobs {
			if item.ID <= 0 || item.Name == "" || item.Stage == "" || strings.EqualFold(item.Type, "bridge") || len(item.Downstream) > 0 && string(item.Downstream) != "null" || item.Pipeline == nil || item.Pipeline.ID != pipelineID || item.Pipeline.ProjectID != projectID || item.Pipeline.SHA != sha || item.Commit == nil || item.Commit.ID != sha {
				return 0, failure("protocol", "GitLab pipeline job identity is incomplete or mismatched")
			}
			key := item.Name + "\x00" + item.Stage
			if item.Retried || seen[key] != 0 && seen[key] != item.ID {
				attempt = 2
			}
			seen[key] = item.ID
		}
		next, err := nextPage(headers, page)
		if err != nil {
			return 0, err
		}
		if page == 10 && next != "" {
			return 0, failure("protocol", "GitLab pipeline job pagination exceeds bounded limit")
		}
		if next == "" {
			break
		}
	}
	return attempt, nil
}

func (p *Provider) pipelineCorrelation(ctx context.Context, project, runID string) (string, error) {
	values := []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}{}
	for page := 1; page <= 10; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "per_page": {"100"}}
		status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "pipelines", runID, "variables"}, query, nil)
		if err != nil {
			return "", err
		}
		if status < 200 || status >= 300 {
			return "", responseError(status, headers)
		}
		var pageValues []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err = decode(body, &pageValues); err != nil {
			return "", err
		}
		if strings.TrimSpace(string(body)) == "null" {
			return "", failure("protocol", "GitLab pipeline variables collection is missing")
		}
		values = append(values, pageValues...)
		next, err := nextPage(headers, page)
		if err != nil {
			return "", err
		}
		if next == "" {
			break
		}
		if page == 10 {
			return "", failure("protocol", "GitLab pipeline variable pagination exceeds bounded limit")
		}
	}
	correlation := ""
	for _, variable := range values {
		if variable.Key != "REFORGE_OPERATION" {
			continue
		}
		if !auth.ValidID(variable.Value) {
			return "", failure("protocol", "GitLab pipeline correlation is invalid")
		}
		if correlation != "" {
			return "", failure("protocol", "GitLab pipeline correlation is duplicated")
		}
		correlation = variable.Value
	}
	return correlation, nil
}

func validWorkflowPath(value string) bool {
	return value != "" && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && !strings.Contains(value, "..") && !strings.Contains(value, "://") && !strings.Contains(value, "@") && !strings.ContainsAny(value, "\x00\r\n")
}

func validHTTPURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.Fragment == "" && u.RawQuery == ""
}

func mapPipelineState(status string) string {
	switch strings.ToLower(status) {
	case "created", "pending", "preparing", "scheduled", "waiting_for_resource":
		return "queued"
	case "running":
		return "running"
	case "manual", "blocked":
		return "awaiting_approval"
	case "success":
		return "success"
	case "failed":
		return "failed"
	case "canceled", "cancelled":
		return "cancelled"
	default:
		return "unknown"
	}
}
