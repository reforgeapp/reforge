package privateconnector

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/model"
)

func TestLargeGeneratedBranchOperationWireRoundTripAndOrdinaryLimits(t *testing.T) {
	id := domain.NewID()
	edits := make([]forge.FileEdit, 180)
	content := strings.Repeat("L", 17<<10)
	for i := range edits {
		edits[i] = forge.FileEdit{Path: fmt.Sprintf("dist/THIRD_PARTY_LICENSES/license-%03d.txt", i), Content: []byte(content)}
	}
	request := forge.UpdateBranchRequest{Repository: forge.RepoRef{NativeID: "12", FullName: "org/repo"}, Branch: "reforge/repair/generated", BaseSHA: strings.Repeat("a", 40), Message: "Regenerate licenses", OperationID: id, Edits: edits}
	op := Operation{ID: id, Kind: ForgeUpdateBranch, Branch: &request}
	if err := op.Validate(); err != nil {
		t.Fatalf("large generated branch operation rejected: %v", err)
	}
	raw, err := json.Marshal(op)
	if err != nil || len(raw) <= 1<<20 {
		t.Fatalf("fixture is not over the old private grant limit: bytes=%d err=%v", len(raw), err)
	}
	var decoded Operation
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("large branch operation JSON decode: %v", err)
	}
	if err := decoded.Validate(); err != nil || len(decoded.Branch.Edits) != len(edits) || len(decoded.Branch.Edits[0].Content) != len(content) {
		t.Fatalf("large branch operation wire round trip: files=%d err=%v", len(decoded.Branch.Edits), err)
	}
	grant := Grant{ID: domain.NewID(), RunnerVersion: 1, Target: Target{OrgID: domain.NewID(), RunnerID: domain.NewID()}, Operation: decoded, AuthorityID: domain.NewID(), Connection: Connection{OrgID: domain.NewID(), ID: domain.NewID(), Provider: "github"}, ResultCapability: strings.Repeat("c", 43)}
	wire, err := grant.MarshalWire()
	if err != nil || len(wire) > MaxGrant {
		t.Fatalf("large branch grant marshal: bytes=%d err=%v", len(wire), err)
	}
	if _, err := DecodeGrant(wire); err != nil {
		t.Fatalf("large branch grant rejected on decode: %v", err)
	}

	probe := Operation{ID: domain.NewID(), Kind: ForgeProbe}
	ordinary := Grant{ID: domain.NewID(), RunnerVersion: 1, Target: grant.Target, Operation: probe, AuthorityID: grant.AuthorityID, Connection: Connection{CAPEM: []byte(strings.Repeat("c", 1<<20))}, ResultCapability: strings.Repeat("d", 43)}
	ordinaryWire, err := ordinary.MarshalWire()
	if err != nil || len(ordinaryWire) <= 1<<20 {
		t.Fatalf("fixture is not over the ordinary grant limit: bytes=%d err=%v", len(ordinaryWire), err)
	}
	if _, err := DecodeGrant(ordinaryWire); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ordinary grant limit was broadened: %v", err)
	}

	turnID := domain.NewID()
	turn := model.Turn{OperationID: turnID, Model: "fixture", Messages: []model.Message{{Role: "user"}}, MaxOutputTokens: 1, TimeoutMS: 1000}
	low, high := 0, 1<<20
	for low < high {
		mid := low + (high-low+1)/2
		turn.Messages[0].Text = strings.Repeat("x", mid)
		if turn.Valid() {
			low = mid
		} else {
			high = mid - 1
		}
	}
	turn.Messages[0].Text = strings.Repeat("x", low)
	ordinaryOperation := Operation{ID: turnID, Kind: ModelTurn, Turn: &turn}
	ordinaryJSON, err := json.Marshal(ordinaryOperation)
	if err != nil || len(ordinaryJSON) <= 512<<10 || ordinaryOperation.Validate() == nil {
		t.Fatalf("ordinary operation request cap changed: bytes=%d err=%v", len(ordinaryJSON), err)
	}
}

func TestRefreshBranchGrantIsBoundAndMutation(t *testing.T) {
	id := domain.NewID()
	request := forge.RefreshBranchRequest{Repository: forge.RepoRef{NativeID: "12", FullName: "org/repo"}, ChangeID: "3", HeadBranch: "reforge/repair/task", TargetBranch: "master", ExpectedHeadSHA: strings.Repeat("a", 40), ExpectedTargetSHA: strings.Repeat("b", 40), OperationID: id}
	op := Operation{ID: id, Kind: ForgeRefreshBranch, Refresh: &request}
	if err := op.Validate(); err != nil || !op.Mutation() {
		t.Fatalf("valid mutation: %v", err)
	}
	for _, change := range []func(*forge.RefreshBranchRequest){
		func(r *forge.RefreshBranchRequest) { r.OperationID = domain.NewID() },
		func(r *forge.RefreshBranchRequest) { r.HeadBranch = "feature/pkg" },
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
