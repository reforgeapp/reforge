package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/reforgeapp/reforge/pkg/artifact"
	"github.com/reforgeapp/reforge/pkg/customcmd"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/model"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func RepairProcessor(config sandbox.RuntimeConfig) Processor {
	processor, _ := RepairProcessorWithCloser(config)
	return processor
}

func RepairProcessorWithCloser(config sandbox.RuntimeConfig) (Processor, func() error) {
	var mu sync.Mutex
	var shared sandbox.SandboxRuntime
	var fetchers sync.Map
	sharedRuntime := func(cfg sandbox.RuntimeConfig) (sandbox.SandboxRuntime, error) {
		mu.Lock()
		defer mu.Unlock()
		if shared != nil {
			return shared, nil
		}
		cfg.Fetch = func(ctx context.Context, in sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
			fetch, ok := fetchers.Load(in.JobID + "/" + in.AttemptID)
			if !ok {
				return sandbox.Snapshot{}, sandbox.ErrBoundary
			}
			return fetch.(func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error))(ctx, in)
		}
		runtime, err := NewSandboxRuntime(cfg, cfg.Fetch)
		if err == nil {
			shared = runtime
		}
		return runtime, err
	}
	process := func(ctx context.Context, c *Client, j Job) (completion workflow.Completion, failure error) {
		outer := ctx
		var attempt context.Context
		progressed := false
		defer func() {
			if completion.Outcome == "failed" && progressed && outer.Err() == nil && attempt != nil && errors.Is(attempt.Err(), context.DeadlineExceeded) {
				completion, failure = workflow.Completion{Outcome: "paused", Reason: "Run reached its time limit; continuing from its checkpoint", RetryAfterMS: time.Minute.Milliseconds()}, nil
			}
			if completion.Outcome == "failed" && (errors.Is(failure, ErrTransientControlPlane) || errors.Is(failure, repair.ErrSandbox) || errors.Is(failure, sandbox.ErrResourceLimit)) {
				completion.Retryable = true
			}
			if errors.Is(failure, repair.ErrPaused) {
				completion.Outcome = "paused"
				var delayed domain.Delayed
				if errors.As(failure, &delayed) {
					completion.RetryAfterMS = delayed.After.Milliseconds()
				}
			}
		}()
		failed := workflow.Completion{Outcome: "failed"}
		run, err := c.RepairRun(ctx, j)
		if err != nil {
			return failed, err
		}
		execution := run.Context
		ctx, cancel := context.WithTimeout(ctx, repair.AttemptTimeout(execution.Plan))
		defer cancel()
		attempt = ctx
		baseline, err := c.RepairSnapshot(ctx, j, execution.Plan.BaselineSHA)
		if err != nil {
			return failed, err
		}
		target := baseline
		if execution.Plan.TargetSHA != execution.Plan.BaselineSHA {
			target, err = c.RepairSnapshot(ctx, j, execution.Plan.TargetSHA)
			if err != nil {
				return failed, err
			}
		}
		cfg := config
		if cfg.Backend == "kubernetes" && cfg.Kubernetes != nil {
			kubeConfig := *cfg.Kubernetes
			cfg.Kubernetes = &kubeConfig
			identity, _ := c.Supervisor()
			cfg.Kubernetes.RunnerID = identity.ID
		}
		key := j.Lease.JobID + "/" + j.Lease.AttemptID
		fetch := func(ctx context.Context, in sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
			if in.CommitSHA == baseline.CommitSHA {
				return baseline, nil
			}
			if in.CommitSHA == target.CommitSHA {
				return target, nil
			}
			return c.RepairSnapshot(ctx, j, in.CommitSHA)
		}
		fetchers.Store(key, fetch)
		defer fetchers.Delete(key)
		runtime, err := sharedRuntime(cfg)
		if err != nil {
			return failed, err
		}
		state := j.Task.State
		trust := "untrusted"
		if cfg.Development {
			trust = "development-fixture"
		}
		progress := func(next string) error {
			if state == domain.TaskState(next) {
				return nil
			}
			if err := c.Progress(ctx, j, domain.TaskState(next)); err != nil {
				return err
			}
			state = domain.TaskState(next)
			return nil
		}
		completeWithoutPublication := func() error {
			states := []domain.TaskState{domain.TaskPlanning, domain.TaskRepairing, domain.TaskValidating, domain.TaskPublishing}
			start := 0
			for i, next := range states {
				if state == next {
					start = i + 1
					break
				}
			}
			for _, next := range states[start:] {
				if err := progress(string(next)); err != nil {
					return err
				}
			}
			return nil
		}
		image := planImage(cfg, execution.Plan)
		dependencies, err := prefetch(ctx, cfg, j.Lease.OrgID, image, baseline, target)
		if err != nil {
			return failed, err
		}
		targetFiles, err := repair.Files(target)
		if err != nil {
			return failed, err
		}
		deps := updater{cfg: cfg, runtime: runtime, request: sandbox.WorkspaceRequest{JobID: j.Lease.JobID, AttemptID: j.Lease.AttemptID, CommitSHA: execution.Plan.TargetSHA, Trust: trust, Cache: j.Lease.OrgID}, target: targetFiles}
		engine := repair.Engine{Restore: run.Checkpoint, AllowObsolete: repair.ConflictFinding(run.Context.Finding), ReviewFinding: run.Context.Finding.Source == "repository_review", SaveCheckpoint: func(ctx context.Context, checkpoint repair.Checkpoint) error {
			err := c.RepairCheckpoint(ctx, j, checkpoint)
			if err == nil && (run.Checkpoint == nil || checkpoint.Turns > run.Checkpoint.Turns) {
				progressed = true
			}
			return err
		}, PrepareWorkspace: commandPreparer{cfg: cfg, runtime: runtime, org: j.Lease.OrgID, fetch: fetch, image: image}.prepare, Dependencies: dependencies, CILogs: execution.CILogs, Goal: repair.Goal(execution.Finding) + repair.Instructions(execution.Request.Instructions), OpenFixes: execution.OpenFixes, RecentMerges: execution.RecentMerges, OpenFixFiles: execution.OpenFixFiles, UpdateDependency: deps.update, Regenerate: deps.regenerate, Runtime: runtime, JobID: j.Lease.JobID, AttemptID: j.Lease.AttemptID, Trust: trust, Model: execution.Model, MaxOutputTokens: execution.MaxOutputTokens, TurnTimeout: time.Duration(execution.TurnTimeoutMS) * time.Millisecond, Turn: func(ctx context.Context, in model.Turn) (model.TurnResult, error) { return c.ModelTurn(ctx, j, in) }, Artifact: func(ctx context.Context, name string, data []byte) (string, string, error) {
			m, err := c.Upload(ctx, j, name, "text/plain", artifact.SanitizeTextLog(data))
			return m.ID, m.SHA256, err
		}, Progress: func(_ context.Context, next string) error { return progress(next) }, Log: func(ctx context.Context, kind, message string) {
			_ = c.RepairLogs(ctx, j, []repair.LogEntry{{Kind: kind, Message: message}})
		}}
		var report repair.Report
		if run.Report != nil && (run.Report.Disposition == "superseded" || run.Report.Disposition == "reviewed") {
			if err = completeWithoutPublication(); err != nil {
				return failed, err
			}
			return workflow.Completion{Outcome: "completed"}, nil
		} else if run.Report != nil && run.Report.State == "validated" {
			report = *run.Report
			for _, next := range []domain.TaskState{domain.TaskPlanning, domain.TaskRepairing, domain.TaskValidating} {
				if state == next {
					continue
				}
				if err = c.Progress(ctx, j, next); err != nil {
					return failed, err
				}
				state = next
			}
		} else if execution.CustomProfile != nil {
			report, err = c.runCustomProfile(ctx, j, execution, runtime, baseline, target, engine)
			if _, saveErr := c.saveReport(ctx, j, report); saveErr != nil {
				return failed, saveErr
			}
			if err != nil {
				return failed, err
			}
			if report.State != "validated" {
				return workflow.Completion{Outcome: "completed"}, nil
			}
		} else {
			report, err = engine.Run(ctx, execution.Plan, baseline, target)
			_ = c.RepairLogs(ctx, j, []repair.LogEntry{{Kind: "result", Message: strings.TrimSpace(report.State + " " + report.Disposition + ": " + report.Reason)}})
			if _, saveErr := c.saveReport(ctx, j, report); saveErr != nil {
				return failed, saveErr
			}
			if report.Disposition == "superseded" || report.Disposition == "reviewed" {
				if err = completeWithoutPublication(); err != nil {
					return failed, err
				}
				return workflow.Completion{Outcome: "completed"}, nil
			}
			if err != nil {
				return failed, err
			}
		}
		run, err = c.RepairStage(ctx, j)
		if err != nil {
			return repairStageCompletion(err), err
		}
		native, err := engine.ValidateNative(ctx, execution.Plan, run.CandidateSHA, target, report)
		if err != nil {
			if len(native.Checks) > 0 {
				err = errors.Join(err, c.RepairNativeChecks(ctx, j, native))
			}
			return failed, err
		}
		if _, err = c.RepairPublish(ctx, j, native); err != nil {
			return workflow.Completion{Outcome: "uncertain"}, err
		}
		return workflow.Completion{Outcome: "completed"}, nil
	}
	closeRuntime := func() error {
		mu.Lock()
		runtime := shared
		shared = nil
		mu.Unlock()
		if closer, ok := runtime.(interface{ Close() error }); ok {
			return closer.Close()
		}
		return nil
	}
	return process, closeRuntime
}

