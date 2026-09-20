package privateconnector

import (
	"context"
	"errors"
	"fmt"
	"time"

	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/source"
)

func inspectMerge(ctx context.Context, provider forge.Provider, args ChangeArgs) (*forge.MergeEvidence, error) {
	if provider == nil || args.Repository.NativeID == "" || args.Repository.FullName == "" || args.ChangeID == "" {
		return nil, ErrInvalid
	}
	change, err := provider.ReadChange(ctx, args.Repository, args.ChangeID)
	if err != nil {
		return nil, err
	}
	if change.ID != args.ChangeID || !mergeIdentity(change, args.Repository, change) {
		return nil, fmt.Errorf("merge change identity incomplete")
	}
	rules, err := provider.ReadEffectiveRules(ctx, args.Repository, change.TargetBranch)
	if err != nil {
		return nil, err
	}
	evidence := &forge.MergeEvidence{Change: change, Rules: rules, Queue: forge.QueueState{State: "not_required"}, ObservedAt: time.Now().UTC()}
	if rules.RequireQueue {
		queue, queueErr := provider.ReadQueueState(ctx, args.Repository, change.ID)
		if queueErr != nil {
			if !errors.Is(queueErr, ErrUnsupported) && !isUnsupported(queueErr) {
				return evidence, queueErr
			}
			queue = forge.QueueState{State: "unsupported"}
		}
		evidence.Queue = queue
		if queue.HeadSHA == change.HeadSHA && queue.TargetSHA == change.TargetSHA && source.ValidSHA(queue.TestedSHA, "sha1") {
			evidence.Checks, err = provider.ListChecks(ctx, change.TargetRepository, queue.TestedSHA)
		} else {
			evidence.Checks = []forge.Check{}
		}
	} else {
		evidence.Checks, err = provider.ListChecks(ctx, change.HeadRepository, change.HeadSHA)
	}
	if err != nil {
		return evidence, err
	}
	evidence.Approvals, err = provider.ReadApprovals(ctx, args.Repository, change.ID)
	if err != nil {
		return evidence, err
	}
	evidence.Native, err = provider.EvaluateNativeEligibility(ctx, args.Repository, change.ID)
	if err != nil {
		return evidence, err
	}
	evidence.Capabilities, err = provider.ProbeCapabilities(ctx)
	if err != nil {
		return evidence, err
	}
	final, err := provider.ReadChange(ctx, args.Repository, args.ChangeID)
	if err != nil {
		return evidence, err
	}
	if !mergeIdentity(final, args.Repository, change) {
		return evidence, fmt.Errorf("merge change moved during inspection")
	}
	evidence.Change = final
	return evidence, nil
}

func mergeIdentity(change forge.Change, repository forge.RepoRef, expected forge.Change) bool {
	return change.ID != "" && change.ID == expected.ID && change.Repository == repository && change.TargetRepository == repository && change.HeadRepository.NativeID != "" && change.TargetRepository.NativeID != "" && change.HeadRepository == expected.HeadRepository && change.TargetRepository == expected.TargetRepository && source.ValidSHA(change.HeadSHA, "sha1") && change.HeadSHA == expected.HeadSHA && source.ValidSHA(change.TargetSHA, "sha1") && change.TargetSHA == expected.TargetSHA && change.HeadBranch != "" && change.HeadBranch == expected.HeadBranch && change.TargetBranch != "" && change.TargetBranch == expected.TargetBranch && change.State == expected.State && change.Draft == expected.Draft
}

func isUnsupported(err error) bool {
	var providerErr *domain.ProviderError
	return errors.As(err, &providerErr) && providerErr.Kind == "unsupported"
}
