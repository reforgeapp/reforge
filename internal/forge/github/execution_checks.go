package github

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

type githubExecutionCheck struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	HeadSHA    string `json:"head_sha"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	ExternalID string `json:"external_id"`
	App        struct {
		ID int64 `json:"id"`
	} `json:"app"`
}

func (p *Provider) WriteExecutionCheck(ctx context.Context, in forge.ExecutionCheckRequest) (forge.ExecutionCheck, error) {
	if err := validateExecutionCheck(in); err != nil {
		return forge.ExecutionCheck{}, err
	}
	if p.app == nil {
		return forge.ExecutionCheck{}, failure("unsupported", "GitHub check runs require the GitHub App")
	}
	if _, err := p.GetRepository(ctx, in.Repository); err != nil {
		return forge.ExecutionCheck{}, err
	}
	if _, err := p.authenticatedBot(ctx); err != nil {
		return forge.ExecutionCheck{}, err
	}
	appID := p.appID()
	if in.CheckID != "" {
		current, err := p.readExecutionCheck(ctx, in.Repository, in.CheckID)
		if err != nil {
			return forge.ExecutionCheck{}, err
		}
		if current.HeadSHA != in.SHA || current.Name != in.Name || current.ExternalID != in.OperationID || strconv.FormatInt(current.App.ID, 10) != appID {
			return forge.ExecutionCheck{}, failure("identity", "GitHub check run identity changed")
		}
	}
	status, conclusion, _ := executionCheckState(in.State)
	headSHA := in.SHA
	if in.CheckID != "" {
		headSHA = ""
	}
	body, err := json.Marshal(struct {
		Name       string `json:"name"`
		HeadSHA    string `json:"head_sha,omitempty"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion,omitempty"`
		ExternalID string `json:"external_id,omitempty"`
	}{Name: in.Name, HeadSHA: headSHA, Status: status, Conclusion: conclusion, ExternalID: in.OperationID})
	if err != nil {
		return forge.ExecutionCheck{}, failure("provider", "GitHub check request encoding failed")
	}
	segments, err := repositoryPath(in.Repository)
	if err != nil {
		return forge.ExecutionCheck{}, err
	}
	segments = append(segments, "check-runs")
	method := http.MethodPost
	if in.CheckID != "" {
		method = http.MethodPatch
		segments = append(segments, in.CheckID)
	}
	statusCode, headers, raw, err := p.request(ctx, method, segments, nil, &requestBody{data: body})
	if err != nil {
		return forge.ExecutionCheck{}, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return forge.ExecutionCheck{}, responseError(statusCode, headers)
	}
	var check githubExecutionCheck
	if err := decode(raw, &check); err != nil {
		return forge.ExecutionCheck{}, uncertainCheckMutation("GitHub check run response was invalid")
	}
	if in.CheckID != "" {
		id, _ := strconv.ParseInt(in.CheckID, 10, 64)
		if check.ID != id {
			return forge.ExecutionCheck{}, uncertainCheckMutation("GitHub check run response identity changed")
		}
	}
	created, err := validateExecutionResponse(check, in.Name, in.SHA, in.OperationID, appID, in.State)
	if err != nil {
		return forge.ExecutionCheck{}, uncertainCheckMutation("GitHub check run response identity or state was invalid")
	}
	canonical, err := p.readExecutionCheck(ctx, in.Repository, created.ID)
	if err != nil {
		return forge.ExecutionCheck{}, uncertainCheckMutation("GitHub check run canonical read failed")
	}
	result, err := validateExecutionResponse(canonical, in.Name, in.SHA, in.OperationID, appID, in.State)
	if err != nil {
		return forge.ExecutionCheck{}, uncertainCheckMutation("GitHub check run canonical identity or state changed")
	}
	return result, nil
}

