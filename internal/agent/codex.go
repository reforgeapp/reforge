package agent

import (
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/sandbox"
)

type threadResponse struct {
	Thread struct {
		ID string `json:"id"`
	} `json:"thread"`
	Model             string `json:"model"`
	ModelProvider     string `json:"modelProvider"`
	ApprovalPolicy    string `json:"approvalPolicy"`
	ApprovalsReviewer string `json:"approvalsReviewer"`
	Sandbox           struct {
		Type          string `json:"type"`
		NetworkAccess bool   `json:"networkAccess"`
	} `json:"sandbox"`
}
type Codex struct {
	closed          bool
	actions         sync.Mutex
	authChanged     bool
	activeEffects   int
	pendingRequests map[string]bool
	config          CodexConfig
	mu              sync.Mutex
	setup           sync.Mutex
	rpc             *rpc
	request         Request
	session         string
	thread          string
	turn            string
	state           string
	events          chan Event
	terminal        chan struct{}
	cancel          context.CancelFunc
	runctx          context.Context
	calls           map[string]bool
	approvals       map[string]bool
	toolCount       int
	ready           chan struct{}
	streaming       bool
	loginStatusID   string
	loginStatus     string
	loginID         string
}

func NewCodex(config CodexConfig) (*Codex, error) {
	b := config.Binding
	if !auth.ValidID(b.OrgID) || !auth.ValidID(b.ConnectionID) || !auth.ValidID(b.RunnerID) || b.ConnectionVersion < 1 || b.CredentialVersion < 1 || b.AccountID == "" || b.Model == "" || b.RuntimeDigest == "" || b.Deployment == "" || config.Open == nil {
		return nil, ErrDisabled
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Minute
	}
	if config.Timeout < time.Second || config.Timeout > time.Hour {
		return nil, ErrDisabled
	}
	if config.MaxToolCalls == 0 {
		config.MaxToolCalls = 32
	}
	if config.MaxToolCalls < 1 || config.MaxToolCalls > 128 {
		return nil, ErrDisabled
	}
	return &Codex{config: config, state: "idle", pendingRequests: map[string]bool{}}, nil
}
func (c *Codex) qualify(ctx context.Context, custodyOnly bool) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ErrDisabled
	}
	if c.config.Qualify == nil {
		return ErrDisabled
	}
	q, err := c.config.Qualify(ctx, c.config.Binding)
	if err != nil || !qualified(q, c.config.Binding, custodyOnly) {
		return ErrDisabled
	}
	return nil
}
func (c *Codex) connect(ctx context.Context) error {
	c.setup.Lock()
	defer c.setup.Unlock()
	c.mu.Lock()
	existing := c.rpc
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ErrDisabled
	}
	if existing != nil {
		select {
		case <-existing.done:
			return ErrUncertain
		default:
			return nil
		}
	}
	if err := c.qualify(ctx, true); err != nil {
		return err
	}
	runtime, err := c.config.Open(ctx, c.config.Binding)
	if err != nil {
		return ErrDisabled
	}
	c.mu.Lock()
	closed = c.closed
	c.mu.Unlock()
	if closed || runtime.Transport == nil || runtime.Binding != c.config.Binding {
		if runtime.Transport != nil {
			_ = runtime.Transport.Close()
		}
		return ErrDisabled
	}
	r := newRPC(runtime.Transport)
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var result struct {
		UserAgent string `json:"userAgent"`
	}
	if err = r.call(probeCtx, "initialize", map[string]any{"clientInfo": map[string]any{"name": "reforge", "title": "Reforge", "version": "0.1.0"}, "capabilities": map[string]any{"experimentalApi": true}}, &result); err != nil {
		r.close()
		return err
	}
	if !strings.Contains(result.UserAgent, "/"+CodexVersion+" ") && result.UserAgent != "codex_cli_rs/"+CodexVersion {
		r.close()
		return ErrDisabled
	}
	if err = r.notify(probeCtx, "initialized", map[string]any{}); err != nil {
		r.close()
		return err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		r.close()
		return ErrDisabled
	}
	c.rpc = r
	c.mu.Unlock()
	go c.pump(r)
	return nil
}
func (c *Codex) account(ctx context.Context) error {
	var result struct {
		Account *struct {
			Type string `json:"type"`
		} `json:"account"`
	}
	if err := c.rpc.call(ctx, "account/read", map[string]bool{"refreshToken": false}, &result); err != nil {
		return err
	}
	if result.Account == nil {
		return ErrDisabled
	}
	if result.Account.Type != "chatgpt" {
		return ErrDenied
	}
	return nil
}
func (c *Codex) ProbeVersionAndAuth(ctx context.Context) (Probe, error) {
	p := Probe{Runtime: "codex", Version: CodexVersion, AuthState: "unknown", BillingRoute: CodexBillingRoute, Features: OfficialSupportMatrix()}
	if c.config.Qualify != nil {
		q, _ := c.config.Qualify(ctx, c.config.Binding)
		p.Features = qualificationFeatures(q, c.config.Binding)
	}
	if err := c.connect(ctx); err != nil {
		return p, err
	}
	if err := c.account(ctx); err != nil {
		return p, err
	}
	p.AuthState = "chatgpt"
	c.mu.Lock()
	changed := c.authChanged
	c.mu.Unlock()
	if changed {
		p.Features["auth_custody"] = disabledFeature("authentication changed", "rebuild the versioned binding after account qualification")
		return p, ErrDisabled
	}
	if err := c.qualify(ctx, false); err != nil {
		return p, err
	}
	p.Features["broker_tools"] = domain.Capability{State: domain.Supported, Scope: "qualified runtime binding", Reason: "only registered sandbox broker tools are eligible", Source: "dated controller qualification", LastChecked: time.Now().UTC()}
	p.Features["native_host_tools"] = disabledFeature("native command, file and permission approvals are denied", "use registered sandbox broker tools")
	return p, nil
}
func (c *Codex) authorize(ctx context.Context, effect Effect, perform func() error) error {
	if err := c.qualify(ctx, false); err != nil {
		return err
	}
	if c.config.Authorize == nil {
		return ErrDisabled
	}
	calls := 0
	var effectErr error
	err := c.config.Authorize(ctx, effect, func() error {
		calls++
		if calls != 1 {
			return ErrDenied
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		effectErr = perform()
		return effectErr
	})
	if effectErr != nil {
		return ErrUncertain
	}
	if err != nil {
		if calls > 0 {
			return ErrUncertain
		}
		return err
	}
	if calls != 1 {
		return ErrDenied
	}
	return nil
}
func (c *Codex) Start(ctx context.Context, request Request) (string, error) {
	c.actions.Lock()
	defer c.actions.Unlock()
	c.mu.Lock()
	changed := c.authChanged
	c.mu.Unlock()
	if changed {
		return "", ErrDisabled
	}
	if !auth.ValidID(request.JobID) || !auth.ValidID(request.AttemptID) || request.PolicyHash == "" || request.Model != c.config.Binding.Model || request.Workspace.ID == "" || request.Workspace.CommitSHA == "" || request.Workspace.Image == "" || request.Prompt == "" || len(request.Prompt) > 256<<10 || request.MaxTurns != 1 || c.config.Sandbox == nil {
		return "", ErrDenied
	}
	if err := c.qualify(ctx, false); err != nil {
		return "", err
	}
	if err := c.connect(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	if c.closed || c.state != "idle" {
		c.mu.Unlock()
		return "", ErrDenied
	}
	c.state = "starting"
	c.request = request
	c.session = domain.NewID()
	c.events = make(chan Event, 128)
	c.terminal = make(chan struct{})
	c.ready = make(chan struct{})
	c.calls = map[string]bool{}
	c.approvals = map[string]bool{}
	c.runctx, c.cancel = context.WithTimeout(ctx, c.config.Timeout)
	runctx := c.runctx
	session := c.session
	c.mu.Unlock()
	if err := c.account(runctx); err != nil {
		c.finish("failed", err)
		return "", err
	}
	effect := Effect{Binding: c.config.Binding, Request: request, OperationID: domain.NewID(), Kind: "turn/start"}
	err := c.authorize(runctx, effect, func() error {
		var result threadResponse
		params := map[string]any{"model": request.Model, "modelProvider": "openai", "cwd": "/workspace", "approvalPolicy": "untrusted", "approvalsReviewer": "user", "sandbox": "read-only", "ephemeral": true, "dynamicTools": toolDefinitions(), "developerInstructions": "Use only the reforge_read, reforge_patch and reforge_exec tools for repository work. Native commands and file modifications are denied.", "config": map[string]any{"forced_login_method": "chatgpt", "web_search": "disabled", "features.shell_tool": false, "features.unified_exec": false, "features.multi_agent": false}}
		if err := c.rpc.call(runctx, "thread/start", params, &result); err != nil {
			return err
		}
		if result.Thread.ID == "" || result.Model != request.Model || result.ModelProvider != "openai" || result.ApprovalPolicy != "untrusted" || result.ApprovalsReviewer != "user" || result.Sandbox.Type != "readOnly" || result.Sandbox.NetworkAccess {
			return ErrProtocol
		}
		c.mu.Lock()
		c.thread = result.Thread.ID
		c.mu.Unlock()
		var turn struct {
			Turn struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"turn"`
		}
		if err := c.rpc.call(runctx, "turn/start", map[string]any{"threadId": result.Thread.ID, "model": request.Model, "input": []any{map[string]any{"type": "text", "text": request.Prompt, "text_elements": []any{}}}}, &turn); err != nil {
			return err
		}
		if turn.Turn.ID == "" || turn.Turn.Status != "inProgress" {
			return ErrProtocol
		}
		c.mu.Lock()
		if c.turn != "" && c.turn != turn.Turn.ID {
			c.mu.Unlock()
			return ErrProtocol
		}
		c.turn = turn.Turn.ID
		if c.state == "starting" {
			c.state = "running"
		}
		c.mu.Unlock()
		return nil
	})
	if err != nil {
		c.finish("uncertain", err)
		c.rpc.close()
		return "", err
	}
	close(c.ready)
	go func() {
		select {
		case <-runctx.Done():
			select {
			case <-c.terminal:
				return
			default:
			}
			c.finish("uncertain", ErrUncertain)
			c.rpc.close()
		case <-c.terminal:
		}
	}()
	return session, nil
}
func (c *Codex) StreamEvents(ctx context.Context, session string, emit func(Event) error) error {
	c.mu.Lock()
	if session != c.session || emit == nil || c.streaming {
		c.mu.Unlock()
		return ErrDenied
	}
	c.streaming = true
	events, terminal := c.events, c.terminal
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.streaming = false; c.mu.Unlock() }()
	for {
		select {
		case <-ctx.Done():
			_ = c.Cancel(context.Background(), session)
			return ErrUncertain
		case event := <-events:
			if err := emit(event); err != nil {
				_ = c.Cancel(context.Background(), session)
				return ErrUncertain
			}
		case <-terminal:
			for {
				select {
				case event := <-events:
					if err := emit(event); err != nil {
						return ErrUncertain
					}
				default:
					c.mu.Lock()
					state := c.state
					c.mu.Unlock()
					if state == "completed" || state == "canceled" {
						return nil
					}
					return ErrUncertain
				}
			}
		}
	}
}
func (c *Codex) emit(event Event) bool {
	c.mu.Lock()
	events := c.events
	c.mu.Unlock()
	if events == nil {
		return true
	}
	select {
	case events <- event:
		return true
	default:
		c.finish("uncertain", ErrUncertain)
		return false
	}
}
func (c *Codex) finish(state string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.terminal == nil || c.state == "completed" || c.state == "canceled" || c.state == "uncertain" || c.state == "failed" || c.state == "quota" {
		return
	}
	previous := c.state
	c.state = state
	event := Event{Type: state, ID: c.turn, Data: json.RawMessage(`{"billing_route":"codex.chatgpt_subscription","usage_known":false}`)}
	select {
	case c.events <- event:
	default:
	}
	close(c.terminal)
	if c.cancel != nil && state != "completed" && (previous != "starting" || state == "uncertain") {
		c.cancel()
	}
}
func (c *Codex) Cancel(ctx context.Context, session string) error {
	c.mu.Lock()
	if session != c.session || c.rpc == nil {
		c.mu.Unlock()
		return ErrDenied
	}
	thread, turn, r := c.thread, c.turn, c.rpc
	cancel := c.cancel
	c.mu.Unlock()
	ctx, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	var err error
	if thread != "" && turn != "" {
		err = r.call(ctx, "turn/interrupt", map[string]string{"threadId": thread, "turnId": turn}, nil)
	} else {
		err = ErrUncertain
	}
	if err != nil {
		c.finish("uncertain", err)
		r.close()
		if cancel != nil {
			cancel()
		}
		return ErrUncertain
	}
	c.mu.Lock()
	terminal := c.terminal
	c.mu.Unlock()
	select {
	case <-terminal:
		return nil
	case <-ctx.Done():
		c.finish("uncertain", ErrUncertain)
		r.close()
		if cancel != nil {
			cancel()
		}
		return ErrUncertain
	}
}
func (c *Codex) ResumeIfSupported(ctx context.Context, session string) error {
	c.mu.Lock()
	if session != c.session || c.thread == "" || (c.state != "completed" && c.state != "canceled") {
		c.mu.Unlock()
		return ErrDenied
	}
	thread, request := c.thread, c.request
	c.mu.Unlock()
	return c.authorize(ctx, Effect{Binding: c.config.Binding, Request: request, Kind: "thread/resume", OperationID: domain.NewID()}, func() error {
		var result threadResponse
		if err := c.rpc.call(ctx, "thread/resume", map[string]any{"threadId": thread, "model": request.Model, "approvalPolicy": "untrusted", "approvalsReviewer": "user", "sandbox": "read-only"}, &result); err != nil {
			return err
		}
		if result.Thread.ID != thread || result.Model != request.Model || result.ModelProvider != "openai" || result.ApprovalPolicy != "untrusted" || result.ApprovalsReviewer != "user" || result.Sandbox.Type != "readOnly" || result.Sandbox.NetworkAccess {
			return ErrProtocol
		}
		return nil
	})
}
func (c *Codex) DecideApproval(ctx context.Context, session string, approval Approval) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if session != c.session || approval.PolicyHash != c.request.PolicyHash || !c.approvals[approval.ID] || approval.Allow {
		return ErrDenied
	}
	return ctx.Err()
}
func (c *Codex) DelegateIsolatedCommand(ctx context.Context, session string, command sandbox.Command) (sandbox.CommandResult, error) {
	c.mu.Lock()
	if session != c.session || c.state != "running" {
		c.mu.Unlock()
		return sandbox.CommandResult{}, ErrDenied
	}
	request := c.request
	runctx := c.runctx
	c.activeEffects++
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.activeEffects--; c.mu.Unlock() }()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(runctx, cancel)
	defer stop()
	if err := validateCommand(command); err != nil {
		return sandbox.CommandResult{}, err
	}
	var result sandbox.CommandResult
	err := c.authorize(ctx, Effect{Binding: c.config.Binding, Request: request, Kind: "sandbox/exec", OperationID: domain.NewID()}, func() error {
		var err error
		result, err = c.config.Sandbox.ExecuteBoundedCommand(ctx, request.Workspace, command)
		return err
	})
	return result, err
}
func (c *Codex) Close() error {
	c.mu.Lock()
	c.closed = true
	r := c.rpc
	cancel := c.cancel
	c.mu.Unlock()
	c.finish("uncertain", ErrUncertain)
	if cancel != nil {
		cancel()
	}
	if r != nil {
		r.close()
	}
	return nil
}
func (c *Codex) ManagedLogin(ctx context.Context, action UserAction) (Login, error) {
	c.actions.Lock()
	defer c.actions.Unlock()
	c.mu.Lock()
	active := c.state == "running" || c.state == "starting"
	c.mu.Unlock()
	if active {
		return Login{}, ErrDenied
	}
	if action.Kind != "login" || !auth.ValidID(action.ID) || !auth.ValidID(action.UserID) || c.config.AuthorizeUserAction == nil {
		return Login{}, ErrDenied
	}
	if err := c.connect(ctx); err != nil {
		return Login{}, err
	}
	var response struct {
		Type    string `json:"type"`
		LoginID string `json:"loginId"`
		AuthURL string `json:"authUrl"`
	}
	calls := 0
	var effectErr error
	err := c.config.AuthorizeUserAction(ctx, c.config.Binding, action, func() error {
		calls++
		if calls != 1 {
			return ErrDenied
		}
		c.mu.Lock()
		c.authChanged = true
		c.mu.Unlock()
		effectErr = c.rpc.call(ctx, "account/login/start", map[string]string{"type": "chatgpt"}, &response)
		return effectErr
	})
	if err != nil || effectErr != nil || calls != 1 {
		if calls > 0 {
			return Login{}, ErrUncertain
		}
		return Login{}, ErrDenied
	}
	u, err := url.Parse(response.AuthURL)
	if err != nil || response.Type != "chatgpt" || response.LoginID == "" || u.Scheme != "https" || u.User != nil || (u.Host != "auth.openai.com" && u.Host != "chatgpt.com") {
		return Login{}, ErrProtocol
	}
	c.mu.Lock()
	c.loginID = response.LoginID
	if c.loginStatusID != response.LoginID {
		c.loginStatus = "pending"
	}
	c.mu.Unlock()
	return Login{ID: response.LoginID, URL: response.AuthURL}, nil
}
func (c *Codex) ManagedLogout(ctx context.Context, action UserAction) error {
	c.actions.Lock()
	defer c.actions.Unlock()
	if action.Kind != "logout" || !auth.ValidID(action.ID) || !auth.ValidID(action.UserID) || c.config.AuthorizeUserAction == nil {
		return ErrDenied
	}
	if err := c.connect(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	active := c.state == "running" || c.state == "starting"
	c.mu.Unlock()
	if active {
		return ErrDenied
	}
	calls := 0
	var effectErr error
	err := c.config.AuthorizeUserAction(ctx, c.config.Binding, action, func() error {
		calls++
		if calls != 1 {
			return ErrDenied
		}
		c.mu.Lock()
		c.authChanged = true
		c.mu.Unlock()
		effectErr = c.rpc.call(ctx, "account/logout", nil, nil)
		return effectErr
	})
	if err != nil || effectErr != nil || calls != 1 {
		if calls > 0 {
			return ErrUncertain
		}
		return ErrDenied
	}
	return nil
}
func validateCommand(command sandbox.Command) error {
	if len(command.Args) == 0 || len(command.Args) > 64 || command.Timeout <= 0 || command.Timeout > time.Minute || command.MaxOutputBytes < 1 || command.MaxOutputBytes > 256<<10 || command.NetworkProfile != "none" || filepath.IsAbs(command.Directory) || strings.Contains(command.Directory, "\\") {
		return ErrDenied
	}
	clean := filepath.Clean(command.Directory)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return ErrDenied
	}
	total := 0
	for _, arg := range command.Args {
		total += len(arg)
		if strings.ContainsRune(arg, 0) {
			return ErrDenied
		}
	}
	if total > 32<<10 {
		return ErrDenied
	}
	return nil
}

var _ AgentExecutor = (*Codex)(nil)

func (c *Codex) ManagedLoginStatus(ctx context.Context, loginID string) (string, error) {
	if err := c.qualify(ctx, true); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if loginID == "" || loginID != c.loginID {
		return "", ErrDenied
	}
	return c.loginStatus, nil
}
