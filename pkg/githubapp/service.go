package githubapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/reforgeapp/reforge/pkg/heartbeat"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/inventory"
	"github.com/reforgeapp/reforge/pkg/secrets"
	"github.com/reforgeapp/reforge/pkg/store"
)

const (
	webOrigin = "https://github.com"
	apiOrigin = "https://api.github.com"
	setupTTL  = 15 * time.Minute
)

var (
	ErrUnavailable = errors.New("GitHub App setup unavailable")
	errClaimed     = errors.New("installation already connected")
	orgPattern     = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`)
)

type Options struct {
	PublicURL   string
	Edition     string
	Development bool
	Hosted      *Hosted
	Web, API    string
	Transport   http.RoundTripper
}

type Service struct {
	db       *store.Store
	auth     *auth.Service
	vault    *secrets.Vault
	conns    *connections.Service
	inv      *inventory.Service
	opt      Options
	web, api string
	client   *http.Client
}

func New(db *store.Store, identity *auth.Service, vault *secrets.Vault, conns *connections.Service, inv *inventory.Service, opt Options) *Service {
	s := &Service{db: db, auth: identity, vault: vault, conns: conns, inv: inv, opt: opt, web: webOrigin, api: apiOrigin}
	if opt.Web != "" {
		s.web = strings.TrimRight(opt.Web, "/")
	}
	if opt.API != "" {
		s.api = strings.TrimRight(opt.API, "/")
	}
	transport := opt.Transport
	if transport == nil {
		transport = http.DefaultTransport.(*http.Transport).Clone()
		transport.(*http.Transport).Proxy = nil
	}
	s.client = &http.Client{Timeout: 10 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if opt.Hosted != nil && conns != nil {
		conns.SetPlatformApp(connections.PlatformApp{AppID: strconv.FormatInt(opt.Hosted.AppID, 10), PrivateKeyPEM: opt.Hosted.PrivateKeyPEM})
	}
	return s
}

func (s *Service) WebOrigin() string { return s.web }

type Pending struct {
	ID        string    `json:"id"`
	Phase     string    `json:"phase"`
	AppSlug   string    `json:"app_slug,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	ResumeURL string    `json:"resume_url,omitempty"`
}

type Status struct {
	Mode    string   `json:"mode"`
	Reason  string   `json:"reason,omitempty"`
	Pending *Pending `json:"pending,omitempty"`
}

type Created struct {
	ID         string `json:"id"`
	HandoffURL string `json:"handoff_url"`
}

