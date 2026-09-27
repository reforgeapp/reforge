package mergecontrol

import (
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/policy"
	"reforge/internal/source"
	"slices"
	"strings"
	"time"
)

func Evaluate(snapshot Snapshot, resolved policy.Resolved, method string, authority Authority, now time.Time) Gate {
	change, rules := snapshot.Change, snapshot.Rules
	phase := "merge"
	queueGate := rules.RequireQueue && authority.ExecutionPublisher != "" && snapshot.ExecutionCheck == (forge.CheckRule{Name: forge.QueueExecutionCheckName, PublisherID: authority.ExecutionPublisher}) && slices.Contains(rules.RequiredChecks, snapshot.ExecutionCheck)
	train := snapshot.TrainGate
	if snapshot.Capabilities.Provider == "gitlab" {
		queueGate = queueGate && train != nil && train.CIConfigSHA256 == authority.CIConfigSHA256 && source.ValidSHA(authority.CIConfigSHA256, "sha256") && train.PublisherID == authority.ExecutionPublisher && train.Name == forge.QueueExecutionCheckName && train.HeadSHA == change.HeadSHA && train.TargetSHA == change.TargetSHA
	}
	if queueGate {
		phase = "queue_admission"
		if snapshot.Queue.ID != "" {
			phase = "queue_execution"
		}
	}
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
	reforge := authority.ReforgeEnforced && (rules.Unprotected || rules.ActorCanBypass) && !rules.RequireQueue
	nativeCurrent := snapshot.Native.HeadSHA == change.HeadSHA && snapshot.Native.TargetSHA == change.TargetSHA && (snapshot.Native.State == "eligible" || reforge && snapshot.Native.OnlyProtection())
	ready := (change.State == "open" || change.State == "opened") && !change.Draft && change.AuthorID != "" && change.HeadBranch != "" && change.TargetBranch != "" && source.ValidSHA(change.HeadSHA, "sha1") && source.ValidSHA(change.TargetSHA, "sha1") && change.Repository.NativeID != "" && change.Repository == change.TargetRepository && change.HeadRepository.NativeID != ""
	qualified := authority.Qualified && authority.QualificationReference != "" && snapshot.Capabilities.Provider != "" && snapshot.Capabilities.ServerVersion != "" && (!rules.RequireQueue || queueGate)
	add("execution_authority", ready && nativeCurrent && qualified && authority.PathsVerified && (!rules.ActorCanBypass || reforge), authority.QualificationReference, 0)
	add("native_rules", (rules.State == domain.Supported || reforge) && source.ValidSHA(rules.Hash, "sha256") && slices.Contains(rules.AllowedMergeMethods, method), "native-rules:"+rules.Hash, 0)
	trustedChecks, checked := true, 0
	for _, requirement := range rules.RequiredChecks {
		if queueGate && requirement == snapshot.ExecutionCheck {
			continue
		}
		checked++
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
	if reforge {
		missing, pending := forge.MissingChecks(snapshot.TargetChecks, snapshot.Checks, discovery.BotUpdateJob)
		trustedChecks, checked = len(missing) == 0 && !pending && !StaleChecks(snapshot.Checks, binding.Tested, now), 1
		for _, check := range snapshot.Checks {
			if check.HeadSHA != binding.Tested || !discovery.BotUpdateJob(check.Name) && check.Conclusion != "" && check.Conclusion != "success" && check.Conclusion != "neutral" && check.Conclusion != "skipped" {
				trustedChecks = false
			}
		}
	}
	add("native_checks", nativeCurrent && trustedChecks, "native-checks:"+binding.Tested, 0)
	trustedChecks = trustedChecks && (checked > 0 || queueGate && phase == "queue_execution" && train != nil && train.ChecksReady && train.SHA == binding.Tested && train.QueueID == snapshot.Queue.ID && train.JobID != "" && train.PipelineID != "")
	reviewers := map[string]bool{}
	for _, approval := range snapshot.Approvals {
		if approval.ActorID != "" && approval.ActorID != change.AuthorID && approval.HeadSHA == change.HeadSHA && !approval.Dismissed && strings.EqualFold(approval.State, "approved") {
			reviewers[approval.ActorID] = true
		}
	}
	reviews := reforge || len(reviewers) >= rules.RequiredApprovals && (!rules.RequireCodeOwners || rules.CodeOwnersEnforced == domain.Supported)
	add("native_reviews", nativeCurrent && reviews, "native-reviews:"+change.ID, len(reviewers))
	local := authority.ValidationHead == change.HeadSHA && authority.ValidationTarget == change.TargetSHA && authority.ValidationReference != ""
	if phase == "queue_execution" {
		local = false
	}
	validationRef := authority.ValidationReference
	if !local && trustedChecks {
		validationRef = "native-validation:" + binding.Tested
	}
	add("validation", local || trustedChecks, validationRef, 0)
	strict := rules.RequireStrictTarget && rules.StrictTargetEnforced == domain.Supported
	if rules.RequireQueue {
		strict = qualified && snapshot.Queue.HeadSHA == change.HeadSHA && snapshot.Queue.TargetSHA == change.TargetSHA && snapshot.Queue.TestedSHA != "" && snapshot.Queue.ID != "" && trustedChecks
		if queueGate && phase == "queue_admission" {
			strict = qualified && snapshot.Queue.State == "not_queued" && snapshot.Queue.HeadSHA == change.HeadSHA && snapshot.Queue.TargetSHA == change.TargetSHA
		}
		if queueGate && phase == "queue_execution" {
			strict = strict && source.ValidSHA(snapshot.Queue.TestedSHA, "sha1") && snapshot.Queue.TestedSHA != change.HeadSHA && slices.Contains([]string{"awaiting_checks", "queued", "mergeable", "idle", "fresh"}, snapshot.Queue.State)
		}
	}
	if queueGate && train != nil && phase == "queue_execution" {
		strict = strict && train.State == "manual" && train.ChecksReady && train.SHA == snapshot.Queue.TestedSHA
	}
	if reforge {
		strict = snapshot.UpToDate
	}
	add("target_enforcement", strict, "target-enforcement:"+rules.Hash, 0)
	add("exact_head_guard", qualified && authority.ExactHeadEnforced, "native-capability:"+binding.CapabilityVersion, 0)
	add("merge_authority", authority.MergeControlled && authority.CooperationVerified && !authority.CompanionsBlocked, "repository-merge-authority", 0)
	decision := policy.Evaluate(resolved, policy.Input{Action: policy.Merge, MergeMethod: method, Current: binding, Evidence: evidence, Paths: authority.Paths, Usage: authority.Usage, Now: now})
	if len(authority.Blockers) > 0 {
		decision.Outcome = "deny"
		decision.Blockers = append(decision.Blockers, authority.Blockers...)
	}
	return Gate{ReforgeEnforced: reforge, Phase: phase, Method: method, Snapshot: snapshot, Decision: decision, Binding: binding, ExpiresAt: snapshot.ObservedAt.Add(time.Minute)}
}

const CheckFreshness = 24 * time.Hour

func StaleChecks(checks []forge.Check, head string, now time.Time) bool {
	for _, check := range checks {
		if check.HeadSHA == head && !discovery.BotUpdateJob(check.Name) && !pendingCheck(check.Status) && (check.CompletedAt == nil || now.Sub(*check.CompletedAt) > CheckFreshness) {
			return true
		}
	}
	return false
}

func pendingCheck(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "in_progress", "running", "pending":
		return true
	default:
		return false
	}
}
