package workflow

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
)

func EmitTx(ctx context.Context, tx pgx.Tx, event domain.Event) error {
	if !auth.ValidID(event.OrgID) || !auth.ValidID(event.AggregateID) || event.Type == "" || len(event.Type) > 100 || len(event.Data) > 65536 || !json.Valid(event.Data) {
		return auth.ErrInvalid
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workflow_event_heads(org_id) VALUES($1) ON CONFLICT DO NOTHING`, event.OrgID); err != nil {
		return err
	}
	var id int64
	if err := tx.QueryRow(ctx, `UPDATE workflow_event_heads SET last_id=last_id+1 WHERE org_id=$1 RETURNING last_id`, event.OrgID).Scan(&id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO workflow_events(org_id,id,repository_id,type,aggregate_type,aggregate_id,aggregate_version,request_id,data_version,data) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, event.OrgID, id, optional(event.RepositoryID), event.Type, event.AggregateType, event.AggregateID, event.AggregateVersion, event.RequestID, event.DataVersion, event.Data)
	return err
}
func emitTask(ctx context.Context, tx pgx.Tx, t Task, eventType, requestID string) error {
	return EmitTx(ctx, tx, domain.Event{OrgID: t.OrgID, RepositoryID: t.RepositoryID, Type: eventType, AggregateType: "task", AggregateID: t.ID, AggregateVersion: t.Version, RequestID: requestID, DataVersion: 1, Data: mustJSON(map[string]any{"state": t.State, "reason": t.Reason, "cancel_version": t.CancelVersion, "policy_hash": t.PolicyHash})})
}

func (s *Service) Replay(ctx context.Context, session auth.Session, orgID string, after int64, limit int) (EventPage, error) {
	page := EventPage{Items: []domain.Event{}, Cursor: after}
	if after < 0 || limit < 1 || limit > 200 {
		return page, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		var last, retained int64
		err := tx.QueryRow(ctx, `SELECT last_id,retained_after FROM workflow_event_heads WHERE org_id=$1`, orgID).Scan(&last, &retained)
		if errors.Is(err, pgx.ErrNoRows) {
			page.Complete = true
			return nil
		}
		if err != nil {
			return err
		}
		if after < retained {
			return ErrCursor
		}
		if after > last {
			return auth.ErrInvalid
		}
		rows, err := tx.Query(ctx, `SELECT id,org_id::text,coalesce(repository_id::text,''),type,aggregate_type,aggregate_id::text,aggregate_version,occurred_at,request_id,data_version,data FROM workflow_events WHERE org_id=$1 AND id>$2 AND ((repository_id IS NULL AND $3) OR (repository_id IS NOT NULL AND ($4 OR repository_id::text=ANY($5::text[])))) AND id<=$7 ORDER BY id LIMIT $6`, orgID, after, a.Role == domain.Owner, a.AllRepositories, a.RepositoryIDs, limit+1, last)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e domain.Event
			if err = rows.Scan(&e.ID, &e.OrgID, &e.RepositoryID, &e.Type, &e.AggregateType, &e.AggregateID, &e.AggregateVersion, &e.OccurredAt, &e.RequestID, &e.DataVersion, &e.Data); err != nil {
				return err
			}
			page.Items = append(page.Items, e)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		page.Complete = len(page.Items) <= limit
		page.ScanAfter = last
		if !page.Complete {
			page.Items = page.Items[:limit]
			page.ScanAfter = page.Items[limit-1].ID
		}
		if len(page.Items) > 0 {
			page.Cursor = page.Items[len(page.Items)-1].ID
		}
		return nil
	})
	return page, err
}

func (s *Service) PruneEvents(ctx context.Context, orgID string, through int64) error {
	if !auth.ValidID(orgID) || through < 0 {
		return auth.ErrInvalid
	}
	return s.db.Tenant(ctx, orgID, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, orgID); err != nil {
			return err
		}
		var last int64
		if err := tx.QueryRow(ctx, `SELECT last_id FROM workflow_event_heads WHERE org_id=$1 FOR UPDATE`, orgID).Scan(&last); err != nil {
			return err
		}
		if through > last {
			return auth.ErrInvalid
		}
		if _, err := tx.Exec(ctx, `DELETE FROM workflow_events WHERE org_id=$1 AND id<=$2`, orgID, through); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE workflow_event_heads SET retained_after=GREATEST(retained_after,$2) WHERE org_id=$1`, orgID, through)
		return err
	})
}
