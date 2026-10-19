package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/reforgeapp/reforge/pkg/model"
)

type nativeBuffer struct {
	mu    sync.Mutex
	data  bytes.Buffer
	limit int
}

func (b *nativeBuffer) Write(raw []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.data.Len()+len(raw) > b.limit {
		return 0, ErrProtocol
	}
	return b.data.Write(raw)
}

func (b *nativeBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data.Bytes()...)
}

func claudeAccount(ctx context.Context, host NativeHost) (*NativeAccount, error) {
	output := &nativeBuffer{limit: 64 << 10}
	code, err := host.Exec(ctx, []string{"/usr/local/bin/claude", "auth", "status"}, nil, output, io.Discard)
	if err != nil {
		return nil, err
	}
	var response struct {
		LoggedIn bool   `json:"loggedIn"`
		Method   string `json:"authMethod"`
		Email    string `json:"email"`
		OrgID    string `json:"orgId"`
		Plan     string `json:"subscriptionType"`
	}
	if json.Unmarshal(output.Bytes(), &response) != nil {
		return nil, ErrProtocol
	}
	if code != 0 || !response.LoggedIn {
		return nil, nil
	}
	if response.Method != "claude.ai" || response.Email == "" || len(response.Email) > 320 || len(response.OrgID) > 128 || len(response.Plan) > 100 {
		return nil, ErrDenied
	}
	return &NativeAccount{ID: response.OrgID + "/" + response.Email, Email: response.Email, Plan: response.Plan}, nil
}

func claudeState(ctx context.Context, host NativeHost, action string) (NativeState, error) {
	state := NativeState{State: "disconnected", Version: NativeClaudeVersion}
	if action == "logout" {
		code, err := host.Exec(ctx, []string{"/usr/local/bin/claude", "auth", "logout"}, nil, io.Discard, io.Discard)
		if err == nil && code != 0 {
			err = ErrDenied
		}
		return state, err
	}
	var err error
	state.Account, err = claudeAccount(ctx, host)
	if err != nil || state.Account == nil {
		return state, err
	}
	state.State = "connected"
	if action == "models" {
		state.Models = []model.Model{{ID: "sonnet", Name: "Sonnet"}, {ID: "opus", Name: "Opus"}, {ID: "haiku", Name: "Haiku"}}
	}
	return state, nil
}

var nativeAuthURL = regexp.MustCompile(`https://[^\s\x1b<>"']+`)

type claudeLoginOutput struct {
	mu      sync.Mutex
	buffer  []byte
	manager *NativeManager
	job     *nativeLogin
}

func (o *claudeLoginOutput) Write(raw []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.buffer)+len(raw) > 32<<10 {
		return 0, ErrProtocol
	}
	o.buffer = append(o.buffer, raw...)
	for _, bounds := range nativeAuthURL.FindAllIndex(o.buffer, -1) {
		if bounds[1] == len(o.buffer) {
			continue
		}
		value := string(o.buffer[bounds[0]:bounds[1]])
		if nativeURL(value, "claude_code") {
			o.manager.updateLogin(o.job, func(s *NativeState) {
				s.State, s.Version, s.CodeOnly = "pending", NativeClaudeVersion, true
				s.Login.URL = value
			})
		}
	}
	return len(raw), nil
}

func (m *NativeManager) claudeLogin(ctx context.Context, host NativeHost, job *nativeLogin) error {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = writer.Close()
		case <-done:
		case code := <-job.input:
			_, _ = io.WriteString(writer, code+"\n")
			_ = writer.Close()
		}
	}()
	output := &claudeLoginOutput{manager: m, job: job}
	code, err := host.Exec(ctx, []string{"/usr/local/bin/claude", "auth", "login"}, reader, output, output)
	if err != nil || code != 0 {
		return ErrDenied
	}
	account, err := claudeAccount(ctx, host)
	if err != nil || account == nil {
		return ErrDenied
	}
	m.updateLogin(job, func(s *NativeState) {
		s.State, s.Account, s.CodeOnly = "connected", account, false
		s.Login.URL = ""
	})
	return nil
}

func claudeTurn(ctx context.Context, host NativeHost, scope NativeScope, in model.Turn) (model.TurnResult, error) {
	account, err := claudeAccount(ctx, host)
	if err != nil {
		return model.TurnResult{}, nativeFailure("provider", false)
	}
	if account == nil || scope.AccountID == "" || account.ID != scope.AccountID {
		return model.TurnResult{}, nativeFailure("auth", false)
	}
	prompt, err := nativePrompt(in)
	if err != nil {
		return model.TurnResult{}, err
	}
	output := &nativeBuffer{limit: 1 << 20}
	args := []string{"/usr/local/bin/claude", "-p", "--model", in.Model, "--output-format", "json", "--json-schema", nativeReplySchema, "--max-turns", "1", "--tools", "", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--setting-sources", "", "--no-session-persistence", "--system-prompt", nativeInstructions}
	code, err := host.Exec(ctx, args, strings.NewReader(prompt), output, io.Discard)
	if err != nil {
		return model.TurnResult{}, nativeFailure("provider", true)
	}
	var response struct {
		Type       string          `json:"type"`
		Subtype    string          `json:"subtype"`
		IsError    bool            `json:"is_error"`
		Result     string          `json:"result"`
		Structured json.RawMessage `json:"structured_output"`
		Errors     []string        `json:"errors"`
		Error      *struct {
			Type string `json:"type"`
		} `json:"error"`
		Usage struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			Cache      int64 `json:"cache_read_input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(output.Bytes(), &response) != nil || response.Type != "result" {
		return model.TurnResult{}, nativeFailure("provider", true)
	}
	if code != 0 || response.IsError || response.Subtype != "success" {
		kind := "provider"
		if response.Error != nil {
			kind = response.Error.Type
		} else {
			message := strings.ToLower(response.Result + " " + strings.Join(response.Errors, " "))
			if strings.Contains(message, "rate limit") || strings.Contains(message, "usage limit") || strings.Contains(message, "you've hit your limit") {
				kind = "quota"
			}
		}
		return model.TurnResult{}, nativeFailure(kind, true)
	}
	u := response.Usage
	usage := model.Usage{InputTokens: u.Input + u.Cache + u.CacheWrite, OutputTokens: u.Output, CacheTokens: u.Cache, CacheCreationTokens: u.CacheWrite, Source: "claude_code.runtime", Known: u.Input >= 0 && u.Output >= 0 && u.Cache >= 0 && u.CacheWrite >= 0 && u.Input+u.Output+u.Cache+u.CacheWrite > 0}
	if len(response.Structured) == 0 {
		response.Structured = []byte(response.Result)
	}
	return nativeReply(in, response.Structured, usage)
}
