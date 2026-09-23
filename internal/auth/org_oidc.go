package auth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/domain"
	"reforge/internal/network"
	"reforge/internal/secrets"
)

var (
	ErrOIDCProbe         = errors.New("OIDC issuer could not be verified")
	ErrOIDCNotReady      = errors.New("organisation OIDC login is not available until org-aware callback support is implemented")
	ErrOIDCUnavailable   = errors.New("OIDC secret storage unavailable")
	ErrOIDCProbeCooldown = errors.New("OIDC issuer probe cooldown active")
	ErrOIDCProbeBusy     = errors.New("OIDC issuer probe capacity reached")
)

type ProbeCooldownError struct {
	RetryAfter int
}

func (e *ProbeCooldownError) Error() string        { return ErrOIDCProbeCooldown.Error() }
func (e *ProbeCooldownError) Is(target error) bool { return target == ErrOIDCProbeCooldown }

var orgOIDCProbeSlots = make(chan struct{}, 4)

func acquireOrgOIDCProbe() bool {
	select {
	case orgOIDCProbeSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseOrgOIDCProbe() { <-orgOIDCProbeSlots }

type OrgOIDCSettings struct {
	Configured          bool       `json:"configured"`
	SecretPresent       bool       `json:"secret_present"`
	Issuer              string     `json:"issuer,omitempty"`
	ClientID            string     `json:"client_id,omitempty"`
	Status              string     `json:"status"`
	Version             int64      `json:"version"`
	VerifiedAt          *time.Time `json:"verified_at,omitempty"`
	Verified            bool       `json:"verified"`
	ActivationAvailable bool       `json:"activation_available"`
	ActivationBlocked   string     `json:"activation_blocked,omitempty"`
}

type OrgOIDCInput struct {
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type orgOIDCRecord struct {
	ID              string
	Issuer          string
	ClientID        string
	Status          string
	Version         int64
	VerifiedVersion int64
	VerifiedAt      *time.Time
	SecretVersion   int64
	SecretEnvelope  []byte
	ProbeAttemptAt  *time.Time
}

func (s *Service) SetOrgOIDCVault(vault *secrets.Vault) { s.orgOIDCVault = vault }

func (s *Service) OrgOIDC(ctx context.Context, session Session, orgID string) (OrgOIDCSettings, error) {
	out := emptyOrgOIDCSettings()
	err := s.WithActor(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		record, err := loadOrgOIDC(ctx, tx, orgID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		out = settingsFor(record)
		return nil
	})
	return out, err
}

func (s *Service) PutOrgOIDC(ctx context.Context, session Session, orgID string, input OrgOIDCInput, expected int64, requestID string) (OrgOIDCSettings, error) {
	if s.orgOIDCVault == nil || expected < 0 || len(input.ClientSecret) > 65536 || len(input.ClientID) == 0 || len(input.ClientID) > 512 || strings.TrimSpace(input.ClientID) != input.ClientID || strings.ContainsAny(input.ClientID, "\r\n\x00") {
		return OrgOIDCSettings{}, ErrInvalid
	}
	issuer, err := normalizeIssuer(input.Issuer, s.cfg.Development)
	if err != nil {
		return OrgOIDCSettings{}, ErrInvalid
	}
	input.Issuer = issuer
	out := OrgOIDCSettings{}
	err = s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		current, loadErr := loadOrgOIDC(ctx, tx, orgID)
		if loadErr != nil && !errors.Is(loadErr, pgx.ErrNoRows) {
			return loadErr
		}
		created := errors.Is(loadErr, pgx.ErrNoRows)
		if (created && expected != 0) || (!created && current.Version != expected) {
			return ErrConflict
		}
		if created && input.ClientSecret == "" {
			return ErrInvalid
		}
		configID := string(domain.NewID())
		version := int64(1)
		secret := []byte(input.ClientSecret)
		if !created {
			configID = current.ID
			version = current.Version + 1
			if len(secret) == 0 {
				var envelope secrets.Envelope
				if err := json.Unmarshal(current.SecretEnvelope, &envelope); err != nil {
					return ErrOIDCUnavailable
				}
				secret, err = s.orgOIDCVault.OpenContext(ctx, secrets.Binding{OrgID: orgID, ConnectionID: current.ID, Version: current.SecretVersion}, envelope)
				if err != nil {
					return ErrOIDCUnavailable
				}
			}
		}
		defer clear(secret)
		binding := secrets.Binding{OrgID: orgID, ConnectionID: configID, Version: version}
		envelope, err := s.orgOIDCVault.SealContext(ctx, binding, secret)
		if err != nil {
			return ErrOIDCUnavailable
		}
		envelopeJSON, err := json.Marshal(envelope)
		if err != nil {
			return ErrOIDCUnavailable
		}
		var saved orgOIDCRecord
		if created {
			err = tx.QueryRow(ctx, `INSERT INTO org_oidc_configs(org_id,id,issuer,client_id,status,version) VALUES($1,$2,$3,$4,'draft',1) RETURNING id::text,issuer,client_id,status,version,coalesce(verified_version,0),verified_at`, orgID, configID, input.Issuer, input.ClientID).Scan(&saved.ID, &saved.Issuer, &saved.ClientID, &saved.Status, &saved.Version, &saved.VerifiedVersion, &saved.VerifiedAt)
		} else {
			err = tx.QueryRow(ctx, `UPDATE org_oidc_configs SET issuer=$3,client_id=$4,status='draft',version=$5,verified_version=NULL,verified_at=NULL,updated_at=now() WHERE org_id=$1 AND id=$2 RETURNING id::text,issuer,client_id,status,version,coalesce(verified_version,0),verified_at`, orgID, configID, input.Issuer, input.ClientID, version).Scan(&saved.ID, &saved.Issuer, &saved.ClientID, &saved.Status, &saved.Version, &saved.VerifiedVersion, &saved.VerifiedAt)
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO org_oidc_secrets(org_id,config_id,version,envelope) VALUES($1,$2,$3,$4) ON CONFLICT(org_id,config_id) DO UPDATE SET version=EXCLUDED.version,envelope=EXCLUDED.envelope,updated_at=now()`, orgID, configID, version, envelopeJSON)
		if err != nil {
			return err
		}
		saved.SecretVersion = version
		saved.SecretEnvelope = envelopeJSON
		if err = audit(ctx, tx, orgID, string(actor.UserID), "org.oidc.configured", configID, requestID, saved.Version); err != nil {
			return err
		}
		out = settingsFor(saved)
		return nil
	})
	input.ClientSecret = ""
	return out, err
}

func (s *Service) ProbeOrgOIDC(ctx context.Context, session Session, orgID string, expected int64, requestID string) (OrgOIDCSettings, error) {
	if !acquireOrgOIDCProbe() {
		return OrgOIDCSettings{}, ErrOIDCProbeBusy
	}
	defer releaseOrgOIDCProbe()
	var record orgOIDCRecord
	err := s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		loaded, loadErr := loadOrgOIDC(ctx, tx, orgID)
		record = loaded
		if errors.Is(loadErr, pgx.ErrNoRows) {
			return ErrInvalid
		}
		if loadErr != nil {
			return loadErr
		}
		if record.Version != expected {
			return ErrConflict
		}
		if record.ProbeAttemptAt != nil {
			var retryAfter int
			if err := tx.QueryRow(ctx, `SELECT ceil(extract(epoch FROM ($1::timestamptz + interval '30 seconds') - now()))::int`, record.ProbeAttemptAt).Scan(&retryAfter); err != nil {
				return err
			}
			if retryAfter > 0 {
				return &ProbeCooldownError{RetryAfter: retryAfter}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE org_oidc_configs SET probe_attempt_at=now(),verified_version=NULL,verified_at=NULL,updated_at=now() WHERE org_id=$1`, orgID); err != nil {
			return err
		}
		if err := audit(ctx, tx, orgID, string(actor.UserID), "org.oidc.probe_started", record.ID, requestID, record.Version); err != nil {
			return err
		}
		loaded, loadErr = loadOrgOIDC(ctx, tx, orgID)
		record = loaded
		return loadErr
	})
	if errors.Is(err, ErrOIDCProbeCooldown) {
		return OrgOIDCSettings{}, err
	}
	if err != nil {
		return OrgOIDCSettings{}, err
	}
	if _, err = fetchOIDCMetadata(ctx, record.Issuer, s.cfg.Development); err != nil {
		return OrgOIDCSettings{}, ErrOIDCProbe
	}
	out := OrgOIDCSettings{}
	err = s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		current, err := loadOrgOIDC(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if current.Version != expected || current.ID != record.ID {
			return ErrConflict
		}
		_, err = tx.Exec(ctx, `UPDATE org_oidc_configs SET verified_version=version,verified_at=now(),updated_at=now() WHERE org_id=$1`, orgID)
		if err != nil {
			return err
		}
		current, err = loadOrgOIDC(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if err = audit(ctx, tx, orgID, string(actor.UserID), "org.oidc.probed", current.ID, requestID, current.Version); err != nil {
			return err
		}
		out = settingsFor(current)
		return nil
	})
	return out, err
}

func (s *Service) ActivateOrgOIDC(ctx context.Context, session Session, orgID string, expected int64, requestID string) error {
	err := s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		record, err := loadOrgOIDC(ctx, tx, orgID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalid
		}
		if err != nil {
			return err
		}
		if record.Version != expected {
			return ErrConflict
		}
		if record.VerifiedVersion != record.Version || record.Status == "disabled" {
			return ErrInvalid
		}
		return audit(ctx, tx, orgID, string(actor.UserID), "org.oidc.activation_blocked", record.ID, requestID, record.Version)
	})
	if err != nil {
		return err
	}
	return ErrOIDCNotReady
}

