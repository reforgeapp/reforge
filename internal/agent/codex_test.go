package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"reforge/internal/domain"
	"reforge/internal/sandbox"
)

func TestAgentProcessFixture(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == "agent-fixture" {
			marker = i
			break
		}
	}
	if marker < 0 {
		return
	}
	mode := os.Args[marker+1]
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), maxPacket)
	write := func(value any) { raw, _ := json.Marshal(value); _, _ = fmt.Fprintln(os.Stdout, string(raw)) }
	notify := func(method string, params any) { write(map[string]any{"method": method, "params": params}) }
	complete := func(status string) {
		turn := map[string]any{"id": "turn-1", "status": status, "items": []any{}}
		if mode == "quota" {
			turn["error"] = map[string]any{"message": "quota detail not exposed", "codexErrorInfo": "usageLimitExceeded"}
		}
		notify("turn/completed", map[string]any{"threadId": "thread-1", "turn": turn})
	}
	turns := 0
	for scanner.Scan() {
		var p packet
		if json.Unmarshal(scanner.Bytes(), &p) != nil {
			os.Exit(2)
		}
		var params map[string]json.RawMessage
		_ = json.Unmarshal(p.Params, &params)
		reply := func(value any) { write(map[string]any{"id": p.ID, "result": value}) }
		switch p.Method {
		case "initialize":
			if mode == "oversize" {
				reply(map[string]string{"userAgent": strings.Repeat("x", maxPacket+1)})
				continue
			}
			reply(map[string]string{"userAgent": "codex_cli_rs/0.150.1 (fixture; isolated test process)"})
		case "initialized":
		case "account/read":
			kind := "chatgpt"
			if mode == "wrongbilling" {
				kind = "apiKey"
			}
			reply(map[string]any{"account": map[string]string{"type": kind, "planType": "test"}, "requiresOpenaiAuth": true})
		case "thread/start", "thread/resume":
			if string(params["approvalPolicy"]) != `"untrusted"` || string(params["sandbox"]) != `"read-only"` {
				os.Exit(3)
			}
			if p.Method == "thread/start" {
				var tools []any
				if json.Unmarshal(params["dynamicTools"], &tools) != nil || len(tools) != 3 {
					os.Exit(4)
				}
			} else if turns != 1 {
				os.Exit(5)
			}
			selectedModel := "fixture-model"
			if mode == "wrongmodel" {
				selectedModel = "other-model"
			}
			reply(map[string]any{"thread": map[string]string{"id": "thread-1"}, "model": selectedModel, "modelProvider": "openai", "approvalPolicy": "untrusted", "approvalsReviewer": "user", "sandbox": map[string]string{"type": "readOnly"}})
		case "turn/start":
			turns++
			if turns != 1 {
				os.Exit(6)
			}
			reply(map[string]any{"turn": map[string]any{"id": "turn-1", "status": "inProgress", "items": []any{}}})
			notify("turn/started", map[string]any{"threadId": "thread-1", "turn": map[string]string{"id": "turn-1", "status": "inProgress"}})
			switch mode {
			case "native":
				write(map[string]any{"id": 100, "method": "item/commandExecution/requestApproval", "params": map[string]any{"threadId": "thread-1", "turnId": "turn-1", "itemId": "native-1", "startedAtMs": 1, "command": "cat /auth/secret"}})
			case "posteffect":
				notify("item/completed", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "item": map[string]string{"id": "bad-native", "type": "commandExecution", "status": "completed"}})
			case "falsecallback":
				notify("item/tool/call", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "callId": "call-1", "tool": "reforge_exec", "arguments": map[string]any{}})
			case "quota":
				complete("failed")
			case "cancel":
			case "eof":
				os.Exit(0)
			default:
				tool := "reforge_exec"
				args := any(map[string]any{"args": []string{"/bin/echo", "fixture"}, "timeoutMillis": 1000, "maxOutputBytes": 1024})
				if mode == "malicious" {
					tool = "reforge_read"
					args = map[string]string{"path": "../auth.json"}
				}
				if mode == "unknown_tool" {
					tool = "shellCommand"
				}
				write(map[string]any{"id": 100, "method": "item/tool/call", "params": map[string]any{"threadId": "thread-1", "turnId": "turn-1", "callId": "call-1", "tool": tool, "arguments": args}})
				if mode == "premature" {
					complete("completed")
				}
			}
		case "turn/interrupt":
			reply(map[string]any{})
			complete("interrupted")
		case "account/login/start":
			if string(params["type"]) != `"chatgpt"` {
				os.Exit(7)
			}
			reply(map[string]string{"type": "chatgpt", "loginId": "login-1", "authUrl": "https://auth.openai.com/authorize?fixture=1"})
			notify("account/login/completed", map[string]any{"loginId": "login-1", "success": true})
		case "account/logout":
			reply(map[string]any{})
		case "":
			if mode == "native" {
				var result struct {
					Decision string `json:"decision"`
				}
				_ = json.Unmarshal(p.Result, &result)
				if result.Decision != "decline" {
					os.Exit(8)
				}
			} else {
				var result struct {
					Success bool `json:"success"`
				}
				_ = json.Unmarshal(p.Result, &result)
				if !result.Success {
					os.Exit(9)
				}
			}
			if mode == "duplicate" {
				write(map[string]any{"id": 101, "method": "item/tool/call", "params": map[string]any{"threadId": "thread-1", "turnId": "turn-1", "callId": "call-1", "tool": "reforge_exec", "arguments": map[string]any{"args": []string{"/bin/echo", "fixture"}, "timeoutMillis": 1000, "maxOutputBytes": 1024}}})
				continue
			}
			notify("item/agentMessage/delta", map[string]string{"threadId": "thread-1", "turnId": "turn-1", "itemId": "message-1", "delta": "done"})
			complete("completed")
		default:
			os.Exit(10)
		}
	}
	os.Exit(0)
}

