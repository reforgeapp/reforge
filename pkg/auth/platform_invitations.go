package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/store"
)

var ErrInvitationAccepted = fmt.Errorf("invitation already accepted: %w", ErrConflict)
var ErrSlugTaken = fmt.Errorf("short name already in use: %w", ErrConflict)

type PendingInvitation struct {
	ID        string      `json:"id"`
	Email     string      `json:"email"`
	OrgName   string      `json:"org_name"`
	Role      domain.Role `json:"role"`
	ExpiresAt time.Time   `json:"expires_at"`
}

type Tenant struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	Owners       []string  `json:"owners"`
	Members      int       `json:"members"`
	Repositories int       `json:"repositories"`
}

func platformAdmin(ctx context.Context, staff *store.Store, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, staff.Pool, fn)
}

func CreatePlatformInvitation(ctx context.Context, db *store.Store, orgName, email, rawSlug string) (string, error) {
	name := strings.TrimSpace(orgName)
	address, ok := normalizeInvitationEmail(email)
	slug, slugOK := NormaliseSlug(rawSlug)
	if !ok || name == "" || len(name) > 160 || rawSlug != "" && !slugOK {
		return "", ErrInvalid
	}
	token := randomToken()
	err := platformAdmin(ctx, db, func(tx pgx.Tx) error {
		var accepted bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_invitations WHERE target_org_id IS NULL AND email=$1 AND lower(org_name)=lower($2) AND redeemed_at IS NOT NULL)`, address, name).Scan(&accepted); err != nil {
			return err
		}
		if accepted {
			return ErrInvitationAccepted
		}
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organisations WHERE slug=$1)`, slug).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return ErrSlugTaken
		}
		tag, err := tx.Exec(ctx, `UPDATE platform_invitations SET token_hash=$3,slug=nullif($4,''),expires_at=now()+interval '7 days' WHERE target_org_id IS NULL AND email=$1 AND lower(org_name)=lower($2) AND redeemed_at IS NULL AND revoked_at IS NULL`, address, name, digest(token), slug)
		if err != nil || tag.RowsAffected() > 0 {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO platform_invitations(id,token_hash,email,org_name,slug,expires_at) VALUES($1,$2,$3,$4,nullif($5,''),now()+interval '7 days')`, domain.NewID(), digest(token), address, name, slug)
		return err
	})
	return token, err
}

func PlatformInvitations(ctx context.Context, db *store.Store) ([]PendingInvitation, error) {
	var out []PendingInvitation
	err := platformAdmin(ctx, db, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text,email,org_name,role,expires_at FROM platform_invitations WHERE target_org_id IS NULL AND redeemed_at IS NULL AND revoked_at IS NULL AND expires_at>now() ORDER BY created_at DESC LIMIT 200`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[PendingInvitation])
		return err
	})
	return out, err
}

func RevokePlatformInvitation(ctx context.Context, db *store.Store, id string) error {
	if !ValidID(id) {
		return ErrInvalid
	}
	return platformAdmin(ctx, db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE platform_invitations SET revoked_at=now() WHERE id=$1 AND target_org_id IS NULL AND redeemed_at IS NULL`, id)
		return err
	})
}

func PlatformTenants(ctx context.Context, db *store.Store) ([]Tenant, error) {
	var out []Tenant
	err := platformAdmin(ctx, db, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT o.id::text,o.name,o.created_at,
			coalesce(array(SELECT u.email FROM memberships m JOIN users u ON u.id=m.user_id WHERE m.org_id=o.id AND m.role='owner' ORDER BY u.email),'{}'),
			(SELECT count(*) FROM memberships m WHERE m.org_id=o.id),
			(SELECT count(*) FROM repositories r WHERE r.org_id=o.id)
			FROM organisations o ORDER BY o.created_at DESC LIMIT 500`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Tenant])
		return err
	})
	return out, err
}

func (s *Service) CreateMemberInvitation(ctx context.Context, session Session, orgID, email string, role domain.Role, requestID string) (string, string, error) {
	address, ok := normalizeInvitationEmail(email)
	if !ok || !ValidID(orgID) || role == "" {
		return "", "", ErrInvalid
	}
	token := randomToken()
	var orgName string
	err := s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		if err := tx.QueryRow(ctx, `SELECT name FROM organisations WHERE id=$1`, orgID).Scan(&orgName); err != nil {
			return err
		}
		var id string
		err := tx.QueryRow(ctx, `UPDATE platform_invitations SET token_hash=$3,role=$4,invited_by=$5,expires_at=now()+interval '7 days' WHERE target_org_id=$1 AND email=$2 AND redeemed_at IS NULL AND revoked_at IS NULL RETURNING id::text`, orgID, address, digest(token), role, session.User.ID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			id = domain.NewID()
			_, err = tx.Exec(ctx, `INSERT INTO platform_invitations(id,token_hash,email,org_name,target_org_id,role,invited_by,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,now()+interval '7 days')`, id, digest(token), address, orgName, orgID, role, session.User.ID)
		}
		if err != nil {
			return err
		}
		return audit(ctx, tx, orgID, session.User.ID, "membership.invited", id, requestID, 1)
	})
	if err != nil {
		return "", "", err
	}
	return token, orgName, nil
}

