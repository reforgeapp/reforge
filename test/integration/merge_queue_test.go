package integration

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/mergecontrol"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/providers"
	"reforge/internal/source"
)

type queueContractProvider struct {
	db interface {
		Tenant(context.Context, string, string, func(pgx.Tx) error) error
	}
	connections *connections.Service
	org         string
	repo        forge.RepoRef
	head        string
	target      string
	tested      string
	files       map[string]map[string][]byte
	queue       bool
	merged      bool
	ciPassing   bool
	check       *forge.ExecutionCheck
	mergeReads  int
	writes      int
	dropCheck   bool
	mu          sync.Mutex
}

func (p *queueContractProvider) authorize(ctx context.Context, id string, fn func(context.Context, pgx.Tx, connections.Connection) error) error {
	return p.db.Tenant(ctx, p.org, "", func(tx pgx.Tx) error {
		c, err := p.connections.MetadataTx(ctx, tx, p.org, id)
		if err != nil {
			return err
		}
		return fn(ctx, tx, c)
	})
}

func (p *queueContractProvider) Read(ctx context.Context, org, id string, op privateconnector.Operation, fn func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if err := p.authorize(ctx, id, fn); err != nil {
		return privateconnector.Result{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch op.Kind {
	case privateconnector.ForgeQueueInspect:
		return privateconnector.Result{MergeEvidence: p.evidence()}, nil
	case privateconnector.ForgeReadExecutionCheck:
		if p.check == nil {
			return privateconnector.Result{}, nil
		}
		copy := *p.check
		return privateconnector.Result{ExecutionCheck: &copy}, nil
	case privateconnector.ForgeMergeResult:
		p.mergeReads++
		if !p.merged && !p.queue {
			return privateconnector.Result{Merge: &forge.MergeResult{State: "open", NativeID: "1", HeadSHA: p.head}}, nil
		}
		if !p.merged {
			return privateconnector.Result{Merge: &forge.MergeResult{State: "queued", NativeID: "queue-1", HeadSHA: p.head}}, nil
		}
		return privateconnector.Result{Merge: &forge.MergeResult{State: "merged", NativeID: "1", HeadSHA: p.head, MergeSHA: strings.Repeat("d", 40)}}, nil
	default:
		return privateconnector.Result{}, privateconnector.ErrUnsupported
	}
}

func (p *queueContractProvider) Write(ctx context.Context, org, id string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) (string, error), current func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	_, err := p.authorizeWrite(ctx, id, authorize)
	if err != nil {
		return privateconnector.Result{}, err
	}
	if err = p.authorize(ctx, id, current); err != nil {
		return privateconnector.Result{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes++
	if op.Kind == privateconnector.ForgeWriteExecutionCheck {
		request := *op.ExecutionCheck
		p.check = &forge.ExecutionCheck{ID: strconv.Itoa(p.writes), SHA: request.SHA, Name: request.Name, State: request.State, PublisherID: "42", OperationID: request.OperationID}
		if p.dropCheck && request.SHA == p.tested {
			return privateconnector.Result{}, nil
		}
		return privateconnector.Result{ExecutionCheck: p.check}, nil
	}
	if op.Kind == privateconnector.ForgeCancelQueue {
		p.queue = false
		return privateconnector.Result{Queue: &forge.QueueState{HeadSHA: p.head, State: "not_queued"}}, nil
	}
	if op.Kind != privateconnector.ForgeMerge {
		return privateconnector.Result{}, privateconnector.ErrUnsupported
	}
	p.queue = true
	return privateconnector.Result{Merge: &forge.MergeResult{State: "queued", NativeID: "queue-1", HeadSHA: p.head}}, nil
}

func (p *queueContractProvider) authorizeWrite(ctx context.Context, id string, fn func(context.Context, pgx.Tx, connections.Connection) (string, error)) (string, error) {
	var out string
	err := p.db.Tenant(ctx, p.org, "", func(tx pgx.Tx) error {
		c, err := p.connections.MetadataTx(ctx, tx, p.org, id)
		if err != nil {
			return err
		}
		out, err = fn(ctx, tx, c)
		return err
	})
	return out, err
}

func (p *queueContractProvider) SourceReader(org, id string, fn func(context.Context, pgx.Tx, connections.Connection) error) source.Reader {
	return source.Reader{
		Manifest: func(ctx context.Context, repo forge.RepoRef, commit string) (forge.SourceManifest, error) {
			if err := p.authorize(ctx, id, fn); err != nil {
				return forge.SourceManifest{}, err
			}
			m := forge.SourceManifest{Repository: repo, CommitSHA: commit, ObjectFormat: "sha1", Proof: "immutable_ref_api", Complete: true}
			for path, body := range p.files[commit] {
				m.Entries = append(m.Entries, forge.SourceEntry{Path: path, SHA: repairBlob(body), Type: "blob", Mode: "100644"})
			}
			return m, nil
		},
		File: func(ctx context.Context, repo forge.RepoRef, path, commit string) (forge.File, error) {
			if err := p.authorize(ctx, id, fn); err != nil {
				return forge.File{}, err
			}
			body := p.files[commit][path]
			return forge.File{Path: path, SHA: repairBlob(body), Content: body}, nil
		},
	}
}

func (p *queueContractProvider) ForProtection(string, map[string]string) providers.Client { return p }

func (p *queueContractProvider) evidence() *forge.MergeEvidence {
	queue := forge.QueueState{State: "not_queued", HeadSHA: p.head, TargetSHA: p.target}
	if p.queue {
		queue = forge.QueueState{ID: "queue-1", State: "queued", HeadSHA: p.head, TestedSHA: p.tested, TargetSHA: p.target}
	}
	return &forge.MergeEvidence{
		ExecutionCheck: forge.CheckRule{Name: forge.QueueExecutionCheckName, PublisherID: "42"},
		Change:         forge.Change{ID: "1", Repository: p.repo, HeadRepository: p.repo, TargetRepository: p.repo, HeadSHA: p.head, TargetSHA: p.target, HeadBranch: "feature", TargetBranch: "main", AuthorID: "author", State: "open"},
		Rules:          forge.Rules{State: domain.Supported, Hash: strings.Repeat("b", 64), RequiredChecks: []forge.CheckRule{{Name: "ci/test", PublisherID: "42"}, {Name: forge.QueueExecutionCheckName, PublisherID: "42"}}, RequireQueue: true, AllowedMergeMethods: []string{"merge"}},
		Checks: []forge.Check{{ID: "ci-1", Name: "ci/test", PublisherID: "42", HeadSHA: func() string {
			if p.queue {
				return p.tested
			}
			return p.head
		}(), Status: func() string {
			if p.ciPassing {
				return "completed"
			}
			return "in_progress"
		}(), Conclusion: func() string {
			if p.ciPassing {
				return "success"
			}
			return ""
		}()}},
		Native: forge.NativeEligibility{State: "eligible", HeadSHA: p.head, TargetSHA: p.target}, Queue: queue,
		Capabilities: forge.Capabilities{Provider: "github", ServerVersion: "3.0"}, ObservedAt: time.Now().UTC(),
	}
}

func TestMergeQueueAdmissionExecutionContract(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET state='healthy',reason='',server_version='3.0',verified_at=now(),settings=jsonb_build_object('auth_kind','github_app','billing_route','forge','app_id','42') WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PutConfig(ctx, f.owner, f.org, f.repo, discovery.Config{MergeAuthority: "reforge"}, 0, "queue-contract"); err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{MergeMethods: []string{"merge"}}}, "queue contract", "queue-contract")
	if err != nil {
		t.Fatal(err)
	}
	simulation, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Merge, MergeMethod: "merge"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, simulation.Hash, "queue contract", "queue-contract"); err != nil {
		t.Fatal(err)
	}
	head, target, tested := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	repo := forge.RepoRef{NativeID: "1", FullName: "acme/repository-0000"}
	provider := &queueContractProvider{db: f.db, connections: f.connections, org: f.org, repo: repo, head: head, target: target, tested: tested, ciPassing: true, files: map[string]map[string][]byte{
		target: {"README.txt": []byte("base\n")}, head: {"README.txt": []byte("candidate\n")}, tested: {"README.txt": []byte("tested\n")},
	}}
	merges := mergecontrol.New(f.db, f.identity, f.connections, policies, provider)
	qualification := mergecontrol.Qualification{Provider: "github", ServerVersion: "3.0", ConnectionVersion: f.connection.Version, EvidenceReference: "queue-contract", EvidenceSHA256: strings.Repeat("a", 64), VerifiedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour), ExactHead: true, QueueExecutionGate: true}
	cfg, err := merges.PutConfig(ctx, f.owner, f.org, f.repo, mergecontrol.Configuration{Enabled: true, CheckPublishers: map[string]string{forge.QueueExecutionCheckName: "42"}, CooperationReference: "cooperationref", Qualification: qualification}, 0, "queue-contract")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := merges.Inspect(ctx, f.owner, f.org, f.repo, "1", "merge", "queue-contract")
	if err != nil || gate.Phase != "queue_admission" || gate.Decision.Outcome != "allow" {
		t.Fatalf("queue admission preview: %+v %v", gate, err)
	}
	key := domain.NewID()
	op, err := merges.Request(ctx, f.owner, f.org, gate.ID, key, "queue-contract")
	if err != nil || op.State != "queued" || op.NativeQueueID != "queue-1" {
		t.Fatalf("queue admission: %+v %v", op, err)
	}
	if provider.writes != 2 {
		t.Fatalf("native admission writes=%d", provider.writes)
	}
	provider.ciPassing = false
	observed, err := merges.Observe(ctx, f.org, op.ID)
	if err != nil || observed.State != "queued" {
		t.Fatalf("queue execution gate: %+v %v", observed, err)
	}
	if provider.writes != 2 {
		t.Fatalf("execution check writes=%d", provider.writes)
	}
	provider.ciPassing = true
	observed, err = merges.Observe(ctx, f.org, op.ID)
	if err != nil || observed.State != "queued" || provider.writes != 3 || provider.check.SHA != tested {
		t.Fatalf("passing candidate did not release the exact gate: %+v writes=%d err=%v", observed, provider.writes, err)
	}
	provider.merged = true
	observed, err = merges.Observe(ctx, f.org, op.ID)
	if err != nil || observed.State != "merged" || observed.NativeResult == nil || observed.NativeResult.NativeID != "1" {
		t.Fatalf("canonical merge observation: %+v %v", observed, err)
	}
	replay, err := merges.Request(ctx, f.owner, f.org, gate.ID, key, "queue-replay")
	if err != nil || replay.ID != op.ID || replay.State != "merged" {
		t.Fatalf("same-key replay: %+v %v", replay, err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		var state string
		return tx.QueryRow(ctx, `SELECT state FROM merge_execution_checks WHERE org_id=$1 AND operation_id=$2`, f.org, op.ID).Scan(&state)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = merges.PutConfig(ctx, f.owner, f.org, f.repo, cfg, cfg.Version, "queue-drift"); err != nil {
		t.Fatal(err)
	}
	if _, err = merges.Request(ctx, f.owner, f.org, gate.ID, domain.NewID(), "queue-drift"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("configuration drift admitted: %v", err)
	}
	other := newInventoryFixture(t, 0)
	if _, err = merges.Get(ctx, other.owner, other.org, op.ID); err == nil {
		t.Errorf("cross-tenant merge read allowed")
	}
	if err = f.db.Tenant(ctx, other.org, "", func(tx pgx.Tx) error {
		var visible int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM merge_execution_checks WHERE operation_id=$1`, op.ID).Scan(&visible); err != nil {
			return err
		}
		if visible != 0 {
			t.Fatal("cross-tenant execution checks visible")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	provider.queue, provider.merged, provider.ciPassing, provider.dropCheck = false, false, true, true
	provider.mu.Unlock()
	gate, err = merges.Inspect(ctx, f.owner, f.org, f.repo, "1", "merge", "queue-lost-response")
	if err != nil || gate.Decision.Outcome != "allow" {
		t.Fatalf("lost-response preview: %+v %v", gate, err)
	}
	lost, err := merges.Request(ctx, f.owner, f.org, gate.ID, domain.NewID(), "queue-lost-response")
	if err != nil || lost.State != "queued" {
		t.Fatalf("lost-response admission: %+v %v", lost, err)
	}
	writes := provider.writes
	if _, err = merges.Observe(ctx, f.org, lost.ID); err == nil {
		t.Fatal("lost execution-check response treated as success")
	}
	restarted := mergecontrol.New(f.db, f.identity, f.connections, policies, provider)
	if _, err = restarted.Observe(ctx, f.org, lost.ID); !errors.Is(err, privateconnector.ErrUncertain) {
		t.Fatalf("uncertain execution check was retried: %v", err)
	}
	if provider.writes != writes+1 {
		t.Fatalf("uncertain check duplicated native write: before=%d after=%d", writes, provider.writes)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE organisations SET paused=true WHERE id=$1`, f.org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	stopped, err := restarted.Observe(ctx, f.org, lost.ID)
	if err != nil || stopped.State != "cancelled" || !stopped.CancelRequested || provider.queue || provider.writes != writes+2 {
		t.Fatalf("pause did not revoke native admission without another check: %+v writes=%d err=%v", stopped, provider.writes, err)
	}
}
