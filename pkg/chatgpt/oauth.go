package chatgpt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const Issuer = "https://auth.openai.com"
const Resource = "https://api.openai.com/v1"
const Profile = "chatgpt_plan"
const LocalRedirect = "http://127.0.0.1:1456/auth/callback"

var ErrUnavailable = errors.New("ChatGPT plan connection is not configured")
var ErrAuthorization = errors.New("ChatGPT authorization could not be verified; start sign-in again")
var ErrPermission = errors.New("ChatGPT plan usage was not authorized; enable plan usage and reconnect")
var ErrRefresh = errors.New("ChatGPT session expired or revoked; reconnect this account")

type Config struct {
	ClientID         string
	ClientSecret     string `json:"-"`
	RedirectURL      string
	PlanUsageEnabled bool
	SelfHosted       bool
	HostID           string
}

func (c Config) String() string       { return "ChatGPT OAuth configuration [redacted]" }
func (c Config) GoString() string     { return c.String() }
func (c Config) LogValue() slog.Value { return slog.StringValue(c.String()) }

func (c Config) Available() bool {
	u, err := url.Parse(c.RedirectURL)
	if !c.PlanUsageEnabled || err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if c.SelfHosted {
		return c.ClientID == "" && c.ClientSecret == "" && hostIDPattern.MatchString(c.HostID) && c.RedirectURL == LocalRedirect
	}
	return issuedClientID(c.ClientID) && u.Scheme == "https" && u.Host != "" && u.Path == "/auth/openai/callback"
}

var hostIDPattern = regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func issuedClientID(value string) bool {
	return strings.HasPrefix(value, "oaiapp_") && len(value) > len("oaiapp_") && len(value) <= 512 && !strings.ContainsAny(value, " \t\r\n\x00")
}

func (c Config) Accepts(credentials Credentials) bool {
	return c.Available() && credentials.Valid(credentials.ClientID) && (c.SelfHosted && credentials.HostID == c.HostID || !c.SelfHosted && credentials.ClientID == c.ClientID)
}

