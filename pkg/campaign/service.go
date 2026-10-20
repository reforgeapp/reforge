package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

const columns = `id::text,name,kind,state,reason,version,stage,requested_by::text,spec,created_at,updated_at,observing_since,grant_expires_at`

func scan(row pgx.Row) (Campaign, error) {
	var c Campaign
	var raw []byte
	e := row.Scan(&c.ID, &c.Name, &c.Kind, &c.State, &c.Reason, &c.Version, &c.Stage, &c.RequestedBy, &raw, &c.CreatedAt, &c.UpdatedAt, &c.ObservingSince, &c.GrantExpiresAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return c, auth.ErrForbidden
	}
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(raw, &c.Spec)
	return c, e
}

func load(ctx context.Context, tx pgx.Tx, org, id string) (Campaign, error) {
	return scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM campaigns WHERE org_id=$1 AND id=$2`, org, id))
}

func readable(ctx context.Context, tx pgx.Tx, org, id string, a domain.Actor) error {
	var allowed bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM campaigns WHERE org_id=$1 AND id=$2) AND NOT EXISTS(SELECT 1 FROM campaign_repository_scopes WHERE org_id=$1 AND campaign_id=$2 AND NOT ($3 OR repository_id=ANY($4::uuid[])))`, org, id, a.AllRepositories, a.RepositoryIDs).Scan(&allowed)
	if e != nil {
		return e
	}
	if !allowed {
		return auth.ErrForbidden
	}
	return nil
}

func countTx(ctx context.Context, tx pgx.Tx, org, id string) (Counts, error) {
	var c Counts
	e := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE state='excluded'),count(*) FILTER(WHERE state='pending'),count(*) FILTER(WHERE state IN ('queued','running','observing')),count(*) FILTER(WHERE state='succeeded'),count(*) FILTER(WHERE state IN ('failed','cancelled')),count(*) FILTER(WHERE state IN ('blocked','unknown')) FROM campaign_members WHERE org_id=$1 AND campaign_id=$2`, org, id).Scan(&c.Total, &c.Excluded, &c.Pending, &c.Running, &c.Succeeded, &c.Failed, &c.Unknown)
	return c, e
}

func emit(ctx context.Context, tx pgx.Tx, org, actor, action, id, request string, version int64, data any) error {
	raw, e := json.Marshal(data)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, domain.NewID(), org, actor, action, id, request, raw); e != nil {
		return e
	}
	return workflow.EmitTx(ctx, tx, domain.Event{OrgID: org, Type: action, AggregateType: "campaign", AggregateID: id, AggregateVersion: version, DataVersion: 1, Data: raw, RequestID: request})
}

func (s *Service) Create(ctx context.Context, session auth.Session, org, previewID, key, request string) (Campaign, error) {
	var out Campaign
	if session.AutomationID() != "" {
		return out, auth.ErrForbidden
	}
	if !auth.ValidID(previewID) || key == "" || len(key) > 128 {
		return out, auth.ErrInvalid
	}
	e := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		var oldID, oldPreview string
		e := tx.QueryRow(ctx, `SELECT id::text,preview_id::text FROM campaigns WHERE org_id=$1 AND idempotency_key=$2`, org, key).Scan(&oldID, &oldPreview)
		if e == nil {
			if e = readable(ctx, tx, org, oldID, a); e != nil {
				return e
			}
			if oldPreview != previewID {
				return auth.ErrConflict
			}
			out, e = load(ctx, tx, org, oldID)
			return e
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		var raw []byte
		e = tx.QueryRow(ctx, `SELECT document FROM campaign_previews WHERE org_id=$1 AND id=$2 AND requested_by=$3 AND expires_at>now()`, org, previewID, a.UserID).Scan(&raw)
		if errors.Is(e, pgx.ErrNoRows) {
			return auth.ErrConflict
		}
		if e != nil {
			return e
		}
		var p Preview
		if e = json.Unmarshal(raw, &p); e != nil {
			return e
		}
		if len(p.Blockers) > 0 {
			return auth.ErrConflict
		}
		var active int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM campaigns WHERE org_id=$1 AND state NOT IN ('completed','cancelled','failed')`, org).Scan(&active); e != nil {
			return e
		}
		if active >= 100 {
			return auth.ErrConflict
		}
		for _, m := range p.Members {
			for _, repo := range m.Repositories {
				if !auth.CanReadRepository(a, repo) {
					return auth.ErrForbidden
				}
			}
		}
		spec := p.Input
		spec.Members = []MemberInput{}
		raw, e = json.Marshal(spec)
		if e != nil {
			return e
		}
		id := domain.NewID()
		if _, e = tx.Exec(ctx, `INSERT INTO campaigns(org_id,id,preview_id,idempotency_key,requested_by,requested_session_id,spec,name,kind) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, org, id, previewID, key, a.UserID, session.ID, raw, spec.Name, spec.Kind); e != nil {
			return e
		}
		for _, m := range p.Members {
			m.ID = domain.NewID()
			raw, e = json.Marshal(m)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO campaign_members(org_id,campaign_id,id,repository_id,document,state,reason,stage) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, org, id, m.ID, m.RepositoryID, raw, m.State, m.Reason, m.Stage); e != nil {
				return e
			}
			for _, repo := range m.Repositories {
				if _, e = tx.Exec(ctx, `INSERT INTO campaign_repository_scopes(org_id,campaign_id,repository_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, org, id, repo); e != nil {
					return e
				}
			}
		}
		out, e = load(ctx, tx, org, id)
		if e != nil {
			return e
		}
		out.Counts = summarize(p.Members)
		return emit(ctx, tx, org, a.UserID, "campaign.created", id, request, 1, map[string]any{"preview_hash": p.Hash, "members": out.Counts.Total, "excluded": out.Counts.Excluded})
	})
	return out, e
}

