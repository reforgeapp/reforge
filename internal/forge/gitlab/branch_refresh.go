package gitlab

import (
	"context"
	"net/http"
	"strings"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

func (p *Provider) RefreshAppBranch(ctx context.Context, in forge.RefreshBranchRequest) error {
	if p.authorizeRefreshBranch == nil {
		return failure("unsupported", "Persisted branch refresh authorization required")
	}
	if !safeID(in.Repository.NativeID) || !safeID(in.ChangeID) || !validBranch(in.HeadBranch) || !strings.HasPrefix(in.HeadBranch, "reforge/repair/") || len(in.HeadBranch) == len("reforge/repair/") || !validBranch(in.TargetBranch) || !validSHA(in.ExpectedHeadSHA) || !validSHA(in.ExpectedTargetSHA) {
		return failure("invalid", "Incomplete or non-Reforge branch refresh request")
	}
	if err := validOperationID(in.OperationID); err != nil {
		return err
	}
	actor, err := p.authenticatedActor(ctx)
	if err != nil {
		return err
	}
	change, err := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if err != nil {
		return err
	}
	if !sameRepo(change.Repository, in.Repository) || !sameRepo(change.HeadRepository, in.Repository) || !sameRepo(change.TargetRepository, in.Repository) || change.HeadBranch != in.HeadBranch || change.TargetBranch != in.TargetBranch || change.HeadSHA != in.ExpectedHeadSHA || change.TargetSHA != in.ExpectedTargetSHA || (change.State != "open" && change.State != "opened") || change.Draft || change.AuthorID != actor {
		return failure("conflict", "Merge request identity, owner or expected refs changed")
	}
	if err = p.authorizeRefreshBranch(ctx, in); err != nil {
		return err
	}
	status, headers, _, err := p.request(ctx, http.MethodPut, []string{"projects", in.Repository.NativeID, "merge_requests", in.ChangeID, "rebase"}, nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusAccepted {
		if status < 200 || status >= 300 {
			return responseError(status, headers)
		}
		return &domain.ProviderError{Kind: "protocol", Message: "GitLab branch refresh response was not accepted"}
	}
	return nil
}
