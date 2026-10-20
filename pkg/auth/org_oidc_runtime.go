package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/secrets"
	"golang.org/x/oauth2"
)

type oidcLoginAttempt struct {
	Nonce          string
	Verifier       string
	OrgID          string
	ConfigID       string
	ConfigVersion  int64
	Issuer         string
	ClientID       string
	InvitationID   string
	InvitationHash string
	SecretVersion  int64
	SecretEnvelope []byte
}

type orgOIDCProvider struct {
	provider *oidc.Provider
	client   *http.Client
	oauth    oauth2.Config
}

func (s *Service) beginOrgOIDCLogin(ctx context.Context, w http.ResponseWriter, orgID string) (string, error) {
	if !ValidID(orgID) {
		return "", ErrInvalid
	}
	if s.orgOIDCVault == nil {
		return "", ErrOIDCUnavailable
	}
	if !acquireOrgOIDCProbe() {
		return "", ErrOIDCProbeBusy
	}
	defer releaseOrgOIDCProbe()
	config, err := s.orgOIDCLoginConfig(ctx, orgID)
	if err != nil {
		return "", ErrUnauthenticated
	}
	runtime, err := s.newOrgOIDCProvider(ctx, config.Issuer, config.ClientID)
	if err != nil {
		return "", ErrUnauthenticated
	}
	defer runtime.client.CloseIdleConnections()
	state, browser, nonce, verifier := randomToken(), randomToken(), randomToken(), oauth2.GenerateVerifier()
	err = s.identity(ctx, "", map[string]string{
		"reforge.login_hash":   digest(state),
		"reforge.login_org_id": orgID,
	}, func(tx pgx.Tx) error {
		var current string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM org_oidc_configs WHERE org_id=$1 AND id=$2 AND version=$3 AND issuer=$4 AND client_id=$5 AND status='active' AND verified_version=version`, orgID, config.ID, config.Version, config.Issuer, config.ClientID).Scan(&current); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO oidc_logins(state_hash,browser_hash,nonce,verifier,expires_at,org_id,config_id,config_version,issuer,client_id) VALUES($1,$2,$3,$4,now()+interval '10 minutes',$5,$6,$7,$8,$9)`, digest(state), digest(browser), nonce, verifier, orgID, config.ID, config.Version, config.Issuer, config.ClientID)
		return err
	})
	if err != nil {
		return "", err
	}
	http.SetCookie(w, s.cookie(s.oidcCookieName(), browser, 600))
	return runtime.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