func (s *Service) Get(ctx context.Context, session auth.Session, org, id string) (Campaign, error) {
	var out Campaign
	if !auth.ValidID(id) {
		return out, auth.ErrInvalid
	}
	e := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if e := readable(ctx, tx, org, id, a); e != nil {
			return e
		}
		var e error
		out, e = load(ctx, tx, org, id)
		if e != nil {
			return e
		}
		out.Counts, e = countTx(ctx, tx, org, id)
		return e
	})
	return out, e
}

func validPage(cursor string, limit int) bool {
	return limit > 0 && limit <= 100 && (cursor == "" || auth.ValidID(cursor))
}

func (s *Service) List(ctx context.Context, session auth.Session, org, state, cursor string, limit int) (domain.Page[Campaign], error) {
	out := domain.Page[Campaign]{Items: []Campaign{}}
	if !validPage(cursor, limit) || len(state) > 32 {
		return out, auth.ErrInvalid
	}
	e := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		rows, e := tx.Query(ctx, `SELECT `+columns+` FROM campaigns c WHERE org_id=$1 AND ($2='' OR id>NULLIF($2,'')::uuid) AND ($3='' OR state=$3) AND NOT EXISTS(SELECT 1 FROM campaign_repository_scopes r WHERE r.org_id=c.org_id AND r.campaign_id=c.id AND NOT ($4 OR r.repository_id=ANY($5::uuid[]))) ORDER BY id LIMIT $6`, org, cursor, state, a.AllRepositories, a.RepositoryIDs, limit+1)
		if e != nil {
			return e
		}
		for rows.Next() {
			c, e := scan(rows)
			if e != nil {
				rows.Close()
				return e
			}
			out.Items = append(out.Items, c)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		out.Complete = len(out.Items) <= limit
		if !out.Complete {
			out.Items = out.Items[:limit]
			out.NextCursor = out.Items[limit-1].ID
		}
		for i := range out.Items {
			out.Items[i].Counts, e = countTx(ctx, tx, org, out.Items[i].ID)
			if e != nil {
				return e
			}
		}
		return nil
	})
	return out, e
}

const memberColumns = `document,state,reason,stage,coalesce(action_id::text,''),succeeded_at,halt,resume_requested,updated_at`

func scanMember(row pgx.Row) (Member, error) {
	var m Member
	var raw []byte
	var state, reason, action string
	var stage int
	var succeeded *time.Time
	var halt, resume bool
	var observed time.Time
	if e := row.Scan(&raw, &state, &reason, &stage, &action, &succeeded, &halt, &resume, &observed); e != nil {
		return m, e
	}
	if e := json.Unmarshal(raw, &m); e != nil {
		return m, e
	}
	m.Halt, m.ResumeRequested = halt, resume
	m.ObservedAt = observed
	m.State = state
	m.Reason = reason
	m.Stage = stage
	m.ActionID = action
	m.SucceededAt = succeeded
	return m, nil
}
func memberTx(ctx context.Context, tx pgx.Tx, org, campaign, id string) (Member, error) {
	return scanMember(tx.QueryRow(ctx, `SELECT `+memberColumns+` FROM campaign_members WHERE org_id=$1 AND campaign_id=$2 AND id=$3`, org, campaign, id))
}
func memberRows(ctx context.Context, tx pgx.Tx, org, id, cursor string, limit int) ([]Member, error) {
	out := []Member{}
	rows, e := tx.Query(ctx, `SELECT `+memberColumns+` FROM campaign_members WHERE org_id=$1 AND campaign_id=$2 AND ($3='' OR id>NULLIF($3,'')::uuid) ORDER BY id LIMIT $4`, org, id, cursor, limit)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		m, e := scanMember(rows)
		if e != nil {
			return out, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) Members(ctx context.Context, session auth.Session, org, id, cursor string, limit int) (domain.Page[Member], error) {
	out := domain.Page[Member]{Items: []Member{}}
	if !auth.ValidID(id) || !validPage(cursor, limit) {
		return out, auth.ErrInvalid
	}
	e := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if e := readable(ctx, tx, org, id, a); e != nil {
			return e
		}
		var e error
		out.Items, e = memberRows(ctx, tx, org, id, cursor, limit+1)
		if e != nil {
			return e
		}
		out.Complete = len(out.Items) <= limit
		if !out.Complete {
			out.Items = out.Items[:limit]
			out.NextCursor = out.Items[limit-1].ID
		}
		return nil
	})
	return out, e
}

func (s *Service) CheckScopeTx(ctx context.Context, tx pgx.Tx, org, kind, id string) error {
	if kind != "campaign" || !auth.ValidID(id) {
		return auth.ErrInvalid
	}
	var found bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM campaigns WHERE org_id=$1 AND id=$2)`, org, id).Scan(&found); e != nil {
		return e
	}
	if !found {
		return auth.ErrForbidden
	}
	return nil
}

func validReason(reason string) bool { return strings.TrimSpace(reason) != "" && len(reason) <= 1000 }
