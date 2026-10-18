package connections

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/chatgpt"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
	"github.com/reforgeapp/reforge/pkg/network"
	"github.com/reforgeapp/reforge/pkg/secrets"
)

type ChatGPTStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type chatGPTAttempt struct {
	Name         string              `json:"name"`
	Nonce        string              `json:"nonce"`
	Verifier     string              `json:"verifier"`
	ConnectionID string              `json:"connection_id,omitempty"`
	Version      int64               `json:"version,omitempty"`
	ClientID     string              `json:"client_id,omitempty"`
	HostID       string              `json:"host_id,omitempty"`
	Credentials  chatgpt.Credentials `json:"credentials"`
}

type ChatGPTSetup struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Account string        `json:"account"`
	Models  []CatalogItem `json:"models"`
	Model   string        `json:"model,omitempty"`
}

func (s *Service) SetChatGPT(config chatgpt.Config) { s.chatGPT = config }

func (s *Service) CleanupChatGPT(ctx context.Context) error {
	return s.db.Identity(ctx, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.org_id','',true)`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM chatgpt_setups`)
		return err
	})
}

func (s *Service) RunChatGPTCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.CleanupChatGPT(ctx); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "ChatGPT setup cleanup failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) ChatGPTStatus(ctx context.Context, session auth.Session, org string) (ChatGPTStatus, error) {
	out := ChatGPTStatus{Available: s.chatGPT.Available()}
	if !out.Available {
		out.Reason = "ChatGPT plan connection needs self-hosted setup or an approved OpenAI OAuth client"
	}
	err := s.auth.WithActor(ctx, session, org, func(_ pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return auth.ErrForbidden
		}
		return nil
	})
	return out, err
}

func (s *Service) chatGPTClient(ctx context.Context) (*chatgpt.Client, error) {
	if !s.chatGPT.Available() {
		return nil, chatgpt.ErrUnavailable
	}
	client, err := network.NewClient(chatgpt.Issuer, network.Options{})
	if err != nil {
		return nil, chatgpt.ErrUnavailable
	}
	defer client.CloseIdleConnections()
	return chatgpt.New(ctx, s.chatGPT, client)
}

