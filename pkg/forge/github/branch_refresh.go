package github

import (
	"context"
	"net/http"
	"strings"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

func refreshOwner(branch string, change forge.Change, actor string) bool {
	if forge.BotBranch(branch) {
		return change.AuthorLogin == "dependabot[bot]" && strings.EqualFold(change.AuthorType, "Bot")
	}
	return change.AuthorID == actor
}

func (p *Provider) RefreshAppBranch(ctx context.Context, in forge.RefreshBranchRequest) error {
	if p.authorizeRefreshBranch == nil {
		return failure("unsupported", "Persisted branch refresh authorization required")
	}
	if !positive(in.Repository.NativeID) || !positive(in.ChangeID) || !validBranch(in.HeadBranch) || !in.Valid() || !validBranch(in.TargetBranch) || !validSHA(in.ExpectedHeadSHA) || !validSHA(in.ExpectedTargetSHA) {
		return failure("invalid", "Incomplete or non-Reforge branch refresh request")
	}
	if err := validOperationID(in.OperationID); err != nil {
		return err
	}
	actor, err := p.authenticatedBot(ctx)
	if err != nil {
		return err
	}
	change, err := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if err != nil {
		return err
	}
	if !sameRepoRef(change.Repository, in.Repository) || !sameRepoRef(change.HeadRepository, in.Repository) || !sameRepoRef(change.TargetRepository, in.Repository) || change.HeadBranch != in.HeadBranch || change.TargetBranch != in.TargetBranch || change.HeadSHA != in.ExpectedHeadSHA || change.TargetSHA != in.ExpectedTargetSHA || change.State != "open" || change.Draft || !refreshOwner(in.HeadBranch, change, actor) {
		return failure("conflict", "Pull request identity, owner or expected refs changed")
	}
	if err = p.authorizeRefreshBranch(ctx, in); err != nil {
		return err
	}
	segments, err := repositoryPath(in.Repository)
	if err != nil {
		return err
	}
	segments = append(segments, "pulls", in.ChangeID, "update-branch")
	status, headers, _, err := p.request(ctx, http.MethodPut, segments, nil, &requestBody{data: []byte(`{"expected_head_sha":"` + in.ExpectedHeadSHA + `"}`)})
	if err != nil {
		return err
	}
	if status != http.StatusAccepted {
		if status < 200 || status >= 300 {
			return responseError(status, headers)
		}
		return &domain.ProviderError{Kind: "protocol", Message: "GitHub branch refresh response was not accepted"}
	}
	return nil
}
