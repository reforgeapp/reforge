package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"reforge/internal/auth"
	"reforge/internal/forge"
)

type githubWorkflow struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	State   string `json:"state"`
	URL     string `json:"url"`
	HTMLURL string `json:"html_url"`
}

type githubWorkflowRun struct {
	ID           int64     `json:"id"`
	HeadSHA      string    `json:"head_sha"`
	HeadBranch   string    `json:"head_branch"`
	Path         string    `json:"path"`
	Event        string    `json:"event"`
	Status       string    `json:"status"`
	Conclusion   *string   `json:"conclusion"`
	WorkflowID   int64     `json:"workflow_id"`
	DisplayTitle string    `json:"display_title"`
	RunAttempt   int64     `json:"run_attempt"`
	HTMLURL      string    `json:"html_url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Repository   struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (p *Provider) ListAllowedWorkflows(ctx context.Context, reference forge.RepoRef) ([]forge.Workflow, error) {
	repository, err := p.GetRepository(ctx, reference)
	if err != nil {
		return nil, err
	}
	if repository.NativeID != reference.NativeID || repository.FullName != reference.FullName {
		return nil, failure("identity", "GitHub repository identity changed")
	}
	route, err := repositoryPath(reference)
	if err != nil {
		return nil, err
	}
	route = append(route, "actions", "workflows")
	workflows := make([]forge.Workflow, 0)
	seen := map[string]bool{}
	for page := 1; page <= maxPages; {
		query := url.Values{"per_page": {strconv.Itoa(maxPageSize)}, "page": {strconv.Itoa(page)}}
		status, headers, body, requestErr := p.request(ctx, http.MethodGet, route, query, nil)
		if requestErr != nil {
			return nil, requestErr
		}
		if status < 200 || status >= 300 {
			return nil, responseError(status, headers)
		}
		var response struct {
			Workflows []githubWorkflow `json:"workflows"`
		}
		if err := decode(body, &response); err != nil || response.Workflows == nil || len(response.Workflows) > maxPageSize {
			return nil, failure("provider", "GitHub workflow collection is incomplete")
		}
		for _, workflow := range response.Workflows {
			if workflow.ID <= 0 || workflow.Path == "" || workflow.Name == "" || workflow.State == "" {
				return nil, failure("identity", "GitHub workflow identity is incomplete")
			}
			if workflow.State != "active" {
				continue
			}
			id := strconv.FormatInt(workflow.ID, 10)
			if seen[id] || seen[workflow.Path] {
				return nil, failure("identity", "GitHub workflow identity is duplicated")
			}
			seen[id], seen[workflow.Path] = true, true
			workflowURL := workflow.URL
			if workflowURL == "" {
				workflowURL = workflow.HTMLURL
			}
			workflows = append(workflows, forge.Workflow{ID: id, Name: workflow.Name, Path: workflow.Path, URL: workflowURL})
		}
		next, err := p.nextCursor(headers, page)
		if err != nil {
			return nil, err
		}
		if next == "" {
			return workflows, nil
		}
		page, err = strconv.Atoi(next)
		if err != nil || page <= 0 {
			return nil, failure("provider", "GitHub workflow pagination cursor is invalid")
		}
	}
	return nil, failure("provider", "GitHub workflow pagination exceeded the safety bound")
}

func (p *Provider) ReadDeploymentStatus(ctx context.Context, reference forge.RepoRef, runID string) (forge.DeploymentStatus, error) {
	if !positive(reference.NativeID) || !positive(runID) {
		return forge.DeploymentStatus{}, failure("invalid", "Immutable repository and Actions run ID are required")
	}
	route, err := repositoryPath(reference)
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	var run githubWorkflowRun
	if err = p.get(ctx, append(route, "actions", "runs", runID), nil, &run); err != nil {
		return forge.DeploymentStatus{}, err
	}
	if run.ID != mustPositive(runID) || run.Repository.ID != mustPositive(reference.NativeID) || run.Repository.FullName != reference.FullName || !validSHA(run.HeadSHA) || run.WorkflowID <= 0 || run.Path == "" || run.HeadBranch == "" || run.Event == "" || run.RunAttempt <= 0 || !validCanonicalURL(run.HTMLURL) || run.CreatedAt.IsZero() || run.UpdatedAt.IsZero() {
		return forge.DeploymentStatus{}, failure("identity", "GitHub Actions run identity or evidence is incomplete")
	}
	state := workflowState(run.Status, run.Conclusion)
	correlation := ""
	if strings.HasPrefix(run.DisplayTitle, "reforge:") {
		candidate := strings.TrimPrefix(run.DisplayTitle, "reforge:")
		if auth.ValidID(candidate) {
			correlation = candidate
		}
	}
	return forge.DeploymentStatus{ID: strconv.FormatInt(run.ID, 10), State: state, WorkflowSHA: run.HeadSHA, WorkflowID: strconv.FormatInt(run.WorkflowID, 10), WorkflowPath: run.Path, Ref: run.HeadBranch, Event: run.Event, RunAttempt: run.RunAttempt, CorrelationID: correlation, URL: run.HTMLURL, Health: "unknown", CreatedAt: run.CreatedAt, ObservedAt: time.Now().UTC(), UpdatedAt: run.UpdatedAt}, nil
}

func workflowState(status string, conclusion *string) string {
	switch status {
	case "queued", "requested":
		return "queued"
	case "waiting":
		return "awaiting_approval"
	case "pending":
		return "queued"
	case "in_progress":
		return "running"
	case "completed":
		if conclusion == nil {
			return "unknown"
		}
		switch *conclusion {
		case "success":
			return "success"
		case "cancelled":
			return "cancelled"
		case "failure", "timed_out", "neutral", "skipped", "stale":
			return "failed"
		default:
			return "unknown"
		}
	default:
		return "unknown"
	}
}

func validCanonicalURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func mustPositive(value string) int64 {
	result, _ := strconv.ParseInt(value, 10, 64)
	return result
}

var _ forge.ForgeDelivery = (*Provider)(nil)
