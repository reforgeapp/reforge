package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	Address            string
	PublicURL          string
	DatabaseURL        string
	WebDir             string
	MigrationDir       string
	Development        bool
	FixtureAuth        bool
	Edition            string
	EncryptionKey      string `json:"-"`
	OIDCIssuer         string
	OIDCClientID       string
	OIDCClientSecret   string `json:"-"`
	BootstrapToken     string `json:"-"`
	BootstrapExpiresAt time.Time
}

func Load() (Config, error) {
	c := Config{
		Address:          value("REFORGE_ADDRESS", "127.0.0.1:8080"),
		PublicURL:        value("REFORGE_PUBLIC_URL", "http://127.0.0.1:8080"),
		DatabaseURL:      os.Getenv("REFORGE_DATABASE_URL"),
		WebDir:           value("REFORGE_WEB_DIR", "web/dist"),
		MigrationDir:     value("REFORGE_MIGRATION_DIR", "internal/store/migrations"),
		Development:      os.Getenv("REFORGE_MODE") == "development",
		FixtureAuth:      os.Getenv("REFORGE_FIXTURE_AUTH") == "true",
		Edition:          value("REFORGE_EDITION", "self-hosted"),
		EncryptionKey:    os.Getenv("REFORGE_ENCRYPTION_KEY"),
		OIDCIssuer:       os.Getenv("REFORGE_OIDC_ISSUER"),
		OIDCClientID:     os.Getenv("REFORGE_OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("REFORGE_OIDC_CLIENT_SECRET"),
		BootstrapToken:   os.Getenv("REFORGE_BOOTSTRAP_TOKEN"),
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
	if strings.TrimSpace(c.EncryptionKey) == "" {
		return errors.New("REFORGE_ENCRYPTION_KEY is required")
	}
	if !c.FixtureAuth && (c.OIDCIssuer == "" || c.OIDCClientID == "") {
		return errors.New("OIDC issuer and client ID required outside explicit fixture authentication")
	}
	if c.BootstrapToken != "" && (c.Edition != "self-hosted" || c.BootstrapExpiresAt.IsZero() || len(c.BootstrapToken) < 32) {
		return errors.New("bootstrap requires self-hosted edition, an explicit expiry and at least 32 random token characters")
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