type pendingSecrets struct {
	PEM           string `json:"pem,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	ClientSecret  string `json:"client_secret,omitempty"`
	Verifier      string `json:"verifier,omitempty"`
}

type setup struct {
	OrgID, ID, UserID, SessionID, Mode, Phase, Name, GitHubOrg string
	ConnectionID, WebhookID, AppSlug                           string
	AppID, OwnerID, InstallationID                             int64
	Envelope                                                   []byte
	ExpiresAt                                                  time.Time
}

func (s *Service) mode() (string, string) {
	u, err := url.Parse(s.opt.PublicURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(s.opt.Development && u.Scheme == "http" && isLoopback(u.Hostname()))) {
		return "unavailable", "public_https_required"
	}
	if s.opt.Edition == "hosted" {
		if s.opt.Hosted == nil {
			return "unavailable", "hosted_app_not_configured"
		}
		return "hosted", ""
	}
	return "manifest", ""
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Service) checkSetupConfig(st *setup) error {
	switch st.Mode {
	case "hosted":
		if s.opt.Hosted == nil || st.AppID != s.opt.Hosted.AppID {
			return ErrUnavailable
		}
	case "manifest":
		if mode, _ := s.mode(); mode != "manifest" {
			return ErrUnavailable
		}
	default:
		return ErrUnavailable
	}
	return nil
}

func (s *Service) webhookActive() bool {
	u, err := url.Parse(s.opt.PublicURL)
	return err == nil && u.Scheme == "https"
}

func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func digest(value string) string { b := sha256.Sum256([]byte(value)); return hex.EncodeToString(b[:]) }

func (s *Service) Status(ctx context.Context, session auth.Session, org string) (Status, error) {
	out := Status{}
	out.Mode, out.Reason = s.mode()
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return nil
		}
		var p Pending
		err := tx.QueryRow(ctx, `SELECT id::text,phase,app_slug,expires_at FROM github_app_setups WHERE org_id=$1 AND user_id=$2 AND session_id=$3 AND expires_at>now() ORDER BY created_at DESC LIMIT 1`, org, a.UserID, session.ID).Scan(&p.ID, &p.Phase, &p.AppSlug, &p.ExpiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if p.Phase == "awaiting_install" || p.Phase == "created" {
			p.ResumeURL = "/auth/github/setup/" + p.ID
		}
		out.Pending = &p
		return nil
	})
	return out, err
}

func (s *Service) Start(ctx context.Context, session auth.Session, org, name, githubOrg, request string) (Created, error) {
	var out Created
	mode, _ := s.mode()
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 160 || (githubOrg != "" && (!orgPattern.MatchString(githubOrg) || mode != "manifest")) {
		return out, auth.ErrInvalid
	}
	if mode == "unavailable" {
		return out, ErrUnavailable
	}
	if !auth.ValidID(session.ID) {
		return out, auth.ErrForbidden
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		if _, err := tx.Exec(ctx, `DELETE FROM github_app_setups WHERE org_id=$1 AND (user_id=$2 OR expires_at<=now())`, org, a.UserID); err != nil {
			return err
		}
		out.ID = domain.NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO github_app_setups(org_id,id,user_id,session_id,mode,phase,name,github_org,connection_id,webhook_id,expires_at) VALUES($1,$2,$3,$4,$5,'created',$6,$7,$8,$9,now()+$10::interval)`, org, out.ID, a.UserID, session.ID, mode, name, githubOrg, domain.NewID(), domain.NewID(), setupTTL.String()); err != nil {
			return err
		}
		if mode == "hosted" {
			if _, err := tx.Exec(ctx, `UPDATE github_app_setups SET app_id=$3 WHERE org_id=$1 AND id=$2`, org, out.ID, s.opt.Hosted.AppID); err != nil {
				return err
			}
		}
		return audit(ctx, tx, org, a.UserID, "github_app.setup_created", out.ID, request)
	})
	if err != nil {
		return Created{}, err
	}
	out.HandoffURL = "/auth/github/setup/" + out.ID
	return out, nil
}

func (s *Service) Cancel(ctx context.Context, session auth.Session, org, id, request string) error {
	if !auth.ValidID(id) {
		return auth.ErrInvalid
	}
	return s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		tag, err := tx.Exec(ctx, `DELETE FROM github_app_setups WHERE org_id=$1 AND id=$2 AND user_id=$3`, org, id, a.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return auth.ErrForbidden
		}
		return audit(ctx, tx, org, a.UserID, "github_app.setup_cancelled", id, request)
	})
}

type Handoff struct {
	OrgID    string
	Action   string
	Manifest string
	Location string
}

func (s *Service) lookupOrg(ctx context.Context, userID string, settings map[string]string) (string, error) {
	var org string
	err := s.db.Identity(ctx, userID, func(tx pgx.Tx) error {
		for key, value := range settings {
			if _, err := tx.Exec(ctx, `SELECT set_config($1,$2,true)`, key, value); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx, `SELECT org_id::text FROM github_app_setups LIMIT 1`).Scan(&org)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrForbidden
	}
	return org, err
}

func scanSetup(row pgx.Row) (setup, error) {
	var st setup
	var app, owner, installation *int64
	err := row.Scan(&st.OrgID, &st.ID, &st.UserID, &st.SessionID, &st.Mode, &st.Phase, &st.Name, &st.GitHubOrg, &st.ConnectionID, &st.WebhookID, &app, &st.AppSlug, &owner, &installation, &st.Envelope, &st.ExpiresAt)
	for _, pair := range []struct {
		src *int64
		dst *int64
	}{{app, &st.AppID}, {owner, &st.OwnerID}, {installation, &st.InstallationID}} {
		if pair.src != nil {
			*pair.dst = *pair.src
		}
	}
	return st, err
}

const setupColumns = `org_id::text,id::text,user_id::text,session_id::text,mode,phase,name,github_org,connection_id::text,webhook_id::text,app_id,app_slug,owner_id,installation_id,envelope,expires_at`

func (s *Service) transition(ctx context.Context, session auth.Session, org, where string, arg string, from []string, fn func(pgx.Tx, domain.Actor, *setup) error) (setup, error) {
	var st setup
	var denied, mismatch, expired, roleLost bool
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		st, err = scanSetup(tx.QueryRow(ctx, `SELECT `+setupColumns+` FROM github_app_setups WHERE org_id=$1 AND `+where+`=$2 FOR UPDATE`, org, arg))
		if errors.Is(err, pgx.ErrNoRows) {
			return errExpired
		}
		if err != nil {
			return err
		}
		ok := false
		for _, phase := range from {
			ok = ok || st.Phase == phase
		}
		if st.UserID != a.UserID {
			denied = true
			return auth.ErrForbidden
		}
		if st.SessionID != session.ID {
			mismatch = true
			return errSession
		}
		if !st.ExpiresAt.After(time.Now()) {
			if _, err = tx.Exec(ctx, `DELETE FROM github_app_setups WHERE org_id=$1 AND id=$2`, org, st.ID); err != nil {
				return err
			}
			expired = true
			return nil
		}
		if a.Role != domain.Owner {
			if _, err = tx.Exec(ctx, `DELETE FROM github_app_setups WHERE org_id=$1 AND id=$2`, org, st.ID); err != nil {
				return err
			}
			roleLost = true
			return nil
		}
		if !ok {
			return errExpired
		}
		return fn(tx, a, &st)
	})
	if denied {
		return st, auth.ErrForbidden
	}
	if mismatch {
		return st, errSession
	}
	if err != nil {
		return st, err
	}
	if expired {
		return st, errExpired
	}
	if roleLost {
		return st, auth.ErrForbidden
	}
	return st, nil
}

