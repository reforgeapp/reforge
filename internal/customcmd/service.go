package customcmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/store"
)

type Service struct {
	db   *store.Store
	auth *auth.Service
}

func New(db *store.Store, identity *auth.Service) *Service { return &Service{db: db, auth: identity} }

const profileColumns = `id::text,name,version,image_digest,executable,argv,protocol_version,max_wall_seconds,max_output_bytes,max_turns,concurrency,approval_evidence,coalesce(approved_by::text,''),approved_at,revoked_at,created_at`

func scanProfile(row pgx.Row) (Profile, error) {
	var p Profile
	var raw []byte
	if e := row.Scan(&p.ID, &p.Name, &p.Version, &p.ImageDigest, &p.Executable, &raw, &p.ProtocolVersion, &p.MaxWallSeconds, &p.MaxOutputBytes, &p.MaxTurns, &p.Concurrency, &p.ApprovalEvidence, &p.ApprovedBy, &p.ApprovedAt, &p.RevokedAt, &p.CreatedAt); e != nil {
		return p, e
	}
	if e := json.Unmarshal(raw, &p.Argv); e != nil {
		return p, e
	}
	return p, nil
}

func manage(a domain.Actor) bool { return a.Role == domain.Owner || a.Role == domain.Admin }

func audit(ctx context.Context, tx pgx.Tx, org, actor, action, id, request string, data any) error {
	raw, e := json.Marshal(data)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, domain.NewID(), org, actor, action, id, request, raw)
	return e
}

func validPage(cursor string, limit int) bool {
	return limit > 0 && limit <= 100 && (cursor == "" || auth.ValidID(cursor))
}

func (s *Service) Create(ctx context.Context, session auth.Session, org string, in Profile, request string) (Profile, error) {
	var out Profile
	if session.AutomationID() != "" {
		return out, auth.ErrForbidden
	}
	if err := Validate(in); err != nil {
		return out, auth.ErrInvalid
	}
	e := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		raw, err := json.Marshal(in.Argv)
		if err != nil {
			return err
		}
		id := domain.NewID()
		if _, err = tx.Exec(ctx, `INSERT INTO custom_profiles(org_id,id,name,image_digest,executable,argv,protocol_version,max_wall_seconds,max_output_bytes,max_turns,concurrency,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, org, id, strings.TrimSpace(in.Name), in.ImageDigest, in.Executable, raw, in.ProtocolVersion, in.MaxWallSeconds, in.MaxOutputBytes, in.MaxTurns, in.Concurrency, a.UserID); err != nil {
			return err
		}
		out, err = scanProfile(tx.QueryRow(ctx, `SELECT `+profileColumns+` FROM custom_profiles WHERE org_id=$1 AND id=$2`, org, id))
		if err != nil {
			return err
		}
		return audit(ctx, tx, org, a.UserID, "custom_profile.created", id, request, map[string]any{"image_digest": out.ImageDigest, "protocol_version": out.ProtocolVersion})
	})
	return out, e
}

func (s *Service) List(ctx context.Context, session auth.Session, org, cursor string, limit int) (domain.Page[Profile], error) {
	out := domain.Page[Profile]{Items: []Profile{}}
	if !validPage(cursor, limit) {
		return out, auth.ErrInvalid
	}
	e := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		rows, e := tx.Query(ctx, `SELECT `+profileColumns+` FROM custom_profiles WHERE org_id=$1 AND ($2='' OR id>NULLIF($2,'')::uuid) ORDER BY id LIMIT $3`, org, cursor, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			p, e := scanProfile(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, p)
		}
		if e = rows.Err(); e != nil {
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

func (s *Service) Get(ctx context.Context, session auth.Session, org, id string) (Profile, error) {
	var out Profile
	if !auth.ValidID(id) {
		return out, auth.ErrInvalid
	}
	e := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var e error
		out, e = scanProfile(tx.QueryRow(ctx, `SELECT `+profileColumns+` FROM custom_profiles WHERE org_id=$1 AND id=$2`, org, id))
		if errors.Is(e, pgx.ErrNoRows) {
			return auth.ErrForbidden
		}
		return e
	})
	return out, e
}

func (s *Service) Approve(ctx context.Context, session auth.Session, org, id, evidence string, expected int64, request string) (Profile, error) {
	var out Profile
	if session.AutomationID() != "" {
		return out, auth.ErrForbidden
	}
	evidence = strings.TrimSpace(evidence)
	if !auth.ValidID(id) || expected < 1 || evidence == "" || len(evidence) > 1000 {
		return out, auth.ErrInvalid
	}
	e := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		current, err := scanProfile(tx.QueryRow(ctx, `SELECT `+profileColumns+` FROM custom_profiles WHERE org_id=$1 AND id=$2 FOR UPDATE`, org, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrForbidden
		}
		if err != nil {
			return err
		}
		if current.Version != expected {
			return auth.ErrConflict
		}
		if current.RevokedAt != nil {
			return auth.ErrConflict
		}
		if err = Validate(current); err != nil {
			return auth.ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE custom_profiles SET approved_by=$3,approved_at=now(),approval_evidence=$4,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, a.UserID, evidence); err != nil {
			return err
		}
		out, err = scanProfile(tx.QueryRow(ctx, `SELECT `+profileColumns+` FROM custom_profiles WHERE org_id=$1 AND id=$2`, org, id))
		if err != nil {
			return err
		}
		return audit(ctx, tx, org, a.UserID, "custom_profile.approved", id, request, map[string]any{"evidence": evidence, "image_digest": out.ImageDigest, "version": out.Version})
	})
	return out, e
}

func (s *Service) Revoke(ctx context.Context, session auth.Session, org, id string, expected int64, request string) (Profile, error) {
	var out Profile
	if session.AutomationID() != "" {
		return out, auth.ErrForbidden
	}
	if !auth.ValidID(id) || expected < 1 {
		return out, auth.ErrInvalid
	}
	e := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		current, err := scanProfile(tx.QueryRow(ctx, `SELECT `+profileColumns+` FROM custom_profiles WHERE org_id=$1 AND id=$2 FOR UPDATE`, org, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrForbidden
		}
		if err != nil {
			return err
		}
		if current.Version != expected {
			return auth.ErrConflict
		}
		if current.RevokedAt != nil {
			return nil
		}
		if _, err = tx.Exec(ctx, `UPDATE custom_profiles SET revoked_at=now(),version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, id); err != nil {
			return err
		}
		out, err = scanProfile(tx.QueryRow(ctx, `SELECT `+profileColumns+` FROM custom_profiles WHERE org_id=$1 AND id=$2`, org, id))
		if err != nil {
			return err
		}
		return audit(ctx, tx, org, a.UserID, "custom_profile.revoked", id, request, map[string]any{"version": out.Version})
	})
	return out, e
}

func (s *Service) Bind(ctx context.Context, tx pgx.Tx, org, id string, version int64) (Profile, error) {
	p, e := scanProfile(tx.QueryRow(ctx, `SELECT `+profileColumns+` FROM custom_profiles WHERE org_id=$1 AND id=$2`, org, id))
	if e != nil {
		return p, e
	}
	if p.Version != version || !Approved(p, time.Now()) {
		return p, ErrNotApproved
	}
	return p, nil
}
