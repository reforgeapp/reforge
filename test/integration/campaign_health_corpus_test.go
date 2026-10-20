package integration

import (
	"context"
	"testing"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/campaign"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

type campaignMissingHealth struct {
	*campaignExecution
}

func (x campaignMissingHealth) Dispatch(ctx context.Context, session auth.Session, org string, c campaign.Campaign, m campaign.Member) (campaign.Execution, error) {
	result, err := x.campaignExecution.Dispatch(ctx, session, org, c, m)
	result.VerifiedWindowSeconds = 0
	return result, err
}

func campaignServiceWithExecutor(t *testing.T, f *campaignFixture, executor campaign.Executor) *campaign.Service {
	t.Helper()
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	jobs := workflow.New(f.db, f.identity, nil)
	service := campaign.New(f.db, f.identity, policies, jobs, nil, executor)
	jobs.RegisterScopeCheck(service.CheckScopeTx)
	jobs.RegisterCampaignAuthority(service.TaskAuthorityTx)
	return service
}

func TestCampaignMissingHealthStopsExpansion(t *testing.T) {
	f := newCampaignFixture(t, 3)
	service := campaignServiceWithExecutor(t, f, campaignMissingHealth{f.executor})
	f.service = service
	c := f.control(t, f.create(t), "start")
	c = f.step(t, c.ID)
	if c.State != "paused" || c.Counts.Unknown != 1 || f.executor.count() != 1 {
		t.Fatalf("missing health expanded: %+v dispatches=%d", c, f.executor.count())
	}
	c = f.step(t, c.ID)
	if c.State != "paused" || f.executor.count() != 1 {
		t.Fatalf("paused campaign dispatched after missing health: %+v dispatches=%d", c, f.executor.count())
	}
}

func TestCampaignControllerRestartReplaysMemberWithoutRedispatch(t *testing.T) {
	f := newCampaignFixture(t, 3)
	c := f.control(t, f.create(t), "start")
	c = f.step(t, c.ID)
	if c.Counts.Succeeded != 1 || f.executor.count() != 1 {
		t.Fatalf("initial canary: %+v dispatches=%d", c, f.executor.count())
	}
	members, err := f.service.Members(context.Background(), f.owner, f.org, c.ID, "", 100)
	if err != nil || len(members.Items) != 3 {
		t.Fatalf("persisted campaign members: %+v %v", members, err)
	}
	canaryIndex := -1
	for i := range members.Items {
		if members.Items[i].Canary {
			canaryIndex = i
			break
		}
	}
	if canaryIndex < 0 || members.Items[canaryIndex].ActionID == "" {
		t.Fatalf("persisted canary action: %+v", members.Items)
	}
	firstAction := members.Items[canaryIndex].ActionID
	f.service = campaignServiceWithExecutor(t, f, f.executor)
	c = f.step(t, c.ID)
	members, err = f.service.Members(context.Background(), f.owner, f.org, c.ID, "", 100)
	if err != nil || len(members.Items) != 3 || members.Items[canaryIndex].ActionID != firstAction || f.executor.count() != 1 {
		t.Fatalf("restart duplicated canary: member=%+v dispatches=%d err=%v", members.Items[canaryIndex], f.executor.count(), err)
	}
	c = f.step(t, c.ID)
	if c.Stage != 2 || c.State != "expanding" || c.Counts.Succeeded != 2 || f.executor.count() != 2 {
		t.Fatalf("replay did not preserve the canary while entering the next stage: %+v dispatches=%d", c, f.executor.count())
	}
}
