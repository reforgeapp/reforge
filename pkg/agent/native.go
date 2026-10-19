package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
)

const NativeCodexVersion = "0.163.0"
const NativeClaudeVersion = "2.1.302"

type NativeScope struct {
	OrgID        string `json:"org_id"`
	UserID       string `json:"user_id"`
	ConnectionID string `json:"connection_id"`
	RunnerID     string `json:"runner_id"`
	Provider     string `json:"provider"`
	AccountID    string `json:"account_id,omitempty"`
}

func (s NativeScope) Valid() bool {
	return auth.ValidID(s.OrgID) && auth.ValidID(s.UserID) && auth.ValidID(s.ConnectionID) && auth.ValidID(s.RunnerID) && (s.Provider == "codex" || s.Provider == "claude_code") && len(s.AccountID) <= 512
}

func (s NativeScope) Key() string {
	sum := sha256.Sum256([]byte(s.OrgID + "/" + s.UserID + "/" + s.ConnectionID + "/" + s.Provider))
	return hex.EncodeToString(sum[:])
}

type NativeAction struct {
	Kind    string `json:"kind"`
	LoginID string `json:"login_id,omitempty"`
	Code    string `json:"code,omitempty"`
	Model   string `json:"model,omitempty"`
}

func (a NativeAction) Valid() bool {
	switch a.Kind {
	case "verify":
		return a.Code == "" && a.LoginID == "" && a.Model != "" && len(a.Model) <= 200
	case "login", "status", "logout", "models":
		return a.Model == "" && a.Code == "" && (a.LoginID == "" || auth.ValidID(a.LoginID))
	case "code":
		return a.Model == "" && auth.ValidID(a.LoginID) && len(a.Code) > 0 && len(a.Code) <= 2048 && !strings.ContainsAny(a.Code, "\r\n\x00")
	}
	return false
}

func (NativeAction) String() string       { return "native agent action [redacted]" }
func (NativeAction) GoString() string     { return "native agent action [redacted]" }
func (NativeAction) LogValue() slog.Value { return slog.StringValue("native agent action [redacted]") }

type NativeAccount struct {
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
	Plan  string `json:"plan,omitempty"`
}

type NativeState struct {
	State    string         `json:"state"`
	Login    *Login         `json:"login,omitempty"`
	Account  *NativeAccount `json:"account,omitempty"`
	Models   []model.Model  `json:"models,omitempty"`
	Version  string         `json:"version,omitempty"`
	Reason   string         `json:"reason,omitempty"`
	CodeOnly bool           `json:"code_required,omitempty"`
	Model    string         `json:"model,omitempty"`
}

type NativeHost interface {
	Exec(context.Context, []string, io.Reader, io.Writer, io.Writer) (int, error)
	Close() error
}

type NativeOpen func(context.Context, NativeScope) (NativeHost, error)

type NativeManager struct {
	ctx   context.Context
	open  NativeOpen
	mu    sync.Mutex
	jobs  map[string]*nativeLogin
	busy  map[string]bool
	slots chan struct{}
}

type nativeLogin struct {
	scope  NativeScope
	state  NativeState
	cancel context.CancelFunc
	input  chan string
	done   chan struct{}
	until  time.Time
}

func NewNativeManager(ctx context.Context, open NativeOpen) *NativeManager {
	return &NativeManager{ctx: ctx, open: open, jobs: map[string]*nativeLogin{}, busy: map[string]bool{}, slots: make(chan struct{}, 2)}
}

func (m *NativeManager) Configured() bool { return m != nil && m.open != nil }

func (m *NativeManager) acquire(scope NativeScope) (func(), error) {
	if !m.Configured() || !scope.Valid() {
		return nil, ErrDisabled
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy[scope.Key()] {
		return nil, &domain.ProviderError{Kind: "quota", Message: "Agent is busy; waiting for its current work", RetryAfter: time.Minute}
	}
	select {
	case m.slots <- struct{}{}:
	default:
		return nil, &domain.ProviderError{Kind: "quota", Message: "Agent runtime is busy; retries automatically", RetryAfter: time.Minute}
	}
	m.busy[scope.Key()] = true
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.busy, scope.Key())
			m.mu.Unlock()
			<-m.slots
		})
	}, nil
}

