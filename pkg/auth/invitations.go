package auth

import (
	"context"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
	"golang.org/x/oauth2"
)

type OrgOIDCInvitation struct {
	ID        string      `json:"id"`
	Email     string      `json:"email"`
	Role      domain.Role `json:"role"`
	ExpiresAt time.Time   `json:"expires_at"`
	CreatedAt time.Time   `json:"created_at"`
	Redeemed  bool        `json:"redeemed"`
}

type OrgOIDCInvitationInput struct {
	Email     string      `json:"email"`
	Role      domain.Role `json:"role"`
	ExpiresAt time.Time   `json:"expires_at"`
}

type CreatedOrgOIDCInvitation struct {
	OrgOIDCInvitation
	RedemptionURL string `json:"redemption_url"`
}

func normalizeInvitationEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if len(email) > 320 || strings.ContainsAny(email, "\r\n\x00") {
		return "", false
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || strings.Count(email, "@") != 1 {
		return "", false
	}
	return email, true
}

func (s *Service) CreateOrgOIDCInvitation(ctx context.Context, session Session, orgID string, input OrgOIDCInvitationInput, requestID string) (CreatedOrgOIDCInvitation, error) {
	email, ok := normalizeInvitationEmail(input.Email)
	now := time.Now().UTC()
	if !ValidID(orgID) || !ok || !validRole(input.Role) || input.ExpiresAt.Before(now.Add(time.Hour)) || input.ExpiresAt.After(now.Add(30*24*time.Hour)) {
		return CreatedOrgOIDCInvitation{}, ErrInvalid
	}
	id := string(domain.NewID())
	token := randomToken()
	created := CreatedOrgOIDCInvitation{}
	err := s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		var configID, issuer string
		var configVersion int64
		if err := tx.QueryRow(ctx, "SELECT id::text,issuer,version FROM org_oidc_configs WHERE org_id=$1 AND status='active' AND verified_version=version", orgID).Scan(&configID, &issuer, &configVersion); err != nil {
			return ErrOIDCActivation
		}
		err := tx.QueryRow(ctx, "INSERT INTO org_oidc_invitations(org_id,id,email,role,oidc_config_id,oidc_config_version,issuer,token_hash,created_by,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING created_at", orgID, id, email, input.Role, configID, configVersion, issuer, digest(token), actor.UserID, input.ExpiresAt.UTC()).Scan(&created.CreatedAt)
		if err != nil {
			return err
		}
		if err := audit(ctx, tx, orgID, string(actor.UserID), "org.oidc.invitation.created", id, requestID, 1); err != nil {
			return err
		}
		created.OrgOIDCInvitation = OrgOIDCInvitation{ID: id, Email: email, Role: input.Role, ExpiresAt: input.ExpiresAt.UTC(), CreatedAt: created.CreatedAt}
		return nil
	})
	if err != nil {
		return CreatedOrgOIDCInvitation{}, err
	}
	created.RedemptionURL = s.cfg.PublicURL + "/invite#token=" + url.QueryEscape(token)
	return created, nil
}