func (s *Service) sealChatGPT(ctx context.Context, org, id string, value chatGPTAttempt) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	envelope, err := s.vault.SealContext(ctx, secrets.Binding{OrgID: org, ConnectionID: id, Version: 1}, raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func (s *Service) openChatGPT(ctx context.Context, org, id string, raw []byte) (chatGPTAttempt, error) {
	var value chatGPTAttempt
	var envelope secrets.Envelope
	if json.Unmarshal(raw, &envelope) != nil {
		return value, auth.ErrInvalid
	}
	plain, err := s.vault.OpenContext(ctx, secrets.Binding{OrgID: org, ConnectionID: id, Version: 1}, envelope)
	if err != nil {
		return value, err
	}
	defer clear(plain)
	err = json.Unmarshal(plain, &value)
	return value, err
}

func (s *Service) BeginChatGPT(ctx context.Context, session auth.Session, org, name, connection string, expected int64, browser, request string) (string, error) {
	if !s.chatGPT.Available() {
		return "", chatgpt.ErrUnavailable
	}
	if session.AutomationID() != "" {
		return "", auth.ErrForbidden
	}
	if _, err := s.ChatGPTStatus(ctx, session, org); err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 160 || len(browser) != 43 || connection != "" && !auth.ValidID(connection) {
		return "", auth.ErrInvalid
	}
	id, nonce, verifier, state := domain.NewID(), chatgpt.Random(), chatgpt.Random(), chatgpt.Random()
	value := chatGPTAttempt{Name: name, Nonce: nonce, Verifier: verifier, ConnectionID: connection, Version: expected, HostID: s.chatGPT.HostID}
	client, err := s.chatGPTClient(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner || session.AutomationID() != "" {
			return auth.ErrForbidden
		}
		if s.chatGPT.SelfHosted && connection == "" {
			var priorID string
			var raw []byte
			err := tx.QueryRow(ctx, `SELECT id,envelope FROM chatgpt_setups WHERE org_id=$1 AND user_id=$2 AND session_hash=$3 AND phase='exchanging' AND expires_at>now() LIMIT 1`, org, actor.UserID, chatgpt.Digest(session.ID)).Scan(&priorID, &raw)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil {
				prior, err := s.openChatGPT(ctx, org, priorID, raw)
				if err != nil {
					return err
				}
				if prior.ConnectionID == "" && prior.Name == name && prior.HostID == value.HostID {
					value.ClientID = prior.ClientID
				}
			}
		}
		if connection != "" {
			current, err := load(ctx, tx, org, connection)
			if err != nil {
				return err
			}
			if !current.Settings.ChatGPT() || current.State == "revoked" || current.Version != expected {
				return auth.ErrConflict
			}
			var credentials chatgpt.Credentials
			if err := s.chatGPTCredentialsTx(ctx, tx, current, &credentials); err != nil {
				return err
			}
			value.ClientID = credentials.ClientID
		}
		envelope, err := s.sealChatGPT(ctx, org, id, value)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM chatgpt_setups WHERE org_id=$1 AND (expires_at<now() OR user_id=$2)`, org, actor.UserID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO chatgpt_setups(org_id,id,user_id,session_hash,browser_hash,state_hash,envelope,phase) VALUES($1,$2,$3,$4,$5,$6,$7,'pending')`, org, id, actor.UserID, chatgpt.Digest(session.ID), chatgpt.Digest(browser), chatgpt.Digest(state), envelope)
		if err != nil {
			return err
		}
		return record(ctx, tx, actor, "connection.chatgpt_started", id, request, 1)
	})
	if err != nil {
		return "", err
	}
	return client.AuthorizationURL(org+"."+id+"."+state, nonce, verifier, value.ClientID), nil
}

func (s *Service) CompleteChatGPT(ctx context.Context, session auth.Session, state, browser, code, failure string, issued ...string) (string, string, error) {
	parts := strings.Split(state, ".")
	if len(parts) != 3 || !auth.ValidID(parts[0]) || !auth.ValidID(parts[1]) || len(parts[2]) != 43 || len(browser) != 43 {
		return "", "", chatgpt.ErrAuthorization
	}
	org, id := parts[0], parts[1]
	var attempt chatGPTAttempt
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner || session.AutomationID() != "" {
			return auth.ErrForbidden
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `UPDATE chatgpt_setups SET phase='exchanging' WHERE org_id=$1 AND id=$2 AND user_id=$3 AND session_hash=$4 AND browser_hash=$5 AND state_hash=$6 AND phase='pending' AND expires_at>now() RETURNING envelope`, org, id, actor.UserID, chatgpt.Digest(session.ID), chatgpt.Digest(browser), chatgpt.Digest(parts[2])).Scan(&raw); err != nil {
			return chatgpt.ErrAuthorization
		}
		var err error
		attempt, err = s.openChatGPT(ctx, org, id, raw)
		return err
	})
	if err != nil {
		return org, id, err
	}
	if failure != "" || code == "" || len(code) > 4096 {
		return org, id, chatgpt.ErrAuthorization
	}
	client, err := s.chatGPTClient(ctx)
	if err != nil {
		return org, id, err
	}
	defer client.Close()
	callbackClient := ""
	if len(issued) == 1 {
		callbackClient = issued[0]
	}
	expectedClient := s.chatGPT.ClientID
	if s.chatGPT.SelfHosted {
		if attempt.HostID != s.chatGPT.HostID {
			return org, id, chatgpt.ErrAuthorization
		}
		expectedClient = attempt.ClientID
		registrationID, err := client.RegistrationID(expectedClient, callbackClient)
		if err != nil {
			return org, id, err
		}
		attempt.ClientID = registrationID
		err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, actor domain.Actor) error {
			if actor.Role != domain.Owner {
				return auth.ErrForbidden
			}
			envelope, err := s.sealChatGPT(ctx, org, id, attempt)
			if err != nil {
				return err
			}
			command, err := tx.Exec(ctx, `UPDATE chatgpt_setups SET envelope=$4 WHERE org_id=$1 AND id=$2 AND user_id=$3 AND phase='exchanging' AND expires_at>now()`, org, id, actor.UserID, envelope)
			if err == nil && command.RowsAffected() != 1 {
				return chatgpt.ErrAuthorization
			}
			return err
		})
		if err != nil {
			return org, id, err
		}
	}
	attempt.Credentials, err = client.ExchangeRegistration(ctx, code, attempt.Nonce, attempt.Verifier, expectedClient, callbackClient)
	if err != nil {
		return org, id, err
	}
	attempt.Nonce, attempt.Verifier = "", ""
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, actor domain.Actor) error {
		if actor.Role != domain.Owner {
			return auth.ErrForbidden
		}
		if attempt.ConnectionID != "" {
			current, err := s.ResolveTx(ctx, tx, org, attempt.ConnectionID, "")
			if err != nil && !errors.Is(err, chatgpt.ErrRefresh) {
				return err
			}
			var prior chatgpt.Credentials
			if err = s.chatGPTCredentialsTx(ctx, tx, current.Connection, &prior); err != nil {
				return err
			}
			if prior.Subject != attempt.Credentials.Subject || prior.ClientID != attempt.Credentials.ClientID {
				return chatgpt.ErrAuthorization
			}
		}
		envelope, err := s.sealChatGPT(ctx, org, id, attempt)
		if err != nil {
			return err
		}
		command, err := tx.Exec(ctx, `UPDATE chatgpt_setups SET phase='authenticated',envelope=$4 WHERE org_id=$1 AND id=$2 AND user_id=$3 AND phase='exchanging' AND expires_at>now()`, org, id, actor.UserID, envelope)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return chatgpt.ErrAuthorization
		}
		return nil
	})
	return org, id, err
}

