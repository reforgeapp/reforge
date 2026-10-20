package config

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/reforgeapp/reforge/pkg/maintenance/recipes"
)

type Config struct {
	RepairImages       map[string]string
	Address            string
	PublicURL          string
	DatabaseURL        string
	WebDir             string
	MigrationDir       string
	Development        bool
	FixtureAuth        bool
	Edition            string
	DocsURL            string
	EncryptionKey      string `json:"-"`
	EncryptionKeyID    string
	EncryptionKeys     map[string]string `json:"-"`
	KMSRegion          string
	KMSKeyARN          string
	KMSPreviousKeyARNs []string
	PolicyFile         string
	ArtifactDirectory  string
	OIDCIssuer         string
	OIDCClientID       string
	OIDCClientSecret   string `json:"-"`
	SMTPAddress        string
	SMTPUsername       string
	SMTPPassword       string `json:"-"`
	SMTPFrom           string
	SMTPSecurity       string
	BootstrapToken     string `json:"-"`
	BootstrapExpiresAt time.Time
	GitHubApp          GitHubAppFiles
	BuiltinRunnerToken string `json:"-"`
}

type GitHubAppFiles struct {
	AppID, Slug, ClientID                         string
	ClientSecretFile, PrivateKeyFile, WebhookFile string
}

