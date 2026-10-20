package integration

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/campaign"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

type campaignExecution struct {
	mu       sync.Mutex
	identity *auth.Service
	calls    []string
	state    string
	entered  chan auth.Session
	release  chan struct{}
}

func (x *campaignExecution) Dispatch(ctx context.Context, session auth.Session, org string, c campaign.Campaign, m campaign.Member) (campaign.Execution, error) {
	if x.entered != nil {
		select {
		case x.entered <- session:
		case <-ctx.Done():
			return campaign.Execution{}, ctx.Err()
		}
		select {
		case <-x.release:
		case <-ctx.Done():
			return campaign.Execution{}, ctx.Err()
		}
	}
	e := x.identity.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		x.mu.Lock()
		defer x.mu.Unlock()
		x.calls = append(x.calls, m.RepositoryID)
		return nil
	})
	x.mu.Lock()
	state := x.state
	x.mu.Unlock()
	return campaign.Execution{ActionID: domain.NewID(), State: state, Reason: "Controller contract simulation", VerifiedWindowSeconds: c.Spec.ObservationSeconds}, e
}
func (x *campaignExecution) Observe(ctx context.Context, session auth.Session, org string, c campaign.Campaign, m campaign.Member) (campaign.Execution, error) {
	return campaign.Execution{ActionID: m.ActionID, State: m.State, Reason: m.Reason, VerifiedWindowSeconds: c.Spec.ObservationSeconds}, nil
}
func (x *campaignExecution) Cancel(ctx context.Context, session auth.Session, org string, c campaign.Campaign, m campaign.Member) (campaign.Execution, error) {
	return campaign.Execution{ActionID: m.ActionID, State: "cancelled", Reason: "Simulation cancelled"}, nil
}
func (x *campaignExecution) count() int { x.mu.Lock(); defer x.mu.Unlock(); return len(x.calls) }

type campaignFixture struct {
	*discoveryFixture
	service  *campaign.Service
	executor *campaignExecution
	input    campaign.Input
}