func (p *Provider) ReadExecutionCheck(ctx context.Context, repository forge.RepoRef, id string) (forge.ExecutionCheck, error) {
	if id == "" {
		return forge.ExecutionCheck{}, failure("invalid", "GitHub check run identity is invalid")
	}
	if _, err := validateCheckIdentity(repository, id); err != nil {
		return forge.ExecutionCheck{}, err
	}
	if _, err := p.GetRepository(ctx, repository); err != nil {
		return forge.ExecutionCheck{}, err
	}
	if p.app == nil {
		return forge.ExecutionCheck{}, failure("unsupported", "GitHub check runs require the GitHub App")
	}
	if _, err := p.authenticatedBot(ctx); err != nil {
		return forge.ExecutionCheck{}, err
	}
	appID := p.appID()
	check, err := p.readExecutionCheck(ctx, repository, id)
	if err != nil {
		return forge.ExecutionCheck{}, err
	}
	return validateExecutionResponse(check, check.Name, check.HeadSHA, check.ExternalID, appID, "")
}

func (p *Provider) readExecutionCheck(ctx context.Context, repository forge.RepoRef, id string) (githubExecutionCheck, error) {
	segments, err := repositoryPath(repository)
	if err != nil {
		return githubExecutionCheck{}, err
	}
	segments = append(segments, "check-runs", id)
	status, headers, raw, err := p.request(ctx, http.MethodGet, segments, nil, nil)
	if err != nil {
		return githubExecutionCheck{}, err
	}
	if status < 200 || status >= 300 {
		return githubExecutionCheck{}, responseError(status, headers)
	}
	var check githubExecutionCheck
	if err := decode(raw, &check); err != nil {
		return githubExecutionCheck{}, err
	}
	parsed, _ := strconv.ParseInt(id, 10, 64)
	if check.ID != parsed {
		return githubExecutionCheck{}, failure("identity", "GitHub check run identity changed")
	}
	return check, nil
}

func validateExecutionCheck(in forge.ExecutionCheckRequest) error {
	if _, err := validateCheckIdentity(in.Repository, in.CheckID); err != nil {
		return err
	}
	if !validSHA(in.SHA) || strings.TrimSpace(in.Name) != in.Name || in.Name == "" || len(in.Name) > 100 || !auth.ValidID(in.OperationID) {
		return failure("invalid", "GitHub execution check identity is invalid")
	}
	if _, _, ok := executionCheckState(in.State); !ok {
		return failure("invalid", "GitHub execution check state is invalid")
	}
	return nil
}

func validateCheckIdentity(repository forge.RepoRef, id string) (int64, error) {
	if !positive(repository.NativeID) || len(strings.Split(repository.FullName, "/")) != 2 {
		return 0, failure("invalid", "GitHub repository identity is invalid")
	}
	if id == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != id {
		return 0, failure("invalid", "GitHub check run identity is invalid")
	}
	return n, nil
}

func executionCheckState(state string) (string, string, bool) {
	switch state {
	case "pending":
		return "in_progress", "", true
	case "success":
		return "completed", "success", true
	case "failure":
		return "completed", "failure", true
	default:
		return "", "", false
	}
}

func validateExecutionResponse(check githubExecutionCheck, name, sha, operationID, appID, expectedState string) (forge.ExecutionCheck, error) {
	if check.ID <= 0 || check.Name != name || check.HeadSHA != sha || !validSHA(check.HeadSHA) || !auth.ValidID(check.ExternalID) || check.ExternalID != operationID || strconv.FormatInt(check.App.ID, 10) != appID {
		return forge.ExecutionCheck{}, failure("identity", "GitHub check run response identity is incomplete")
	}
	state := ""
	switch check.Status {
	case "queued", "in_progress":
		state = "pending"
	case "completed":
		switch check.Conclusion {
		case "success":
			state = "success"
		case "failure", "cancelled", "timed_out", "action_required":
			state = "failure"
		default:
			return forge.ExecutionCheck{}, failure("provider", "GitHub check run conclusion is unsupported")
		}
	default:
		return forge.ExecutionCheck{}, failure("provider", "GitHub check run status is unsupported")
	}
	if expectedState != "" && state != expectedState {
		return forge.ExecutionCheck{}, failure("identity", "GitHub check run state changed")
	}
	return forge.ExecutionCheck{ID: strconv.FormatInt(check.ID, 10), SHA: check.HeadSHA, Name: check.Name, State: state, PublisherID: appID, OperationID: check.ExternalID}, nil
}

func uncertainCheckMutation(message string) error {
	return &domain.ProviderError{Kind: "uncertain", Message: message, Uncertain: true}
}

var _ forge.ForgeExecutionChecks = (*Provider)(nil)