func (m *NativeManager) Action(ctx context.Context, scope NativeScope, action NativeAction) (NativeState, error) {
	if !m.Configured() || !scope.Valid() || !action.Valid() {
		return NativeState{}, ErrDisabled
	}
	key := scope.Key()
	m.mu.Lock()
	for id, job := range m.jobs {
		if time.Now().After(job.until) {
			job.cancel()
			delete(m.jobs, id)
		}
	}
	job := m.jobs[key]
	if action.Kind == "logout" && job != nil && (job.state.State == "starting" || job.state.State == "pending") {
		job.cancel()
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return NativeState{}, ctx.Err()
		case <-job.done:
			return m.Action(ctx, scope, action)
		}
	}
	if action.LoginID != "" && (job == nil || job.state.Login == nil || job.state.Login.ID != action.LoginID) {
		m.mu.Unlock()
		return NativeState{}, ErrDenied
	}
	if action.Kind == "code" {
		if job.state.State != "pending" || !job.state.CodeOnly {
			m.mu.Unlock()
			return NativeState{}, ErrDenied
		}
		select {
		case job.input <- action.Code:
		default:
			m.mu.Unlock()
			return NativeState{}, ErrDenied
		}
		state := job.state
		m.mu.Unlock()
		return state, nil
	}
	if action.Kind == "status" && job != nil {
		state := job.state
		m.mu.Unlock()
		return state, nil
	}
	if action.Kind == "login" && job != nil && (job.state.State == "starting" || job.state.State == "pending") {
		state := job.state
		m.mu.Unlock()
		return state, nil
	}
	m.mu.Unlock()
	release, err := m.acquire(scope)
	if err != nil {
		return NativeState{}, err
	}
	if action.Kind == "login" {
		work, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		job = &nativeLogin{scope: scope, state: NativeState{State: "starting", Login: &Login{ID: domain.NewID()}}, cancel: cancel, input: make(chan string, 1), done: make(chan struct{}), until: time.Now().Add(15 * time.Minute)}
		m.mu.Lock()
		m.jobs[key] = job
		state := job.state
		m.mu.Unlock()
		go func() {
			defer close(job.done)
			defer release()
			defer cancel()
			host, err := m.open(work, scope)
			if err == nil {
				defer host.Close()
				if scope.Provider == "codex" {
					err = m.codexLogin(work, host, job)
				} else {
					err = m.claudeLogin(work, host, job)
				}
			}
			if err != nil {
				m.updateLogin(job, func(s *NativeState) {
					s.State, s.Reason = "failed", "Sign-in did not complete; try again"
					s.Login.URL, s.Login.Code = "", ""
				})
			}
		}()
		return state, nil
	}
	defer release()
	host, err := m.open(ctx, scope)
	if err != nil {
		return NativeState{}, ErrDisabled
	}
	defer host.Close()
	var state NativeState
	if action.Kind == "verify" {
		var err error
		if scope.Provider == "codex" {
			state, err = codexState(ctx, host, "status")
		} else {
			state, err = claudeState(ctx, host, "status")
		}
		if err != nil || state.Account == nil || scope.AccountID == "" || state.Account.ID != scope.AccountID {
			return NativeState{}, ErrDenied
		}
		in := model.Turn{OperationID: domain.NewID(), Model: action.Model, Messages: []model.Message{{Role: "user", Text: "Reply ready with no tool calls."}}, MaxOutputTokens: 64, TimeoutMS: 120000}
		var verified model.TurnResult
		if scope.Provider == "codex" {
			verified, err = codexTurn(ctx, host, scope, in)
		} else {
			verified, err = claudeTurn(ctx, host, scope, in)
		}
		if err == nil && !verified.Usage.Known {
			err = ErrUncertain
		}
		if err != nil {
			return NativeState{}, err
		}
		state.State, state.Model = "ready", action.Model
		return state, nil
	}
	if scope.Provider == "codex" {
		state, err = codexState(ctx, host, action.Kind)
	} else {
		state, err = claudeState(ctx, host, action.Kind)
	}
	if err == nil && state.Account != nil && scope.AccountID != "" && state.Account.ID != scope.AccountID {
		return NativeState{}, ErrDenied
	}
	if err == nil && action.Kind == "logout" {
		cleaner, ok := host.(interface{ Forget(context.Context) error })
		if !ok {
			return NativeState{}, ErrDisabled
		}
		if err := cleaner.Forget(ctx); err != nil {
			return NativeState{}, err
		}
		m.mu.Lock()
		delete(m.jobs, key)
		m.mu.Unlock()
	}
	return state, err
}

