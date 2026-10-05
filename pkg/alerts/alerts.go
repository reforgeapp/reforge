package alerts

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/mail"
	"net/smtp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/secrets"
	"github.com/reforgeapp/reforge/pkg/store"
)

var ErrNotConfigured = errors.New("server email is not configured")

type SMTP struct {
	Address, Username, Password, From, Security, ServerName string
	SkipVerify                                              bool
}

type Settings struct {
	Enabled     bool     `json:"enabled"`
	Recipients  []string `json:"recipients"`
	Defaults    []string `json:"default_recipients"`
	Configured  bool     `json:"server_configured"`
	Address     string   `json:"smtp_address"`
	Username    string   `json:"smtp_username"`
	From        string   `json:"smtp_from"`
	Security    string   `json:"smtp_security"`
	Verify      bool     `json:"smtp_verify"`
	PasswordSet bool     `json:"smtp_password_set"`
	Password    *string  `json:"smtp_password,omitempty"`
	Version     int64    `json:"version"`
	password    []byte
}

type Vault interface {
	SealContext(context.Context, secrets.Binding, []byte) (secrets.Envelope, error)
	OpenContext(context.Context, secrets.Binding, secrets.Envelope) ([]byte, error)
}

type Service struct {
	vault     Vault
	db        *store.Store
	auth      *auth.Service
	smtp      SMTP
	publicURL string
	send      func(SMTP, []string, email) error
}

func New(db *store.Store, identity *auth.Service, vault Vault, server SMTP, publicURL string) *Service {
	return &Service{db: db, auth: identity, vault: vault, smtp: server, publicURL: strings.TrimRight(publicURL, "/"), send: deliver}
}

func binding(org string) secrets.Binding {
	return secrets.Binding{OrgID: org, ConnectionID: org, Version: 1}
}

func (s *Service) server(ctx context.Context, org string, settings Settings) (SMTP, bool) {
	if settings.Address == "" {
		return s.smtp, s.smtp.Address != "" && s.smtp.From != ""
	}
	server := SMTP{Address: settings.Address, Username: settings.Username, From: settings.From, Security: settings.Security, SkipVerify: !settings.Verify}
	if len(settings.password) > 0 {
		var envelope secrets.Envelope
		if json.Unmarshal(settings.password, &envelope) != nil {
			return server, false
		}
		plain, err := s.vault.OpenContext(ctx, binding(org), envelope)
		if err != nil {
			return server, false
		}
		server.Password = string(plain)
	}
	return server, server.From != ""
}

func manage(a domain.Actor) bool { return a.Role == domain.Owner || a.Role == domain.Admin }

func (s *Service) Get(ctx context.Context, session auth.Session, org string) (Settings, error) {
	var out Settings
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		var err error
		out, err = s.settingsTx(ctx, tx, org)
		return err
	})
	if err == nil {
		out.Defaults, err = s.owners(ctx, org)
	}
	return out, err
}

func normalise(in *Settings) error {
	if len(in.Recipients) > 20 || len(in.Address) > 255 || len(in.Username) > 255 || in.Password != nil && len(*in.Password) > 1024 {
		return auth.ErrInvalid
	}
	if in.Address != "" {
		if host, port, err := net.SplitHostPort(in.Address); err != nil || host == "" || port == "" {
			return auth.ErrInvalid
		}
		if _, err := mail.ParseAddress(in.From); err != nil {
			return auth.ErrInvalid
		}
	}
	if !slices.Contains([]string{"starttls", "tls", "none"}, in.Security) {
		return auth.ErrInvalid
	}
	for i, recipient := range in.Recipients {
		address, err := mail.ParseAddress(strings.TrimSpace(recipient))
		if err != nil {
			return auth.ErrInvalid
		}
		in.Recipients[i] = address.Address
	}
	return nil
}

