package githubapp

import (
	"crypto/rsa"
	"errors"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

	"reforge/internal/config"
	"reforge/internal/forge/github"
)

type Hosted struct {
	AppID         int64
	Slug          string
	ClientID      string
	ClientSecret  []byte `json:"-"`
	PrivateKeyPEM []byte `json:"-"`
	WebhookSecret []byte `json:"-"`
	key           *rsa.PrivateKey
}

func (h Hosted) String() string       { return "GitHub hosted App [redacted]" }
func (h Hosted) GoString() string     { return h.String() }
func (h Hosted) LogValue() slog.Value { return slog.StringValue(h.String()) }

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var clientPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

func LoadHosted(f config.GitHubAppFiles) (*Hosted, error) {
	if f == (config.GitHubAppFiles{}) {
		return nil, nil
	}
	if f.AppID == "" || f.Slug == "" || f.ClientID == "" || f.ClientSecretFile == "" || f.PrivateKeyFile == "" || f.WebhookFile == "" {
		return nil, errors.New("REFORGE_GITHUB_APP_* settings must be supplied together")
	}
	read := func(path string) ([]byte, error) {
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) > 65536 {
			return nil, errors.New("GitHub App secret file unreadable")
		}
		return raw, nil
	}
	id, err := strconv.ParseInt(f.AppID, 10, 64)
	if err != nil || id <= 0 || !slugPattern.MatchString(f.Slug) || !clientPattern.MatchString(f.ClientID) {
		return nil, errors.New("REFORGE_GITHUB_APP_ID, _SLUG and _CLIENT_ID are invalid")
	}
	h := &Hosted{AppID: id, Slug: f.Slug, ClientID: f.ClientID}
	secret, err := read(f.ClientSecretFile)
	if err != nil {
		return nil, err
	}
	h.ClientSecret = []byte(strings.TrimSpace(string(secret)))
	if h.PrivateKeyPEM, err = read(f.PrivateKeyFile); err != nil {
		return nil, err
	}
	webhook, err := read(f.WebhookFile)
	if err != nil {
		return nil, err
	}
	h.WebhookSecret = []byte(strings.TrimSpace(string(webhook)))
	if len(h.ClientSecret) < 16 || len(h.WebhookSecret) < 16 {
		return nil, errors.New("GitHub App client and webhook secrets must be at least 16 characters")
	}
	if h.key, err = github.ParseAppKey(h.PrivateKeyPEM); err != nil {
		return nil, errors.New("REFORGE_GITHUB_APP_PRIVATE_KEY_FILE must contain an RSA private key of at least 2048 bits")
	}
	return h, nil
}
