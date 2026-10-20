package campaign

import (
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
)

const (
	repo1 = "00000000-0000-4000-8000-000000000011"
	repo2 = "00000000-0000-4000-8000-000000000012"
	repo3 = "00000000-0000-4000-8000-000000000013"
	repo4 = "00000000-0000-4000-8000-000000000014"
)

func repairInput() *repair.Input {
	return &repair.Input{FindingID: "00000000-0000-4000-8000-000000000021", FindingVersion: 1, Recipe: "go", ModelConnectionID: "00000000-0000-4000-8000-000000000031", ModelRoute: "local", RunnerPoolID: "00000000-0000-4000-8000-000000000041"}
}

func repairMember(id, group string, state string) Member {
	return Member{RepositoryID: id, Group: group, State: state, Input: MemberInput{RepositoryID: id, Repair: repairInput()}}
}

func baseInput(members []MemberInput) Input {
	return Input{Name: "weekly repairs", Kind: "repair", Selection: "team:platform", Members: members, CanarySize: 1, BatchSize: 2, Concurrency: 3, Success: "published", ObservationSeconds: 60, FailureLimit: 1, FailurePercent: 20}
}

func TestValidateRejectsCrossKindDuplicateAndRecoveryMembers(t *testing.T) {
	valid := MemberInput{RepositoryID: repo1, Repair: repairInput()}
	cases := []Input{
		baseInput([]MemberInput{valid, {RepositoryID: repo1, Repair: repairInput()}}),
		func() Input { in := baseInput([]MemberInput{valid}); in.Kind = "pipeline"; return in }(),
		func() Input {
			in := baseInput([]MemberInput{{RepositoryID: repo1, Environment: "production", Pipeline: &deployment.PreviewRequest{RecoveryOf: "00000000-0000-4000-8000-000000000051"}}})
			in.Kind = "pipeline"
			in.Success = "healthy"
			return in
		}(),
	}
	for index, in := range cases {
		if validate(in) == nil {
			t.Fatalf("case %d accepted invalid campaign member", index)
		}
	}
}

func TestAssignCanariesCoversGroupsAndStagesRemainingMembers(t *testing.T) {
	in := Input{CanarySize: 2, BatchSize: 2}
	members := []Member{
		repairMember(repo1, "go-github-v1", "pending"),
		repairMember(repo2, "go-github-v1", "pending"),
		repairMember(repo3, "js-github-v1", "pending"),
		repairMember(repo4, "js-github-v1", "pending"),
	}
	if blockers := assignCanaries(in, members); len(blockers) != 0 {
		t.Fatalf("automatic canary assignment blocked: %v", blockers)
	}
	canaries := 0
	for _, member := range members {
		if member.Canary {
			canaries++
			if member.Stage != 1 {
				t.Fatalf("canary has wrong stage: %+v", member)
			}
		} else if member.Stage < 2 || member.Stage > 3 {
			t.Fatalf("member escaped bounded batches: %+v", member)
		}
	}
	if canaries != 2 {
		t.Fatalf("canary count=%d want 2", canaries)
	}
}

func TestAssignCanariesRejectsExcludedAndIncompleteExplicitSets(t *testing.T) {
	members := []Member{repairMember(repo1, "go", "pending"), repairMember(repo2, "js", "pending"), repairMember(repo3, "python", "excluded")}
	if blockers := assignCanaries(Input{CanarySize: 2, CanaryIDs: []string{repo1, repo3}}, members); len(blockers) == 0 {
		t.Fatal("excluded explicit canary accepted")
	}
	members[2].State = "pending"
	if blockers := assignCanaries(Input{CanarySize: 2, CanaryIDs: []string{repo1, repo2}}, members); len(blockers) == 0 {
		t.Fatal("explicit canaries missing group accepted")
	}
	if blockers := assignCanaries(Input{CanarySize: 3, BatchSize: 1}, members[:2]); len(blockers) == 0 {
		t.Fatal("canary size beyond eligible members accepted")
	}
}

func TestWithinWindowUsesUTCExclusiveEndAndNotBefore(t *testing.T) {
	notBefore := time.Date(2026, time.September, 20, 1, 0, 0, 0, time.UTC)
	in := Input{NotBefore: &notBefore, Windows: []Window{{Weekdays: []int{0}, StartMinute: 60, EndMinute: 120}}}
	cases := []struct {
		now  time.Time
		want bool
	}{
		{time.Date(2026, time.September, 20, 0, 59, 0, 0, time.UTC), false},
		{time.Date(2026, time.September, 20, 1, 0, 0, 0, time.UTC), true},
		{time.Date(2026, time.September, 20, 1, 59, 0, 0, time.UTC), true},
		{time.Date(2026, time.September, 20, 2, 0, 0, 0, time.UTC), false},
		{time.Date(2026, time.September, 21, 1, 0, 0, 0, time.FixedZone("AEST", 10*60*60)), false},
	}
	for _, tc := range cases {
		if got := withinWindow(in, tc.now); got != tc.want {
			t.Errorf("withinWindow(%s)=%v want %v", tc.now, got, tc.want)
		}
	}
}

func TestSummarizeKeepsExcludedAndUnknownDenominators(t *testing.T) {
	members := []Member{
		{State: "excluded"}, {State: "pending"}, {State: "queued"}, {State: "running"}, {State: "observing"},
		{State: "succeeded"}, {State: "failed"}, {State: "cancelled"}, {State: "blocked"}, {State: "unknown"},
	}
	got := summarize(members)
	want := Counts{Total: 10, Excluded: 1, Pending: 1, Running: 3, Succeeded: 1, Failed: 2, Unknown: 2}
	if got != want {
		t.Fatalf("counts=%+v want %+v", got, want)
	}
}