func (s *Service) chatGPTSetupTx(ctx context.Context, tx pgx.Tx, session auth.Session, actor domain.Actor, id string) (chatGPTAttempt, error) {
	var raw []byte
	if actor.Role != domain.Owner || !auth.ValidID(id) {
		return chatGPTAttempt{}, auth.ErrForbidden
	}
	if err := tx.QueryRow(ctx, `SELECT envelope FROM chatgpt_setups WHERE org_id=$1 AND id=$2 AND user_id=$3 AND session_hash=$4 AND phase='authenticated' AND expires_at>now() FOR UPDATE`, actor.OrgID, id, actor.UserID, chatgpt.Digest(session.ID)).Scan(&raw); err != nil {
		return chatGPTAttempt{}, chatgpt.ErrAuthorization
	}
	return s.openChatGPT(ctx, actor.OrgID, id, raw)
}

func (s *Service) ChatGPTSetup(ctx context.Context, session auth.Session, org, id string) (ChatGPTSetup, error) {
	out := ChatGPTSetup{ID: id, Models: []CatalogItem{}}
	var attempt chatGPTAttempt
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, actor domain.Actor) error {
		var err error
		attempt, err = s.chatGPTSetupTx(ctx, tx, session, actor, id)
		if err == nil && attempt.ConnectionID != "" {
			current, loadErr := load(ctx, tx, org, attempt.ConnectionID)
			if loadErr != nil {
				return loadErr
			}
			out.Model = current.Settings.Model
		}
		return err
	})
	if err != nil {
		return out, err
	}
	out.Name, out.Account = attempt.Name, attempt.Credentials.Email
	client, err := network.NewClient(chatgpt.Resource, network.Options{})
	if err != nil {
		return out, err
	}
	defer client.CloseIdleConnections()
	s.mu.RLock()
	catalog := s.catalog
	s.mu.RUnlock()
	if catalog == nil {
		return out, ErrCatalogUnavailable
	}
	items, err := catalog(ctx, Resolved{Connection: Connection{Kind: "model", Provider: "openai", OrgID: org, Endpoint: chatgpt.Resource, Settings: Settings{Profile: chatgpt.Profile, AuthKind: "chatgpt_oauth", BillingRoute: "subscription"}}, Secret: attempt.Credentials.AccessToken, Client: client})
	if err != nil {
		return out, err
	}
	if len(items) > maxCatalogItems {
		return out, ErrCatalogUnavailable
	}
	for _, item := range items {
		if item.ID != "" && len(item.ID) <= 200 && catalogText(item.ID) && catalogText(item.Name) && !item.Disabled && !strings.Contains(item.ID, attempt.Credentials.AccessToken) && !strings.Contains(item.Name, attempt.Credentials.AccessToken) {
			out.Models = append(out.Models, CatalogItem{ID: item.ID, Name: item.Name})
		}
	}
	return out, nil
}

