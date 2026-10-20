package discovery

import (
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

func TestRateLimitRetryDelayIsBoundedAndScheduled(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		requested time.Duration
		want      time.Duration
	}{
		{name: "minimum delay", want: 5 * time.Minute},
		{name: "provider deadline", requested: 12 * time.Minute, want: 12 * time.Minute},
		{name: "bounded deadline", requested: 48 * time.Hour, want: 24 * time.Hour},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, limited := rateLimitRetry(&domain.ProviderError{Kind: "rate_limit", RetryAfter: testCase.requested})
			if !limited || got != testCase.want {
				t.Fatalf("retry = %s, rate limited = %t", got, limited)
			}
		})
	}
	if _, limited := rateLimitRetry(&domain.ProviderError{Kind: "auth"}); limited {
		t.Fatal("auth error treated as rate limit")
	}
}

func TestLargeTrackedFileCreatesNeutralMaintenanceFinding(t *testing.T) {
	entry := forge.SourceEntry{Path: "bin/app", SHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Mode: "100755", Type: "blob", Size: 48 << 20}
	findings := largeFileObservations([]forge.SourceEntry{entry, {Path: "small", Type: "blob", Size: 1 << 20}}, "repo-id", Evidence{})
	if len(findings) != 1 || findings[0].Category != "repository_maintenance" || findings[0].Evidence.TrackedFiles[0] != entry || !strings.Contains(findings[0].Title, "bin/app") {
		t.Fatalf("findings=%+v", findings)
	}
}

func TestRepairConflictRequiresSameRepoOpenReforgeBranchAndExplicitConflict(t *testing.T) {
	repo := forge.RepoRef{NativeID: "42", FullName: "owner/repo"}
	base := forge.Change{ID: "12", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadBranch: "reforge/repair/fix", HeadSHA: "abc123", State: "open", MergeStatus: "dirty"}
	for _, test := range []struct {
		name   string
		change forge.Change
		want   bool
	}{
		{name: "github dirty", change: base, want: true},
		{name: "gitlab conflict", change: forge.Change{ID: "12", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadBranch: base.HeadBranch, State: "opened", MergeStatus: "conflict"}, want: true},
		{name: "unknown", change: forge.Change{ID: "12", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadBranch: base.HeadBranch, State: "open", MergeStatus: "unknown"}},
		{name: "behind", change: forge.Change{ID: "12", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadBranch: base.HeadBranch, State: "open", MergeStatus: "behind"}},
		{name: "blocked", change: forge.Change{ID: "12", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadBranch: base.HeadBranch, State: "open", MergeStatus: "blocked"}},
		{name: "draft", change: forge.Change{ID: "12", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadBranch: base.HeadBranch, State: "open", Draft: true, MergeStatus: "dirty"}},
		{name: "human branch", change: forge.Change{ID: "12", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadBranch: "feature/fix", State: "open", MergeStatus: "dirty"}},
		{name: "other reforge branch", change: forge.Change{ID: "12", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadBranch: "reforge/bot/fix", State: "open", MergeStatus: "dirty"}},
		{name: "fork", change: forge.Change{ID: "12", Repository: repo, HeadRepository: forge.RepoRef{NativeID: "43", FullName: "fork/repo"}, TargetRepository: repo, HeadBranch: base.HeadBranch, State: "open", MergeStatus: "dirty"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := RepairConflict(test.change); got != test.want {
				t.Fatalf("RepairConflict=%t want=%t", got, test.want)
			}
		})
	}
	check := repairConflictCheck(base)
	if check.HeadSHA != base.HeadSHA || check.Name != "Reforge branch conflict" || check.Conclusion != "failure" || !failedChecks([]forge.Check{check}) {
		t.Fatalf("synthetic conflict check=%+v", check)
	}
}

func TestBotUpdateJobIncludesDependabotConfigurationCheck(t *testing.T) {
	for _, name := range []string{"Dependabot", "dependabot update", ".github/dependabot.yml", ".github/dependabot.yaml"} {
		if !BotUpdateJob(name) {
			t.Fatalf("%q not classified as bot update job", name)
		}
	}
	if BotUpdateJob("Validate") {
		t.Fatal("ordinary validation classified as bot update job")
	}
}
