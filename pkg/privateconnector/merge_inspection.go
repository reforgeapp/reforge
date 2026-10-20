package privateconnector

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/source"
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
	if freshness, ok := provider.(forge.ForgeFreshness); ok && (rules.Unprotected || rules.ActorCanBypass) {
		target, err := provider.ResolveRef(ctx, change.TargetRepository, change.TargetBranch)
		if err != nil {
			return evidence, err
		}
		if evidence.TargetChecks, err = provider.ListChecks(ctx, change.TargetRepository, target); err != nil {
			return evidence, err
		}
		behind, err := freshness.Behind(ctx, args.Repository, target, change.HeadSHA)
		if err != nil {
			return evidence, err
		}
		evidence.UpToDate = behind == 0
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

func inspectQueue(ctx context.Context, provider forge.Provider, args ChangeArgs) (*forge.MergeEvidence, error) {
	if provider == nil || args.Repository.NativeID == "" || args.Repository.FullName == "" || args.ChangeID == "" {
		return nil, ErrInvalid
	}
	prerequisites, ok := provider.(forge.ForgeQueuePrerequisites)
	if !ok {
		return nil, ErrUnsupported
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
	if !rules.RequireQueue {
		return nil, fmt.Errorf("queue is not required")
	}
	evidence := &forge.MergeEvidence{Change: change, Rules: rules, ObservedAt: time.Now().UTC()}
	queue, queueErr := provider.ReadQueueState(ctx, args.Repository, change.ID)
	if queueErr != nil {
		if errors.Is(queueErr, ErrUnsupported) || isUnsupported(queueErr) {
			return evidence, ErrUnsupported
		}
		return evidence, queueErr
	}
	evidence.Queue = queue
	if train, ok := provider.(forge.ForgeTrainGate); ok {
		gate, gateErr := train.ReadTrainGate(ctx, args.Repository, change.ID)
		if gateErr != nil {
			return evidence, gateErr
		}
		if gate.HeadSHA != change.HeadSHA || gate.TargetSHA != change.TargetSHA || gate.QueueID != queue.ID || queue.ID != "" && gate.SHA != queue.TestedSHA {
			return evidence, fmt.Errorf("train candidate changed")
		}
		evidence.TrainGate = &gate
	}
	if queue.ID == "" {
		evidence.Checks, err = provider.ListChecks(ctx, change.HeadRepository, change.HeadSHA)
	} else if queue.HeadSHA == change.HeadSHA && queue.TargetSHA == change.TargetSHA && source.ValidSHA(queue.TestedSHA, "sha1") {
		evidence.Checks, err = provider.ListChecks(ctx, change.TargetRepository, queue.TestedSHA)
	}
	if err != nil {
		return evidence, err
	}
	evidence.Native, evidence.ExecutionCheck, err = prerequisites.EvaluateQueuePrerequisites(ctx, args.Repository, change.ID)
	if err != nil {
		return evidence, err
	}
	if evidence.ExecutionCheck.Name != forge.QueueExecutionCheckName || evidence.ExecutionCheck.PublisherID == "" || evidence.Native.HeadSHA != change.HeadSHA || evidence.Native.TargetSHA != change.TargetSHA {
		return evidence, fmt.Errorf("queue prerequisite identity incomplete")
	}
	evidence.Approvals, err = provider.ReadApprovals(ctx, args.Repository, change.ID)
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