func (s *Service) SaveChatGPT(ctx context.Context, session auth.Session, org, id, selected, request string) (Connection, error) {
	setup, err := s.ChatGPTSetup(ctx, session, org, id)
	if err != nil {
		return Connection{}, err
	}
	found := false
	for _, item := range setup.Models {
		found = found || item.ID == selected
	}
	if !found {
		return Connection{}, auth.ErrInvalid
	}
	var result Connection
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, actor domain.Actor) error {
		attempt, err := s.chatGPTSetupTx(ctx, tx, session, actor, id)
		if err != nil {
			return err
		}
		result = Connection{ID: domain.NewID(), OrgID: org, Kind: "model", Provider: "openai", Name: attempt.Name, Endpoint: chatgpt.Resource, Settings: Settings{Model: selected, Profile: chatgpt.Profile, AuthKind: "chatgpt_oauth", BillingRoute: "subscription", Namespace: attempt.Credentials.Email}, State: "healthy", Reason: "ChatGPT plan authorized; provider usage limits still apply", Capabilities: map[string]domain.Capability{}, CredentialVersion: 1, Version: 1}
		if attempt.ConnectionID != "" {
			result, err = load(ctx, tx, org, attempt.ConnectionID)
			if err != nil {
				return err
			}
			if result.Version != attempt.Version || result.State == "revoked" || result.Settings.Model != selected {
				return auth.ErrConflict
			}
			result.Version++
			result.CredentialVersion++
			result.State, result.Reason = "healthy", "ChatGPT plan reconnected; resume paused runs"
		} else {
			settings, _ := json.Marshal(result.Settings)
			if _, err = tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,reason,verified_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,now())`, org, result.ID, result.Kind, result.Provider, result.Name, result.Endpoint, settings, result.State, result.Reason); err != nil {
				return err
			}
		}
		raw, _ := json.Marshal(attempt.Credentials)
		if err = s.putSecret(ctx, tx, &result, string(raw)); err != nil {
			clear(raw)
			return err
		}
		clear(raw)
		if _, err = tx.Exec(ctx, `UPDATE connections SET state=$3,reason=$4,credential_version=$5,version=$6,verified_at=now() WHERE org_id=$1 AND id=$2`, org, result.ID, result.State, result.Reason, result.CredentialVersion, result.Version); err != nil {
			return err
		}
		route := budget.Route{ConnectionID: result.ID, Model: selected, Name: "default", Mode: "quota", MaxInputTokens: model.MaxRequestBytes + 8192, MaxOutputTokens: budget.ChatGPTMaxOutputTokens, MaxMilliseconds: 600_000, MaxRequests: 1, Qualified: true, QualificationRef: "chatgpt-plan/v1", Version: 1}
		config, _ := json.Marshal(route)
		if _, err = tx.Exec(ctx, `INSERT INTO budget_routes(org_id,connection_id,model,name,config,version) VALUES($1,$2,$3,'default',$4,1) ON CONFLICT DO NOTHING`, org, result.ID, selected, config); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM chatgpt_setups WHERE org_id=$1 AND id=$2`, org, id); err != nil {
			return err
		}
		return record(ctx, tx, actor, "connection.chatgpt_connected", result.ID, request, result.Version)
	})
	return result, err
}

