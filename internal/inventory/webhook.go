package inventory

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/secrets"
)

const MaxWebhookBytes = 1 << 20

func webhookPath(org, id string) string { return "/hooks/v1/" + org + "/" + id }
func (s *Service) ConfigureWebhook(ctx context.Context, session auth.Session, org, connectionID string, expected int64, request string) (IssuedWebhook, error) {
	var out IssuedWebhook
	if !auth.ValidID(connectionID) || expected < 0 {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := owner(a); err != nil {
			return err
		}
		c, err := connection(ctx, tx, org, connectionID)
		if err != nil {
			return err
		}
		var current int64
		err = tx.QueryRow(ctx, `SELECT id::text,version FROM inventory_webhooks WHERE org_id=$1 AND connection_id=$2 FOR UPDATE`, org, connectionID).Scan(&out.ID, &current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if current != expected {
			return auth.ErrConflict
		}
		if out.ID == "" {
			out.ID = domain.NewID()
		}
		raw := make([]byte, 32)
		if _, err = rand.Read(raw); err != nil {
			return err
		}
		out.Secret = base64.RawURLEncoding.EncodeToString(raw)
		if c.Provider == "gitlab" {
			out.Secret = "whsec_" + base64.StdEncoding.EncodeToString(raw)
		}
		clear(raw)
		out.ConnectionID = connectionID
		out.Version = current + 1
		out.Path = webhookPath(org, out.ID)
		envelope, err := s.vault.SealContext(ctx, secrets.Binding{OrgID: org, ConnectionID: out.ID, Version: out.Version}, []byte(out.Secret))
		if err != nil {
			return err
		}
		body, _ := json.Marshal(envelope)
		_, err = tx.Exec(ctx, `INSERT INTO inventory_webhooks(org_id,id,connection_id,envelope,version) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,connection_id) DO UPDATE SET envelope=excluded.envelope,version=excluded.version,revoked_at=NULL`, org, out.ID, connectionID, body, out.Version)
		if err != nil {
			return err
		}
		return audit(ctx, tx, org, a.UserID, "inventory.webhook_rotate", out.ID, request, map[string]any{"version": out.Version, "session_id": a.SessionID})
	})
	if err != nil {
		out.Secret = ""
	}
	return out, err
}
func (s *Service) Webhook(ctx context.Context, session auth.Session, org, connectionID string) (Webhook, error) {
	var w Webhook
	if !auth.ValidID(connectionID) {
		return w, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := owner(a); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT id::text,connection_id::text,version,revoked_at IS NOT NULL FROM inventory_webhooks WHERE org_id=$1 AND connection_id=$2`, org, connectionID).Scan(&w.ID, &w.ConnectionID, &w.Version, &w.Revoked)
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrForbidden
		}
		w.Path = webhookPath(org, w.ID)
		return err
	})
	return w, err
}
func (s *Service) RevokeWebhook(ctx context.Context, session auth.Session, org, connectionID string, expected int64, request string) error {
	if !auth.ValidID(connectionID) || expected < 1 {
		return auth.ErrInvalid
	}
	return s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := owner(a); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE inventory_webhooks SET revoked_at=clock_timestamp(),envelope='{}',version=version+1 WHERE org_id=$1 AND connection_id=$2 AND version=$3`, org, connectionID, expected)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return auth.ErrConflict
		}
		return audit(ctx, tx, org, a.UserID, "inventory.webhook_revoke", connectionID, request, map[string]any{"version": expected + 1, "session_id": a.SessionID})
	})
}
func (s *Service) HandleWebhook(ctx context.Context, org, id string, headers http.Header, body []byte) error {
	if !auth.ValidID(org) || !auth.ValidID(id) || len(body) == 0 || len(body) > MaxWebhookBytes || s.decode == nil {
		return auth.ErrInvalid
	}
	digest := sha256.Sum256(body)
	encoded := hex.EncodeToString(digest[:])
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, org); err != nil {
			return auth.ErrUnauthenticated
		}
		var connectionID string
		var version int64
		var envelopeBytes []byte
		err := tx.QueryRow(ctx, `SELECT connection_id::text,version,envelope FROM inventory_webhooks WHERE org_id=$1 AND id=$2 AND revoked_at IS NULL FOR SHARE`, org, id).Scan(&connectionID, &version, &envelopeBytes)
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		c, err := connection(ctx, tx, org, connectionID)
		if err != nil {
			return auth.ErrUnauthenticated
		}
		var envelope secrets.Envelope
		if err = json.Unmarshal(envelopeBytes, &envelope); err != nil {
			return err
		}
		secret, err := s.vault.OpenContext(ctx, secrets.Binding{OrgID: org, ConnectionID: id, Version: version}, envelope)
		if err != nil {
			return err
		}
		defer clear(secret)
		event, err := s.decode(c.Provider, string(secret), headers, body)
		if err != nil {
			return auth.ErrUnauthenticated
		}
		if !validNative(event.Repository.NativeID) || len(event.DeliveryID) > 256 || len(event.Kind) > 128 {
			return auth.ErrInvalid
		}
		key := event.DeliveryID
		if key == "" {
			key = "digest:" + encoded
		}
		var stored string
		err = tx.QueryRow(ctx, `SELECT digest FROM inventory_deliveries WHERE org_id=$1 AND endpoint_id=$2 AND delivery_key=$3`, org, id, key).Scan(&stored)
		if err == nil {
			if stored != encoded {
				return auth.ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var replay bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_deliveries WHERE org_id=$1 AND endpoint_id=$2 AND digest=$3)`, org, id, encoded).Scan(&replay); err != nil {
			return err
		}
		if replay {
			return nil
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM inventory_deliveries WHERE org_id=$1`, org).Scan(&count); err != nil {
			return err
		}
		if count >= 100000 {
			return ErrBusy
		}
		_, err = tx.Exec(ctx, `INSERT INTO inventory_deliveries(org_id,endpoint_id,delivery_key,digest) VALUES($1,$2,$3,$4)`, org, id, key, encoded)
		if err != nil {
			return err
		}
		var repo string
		err = tx.QueryRow(ctx, `SELECT r.id::text FROM repositories r JOIN inventory_repository_state s ON s.org_id=r.org_id AND s.repository_id=r.id WHERE r.org_id=$1 AND r.connection_id=$2 AND r.native_id=$3`, org, connectionID, event.Repository.NativeID).Scan(&repo)
		if errors.Is(err, pgx.ErrNoRows) {
			_, err = tx.Exec(ctx, `UPDATE inventory_sources SET poll_due=least(poll_due,clock_timestamp()+interval '30 seconds') WHERE org_id=$1 AND connection_id=$2`, org, connectionID)
			return err
		}
		if err != nil {
			return err
		}
		return queueRefresh(ctx, tx, c, repo)
	})
}
