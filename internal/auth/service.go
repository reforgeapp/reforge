package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
	"reforge/internal/domain"
	"reforge/internal/secrets"
	"reforge/internal/store"
)

var (
	ErrUnauthenticated = errors.New("authentication required")
	ErrForbidden       = errors.New("access denied")
	ErrConflict        = errors.New("version conflict")
	ErrInvalid         = errors.New("invalid request")
)

type Config struct {
	PublicURL          string
	Edition            string
	Development        bool
	FixtureAuth        bool
	ListenAddress      string
	OIDCIssuer         string
	OIDCClientID       string
	OIDCClientSecret   string `json:"-"`
	BootstrapToken     string `json:"-"`
	BootstrapExpiresAt time.Time
}

type Service struct {
	db           *store.Store
	cfg          Config
	origin       *url.URL
	verifier     *oidc.IDTokenVerifier
	oauth        oauth2.Config
	httpClient   *http.Client
	orgOIDCVault *secrets.Vault
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Session struct {
	automation        *automationGrant
	ID                string                `json:"-"`
	OrganizationID    string                `json:"-"`
	OIDCConfigID      string                `json:"-"`
	OIDCConfigVersion int64                 `json:"-"`
	User              User                  `json:"user"`
	Organisations     []domain.Organisation `json:"organisations"`
	Memberships       []Membership          `json:"memberships"`
	CSRFToken         string                `json:"csrf_token"`
}

type Membership struct {
	OrgID           string      `json:"org_id"`
	UserID          string      `json:"user_id,omitempty"`
	Role            domain.Role `json:"role"`
	TeamIDs         []string    `json:"team_ids"`
	RepositoryIDs   []string    `json:"repository_ids"`
	AllRepositories bool        `json:"all_repositories"`
	Version         int64       `json:"version"`
}

func New(ctx context.Context, db *store.Store, cfg Config) (*Service, error) {
	origin, err := url.Parse(cfg.PublicURL)
	if err != nil || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || db == nil {
		return nil, ErrInvalid
	}
	host, _, listenErr := net.SplitHostPort(cfg.ListenAddress)
	loopback := func(host string) bool { ip := net.ParseIP(host); return ip != nil && ip.IsLoopback() }
	if cfg.Development && (cfg.Edition != "self-hosted" || listenErr != nil || !loopback(host) || !loopback(origin.Hostname())) {
		return nil, ErrInvalid
	}
	if cfg.FixtureAuth && !cfg.Development {
		return nil, ErrInvalid
	}
	if origin.Scheme != "https" && !(cfg.Development && origin.Scheme == "http") {
		return nil, ErrInvalid
	}
	s := &Service{db: db, cfg: cfg, origin: origin, httpClient: &http.Client{Timeout: 15 * time.Second}}
	if cfg.OIDCIssuer != "" {
		issuer, err := url.Parse(cfg.OIDCIssuer)
		if err != nil || issuer.Host == "" || issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" || (issuer.Scheme != "https" && !(cfg.Development && issuer.Scheme == "http" && loopback(issuer.Hostname()))) || cfg.OIDCClientID == "" {
			return nil, ErrInvalid
		}
		provider, err := oidc.NewProvider(oidc.ClientContext(ctx, s.httpClient), cfg.OIDCIssuer)
		if err != nil {
			return nil, fmt.Errorf("OIDC discovery failed: %w", err)
		}
		s.verifier = provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})
		s.oauth = oauth2.Config{ClientID: cfg.OIDCClientID, ClientSecret: cfg.OIDCClientSecret, Endpoint: provider.Endpoint(), RedirectURL: cfg.PublicURL + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	}
	if cfg.BootstrapToken != "" {
		if cfg.Edition != "self-hosted" || len(cfg.BootstrapToken) < 32 || cfg.BootstrapExpiresAt.IsZero() {
			return nil, ErrInvalid
		}
		err = s.identity(ctx, "", map[string]string{"reforge.bootstrap_hash": digest(cfg.BootstrapToken)}, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO bootstrap(id,token_hash,expires_at) VALUES(true,$1,$2) ON CONFLICT(id) DO NOTHING`, digest(cfg.BootstrapToken), cfg.BootstrapExpiresAt)
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Service) identity(ctx context.Context, userID string, settings map[string]string, fn func(pgx.Tx) error) error {
	return s.db.Identity(ctx, userID, func(tx pgx.Tx) error {
		for key, value := range settings {
			if _, err := tx.Exec(ctx, `SELECT set_config($1,$2,true)`, key, value); err != nil {
				return err
			}
		}
		return fn(tx)
	})
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func digest(value string) string { b := sha256.Sum256([]byte(value)); return hex.EncodeToString(b[:]) }
func equal(a, b string) bool {
	return len(a) > 0 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func (s *Service) CookieName() string {
	if s.origin.Scheme == "https" {
		return "__Host-reforge_session"
	}
	return "reforge_development_session"
}
func (s *Service) cookie(name, value string, age int) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: s.origin.Scheme == "https", SameSite: http.SameSiteLaxMode, MaxAge: age}
}
func (s *Service) SetSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, s.cookie(s.CookieName(), token, 43200))
}
func (s *Service) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, s.cookie(s.CookieName(), "", -1))
}
func (s *Service) RequestToken(r *http.Request) string {
	c, err := r.Cookie(s.CookieName())
	if err != nil {
		return ""
	}
	return c.Value
}
func (s *Service) CheckRequest(r *http.Request, csrf string) error {
	if !strings.EqualFold(r.Host, s.origin.Host) {
		return ErrForbidden
	}
	if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
		return nil
	}
	if r.Header.Get("Origin") != s.cfg.PublicURL || r.Header.Get("Sec-Fetch-Site") == "cross-site" || !equal(r.Header.Get("X-CSRF-Token"), csrf) {
		return ErrForbidden
	}
	return nil
}

func (s *Service) Login(ctx context.Context, w http.ResponseWriter, orgIDs ...string) (string, error) {
	if len(orgIDs) > 1 {
		return "", ErrInvalid
	}
	if len(orgIDs) == 1 {
		if orgIDs[0] == "" {
			return "", ErrInvalid
		}
		return s.beginOrgOIDCLogin(ctx, w, orgIDs[0])
	}
	if s.cfg.FixtureAuth {
		user, err := s.upsertUser(ctx, "reforge:development", "local-owner", "Development owner", "owner@localhost")
		if err != nil {
			return "", err
		}
		err = s.seedDevelopment(ctx, user.ID)
		if err != nil {
			return "", err
		}
		token, err := s.createSession(ctx, user.ID)
		if err != nil {
			return "", err
		}
		s.SetSession(w, token)
		return "/", nil
	}
	if s.verifier == nil {
		return "", ErrForbidden
	}
	state, browser, nonce, verifier := randomToken(), randomToken(), randomToken(), oauth2.GenerateVerifier()
	err := s.identity(ctx, "", map[string]string{"reforge.login_hash": digest(state)}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO oidc_logins(state_hash,browser_hash,nonce,verifier,expires_at) VALUES($1,$2,$3,$4,now()+interval '10 minutes')`, digest(state), digest(browser), nonce, verifier)
		return err
	})
	if err != nil {
		return "", err
	}
	http.SetCookie(w, s.cookie(s.oidcCookieName(), browser, 600))
	return s.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

