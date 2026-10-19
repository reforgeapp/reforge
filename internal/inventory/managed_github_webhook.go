package inventory

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
)

const managedHostedWebhookPath = "/hooks/github/app"

type managedEvent struct {
	Kind         string `json:"-"`
	Action       string `json:"action"`
	Installation struct {
		ID      int64 `json:"id"`
		AppID   int64 `json:"app_id"`
		Account struct {
			ID int64 `json:"id"`
		} `json:"account"`
	} `json:"installation"`
	Repository json.RawMessage `json:"repository"`
	Added      json.RawMessage `json:"repositories_added"`
	Removed    json.RawMessage `json:"repositories_removed"`
}

func verifyManagedSignature(secret string, headers http.Header, body []byte) bool {
	signature := headers.Get("X-Hub-Signature-256")
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	given, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil || len(given) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(given, mac.Sum(nil))
}

func managedIdentity(body []byte) (installationID, appID int64, ok bool) {
	var ev struct {
		Installation struct {
			ID    int64 `json:"id"`
			AppID int64 `json:"app_id"`
		} `json:"installation"`
	}
	if json.Unmarshal(body, &ev) != nil || ev.Installation.ID <= 0 {
		return 0, 0, false
	}
	return ev.Installation.ID, ev.Installation.AppID, true
}

func parseManagedEvent(kind string, body []byte) (managedEvent, bool) {
	var ev managedEvent
	if kind != "installation" && kind != "installation_repositories" {
		return ev, false
	}
	if json.Unmarshal(body, &ev) != nil || ev.Installation.ID <= 0 {
		return ev, false
	}
	if len(ev.Repository) > 0 && string(ev.Repository) != "null" {
		return ev, false
	}
	ev.Kind = kind
	added := len(ev.Added) > 0 && string(ev.Added) != "null"
	removed := len(ev.Removed) > 0 && string(ev.Removed) != "null"
	if kind == "installation_repositories" {
		if !added && !removed {
			return ev, false
		}
		return ev, true
	}
	if ev.Action == "" || added || removed {
		return ev, false
	}
	return ev, true
}

func loadConnectionRaw(ctx context.Context, tx pgx.Tx, org, id string) (connections.Connection, error) {
	var c connections.Connection
	var settings []byte
	err := tx.QueryRow(ctx, `SELECT id::text,org_id::text,kind,provider,state,version,settings FROM connections WHERE org_id=$1 AND id=$2 FOR SHARE`, org, id).Scan(&c.ID, &c.OrgID, &c.Kind, &c.Provider, &c.State, &c.Version, &settings)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, auth.ErrForbidden
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(settings, &c.Settings); err != nil {
		return c, err
	}
	return c, nil
}

func recordDelivery(ctx context.Context, tx pgx.Tx, org, endpointID, key, encoded string) (bool, error) {
	var stored string
	err := tx.QueryRow(ctx, `SELECT digest FROM inventory_deliveries WHERE org_id=$1 AND endpoint_id=$2 AND delivery_key=$3`, org, endpointID, key).Scan(&stored)
	if err == nil {
		if stored != encoded {
			return false, auth.ErrConflict
		}
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var replay bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_deliveries WHERE org_id=$1 AND endpoint_id=$2 AND digest=$3)`, org, endpointID, encoded).Scan(&replay); err != nil {
		return false, err
	}
	if replay {
		return true, nil
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM inventory_deliveries WHERE org_id=$1`, org).Scan(&count); err != nil {
		return false, err
	}
	if count >= 100000 {
		return false, ErrBusy
	}
	_, err = tx.Exec(ctx, `INSERT INTO inventory_deliveries(org_id,endpoint_id,delivery_key,digest) VALUES($1,$2,$3,$4)`, org, endpointID, key, encoded)
	return false, err
}

