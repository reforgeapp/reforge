package mergecontrol

import (
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/policy"
)

func evaluationFixture(now time.Time) (Snapshot, policy.Resolved, Authority) {
	head := strings.Repeat("a", 40)
	target := strings.Repeat("b", 40)
	repo := forge.RepoRef{NativeID: "repo", FullName: "org/repo"}
	rules := forge.Rules{
		State: domain.Supported, Hash: strings.Repeat("d", 64), AllowedMergeMethods: []string{"squash"},
		RequiredChecks:      []forge.CheckRule{{Name: "test", PublisherID: "ci"}},
		RequireStrictTarget: true, StrictTargetEnforced: domain.Supported,
		RequiredApprovals: 1,
	}
	change := forge.Change{ID: "change", Repository: repo, HeadRepository: repo, TargetRepository: repo, HeadSHA: head, TargetSHA: target, HeadBranch: "feature", TargetBranch: "main", AuthorID: "author", State: "open"}
	snapshot := Snapshot{
		Change: change, Rules: rules,
		Checks:       []forge.Check{{Name: "test", PublisherID: "ci", HeadSHA: head, Status: "completed", Conclusion: "success"}},
		Approvals:    []forge.Approval{{ID: "approval", ActorID: "reviewer", HeadSHA: head, State: "approved"}},
		Native:       forge.NativeEligibility{State: "eligible", HeadSHA: head, TargetSHA: target},
		Capabilities: forge.Capabilities{Provider: "gitea", ServerVersion: "1"},
		ObservedAt:   now,
	}
	layers := []policy.Layer{{
		Scope: policy.Scope{Kind: "organisation", ID: "org"}, VersionID: "policy-v1", BindingVersion: 1,
		Policy: policy.Policy{Schema: "maintenance/v1", Allow: policy.Lists{MergeMethods: []string{"squash"}}, ForbiddenPaths: []string{"infra/**"}},
	}}
	resolved := policy.Resolve(layers, "repo", "", false)
	authority := Authority{PathsVerified: true, ValidationHead: head, ValidationTarget: target, ValidationReference: "local-validation", MergeControlled: true, CooperationVerified: true, Qualified: true, ExactHeadEnforced: true, QualificationReference: "qualification", Paths: []string{"src/value.js"}}
	return snapshot, resolved, authority
}

func TestEvaluateProtectionCorpus(t *testing.T) {
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*Snapshot, *policy.Resolved, *Authority, *time.Time)
	}{
		{"wrong publisher", func(s *Snapshot, _ *policy.Resolved, _ *Authority, _ *time.Time) {
			s.Checks[0].PublisherID = "attacker"
		}},
		{"stale approval head", func(s *Snapshot, _ *policy.Resolved, _ *Authority, _ *time.Time) {
			s.Approvals[0].HeadSHA = strings.Repeat("c", 40)
		}},
		{"native head changed", func(s *Snapshot, _ *policy.Resolved, _ *Authority, _ *time.Time) {
			s.Native.HeadSHA = strings.Repeat("c", 40)
		}},
		{"native target changed", func(s *Snapshot, _ *policy.Resolved, _ *Authority, _ *time.Time) {
			s.Native.TargetSHA = strings.Repeat("c", 40)
		}},
		{"strict target missing", func(s *Snapshot, _ *policy.Resolved, _ *Authority, _ *time.Time) {
			s.Rules.StrictTargetEnforced = domain.Unsupported
		}},
		{"actor bypass", func(s *Snapshot, _ *policy.Resolved, _ *Authority, _ *time.Time) { s.Rules.ActorCanBypass = true }},
		{"qualification missing", func(_ *Snapshot, _ *policy.Resolved, a *Authority, _ *time.Time) { a.Qualified = false }},
		{"paths unverified", func(_ *Snapshot, _ *policy.Resolved, a *Authority, _ *time.Time) { a.PathsVerified = false }},
		{"policy forbidden path", func(_ *Snapshot, _ *policy.Resolved, a *Authority, _ *time.Time) {
			a.Paths = []string{"infra/secrets.yml"}
		}},
		{"evidence expired", func(s *Snapshot, _ *policy.Resolved, _ *Authority, n *time.Time) { s.ObservedAt = n.Add(-time.Hour) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, resolved, authority := evaluationFixture(now)
			tc.mutate(&snapshot, &resolved, &authority, &now)
			got := Evaluate(snapshot, resolved, "squash", authority, now)
			if got.Decision.Outcome != "deny" && got.Decision.Outcome != "unknown" {
				t.Fatalf("accepted invalid evidence: %+v", got.Decision)
			}
		})
	}
}

