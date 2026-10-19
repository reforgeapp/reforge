package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/maintenance/discovery"
	"github.com/reforgeapp/reforge/internal/mergecontrol"
	"github.com/reforgeapp/reforge/internal/policy"
	"github.com/reforgeapp/reforge/internal/privateconnector"
	"github.com/reforgeapp/reforge/internal/providers"
)

type trainContractProvider struct {
	*queueContractProvider
	trainState  string
	checksReady bool
	merged      bool
	dropRelease bool
	trainWrites int
}

func (p *trainContractProvider) ForProtection(string, map[string]string) providers.Client { return p }

func (p *trainContractProvider) Read(ctx context.Context, org, id string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if err := p.authorize(ctx, id, authorize); err != nil {
		return privateconnector.Result{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if op.Kind == privateconnector.ForgeQueueInspect {
		return privateconnector.Result{MergeEvidence: p.trainEvidence()}, nil
	}
	if op.Kind == privateconnector.ForgeMergeResult {
		state := "queued"
		nativeID := "queue-1"
		mergeSHA := ""
		if p.merged {
			state = "merged"
			nativeID = "1"
			mergeSHA = strings.Repeat("d", 40)
		}
		return privateconnector.Result{Merge: &forge.MergeResult{State: state, NativeID: nativeID, HeadSHA: p.head, MergeSHA: mergeSHA}}, nil
	}
	return privateconnector.Result{}, privateconnector.ErrUnsupported
}

func (p *trainContractProvider) Write(ctx context.Context, org, id string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) (string, error), current func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if _, err := p.authorizeWrite(ctx, id, authorize); err != nil {
		return privateconnector.Result{}, err
	}
	if err := p.authorize(ctx, id, current); err != nil {
		return privateconnector.Result{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes++
	if op.Kind == privateconnector.ForgeCancelQueue {
		p.queue = false
		p.trainState = "not_queued"
		return privateconnector.Result{Queue: &forge.QueueState{State: "not_queued", HeadSHA: p.head, TargetSHA: p.target}}, nil
	}
	if op.Kind == privateconnector.ForgeMerge {
		p.queue = true
		return privateconnector.Result{Merge: &forge.MergeResult{State: "queued", NativeID: "queue-1", HeadSHA: p.head}}, nil
	}
	if op.Kind != privateconnector.ForgeReleaseTrainGate {
		return privateconnector.Result{}, privateconnector.ErrUnsupported
	}
	p.trainWrites++
	p.trainState = "pending"
	result := p.trainGate()
	if p.dropRelease {
		return privateconnector.Result{}, nil
	}
	return privateconnector.Result{TrainGate: &result}, nil
}

func (p *trainContractProvider) trainGate() forge.TrainGate {
	if !p.queue {
		return forge.TrainGate{HeadSHA: p.head, TargetSHA: p.target, CIConfigSHA256: strings.Repeat("c", 64), Name: forge.QueueExecutionCheckName, PublisherID: "42", State: "not_queued"}
	}
	return forge.TrainGate{QueueID: "queue-1", PipelineID: "pipeline-1", JobID: "42", HeadSHA: p.head, TargetSHA: p.target, SHA: p.tested, CIConfigSHA256: strings.Repeat("c", 64), Name: forge.QueueExecutionCheckName, PublisherID: "42", State: p.trainState, ChecksReady: p.checksReady}
}

func (p *trainContractProvider) trainEvidence() *forge.MergeEvidence {
	queue := forge.QueueState{State: "not_queued", HeadSHA: p.head, TargetSHA: p.target}
	if p.queue {
		queue = forge.QueueState{ID: "queue-1", State: "queued", HeadSHA: p.head, TestedSHA: p.tested, TargetSHA: p.target}
	}
	return &forge.MergeEvidence{TrainGate: func() *forge.TrainGate { g := p.trainGate(); return &g }(), ExecutionCheck: forge.CheckRule{Name: forge.QueueExecutionCheckName, PublisherID: "42"}, Change: forge.Change{ID: "1", Repository: p.repo, HeadRepository: p.repo, TargetRepository: p.repo, HeadSHA: p.head, TargetSHA: p.target, HeadBranch: "feature", TargetBranch: "main", AuthorID: "author", State: "open"}, Rules: forge.Rules{State: domain.Supported, Hash: strings.Repeat("b", 64), RequiredChecks: []forge.CheckRule{{Name: "ci/test", PublisherID: "42"}, {Name: forge.QueueExecutionCheckName, PublisherID: "42"}}, RequireQueue: true, AllowedMergeMethods: []string{"merge"}}, Checks: []forge.Check{{Name: "ci/test", PublisherID: "42", HeadSHA: func() string {
		if p.queue {
			return p.tested
		}
		return p.head
	}(), Status: "completed", Conclusion: "success"}}, Native: forge.NativeEligibility{State: "eligible", HeadSHA: p.head, TargetSHA: p.target}, Queue: queue, Capabilities: forge.Capabilities{Provider: "gitlab", ServerVersion: "16.0"}, ObservedAt: time.Now().UTC()}
}

func TestMergeTrainAdmissionReleaseAndUncertainRecovery(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET provider='gitlab',state='healthy',reason='',server_version='16.0',verified_at=now(),settings=jsonb_build_object('auth_kind','token','billing_route','forge') WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PutConfig(ctx, f.owner, f.org, f.repo, discovery.Config{MergeAuthority: "reforge"}, 0, "train-contract"); err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{MergeMethods: []string{"merge"}}}, "train contract", "train-contract")
	if err != nil {
		t.Fatal(err)
	}
	simulation, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Merge, MergeMethod: "merge"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, simulation.Hash, "train contract", "train-contract"); err != nil {
		t.Fatal(err)
	}
	head, target, tested := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	repo := forge.RepoRef{NativeID: "1", FullName: "acme/repository-0000"}
	base := &queueContractProvider{db: f.db, connections: f.connections, org: f.org, repo: repo, head: head, target: target, tested: tested, ciPassing: true, files: map[string]map[string][]byte{target: {"base.txt": []byte("base\n")}, head: {"base.txt": []byte("head\n")}, tested: {"base.txt": []byte("tested\n")}}}
	provider := &trainContractProvider{queueContractProvider: base, trainState: "manual", checksReady: false}
	merges := mergecontrol.New(f.db, f.identity, f.connections, policies, provider)
	qhash := strings.Repeat("c", 64)
	qualification := mergecontrol.Qualification{Provider: "gitlab", ServerVersion: "16.0", ConnectionVersion: f.connection.Version, CIConfigSHA256: qhash, EvidenceReference: "gitlab-train-contract", EvidenceSHA256: strings.Repeat("a", 64), VerifiedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour), ExactHead: true, QueueExecutionGate: true}
	cfg, err := merges.PutConfig(ctx, f.owner, f.org, f.repo, mergecontrol.Configuration{Enabled: true, CheckPublishers: map[string]string{forge.QueueExecutionCheckName: "42"}, CooperationReference: "cooperationref", Qualification: qualification}, 0, "train-contract")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := merges.Inspect(ctx, f.owner, f.org, f.repo, "1", "merge", "train-contract")
	if err != nil || gate.Phase != "queue_admission" || gate.Decision.Outcome != "allow" {
		t.Fatalf("train admission preview: %+v %v", gate, err)
	}
	op, err := merges.Request(ctx, f.owner, f.org, gate.ID, domain.NewID(), "train-contract")
	if err != nil || op.State != "queued" || provider.writes != 1 || provider.trainWrites != 0 {
		t.Fatalf("train admission: %+v %v writes=%d train=%d", op, err, provider.writes, provider.trainWrites)
	}
	op, err = merges.Observe(ctx, f.org, op.ID)
	if err != nil || op.State != "queued" || provider.trainWrites != 0 {
		t.Fatalf("train release: %+v %v", op, err)
	}
	provider.mu.Lock()
	provider.checksReady = true
	provider.mu.Unlock()
	op, err = merges.Observe(ctx, f.org, op.ID)
	if err != nil || op.State != "queued" || provider.trainWrites != 1 {
		t.Fatalf("train release: %+v %v", op, err)
	}
	provider.mu.Lock()
	provider.merged = true
	provider.mu.Unlock()
	op, err = merges.Observe(ctx, f.org, op.ID)
	if err != nil || op.State != "merged" {
		t.Fatalf("canonical train merge: %+v %v", op, err)
	}
	provider.mu.Lock()
	provider.queue, provider.merged, provider.checksReady, provider.trainState, provider.dropRelease = false, false, true, "manual", true
	provider.mu.Unlock()
	gate, err = merges.Inspect(ctx, f.owner, f.org, f.repo, "1", "merge", "train-lost")
	if err != nil {
		t.Fatal(err)
	}
	lost, err := merges.Request(ctx, f.owner, f.org, gate.ID, domain.NewID(), "train-lost")
	if err != nil || lost.State != "queued" {
		t.Fatalf("lost train admission: %+v %v", lost, err)
	}
	if _, err = merges.Observe(ctx, f.org, lost.ID); !errors.Is(err, privateconnector.ErrUncertain) {
		t.Fatalf("lost train response not uncertain: %v", err)
	}
	restarted := mergecontrol.New(f.db, f.identity, f.connections, policies, provider)
	if _, err = restarted.Observe(ctx, f.org, lost.ID); err != nil {
		t.Fatalf("pending train gate did not remain queued: %v", err)
	}
	if provider.trainWrites != 2 {
		t.Fatalf("train release duplicated: %d", provider.trainWrites)
	}
	provider.mu.Lock()
	provider.queue, provider.trainState = false, "not_queued"
	provider.mu.Unlock()
	cfg.Enabled = false
	if _, err = merges.PutConfig(ctx, f.owner, f.org, f.repo, cfg, cfg.Version, "train-pause"); err != nil {
		t.Fatal(err)
	}
	cancelled, err := restarted.Observe(ctx, f.org, lost.ID)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatalf("paused train did not cancel: %+v %v", cancelled, err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		var state string
		return tx.QueryRow(ctx, `SELECT state FROM merge_execution_checks WHERE org_id=$1 AND operation_id=$2 AND sha=$3`, f.org, lost.ID, tested).Scan(&state)
	}); err != nil {
		t.Fatal(err)
	}
}