func Load() (Config, error) {
	c := Config{
		Address:           value("REFORGE_ADDRESS", "127.0.0.1:8080"),
		PublicURL:         value("REFORGE_PUBLIC_URL", "http://127.0.0.1:8080"),
		DatabaseURL:       os.Getenv("REFORGE_DATABASE_URL"),
		WebDir:            value("REFORGE_WEB_DIR", "web/dist"),
		MigrationDir:      value("REFORGE_MIGRATION_DIR", "pkg/store/migrations"),
		Development:       os.Getenv("REFORGE_MODE") == "development",
		FixtureAuth:       os.Getenv("REFORGE_FIXTURE_AUTH") == "true",
		Edition:           value("REFORGE_EDITION", "self-hosted"),
		DocsURL:           value("REFORGE_DOCS_URL", "/docs/"),
		EncryptionKey:     os.Getenv("REFORGE_ENCRYPTION_KEY"),
		EncryptionKeyID:   value("REFORGE_ENCRYPTION_KEY_ID", "primary"),
		KMSRegion:         os.Getenv("REFORGE_KMS_REGION"),
		KMSKeyARN:         os.Getenv("REFORGE_KMS_KEY_ARN"),
		PolicyFile:        os.Getenv("REFORGE_POLICY_FILE"),
		ArtifactDirectory: value("REFORGE_ARTIFACT_DIRECTORY", "var/artifacts"),
		OIDCIssuer:        os.Getenv("REFORGE_OIDC_ISSUER"),
		OIDCClientID:      os.Getenv("REFORGE_OIDC_CLIENT_ID"),
		OIDCClientSecret:  os.Getenv("REFORGE_OIDC_CLIENT_SECRET"),
		SMTPAddress:       os.Getenv("REFORGE_SMTP_ADDRESS"),
		SMTPUsername:      os.Getenv("REFORGE_SMTP_USERNAME"),
		SMTPPassword:      os.Getenv("REFORGE_SMTP_PASSWORD"),
		SMTPFrom:          os.Getenv("REFORGE_SMTP_FROM"),
		SMTPSecurity:      os.Getenv("REFORGE_SMTP_SECURITY"),
		BootstrapToken:    os.Getenv("REFORGE_BOOTSTRAP_TOKEN"),
		GitHubApp: GitHubAppFiles{
			AppID: os.Getenv("REFORGE_GITHUB_APP_ID"), Slug: os.Getenv("REFORGE_GITHUB_APP_SLUG"), ClientID: os.Getenv("REFORGE_GITHUB_APP_CLIENT_ID"),
			ClientSecretFile: os.Getenv("REFORGE_GITHUB_APP_CLIENT_SECRET_FILE"), PrivateKeyFile: os.Getenv("REFORGE_GITHUB_APP_PRIVATE_KEY_FILE"), WebhookFile: os.Getenv("REFORGE_GITHUB_APP_WEBHOOK_SECRET_FILE"),
		},
	}
	if raw := os.Getenv("REFORGE_REPAIR_IMAGES"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.RepairImages); err != nil {
			return c, errors.New("REFORGE_REPAIR_IMAGES must map recipe names to sha256 image digests")
		}
	}
	if dir := os.Getenv("REFORGE_BUILTIN_RUNNER_DIR"); dir != "" {
		if err := c.loadBuiltinRunner(dir); err != nil {
			return c, err
		}
	}
	var pathErr error
	c.ArtifactDirectory, pathErr = filepath.Abs(c.ArtifactDirectory)
	if pathErr != nil {
		return c, errors.New("artifact directory is invalid")
	}
	if previous := os.Getenv("REFORGE_KMS_PREVIOUS_KEY_ARNS"); previous != "" {
		c.KMSPreviousKeyARNs = strings.Split(previous, ",")
	}
	if raw := os.Getenv("REFORGE_ENCRYPTION_KEYS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.EncryptionKeys); err != nil {
			return c, errors.New("REFORGE_ENCRYPTION_KEYS must be a key ID to base64 key JSON object")
		}
	}
	if c.EncryptionKeys == nil {
		c.EncryptionKeys = map[string]string{c.EncryptionKeyID: c.EncryptionKey}
	}
	if c.BootstrapToken != "" {
		var err error
		c.BootstrapExpiresAt, err = time.Parse(time.RFC3339, os.Getenv("REFORGE_BOOTSTRAP_EXPIRES_AT"))
		if err != nil {
			return c, errors.New("REFORGE_BOOTSTRAP_EXPIRES_AT must be RFC3339 when bootstrap is enabled")
		}
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	for name, digest := range c.RepairImages {
		body, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
		if !slices.Contains(recipes.Toolchains, name) || !strings.HasPrefix(digest, "sha256:") || err != nil || len(body) != 32 || strings.ToLower(digest) != digest {
			return errors.New("REFORGE_REPAIR_IMAGES requires supported recipe names and lowercase sha256 image digests")
		}
	}
	if c.DatabaseURL == "" {
		return errors.New("REFORGE_DATABASE_URL is required")
	}
	if c.Edition != "hosted" && c.Edition != "self-hosted" {
		return errors.New("invalid edition")
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return errors.New("public URL must be an origin")
	}
	if u.Scheme != "https" && !(c.Development && u.Scheme == "http" && loopback(u.Hostname())) {
		return errors.New("HTTPS public URL required outside loopback development")
	}
	if c.Development || c.FixtureAuth {
		host, _, err := net.SplitHostPort(c.Address)
		if err != nil || !loopback(host) || !loopback(u.Hostname()) || c.Edition == "hosted" {
			return errors.New("development requires self-hosted edition and loopback listen/public addresses")
		}
	}
	if c.FixtureAuth && !c.Development {
		return errors.New("fixture authentication requires explicit development mode")
	}
	if c.KMSRegion != "" || c.KMSKeyARN != "" {
		if c.Edition != "hosted" || c.KMSRegion == "" || c.KMSKeyARN == "" {
			return errors.New("KMS encryption requires the hosted edition, REFORGE_KMS_REGION and REFORGE_KMS_KEY_ARN")
		}
	} else {
		keys := c.EncryptionKeys
		if keys == nil {
			keys = map[string]string{"primary": c.EncryptionKey}
		}
		for id, encoded := range keys {
			key, err := base64.StdEncoding.DecodeString(encoded)
			if id == "" || err != nil || len(key) != 32 {
				return errors.New("encryption keys must be 32-byte base64 values")
			}
		}
		if len(keys) == 0 {
			return errors.New("encryption keyring is required")
		}
	}
	if !c.FixtureAuth && (c.OIDCIssuer == "" || c.OIDCClientID == "") {
		return errors.New("OIDC issuer and client ID required outside explicit fixture authentication")
	}
	g := c.GitHubApp
	set := 0
	for _, v := range []string{g.AppID, g.Slug, g.ClientID, g.ClientSecretFile, g.PrivateKeyFile, g.WebhookFile} {
		if v != "" {
			set++
		}
	}
	if set != 0 && (set != 6 || c.Edition != "hosted") {
		return errors.New("REFORGE_GITHUB_APP_ID, _SLUG, _CLIENT_ID, _CLIENT_SECRET_FILE, _PRIVATE_KEY_FILE and _WEBHOOK_SECRET_FILE must all be set, for the hosted edition only")
	}
	if c.BuiltinRunnerToken != "" && (c.Edition != "self-hosted" || len(c.BuiltinRunnerToken) < 43) {
		return errors.New("the built-in runner requires the self-hosted edition and a 32-byte token")
	}
	if c.BootstrapToken != "" && (c.Edition != "self-hosted" || c.BootstrapExpiresAt.IsZero() || len(c.BootstrapToken) < 32) {
		return errors.New("bootstrap requires self-hosted edition, an explicit expiry and at least 32 random token characters")
	}
	return nil
}

func (c *Config) loadBuiltinRunner(dir string) error {
	read := func(name string) ([]byte, error) {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0027 != 0 || info.Size() > 4096 {
			return nil, errors.New("REFORGE_BUILTIN_RUNNER_DIR requires private regular files " + name)
		}
		return os.ReadFile(path)
	}
	token, err := read("token")
	if err != nil {
		return err
	}
	c.BuiltinRunnerToken = strings.TrimSpace(string(token))
	if len(c.RepairImages) == 0 {
		images, err := read("repair-images.json")
		if err != nil {
			return err
		}
		if json.Unmarshal(images, &c.RepairImages) != nil {
			return errors.New("built-in runner repair-images.json is invalid")
		}
	}
	return nil
}

func loopback(host string) bool { ip := net.ParseIP(host); return ip != nil && ip.IsLoopback() }
func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
