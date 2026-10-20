package integration

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/reforgeapp/reforge/pkg/auth"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/mergecontrol"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/providers"
)

type botRevalidationProvider struct {
	*queueContractProvider
	companionState  string
	companionBranch string
	companionHead   string
	originalState   string
}

func (p *botRevalidationProvider) ForProtection(string, map[string]string) providers.Client { return p }

func (p *botRevalidationProvider) Read(ctx context.Context, org, id string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if op.Kind != privateconnector.ForgeReadChange {
		return p.queueContractProvider.Read(ctx, org, id, op, authorize)
	}
	if err := p.authorize(ctx, id, authorize); err != nil {
		return privateconnector.Result{}, err
	}
	changeID := op.Change.ChangeID
	state, head, branch := p.originalState, p.head, "feature"
	if changeID == "2" {
		state, head, branch = p.companionState, p.companionHead, p.companionBranch
	}
	merged := state == "merged"
	return privateconnector.Result{Change: &forge.Change{ID: changeID, Repository: p.repo, HeadRepository: p.repo, TargetRepository: p.repo, HeadSHA: head, HeadBranch: branch, TargetSHA: p.target, TargetBranch: "main", State: state, MergeSHA: func() string {
		if merged {
			return strings.Repeat("e", 40)
		}
		return ""
	}()}}, nil
}

func TestMergeBotRevalidationCurrentAuthorityContract(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET state='healthy',reason='',server_version='3.0',verified_at=now(),settings=jsonb_build_object('auth_kind','github_app','billing_route','forge','app_id','42') WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PutConfig(ctx, f.owner, f.org, f.repo, discovery.Config{MergeAuthority: "reforge"}, 0, "bot-contract"); err != nil {
		t.Fatal(err)
	}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := policies.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{MergeMethods: []string{"merge"}}}, "bot contract", "bot-contract")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := policies.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Merge, MergeMethod: "merge"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policies.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, sim.Hash, "bot contract", "bot-contract"); err != nil {
		t.Fatal(err)
	}
	head, target := strings.Repeat("a", 40), strings.Repeat("b", 40)
	repo := forge.RepoRef{NativeID: "1", FullName: "acme/repository-0000"}
	provider := &botRevalidationProvider{queueContractProvider: &queueContractProvider{db: f.db, connections: f.connections, org: f.org, repo: repo, head: head, target: target, tested: head, ciPassing: true, files: map[string]map[string][]byte{target: {"base.txt": []byte("base\n")}, head: {"base.txt": []byte("head\n")}}}, companionState: "open", companionHead: strings.Repeat("c", 40), originalState: "open"}
	merges := mergecontrol.New(f.db, f.identity, f.connections, policies, provider)
	qualification := mergecontrol.Qualification{Provider: "github", ServerVersion: "3.0", ConnectionVersion: f.connection.Version, EvidenceReference: "bot-contract", EvidenceSHA256: strings.Repeat("a", 64), VerifiedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour), ExactHead: true, QueueExecutionGate: true}
	if _, err = merges.PutConfig(ctx, f.owner, f.org, f.repo, mergecontrol.Configuration{Enabled: true, CheckPublishers: map[string]string{forge.QueueExecutionCheckName: "42"}, CooperationReference: "cooperationref", Qualification: qualification}, 0, "bot-contract"); err != nil {
		t.Fatal(err)
	}
	finding := f.observe(t, f.observation("bot-change", "main", head, nil))
	task, operation := domain.NewID(), domain.NewID()
	provider.companionBranch = "reforge/repair/" + task
	contextRaw, _ := json.Marshal(map[string]any{"finding": map[string]any{"evidence": map[string]any{"change": map[string]string{"id": "1", "target_branch": "main"}}}})
	nativeRaw, _ := json.Marshal(map[string]any{"id": "2", "repository": repo, "head_sha": provider.companionHead, "head_branch": provider.companionBranch, "target_branch": "main", "state": "open"})
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,'repair','1','main',$7,$7,'completed',1,$8)`, f.org, task, f.repo, operation, "bot-contract", strings.Repeat("a", 64), strings.Repeat("a", 64), f.owner.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,state,candidate_sha,native_change,branch) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'published',$9,$10,$11)`, f.org, task, f.repo, finding.ID, f.owner.User.ID, finding.Version, finding.EvidenceDigest, contextRaw, provider.companionHead, nativeRaw, provider.companionBranch)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = merges.ObserveBotRepair(ctx, f.org, task); err != nil {
		t.Fatal(err)
	}
	rows, err := merges.Revalidations(ctx, f.owner, f.org, f.repo, "1")
	if err != nil || len(rows) != 1 || rows[0].State != "waiting_companion" {
		t.Fatalf("open companion: %+v %v", rows, err)
	}
	provider.companionState = "merged"
	target = strings.Repeat("e", 40)
	provider.target = target
	provider.files[target] = map[string][]byte{"base.txt": []byte("base with companion\n")}
	provider.ciPassing = false
	if err = merges.ObserveBotRepair(ctx, f.org, task); err != nil {
		t.Fatal(err)
	}
	rows, _ = merges.Revalidations(ctx, f.owner, f.org, f.repo, "1")
	if len(rows) != 1 || rows[0].State != "blocked" {
		t.Fatalf("failed original CI: %+v", rows)
	}
	provider.ciPassing = true
	head = strings.Repeat("f", 40)
	provider.head = head
	provider.files[head] = map[string][]byte{"base.txt": []byte("refreshed bot head\n")}
	merges = mergecontrol.New(f.db, f.identity, f.connections, policies, provider)
	if err = merges.ObserveBotRepair(ctx, f.org, task); err != nil {
		t.Fatal(err)
	}
	rows, _ = merges.Revalidations(ctx, f.owner, f.org, f.repo, "1")
	if len(rows) != 1 || rows[0].State != "ready" || rows[0].Gate == nil {
		t.Fatalf("fresh ready gate: %+v", rows)
	}
	if rows[0].Gate.Snapshot.Change.HeadSHA != head || rows[0].Gate.Binding.Target != target {
		t.Fatalf("gate binding: %+v", rows[0].Gate.Binding)
	}
	provider.companionHead = strings.Repeat("d", 40)
	if err = merges.ObserveBotRepair(ctx, f.org, task); err == nil {
		t.Fatal(err)
	}
	rows, _ = merges.Revalidations(ctx, f.owner, f.org, f.repo, "1")
	if len(rows) != 1 || rows[0].State != "blocked" {
		t.Fatalf("stale companion: %+v", rows)
	}
	other := newInventoryFixture(t, 0)
	if rows, err = merges.Revalidations(ctx, f.owner, other.org, f.repo, "1"); err != nil || len(rows) != 0 {
		t.Fatalf("foreign tenant rows=%+v error=%v", rows, err)
	}
	if provider.writes != 0 {
		t.Fatal("read-only revalidation mutated provider")
	}
	provider.companionHead = strings.Repeat("c", 40)
	if err = merges.ObserveBotRepair(ctx, f.org, task); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET role='viewer' WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = merges.ObserveBotRepair(ctx, f.org, task); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("revoked author: %v", err)
	}
	rows, err = merges.Revalidations(ctx, f.owner, f.org, f.repo, "1")
	if err != nil || len(rows) != 1 || rows[0].State != "blocked" || rows[0].Gate != nil {
		t.Fatalf("revoked authority state: %+v %v", rows, err)
	}
	if err = merges.ObserveBotRepair(ctx, other.org, task); err == nil {
		t.Fatal("other tenant observed foreign task")
	}
}