func (s *Service) Put(ctx context.Context, session auth.Session, org string, in Settings, expected int64) (Settings, error) {
	var out Settings
	if err := normalise(&in); err != nil {
		return out, err
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		current, err := s.settingsTx(ctx, tx, org)
		if err != nil {
			return err
		}
		if current.Version != expected {
			return auth.ErrConflict
		}
		raw, _ := json.Marshal(in.Recipients)
		password := current.password
		if in.Password != nil {
			password = nil
			if *in.Password != "" {
				envelope, err := s.vault.SealContext(ctx, binding(org), []byte(*in.Password))
				if err != nil {
					return err
				}
				password, _ = json.Marshal(envelope)
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO alert_settings(org_id,enabled,recipients,smtp_address,smtp_username,smtp_from,smtp_password,smtp_security,smtp_verify,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,2) ON CONFLICT(org_id) DO UPDATE SET enabled=EXCLUDED.enabled,recipients=EXCLUDED.recipients,smtp_address=EXCLUDED.smtp_address,smtp_username=EXCLUDED.smtp_username,smtp_from=EXCLUDED.smtp_from,smtp_password=EXCLUDED.smtp_password,smtp_security=EXCLUDED.smtp_security,smtp_verify=EXCLUDED.smtp_verify,version=alert_settings.version+1`, org, in.Enabled, raw, in.Address, in.Username, in.From, password, in.Security, in.Verify); err != nil {
			return err
		}
		out, err = s.settingsTx(ctx, tx, org)
		return err
	})
	if err == nil {
		out.Defaults, err = s.owners(ctx, org)
	}
	return out, err
}

func (s *Service) Test(ctx context.Context, session auth.Session, org string, settings Settings) error {
	if err := normalise(&settings); err != nil {
		return err
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		saved, err := s.settingsTx(ctx, tx, org)
		settings.password = saved.password
		return err
	})
	if err != nil {
		return err
	}
	server, ok := s.server(ctx, org, settings)
	if settings.Password != nil {
		server.Password = *settings.Password
	}
	if !ok {
		return ErrNotConfigured
	}
	if settings.Defaults, err = s.owners(ctx, org); err != nil {
		return err
	}
	e := digest(s.publicURL, org, s.orgName(ctx, org), nil)
	e.Subject, e.Heading, e.Intro = "Reforge: test alert", "Alerts are working", "Reforge can deliver alerts to this address. You will hear from us when autopilot needs a person, pauses or is blocked."
	return s.send(server, recipients(settings), e)
}

func (s *Service) settingsTx(ctx context.Context, tx pgx.Tx, org string) (Settings, error) {
	out := Settings{Enabled: true, Recipients: []string{}, Security: "starttls", Verify: true}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT enabled,recipients,smtp_address,smtp_username,smtp_from,smtp_password,smtp_security,smtp_verify,version FROM alert_settings WHERE org_id=$1`, org).Scan(&out.Enabled, &raw, &out.Address, &out.Username, &out.From, &out.password, &out.Security, &out.Verify, &out.Version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if raw != nil {
		_ = json.Unmarshal(raw, &out.Recipients)
	}
	out.PasswordSet = len(out.password) > 0
	out.Configured = out.Address != "" && out.From != "" || out.Address == "" && s.smtp.Address != "" && s.smtp.From != ""
	return out, nil
}

func (s *Service) owners(ctx context.Context, org string) ([]string, error) {
	var ids []string
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT user_id::text FROM memberships WHERE org_id=$1 AND role='owner' ORDER BY user_id LIMIT 20`, org)
		if err != nil {
			return err
		}
		ids, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	emails := []string{}
	for _, id := range ids {
		var email string
		if e := s.db.Identity(ctx, id, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT email FROM users WHERE id=$1`, id).Scan(&email)
		}); e == nil && email != "" {
			emails = append(emails, email)
		}
	}
	return emails, err
}

func recipients(s Settings) []string {
	if len(s.Recipients) > 0 {
		return s.Recipients
	}
	return s.Defaults
}

