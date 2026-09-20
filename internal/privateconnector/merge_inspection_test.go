package privateconnector

import (
	"context"
	"strings"
	"testing"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

type inspectionProvider struct {
	forge.Provider
	reads      []forge.Change
	rules      forge.Rules
	queue      forge.QueueState
	checks     []forge.Check
	checkRepo  forge.RepoRef
	checkSHA   string
	queueError error
}

func (p *inspectionProvider) ReadChange(context.Context, forge.RepoRef, string) (forge.Change, error) {
	change := p.reads[0]
	p.reads = p.reads[1:]
	return change, nil
}
func (p *inspectionProvider) ReadEffectiveRules(context.Context, forge.RepoRef, string) (forge.Rules, error) {
	return p.rules, nil
}
func (p *inspectionProvider) ReadQueueState(context.Context, forge.RepoRef, string) (forge.QueueState, error) {
	return p.queue, p.queueError
}
func (p *inspectionProvider) ListChecks(_ context.Context, repo forge.RepoRef, sha string) ([]forge.Check, error) {
	p.checkRepo = repo
	p.checkSHA = sha
	return p.checks, nil
}
func (p *inspectionProvider) ReadApprovals(context.Context, forge.RepoRef, string) ([]forge.Approval, error) {
	return []forge.Approval{{ID: "approval"}}, nil
}
func (p *inspectionProvider) EvaluateNativeEligibility(context.Context, forge.RepoRef, string) (forge.NativeEligibility, error) {
	return forge.NativeEligibility{State: "eligible"}, nil
}
func (p *inspectionProvider) ProbeCapabilities(context.Context) (forge.Capabilities, error) {
	return forge.Capabilities{Provider: "fixture"}, nil
}

func inspectionChange() forge.Change {
	repo := forge.RepoRef{NativeID: "1", FullName: "org/repo"}
	return forge.Change{ID: "7", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadSHA: strings.Repeat("a", 40), TargetSHA: strings.Repeat("b", 40), HeadBranch: "feature", TargetBranch: "main", State: "open"}
}

func TestInspectMergeRequiresRequestedChangeID(t *testing.T) {
	change := inspectionChange()
	p := &inspectionProvider{reads: []forge.Change{change}}
	if _, err := inspectMerge(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: "8"}); err == nil {
		t.Fatal("wrong change accepted")
	}
}

func TestInspectMergeBindsFinalIdentity(t *testing.T) {
	change := inspectionChange()
	retargeted := change
	retargeted.HeadSHA = strings.Repeat("d", 40)
	p := &inspectionProvider{reads: []forge.Change{change, retargeted}}
	if _, err := inspectMerge(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID}); err == nil {
		t.Fatal("head movement accepted")
	}
}

func TestInspectMergeUsesQueueTestedHeadForChecks(t *testing.T) {
	change := inspectionChange()
	tested := strings.Repeat("c", 40)
	p := &inspectionProvider{reads: []forge.Change{change, change}, rules: forge.Rules{RequireQueue: true}, queue: forge.QueueState{State: "tested", HeadSHA: change.HeadSHA, TargetSHA: change.TargetSHA, TestedSHA: tested}, checks: []forge.Check{{ID: "check"}}}
	evidence, err := inspectMerge(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID})
	if err != nil || evidence == nil || p.checkSHA != tested || evidence.Queue.TestedSHA != tested {
		t.Fatalf("evidence=%+v check sha=%s error=%v", evidence, p.checkSHA, err)
	}
}

func TestInspectMergeUsesHeadRepositoryForForkChecks(t *testing.T) {
	change := inspectionChange()
	change.HeadRepository = forge.RepoRef{NativeID: "2", FullName: "fork/repo"}
	p := &inspectionProvider{reads: []forge.Change{change, change}}
	if _, err := inspectMerge(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID}); err != nil {
		t.Fatal(err)
	}
	if p.checkRepo != change.HeadRepository || p.checkSHA != change.HeadSHA {
		t.Fatalf("checks repo=%+v sha=%s", p.checkRepo, p.checkSHA)
	}
}

func TestInspectMergeRejectsDraftChangeMovement(t *testing.T) {
	change := inspectionChange()
	draft := change
	draft.Draft = true
	p := &inspectionProvider{reads: []forge.Change{change, draft}}
	if _, err := inspectMerge(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID}); err == nil {
		t.Fatal("draft movement accepted")
	}
}

func TestInspectMergeUnsupportedQueueIsUnknown(t *testing.T) {
	change := inspectionChange()
	p := &inspectionProvider{reads: []forge.Change{change, change}, rules: forge.Rules{RequireQueue: true}, queueError: &domain.ProviderError{Kind: "unsupported", Message: "queue unavailable"}}
	evidence, err := inspectMerge(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID})
	if err != nil || evidence == nil || evidence.Queue.State != "unsupported" || len(evidence.Checks) != 0 {
		t.Fatalf("evidence=%+v error=%v", evidence, err)
	}
}