func newCampaignFixture(t *testing.T, size int) *campaignFixture {
	t.Helper()
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	p, e := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if e != nil {
		t.Fatal(e)
	}
	version, e := p.CreateVersion(ctx, f.owner, f.org, policy.Scope{Kind: "organisation", ID: f.org}, policy.Policy{Schema: "maintenance/v1"}, "Campaign contract", "test")
	if e != nil {
		t.Fatal(e)
	}
	sim, e := p.Simulate(ctx, f.owner, f.org, version.ID, f.repo, "", policy.Input{Action: policy.Deploy})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Activate(ctx, f.owner, f.org, version.ID, f.repo, "", 0, sim.Hash, "Campaign contract", "test"); e != nil {
		t.Fatal(e)
	}
	jobs := workflow.New(f.db, f.identity, nil)
	x := &campaignExecution{identity: f.identity, state: "succeeded"}
	s := campaign.New(f.db, f.identity, p, jobs, nil, x)
	jobs.RegisterScopeCheck(s.CheckScopeTx)
	jobs.RegisterCampaignAuthority(s.TaskAuthorityTx)
	pub, key, e := ed25519.GenerateKey(nil)
	if e != nil {
		t.Fatal(e)
	}
	input := campaign.Input{Name: "Canary contract", Kind: "pipeline", Selection: "fixed test selection", CanarySize: 1, BatchSize: 2, Concurrency: 2, Success: "healthy", ObservationSeconds: 1}
	e = f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `UPDATE connections SET state='healthy',server_version='3.0',verified_at=now() WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID); e != nil {
			return e
		}
		for i := 0; i < size; i++ {
			repo := f.repo
			if i > 0 {
				repo = domain.NewID()
				if _, e := tx.Exec(ctx, `INSERT INTO repositories(id,org_id,name,native_id,connection_id,accessible) VALUES($1,$2,$3,$4,$5,true)`, repo, f.org, fmt.Sprintf("campaign/repo-%04d", i), fmt.Sprint(i+100), f.connection.ID); e != nil {
					return e
				}
			}
			env := fmt.Sprintf("canary-%s-%04d", f.org[:8], i)
			cfg := deployment.Configuration{Environment: env, RepositoryID: repo, Version: 1, Enabled: true, Mode: "pipeline", ObservationSeconds: 1, ProvenancePublicKey: base64.StdEncoding.EncodeToString(pub), Workflow: deployment.Workflow{Path: ".github/workflows/deploy.yml"}, Qualification: deployment.Qualification{Provider: "github", ServerVersion: "3.0", ConnectionVersion: f.connection.Version, VerifiedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour), PinnedInputs: true, NativeEnforcement: true, EnvironmentSerialization: true, NoBypass: true}}
			raw, _ := json.Marshal(cfg)
			if _, e := tx.Exec(ctx, `INSERT INTO deployment_configurations(org_id,environment,repository_id,version,document) VALUES($1,$2,$3,1,$4)`, f.org, env, repo, raw); e != nil {
				return e
			}
			proof := deployment.Provenance{OrgID: f.org, RepositoryID: repo, SourceSHA: strings.Repeat("a", 40), ArtifactDigest: "sha256:" + strings.Repeat("b", 64), BuildID: "campaign-contract", IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour)}
			raw, _ = json.Marshal(proof)
			input.Members = append(input.Members, campaign.MemberInput{RepositoryID: repo, Environment: env, Pipeline: &deployment.PreviewRequest{ChangeID: "1", SourceSHA: proof.SourceSHA, ArtifactDigest: proof.ArtifactDigest, Provenance: deployment.SignedProvenance{Document: proof, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw))}}})
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_ = f.db.Tenant(context.Background(), f.org, "", func(tx pgx.Tx) error {
			_, e := tx.Exec(context.Background(), `UPDATE campaigns SET state='cancelled',controller_id=NULL,controller_until=NULL WHERE org_id=$1 AND state NOT IN ('completed','cancelled')`, f.org)
			return e
		})
	})
	return &campaignFixture{f, s, x, input}
}
func (f *campaignFixture) create(t *testing.T) campaign.Campaign {
	t.Helper()
	ctx := context.Background()
	preview, e := f.service.Preview(ctx, f.owner, f.org, f.input)
	if e != nil || len(preview.Blockers) > 0 {
		for _, m := range preview.Members {
			t.Logf("eligibility %s: %s", m.RepositoryID, m.Reason)
		}
		t.Fatalf("preview %+v %v", preview.Blockers, e)
	}
	c, e := f.service.Create(ctx, f.owner, f.org, preview.ID, domain.NewID(), "campaign-test")
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func (f *campaignFixture) control(t *testing.T, c campaign.Campaign, action string) campaign.Campaign {
	t.Helper()
	out, e := f.service.Control(context.Background(), f.owner, f.org, c.ID, action, "Reviewed contract scenario", "continue_current_stage", c.Version, "campaign-test")
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func (f *campaignFixture) step(t *testing.T, id string) campaign.Campaign {
	t.Helper()
	ctx := context.Background()
	e := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE campaigns SET observe_due=now()-interval '1 second' WHERE org_id=$1 AND id=$2`, f.org, id)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	if e = f.service.Step(ctx, f.org); e != nil {
		t.Fatal(e)
	}
	c, e := f.service.Get(ctx, f.owner, f.org, id)
	if e != nil {
		t.Fatal(e)
	}
	return c
}