func repairStageCompletion(err error) workflow.Completion {
	if errors.Is(err, ErrSourceMoved) {
		return workflow.Completion{Outcome: "failed"}
	}
	return workflow.Completion{Outcome: "uncertain"}
}

func (c *Client) runCustomProfile(ctx context.Context, j Job, execution repair.ExecutionContext, runtime sandbox.SandboxRuntime, baseline, target sandbox.Snapshot, engine repair.Engine) (repair.Report, error) {
	out := repair.Report{PlanDigest: execution.Plan.Digest, State: "handoff", Reason: "Custom command profile did not produce a validated repair", Baseline: []repair.CheckResult{}, Candidate: []repair.CheckResult{}, Target: []repair.CheckResult{}, Patches: []sandbox.Patch{}, Artifacts: []string{}}
	spec := execution.CustomProfile
	authorized, err := c.CustomAuthorize(ctx, j)
	if err != nil {
		out.Reason = "Custom profile dispatch was not authorized"
		return out, err
	}
	timeout := time.Duration(spec.MaxWallSeconds) * time.Second
	if timeout <= 0 || timeout > time.Hour {
		timeout = 10 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout+30*time.Second)
	defer cancel()
	trust := "untrusted"
	if engine.Trust != "" {
		trust = engine.Trust
	}
	workspace, err := runtime.PreparePinnedWorkspace(runCtx, sandbox.WorkspaceRequest{JobID: j.Lease.JobID, AttemptID: j.Lease.AttemptID, CommitSHA: execution.Plan.BaselineSHA, Image: spec.ImageDigest, Trust: trust, Timeout: timeout, Dependencies: engine.Dependencies})
	if err != nil {
		out.Reason = "Custom profile workspace was unavailable"
		return out, err
	}
	defer func() {
		cleanup, stopCleanup := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stopCleanup()
		_ = runtime.Destroy(cleanup, workspace)
	}()
	request, err := json.Marshal(map[string]any{
		"job_id":       j.Lease.JobID,
		"attempt_id":   j.Lease.AttemptID,
		"plan_digest":  execution.Plan.Digest,
		"baseline_sha": execution.Plan.BaselineSHA,
		"target_sha":   execution.Plan.TargetSHA,
		"repository":   execution.Repository.FullName,
		"recipe":       execution.Plan.Recipe.Name,
	})
	if err != nil {
		out.Reason = "Custom profile request could not be encoded"
		return out, err
	}
	executor := customcmd.NewExecutor(customcmd.SandboxLauncher{Runtime: runtime})
	result, execErr := executor.RunSpec(runCtx, *spec, customcmd.Input{JobID: j.Lease.JobID, AttemptID: j.Lease.AttemptID, Workspace: workspace, Request: request, PolicyHash: execution.PolicyHash})
	reportIn := customcmd.ReportInput{RunID: authorized.RunID, State: result.State, Reason: result.Reason, Usage: result.Usage, Events: result.Events}
	if reportIn.State == "" {
		reportIn.State = "unknown"
	}
	if _, reportErr := c.CustomReport(ctx, j, reportIn); reportErr != nil {
		out.Reason = "Custom profile result could not be recorded"
		return out, errors.Join(execErr, reportErr)
	}
	out.Reason = "Custom profile " + reportIn.State + ": " + result.Reason
	if execErr != nil || result.State != "completed_unverified" {
		return out, execErr
	}
	patches, err := c.customProfilePatches(runCtx, runtime, workspace, baseline)
	if err != nil {
		out.Reason = "Custom profile workspace could not be read for validation"
		return out, err
	}
	if len(patches) == 0 {
		out.Reason = "Custom profile exited without changing source; nothing to validate"
		return out, nil
	}
	validated, err := engine.ValidateCustom(ctx, execution.Plan, baseline, target, patches)
	if err != nil {
		return validated, err
	}
	return validated, nil
}

func (c *Client) customProfilePatches(ctx context.Context, runtime sandbox.SandboxRuntime, workspace sandbox.Workspace, baseline sandbox.Snapshot) ([]sandbox.Patch, error) {
	files, err := repair.Files(baseline)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	patches := []sandbox.Patch{}
	for _, path := range paths {
		file, err := runtime.CollectArtifact(ctx, workspace, path)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(file.Data, files[path]) {
			patches = append(patches, sandbox.Patch{Path: path, Content: file.Data})
		}
	}
	return patches, nil
}

func (c *Client) saveReport(ctx context.Context, j Job, report repair.Report) (repair.Run, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	return c.RepairReport(ctx, j, report)
}
