package githubapp

import (
	"context"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/forge/github"
)

var errProvider = errors.New("GitHub request failed")
var ErrWebhookUnavailable = fmt.Errorf("%w: GitHub App webhook endpoint is unavailable", ErrUnavailable)

func parseKey(raw string) (*rsa.PrivateKey, error) { return github.ParseAppKey([]byte(raw)) }

func (s *Service) call(ctx context.Context, method, target, bearer string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return errProvider
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Reforge")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return errProvider
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (256<<10)+1))
	if err != nil || len(raw) > 256<<10 || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errProvider
	}
	if json.Unmarshal(raw, out) != nil {
		return errProvider
	}
	return nil
}

func (s *Service) exchange(ctx context.Context, clientID, clientSecret, code, verifier string) (string, error) {
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	form := url.Values{"client_id": {clientID}, "client_secret": {clientSecret}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {strings.TrimRight(s.opt.PublicURL, "/") + "/auth/github/oauth/callback"}}
	if err := s.call(ctx, "POST", s.web+"/login/oauth/access_token", "", form, &out); err != nil || out.Error != "" || out.AccessToken == "" {
		return "", errProvider
	}
	return out.AccessToken, nil
}

func (s *Service) verify(ctx context.Context, userToken string, appID, installationID int64, pem []byte) (installation, string) {
	var inst installation
	if installationID <= 0 {
		return inst, "installation_missing"
	}
	var user struct {
		ID int64 `json:"id"`
	}
	if s.call(ctx, "GET", s.api+"/user", userToken, nil, &user) != nil || user.ID <= 0 {
		return inst, "provider_unavailable"
	}
	var listed *installation
	for page := 1; page <= 5 && listed == nil; page++ {
		var list struct {
			Installations []installation `json:"installations"`
		}
		if s.call(ctx, "GET", s.api+"/user/installations?per_page=100&page="+strconv.Itoa(page), userToken, nil, &list) != nil {
			return inst, "provider_unavailable"
		}
		for i := range list.Installations {
			if list.Installations[i].ID == installationID {
				listed = &list.Installations[i]
			}
		}
		if len(list.Installations) < 100 {
			break
		}
	}
	if listed == nil || listed.AppID != appID || listed.Account.ID <= 0 {
		return inst, "installation_unavailable"
	}
	switch listed.Account.Type {
	case "User":
		if listed.Account.ID != user.ID {
			return inst, "not_owner"
		}
	case "Organization":
		if !orgPattern.MatchString(listed.Account.Login) {
			return inst, "not_owner"
		}
		var membership struct {
			State        string `json:"state"`
			Role         string `json:"role"`
			Organization struct {
				ID int64 `json:"id"`
			} `json:"organization"`
		}
		if s.call(ctx, "GET", s.api+"/user/memberships/orgs/"+url.PathEscape(listed.Account.Login), userToken, nil, &membership) != nil || membership.State != "active" || membership.Role != "admin" || membership.Organization.ID != listed.Account.ID {
			return inst, "not_owner"
		}
	default:
		return inst, "not_owner"
	}
	rsaKey, err := github.ParseAppKey(pem)
	if err != nil {
		return inst, "setup_failed"
	}
	jwt, err := github.AppJWT(strconv.FormatInt(appID, 10), rsaKey)
	if err != nil {
		return inst, "setup_failed"
	}
	if s.call(ctx, "GET", s.api+"/app/installations/"+strconv.FormatInt(installationID, 10), jwt, nil, &inst) != nil {
		return inst, "installation_unavailable"
	}
	if inst.ID != installationID || inst.AppID != appID || inst.Account.ID != listed.Account.ID || inst.SuspendedAt != nil {
		return inst, "installation_unavailable"
	}
	return inst, ""
}

func verifySignature(secret []byte, headers http.Header, body []byte) error {
	signature := headers.Get("X-Hub-Signature-256")
	if !strings.HasPrefix(signature, "sha256=") {
		return auth.ErrUnauthenticated
	}
	given, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil || len(given) != sha256.Size {
		return auth.ErrUnauthenticated
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	if !hmac.Equal(given, mac.Sum(nil)) {
		return auth.ErrUnauthenticated
	}
	return nil
}

func (s *Service) HandleWebhook(ctx context.Context, headers http.Header, body []byte) error {
	if s.opt.Hosted == nil {
		return auth.ErrForbidden
	}
	if err := verifySignature(s.opt.Hosted.WebhookSecret, headers, body); err != nil {
		return err
	}
	var event struct {
		Action       string `json:"action"`
		Installation struct {
			ID int64 `json:"id"`
		} `json:"installation"`
	}
	if json.Unmarshal(body, &event) != nil || event.Installation.ID <= 0 {
		return nil
	}
	var org, connectionID string
	appID := strconv.FormatInt(s.opt.Hosted.AppID, 10)
	err := s.db.Identity(ctx, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.github_app_id',$1,true),set_config('reforge.github_installation_id',$2,true)`, appID, strconv.FormatInt(event.Installation.ID, 10)); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT org_id::text,connection_id::text FROM github_installation_bindings WHERE app_id=$1 AND installation_id=$2`, s.opt.Hosted.AppID, event.Installation.ID).Scan(&org, &connectionID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var endpoint, state string
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT state FROM connections WHERE org_id=$1 AND id=$2`, org, connectionID).Scan(&state); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id::text FROM inventory_webhooks WHERE org_id=$1 AND connection_id=$2 AND revoked_at IS NULL`, org, connectionID).Scan(&endpoint)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if state == "revoked" {
			return nil
		}
		return ErrWebhookUnavailable
	}
	if err != nil {
		return err
	}
	if endpoint == "" || s.inv == nil {
		return ErrWebhookUnavailable
	}
	return s.inv.HandleHostedWebhook(ctx, org, connectionID, endpoint, s.opt.Hosted.AppID, s.opt.Hosted.WebhookSecret, headers, body)
}