func (s *Service) Notify(ctx context.Context, org string) error {
	var settings Settings
	var items []item
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var err error
		if settings, err = s.settingsTx(ctx, tx, org); err != nil || !settings.Enabled || !settings.Configured {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT key,kind,repository,title,detail,url,action FROM (
SELECT 'blocked:'||f.id||':'||md5(b.detail) AS key,CASE WHEN p.person THEN 'person' ELSE 'blocked' END AS kind,r.name AS repository,f.title AS title,regexp_replace(b.detail,'^Needs a person: ','') AS detail,coalesce(nullif(f.evidence->>'action_url',''),$2||'/org/'||f.org_id||'/findings?finding='||f.id) AS url,coalesce(nullif(f.evidence->>'action_label',''),'View finding') AS action FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id LEFT JOIN autopilot_attempts a ON a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version AND a.outcome='blocked' CROSS JOIN LATERAL (SELECT coalesce(f.evidence->'blockers'->>0,'') LIKE 'Needs a person%' AS person) p CROSS JOIN LATERAL (SELECT CASE WHEN p.person THEN f.evidence->'blockers'->>0 ELSE coalesce(a.reason,'') END AS detail) b WHERE f.org_id=$1 AND f.state='open' AND p.person
UNION ALL SELECT 'awaiting:'||a.task_id,'review',r.name,f.title,a.merge_reason,coalesce((SELECT rr.native_change->>'url' FROM repair_runs rr WHERE rr.org_id=a.org_id AND rr.task_id=a.task_id),''),'Review pull request' FROM autopilot_attempts a JOIN maintenance_findings f ON f.org_id=a.org_id AND f.id=a.finding_id AND f.version=a.finding_version JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE a.org_id=$1 AND a.merge_reason LIKE 'Awaiting human%'
) pending WHERE NOT EXISTS(SELECT 1 FROM alert_deliveries d WHERE d.org_id=$1 AND d.key=pending.key) ORDER BY key LIMIT 50`, org, s.publicURL)
		if err != nil {
			return err
		}
		items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[item])
		return err
	})
	if err != nil || len(items) == 0 || !settings.Enabled || !settings.Configured {
		return err
	}
	server, ok := s.server(ctx, org, settings)
	if !ok {
		return ErrNotConfigured
	}
	if settings.Defaults, err = s.owners(ctx, org); err != nil {
		return err
	}
	to := recipients(settings)
	if len(to) == 0 {
		return nil
	}
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = it.Key
	}
	if err = s.send(server, to, digest(s.publicURL, org, s.orgName(ctx, org), items)); err != nil {
		return err
	}
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO alert_deliveries(org_id,key) SELECT $1,unnest($2::text[]) ON CONFLICT DO NOTHING`, org, keys)
		return err
	})
}

func (s *Service) orgName(ctx context.Context, org string) string {
	var name string
	_ = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT name FROM organisations WHERE id=$1`, org).Scan(&name)
	})
	return name
}

func deliver(server SMTP, to []string, e email) error {
	body, err := e.html()
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(server.Address)
	if err != nil {
		return err
	}
	name := host
	if server.ServerName != "" {
		name = server.ServerName
	}
	config := &tls.Config{ServerName: name, MinVersion: tls.VersionTLS12, InsecureSkipVerify: server.SkipVerify}
	var conn net.Conn
	if server.Security == "tls" {
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", server.Address, config)
	} else {
		conn, err = net.DialTimeout("tcp", server.Address, 15*time.Second)
	}
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.Close()
	if server.Security != "tls" && server.Security != "none" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("mail server does not offer STARTTLS; choose implicit TLS or no encryption")
		}
		if err = client.StartTLS(config); err != nil {
			return err
		}
	}
	if server.Username != "" && server.Security != "none" {
		if err = client.Auth(smtp.PlainAuth("", server.Username, server.Password, host)); err != nil {
			return err
		}
	}
	sender, err := mail.ParseAddress(server.From)
	if err != nil {
		return err
	}
	if err = client.Mail(sender.Address); err != nil {
		return err
	}
	for _, recipient := range to {
		if err = client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = writer.Write(compose(server.From, to, e.Subject, e.text(), body)); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (s *Service) Invite(ctx context.Context, session auth.Session, org, to, orgName, token string) error {
	var settings Settings
	if err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		settings, err = s.settingsTx(ctx, tx, org)
		return err
	}); err != nil {
		return err
	}
	server, ok := s.server(ctx, org, settings)
	if !ok {
		return ErrNotConfigured
	}
	return SendInvitation(server, to, orgName, s.publicURL+"/auth/join?token="+token)
}