type Credentials struct {
	ClientID     string    `json:"client_id"`
	Subject      string    `json:"subject"`
	Email        string    `json:"email"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	Scopes       string    `json:"scopes"`
	Expiry       time.Time `json:"expiry"`
	HostID       string    `json:"ext_agent_host_id,omitempty"`
}

func (c Credentials) String() string       { return "ChatGPT credentials [redacted]" }
func (c Credentials) GoString() string     { return c.String() }
func (c Credentials) LogValue() slog.Value { return slog.StringValue(c.String()) }

type Client struct {
	config     Config
	provider   *oidc.Provider
	oauth      oauth2.Config
	http       *http.Client
	revocation string
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

func New(ctx context.Context, config Config, client *http.Client) (*Client, error) {
	if !config.Available() || client == nil {
		return nil, ErrUnavailable
	}
	clone := *client
	clone.Timeout = 10 * time.Second
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, &clone), Issuer)
	if err != nil {
		return nil, ErrUnavailable
	}
	var metadata struct {
		JWKS       string `json:"jwks_uri"`
		Revocation string `json:"revocation_endpoint"`
	}
	if provider.Claims(&metadata) != nil {
		return nil, ErrUnavailable
	}
	endpoint := provider.Endpoint()
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL, metadata.JWKS, metadata.Revocation} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host != "auth.openai.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, ErrUnavailable
		}
	}
	endpoint.AuthStyle = oauth2.AuthStyleInParams
	if config.ClientSecret != "" {
		endpoint.AuthStyle = oauth2.AuthStyleInHeader
	}
	clientID := config.ClientID
	if config.SelfHosted {
		clientID = "dynamic_agent_client"
	}
	return &Client{config: config, provider: provider, http: &clone, revocation: metadata.Revocation, oauth: oauth2.Config{ClientID: clientID, ClientSecret: config.ClientSecret, RedirectURL: config.RedirectURL, Endpoint: endpoint, Scopes: []string{"openid", "profile", "email", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"}}}, nil
}

func Random() string {
	var raw [32]byte
	_, _ = rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func Digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (c *Client) AuthorizationURL(state, nonce, verifier string, account ...string) string {
	config := c.oauth
	options := []oauth2.AuthCodeOption{oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("resource", Resource)}
	if c.config.SelfHosted {
		options = append(options, oauth2.SetAuthURLParam("ext_agent_host_id", c.config.HostID))
		if len(account) == 1 && issuedClientID(account[0]) {
			config.ClientID = account[0]
		} else {
			options = append(options, oauth2.SetAuthURLParam("agent_name_hint", "Reforge"))
		}
	}
	return config.AuthCodeURL(state, options...)
}

func (c *Client) Exchange(ctx context.Context, code, nonce, verifier string) (Credentials, error) {
	return c.ExchangeRegistration(ctx, code, nonce, verifier, c.config.ClientID, "")
}

func (c *Client) ExchangeRegistration(ctx context.Context, code, nonce, verifier, expected, issued string) (Credentials, error) {
	config := c.oauth
	clientID, err := c.RegistrationID(expected, issued)
	if err != nil {
		return Credentials{}, err
	}
	config.ClientID = clientID
	token, err := config.Exchange(context.WithValue(ctx, oauth2.HTTPClient, c.http), code, oauth2.VerifierOption(verifier), oauth2.SetAuthURLParam("resource", Resource))
	if err != nil {
		return Credentials{}, ErrAuthorization
	}
	raw, _ := token.Extra("id_token").(string)
	id, err := c.provider.Verifier(&oidc.Config{ClientID: config.ClientID, SupportedSigningAlgs: []string{"RS256"}}).Verify(oidc.ClientContext(ctx, c.http), raw)
	if err != nil || id.Nonce != nonce || id.Subject == "" {
		return Credentials{}, ErrAuthorization
	}
	var claims struct {
		Email string `json:"email"`
	}
	if id.Claims(&claims) != nil {
		return Credentials{}, ErrAuthorization
	}
	scopes, _ := token.Extra("scope").(string)
	credentials := Credentials{ClientID: config.ClientID, Subject: id.Subject, Email: claims.Email, AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, IDToken: raw, Scopes: scopes, Expiry: token.Expiry, HostID: c.config.HostID}
	if !strings.EqualFold(token.TokenType, "Bearer") || !c.config.Accepts(credentials) {
		return Credentials{}, ErrPermission
	}
	return credentials, nil
}

func (c *Client) RegistrationID(expected, issued string) (string, error) {
	if c.config.SelfHosted {
		if expected != "" && (!issuedClientID(expected) || issued != "" && issued != expected) {
			return "", ErrAuthorization
		}
		if expected == "" {
			expected = issued
		}
		if !issuedClientID(expected) {
			return "", ErrAuthorization
		}
		return expected, nil
	}
	if issued != "" && issued != c.config.ClientID || expected != "" && expected != c.config.ClientID {
		return "", ErrAuthorization
	}
	return c.config.ClientID, nil
}

func (c Credentials) Valid(clientID string) bool {
	return issuedClientID(c.ClientID) && c.ClientID == clientID && c.Subject != "" && c.AccessToken != "" && c.RefreshToken != "" && !c.Expiry.IsZero() && slices.Contains(strings.Fields(c.Scopes), "chatgpt.tokens.use.direct") && slices.Contains(strings.Fields(c.Scopes), "resource.invoke")
}

func (c *Client) Refresh(ctx context.Context, current Credentials) (Credentials, error) {
	if !c.config.Accepts(current) {
		return Credentials{}, ErrRefresh
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauth.Endpoint.TokenURL, strings.NewReader(url.Values{"grant_type": {"refresh_token"}, "client_id": {current.ClientID}, "refresh_token": {current.RefreshToken}, "resource": {Resource}}.Encode()))
	if err != nil {
		return Credentials{}, ErrUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.config.ClientSecret != "" {
		request.SetBasicAuth(url.QueryEscape(c.config.ClientID), url.QueryEscape(c.config.ClientSecret))
	}
	response, err := c.http.Do(request)
	if err != nil {
		return Credentials{}, ErrUnavailable
	}
	defer response.Body.Close()
	next, err := refreshResponse(response, current)
	if err != nil {
		return Credentials{}, err
	}
	if next.IDToken != current.IDToken {
		id, err := c.provider.Verifier(&oidc.Config{ClientID: current.ClientID, SupportedSigningAlgs: []string{"RS256"}}).Verify(oidc.ClientContext(ctx, c.http), next.IDToken)
		if err != nil || id.Subject != current.Subject {
			return Credentials{}, ErrRefresh
		}
	}
	return next, nil
}

func refreshResponse(response *http.Response, current Credentials) (Credentials, error) {
	var payload struct {
		AccessToken  string  `json:"access_token"`
		RefreshToken string  `json:"refresh_token"`
		IDToken      string  `json:"id_token"`
		TokenType    string  `json:"token_type"`
		ExpiresIn    int64   `json:"expires_in"`
		Scope        *string `json:"scope"`
		Error        string  `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 128<<10)).Decode(&payload) != nil {
		return Credentials{}, ErrUnavailable
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if slices.Contains([]string{"invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused"}, payload.Error) {
			return Credentials{}, ErrRefresh
		}
		return Credentials{}, ErrUnavailable
	}
	if payload.AccessToken == "" || !strings.EqualFold(payload.TokenType, "Bearer") || payload.ExpiresIn <= 0 || payload.ExpiresIn > 86400 {
		return Credentials{}, ErrUnavailable
	}
	current.AccessToken = payload.AccessToken
	if payload.RefreshToken != "" {
		current.RefreshToken = payload.RefreshToken
	}
	if payload.IDToken != "" {
		current.IDToken = payload.IDToken
	}
	if payload.Scope != nil {
		current.Scopes = *payload.Scope
	}
	current.Expiry = time.Now().UTC().Add(time.Duration(payload.ExpiresIn) * time.Second)
	if !current.Valid(current.ClientID) {
		return Credentials{}, ErrPermission
	}
	return current, nil
}

func (c *Client) Revoke(ctx context.Context, current Credentials) error {
	if !issuedClientID(current.ClientID) || current.RefreshToken == "" || c.config.SelfHosted && current.HostID != c.config.HostID || !c.config.SelfHosted && current.ClientID != c.config.ClientID {
		return ErrRefresh
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.revocation, strings.NewReader(url.Values{"token": {current.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {current.ClientID}}.Encode()))
	if err != nil {
		return ErrUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.config.ClientSecret != "" {
		request.SetBasicAuth(url.QueryEscape(c.config.ClientID), url.QueryEscape(c.config.ClientSecret))
	}
	response, err := c.http.Do(request)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	return nil
}
