package privateconnector

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/forge/gitea"
	"github.com/reforgeapp/reforge/internal/forge/github"
	"github.com/reforgeapp/reforge/internal/forge/gitlab"
)

func BindForgeOperation(provider forge.Provider, operation Operation) forge.Provider {
	same := func(a, b any) bool {
		left, err := json.Marshal(a)
		if err != nil {
			return false
		}
		right, err := json.Marshal(b)
		return err == nil && bytes.Equal(left, right)
	}
	branch := func(ctx context.Context, in forge.UpdateBranchRequest) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if operation.Kind != ForgeUpdateBranch || operation.Branch == nil || !same(in, *operation.Branch) {
			return auth.ErrForbidden
		}
		return nil
	}
	refresh := func(ctx context.Context, in forge.RefreshBranchRequest) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if operation.Kind != ForgeRefreshBranch || operation.Refresh == nil || !same(in, *operation.Refresh) {
			return auth.ErrForbidden
		}
		return nil
	}
	change := func(ctx context.Context, in forge.CreateChangeRequest) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if operation.Kind != ForgeCreateChange || operation.Create == nil || !same(in, *operation.Create) {
			return auth.ErrForbidden
		}
		return nil
	}
	merge := func(ctx context.Context, in forge.MergeRequest, current forge.Change, rules forge.Rules) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if operation.Kind != ForgeMerge || operation.Merge == nil || !same(in, *operation.Merge) || current.HeadSHA != in.ExpectedHeadSHA || current.TargetSHA != in.ExpectedTargetSHA || rules.Hash != in.RulesHash {
			return auth.ErrForbidden
		}
		return nil
	}
	train := func(ctx context.Context, in forge.TrainGateRequest) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if operation.Kind != ForgeReleaseTrainGate || operation.TrainGate == nil || !same(in, *operation.TrainGate) {
			return auth.ErrForbidden
		}
		return nil
	}
	delivery := func(ctx context.Context, in forge.PipelineRequest, gates forge.DeploymentGates) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if (operation.Kind != ForgePipelineCancel && operation.Kind != ForgePipelineTrigger && operation.Kind != ForgePipelineRecover) || operation.Pipeline == nil || !same(in, *operation.Pipeline) || gates.RulesHash != in.RulesHash {
			return auth.ErrForbidden
		}
		return nil
	}
	switch p := provider.(type) {
	case *gitea.Provider:
		return p.WithBranchRefreshAuthorizer(refresh).WithBranchAuthorizer(branch)
	case *github.Provider:
		return p.WithBranchRefreshAuthorizer(refresh).WithBranchAuthorizer(branch).WithChangeAuthorizer(change).WithMergeGuard(merge).WithDeliveryGuard(delivery)
	case *gitlab.Provider:
		return p.WithBranchRefreshAuthorizer(refresh).WithBranchAuthorizer(branch).WithChangeAuthorizer(change).WithMergeGuard(merge).WithTrainGateAuthorizer(train).WithDeliveryGuard(delivery)
	}
	return provider
}
