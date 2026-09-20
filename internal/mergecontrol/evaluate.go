package mergecontrol

import (
	"reforge/internal/domain"
	"reforge/internal/policy"
	"reforge/internal/source"
	"slices"
	"strings"
	"time"
)

func Evaluate(snapshot Snapshot, resolved policy.Resolved, method string, authority Authority, now time.Time) Gate {
	change, rules := snapshot.Change, snapshot.Rules
	binding := policy.Binding{Head: change.HeadSHA, Target: change.TargetSHA, Tested: change.HeadSHA, PolicyHash: resolved.Hash, ProviderRules: rules.Hash, CapabilityVersion: snapshot.Capabilities.Provider + "/" + snapshot.Capabilities.ServerVersion}
	if rules.RequireQueue && snapshot.Queue.TestedSHA != "" {
		binding.Tested = snapshot.Queue.TestedSHA
	}
	evidence := []policy.Evidence{}
	add := func(id string, satisfied bool, reference string, approvals int) {
		state := "unknown"
		if satisfied {
			state = "satisfied"
		}
		evidence = append(evidence, policy.Evidence{ID: id, State: state, Reference: reference, Binding: binding, ObservedAt: snapshot.ObservedAt, Approvals: approvals})
	}
	nativeCurrent := snapshot.Native.HeadSHA == change.HeadSHA && snapshot.Native.TargetSHA == change.TargetSHA && snapshot.Native.State == "eligible"
	ready := (change.State == "open" || change.State == "opened") && !change.Draft && change.AuthorID != "" && change.HeadBranch != "" && change.TargetBranch != "" && source.ValidSHA(change.HeadSHA, "sha1") && source.ValidSHA(change.TargetSHA, "sha1") && change.Repository.NativeID != "" && change.Repository == change.TargetRepository && change.HeadRepository.NativeID != ""
	qualified := authority.Qualified && authority.QualificationReference != "" && snapshot.Capabilities.Provider != "" && snapshot.Capabilities.ServerVersion != ""
	add("execution_authority", ready && nativeCurrent && qualified && authority.PathsVerified && !rules.ActorCanBypass, authority.QualificationReference, 0)
	add("native_rules", rules.State == domain.Supported && source.ValidSHA(rules.Hash, "sha256") && slices.Contains(rules.AllowedMergeMethods, method), "native-rules:"+rules.Hash, 0)
	trustedChecks := len(rules.RequiredChecks) > 0
	for _, requirement := range rules.RequiredChecks {
		matched, failed := false, false
		if requirement.Name == "" || requirement.PublisherID == "" {
			trustedChecks = false
			continue
		}
		for _, check := range snapshot.Checks {
			if check.Name != requirement.Name || check.PublisherID != requirement.PublisherID || check.HeadSHA != binding.Tested {
				continue
			}
			if check.Conclusion == "success" && (check.Status == "completed" || check.Status == "success") {
				matched = true
			} else {
				failed = true
			}
		}
		trustedChecks = trustedChecks && matched && !failed
	}
	add("native_checks", nativeCurrent && (len(rules.RequiredChecks) == 0 || trustedChecks), "native-checks:"+binding.Tested, 0)
	reviewers := map[string]bool{}
	for _, approval := range snapshot.Approvals {
		if approval.ActorID != "" && approval.ActorID != change.AuthorID && approval.HeadSHA == change.HeadSHA && !approval.Dismissed && strings.EqualFold(approval.State, "approved") {
			reviewers[approval.ActorID] = true
		}
	}
	reviews := len(reviewers) >= rules.RequiredApprovals && (!rules.RequireCodeOwners || rules.CodeOwnersEnforced == domain.Supported)
	add("native_reviews", nativeCurrent && reviews, "native-reviews:"+change.ID, len(reviewers))
	local := authority.ValidationHead == change.HeadSHA && authority.ValidationTarget == change.TargetSHA && authority.ValidationReference != ""
	validationRef := authority.ValidationReference
	if !local && trustedChecks {
		validationRef = "native-validation:" + binding.Tested
	}
	add("validation", local || trustedChecks, validationRef, 0)
	strict := rules.RequireStrictTarget && rules.StrictTargetEnforced == domain.Supported
	if rules.RequireQueue {
		strict = qualified && snapshot.Queue.HeadSHA == change.HeadSHA && snapshot.Queue.TargetSHA == change.TargetSHA && snapshot.Queue.TestedSHA != "" && snapshot.Queue.ID != "" && trustedChecks
	}
	add("target_enforcement", strict, "target-enforcement:"+rules.Hash, 0)
	add("exact_head_guard", qualified && authority.ExactHeadEnforced, "native-capability:"+binding.CapabilityVersion, 0)
	add("merge_authority", authority.MergeControlled && authority.CooperationVerified && !authority.CompanionsBlocked, "repository-merge-authority", 0)
	decision := policy.Evaluate(resolved, policy.Input{Action: policy.Merge, MergeMethod: method, Current: binding, Evidence: evidence, Paths: authority.Paths, Usage: authority.Usage, Now: now})
	return Gate{Method: method, Snapshot: snapshot, Decision: decision, Binding: binding, ExpiresAt: snapshot.ObservedAt.Add(time.Minute)}
}