func (m *NativeManager) updateLogin(job *nativeLogin, update func(*NativeState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.jobs[job.scope.Key()] == job {
		state := job.state
		if state.Login != nil {
			login := *state.Login
			state.Login = &login
		}
		update(&state)
		job.state = state
	}
}

func (m *NativeManager) Turn(ctx context.Context, scope NativeScope, in model.Turn) (model.TurnResult, error) {
	if !in.Valid() {
		return model.TurnResult{}, ErrDenied
	}
	release, err := m.acquire(scope)
	if err != nil {
		return model.TurnResult{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(in.TimeoutMS)*time.Millisecond)
	defer cancel()
	host, err := m.open(ctx, scope)
	if err != nil {
		return model.TurnResult{}, &domain.ProviderError{Kind: "quota", Message: "Agent runtime temporarily unavailable; retries automatically", RetryAfter: time.Minute}
	}
	defer host.Close()
	if scope.Provider == "codex" {
		return codexTurn(ctx, host, scope, in)
	}
	return claudeTurn(ctx, host, scope, in)
}

func nativeURL(raw, provider string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	if provider == "codex" {
		return u.Host == "auth.openai.com" || u.Host == "chatgpt.com"
	}
	return u.Host == "claude.ai" || u.Host == "claude.com" || u.Host == "console.anthropic.com" || u.Host == "platform.claude.com"
}

func nativeFailure(kind string, uncertain bool) error {
	if kind == "usageLimitExceeded" || kind == "rate_limit_error" || kind == "rate_limit" || kind == "quota" {
		return &domain.ProviderError{Kind: "quota", Message: "Subscription allowance reached; retries automatically", RetryAfter: 15 * time.Minute, Uncertain: uncertain}
	}
	if kind == "unauthorized" || kind == "authentication_error" || kind == "auth" {
		return &domain.ProviderError{Kind: "auth", Message: "Sign in to your agent account again", Uncertain: uncertain}
	}
	return &domain.ProviderError{Kind: "provider", Message: "Agent could not complete this turn", Uncertain: uncertain}
}

type nativeTransport struct {
	input  *io.PipeWriter
	output *io.PipeReader
	cancel context.CancelFunc
	once   sync.Once
}

func openNativeTransport(ctx context.Context, host NativeHost, args []string) Transport {
	ctx, cancel := context.WithCancel(ctx)
	in, input := io.Pipe()
	output, out := io.Pipe()
	t := &nativeTransport{input: input, output: output, cancel: cancel}
	go func() {
		defer in.Close()
		defer out.Close()
		_, err := host.Exec(ctx, args, in, out, io.Discard)
		_ = out.CloseWithError(err)
	}()
	return t
}

func (t *nativeTransport) Read(b []byte) (int, error)  { return t.output.Read(b) }
func (t *nativeTransport) Write(b []byte) (int, error) { return t.input.Write(b) }
func (t *nativeTransport) Close() error {
	t.once.Do(func() { t.cancel(); _ = t.input.Close(); _ = t.output.Close() })
	return nil
}
