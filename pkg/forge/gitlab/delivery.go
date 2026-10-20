package gitlab

import (
	"context"
	"encoding/json"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (p *Provider) WithDeliveryGuard(guard forge.DeliveryGuard) *Provider {
	q := *p
	q.deliveryGuard = guard
	return &q
}
func (p *Provider) ReadDeploymentGates(ctx context.Context, r forge.RepoRef, environment string) (forge.DeploymentGates, error) {
	out := forge.DeploymentGates{State: "unknown", Environment: environment, NativeEnforced: domain.Unknown, Blockers: []string{}}
	if !forge.ValidEnvironment(environment) {
		return out, failure("invalid", "Deployment environment is invalid")
	}
	repo, err := p.GetRepository(ctx, r)
	if err != nil {
		return out, err
	}
	if repo.RepoRef != r {
		return out, failure("identity", "Deployment repository changed")
	}
	rows, err := p.pages(ctx, []string{"projects", r.NativeID, "protected_environments"}, nil)
	if err != nil {
		return out, err
	}
	var matched json.RawMessage
	for _, raw := range rows {
		var environmentRule struct {
			Name     string             `json:"name"`
			Access   *[]json.RawMessage `json:"deploy_access_levels"`
			Required *int               `json:"required_approval_count"`
			Rules    *[]struct {
				Required *int `json:"required_approvals"`
			} `json:"approval_rules"`
		}
		if json.Unmarshal(raw, &environmentRule) != nil || environmentRule.Name == "" {
			return out, failure("identity", "Protected environment is incomplete")
		}
		if !matches(environmentRule.Name, environment) {
			continue
		}
		if environmentRule.Name != environment || matched != nil {
			return out, failure("unsupported", "Overlapping protected environment patterns need qualification")
		}
		if environmentRule.Access == nil || len(*environmentRule.Access) == 0 || environmentRule.Required == nil || *environmentRule.Required < 0 || environmentRule.Rules == nil {
			return out, failure("identity", "Environment deployment and approval rules are incomplete")
		}
		for _, rule := range *environmentRule.Rules {
			if rule.Required == nil || *rule.Required < 1 {
				return out, failure("identity", "Environment approval requirement is incomplete")
			}
		}
		matched = raw
	}
	if matched == nil {
		return out, failure("unsupported", "Configure a protected native deployment environment")
	}
	out.State = "configured"
	out.NativeEnforced = domain.Supported
	out.RulesHash = forge.DeliveryRulesHash(matched)
	out.ApprovalURL = repo.URL + "/-/environments"
	return out, nil
}

func (p *Provider) TriggerOrObservePipeline(ctx context.Context, in forge.PipelineRequest) (forge.DeploymentStatus, error) {
	if !forge.ValidPipelineRequest(in) || in.WorkflowID != "pipeline" {
		return forge.DeploymentStatus{}, failure("invalid", "Pinned GitLab pipeline request is required")
	}
	if in.ObserveOnly {
		return p.observePipeline(ctx, in)
	}
	if p.deliveryGuard == nil {
		return forge.DeploymentStatus{}, failure("forbidden", "Delivery controller authority is required")
	}
	if err := forge.VerifyPipelineConfiguration(ctx, p, in); err != nil {
		return forge.DeploymentStatus{}, err
	}
	workflows, err := p.ListAllowedWorkflows(ctx, in.Repository)
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	if len(workflows) != 1 || workflows[0].Path != in.WorkflowPath {
		return forge.DeploymentStatus{}, failure("conflict", "Allowlisted pipeline configuration changed")
	}
	gates, err := p.ReadDeploymentGates(ctx, in.Repository, in.Environment)
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	if gates.NativeEnforced != domain.Supported || gates.RulesHash != in.RulesHash {
		return forge.DeploymentStatus{}, failure("conflict", "Native deployment environment changed")
	}
	kind, name, err := refParts(in.Ref)
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	var ref struct {
		Protected *bool `json:"protected"`
		Commit    struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err = p.read(ctx, []string{"projects", in.Repository.NativeID, "repository", kind, name}, &ref); err != nil {
		return forge.DeploymentStatus{}, err
	}
	if ref.Protected == nil || !*ref.Protected || ref.Commit.ID != in.WorkflowSHA {
		return forge.DeploymentStatus{}, failure("conflict", "Deployment requires the pinned protected ref")
	}
	if err = p.deliveryGuard(ctx, in, gates); err != nil {
		return forge.DeploymentStatus{}, err
	}
	variables := []map[string]string{{"key": "REFORGE_WORKFLOW_SHA", "value": in.WorkflowSHA, "variable_type": "env_var"}, {"key": "REFORGE_OPERATION", "value": in.CorrelationID, "variable_type": "env_var"}, {"key": "REFORGE_SOURCE_SHA", "value": in.SourceSHA, "variable_type": "env_var"}, {"key": "REFORGE_ARTIFACT_DIGEST", "value": in.ArtifactDigest, "variable_type": "env_var"}, {"key": "REFORGE_ENVIRONMENT", "value": in.Environment, "variable_type": "env_var"}}
	for k, v := range in.Inputs {
		variables = append(variables, map[string]string{"key": k, "value": v, "variable_type": "env_var"})
	}
	payload, _ := json.Marshal(map[string]any{"ref": name, "variables": variables})
	status, headers, body, err := p.request(ctx, http.MethodPost, []string{"projects", in.Repository.NativeID, "pipeline"}, nil, &requestBody{data: payload})
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	if status != http.StatusCreated {
		return forge.DeploymentStatus{}, responseError(status, headers)
	}
	var run struct {
		ID        int64  `json:"id"`
		SHA       string `json:"sha"`
		ProjectID int64  `json:"project_id"`
	}
	if decode(body, &run) != nil || run.ID <= 0 || run.SHA != in.WorkflowSHA || stringID(run.ProjectID) != in.Repository.NativeID {
		return forge.DeploymentStatus{}, failure("uncertain", "Pipeline accepted without complete matching identity; reconcile")
	}
	return forge.DeploymentStatus{ID: stringID(run.ID), State: "queued", WorkflowSHA: in.WorkflowSHA, WorkflowID: in.WorkflowID, WorkflowPath: in.WorkflowPath, Ref: in.Ref, Event: "api", RunAttempt: 1, CorrelationID: in.CorrelationID, ObservedAt: time.Now().UTC()}, nil
}

func (p *Provider) observePipeline(ctx context.Context, in forge.PipelineRequest) (forge.DeploymentStatus, error) {
	if in.RunID != "" {
		out, err := p.ReadDeploymentStatus(ctx, in.Repository, in.RunID)
		if err != nil {
			return out, err
		}
		if !forge.PipelineMatches(in, out) || out.Event != "api" {
			out.State = "unknown"
			return out, failure("conflict", "Native pipeline execution does not match deployment")
		}
		return out, nil
	}
	found := ""
	examined := 0
	for page := 1; page <= 10; page++ {
		status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", in.Repository.NativeID, "pipelines"}, url.Values{"sha": {in.WorkflowSHA}, "source": {"api"}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}, nil)
		if err != nil {
			return forge.DeploymentStatus{}, err
		}
		if status != http.StatusOK {
			return forge.DeploymentStatus{}, responseError(status, headers)
		}
		var rows []struct {
			ID      int64     `json:"id"`
			SHA     string    `json:"sha"`
			Ref     string    `json:"ref"`
			Created time.Time `json:"created_at"`
		}
		if err = decode(body, &rows); err != nil {
			return forge.DeploymentStatus{}, err
		}
		for _, row := range rows {
			if row.ID <= 0 || row.SHA != in.WorkflowSHA {
				return forge.DeploymentStatus{}, failure("identity", "Pipeline listing has inconsistent source identity")
			}
			if !row.Created.IsZero() && !in.RequestedAt.IsZero() && row.Created.Before(in.RequestedAt.Add(-time.Minute)) {
				continue
			}
			if row.Ref != strings.TrimPrefix(strings.TrimPrefix(in.Ref, "refs/heads/"), "refs/tags/") {
				continue
			}
			examined++
			if examined > 100 {
				return forge.DeploymentStatus{}, failure("incomplete", "Too many candidate pipelines to reconcile safely")
			}
			run, err := p.ReadDeploymentStatus(ctx, in.Repository, stringID(row.ID))
			if err != nil {
				return forge.DeploymentStatus{}, err
			}
			if run.CorrelationID != in.CorrelationID {
				continue
			}
			if found != "" {
				return forge.DeploymentStatus{}, failure("conflict", "Multiple pipelines share this operation; inspect native history")
			}
			if !forge.PipelineMatches(in, run) {
				return forge.DeploymentStatus{}, failure("conflict", "Correlated pipeline source or attempt changed")
			}
			found = run.ID
		}
		next := headers.Get("X-Next-Page")
		if next == "" && len(rows) < 100 || next == "" && headers.Get("X-Page") != "" {
			if found == "" {
				return forge.DeploymentStatus{State: "reconciling", CorrelationID: in.CorrelationID, ObservedAt: time.Now().UTC()}, nil
			}
			in.RunID = found
			return p.observePipeline(ctx, in)
		}
		if next != "" && next != strconv.Itoa(page+1) {
			return forge.DeploymentStatus{}, failure("incomplete", "Pipeline pagination is inconsistent")
		}
	}
	return forge.DeploymentStatus{}, failure("incomplete", "Pipeline reconciliation page limit reached")
}
func (p *Provider) RequestAllowedRecovery(ctx context.Context, in forge.PipelineRequest) (forge.DeploymentStatus, error) {
	return p.TriggerOrObservePipeline(ctx, in)
}

func (p *Provider) CancelPipeline(ctx context.Context, in forge.PipelineRequest) (forge.DeploymentStatus, error) {
	if !forge.ValidPipelineRequest(in) || !safeID(in.RunID) || in.ObserveOnly || p.deliveryGuard == nil {
		return forge.DeploymentStatus{}, failure("forbidden", "Exact pipeline and cancellation authority are required")
	}
	current, err := p.ReadDeploymentStatus(ctx, in.Repository, in.RunID)
	if err != nil {
		return current, err
	}
	if !forge.PipelineMatches(in, current) || current.Event != "api" {
		return forge.DeploymentStatus{}, failure("conflict", "Cancellation pipeline identity changed")
	}
	if current.State == "cancelled" || current.State == "success" || current.State == "failed" {
		return current, nil
	}
	if err = p.deliveryGuard(ctx, in, forge.DeploymentGates{Environment: in.Environment, RulesHash: in.RulesHash}); err != nil {
		return forge.DeploymentStatus{}, err
	}
	status, headers, _, err := p.request(ctx, http.MethodPost, []string{"projects", in.Repository.NativeID, "pipelines", in.RunID, "cancel"}, nil, &requestBody{data: []byte(`{}`)})
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	if status != http.StatusOK {
		return forge.DeploymentStatus{}, responseError(status, headers)
	}
	in.ObserveOnly = true
	return p.observePipeline(ctx, in)
}