func (s *Service) applyManagedEvent(ctx context.Context, tx pgx.Tx, org, endpointID, connectionID string, c connections.Connection, ev managedEvent, headers http.Header, encoded string) error {
	if c.Settings.InstallationID == "" || c.Settings.InstallationID != strconv.FormatInt(ev.Installation.ID, 10) {
		return auth.ErrUnauthenticated
	}
	var bindingApp, bindingAccount int64
	err := tx.QueryRow(ctx, `SELECT app_id, account_id FROM github_installation_bindings WHERE org_id=$1 AND connection_id=$2`, org, connectionID).Scan(&bindingApp, &bindingAccount)
	if errors.Is(err, pgx.ErrNoRows) {
		if c.State == "revoked" {
			return nil
		}
		return auth.ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if ev.Installation.AppID > 0 && ev.Installation.AppID != bindingApp {
		return auth.ErrUnauthenticated
	}
	if c.Settings.AppID == "" || strconv.FormatInt(bindingApp, 10) != c.Settings.AppID {
		return auth.ErrUnauthenticated
	}
	if ev.Installation.Account.ID <= 0 || ev.Installation.Account.ID != bindingAccount {
		return auth.ErrUnauthenticated
	}
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM connections WHERE org_id=$1 AND id=$2 FOR UPDATE`, org, connectionID).Scan(&locked); err != nil {
		return err
	}
	key := headers.Get("X-GitHub-Delivery")
	if len(key) > 256 {
		return auth.ErrInvalid
	}
	if key == "" {
		key = "digest:" + encoded
	}
	replay, err := recordDelivery(ctx, tx, org, endpointID, key, encoded)
	if err != nil {
		return err
	}
	if replay {
		return nil
	}
	delivery := headers.Get("X-GitHub-Delivery")
	if ev.Kind == "installation_repositories" {
		if c.State != "revoked" && c.State != "disabled" {
			_, err = tx.Exec(ctx, `UPDATE inventory_sources SET poll_due=least(poll_due,clock_timestamp()+interval '30 seconds') WHERE org_id=$1 AND connection_id=$2`, org, connectionID)
		}
		return err
	}
	state, reason := "", ""
	switch ev.Action {
	case "deleted":
		state, reason = "revoked", "GitHub App uninstalled; reinstall and connect again to restore access"
	case "suspend":
		state, reason = "disabled", "GitHub App installation suspended; unsuspend it in GitHub, then run a capability test"
	case "unsuspend", "new_permissions_accepted":
		state, reason = "unverified", "GitHub App installation changed; run a capability test"
	default:
		return nil
	}
	var tag pgconn.CommandTag
	switch state {
	case "revoked":
		tag, err = tx.Exec(ctx, `UPDATE connections SET state='revoked',reason=$3,capabilities='{}',verified_at=NULL,version=version+1,revoked_at=now() WHERE org_id=$1 AND id=$2 AND state<>'revoked'`, org, connectionID, reason)
	case "disabled":
		tag, err = tx.Exec(ctx, `UPDATE connections SET state='disabled',reason=$3,capabilities='{}',verified_at=NULL,version=version+1 WHERE org_id=$1 AND id=$2 AND state NOT IN ('revoked','disabled')`, org, connectionID, reason)
	default:
		tag, err = tx.Exec(ctx, `UPDATE connections SET state='unverified',reason=$3,capabilities='{}',verified_at=NULL,version=version+1 WHERE org_id=$1 AND id=$2 AND state NOT IN ('revoked','unverified')`, org, connectionID, reason)
	}
	if err != nil {
		return err
	}
	deleted := int64(0)
	if state == "revoked" {
		btag, err := tx.Exec(ctx, `DELETE FROM github_installation_bindings WHERE org_id=$1 AND connection_id=$2`, org, connectionID)
		if err != nil {
			return err
		}
		deleted = btag.RowsAffected()
	}
	if tag.RowsAffected() == 0 && deleted == 0 {
		return nil
	}
	return audit(ctx, tx, org, "github", "connection.github_installation_"+state, connectionID, delivery, map[string]any{"state": state})
}
