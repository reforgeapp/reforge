package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"reforge/internal/customcmd"
	"reforge/internal/domain"
	"reforge/internal/maintenance/repair"
	"reforge/internal/model"
	"reforge/internal/sandbox"
	"reforge/internal/workflow"
)

func RepairProcessor(config sandbox.RuntimeConfig) Processor {
	return func(ctx context.Context, c *Client, j Job) (completion workflow.Completion, failure error) {
		failed := workflow.Completion{Outcome: "failed"}
		run, err := c.RepairRun(ctx, j)
		if err != nil {
			return failed, err
		}
		execution := run.Context
		ctx, cancel := context.WithTimeout(ctx, repair.AttemptTimeout(execution.Plan))
		defer cancel()
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
		cfg.Fetch = func(ctx context.Context, in sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
			if in.JobID != j.Lease.JobID || in.AttemptID != j.Lease.AttemptID {
				return sandbox.Snapshot{}, sandbox.ErrBoundary
			}
			if in.CommitSHA == baseline.CommitSHA {
				return baseline, nil
			}
			if in.CommitSHA == target.CommitSHA {
				return target, nil
			}
			return c.RepairSnapshot(ctx, j, in.CommitSHA)
		}
		runtime, err := sandbox.NewRuntime(cfg)
		if err != nil {
			return failed, err
		}
		defer func() {
			if err := runtime.Close(); err != nil {
				completion = failed
				failure = errors.Join(failure, err)
			}
		}()
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
		dependencies, err := prefetch(ctx, cfg, j.Lease.OrgID, execution.Plan.Image, baseline, target)
		if err != nil {
			return failed, err
		}
		targetFiles, err := repair.Files(target)
		if err != nil {
			return failed, err
		}
		engine := repair.Engine{Dependencies: dependencies, CILogs: execution.CILogs, OpenFixes: execution.OpenFixes, OpenFixFiles: execution.OpenFixFiles, UpdateDependency: updater{cfg: cfg, runtime: runtime, request: sandbox.WorkspaceRequest{JobID: j.Lease.JobID, AttemptID: j.Lease.AttemptID, CommitSHA: execution.Plan.TargetSHA, Trust: trust}, target: targetFiles}.update, Runtime: runtime, JobID: j.Lease.JobID, AttemptID: j.Lease.AttemptID, Trust: trust, Model: execution.Model, MaxOutputTokens: execution.MaxOutputTokens, TurnTimeout: time.Duration(execution.TurnTimeoutMS) * time.Millisecond, Turn: func(ctx context.Context, in model.Turn) (model.TurnResult, error) { return c.ModelTurn(ctx, j, in) }, Artifact: func(ctx context.Context, name string, data []byte) (string, error) {
			m, err := c.Upload(ctx, j, name, "text/plain", data)
			return m.ID, err
		}, Progress: func(_ context.Context, next string) error { return progress(next) }}
		var report repair.Report
		if run.Report != nil && run.Report.State == "validated" {
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
			if _, saveErr := c.saveReport(ctx, j, report); saveErr != nil {
				return failed, saveErr
			}
			if err != nil {
				return failed, err
			}
		}
		run, err = c.RepairStage(ctx, j)
		if err != nil {
			return workflow.Completion{Outcome: "uncertain"}, err
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