func (s *Service) chatGPTCredentialsTx(ctx context.Context, tx pgx.Tx, c Connection, credentials *chatgpt.Credentials) error {
	if !c.Settings.ChatGPT() || c.Provider != "openai" || c.Endpoint != chatgpt.Resource || c.Route != nil || c.SecretID == nil {
		return chatgpt.ErrRefresh
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT envelope FROM connection_secrets WHERE org_id=$1 AND connection_id=$2 AND version=$3`, c.OrgID, c.ID, c.CredentialVersion).Scan(&raw); err != nil {
		return err
	}
	var envelope secrets.Envelope
	if json.Unmarshal(raw, &envelope) != nil {
		return chatgpt.ErrRefresh
	}
	plain, err := s.vault.OpenContext(ctx, secrets.Binding{OrgID: c.OrgID, ConnectionID: c.ID, Version: c.CredentialVersion}, envelope)
	if err != nil {
		return err
	}
	defer clear(plain)
	if json.Unmarshal(plain, credentials) != nil {
		return chatgpt.ErrRefresh
	}
	return nil
}

func (s *Service) resolveChatGPTTx(_ context.Context, _ pgx.Tx, resolved Resolved) (Resolved, error) {
	var credentials chatgpt.Credentials
	err := json.Unmarshal([]byte(resolved.Secret), &credentials)
	resolved.Secret = ""
	if err != nil || !resolved.Connection.ModelReady() || !s.chatGPT.Accepts(credentials) || !credentials.Expiry.After(time.Now().Add(15*time.Second)) {
		return resolved, chatgpt.ErrRefresh
	}
	resolved.Secret = credentials.AccessToken
	return resolved, nil
}

func (s *Service) RefreshChatGPT(ctx context.Context, org, id string) error {
	if !s.chatGPT.Available() {
		return chatgpt.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
	defer cancel()
	needsRefresh := false
	if err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		c, err := load(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !c.Settings.ChatGPT() {
			return nil
		}
		if c.State == "revoked" {
			return ErrRevoked
		}
		var credentials chatgpt.Credentials
		if err := s.chatGPTCredentialsTx(ctx, tx, c, &credentials); err != nil {
			return err
		}
		needsRefresh = !s.chatGPT.Accepts(credentials) || !credentials.Expiry.After(time.Now().Add(2*time.Minute))
		return nil
	}); err != nil {
		return err
	}
	if !needsRefresh {
		return nil
	}
	client, err := s.chatGPTClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var terminal error
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM connections WHERE org_id=$1 AND id=$2 FOR UPDATE`, org, id); err != nil {
			return err
		}
		c, err := load(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !c.Settings.ChatGPT() {
			return nil
		}
		if c.State == "revoked" {
			return ErrRevoked
		}
		var credentials chatgpt.Credentials
		if err = s.chatGPTCredentialsTx(ctx, tx, c, &credentials); err != nil {
			return err
		}
		if s.chatGPT.Accepts(credentials) && credentials.Expiry.After(time.Now().Add(2*time.Minute)) {
			return nil
		}
		refreshContext, stop := context.WithTimeout(ctx, 8*time.Second)
		defer stop()
		next, err := client.Refresh(refreshContext, credentials)
		if errors.Is(err, chatgpt.ErrRefresh) || errors.Is(err, chatgpt.ErrPermission) {
			terminal = err
			credentials.AccessToken, credentials.RefreshToken, credentials.IDToken = "", "", ""
			raw, _ := json.Marshal(credentials)
			if err = s.putSecret(ctx, tx, &c, string(raw)); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE connections SET state='degraded',reason='Reconnect your ChatGPT account',version=version+1 WHERE org_id=$1 AND id=$2`, org, id)
			return err
		}
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(next)
		defer clear(raw)
		return s.putSecret(ctx, tx, &c, string(raw))
	})
	if err != nil {
		return err
	}
	return terminal
}
