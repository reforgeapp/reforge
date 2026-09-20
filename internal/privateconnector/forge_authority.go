package privateconnector

import (
	"bytes"
	"context"
	"encoding/json"
	"reforge/internal/auth"
	"reforge/internal/forge"
	"reforge/internal/forge/gitea"
	"reforge/internal/forge/github"
	"reforge/internal/forge/gitlab"
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
	switch p := provider.(type) {
	case *gitea.Provider:
		return p.WithBranchAuthorizer(branch)
	case *github.Provider:
		return p.WithBranchAuthorizer(branch).WithChangeAuthorizer(change).WithMergeGuard(merge)
	case *gitlab.Provider:
		return p.WithBranchAuthorizer(branch).WithChangeAuthorizer(change).WithMergeGuard(merge)
	}
	return provider
}
