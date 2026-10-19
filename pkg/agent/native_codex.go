package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
)

func nativeCodex(ctx context.Context, host NativeHost) (*rpc, error) {
	r := newRPC(openNativeTransport(ctx, host, []string{"/usr/local/bin/codex", "app-server", "--listen", "stdio://", "-c", `forced_login_method="chatgpt"`, "-c", `cli_auth_credentials_store="file"`, "-c", `history.persistence="none"`, "-c", `features.remote_plugin=false`, "-c", `features.shell_tool=false`, "-c", `features.multi_agent=false`, "-c", `web_search="disabled"`}))
	var initialized struct {
		UserAgent string `json:"userAgent"`
	}
	if err := r.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "reforge", "title": "Reforge", "version": "1"}, "capabilities": map[string]any{"experimentalApi": true}}, &initialized); err != nil {
		r.close()
		return nil, err
	}
	if !strings.Contains(initialized.UserAgent, "/"+NativeCodexVersion) {
		r.close()
		return nil, ErrDisabled
	}
	if err := r.notify(ctx, "initialized", nil); err != nil {
		r.close()
		return nil, err
	}
	return r, nil
}

func codexAccount(ctx context.Context, r *rpc) (*NativeAccount, error) {
	var response struct {
		Account *struct {
			Type  string `json:"type"`
			Email string `json:"email"`
			Plan  string `json:"planType"`
		} `json:"account"`
		Routing *struct {
			AccountID string `json:"chatgptAccountId"`
		} `json:"workspaceRouting"`
	}
	if err := r.call(ctx, "account/read", map[string]bool{"refreshToken": true}, &response); err != nil {
		return nil, err
	}
	if response.Account == nil {
		return nil, nil
	}
	if response.Account.Type != "chatgpt" {
		return nil, ErrDenied
	}
	id := response.Account.Email
	if response.Routing != nil && response.Routing.AccountID != "" {
		id = response.Routing.AccountID + "/" + id
	}
	if id == "" || len(id) > 512 || len(response.Account.Email) > 320 {
		return nil, ErrProtocol
	}
	return &NativeAccount{ID: id, Email: response.Account.Email, Plan: response.Account.Plan}, nil
}

func codexModels(ctx context.Context, r *rpc) ([]model.Model, error) {
	var models []model.Model
	cursor := ""
	for range 10 {
		var response struct {
			Data []struct {
				ID   string `json:"model"`
				Name string `json:"displayName"`
			} `json:"data"`
			Next string `json:"nextCursor"`
		}
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := r.call(ctx, "model/list", params, &response); err != nil {
			return nil, err
		}
		for _, item := range response.Data {
			if item.ID != "" && len(item.ID) <= 200 && len(item.Name) <= 300 {
				models = append(models, model.Model{ID: item.ID, Name: item.Name})
			}
		}
		if response.Next == "" {
			return models, nil
		}
		if response.Next == cursor {
			return nil, ErrProtocol
		}
		cursor = response.Next
	}
	return nil, ErrProtocol
}

func codexState(ctx context.Context, host NativeHost, action string) (NativeState, error) {
	r, err := nativeCodex(ctx, host)
	if err != nil {
		return NativeState{}, err
	}
	defer r.close()
	state := NativeState{State: "disconnected", Version: NativeCodexVersion}
	if action == "logout" {
		return state, r.call(ctx, "account/logout", nil, nil)
	}
	state.Account, err = codexAccount(ctx, r)
	if err != nil || state.Account == nil {
		return state, err
	}
	state.State = "connected"
	if action == "models" {
		state.Models, err = codexModels(ctx, r)
	}
	return state, err
}

func (m *NativeManager) codexLogin(ctx context.Context, host NativeHost, job *nativeLogin) error {
	r, err := nativeCodex(ctx, host)
	if err != nil {
		return err
	}
	defer r.close()
	var response struct {
		Type string `json:"type"`
		ID   string `json:"loginId"`
		URL  string `json:"verificationUrl"`
		Code string `json:"userCode"`
	}
	if err = r.call(ctx, "account/login/start", map[string]string{"type": "chatgptDeviceCode"}, &response); err != nil {
		return err
	}
	if response.Type != "chatgptDeviceCode" || response.ID == "" || !nativeURL(response.URL, "codex") || response.Code == "" || len(response.Code) > 64 {
		return ErrProtocol
	}
	m.updateLogin(job, func(s *NativeState) {
		s.State, s.Version = "pending", NativeCodexVersion
		s.Login.URL, s.Login.Code = response.URL, response.Code
	})
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.done:
			return ErrUncertain
		case event := <-r.incoming:
			if len(event.ID) > 0 {
				_ = r.reject(ctx, event.ID)
				return ErrDenied
			}
			if event.Method != "account/login/completed" {
				continue
			}
			var completed struct {
				ID      string `json:"loginId"`
				Success bool   `json:"success"`
			}
			if json.Unmarshal(event.Params, &completed) != nil || completed.ID != response.ID || !completed.Success {
				return ErrDenied
			}
			account, err := codexAccount(ctx, r)
			if err != nil || account == nil {
				return ErrDenied
			}
			m.updateLogin(job, func(s *NativeState) {
				s.State, s.Account = "connected", account
				s.Login.URL, s.Login.Code = "", ""
			})
			return nil
		}
	}
}