func (s *Service) Callback(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	query := r.URL.Query()
	states, codes := query["state"], query["code"]
	if len(states) != 1 || len(codes) != 1 {
		return ErrUnauthenticated
	}
	state, code := states[0], codes[0]
	if len(state) != 43 || len(code) == 0 || len(code) > 4096 {
		return ErrUnauthenticated
	}
	browser, err := r.Cookie(s.oidcCookieName())
	if err != nil || len(browser.Value) != 43 {
		return ErrUnauthenticated
	}
	attempt, err := s.consumeOIDCLogin(ctx, state, browser.Value)
	if err != nil {
		return ErrUnauthenticated
	}
	http.SetCookie(w, s.cookie(s.oidcCookieName(), "", -1))
	if attempt.OrgID != "" {
		return s.completeOrgOIDCCallback(ctx, w, attempt, code)
	}
	if s.verifier == nil {
		return ErrUnauthenticated
	}
	token, err := s.oauth.Exchange(context.WithValue(ctx, oauth2.HTTPClient, s.httpClient), code, oauth2.VerifierOption(attempt.Verifier))
	if err != nil {
		return ErrUnauthenticated
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return ErrUnauthenticated
	}
	id, err := s.verifier.Verify(oidc.ClientContext(ctx, s.httpClient), raw)
	if err != nil || !equal(id.Nonce, attempt.Nonce) || id.Subject == "" {
		return ErrUnauthenticated
	}
	var claims struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err = id.Claims(&claims); err != nil {
		return ErrUnauthenticated
	}
	user, err := s.upsertUser(ctx, id.Issuer, id.Subject, claims.Name, claims.Email)
	if err != nil {
		return err
	}
	session, err := s.createSession(ctx, user.ID)
	if err != nil {
		return err
	}
	s.SetSession(w, session)
	return nil
}

func (s *Service) upsertUser(ctx context.Context, issuer, subject, name, email string) (User, error) {
	var user User
	err := s.identity(ctx, "", map[string]string{"reforge.issuer": issuer, "reforge.subject": subject}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,$2,$3,$4,$5) ON CONFLICT(issuer,subject) DO UPDATE SET name=EXCLUDED.name,email=EXCLUDED.email RETURNING id::text,name,email`, domain.NewID(), issuer, subject, name, email).Scan(&user.ID, &user.Name, &user.Email)
	})
	return user, err
}
func (s *Service) createSession(ctx context.Context, userID string) (string, error) {
	token := randomToken()
	err := s.db.Identity(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,now()+interval '12 hours')`, domain.NewID(), userID, digest(token), randomToken())
		return err
	})
	return token, err
}