func (s *Service) DisableOrgOIDC(ctx context.Context, session Session, orgID string, expected int64, requestID string) (OrgOIDCSettings, error) {
	out := OrgOIDCSettings{}
	err := s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return ErrForbidden
		}
		record, err := loadOrgOIDC(ctx, tx, orgID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalid
		}
		if err != nil {
			return err
		}
		if record.Version != expected {
			return ErrConflict
		}
		_, err = tx.Exec(ctx, `UPDATE org_oidc_configs SET status='disabled',version=version+1,verified_version=NULL,verified_at=NULL,updated_at=now() WHERE org_id=$1`, orgID)
		if err != nil {
			return err
		}
		record, err = loadOrgOIDC(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if err = audit(ctx, tx, orgID, string(actor.UserID), "org.oidc.disabled", record.ID, requestID, record.Version); err != nil {
			return err
		}
		out = settingsFor(record)
		return nil
	})
	return out, err
}

func emptyOrgOIDCSettings() OrgOIDCSettings {
	return OrgOIDCSettings{Status: "unconfigured", ActivationAvailable: false, ActivationBlocked: ErrOIDCNotReady.Error()}
}

func settingsFor(record orgOIDCRecord) OrgOIDCSettings {
	status := record.Status
	verified := record.VerifiedVersion == record.Version && record.VerifiedAt != nil
	if status != "disabled" && verified {
		status = "probe_verified"
	}
	return OrgOIDCSettings{Configured: true, SecretPresent: true, Issuer: record.Issuer, ClientID: record.ClientID, Status: status, Version: record.Version, VerifiedAt: record.VerifiedAt, Verified: verified, ActivationAvailable: false, ActivationBlocked: ErrOIDCNotReady.Error()}
}