func (s *Service) MemberInvitations(ctx context.Context, session Session, orgID string) ([]PendingInvitation, error) {
	out := []PendingInvitation{}
	err := s.WithActor(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT id::text,email,org_name,role,expires_at FROM platform_invitations WHERE target_org_id=$1 AND redeemed_at IS NULL AND revoked_at IS NULL AND expires_at>now() ORDER BY created_at DESC LIMIT 200`, orgID)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[PendingInvitation])
		return err
	})
	return out, err
}

func (s *Service) RevokeMemberInvitation(ctx context.Context, session Session, orgID, id, requestID string) error {
	if !ValidID(orgID) || !ValidID(id) {
		return ErrInvalid
	}
	return s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		tag, err := tx.Exec(ctx, `UPDATE platform_invitations SET revoked_at=now() WHERE id=$1 AND target_org_id=$2 AND redeemed_at IS NULL AND revoked_at IS NULL`, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		return audit(ctx, tx, orgID, session.User.ID, "membership.invitation_revoked", id, requestID, 1)
	})
}

func (s *Service) RedeemPlatformInvitation(ctx context.Context, session Session, token, requestID string) (domain.Organisation, error) {
	org := domain.Organisation{Version: 1}
	if len(token) != 43 {
		return org, ErrForbidden
	}
	var id, target, slug string
	var role domain.Role
	err := s.db.Identity(ctx, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.platform_invitation_hash',$1,true)`, digest(token)); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id::text,org_name,coalesce(target_org_id::text,''),role,coalesce(slug,'') FROM platform_invitations WHERE token_hash=$1 AND redeemed_at IS NULL AND revoked_at IS NULL AND expires_at>now()`, digest(token)).Scan(&id, &org.Name, &target, &role, &slug)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return org, ErrForbidden
	}
	if err != nil {
		return org, err
	}
	if target == "" && s.cfg.Edition != "hosted" {
		return org, ErrForbidden
	}
	org.ID = target
	if org.ID == "" {
		org.ID = domain.NewID()
	}
	err = s.db.Tenant(ctx, org.ID, session.User.ID, func(tx pgx.Tx) error {
		if err := lockSession(ctx, tx, session); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.platform_invitation_hash',$1,true)`, digest(token)); err != nil {
			return err
		}
		action := "membership.invitation_redeemed"
		if target == "" {
			action = "organisation.invitation_redeemed"
			tag, err := tx.Exec(ctx, `INSERT INTO organisations(id,name,slug) VALUES($1,$2,nullif($3,'')) ON CONFLICT (slug) DO NOTHING`, org.ID, org.Name, slug)
			if err == nil && tag.RowsAffected() == 0 {
				_, err = tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,$2)`, org.ID, org.Name)
			}
			if err != nil {
				return err
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE platform_invitations SET redeemed_at=now(),redeemed_by=$2,org_id=$3 WHERE id=$1 AND token_hash=$4 AND redeemed_at IS NULL AND revoked_at IS NULL AND expires_at>now()`, id, session.User.ID, org.ID, digest(token))
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrForbidden
		}
		if _, err = tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,$3,true) ON CONFLICT DO NOTHING`, org.ID, session.User.ID, role); err != nil {
			return err
		}
		return audit(ctx, tx, org.ID, session.User.ID, action, org.ID, requestID, 1)
	})
	return org, err
}

func (s *Service) joinCookieName() string {
	if s.origin.Scheme == "https" {
		return "__Host-reforge_join"
	}
	return "reforge_development_join"
}

func (s *Service) SetJoin(w http.ResponseWriter, token string) {
	http.SetCookie(w, s.cookie(s.joinCookieName(), token, 1800))
}

func (s *Service) Join(r *http.Request) string {
	c, err := r.Cookie(s.joinCookieName())
	if err != nil || len(c.Value) != 43 {
		return ""
	}
	return c.Value
}

func (s *Service) ClearJoin(w http.ResponseWriter) {
	http.SetCookie(w, s.cookie(s.joinCookieName(), "", -1))
}