func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	session := Session{Organisations: []domain.Organisation{}, Memberships: []Membership{}}
	if len(token) != 43 {
		return session, ErrUnauthenticated
	}
	err := s.identity(ctx, "", map[string]string{"reforge.session_hash": digest(token)}, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id::text,user_id::text,csrf_token,coalesce(org_id::text,''),coalesce(oidc_config_id::text,''),coalesce(oidc_config_version,0) FROM sessions WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at>now()`, digest(token)).Scan(&session.ID, &session.User.ID, &session.CSRFToken, &session.OrganizationID, &session.OIDCConfigID, &session.OIDCConfigVersion); err != nil {
			return err
		}
		if session.OrganizationID != "" {
			if !ValidID(session.OrganizationID) || !ValidID(session.OIDCConfigID) || session.OIDCConfigVersion < 1 {
				return ErrUnauthenticated
			}
			for key, value := range map[string]string{
				"reforge.login_org_id":         session.OrganizationID,
				"reforge.login_config_id":      session.OIDCConfigID,
				"reforge.login_config_version": formatOIDCVersion(session.OIDCConfigVersion),
			} {
				if _, err := tx.Exec(ctx, `SELECT set_config($1,$2,true)`, key, value); err != nil {
					return err
				}
			}
			var active bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_oidc_configs WHERE org_id=$1 AND id=$2 AND version=$3 AND status='active' AND verified_version=version)`, session.OrganizationID, session.OIDCConfigID, session.OIDCConfigVersion).Scan(&active); err != nil || !active {
				return ErrUnauthenticated
			}
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.user_id',$1,true)`, session.User.ID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT name,email FROM users WHERE id=$1`, session.User.ID).Scan(&session.User.Name, &session.User.Email); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT org_id::text,role,all_repositories,version FROM memberships WHERE user_id=$1 AND ($2='' OR org_id=$2::uuid) ORDER BY org_id`, session.User.ID, session.OrganizationID)
		if err != nil {
			return err
		}
		for rows.Next() {
			m := Membership{UserID: session.User.ID, TeamIDs: []string{}, RepositoryIDs: []string{}}
			if err = rows.Scan(&m.OrgID, &m.Role, &m.AllRepositories, &m.Version); err != nil {
				rows.Close()
				return err
			}
			session.Memberships = append(session.Memberships, m)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if session.OrganizationID != "" && len(session.Memberships) != 1 {
			return ErrUnauthenticated
		}
		for i := range session.Memberships {
			m := &session.Memberships[i]
			if _, err = tx.Exec(ctx, `SELECT set_config('reforge.org_id',$1,true)`, m.OrgID); err != nil {
				return err
			}
			var org domain.Organisation
			if err = tx.QueryRow(ctx, `SELECT id::text,name,version,paused FROM organisations WHERE id=$1`, m.OrgID).Scan(&org.ID, &org.Name, &org.Version, &org.Paused); err != nil {
				return err
			}
			session.Organisations = append(session.Organisations, org)
			if err = loadBindings(ctx, tx, m); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrUnauthenticated
	}
	return session, err
}

func (s *Service) Logout(ctx context.Context, session Session) error {
	return s.db.Identity(ctx, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE id=$1 AND user_id=$2`, session.ID, session.User.ID)
		return err
	})
}

func (s *Service) RevokeSessions(ctx context.Context, session Session) error {
	return s.db.Identity(ctx, session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1`, session.User.ID)
		return err
	})
}

func (s *Service) Bootstrap(ctx context.Context, session Session, token, name, requestID string) (domain.Organisation, error) {
	org := domain.Organisation{ID: domain.NewID(), Name: strings.TrimSpace(name), Version: 1}
	if s.cfg.Edition != "self-hosted" || len(token) < 32 || len(org.Name) < 1 || len(org.Name) > 160 {
		return org, ErrForbidden
	}
	err := s.db.Tenant(ctx, org.ID, session.User.ID, func(tx pgx.Tx) error {
		if err := lockSession(ctx, tx, session); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.bootstrap_hash',$1,true)`, digest(token)); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE bootstrap SET consumed_at=now() WHERE id AND token_hash=$1 AND consumed_at IS NULL AND expires_at>now()`, digest(token))
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return ErrForbidden
		}
		if _, err = tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,$2)`, org.ID, org.Name); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org.ID, session.User.ID); err != nil {
			return err
		}
		return audit(ctx, tx, org.ID, session.User.ID, "organisation.bootstrap", org.ID, requestID, 1)
	})
	return org, err
}

func (s *Service) seedDevelopment(ctx context.Context, userID string) error {
	const orgID = "00000000-0000-4000-8000-000000000001"
	return s.db.Tenant(ctx, orgID, userID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Development') ON CONFLICT(id) DO NOTHING`, orgID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true) ON CONFLICT DO NOTHING`, orgID, userID)
		return err
	})
}

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(slog.String("public_url", c.PublicURL), slog.String("edition", c.Edition), slog.Bool("fixture_auth", c.FixtureAuth))
}

func (s *Service) oidcCookieName() string {
	if s.origin.Scheme == "https" {
		return "__Host-reforge_oidc"
	}
	return "reforge_development_oidc"
}

func (c Config) String() string {
	return "auth.Config{public_url:" + c.PublicURL + ",edition:" + c.Edition + "}"
}
