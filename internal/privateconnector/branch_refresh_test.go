package privateconnector

import (
	"strings"
	"testing"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

func TestRefreshBranchGrantIsBoundAndMutation(t *testing.T) {
	id := domain.NewID()
	request := forge.RefreshBranchRequest{Repository: forge.RepoRef{NativeID: "12", FullName: "org/repo"}, ChangeID: "3", HeadBranch: "reforge/repair/task", TargetBranch: "master", ExpectedHeadSHA: strings.Repeat("a", 40), ExpectedTargetSHA: strings.Repeat("b", 40), OperationID: id}
	op := Operation{ID: id, Kind: ForgeRefreshBranch, Refresh: &request}
	if err := op.Validate(); err != nil || !op.Mutation() {
		t.Fatalf("valid mutation: %v", err)
	}
	for _, change := range []func(*forge.RefreshBranchRequest){
		func(r *forge.RefreshBranchRequest) { r.OperationID = domain.NewID() },
		func(r *forge.RefreshBranchRequest) { r.HeadBranch = "dependabot/npm/pkg" },
		func(r *forge.RefreshBranchRequest) { r.HeadBranch = "reforge/repair/" },
		func(r *forge.RefreshBranchRequest) { r.TargetBranch = r.HeadBranch },
		func(r *forge.RefreshBranchRequest) { r.ExpectedHeadSHA = strings.Repeat("x", 40) },
	} {
		invalid := request
		change(&invalid)
		op.Refresh = &invalid
		if op.Validate() == nil {
			t.Fatal("unbound or invalid refresh accepted")
		}
	}
	op.Refresh = &request
	op.Change = &ChangeArgs{Repository: request.Repository, ChangeID: request.ChangeID}
	if op.Validate() == nil {
		t.Fatal("mixed operation accepted")
	}
	if !(Result{}).valid(ForgeRefreshBranch) || (Result{SHA: request.ExpectedHeadSHA}).valid(ForgeRefreshBranch) {
		t.Fatal("refresh acknowledgement shape invalid")
	}
}

func TestBehindResultRejectsMissingOrMixedEvidence(t *testing.T) {
	behind := 2
	if !(Result{Behind: &behind}).valid(ForgeBehind) {
		t.Fatal("branch freshness evidence rejected")
	}
	if (Result{}).valid(ForgeBehind) || (Result{Behind: &behind, SHA: strings.Repeat("a", 40)}).valid(ForgeBehind) {
		t.Fatal("ambiguous branch freshness evidence accepted")
	}
}
