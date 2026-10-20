package github

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
	out := forge.DeploymentGates{State: "unknown", NativeEnforced: domain.Unknown, Environment: environment, Blockers: []string{}}
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
	route, err := repositoryPath(r)
	if err != nil {
		return out, err
	}
	var raw struct {
		ID       int64              `json:"id"`
		Name     string             `json:"name"`
		URL      string             `json:"html_url"`
		Rules    *[]json.RawMessage `json:"protection_rules"`
		Branches json.RawMessage    `json:"deployment_branch_policy"`
	}
	if err = p.get(ctx, append(route, "environments", environment), nil, &raw); err != nil {
		return out, err
	}
	if raw.ID <= 0 || raw.Name != environment || raw.Rules == nil || len(raw.Branches) == 0 {
		return out, failure("identity", "Native environment protection is incomplete")
	}
	for _, body := range *raw.Rules {
		var rule struct {
			Type      string `json:"type"`
			Prevent   *bool  `json:"prevent_self_review"`
			Reviewers []struct {
				Type     string `json:"type"`
				Reviewer struct {
					ID int64 `json:"id"`
				} `json:"reviewer"`
			} `json:"reviewers"`
			Wait *int `json:"wait_timer"`
		}
		if json.Unmarshal(body, &rule) != nil {
			return out, failure("provider", "Environment rule is malformed")
		}
		switch rule.Type {
		case "required_reviewers":
			if rule.Prevent == nil || !*rule.Prevent || len(rule.Reviewers) == 0 {
				return out, failure("unsupported", "Native reviewers must prevent self-review")
			}
			for _, reviewer := range rule.Reviewers {
				if reviewer.Reviewer.ID <= 0 || (reviewer.Type != "User" && reviewer.Type != "Team") {
					return out, failure("identity", "Native reviewer identity is incomplete")
				}
			}
		case "wait_timer":
			if rule.Wait == nil || *rule.Wait < 0 {
				return out, failure("identity", "Native wait timer is incomplete")
			}
		case "branch_policy":
		default:
			return out, failure("unsupported", "Custom native deployment rules need a qualified inspection profile")
		}
	}
	var branches any
	if string(raw.Branches) != "null" {
		var v struct {
			Protected *bool `json:"protected_branches"`
			Custom    *bool `json:"custom_branch_policies"`
		}
		if json.Unmarshal(raw.Branches, &v) != nil || v.Protected == nil || v.Custom == nil {
			return out, failure("identity", "Deployment branch policy is incomplete")
		}
		if *v.Custom {
			var policies struct {
				Total int               `json:"total_count"`
				Items []json.RawMessage `json:"branch_policies"`
			}
			if err = p.get(ctx, append(route, "environments", environment, "deployment-branch-policies"), url.Values{"per_page": {"100"}}, &policies); err != nil {
				return out, err
			}
			if policies.Total != len(policies.Items) || policies.Total > 100 {
				return out, failure("incomplete", "Deployment branch policies are incomplete")
			}
			branches = policies.Items
		}
	}
	out.NativeEnforced = domain.Supported
	out.State = "configured"
	out.ApprovalURL = raw.URL
	out.RulesHash = forge.DeliveryRulesHash(struct {
		ID       int64
		Name     string
		Rules    []json.RawMessage
		Policy   json.RawMessage
		Branches any
	}{raw.ID, raw.Name, *raw.Rules, raw.Branches, branches})
	return out, nil
}

