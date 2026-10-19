package gitea

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/reforgeapp/reforge/internal/forge"
)

func (p *Provider) RefreshAppBranch(ctx context.Context, in forge.RefreshBranchRequest) error {
	if p.authorizeRefreshBranch == nil {
		return failure("forbidden", "Persisted branch refresh authorization required")
	}
	if !positive(in.Repository.NativeID) || !positive(in.ChangeID) || !validRefreshBranch(in.HeadBranch) || !strings.HasPrefix(in.HeadBranch, "reforge/repair/") || len(in.HeadBranch) == len("reforge/repair/") || !validRefreshBranch(in.TargetBranch) || !sha(in.ExpectedHeadSHA) || !sha(in.ExpectedTargetSHA) || !validOperation(in.OperationID) {
		return failure("invalid", "Incomplete or non-Reforge branch refresh request")
	}
	var actor user
	if err := p.request(ctx, http.MethodGet, "/user", nil, &actor); err != nil {
		return err
	}
	if actor.ID <= 0 {
		return failure("identity", "Authenticated actor identity is missing")
	}
	change, err := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if err != nil {
		return err
	}
	sameRepo := func(actual forge.RepoRef) bool {
		return actual.NativeID == in.Repository.NativeID && (in.Repository.FullName == "" || actual.FullName == in.Repository.FullName)
	}
	if !sameRepo(change.Repository) || !sameRepo(change.HeadRepository) || !sameRepo(change.TargetRepository) || change.HeadBranch != in.HeadBranch || change.TargetBranch != in.TargetBranch || change.HeadSHA != in.ExpectedHeadSHA || change.TargetSHA != in.ExpectedTargetSHA || change.State != "open" || change.Draft || change.AuthorID != strconv.FormatInt(actor.ID, 10) {
		return failure("conflict", "Pull request identity, owner or expected refs changed")
	}
	if err = p.authorizeRefreshBranch(ctx, in); err != nil {
		return err
	}
	route, err := repoPath(in.Repository)
	if err != nil {
		return err
	}
	return p.request(ctx, http.MethodPost, route+"/pulls/"+in.ChangeID+"/update?style=rebase", nil, nil)
}

func validRefreshBranch(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") || strings.HasSuffix(value, ".") || strings.ContainsAny(value, " ~^:?*[%\\\x00\r\n\t") || strings.Contains(value, "..") || strings.Contains(value, "@{") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}