func loadOrgOIDC(ctx context.Context, tx pgx.Tx, orgID string) (orgOIDCRecord, error) {
	var record orgOIDCRecord
	err := tx.QueryRow(ctx, `SELECT c.id::text,c.issuer,c.client_id,c.status,c.version,coalesce(c.verified_version,0),c.verified_at,s.version,s.envelope,c.probe_attempt_at FROM org_oidc_configs c JOIN org_oidc_secrets s ON s.org_id=c.org_id AND s.config_id=c.id WHERE c.org_id=$1`, orgID).Scan(&record.ID, &record.Issuer, &record.ClientID, &record.Status, &record.Version, &record.VerifiedVersion, &record.VerifiedAt, &record.SecretVersion, &record.SecretEnvelope, &record.ProbeAttemptAt)
	return record, err
}

type oidcMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

func normalizeIssuer(raw string, development bool) (string, error) {
	if len(raw) == 0 || len(raw) > 2048 || strings.TrimSpace(raw) != raw {
		return "", ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Hostname() == "" || strings.Contains(u.Host, "%") {
		return "", ErrInvalid
	}
	if u.Scheme != "https" && !(development && u.Scheme == "http" && loopbackIssuerHost(u.Hostname())) {
		return "", ErrInvalid
	}
	if u.Scheme == "https" && u.Port() != "" && u.Port() != "443" {
		return "", ErrInvalid
	}
	if u.Scheme == "http" && (!development || !loopbackIssuerHost(u.Hostname())) {
		return "", ErrInvalid
	}
	if _, safe := safeOIDCPath(u.EscapedPath()); !safe || u.RawPath != "" {
		return "", ErrInvalid
	}
	return u.String(), nil
}

func loopbackIssuerHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func validateResolvedHost(ctx context.Context, host string, development bool) ([]netip.Addr, error) {
	if development && loopbackIssuerHost(host) {
		if ip, err := netip.ParseAddr(host); err == nil {
			return []netip.Addr{ip}, nil
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("::1")}, nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !network.IsPublicIP(ip) {
			return nil, ErrInvalid
		}
		return []netip.Addr{ip}, nil
	}
	if strings.HasSuffix(strings.ToLower(host), ".localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return nil, ErrInvalid
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, ErrInvalid
	}
	for _, ip := range ips {
		if !network.IsPublicIP(ip) {
			return nil, ErrInvalid
		}
	}
	return ips, nil
}

func issuerHTTPClient(ctx context.Context, issuer *url.URL, development bool) (*http.Client, error) {
	if _, err := validateResolvedHost(ctx, issuer.Hostname(), development); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 20 * time.Second}
	expectedPort := issuer.Port()
	if expectedPort == "" {
		if issuer.Scheme == "https" {
			expectedPort = "443"
		} else {
			expectedPort = "80"
		}
	}
	base := &http.Transport{
		Proxy:                  nil,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    3 * time.Second,
		ResponseHeaderTimeout:  4 * time.Second,
		MaxResponseHeaderBytes: 32 << 10,
		MaxConnsPerHost:        4,
		MaxIdleConnsPerHost:    2,
		IdleConnTimeout:        15 * time.Second,
		ForceAttemptHTTP2:      true,
		DialContext: func(ctx context.Context, transportNetwork, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || !strings.EqualFold(host, issuer.Hostname()) || port != expectedPort {
				return nil, ErrInvalid
			}
			ips, err := validateResolvedHost(ctx, host, development)
			if err != nil {
				return nil, err
			}
			var last error
			for _, ip := range ips {
				conn, dialErr := dialer.DialContext(ctx, transportNetwork, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				last = dialErr
			}
			return nil, last
		},
	}
	guard := &issuerRoundTripper{base: base, issuer: issuer, path: strings.TrimRight(issuer.Path, "/")}
	return &http.Client{Transport: guard, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

type issuerRoundTripper struct {
	base   *http.Transport
	issuer *url.URL
	path   string
}

func (t *issuerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, ErrInvalid
	}
	u := request.URL
	if u.User != nil || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || u.ForceQuery || u.RawQuery != "" || !strings.EqualFold(u.Scheme, t.issuer.Scheme) || !strings.EqualFold(u.Host, t.issuer.Host) || request.Host != "" && !strings.EqualFold(request.Host, t.issuer.Host) {
		return nil, ErrInvalid
	}
	requestPath, ok := safeOIDCPath(u.EscapedPath())
	if !ok || t.path != "" && requestPath != t.path && !strings.HasPrefix(requestPath, t.path+"/") {
		return nil, ErrInvalid
	}
	switch request.Method {
	case http.MethodGet, http.MethodPost, http.MethodHead:
	default:
		return nil, ErrInvalid
	}
	return t.base.RoundTrip(request)
}

func safeOIDCPath(raw string) (string, bool) {
	if raw == "" {
		return "/", true
	}
	for attempt := 0; attempt < 8; attempt++ {
		if !strings.HasPrefix(raw, "/") || strings.Contains(raw, "//") || strings.ContainsAny(raw, "\\;\x00\r\n") {
			return "", false
		}
		for _, segment := range strings.Split(raw, "/") {
			if segment == "." || segment == ".." {
				return "", false
			}
		}
		if !strings.Contains(raw, "%") {
			return raw, true
		}
		decoded, err := url.PathUnescape(raw)
		if err != nil {
			return "", false
		}
		raw = decoded
	}
	return "", false
}

func endpointInsideIssuer(endpoint string, issuer *url.URL) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.EqualFold(u.Scheme, issuer.Scheme) || !strings.EqualFold(u.Host, issuer.Host) || strings.Contains(u.Path, "\\") {
		return false
	}
	for _, segment := range strings.Split(u.EscapedPath(), "/") {
		decoded, decodeErr := url.PathUnescape(segment)
		if decodeErr != nil || decoded == "." || decoded == ".." || strings.Contains(decoded, "/") || strings.Contains(decoded, "\\") {
			return false
		}
	}
	base, baseOK := safeOIDCPath(issuer.EscapedPath())
	path, pathOK := safeOIDCPath(u.EscapedPath())
	if !baseOK || !pathOK {
		return false
	}
	base = strings.TrimRight(base, "/")
	return base == "" || path == base || strings.HasPrefix(path, base+"/")
}

func fetchOIDCMetadata(ctx context.Context, rawIssuer string, development bool) (oidcMetadata, error) {
	issuerValue, err := normalizeIssuer(rawIssuer, development)
	if err != nil {
		return oidcMetadata{}, err
	}
	issuer, _ := url.Parse(issuerValue)
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	client, err := issuerHTTPClient(requestCtx, issuer, development)
	if err != nil {
		return oidcMetadata{}, err
	}
	metadataURL := strings.TrimRight(issuerValue, "/") + "/.well-known/openid-configuration"
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return oidcMetadata{}, ErrInvalid
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return oidcMetadata{}, ErrOIDCProbe
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return oidcMetadata{}, ErrOIDCProbe
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		return oidcMetadata{}, ErrOIDCProbe
	}
	var metadata oidcMetadata
	if err := json.Unmarshal(body, &metadata); err != nil || metadata.Issuer != issuerValue {
		return oidcMetadata{}, ErrOIDCProbe
	}
	for _, endpoint := range []string{metadata.AuthorizationEndpoint, metadata.TokenEndpoint, metadata.JWKSURI} {
		if !endpointInsideIssuer(endpoint, issuer) {
			return oidcMetadata{}, ErrOIDCProbe
		}
	}
	return metadata, nil
}
