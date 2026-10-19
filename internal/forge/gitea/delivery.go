package gitea

import (
	"context"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

func (p *Provider) ListAllowedWorkflows(context.Context, forge.RepoRef) ([]forge.Workflow, error) {
	return nil, failure("unsupported", "Actions delivery requires a certified workflow allowlist and protected-environment enforcement")
}
func (p *Provider) TriggerOrObservePipeline(context.Context, forge.PipelineRequest) (forge.DeploymentStatus, error) {
	return forge.DeploymentStatus{}, failure("unsupported", "Gitea Actions delivery has not been certified; use a qualified external CI route")
}
func (p *Provider) ReadDeploymentGates(context.Context, forge.RepoRef, string) (forge.DeploymentGates, error) {
	return forge.DeploymentGates{State: "unknown", NativeEnforced: domain.Unknown, Blockers: []string{"Protected environment enforcement has not been certified"}}, nil
}
func (p *Provider) ReadDeploymentStatus(context.Context, forge.RepoRef, string) (forge.DeploymentStatus, error) {
	return forge.DeploymentStatus{}, failure("unsupported", "Gitea deployment correlation and artifact provenance have not been certified")
}
func (p *Provider) RequestAllowedRecovery(context.Context, forge.PipelineRequest) (forge.DeploymentStatus, error) {
	return forge.DeploymentStatus{}, failure("unsupported", "Native recovery requires a certified protected workflow")
}