func (s *Service) OrgOIDCInvitations(ctx context.Context, session Session, orgID string, limit int, cursor string) (domain.Page[OrgOIDCInvitation], error) {
	page := domain.Page[OrgOIDCInvitation]{Items: []OrgOIDCInvitation{}}
	if limit < 1 || limit > 200 || (cursor != "" && !ValidID(cursor)) {
		return page, ErrInvalid
	}
	err := s.WithActor(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT id::text,email,role,expires_at,created_at,redeemed_at IS NOT NULL FROM org_oidc_invitations WHERE org_id=$1 AND id::text>$2 ORDER BY id LIMIT $3`, orgID, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item OrgOIDCInvitation
			if err := rows.Scan(&item.ID, &item.Email, &item.Role, &item.ExpiresAt, &item.CreatedAt, &item.Redeemed); err != nil {
				return err
			}
			page.Items = append(page.Items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		page.Complete = len(page.Items) <= limit
		if !page.Complete {
			page.Items = page.Items[:limit]
			page.NextCursor = page.Items[limit-1].ID
		}
		return nil
	})
	return page, err
}

func (s *Service) RevokeOrgOIDCInvitation(ctx context.Context, session Session, orgID, invitationID, requestID string) error {
	if !ValidID(orgID) || !ValidID(invitationID) {
		return ErrInvalid
	}
	return s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		tag, err := tx.Exec(ctx, `DELETE FROM org_oidc_invitations WHERE org_id=$1 AND id=$2 AND redeemed_at IS NULL`, orgID, invitationID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrForbidden
		}
		return audit(ctx, tx, orgID, string(actor.UserID), "org.oidc.invitation.revoked", invitationID, requestID, 1)
	})
}

func (s *Service) CheckInvitationRequest(r *http.Request) error {
	if !strings.EqualFold(r.Host, s.origin.Host) || r.Header.Get("Origin") != s.cfg.PublicURL || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return ErrForbidden
	}
	return nil
}

func (s *Service) BeginOrgOIDCInvitation(ctx context.Context, w http.ResponseWriter, token string) (string, error) {
	if len(token) != 43 || s.orgOIDCVault == nil {
		return "", ErrUnauthenticated
	}
	if !acquireOrgOIDCProbe() {
		return "", ErrOIDCProbeBusy
	}
	defer releaseOrgOIDCProbe()
	state, browser, nonce, verifier := randomToken(), randomToken(), randomToken(), oauth2.GenerateVerifier()
	var attempt oidcLoginAttempt
	err := s.identity(ctx, "", map[string]string{
		"reforge.login_hash":      digest(state),
		"reforge.invitation_hash": digest(token),
	}, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT org_id::text,id::text,oidc_config_id::text,oidc_config_version,issuer FROM org_oidc_invitations WHERE token_hash=$1 AND redeemed_at IS NULL AND expires_at>now()", digest(token)).Scan(&attempt.OrgID, &attempt.InvitationID, &attempt.ConfigID, &attempt.ConfigVersion, &attempt.Issuer); err != nil {
			return ErrUnauthenticated
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('reforge.login_org_id',$1,true)", attempt.OrgID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "SELECT client_id FROM org_oidc_configs WHERE org_id=$1 AND id=$2 AND version=$3 AND issuer=$4 AND status='active' AND verified_version=version", attempt.OrgID, attempt.ConfigID, attempt.ConfigVersion, attempt.Issuer).Scan(&attempt.ClientID); err != nil {
			return ErrUnauthenticated
		}
		attempt.InvitationHash = digest(token)
		_, err := tx.Exec(ctx, "INSERT INTO oidc_logins(state_hash,browser_hash,nonce,verifier,expires_at,org_id,config_id,config_version,issuer,client_id,invitation_org_id,invitation_id,invitation_hash) VALUES($1,$2,$3,$4,now()+interval '10 minutes',$5,$6,$7,$8,$9,$5,$10,$11)", digest(state), digest(browser), nonce, verifier, attempt.OrgID, attempt.ConfigID, attempt.ConfigVersion, attempt.Issuer, attempt.ClientID, attempt.InvitationID, attempt.InvitationHash)
		return err
	})
	if err != nil {
		return "", ErrUnauthenticated
	}
	runtime, err := s.newOrgOIDCProvider(ctx, attempt.Issuer, attempt.ClientID)
	if err != nil {
		return "", ErrUnauthenticated
	}
	defer runtime.client.CloseIdleConnections()
	http.SetCookie(w, s.cookie(s.oidcCookieName(), browser, 600))
	return runtime.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

func (s *Service) completeOrgOIDCInvitation(ctx context.Context, attempt oidcLoginAttempt, subject, rawEmail, name string) (string, error) {
	email, ok := normalizeInvitationEmail(rawEmail)
	if !ok || !ValidID(attempt.OrgID) || !ValidID(attempt.ConfigID) || !ValidID(attempt.InvitationID) || len(attempt.InvitationHash) != 64 || subject == "" || attempt.ConfigVersion < 1 {
		return "", ErrUnauthenticated
	}
	token, csrf := randomToken(), randomToken()
	var userID string
	err := s.identity(ctx, "", map[string]string{
		"reforge.invitation_hash":      attempt.InvitationHash,
		"reforge.login_org_id":         attempt.OrgID,
		"reforge.login_config_id":      attempt.ConfigID,
		"reforge.login_config_version": formatOIDCVersion(attempt.ConfigVersion),
		"reforge.issuer":               attempt.Issuer,
		"reforge.subject":              subject,
		"reforge.org_id":               attempt.OrgID,
	}, func(tx pgx.Tx) error {
		var orgID string
		if err := tx.QueryRow(ctx, "SELECT id::text FROM organisations WHERE id=$1 FOR SHARE", attempt.OrgID).Scan(&orgID); err != nil {
			return ErrUnauthenticated
		}
		var inviteEmail, invitationIssuer, invitationConfigID string
		var role domain.Role
		var invitationConfigVersion int64
		if err := tx.QueryRow(ctx, "SELECT email,role,issuer,oidc_config_id::text,oidc_config_version FROM org_oidc_invitations WHERE org_id=$1 AND id=$2 AND token_hash=$3 AND redeemed_at IS NULL AND expires_at>now() FOR UPDATE", attempt.OrgID, attempt.InvitationID, attempt.InvitationHash).Scan(&inviteEmail, &role, &invitationIssuer, &invitationConfigID, &invitationConfigVersion); err != nil || inviteEmail != email || invitationIssuer != attempt.Issuer || invitationConfigID != attempt.ConfigID || invitationConfigVersion != attempt.ConfigVersion {
			return ErrUnauthenticated
		}
		var active bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM org_oidc_configs WHERE org_id=$1 AND id=$2 AND version=$3 AND issuer=$4 AND status='active' AND verified_version=version)", attempt.OrgID, attempt.ConfigID, attempt.ConfigVersion, attempt.Issuer).Scan(&active); err != nil || !active {
			return ErrUnauthenticated
		}
		displayName := strings.TrimSpace(name)
		if displayName == "" || len(displayName) > 256 {
			displayName = email
		}
		_, err := tx.Exec(ctx, "INSERT INTO users(id,issuer,subject,name,email) VALUES($1,$2,$3,$4,$5) ON CONFLICT(issuer,subject) DO NOTHING", domain.NewID(), attempt.Issuer, subject, displayName, email)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "SELECT id::text FROM users WHERE issuer=$1 AND subject=$2", attempt.Issuer, subject).Scan(&userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('reforge.user_id',$1,true)", userID); err != nil {
			return err
		}
		allRepositories := role == domain.Owner
		tag, err := tx.Exec(ctx, "INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,$3,$4) ON CONFLICT(org_id,user_id) DO NOTHING", attempt.OrgID, userID, role, allRepositories)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		if _, err := tx.Exec(ctx, "INSERT INTO sessions(id,user_id,org_id,oidc_config_id,oidc_config_version,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,now()+interval '12 hours')", domain.NewID(), userID, attempt.OrgID, attempt.ConfigID, attempt.ConfigVersion, digest(token), csrf); err != nil {
			return err
		}
		tag, err = tx.Exec(ctx, "UPDATE org_oidc_invitations SET redeemed_at=now() WHERE org_id=$1 AND id=$2 AND token_hash=$3 AND redeemed_at IS NULL AND expires_at>now()", attempt.OrgID, attempt.InvitationID, attempt.InvitationHash)
		if err != nil || tag.RowsAffected() != 1 {
			return ErrUnauthenticated
		}
		return audit(ctx, tx, attempt.OrgID, "org-oidc-invitation", "org.oidc.invitation.redeemed", userID, "oidc-invite:"+attempt.InvitationID, 1)
	})
	if err != nil {
		return "", err
	}
	return token, nil
}
