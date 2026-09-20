package runnerclient

import (
	"context"
	"errors"
	"reforge/internal/domain"
	"reforge/internal/maintenance/repair"
	"reforge/internal/model"
	"reforge/internal/sandbox"
	"reforge/internal/workflow"
	"time"
)

func RepairProcessor(config sandbox.RuntimeConfig) Processor {
	return func(ctx context.Context, c *Client, j Job) (completion workflow.Completion, failure error) {
		failed := workflow.Completion{Outcome: "failed"}
		run, err := c.RepairRun(ctx, j)
		if err != nil {
			return failed, err
		}
		execution := run.Context
		ctx, cancel := context.WithTimeout(ctx, time.Duration(execution.Plan.Recipe.TimeoutSeconds)*time.Second)
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
		engine := repair.Engine{Runtime: runtime, JobID: j.Lease.JobID, AttemptID: j.Lease.AttemptID, Trust: trust, Model: execution.Model, MaxOutputTokens: execution.MaxOutputTokens, TurnTimeout: time.Duration(execution.TurnTimeoutMS) * time.Millisecond, Turn: func(ctx context.Context, in model.Turn) (model.TurnResult, error) { return c.ModelTurn(ctx, j, in) }, Artifact: func(ctx context.Context, name string, data []byte) (string, error) {
			m, err := c.Upload(ctx, j, name, "text/plain", data)
			return m.ID, err
		}, Progress: func(ctx context.Context, next string) error {
			if state == domain.TaskState(next) {
				return nil
			}
			if err := c.Progress(ctx, j, domain.TaskState(next)); err != nil {
				return err
			}
			state = domain.TaskState(next)
			return nil
		}}
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
		} else {
			report, err = engine.Run(ctx, execution.Plan, baseline, target)
			if _, saveErr := c.RepairReport(ctx, j, report); saveErr != nil {
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
		native, err := engine.ValidateNative(ctx, execution.Plan, run.CandidateSHA, target, report.Baseline)
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