func (p *Provider) TriggerOrObservePipeline(ctx context.Context, in forge.PipelineRequest) (forge.DeploymentStatus, error) {
	if !forge.ValidPipelineRequest(in) {
		return forge.DeploymentStatus{}, failure("invalid", "Pinned workflow request is required")
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
	route, err := repositoryPath(in.Repository)
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	var workflow struct {
		ID    int64  `json:"id"`
		Path  string `json:"path"`
		State string `json:"state"`
	}
	if err = p.get(ctx, append(route, "actions", "workflows", in.WorkflowID), nil, &workflow); err != nil {
		return forge.DeploymentStatus{}, err
	}
	if strconv.FormatInt(workflow.ID, 10) != in.WorkflowID || workflow.Path != in.WorkflowPath || workflow.State != "active" {
		return forge.DeploymentStatus{}, failure("conflict", "Allowlisted workflow changed")
	}
	gates, err := p.ReadDeploymentGates(ctx, in.Repository, in.Environment)
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	if gates.NativeEnforced != domain.Supported || gates.RulesHash != in.RulesHash {
		return forge.DeploymentStatus{}, failure("conflict", "Native environment gates changed")
	}
	current, err := p.ResolveRef(ctx, in.Repository, in.Ref)
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	if current != in.WorkflowSHA {
		return forge.DeploymentStatus{}, failure("conflict", "Deployment ref moved")
	}
	if err = p.deliveryGuard(ctx, in, gates); err != nil {
		return forge.DeploymentStatus{}, err
	}
	inputs := map[string]string{"reforge_operation": in.CorrelationID, "reforge_workflow_sha": in.WorkflowSHA, "reforge_source_sha": in.SourceSHA, "reforge_artifact_digest": in.ArtifactDigest, "reforge_environment": in.Environment}
	for k, v := range in.Inputs {
		inputs[k] = v
	}
	ref := strings.TrimPrefix(strings.TrimPrefix(in.Ref, "refs/heads/"), "refs/tags/")
	body, _ := json.Marshal(map[string]any{"ref": ref, "inputs": inputs})
	status, headers, _, err := p.request(ctx, http.MethodPost, append(route, "actions", "workflows", in.WorkflowID, "dispatches"), nil, &requestBody{data: body})
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	if status != http.StatusNoContent {
		return forge.DeploymentStatus{}, responseError(status, headers)
	}
	return forge.DeploymentStatus{State: "reconciling", WorkflowSHA: in.WorkflowSHA, WorkflowID: in.WorkflowID, CorrelationID: in.CorrelationID, ObservedAt: time.Now().UTC()}, nil
}

func (p *Provider) observePipeline(ctx context.Context, in forge.PipelineRequest) (forge.DeploymentStatus, error) {
	if in.RunID != "" {
		out, err := p.ReadDeploymentStatus(ctx, in.Repository, in.RunID)
		if err != nil {
			return out, err
		}
		if !forge.PipelineMatches(in, out) || out.Event != "workflow_dispatch" {
			out.State = "unknown"
			return out, failure("conflict", "Native workflow execution does not match the deployment")
		}
		return out, nil
	}
	route, err := repositoryPath(in.Repository)
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	found := ""
	for page := 1; page <= 10; page++ {
		var runs struct {
			Total int `json:"total_count"`
			Items []struct {
				ID      int64     `json:"id"`
				Title   string    `json:"display_title"`
				SHA     string    `json:"head_sha"`
				Created time.Time `json:"created_at"`
			} `json:"workflow_runs"`
		}
		query := url.Values{"event": {"workflow_dispatch"}, "head_sha": {in.WorkflowSHA}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}
		if !in.RequestedAt.IsZero() {
			query.Set("created", ">="+in.RequestedAt.Add(-time.Minute).UTC().Format(time.RFC3339))
		}
		if err = p.get(ctx, append(route, "actions", "workflows", in.WorkflowID, "runs"), query, &runs); err != nil {
			return forge.DeploymentStatus{}, err
		}
		if runs.Total > 1000 {
			return forge.DeploymentStatus{}, failure("incomplete", "Too many workflow runs to reconcile safely")
		}
		for _, run := range runs.Items {
			if run.Title != "reforge:"+in.CorrelationID {
				continue
			}
			if run.ID <= 0 || run.SHA != in.WorkflowSHA || run.Created.IsZero() || !in.RequestedAt.IsZero() && run.Created.Before(in.RequestedAt.Add(-time.Minute)) {
				return forge.DeploymentStatus{}, failure("identity", "Deployment run correlation is incomplete")
			}
			if found != "" {
				return forge.DeploymentStatus{}, failure("conflict", "Multiple native runs share this operation; inspect native history")
			}
			found = strconv.FormatInt(run.ID, 10)
		}
		if len(runs.Items) < 100 || page*100 >= runs.Total {
			if found == "" {
				return forge.DeploymentStatus{State: "reconciling", CorrelationID: in.CorrelationID, ObservedAt: time.Now().UTC()}, nil
			}
			in.RunID = found
			return p.observePipeline(ctx, in)
		}
	}
	return forge.DeploymentStatus{}, failure("incomplete", "Workflow reconciliation page limit reached")
}

func (p *Provider) RequestAllowedRecovery(ctx context.Context, in forge.PipelineRequest) (forge.DeploymentStatus, error) {
	return p.TriggerOrObservePipeline(ctx, in)
}

func (p *Provider) CancelPipeline(ctx context.Context, in forge.PipelineRequest) (forge.DeploymentStatus, error) {
	if !forge.ValidPipelineRequest(in) || !positive(in.RunID) || in.ObserveOnly || p.deliveryGuard == nil {
		return forge.DeploymentStatus{}, failure("forbidden", "Exact native run and cancellation authority are required")
	}
	current, err := p.ReadDeploymentStatus(ctx, in.Repository, in.RunID)
	if err != nil {
		return current, err
	}
	if !forge.PipelineMatches(in, current) || current.Event != "workflow_dispatch" {
		return forge.DeploymentStatus{}, failure("conflict", "Cancellation run identity changed")
	}
	if current.State == "cancelled" || current.State == "success" || current.State == "failed" {
		return current, nil
	}
	if err = p.deliveryGuard(ctx, in, forge.DeploymentGates{Environment: in.Environment, RulesHash: in.RulesHash}); err != nil {
		return forge.DeploymentStatus{}, err
	}
	route, err := repositoryPath(in.Repository)
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	status, headers, _, err := p.request(ctx, http.MethodPost, append(route, "actions", "runs", in.RunID, "cancel"), nil, &requestBody{data: []byte(`{}`)})
	if err != nil {
		return forge.DeploymentStatus{}, err
	}
	if status != http.StatusAccepted {
		return forge.DeploymentStatus{}, responseError(status, headers)
	}
	in.ObserveOnly = true
	return p.observePipeline(ctx, in)
}
