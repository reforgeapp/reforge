package privateconnector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"reforge/internal/forge"
)

type queueInspectionProvider struct {
	*inspectionProvider
	native forge.NativeEligibility
	gate   forge.CheckRule
	events *[]string
}

func (p *queueInspectionProvider) ListChecks(ctx context.Context, repo forge.RepoRef, sha string) ([]forge.Check, error) {
	if p.events != nil {
		*p.events = append(*p.events, "checks")
	}
	return p.inspectionProvider.ListChecks(ctx, repo, sha)
}

func (p *queueInspectionProvider) EvaluateQueuePrerequisites(context.Context, forge.RepoRef, string) (forge.NativeEligibility, forge.CheckRule, error) {
	if p.events != nil {
		*p.events = append(*p.events, "admission")
	}
	return p.native, p.gate, nil
}

func queueProvider(change forge.Change) *queueInspectionProvider {
	return &queueInspectionProvider{
		inspectionProvider: &inspectionProvider{reads: []forge.Change{change, change}, rules: forge.Rules{RequireQueue: true}},
		native:             forge.NativeEligibility{State: "eligible", HeadSHA: change.HeadSHA, TargetSHA: change.TargetSHA},
		gate:               forge.CheckRule{Name: forge.QueueExecutionCheckName, PublisherID: "publisher"},
	}
}

func TestInspectQueueRequiresPrerequisitesAndQueueRule(t *testing.T) {
	change := inspectionChange()
	if _, err := inspectQueue(context.Background(), &inspectionProvider{reads: []forge.Change{change}}, ChangeArgs{Repository: change.Repository, ChangeID: change.ID}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("missing prerequisites error=%v", err)
	}
	p := queueProvider(change)
	p.rules.RequireQueue = false
	if _, err := inspectQueue(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID}); err == nil {
		t.Fatal("non-queue change accepted")
	}
}

func TestInspectQueueChecksHeadBeforeAdmission(t *testing.T) {
	change := inspectionChange()
	p := queueProvider(change)
	events := []string{}
	p.events = &events
	evidence, err := inspectQueue(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID})
	if err != nil || evidence == nil || p.checkSHA != change.HeadSHA || p.checkRepo != change.HeadRepository || strings.Join(events, ",") != "checks,admission" {
		t.Fatalf("evidence=%+v repo=%+v sha=%s events=%v error=%v", evidence, p.checkRepo, p.checkSHA, events, err)
	}
}

func TestInspectQueueUsesTestedCommitOnlyForExactQueueProof(t *testing.T) {
	change := inspectionChange()
	tested := strings.Repeat("c", 40)
	p := queueProvider(change)
	p.queue = forge.QueueState{ID: "queue", State: "tested", HeadSHA: change.HeadSHA, TargetSHA: change.TargetSHA, TestedSHA: tested}
	if _, err := inspectQueue(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID}); err != nil || p.checkSHA != tested || p.checkRepo != change.TargetRepository {
		t.Fatalf("repo=%+v sha=%s error=%v", p.checkRepo, p.checkSHA, err)
	}
	p = queueProvider(change)
	p.queue = forge.QueueState{ID: "queue", State: "tested", HeadSHA: strings.Repeat("d", 40), TargetSHA: change.TargetSHA, TestedSHA: tested}
	if _, err := inspectQueue(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID}); err != nil || p.checkSHA != "" {
		t.Fatalf("invalid queue proof used checks sha=%s error=%v", p.checkSHA, err)
	}
}

func TestInspectQueueRequiresExactGateAndStableHead(t *testing.T) {
	change := inspectionChange()
	p := queueProvider(change)
	p.gate.Name = "other"
	if _, err := inspectQueue(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID}); err == nil {
		t.Fatal("wrong execution gate accepted")
	}
	p = queueProvider(change)
	changed := change
	changed.HeadSHA = strings.Repeat("d", 40)
	p.reads = []forge.Change{change, changed}
	if _, err := inspectQueue(context.Background(), p, ChangeArgs{Repository: change.Repository, ChangeID: change.ID}); err == nil {
		t.Fatal("head drift accepted")
	}
}
