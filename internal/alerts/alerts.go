package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/store"
)

var ErrNotConfigured = errors.New("server email is not configured")

type SMTP struct {
	Address, Username, Password, From string
}

type Settings struct {
	Enabled    bool     `json:"enabled"`
	Recipients []string `json:"recipients"`
	Defaults   []string `json:"default_recipients"`
	Configured bool     `json:"server_configured"`
	Version    int64    `json:"version"`
}

type Service struct {
	db        *store.Store
	auth      *auth.Service
	smtp      SMTP
	publicURL string
	send      func(SMTP, []string, string, string) error
}

func New(db *store.Store, identity *auth.Service, server SMTP, publicURL string) *Service {
	return &Service{db: db, auth: identity, smtp: server, publicURL: strings.TrimRight(publicURL, "/"), send: deliver}
}

func (s *Service) configured() bool { return s.smtp.Address != "" && s.smtp.From != "" }

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
	return out, err
}

func (s *Service) Put(ctx context.Context, session auth.Session, org string, in Settings, expected int64) (Settings, error) {
	var out Settings
	if len(in.Recipients) > 20 {
		return out, auth.ErrInvalid
	}
	for i, recipient := range in.Recipients {
		address, err := mail.ParseAddress(strings.TrimSpace(recipient))
		if err != nil {
			return out, auth.ErrInvalid
		}
		in.Recipients[i] = address.Address
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
		if _, err = tx.Exec(ctx, `INSERT INTO alert_settings(org_id,enabled,recipients,version) VALUES($1,$2,$3,2) ON CONFLICT(org_id) DO UPDATE SET enabled=EXCLUDED.enabled,recipients=EXCLUDED.recipients,version=alert_settings.version+1`, org, in.Enabled, raw); err != nil {
			return err
		}
		out, err = s.settingsTx(ctx, tx, org)
		return err
	})
	return out, err
}

func (s *Service) Test(ctx context.Context, session auth.Session, org string) error {
	var settings Settings
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		var err error
		settings, err = s.settingsTx(ctx, tx, org)
		return err
	})
	if err != nil {
		return err
	}
	if !s.configured() {
		return ErrNotConfigured
	}
	return s.send(s.smtp, recipients(settings), "Reforge alert test", "Reforge can send alerts to this address.\n\n"+s.publicURL+"/org/"+org+"/organisation\n")
}

func (s *Service) settingsTx(ctx context.Context, tx pgx.Tx, org string) (Settings, error) {
	out := Settings{Enabled: true, Recipients: []string{}, Configured: s.configured()}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT enabled,recipients,version FROM alert_settings WHERE org_id=$1`, org).Scan(&out.Enabled, &raw, &out.Version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if raw != nil {
		_ = json.Unmarshal(raw, &out.Recipients)
	}
	rows, err := tx.Query(ctx, `SELECT u.email FROM memberships m JOIN users u ON u.id=m.user_id WHERE m.org_id=$1 AND m.role='owner' AND u.email<>'' ORDER BY u.email LIMIT 20`, org)
	if err != nil {
		return out, err
	}
	out.Defaults, err = pgx.CollectRows(rows, pgx.RowTo[string])
	return out, err
}

func recipients(s Settings) []string {
	if len(s.Recipients) > 0 {
		return s.Recipients
	}
	return s.Defaults
}

type item struct{ key, line string }

func (s *Service) Notify(ctx context.Context, org string) error {
	if !s.configured() {
		return nil
	}
	var settings Settings
	var items []item
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var err error
		if settings, err = s.settingsTx(ctx, tx, org); err != nil || !settings.Enabled {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT key,line FROM (
SELECT 'paused:'||t.id AS key,'Run paused: '||r.name||' — '||t.reason||' '||$2||'/org/'||t.org_id||'/runs?run='||t.id AS line FROM workflow_tasks t JOIN repositories r ON r.org_id=t.org_id AND r.id=t.repository_id WHERE t.org_id=$1 AND t.state='queued' AND t.reason LIKE 'Paused:%'
UNION ALL SELECT 'blocked:'||f.id||':'||f.version,'Blocked: '||r.name||' — '||f.title||': '||coalesce(f.evidence->'blockers'->>0,a.reason,'') FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id LEFT JOIN autopilot_attempts a ON a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version AND a.outcome='blocked' WHERE f.org_id=$1 AND f.state='open' AND (a.finding_id IS NOT NULL OR jsonb_array_length(coalesce(f.evidence->'blockers','[]'))>0)
UNION ALL SELECT 'awaiting:'||f.id||':'||f.version,'Needs a person: '||r.name||' — '||f.title||' — '||a.merge_reason||' '||coalesce((SELECT rr.native_change->>'url' FROM repair_runs rr WHERE rr.org_id=a.org_id AND rr.task_id=a.task_id),'') FROM autopilot_attempts a JOIN maintenance_findings f ON f.org_id=a.org_id AND f.id=a.finding_id AND f.version=a.finding_version JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE a.org_id=$1 AND a.merge_reason LIKE 'Awaiting human%'
) pending WHERE NOT EXISTS(SELECT 1 FROM alert_deliveries d WHERE d.org_id=$1 AND d.key=pending.key) ORDER BY key LIMIT 50`, org, s.publicURL)
		if err != nil {
			return err
		}
		items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[item])
		return err
	})
	if err != nil || len(items) == 0 || !settings.Enabled {
		return err
	}
	to := recipients(settings)
	if len(to) == 0 {
		return nil
	}
	lines := make([]string, len(items))
	keys := make([]string, len(items))
	for i, it := range items {
		lines[i], keys[i] = "- "+it.line, it.key
	}
	if err = s.send(s.smtp, to, fmt.Sprintf("Reforge: %d item(s) need attention", len(items)), strings.Join(lines, "\n")+"\n"); err != nil {
		return err
	}
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO alert_deliveries(org_id,key) SELECT $1,unnest($2::text[]) ON CONFLICT DO NOTHING`, org, keys)
		return err
	})
}

func deliver(server SMTP, to []string, subject, body string) error {
	host, _, err := net.SplitHostPort(server.Address)
	if err != nil {
		return err
	}
	var auth smtp.Auth
	if server.Username != "" {
		auth = smtp.PlainAuth("", server.Username, server.Password, host)
	}
	to = slices.Clone(to)
	message := "From: " + server.From + "\r\nTo: " + strings.Join(to, ", ") + "\r\nSubject: " + subject + "\r\nDate: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + strings.ReplaceAll(body, "\n", "\r\n")
	return smtp.SendMail(server.Address, auth, server.From, to, []byte(message))
}
