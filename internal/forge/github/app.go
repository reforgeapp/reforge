package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

type AppConfig struct {
	AppID          string
	InstallationID string
	PrivateKeyPEM  []byte `json:"-"`
}

func (a AppConfig) String() string       { return "GitHub App credentials [redacted]" }
func (a AppConfig) GoString() string     { return a.String() }
func (a AppConfig) LogValue() slog.Value { return slog.StringValue(a.String()) }

type installationAuth struct {
	mu                    sync.Mutex
	appID, installationID string
	key                   *rsa.PrivateKey
	token, actorID, slug  string
	expires               time.Time
}

func failure(kind, message string) error { return &domain.ProviderError{Kind: kind, Message: message} }
func transportFailure(method, message string) error {
	if method != "GET" && method != "HEAD" {
		return &domain.ProviderError{Kind: "uncertain", Message: message, Uncertain: true}
	}
	return failure("transient", message)
}
func positive(value string) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n > 0
}
func NewApp(ctx context.Context, cfg forge.Config, app AppConfig) (*Provider, error) {
	if !positive(app.AppID) || !positive(app.InstallationID) || len(app.PrivateKeyPEM) > 65536 {
		return nil, failure("configuration", "GitHub App IDs and private key are required")
	}
	key, err := ParseAppKey(app.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	cfg.Token = "app-bootstrap"
	p, err := New(cfg)
	if err != nil {
		return nil, err
	}
	p.config.Token = ""
	p.app = &installationAuth{appID: app.AppID, installationID: app.InstallationID, key: key}
	if _, err = p.installationToken(ctx); err != nil {
		return nil, err
	}
	return p, nil
}
func ParseAppKey(raw []byte) (*rsa.PrivateKey, error) {
	if len(raw) > 65536 {
		return nil, failure("configuration", "Invalid GitHub App private key")
	}
	block, rest := pem.Decode(raw)
	if block == nil || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, failure("configuration", "Invalid GitHub App private key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		parsed, e := x509.ParsePKCS8PrivateKey(block.Bytes)
		if e != nil {
			return nil, failure("configuration", "Invalid GitHub App private key")
		}
		key, _ = parsed.(*rsa.PrivateKey)
	}
	if key == nil || key.N.BitLen() < 2048 || key.Validate() != nil {
		return nil, failure("configuration", "GitHub App requires a valid RSA private key of at least 2048 bits")
	}
	return key, nil
}
func AppJWT(appID string, key *rsa.PrivateKey) (string, error) {
	return (&installationAuth{appID: appID, key: key}).jwt()
}
func (a *installationAuth) jwt() (string, error) {
	now := time.Now()
	payload, _ := json.Marshal(map[string]any{"iss": a.appID, "iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix()})
	value := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(value))
	signature, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", failure("auth", "GitHub App signing failed")
	}
	return value + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
func (p *Provider) installationToken(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	a := p.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if ctx.Err() != nil {
		return "", failure("transient", "GitHub installation authentication deadline exceeded")
	}
	if a.token != "" && time.Now().Add(time.Minute).Before(a.expires) {
		return a.token, nil
	}
	jwt, err := a.jwt()
	if err != nil {
		return "", err
	}
	read := func(method string, path []string, credential string, out any) error {
		status, headers, raw, err := p.requestToken(ctx, method, path, nil, nil, credential)
		if err != nil {
			return err
		}
		if status < 200 || status >= 300 {
			return responseError(status, headers)
		}
		return decode(raw, out)
	}
	var app struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	}
	if err = read("GET", []string{"app"}, jwt, &app); err != nil {
		return "", err
	}
	if strconv.FormatInt(app.ID, 10) != a.appID || app.Slug == "" || strings.ContainsAny(app.Slug, "/\\%?#") {
		return "", failure("identity", "Authenticated GitHub App identity changed")
	}
	var installation struct {
		ID          int64      `json:"id"`
		AppID       int64      `json:"app_id"`
		SuspendedAt *time.Time `json:"suspended_at"`
	}
	if err = read("GET", []string{"app", "installations", a.installationID}, jwt, &installation); err != nil {
		return "", err
	}
	if strconv.FormatInt(installation.ID, 10) != a.installationID || installation.AppID != app.ID || installation.SuspendedAt != nil {
		return "", failure("auth", "GitHub installation is unavailable or belongs to another App")
	}
	var value struct {
		Token   string    `json:"token"`
		Expires time.Time `json:"expires_at"`
	}
	if err = read("POST", []string{"app", "installations", a.installationID, "access_tokens"}, jwt, &value); err != nil {
		return "", err
	}
	if value.Token == "" || !value.Expires.After(time.Now().Add(time.Minute)) || value.Expires.After(time.Now().Add(time.Hour+time.Minute)) {
		return "", failure("auth", "GitHub returned invalid installation token expiry")
	}
	var bot struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Type  string `json:"type"`
	}
	if err = read("GET", []string{"users", app.Slug + "[bot]"}, value.Token, &bot); err != nil {
		return "", err
	}
	if bot.ID <= 0 || bot.Type != "Bot" || bot.Login != app.Slug+"[bot]" || a.actorID != "" && a.actorID != strconv.FormatInt(bot.ID, 10) {
		return "", failure("identity", "GitHub App bot identity is unavailable or changed")
	}
	a.token, a.expires, a.actorID, a.slug = value.Token, value.Expires, strconv.FormatInt(bot.ID, 10), app.Slug
	return a.token, nil
}
func (p *Provider) authenticatedBot(ctx context.Context) (string, error) {
	if p.app == nil {
		return "", failure("auth", "Authenticated GitHub App installation required for owned operations")
	}
	if _, err := p.installationToken(ctx); err != nil {
		return "", err
	}
	p.app.mu.Lock()
	defer p.app.mu.Unlock()
	if !positive(p.app.actorID) {
		return "", failure("identity", "Authenticated App bot identity missing")
	}
	return p.app.actorID, nil
}

func (p *Provider) invalidateToken(credential string) {
	if p.app == nil {
		return
	}
	p.app.mu.Lock()
	defer p.app.mu.Unlock()
	if p.app.token == credential {
		p.app.token = ""
		p.app.expires = time.Time{}
	}
}
