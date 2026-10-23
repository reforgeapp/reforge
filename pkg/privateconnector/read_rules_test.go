package privateconnector

import (
	"context"
	"testing"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

type rulesForgeProvider struct {
	forge.Provider
	repo   forge.RepoRef
	rules  forge.Rules
	branch string
}

func (p *rulesForgeProvider) GetRepository(_ context.Context, repo forge.RepoRef) (forge.Repository, error) {
	return forge.Repository{RepoRef: repo, DefaultBranch: "trunk"}, nil
}

func (p *rulesForgeProvider) ReadEffectiveRules(_ context.Context, repo forge.RepoRef, branch string) (forge.Rules, error) {
	p.repo, p.branch = repo, branch
	return p.rules, nil
}

func TestForgeReadRulesReturnsEffectiveDefaultBranchRules(t *testing.T) {
	repo := forge.RepoRef{NativeID: "123", FullName: "team/repo"}
	provider := &rulesForgeProvider{rules: forge.Rules{State: domain.Supported, Hash: "rules-hash", AllowedMergeMethods: []string{"squash"}}}
	result, err := ReadForge(context.Background(), provider, Operation{ID: domain.NewID(), Kind: ForgeReadRules, Repository: &RepositoryArgs{Repository: repo}})
	if err != nil || result.Rules == nil || result.Rules.Hash != "rules-hash" || provider.repo != repo || provider.branch != "trunk" {
		t.Fatalf("rules result=%+v repo=%+v branch=%q err=%v", result, provider.repo, provider.branch, err)
	}
}
