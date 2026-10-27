package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/store"
)

func CreatePlatformInvitation(ctx context.Context, db *store.Store, orgName, email string) (string, error) {
	name := strings.TrimSpace(orgName)
	address, ok := normalizeInvitationEmail(email)
	if !ok || name == "" || len(name) > 160 {
		return "", ErrInvalid
	}
	token := randomToken()
	err := db.Identity(ctx, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.platform_invitation_hash',$1,true)`, digest(token)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO platform_invitations(id,token_hash,email,org_name,expires_at) VALUES($1,$2,$3,$4,now()+interval '7 days')`, domain.NewID(), digest(token), address, name)
		return err
	})
	return token, err
}

func (s *Service) RedeemPlatformInvitation(ctx context.Context, session Session, token, requestID string) (domain.Organisation, error) {
	org := domain.Organisation{ID: domain.NewID(), Version: 1}
	if s.cfg.Edition != "hosted" || len(token) != 43 {
		return org, ErrForbidden
	}
	err := s.db.Tenant(ctx, org.ID, session.User.ID, func(tx pgx.Tx) error {
		if err := lockSession(ctx, tx, session); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.platform_invitation_hash',$1,true)`, digest(token)); err != nil {
			return err
		}
		var id string
		err := tx.QueryRow(ctx, `SELECT id::text,org_name FROM platform_invitations WHERE token_hash=$1 AND redeemed_at IS NULL AND expires_at>now() FOR UPDATE`, digest(token)).Scan(&id, &org.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrForbidden
		}
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,$2)`, org.ID, org.Name); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org.ID, session.User.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE platform_invitations SET redeemed_at=now(),redeemed_by=$2,org_id=$3 WHERE id=$1`, id, session.User.ID, org.ID); err != nil {
			return err
		}
		return audit(ctx, tx, org.ID, session.User.ID, "organisation.invitation_redeemed", org.ID, requestID, 1)
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
