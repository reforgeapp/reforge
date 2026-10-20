package repair

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

const maxRunLogs = 5000

type LogEntry struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type LogRecord struct {
	Seq       int64     `json:"seq"`
	AttemptID string    `json:"attempt_id"`
	Kind      string    `json:"kind"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) AppendLogs(ctx context.Context, credential string, entries []LogEntry) error {
	if len(entries) == 0 || len(entries) > 50 {
		return auth.ErrInvalid
	}
	for i, entry := range entries {
		if !slices.Contains([]string{"stage", "model", "tool", "result"}, entry.Kind) {
			return auth.ErrInvalid
		}
		entries[i].Message = clipLog(entry.Message)
	}
	return s.runners.WithJob(ctx, credential, "repair.report", func(tx pgx.Tx, l workflow.Lease, _ workflow.Task) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, l.TaskID); err != nil {
			return err
		}
		var next int64
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(seq),0) FROM run_logs WHERE org_id=$1 AND task_id=$2`, l.OrgID, l.TaskID).Scan(&next); err != nil {
			return err
		}
		for _, entry := range entries {
			if next >= maxRunLogs {
				return nil
			}
			next++
			if _, err := tx.Exec(ctx, `INSERT INTO run_logs(org_id,task_id,seq,attempt_id,kind,message) VALUES($1,$2,$3,$4,$5,$6)`, l.OrgID, l.TaskID, next, l.AttemptID, entry.Kind, entry.Message); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) Logs(ctx context.Context, session auth.Session, org, id string, after int64) ([]LogRecord, error) {
	out := []LogRecord{}
	t, err := s.workflow.Get(ctx, session, org, id)
	if err != nil {
		return out, err
	}
	err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, t.RepositoryID) {
			return auth.ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT seq,attempt_id::text,kind,message,created_at FROM run_logs WHERE org_id=$1 AND task_id=$2 AND seq>$3 ORDER BY seq LIMIT 500`, org, id, after)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[LogRecord])
		return err
	})
	return out, err
}

func clipLog(message string) string {
	message = strings.ToValidUTF8(message, "")
	if len(message) <= 16<<10 {
		return message
	}
	cut := 16 << 10
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	return message[:cut] + "\n[truncated]"
}
