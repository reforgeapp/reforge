package forge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/reforgeapp/reforge/pkg/domain"
	"path"
	"regexp"
	"strings"
)

var deliveryID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var deliverySHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
var deliveryHash = regexp.MustCompile(`^[0-9a-f]{64}$`)
var deliveryInput = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
var deliveryEnvironment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_. /-]{0,127}$`)

type DeliveryGuard func(context.Context, PipelineRequest, DeploymentGates) error

func ValidPipelineRequest(in PipelineRequest) bool {
	if !deliveryID.MatchString(in.CorrelationID) || !deliverySHA.MatchString(in.SourceSHA) || !deliverySHA.MatchString(in.WorkflowSHA) || !deliveryHash.MatchString(in.ConfigSHA256) || !ValidArtifactDigest(in.ArtifactDigest) || !ValidEnvironment(in.Environment) || in.Repository.NativeID == "" || len(in.Repository.FullName) > 1024 || in.WorkflowID == "" || len(in.WorkflowID) > 256 || in.WorkflowPath == "" || path.Clean(in.WorkflowPath) != in.WorkflowPath || strings.HasPrefix(in.WorkflowPath, "/") || strings.HasPrefix(in.WorkflowPath, "../") || len(in.WorkflowPath) > 1024 || len(in.Ref) > 255 || !(strings.HasPrefix(in.Ref, "refs/heads/") || strings.HasPrefix(in.Ref, "refs/tags/")) || len(in.Inputs) > 20 || !deliveryHash.MatchString(in.RulesHash) {
		return false
	}
	for k, v := range in.Inputs {
		if !deliveryInput.MatchString(k) || strings.HasPrefix(strings.ToLower(k), "reforge_") || len(v) > 512 {
			return false
		}
	}
	return true
}

func ValidArtifactDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && deliveryHash.MatchString(strings.TrimPrefix(value, "sha256:"))
}
func ValidEnvironment(value string) bool {
	return deliveryEnvironment.MatchString(value) && strings.TrimSpace(value) == value
}
func DeliveryRulesHash(v any) string {
	raw, _ := json.Marshal(v)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func VerifyPipelineConfiguration(ctx context.Context, provider ForgeInventory, in PipelineRequest) error {
	if !ValidPipelineRequest(in) {
		return &domain.ProviderError{Kind: "invalid", Message: "Pinned deployment source, configuration, environment, digest and correlation are required"}
	}
	repository, err := provider.GetRepository(ctx, in.Repository)
	if err != nil {
		return err
	}
	if repository.NativeID != in.Repository.NativeID || repository.FullName != in.Repository.FullName {
		return &domain.ProviderError{Kind: "identity", Message: "Deployment repository changed"}
	}
	current, err := provider.ResolveRef(ctx, in.Repository, in.Ref)
	if err != nil {
		return err
	}
	if current != in.WorkflowSHA {
		return &domain.ProviderError{Kind: "conflict", Message: "Deployment ref moved"}
	}
	file, err := provider.ReadFileAtRef(ctx, in.Repository, in.WorkflowPath, in.WorkflowSHA)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(file.Content)
	if file.Path != in.WorkflowPath || hex.EncodeToString(digest[:]) != in.ConfigSHA256 {
		return &domain.ProviderError{Kind: "conflict", Message: "Deployment configuration changed"}
	}
	return nil
}

func PipelineMatches(in PipelineRequest, out DeploymentStatus) bool {
	return out.ID != "" && out.WorkflowSHA == in.WorkflowSHA && out.CorrelationID == in.CorrelationID && out.WorkflowID == in.WorkflowID && out.RunAttempt == 1 && strings.SplitN(out.WorkflowPath, "@", 2)[0] == in.WorkflowPath && (out.Ref == in.Ref || "refs/heads/"+out.Ref == in.Ref || "refs/tags/"+out.Ref == in.Ref)
}

func InspectPipeline(ctx context.Context, provider Provider, in PipelineRequest) (DeploymentGates, error) {
	if err := VerifyPipelineConfiguration(ctx, provider, in); err != nil {
		return DeploymentGates{}, err
	}
	workflows, err := provider.ListAllowedWorkflows(ctx, in.Repository)
	if err != nil {
		return DeploymentGates{}, err
	}
	found := false
	for _, w := range workflows {
		if w.ID == in.WorkflowID && w.Path == in.WorkflowPath {
			if found {
				return DeploymentGates{}, &domain.ProviderError{Kind: "identity", Message: "Workflow identity is duplicated"}
			}
			found = true
		}
	}
	if !found {
		return DeploymentGates{}, &domain.ProviderError{Kind: "conflict", Message: "Allowlisted workflow identity or path changed"}
	}
	gates, err := provider.ReadDeploymentGates(ctx, in.Repository, in.Environment)
	if err != nil {
		return gates, err
	}
	if gates.NativeEnforced != domain.Supported || gates.RulesHash != in.RulesHash {
		return gates, &domain.ProviderError{Kind: "conflict", Message: "Native deployment gates changed"}
	}
	return gates, nil
}

type ForgePipelineControl interface {
	CancelPipeline(context.Context, PipelineRequest) (DeploymentStatus, error)
}