var errExpired = errors.New("setup expired")
var errSession = errors.New("setup started in another session")

func (s *Service) seal(ctx context.Context, st setup, p pendingSecrets) ([]byte, error) {
	raw, _ := json.Marshal(p)
	defer clear(raw)
	envelope, err := s.vault.SealContext(ctx, secrets.Binding{OrgID: st.OrgID, ConnectionID: st.ID, Version: 1}, raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func (s *Service) open(ctx context.Context, st setup) (pendingSecrets, error) {
	var p pendingSecrets
	if len(st.Envelope) == 0 {
		return p, nil
	}
	var envelope secrets.Envelope
	if err := json.Unmarshal(st.Envelope, &envelope); err != nil {
		return p, err
	}
	plain, err := s.vault.OpenContext(ctx, secrets.Binding{OrgID: st.OrgID, ConnectionID: st.ID, Version: 1}, envelope)
	if err != nil {
		return p, err
	}
	defer clear(plain)
	return p, json.Unmarshal(plain, &p)
}

func (s *Service) Handoff(ctx context.Context, session auth.Session, id string) (Handoff, error) {
	var out Handoff
	if !auth.ValidID(id) || !auth.ValidID(session.User.ID) {
		return out, errExpired
	}
	org, err := s.lookupOrg(ctx, session.User.ID, map[string]string{"reforge.github_setup_id": id})
	if err != nil {
		return out, errExpired
	}
	out.OrgID = org
	state := token()
	st, err := s.transition(ctx, session, org, "id", id, []string{"created", "awaiting_install"}, func(tx pgx.Tx, a domain.Actor, st *setup) error {
		if err := s.checkSetupConfig(st); err != nil {
			return err
		}
		next := "handed_off"
		if st.Phase == "awaiting_install" {
			next = "awaiting_install"
		}
		_, err := tx.Exec(ctx, `UPDATE github_app_setups SET phase=$3,state_hash=$4 WHERE org_id=$1 AND id=$2`, org, st.ID, next, digest(state))
		return err
	})
	if err != nil {
		return out, err
	}
	if st.Mode == "manifest" && st.Phase == "created" {
		out.Action = s.web + "/settings/apps/new?state=" + url.QueryEscape(state)
		if st.GitHubOrg != "" {
			out.Action = s.web + "/organizations/" + url.PathEscape(st.GitHubOrg) + "/settings/apps/new?state=" + url.QueryEscape(state)
		}
		out.Manifest = s.manifest(st)
		return out, nil
	}
	slug := st.AppSlug
	if st.Mode == "hosted" {
		slug = s.opt.Hosted.Slug
	}
	out.Location = s.web + "/apps/" + url.PathEscape(slug) + "/installations/new?state=" + url.QueryEscape(state)
	return out, nil
}

func (s *Service) manifest(st setup) string {
	public := strings.TrimRight(s.opt.PublicURL, "/")
	body, _ := json.Marshal(map[string]any{
		"name":                     st.Name,
		"url":                      public,
		"redirect_url":             public + "/auth/github/manifest/callback",
		"callback_urls":            []string{public + "/auth/github/oauth/callback"},
		"setup_url":                public + "/auth/github/install/callback",
		"setup_on_update":          true,
		"request_oauth_on_install": false,
		"public":                   false,
		"hook_attributes":          map[string]any{"url": public + "/hooks/v1/" + st.OrgID + "/" + st.WebhookID, "active": s.webhookActive()},
		"default_permissions":      Permissions,
		"default_events":           Events,
	})
	return string(body)
}

var Permissions = map[string]string{
	"metadata":             "read",
	"contents":             "write",
	"pull_requests":        "write",
	"checks":               "write",
	"statuses":             "read",
	"actions":              "write",
	"workflows":            "write",
	"administration":       "read",
	"members":              "read",
	"issues":               "read",
	"vulnerability_alerts": "read",
}

var Events = []string{"push", "pull_request", "repository", "check_run", "check_suite", "status", "workflow_run"}

type Result struct {
	OrgID        string
	Outcome      string
	Reason       string
	ConnectionID string
	Location     string
}

func failed(org, reason string) Result { return Result{OrgID: org, Outcome: "failed", Reason: reason} }

func outcome(org string, err error) Result {
	switch {
	case errors.Is(err, auth.ErrForbidden), errors.Is(err, auth.ErrUnauthenticated):
		return failed(org, "owner_required")
	case errors.Is(err, errSession):
		return failed(org, "session_mismatch")
	case errors.Is(err, errExpired), errors.Is(err, pgx.ErrNoRows):
		return Result{OrgID: org, Outcome: "expired"}
	case errors.Is(err, ErrUnavailable):
		return failed(org, "provider_unavailable")
	default:
		return failed(org, "setup_failed")
	}
}

func (s *Service) byState(ctx context.Context, session auth.Session, state string) (string, error) {
	if len(state) < 32 || len(state) > 128 || !auth.ValidID(session.User.ID) {
		return "", errExpired
	}
	org, err := s.lookupOrg(ctx, session.User.ID, map[string]string{"reforge.github_state_hash": digest(state)})
	if err != nil {
		return "", errExpired
	}
	return org, nil
}

func (s *Service) discard(ctx context.Context, session auth.Session, org, id string) {
	_ = s.auth.WithMutation(context.WithoutCancel(ctx), session, org, func(tx pgx.Tx, a domain.Actor) error {
		_, err := tx.Exec(ctx, `DELETE FROM github_app_setups WHERE org_id=$1 AND id=$2`, org, id)
		return err
	})
}

func (s *Service) ManifestCallback(ctx context.Context, session auth.Session, code, state string) Result {
	org, err := s.byState(ctx, session, state)
	if err != nil {
		return outcome("", err)
	}
	st, err := s.transition(ctx, session, org, "state_hash", digest(state), []string{"handed_off"}, func(tx pgx.Tx, a domain.Actor, st *setup) error {
		if st.Mode != "manifest" {
			return errExpired
		}
		if err := s.checkSetupConfig(st); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE github_app_setups SET phase='converting',state_hash=NULL WHERE org_id=$1 AND id=$2`, org, st.ID)
		return err
	})
	if err != nil {
		return outcome(org, err)
	}
	if code == "" || len(code) > 256 {
		s.discard(ctx, session, org, st.ID)
		return failed(org, "conversion_failed")
	}
	var app struct {
		ID            int64   `json:"id"`
		Slug          string  `json:"slug"`
		ClientID      string  `json:"client_id"`
		ClientSecret  string  `json:"client_secret"`
		WebhookSecret *string `json:"webhook_secret"`
		PEM           string  `json:"pem"`
		Owner         struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"owner"`
	}
	if err = s.call(ctx, "POST", s.api+"/app-manifests/"+url.PathEscape(code)+"/conversions", "", nil, &app); err != nil {
		s.discard(ctx, session, org, st.ID)
		return failed(org, "conversion_failed")
	}
	webhookSecret := ""
	if app.WebhookSecret != nil {
		webhookSecret = *app.WebhookSecret
	}
	ownerType := "User"
	if st.GitHubOrg != "" {
		ownerType = "Organization"
	}
	if _, keyErr := parseKey(app.PEM); keyErr != nil || app.ID <= 0 || !slugPattern.MatchString(app.Slug) || !clientPattern.MatchString(app.ClientID) || app.ClientSecret == "" || app.Owner.ID <= 0 || app.Owner.Type != ownerType || (st.GitHubOrg != "" && !strings.EqualFold(app.Owner.Login, st.GitHubOrg)) || (s.webhookActive() && len(webhookSecret) < 16) {
		s.discard(ctx, session, org, st.ID)
		return failed(org, "app_created_incomplete")
	}
	next := token()
	_, err = s.transition(ctx, session, org, "id", st.ID, []string{"converting"}, func(tx pgx.Tx, a domain.Actor, current *setup) error {
		envelope, err := s.seal(ctx, *current, pendingSecrets{PEM: app.PEM, WebhookSecret: webhookSecret, ClientID: app.ClientID, ClientSecret: app.ClientSecret})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE github_app_setups SET phase='awaiting_install',state_hash=$3,envelope=$4,app_id=$5,app_slug=$6,owner_id=$7 WHERE org_id=$1 AND id=$2`, org, current.ID, digest(next), envelope, app.ID, app.Slug, app.Owner.ID)
		return err
	})
	app.PEM, app.ClientSecret, webhookSecret = "", "", ""
	if err != nil {
		r := outcome(org, err)
		r.Outcome, r.Reason = "failed", "app_created_incomplete"
		return r
	}
	return Result{OrgID: org, Outcome: "redirect", Location: s.web + "/apps/" + url.PathEscape(app.Slug) + "/installations/new?state=" + url.QueryEscape(next)}
}

func (s *Service) clientID(p pendingSecrets) string {
	if s.opt.Hosted != nil && p.ClientID == "" {
		return s.opt.Hosted.ClientID
	}
	return p.ClientID
}

func (s *Service) InstallCallback(ctx context.Context, session auth.Session, installation, action, state string) Result {
	org, err := s.byState(ctx, session, state)
	if err != nil {
		return outcome("", err)
	}
	id, parseErr := strconv.ParseInt(installation, 10, 64)
	next, verifier := token(), token()
	var location string
	var pending bool
	_, err = s.transition(ctx, session, org, "state_hash", digest(state), []string{"handed_off", "awaiting_install"}, func(tx pgx.Tx, a domain.Actor, st *setup) error {
		if err := s.checkSetupConfig(st); err != nil {
			return err
		}
		if st.Mode == "manifest" && st.Phase != "awaiting_install" {
			return errExpired
		}
		if action == "request" || parseErr != nil || id <= 0 {
			pending = true
			_, err := tx.Exec(ctx, `UPDATE github_app_setups SET phase='awaiting_install',state_hash=NULL WHERE org_id=$1 AND id=$2`, org, st.ID)
			return err
		}
		p, err := s.open(ctx, *st)
		if err != nil {
			return err
		}
		p.Verifier = verifier
		envelope, err := s.seal(ctx, *st, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(verifier))
		query := url.Values{"client_id": {s.clientID(p)}, "redirect_uri": {strings.TrimRight(s.opt.PublicURL, "/") + "/auth/github/oauth/callback"}, "state": {next}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "allow_signup": {"false"}}
		location = s.web + "/login/oauth/authorize?" + query.Encode()
		_, err = tx.Exec(ctx, `UPDATE github_app_setups SET phase='authorizing',state_hash=$3,envelope=$4,installation_id=$5 WHERE org_id=$1 AND id=$2`, org, st.ID, digest(next), envelope, id)
		return err
	})
	if err != nil {
		return outcome(org, err)
	}
	if pending {
		if action == "request" {
			return Result{OrgID: org, Outcome: "pending_approval"}
		}
		return failed(org, "installation_missing")
	}
	return Result{OrgID: org, Outcome: "redirect", Location: location}
}

type installation struct {
	ID      int64 `json:"id"`
	AppID   int64 `json:"app_id"`
	Account struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
	SuspendedAt *time.Time `json:"suspended_at"`
}

func (s *Service) OAuthCallback(ctx context.Context, session auth.Session, code, state, providerError string) Result {
	org, err := s.byState(ctx, session, state)
	if err != nil {
		return outcome("", err)
	}
	st, err := s.transition(ctx, session, org, "state_hash", digest(state), []string{"authorizing"}, func(tx pgx.Tx, a domain.Actor, st *setup) error {
		if err := s.checkSetupConfig(st); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE github_app_setups SET phase='completing',state_hash=NULL WHERE org_id=$1 AND id=$2`, org, st.ID)
		return err
	})
	if err != nil {
		return outcome(org, err)
	}
	fail := func(reason string) Result {
		s.discard(ctx, session, org, st.ID)
		if st.Mode == "manifest" && reason != "already_connected" {
			reason = "app_created_" + reason
		}
		return failed(org, reason)
	}
	if providerError != "" || code == "" || len(code) > 256 {
		return fail("denied")
	}
	p, err := s.open(ctx, st)
	if err != nil || p.Verifier == "" {
		return fail("setup_failed")
	}
	appID, key, clientSecret := st.AppID, []byte(p.PEM), p.ClientSecret
	if st.Mode == "hosted" {
		appID, key, clientSecret = s.opt.Hosted.AppID, s.opt.Hosted.PrivateKeyPEM, string(s.opt.Hosted.ClientSecret)
	}
	userToken, err := s.exchange(ctx, s.clientID(p), clientSecret, code, p.Verifier)
	clientSecret = ""
	if err != nil {
		return fail("oauth_failed")
	}
	inst, reason := s.verify(ctx, userToken, appID, st.InstallationID, key)
	userToken = ""
	if reason != "" {
		return fail(reason)
	}
	if st.Mode == "manifest" && inst.Account.ID != st.OwnerID {
		return fail("not_owner")
	}
	var connectionID string
	_, err = s.transition(ctx, session, org, "id", st.ID, []string{"completing"}, func(tx pgx.Tx, a domain.Actor, current *setup) error {
		c := connections.Connection{ID: current.ConnectionID, OrgID: org, Kind: "forge", Provider: "github", Name: current.Name, Endpoint: connections.GitHubAPI, Settings: connections.Settings{BillingRoute: "forge", Namespace: "installation", AppID: strconv.FormatInt(appID, 10), InstallationID: strconv.FormatInt(inst.ID, 10)}}
		secret, webhookSecret := "", ""
		if current.Mode == "hosted" {
			c.Settings.AuthKind, c.Settings.Managed = connections.PlatformAuthKind, "github_hosted"
			if s.opt.Hosted != nil {
				webhookSecret = string(s.opt.Hosted.WebhookSecret)
			}
		} else {
			c.Settings.AuthKind, c.Settings.Managed, secret = "github_app", "github_manifest", p.PEM
			webhookSecret = p.WebhookSecret
			if !s.webhookActive() {
				c.Settings.WebhookPending = true
				c.State, c.Reason = "unverified", "Webhook inactive: set a public HTTPS URL and update the GitHub App webhook, then run a capability test"
			}
		}
		if err := s.conns.CreateManagedTx(ctx, tx, a, &c, secret, current.ID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO github_installation_bindings(app_id,installation_id,account_id,org_id,connection_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, appID, inst.ID, inst.Account.ID, org, c.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errClaimed
		}
		if webhookSecret != "" {
			if _, err := s.inv.AdoptWebhookTx(ctx, tx, a, org, c.ID, current.WebhookID, webhookSecret, current.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM github_app_setups WHERE org_id=$1 AND id=$2`, org, current.ID); err != nil {
			return err
		}
		connectionID = c.ID
		return audit(ctx, tx, org, a.UserID, "github_app.setup_completed", current.ID, current.ID)
	})
	p = pendingSecrets{}
	switch {
	case errors.Is(err, errClaimed):
		return fail("already_connected")
	case err != nil:
		r := outcome(org, err)
		if r.Outcome == "failed" {
			return fail(r.Reason)
		}
		return r
	case connectionID == "":
		return Result{OrgID: org, Outcome: "expired"}
	}
	return Result{OrgID: org, Outcome: "connected", ConnectionID: connectionID}
}

func audit(ctx context.Context, tx pgx.Tx, org, actor, action, id, request string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,'{}')`, domain.NewID(), org, actor, action, id, request)
	return err
}

func (s *Service) Cleanup(ctx context.Context) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, key := range []string{"reforge.org_id", "reforge.user_id", "reforge.github_state_hash", "reforge.github_setup_id"} {
		if _, err := tx.Exec(ctx, `SELECT set_config($1,'',true)`, key); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM github_app_setups`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Run(ctx context.Context) {
	if err := s.Cleanup(ctx); err != nil {
		slog.WarnContext(ctx, "github app setup cleanup failed", "error", err)
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := s.Cleanup(ctx)
			heartbeat.Beat("github-app", time.Minute, err)
			if err != nil {
				slog.WarnContext(ctx, "github app setup cleanup failed", "error", err)
			}
		}
	}
}