func TestCampaignCanaryStopsAndFixedMembership(t *testing.T) {
	f := newCampaignFixture(t, 4)
	c := f.create(t)
	c = f.control(t, c, "start")
	f.executor.state = "failed"
	c = f.step(t, c.ID)
	if c.State != "paused" || f.executor.count() != 1 || c.Counts.Pending != 3 {
		t.Fatalf("failed canary expanded: %+v calls=%d", c, f.executor.count())
	}
	if _, e := f.service.Control(context.Background(), f.owner, f.org, c.ID, "resume", "Review failure", "continue_current_stage", c.Version, "test"); !errors.Is(e, auth.ErrConflict) {
		t.Fatalf("failed canary resumed: %v", e)
	}
	f.input.Selection = "changed filter"
	f.input.Members = f.input.Members[:1]
	page, e := f.service.Members(context.Background(), f.owner, f.org, c.ID, "", 100)
	if e != nil || len(page.Items) != 4 {
		t.Fatalf("membership grew/shrank after selection change: %+v %v", page, e)
	}
}
func TestCampaignProgressionAndChangedCanaryPins(t *testing.T) {
	f := newCampaignFixture(t, 4)
	c := f.control(t, f.create(t), "start")
	c = f.step(t, c.ID)
	if c.Counts.Succeeded != 1 || f.executor.count() != 1 {
		t.Fatalf("canary outcome: %+v", c)
	}
	c = f.step(t, c.ID)
	if c.Stage != 2 || c.State != "expanding" {
		t.Fatalf("stage transition: %+v", c)
	}
	c = f.step(t, c.ID)
	c = f.step(t, c.ID)
	if c.Counts.Succeeded != 3 || f.executor.count() != 3 {
		t.Fatalf("bounded second stage: %+v", c)
	}
	c = f.step(t, c.ID)
	if c.Stage != 3 {
		t.Fatalf("third stage: %+v", c)
	}
	c = f.step(t, c.ID)
	c = f.step(t, c.ID)
	if c.State != "completed" || c.Counts.Succeeded != 4 {
		t.Fatalf("completion: %+v", c)
	}
	g := newCampaignFixture(t, 3)
	changed := g.control(t, g.create(t), "start")
	changed = g.step(t, changed.ID)
	e := g.db.Tenant(context.Background(), g.org, "", func(tx pgx.Tx) error {
		_, e := tx.Exec(context.Background(), `UPDATE connections SET version=version+1 WHERE org_id=$1 AND id=$2`, g.org, g.connection.ID)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	changed = g.step(t, changed.ID)
	if changed.State != "paused" || g.executor.count() != 1 {
		t.Fatalf("changed canary evidence expanded: %+v", changed)
	}
}
func TestCampaignPauseFencesConcurrentDispatchAndRevocation(t *testing.T) {
	f := newCampaignFixture(t, 2)
	c := f.control(t, f.create(t), "start")
	f.executor.entered = make(chan auth.Session, 1)
	f.executor.release = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- f.service.Step(context.Background(), f.org) }()
	var grant auth.Session
	select {
	case grant = <-f.executor.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatch was not reached")
	}
	c = f.control(t, c, "pause")
	close(f.executor.release)
	if e := <-done; e != nil && !errors.Is(e, workflow.ErrFence) {
		t.Fatal(e)
	}
	if f.executor.count() != 0 {
		t.Fatal("paused controller performed an effect")
	}
	e := f.identity.WithMutation(context.Background(), grant, f.org, func(pgx.Tx, domain.Actor) error { return nil })
	if !errors.Is(e, workflow.ErrFence) {
		t.Fatalf("stale lease retained authority: %v", e)
	}
	c = f.control(t, c, "resume")
	e = f.db.Tenant(context.Background(), f.org, "", func(tx pgx.Tx) error {
		_, e := tx.Exec(context.Background(), `UPDATE memberships SET role='viewer' WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	c = f.step(t, c.ID)
	if c.State != "paused" || f.executor.count() != 0 {
		t.Fatalf("revoked initiator dispatched: %+v", c)
	}
}
func TestCampaignUnknownCanaryAndExpiredGrantStopExpansion(t *testing.T) {
	f := newCampaignFixture(t, 3)
	c := f.control(t, f.create(t), "start")
	f.executor.state = "unknown"
	c = f.step(t, c.ID)
	c = f.step(t, c.ID)
	if c.State != "paused" || f.executor.count() != 1 {
		t.Fatalf("unknown canary expanded: %+v", c)
	}
	g := newCampaignFixture(t, 2)
	expired := g.control(t, g.create(t), "start")
	e := g.db.Tenant(context.Background(), g.org, "", func(tx pgx.Tx) error {
		_, e := tx.Exec(context.Background(), `UPDATE campaigns SET grant_expires_at=now()-interval '1 second' WHERE org_id=$1 AND id=$2`, g.org, expired.ID)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	expired = g.step(t, expired.ID)
	if expired.State != "paused" || g.executor.count() != 0 {
		t.Fatalf("expired approval dispatched: %+v", expired)
	}
}

func TestCampaignThousandRepositoryTenantFairness(t *testing.T) {
	large := newCampaignFixture(t, 1000)
	big := large.control(t, large.create(t), "start")
	small := newCampaignFixture(t, 1)
	little := small.control(t, small.create(t), "start")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- large.service.Run(ctx) }()
	deadline := time.NewTicker(100 * time.Millisecond)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal("small tenant starved behind thousand-member campaign")
		case <-deadline.C:
			current, e := small.service.Get(context.Background(), small.owner, small.org, little.ID)
			if e != nil {
				t.Fatal(e)
			}
			if current.Counts.Succeeded != 1 {
				continue
			}
			cancel()
			<-done
			page, e := large.service.Members(context.Background(), large.owner, large.org, big.ID, "", 100)
			if e != nil || len(page.Items) != 100 || page.Complete {
				t.Fatalf("large snapshot pagination: %d %v %v", len(page.Items), page.Complete, e)
			}
			return
		}
	}
}