func TestEvaluateAllowsCompleteQualifiedNativeMerge(t *testing.T) {
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	snapshot, resolved, authority := evaluationFixture(now)
	got := Evaluate(snapshot, resolved, "squash", authority, now)
	if got.Decision.Outcome != "allow" {
		t.Fatalf("valid merge rejected: %+v", got.Decision)
	}
}

func TestEvaluateReforgeEnforcedUnprotectedMerge(t *testing.T) {
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	snapshot, resolved, authority := evaluationFixture(now)
	head := snapshot.Change.HeadSHA
	snapshot.Rules = forge.Rules{State: domain.Supported, Hash: snapshot.Rules.Hash, AllowedMergeMethods: []string{"squash"}, ActorCanBypass: true, Unprotected: true}
	old := now.Add(-48 * time.Hour)
	snapshot.Checks = []forge.Check{{Name: "Validate", HeadSHA: head, Status: "completed", Conclusion: "success", CompletedAt: &old}}
	snapshot.TargetChecks = []forge.Check{{Name: "Validate", Conclusion: "success"}}
	snapshot.UpToDate = true
	authority.ReforgeEnforced, authority.ValidationReference = true, ""
	if got := Evaluate(snapshot, resolved, "squash", authority, now); got.Decision.Outcome == "allow" {
		t.Fatal("merge allowed on checks older than the freshness window")
	}
	snapshot.Checks[0].CompletedAt = &now
	if got := Evaluate(snapshot, resolved, "squash", authority, now); got.Decision.Outcome != "allow" {
		t.Fatalf("reforge-enforced merge rejected: %+v", got.Decision)
	}
	snapshot.TargetChecks = append(snapshot.TargetChecks, forge.Check{Name: "Security scan", Conclusion: "success"})
	if got := Evaluate(snapshot, resolved, "squash", authority, now); got.Decision.Outcome == "allow" {
		t.Fatal("merge allowed with a target check missing from the pull request")
	}
	snapshot.TargetChecks, snapshot.UpToDate = snapshot.TargetChecks[:1], false
	if got := Evaluate(snapshot, resolved, "squash", authority, now); got.Decision.Outcome == "allow" {
		t.Fatal("merge allowed while behind the target")
	}
	authority.ReforgeEnforced, snapshot.UpToDate = false, true
	if got := Evaluate(snapshot, resolved, "squash", authority, now); got.Decision.Outcome == "allow" {
		t.Fatal("admin bypass accepted without Reforge enforcement")
	}
	authority.ReforgeEnforced = true
	snapshot.Rules = forge.Rules{State: domain.Unknown, Hash: snapshot.Rules.Hash, AllowedMergeMethods: []string{"squash"}, ActorCanBypass: true, RequiredApprovals: 1}
	snapshot.Approvals = nil
	snapshot.Native.State, snapshot.Native.Blockers, snapshot.Native.Protection = "blocked", []string{"Current-head approvals are missing"}, []string{"Current-head approvals are missing"}
	if got := Evaluate(snapshot, resolved, "squash", authority, now); got.Decision.Outcome != "allow" || !got.ReforgeEnforced {
		t.Fatalf("bypassable ruleset not governed by Reforge: %+v", got.Decision)
	}
	snapshot.Native.Blockers = append(snapshot.Native.Blockers, "Native review requested changes")
	if got := Evaluate(snapshot, resolved, "squash", authority, now); got.Decision.Outcome == "allow" {
		t.Fatal("requested changes waived by Reforge enforcement")
	}
}
