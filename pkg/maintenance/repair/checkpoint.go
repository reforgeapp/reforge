package repair

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

const MaxCheckpointBytes = 16 << 20
const MaxReportBytes = 32 << 20
const maxCheckpointRecentBytes = 32 << 10
const maxCheckpointDependencies = 100

func (c Checkpoint) ProgressDigest() string {
	patches := slices.Clone(c.Patches)
	slices.SortFunc(patches, func(a, b sandbox.Patch) int { return strings.Compare(a.Path, b.Path) })
	dependencies := slices.Clone(c.Dependencies)
	slices.SortFunc(dependencies, func(a, b DependencyUpdate) int {
		return strings.Compare(strings.Join([]string{a.Ecosystem, a.Directory, a.Package}, "\x00"), strings.Join([]string{b.Ecosystem, b.Directory, b.Package}, "\x00"))
	})
	body, _ := json.Marshal(struct {
		Patches      []sandbox.Patch
		Dependencies []DependencyUpdate
	}{patches, dependencies})
	return hashBytes(body)
}

func resumeCheckpoint(current ExecutionContext, previous Run) *Checkpoint {
	p := current.Plan
	old := previous.Context
	if !current.Request.Owner || !old.Request.Owner || !p.Owner || !old.Plan.Owner || previous.Checkpoint == nil || len(previous.Checkpoint.Patches) == 0 || previous.State != "queued" && previous.State != "handoff" || previous.Branch != "" || previous.CandidateSHA != "" || previous.Change != nil || len(previous.CandidateChecks) != 0 || len(previous.CandidateArtifacts) != 0 || current.Finding.ID != old.Finding.ID || current.Finding.RepositoryID != old.Finding.RepositoryID || current.ConnectionID != old.ConnectionID || p.TargetSHA != old.Plan.TargetSHA || p.BaselineSHA != old.Plan.BaselineSHA || p.Recipe.Name != old.Plan.Recipe.Name {
		return nil
	}
	c := *previous.Checkpoint
	c.PlanDigest, c.Turns = p.Digest, 0
	if !c.ValidFor(p) {
		return nil
	}
	return &c
}

func priorCheckpointTx(ctx context.Context, tx pgx.Tx, org string, current ExecutionContext) (*Checkpoint, error) {
	if !current.Request.Owner || !current.Plan.Owner || current.Plan.Recipe.ReadOnly {
		return nil, nil
	}
	var task string
	err := tx.QueryRow(ctx, `SELECT rr.task_id::text FROM repair_runs rr JOIN workflow_tasks t ON t.org_id=rr.org_id AND t.id=rr.task_id WHERE rr.org_id=$1 AND rr.finding_id=$2 AND rr.repository_id=$3 AND t.state='failed' AND rr.checkpoint IS NOT NULL AND rr.context->'plan'->>'target_sha'=$4 AND NOT EXISTS(SELECT 1 FROM workflow_outbox o WHERE o.org_id=rr.org_id AND o.task_id=rr.task_id) ORDER BY t.created_at DESC,t.id DESC LIMIT 1`, org, current.Finding.ID, current.Finding.RepositoryID, current.Plan.TargetSHA).Scan(&task)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	previous, err := loadRun(ctx, tx, org, task)
	if err != nil {
		return nil, err
	}
	return resumeCheckpoint(current, previous), nil
}

func (c Checkpoint) ValidFor(p Plan) bool {
	if !p.Valid() || !p.Owner || c.PlanDigest != p.Digest || c.Turns < 0 || c.Turns > p.Recipe.MaxTurns || !withinFiles(p, c.Patches) || len(c.Dependencies) > maxCheckpointDependencies || len(c.Recent) > maxCheckpointRecentBytes || !utf8.ValidString(c.Recent) {
		return false
	}
	body, err := json.Marshal(c)
	if err != nil || len(body) > MaxCheckpointBytes {
		return false
	}
	seenPaths := map[string]bool{}
	for _, patch := range c.Patches {
		if !guest.ValidPath(patch.Path) || !utf8.ValidString(patch.Path) || seenPaths[patch.Path] || secretFile(patch.Path, patch.Content) || patch.Delete && len(patch.Content) > 0 || bytes.IndexByte(patch.Content, 0) >= 0 || !utf8.Valid(patch.Content) {
			return false
		}
		for _, forbidden := range p.ForbiddenPaths {
			if policy.ForbiddenPath(forbidden, patch.Path) {
				return false
			}
		}
		if p.Recipe.ReadOnly || len(p.Recipe.AllowedPaths) > 0 && !slices.ContainsFunc(p.Recipe.AllowedPaths, func(glob string) bool { return policy.ForbiddenPath(glob, patch.Path) }) {
			return false
		}
		seenPaths[patch.Path] = true
	}
	if !withinBytes(p, c.Patches) {
		return false
	}
	seenDependencies := map[string]bool{}
	for _, update := range c.Dependencies {
		if !update.Valid() {
			return false
		}
		key := strings.Join([]string{update.Ecosystem, update.Directory, update.Package}, "\x00")
		if seenDependencies[key] {
			return false
		}
		seenDependencies[key] = true
	}
	return true
}

func (s *Service) SaveCheckpoint(ctx context.Context, credential string, in Checkpoint) error {
	raw, err := json.Marshal(in)
	if err != nil || len(raw) > MaxCheckpointBytes {
		return auth.ErrInvalid
	}
	return s.runners.WithJob(ctx, credential, "repair.report", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		run, err := loadRun(ctx, tx, l.OrgID, l.TaskID)
		if err != nil {
			return err
		}
		p := run.Context.Plan
		if !p.Owner || !run.Context.Request.Owner || !p.Valid() || !in.ValidFor(p) || run.Context.PolicyHash == "" || run.Context.PolicyHash != t.PolicyHash || run.Context.Finding.RepositoryID != t.RepositoryID || p.Recipe.Name != t.Recipe || p.Recipe.Version != t.RecipeVersion {
			return auth.ErrInvalid
		}
		if t.State != domain.TaskPlanning && t.State != domain.TaskRepairing && t.State != domain.TaskValidating {
			return workflow.ErrPolicy
		}
		if run.State != "queued" && run.State != "handoff" {
			return workflow.ErrPolicy
		}
		if run.Checkpoint != nil && in.Turns < run.Checkpoint.Turns {
			return auth.ErrConflict
		}
		result, err := tx.Exec(ctx, `UPDATE repair_runs SET checkpoint=$3::jsonb,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND state IN ('queued','handoff')`, l.OrgID, l.TaskID, raw)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return workflow.ErrFence
		}
		return nil
	})
}
