package repair

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/policy"
	"reforge/internal/sandbox/guest"
	"reforge/internal/workflow"
)

const maxCheckpointBytes = 12 << 20
const maxCheckpointRecentBytes = 32 << 10
const maxCheckpointDependencies = 100

func (c Checkpoint) ValidFor(p Plan) bool {
	if !p.Valid() || !p.Owner || c.PlanDigest != p.Digest || c.Turns < 0 || c.Turns > p.Recipe.MaxTurns || len(c.Patches) > p.Recipe.MaxFiles || len(c.Dependencies) > maxCheckpointDependencies || len(c.Recent) > maxCheckpointRecentBytes || !utf8.ValidString(c.Recent) {
		return false
	}
	body, err := json.Marshal(c)
	if err != nil || len(body) > maxCheckpointBytes {
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
	if err != nil || len(raw) > maxCheckpointBytes {
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