func codexQuota(ctx context.Context, r *rpc) error {
	var response struct {
		Limits struct {
			Primary *struct {
				Used  float64 `json:"usedPercent"`
				Reset int64   `json:"resetsAt"`
			} `json:"primary"`
			Secondary *struct {
				Used  float64 `json:"usedPercent"`
				Reset int64   `json:"resetsAt"`
			} `json:"secondary"`
		} `json:"rateLimits"`
	}
	if err := r.call(ctx, "account/rateLimits/read", nil, &response); err != nil {
		return err
	}
	for _, limit := range []*struct {
		Used  float64 `json:"usedPercent"`
		Reset int64   `json:"resetsAt"`
	}{response.Limits.Primary, response.Limits.Secondary} {
		if limit != nil && limit.Used >= 100 {
			return &domain.ProviderError{Kind: "quota", Message: "Subscription allowance reached; retries automatically", RetryAfter: min(max(time.Until(time.Unix(limit.Reset, 0)), time.Minute), 6*time.Hour)}
		}
	}
	return nil
}

func codexTurn(ctx context.Context, host NativeHost, scope NativeScope, in model.Turn) (model.TurnResult, error) {
	r, err := nativeCodex(ctx, host)
	if err != nil {
		return model.TurnResult{}, err
	}
	defer r.close()
	account, err := codexAccount(ctx, r)
	if err != nil {
		return model.TurnResult{}, nativeFailure("provider", false)
	}
	if account == nil || scope.AccountID == "" || account.ID != scope.AccountID {
		return model.TurnResult{}, nativeFailure("auth", false)
	}
	if err = codexQuota(ctx, r); err != nil {
		return model.TurnResult{}, err
	}
	prompt, err := nativePrompt(in)
	if err != nil {
		return model.TurnResult{}, err
	}
	var thread threadResponse
	params := map[string]any{"model": in.Model, "modelProvider": "openai", "allowProviderModelFallback": false, "cwd": "/workspace", "approvalPolicy": "untrusted", "approvalsReviewer": "user", "sandbox": "read-only", "ephemeral": true, "dynamicTools": []any{}, "config": map[string]any{"features.shell_tool": false, "features.multi_agent": false, "web_search": "disabled", "forced_login_method": "chatgpt"}}
	if err = r.call(ctx, "thread/start", params, &thread); err != nil {
		return model.TurnResult{}, err
	}
	if thread.Thread.ID == "" || thread.Model != in.Model || thread.ModelProvider != "openai" || thread.ApprovalPolicy != "untrusted" || thread.ApprovalsReviewer != "user" || thread.Sandbox.Type != "readOnly" || thread.Sandbox.NetworkAccess {
		return model.TurnResult{}, ErrDenied
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err = r.call(ctx, "turn/start", map[string]any{"threadId": thread.Thread.ID, "model": in.Model, "outputSchema": json.RawMessage(nativeReplySchema), "input": []any{map[string]any{"type": "text", "text": prompt, "text_elements": []any{}}}}, &turn); err != nil {
		return model.TurnResult{}, nativeFailure("provider", true)
	}
	if turn.Turn.ID == "" {
		return model.TurnResult{}, nativeFailure("provider", true)
	}
	text := ""
	usage := model.Usage{Source: "codex.runtime"}
	for {
		select {
		case <-ctx.Done():
			return model.TurnResult{}, nativeFailure("provider", true)
		case <-r.done:
			return model.TurnResult{}, nativeFailure("provider", true)
		case event := <-r.incoming:
			if len(event.ID) > 0 {
				_ = r.reject(ctx, event.ID)
				return model.TurnResult{}, nativeFailure("provider", true)
			}
			var header struct {
				Thread string `json:"threadId"`
				Turn   string `json:"turnId"`
			}
			if json.Unmarshal(event.Params, &header) != nil || header.Thread != "" && header.Thread != thread.Thread.ID || header.Turn != "" && header.Turn != turn.Turn.ID {
				return model.TurnResult{}, nativeFailure("provider", true)
			}
			switch event.Method {
			case "item/completed":
				var item struct {
					Item struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"item"`
				}
				if json.Unmarshal(event.Params, &item) != nil {
					return model.TurnResult{}, nativeFailure("provider", true)
				}
				if item.Item.Type == "agentMessage" {
					text = item.Item.Text
				}
			case "thread/tokenUsage/updated":
				var update struct {
					Usage struct {
						Total struct {
							Input  int64 `json:"inputTokens"`
							Output int64 `json:"outputTokens"`
							Cache  int64 `json:"cachedInputTokens"`
						} `json:"total"`
					} `json:"tokenUsage"`
				}
				if json.Unmarshal(event.Params, &update) == nil {
					usage.InputTokens, usage.OutputTokens, usage.CacheTokens = update.Usage.Total.Input, update.Usage.Total.Output, update.Usage.Total.Cache
					usage.Known = usage.InputTokens >= 0 && usage.OutputTokens >= 0 && usage.CacheTokens >= 0 && usage.InputTokens+usage.OutputTokens > 0
				}
			case "turn/completed":
				var terminal struct {
					Turn struct {
						ID     string `json:"id"`
						Status string `json:"status"`
						Error  *struct {
							Info json.RawMessage `json:"codexErrorInfo"`
						} `json:"error"`
					} `json:"turn"`
				}
				if json.Unmarshal(event.Params, &terminal) != nil || terminal.Turn.ID != turn.Turn.ID {
					return model.TurnResult{}, nativeFailure("provider", true)
				}
				if terminal.Turn.Status != "completed" {
					if terminal.Turn.Error != nil && strings.Contains(string(terminal.Turn.Error.Info), "usageLimitExceeded") {
						return model.TurnResult{}, nativeFailure("usageLimitExceeded", true)
					}
					return model.TurnResult{}, nativeFailure("provider", true)
				}
				return nativeReply(in, []byte(text), usage)
			}
		}
	}
}