type fixtureSandbox struct {
	calls     atomic.Int64
	before    func()
	workspace sandbox.Workspace
}

func (s *fixtureSandbox) PreparePinnedWorkspace(context.Context, sandbox.WorkspaceRequest) (sandbox.Workspace, error) {
	return s.workspace, nil
}
func (s *fixtureSandbox) ExecuteBoundedCommand(ctx context.Context, w sandbox.Workspace, c sandbox.Command) (sandbox.CommandResult, error) {
	if s.before != nil {
		s.before()
	}
	if w.ID != s.workspace.ID || c.NetworkProfile != "none" {
		return sandbox.CommandResult{}, ErrDenied
	}
	s.calls.Add(1)
	return sandbox.CommandResult{Output: []byte("fixture output")}, nil
}
func (s *fixtureSandbox) ApplyPatch(context.Context, sandbox.Workspace, []sandbox.Patch) error {
	s.calls.Add(1)
	return nil
}
func (s *fixtureSandbox) CollectArtifact(context.Context, sandbox.Workspace, string) (sandbox.Artifact, error) {
	s.calls.Add(1)
	return sandbox.Artifact{Data: []byte("fixture")}, nil
}
func (s *fixtureSandbox) Destroy(context.Context, sandbox.Workspace) error { return nil }
func fixtureBridge(t *testing.T, mode string) (*Codex, *fixtureSandbox, *[]string, *sync.Mutex) {
	t.Helper()
	scope := Binding{OrgID: domain.NewID(), ConnectionID: domain.NewID(), RunnerID: domain.NewID(), ConnectionVersion: 1, CredentialVersion: 1, AccountID: "fixture-account", Model: "fixture-model", RuntimeDigest: "fixture-sha256", Deployment: "development-fixture"}
	workspace := sandbox.Workspace{ID: "workspace-1", CommitSHA: strings.Repeat("a", 40), Image: "sha256:fixture"}
	box := &fixtureSandbox{workspace: workspace}
	claims := []string{}
	mu := &sync.Mutex{}
	config := CodexConfig{Binding: scope, Timeout: 5 * time.Second, Sandbox: box, Open: func(ctx context.Context, binding Binding) (Runtime, error) {
		command := exec.Command(os.Args[0], "-test.run=^TestAgentProcessFixture$", "--", "agent-fixture", mode)
		command.Env = []string{"PATH=/usr/bin:/bin"}
		command.Dir = t.TempDir()
		process, err := StartPreparedRuntime(command)
		return Runtime{Transport: process, Binding: binding}, err
	}, Qualify: func(context.Context, Binding) (Qualification, error) {
		return Qualification{Binding: scope, EvidenceID: domain.NewID(), CheckedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), AuthCustody: true, NativeToolContainment: true, Terms: true, Topology: true, Entitlement: true, Quota: true, NoPaidOverage: true}, nil
	}, Authorize: func(ctx context.Context, effect Effect, perform func() error) error {
		if effect.Binding != scope || effect.Request.Workspace.ID != workspace.ID || effect.Request.PolicyHash != "policy-1" || !authID(effect.OperationID) {
			return ErrDenied
		}
		mu.Lock()
		claims = append(claims, effect.Kind)
		mu.Unlock()
		return perform()
	}, AuthorizeUserAction: func(ctx context.Context, b Binding, action UserAction, perform func() error) error { return perform() }}
	bridge, err := NewCodex(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bridge.Close() })
	box.before = func() {
		mu.Lock()
		defer mu.Unlock()
		if len(claims) < 2 || claims[len(claims)-1] != "reforge_exec" {
			t.Error("effect preceded durable authorization")
		}
	}
	return bridge, box, &claims, mu
}
func authID(id string) bool { return len(id) == 36 }
func fixtureRequest(box *fixtureSandbox) Request {
	return Request{JobID: domain.NewID(), AttemptID: domain.NewID(), Prompt: "inspect fixture", Workspace: box.workspace, Model: "fixture-model", PolicyHash: "policy-1", MaxTurns: 1}
}
func TestBrokerPreEffectClaimAndResume(t *testing.T) {
	c, box, claims, mu := fixtureBridge(t, "broker")
	session, err := c.Start(context.Background(), fixtureRequest(box))
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = c.StreamEvents(context.Background(), session, func(event Event) error { events = append(events, event); return nil }); err != nil {
		t.Fatal(err)
	}
	if box.calls.Load() != 1 || events[len(events)-1].Type != "completed" {
		t.Fatalf("incorrect execution/events: %#v", events)
	}
	if err = c.ResumeIfSupported(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*claims) != 3 || (*claims)[0] != "turn/start" || (*claims)[1] != "reforge_exec" || (*claims)[2] != "thread/resume" {
		t.Fatalf("unexpected effect claims: %#v", *claims)
	}
}
func TestNativeApprovalDeniedBeforeEffect(t *testing.T) {
	c, box, _, _ := fixtureBridge(t, "native")
	request := fixtureRequest(box)
	session, err := c.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var approval string
	err = c.StreamEvents(context.Background(), session, func(event Event) error {
		if event.Type == "approval_denied" {
			approval = event.ApprovalID
		}
		return nil
	})
	if err != nil || box.calls.Load() != 0 || approval == "" {
		t.Fatalf("native approval boundary failed: %v", err)
	}
	if err = c.DecideApproval(context.Background(), session, Approval{ID: approval, Allow: true, PolicyHash: request.PolicyHash}); !errors.Is(err, ErrDenied) {
		t.Fatal("native execution allowed")
	}
}
func TestPostEffectAndMaliciousToolsFailClosed(t *testing.T) {
	for _, mode := range []string{"posteffect", "falsecallback", "malicious", "unknown_tool", "eof"} {
		t.Run(mode, func(t *testing.T) {
			c, box, _, _ := fixtureBridge(t, mode)
			session, err := c.Start(context.Background(), fixtureRequest(box))
			if err == nil {
				err = c.StreamEvents(context.Background(), session, func(Event) error { return nil })
			}
			if err == nil || box.calls.Load() != 0 {
				t.Fatal("unsafe event caused success or broker effect")
			}
		})
	}
}
func TestCancelUsesNativeInterruptAndNoReplay(t *testing.T) {
	c, box, _, _ := fixtureBridge(t, "cancel")
	session, err := c.Start(context.Background(), fixtureRequest(box))
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Cancel(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	var terminal string
	if err = c.StreamEvents(context.Background(), session, func(event Event) error { terminal = event.Type; return nil }); err != nil || terminal != "canceled" {
		t.Fatalf("cancel not acknowledged: %s %v", terminal, err)
	}
}
func TestQuotaAndBillingRouteNeverFallback(t *testing.T) {
	c, box, _, _ := fixtureBridge(t, "quota")
	session, err := c.Start(context.Background(), fixtureRequest(box))
	if err != nil {
		t.Fatal(err)
	}
	var terminal Event
	err = c.StreamEvents(context.Background(), session, func(event Event) error { terminal = event; return nil })
	if err == nil || terminal.Type != "quota" || !strings.Contains(string(terminal.Data), CodexBillingRoute) || box.calls.Load() != 0 {
		t.Fatalf("quota handling failed: %v %#v", err, terminal)
	}
	c2, box2, _, _ := fixtureBridge(t, "wrongbilling")
	if _, err = c2.Start(context.Background(), fixtureRequest(box2)); !errors.Is(err, ErrDenied) {
		t.Fatal("API billing silently accepted")
	}
}
func TestQualificationAndExplicitManagedAuth(t *testing.T) {
	c, _, _, _ := fixtureBridge(t, "login")
	if _, err := c.ManagedLogin(context.Background(), UserAction{}); !errors.Is(err, ErrDenied) {
		t.Fatal("implicit login accepted")
	}
	login, err := c.ManagedLogin(context.Background(), UserAction{ID: domain.NewID(), UserID: domain.NewID(), Kind: "login"})
	if err != nil || login.ID != "login-1" {
		t.Fatal(err)
	}
	if err = c.ManagedLogout(context.Background(), UserAction{ID: domain.NewID(), UserID: domain.NewID(), Kind: "logout"}); err != nil {
		t.Fatal(err)
	}
	c2, box, _, _ := fixtureBridge(t, "broker")
	original := c2.config.Qualify
	c2.config.Qualify = func(ctx context.Context, b Binding) (Qualification, error) {
		q, e := original(ctx, b)
		q.Binding.AccountID = "other-account"
		return q, e
	}
	if _, err = c2.Start(context.Background(), fixtureRequest(box)); !errors.Is(err, ErrDisabled) {
		t.Fatal("other account qualification accepted")
	}
	if c2.rpc != nil {
		t.Fatal("unqualified runtime launched")
	}
}

func TestPrematureCompletionCannotAuthorizeLaterEffect(t *testing.T) {
	c, box, _, _ := fixtureBridge(t, "premature")
	original := c.config.Authorize
	c.config.Authorize = func(ctx context.Context, effect Effect, perform func() error) error {
		if effect.Kind == "reforge_exec" {
			<-ctx.Done()
			return ctx.Err()
		}
		return original(ctx, effect, perform)
	}
	session, err := c.Start(context.Background(), fixtureRequest(box))
	if err == nil {
		err = c.StreamEvents(context.Background(), session, func(Event) error { return nil })
	}
	if err == nil || box.calls.Load() != 0 {
		t.Fatal("premature completion released pending effect")
	}
}
func TestInstalledCodexVersionAndSchema(t *testing.T) {
	binary := os.Getenv("REFORGE_TEST_CODEX")
	if binary == "" {
		t.Skip("installed runtime schema check not enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--version")
	command.Env = []string{"PATH=/usr/bin:/bin"}
	output, err := command.Output()
	if err != nil || strings.TrimSpace(string(output)) != "codex-cli "+CodexVersion {
		t.Fatal("installed runtime version differs")
	}
	directory := t.TempDir()
	command = exec.CommandContext(ctx, binary, "app-server", "generate-json-schema", "--experimental", "--out", directory)
	command.Env = []string{"PATH=/usr/bin:/bin"}
	if err = command.Run(); err != nil {
		t.Fatal("installed runtime schema generation failed")
	}
	raw, err := os.ReadFile(directory + "/v2/ThreadStartParams.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(raw, &schema) != nil || len(schema.Properties["dynamicTools"]) == 0 || len(schema.Properties["approvalPolicy"]) == 0 {
		t.Fatal("required pinned protocol fields absent")
	}
	raw, err = os.ReadFile(directory + "/DynamicToolCallParams.json")
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &schema) != nil || len(schema.Properties["callId"]) == 0 || len(schema.Properties["arguments"]) == 0 {
		t.Fatal("dynamic tool protocol differs")
	}
}

func TestDuplicateToolAndOversizedPacketFailClosed(t *testing.T) {
	c, box, _, _ := fixtureBridge(t, "duplicate")
	session, err := c.Start(context.Background(), fixtureRequest(box))
	if err != nil {
		t.Fatal(err)
	}
	err = c.StreamEvents(context.Background(), session, func(Event) error { return nil })
	if err == nil || box.calls.Load() != 1 {
		t.Fatal("duplicate native call repeated an effect")
	}
	c2, box2, _, _ := fixtureBridge(t, "oversize")
	if _, err = c2.Start(context.Background(), fixtureRequest(box2)); err == nil || box2.calls.Load() != 0 {
		t.Fatal("oversized protocol packet accepted")
	}
}

func TestManagedAuthCompletionAndFailedCommitInvalidateRoute(t *testing.T) {
	c, _, _, _ := fixtureBridge(t, "login")
	login, err := c.ManagedLogin(context.Background(), UserAction{ID: domain.NewID(), UserID: domain.NewID(), Kind: "login"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		status, err := c.ManagedLoginStatus(context.Background(), login.ID)
		if err != nil {
			t.Fatal(err)
		}
		if status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native login completion lost")
		}
		time.Sleep(time.Millisecond)
	}
	c2, box, _, _ := fixtureBridge(t, "login")
	c2.config.AuthorizeUserAction = func(ctx context.Context, b Binding, action UserAction, perform func() error) error {
		if err := perform(); err != nil {
			return err
		}
		return errors.New("commit failure")
	}
	if _, err = c2.ManagedLogin(context.Background(), UserAction{ID: domain.NewID(), UserID: domain.NewID(), Kind: "login"}); !errors.Is(err, ErrUncertain) {
		t.Fatal("post-effect auth failure not uncertain")
	}
	if _, err = c2.Start(context.Background(), fixtureRequest(box)); !errors.Is(err, ErrDisabled) {
		t.Fatal("authentication change reused old qualification")
	}
}

func TestAuthorityCannotSwallowRuntimeEffectFailure(t *testing.T) {
	c, box, _, _ := fixtureBridge(t, "wrongmodel")
	c.config.Authorize = func(ctx context.Context, effect Effect, perform func() error) error { _ = perform(); return nil }
	if _, err := c.Start(context.Background(), fixtureRequest(box)); !errors.Is(err, ErrUncertain) {
		t.Fatal("authority swallowed native binding failure")
	}
	if box.calls.Load() != 0 {
		t.Fatal("wrong model obtained sandbox access")
	}
}

func TestCloseDuringOpenNeverInitializesLateRuntime(t *testing.T) {
	c, _, _, _ := fixtureBridge(t, "broker")
	original := c.config.Open
	entered := make(chan struct{})
	release := make(chan struct{})
	c.config.Open = func(ctx context.Context, b Binding) (Runtime, error) {
		close(entered)
		<-release
		return original(ctx, b)
	}
	done := make(chan error, 1)
	go func() { _, err := c.ProbeVersionAndAuth(context.Background()); done <- err }()
	<-entered
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrDisabled) {
		t.Fatalf("closed startup returned %v", err)
	}
	c.mu.Lock()
	connected := c.rpc != nil
	c.mu.Unlock()
	if connected {
		t.Fatal("closed bridge retained a runtime")
	}
	if _, err := c.ProbeVersionAndAuth(context.Background()); !errors.Is(err, ErrDisabled) {
		t.Fatal("closed bridge reopened")
	}
}