func (s *Service) orgOIDCLoginConfig(ctx context.Context, orgID string) (orgOIDCRecord, error) {
	var record orgOIDCRecord
	err := s.identity(ctx, "", map[string]string{"reforge.login_org_id": orgID}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text,issuer,client_id,version FROM org_oidc_configs WHERE org_id=$1 AND status='active' AND verified_version=version`, orgID).Scan(&record.ID, &record.Issuer, &record.ClientID, &record.Version)
	})
	return record, err
}

func (s *Service) consumeOIDCLogin(ctx context.Context, state, browser string) (oidcLoginAttempt, error) {
	var attempt oidcLoginAttempt
	err := s.identity(ctx, "", map[string]string{"reforge.login_hash": digest(state)}, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `DELETE FROM oidc_logins WHERE state_hash=$1 AND browser_hash=$2 AND expires_at>now() RETURNING nonce,verifier,coalesce(org_id::text,''),coalesce(config_id::text,''),coalesce(config_version,0),coalesce(issuer,''),coalesce(client_id,''),coalesce(invitation_id::text,''),coalesce(invitation_hash,'')`, digest(state), digest(browser)).Scan(&attempt.Nonce, &attempt.Verifier, &attempt.OrgID, &attempt.ConfigID, &attempt.ConfigVersion, &attempt.Issuer, &attempt.ClientID, &attempt.InvitationID, &attempt.InvitationHash)
		if err != nil || attempt.OrgID == "" {
			return err
		}
		if !ValidID(attempt.OrgID) || !ValidID(attempt.ConfigID) || attempt.ConfigVersion < 1 || attempt.Issuer == "" || attempt.ClientID == "" {
			return ErrUnauthenticated
		}
		for key, value := range map[string]string{
			"reforge.login_org_id":         attempt.OrgID,
			"reforge.login_config_id":      attempt.ConfigID,
			"reforge.login_config_version": formatOIDCVersion(attempt.ConfigVersion),
		} {
			if _, err := tx.Exec(ctx, `SELECT set_config($1,$2,true)`, key, value); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx, `SELECT c.issuer,c.client_id,s.version,s.envelope FROM org_oidc_configs c JOIN org_oidc_secrets s ON s.org_id=c.org_id AND s.config_id=c.id WHERE c.org_id=$1 AND c.id=$2 AND c.version=$3 AND c.issuer=$4 AND c.client_id=$5 AND c.status='active' AND c.verified_version=c.version AND s.version=$3`, attempt.OrgID, attempt.ConfigID, attempt.ConfigVersion, attempt.Issuer, attempt.ClientID).Scan(&attempt.Issuer, &attempt.ClientID, &attempt.SecretVersion, &attempt.SecretEnvelope)
	})
	return attempt, err
}

func formatOIDCVersion(version int64) string { return strconv.FormatInt(version, 10) }

func (s *Service) newOrgOIDCProvider(ctx context.Context, issuer, clientID string) (orgOIDCProvider, error) {
	metadata, err := fetchOIDCMetadata(ctx, issuer, s.cfg.Development)
	if err != nil {
		return orgOIDCProvider{}, err
	}
	parsed, err := url.Parse(issuer)
	if err != nil {
		return orgOIDCProvider{}, ErrInvalid
	}
	client, err := issuerHTTPClient(ctx, parsed, s.cfg.Development)
	if err != nil {
		return orgOIDCProvider{}, err
	}
	provider := (&oidc.ProviderConfig{
		IssuerURL: issuer,
		AuthURL:   metadata.AuthorizationEndpoint,
		TokenURL:  metadata.TokenEndpoint,
		JWKSURL:   metadata.JWKSURI,
	}).NewProvider(oidc.ClientContext(ctx, client))
	return orgOIDCProvider{
		provider: provider,
		client:   client,
		oauth: oauth2.Config{
			ClientID:    clientID,
			Endpoint:    provider.Endpoint(),
			RedirectURL: s.cfg.PublicURL + "/auth/callback",
			Scopes:      []string{oidc.ScopeOpenID, "profile", "email"},
		},
	}, nil
}

func (s *Service) completeOrgOIDCCallback(ctx context.Context, w http.ResponseWriter, attempt oidcLoginAttempt, code string) error {
	if !acquireOrgOIDCProbe() {
		return ErrUnauthenticated
	}
	defer releaseOrgOIDCProbe()
	if attempt.SecretVersion != attempt.ConfigVersion || s.orgOIDCVault == nil {
		return ErrUnauthenticated
	}
	var envelope secrets.Envelope
	if json.Unmarshal(attempt.SecretEnvelope, &envelope) != nil {
		return ErrUnauthenticated
	}
	secret, err := s.orgOIDCVault.OpenContext(ctx, secrets.Binding{OrgID: attempt.OrgID, ConnectionID: attempt.ConfigID, Version: attempt.SecretVersion}, envelope)
	if err != nil {
		return ErrUnauthenticated
	}
	defer clear(secret)
	runtime, err := s.newOrgOIDCProvider(ctx, attempt.Issuer, attempt.ClientID)
	if err != nil {
		return ErrUnauthenticated
	}
	defer runtime.client.CloseIdleConnections()
	runtime.oauth.ClientSecret = string(secret)
	token, err := runtime.oauth.Exchange(context.WithValue(ctx, oauth2.HTTPClient, runtime.client), code, oauth2.VerifierOption(attempt.Verifier))
	runtime.oauth.ClientSecret = ""
	if err != nil {
		return ErrUnauthenticated
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return ErrUnauthenticated
	}
	verifier := runtime.provider.Verifier(&oidc.Config{ClientID: attempt.ClientID})
	id, err := verifier.Verify(oidc.ClientContext(ctx, runtime.client), raw)
	if err != nil || id.Issuer != attempt.Issuer || !equal(id.Nonce, attempt.Nonce) || id.Subject == "" {
		return ErrUnauthenticated
	}
	if attempt.InvitationID != "" {
		var claims struct {
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Name          string `json:"name"`
		}
		if id.Claims(&claims) != nil || !claims.EmailVerified {
			return ErrUnauthenticated
		}
		session, err := s.completeOrgOIDCInvitation(ctx, attempt, id.Subject, claims.Email, claims.Name)
		if err != nil {
			return ErrUnauthenticated
		}
		s.SetSession(w, session)
		return nil
	}
	session, err := s.createOrgOIDCSession(ctx, attempt, id.Subject)
	if err != nil {
		return ErrUnauthenticated
	}
	s.SetSession(w, session)
	return nil
}

func (s *Service) createOrgOIDCSession(ctx context.Context, attempt oidcLoginAttempt, subject string) (string, error) {
	if !ValidID(attempt.OrgID) || !ValidID(attempt.ConfigID) || subject == "" {
		return "", ErrUnauthenticated
	}
	token, csrf := randomToken(), randomToken()
	var userID string
	err := s.identity(ctx, "", map[string]string{
		"reforge.issuer":               attempt.Issuer,
		"reforge.subject":              subject,
		"reforge.login_org_id":         attempt.OrgID,
		"reforge.login_config_id":      attempt.ConfigID,
		"reforge.login_config_version": formatOIDCVersion(attempt.ConfigVersion),
	}, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE issuer=$1 AND subject=$2`, attempt.Issuer, subject).Scan(&userID); err != nil {
			return ErrUnauthenticated
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.user_id',$1,true),set_config('reforge.org_id',$2,true)`, userID, attempt.OrgID); err != nil {
			return err
		}
		var orgID string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR SHARE`, attempt.OrgID).Scan(&orgID); err != nil {
			return ErrUnauthenticated
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_oidc_configs WHERE org_id=$1 AND id=$2 AND version=$3 AND issuer=$4 AND status='active' AND verified_version=version)`, attempt.OrgID, attempt.ConfigID, attempt.ConfigVersion, attempt.Issuer).Scan(&active); err != nil || !active {
			return ErrUnauthenticated
		}
		var member bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships WHERE org_id=$1 AND user_id=$2)`, attempt.OrgID, userID).Scan(&member); err != nil || !member {
			return ErrUnauthenticated
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,org_id,oidc_config_id,oidc_config_version,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,now()+interval '12 hours')`, domain.NewID(), userID, attempt.OrgID, attempt.ConfigID, attempt.ConfigVersion, digest(token), csrf)
		return err
	})
	return token, err
}
