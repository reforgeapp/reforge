package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
)

func TestNativeScopeSeparatesOrganizationUserAndConnection(t *testing.T) {
	scope := NativeScope{OrgID: domain.NewID(), UserID: domain.NewID(), ConnectionID: domain.NewID(), RunnerID: domain.NewID(), Provider: "codex"}
	if !scope.Valid() {
		t.Fatal("fixture scope is invalid")
	}
	key := scope.Key()
	for name, update := range map[string]func(*NativeScope){
		"organization": func(s *NativeScope) { s.OrgID = domain.NewID() },
		"user":         func(s *NativeScope) { s.UserID = domain.NewID() },
		"connection":   func(s *NativeScope) { s.ConnectionID = domain.NewID() },
	} {
		t.Run(name, func(t *testing.T) {
			other := scope
			update(&other)
			if other.Key() == key {
				t.Fatalf("%s change reused credential scope key", name)
			}
		})
	}
}

func TestNativeReplyRejectsInvalidToolCalls(t *testing.T) {
	turn := model.Turn{Tools: []model.Tool{{Name: "lookup", Schema: json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"],"additionalProperties":false}`)}}}
	cases := map[string]string{
		"unknown tool":        `{"text":"","tool_calls":[{"id":"one","name":"shell","arguments":"{}"}]}`,
		"duplicate ids":       `{"text":"","tool_calls":[{"id":"same","name":"lookup","arguments":"{\"key\":\"a\"}"},{"id":"same","name":"lookup","arguments":"{\"key\":\"b\"}"}]}`,
		"malformed arguments": `{"text":"","tool_calls":[{"id":"one","name":"lookup","arguments":"{broken"}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := nativeReply(turn, []byte(raw), model.Usage{}); err == nil {
				t.Fatal("invalid tool call was accepted")
			}
		})
	}
}

func TestNativeManagerBusyLimitsConcurrentScopes(t *testing.T) {
	manager := NewNativeManager(context.Background(), func(context.Context, NativeScope) (NativeHost, error) {
		return nil, errors.New("unused")
	})
	scopes := []NativeScope{
		{OrgID: domain.NewID(), UserID: domain.NewID(), ConnectionID: domain.NewID(), RunnerID: domain.NewID(), Provider: "codex"},
		{OrgID: domain.NewID(), UserID: domain.NewID(), ConnectionID: domain.NewID(), RunnerID: domain.NewID(), Provider: "codex"},
		{OrgID: domain.NewID(), UserID: domain.NewID(), ConnectionID: domain.NewID(), RunnerID: domain.NewID(), Provider: "codex"},
	}
	releaseFirst, err := manager.acquire(scopes[0])
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()
	if _, err := manager.acquire(scopes[0]); err == nil {
		t.Fatal("same scope acquired twice")
	} else {
		var providerErr *domain.ProviderError
		if !errors.As(err, &providerErr) || providerErr.Kind != "quota" || providerErr.RetryAfter <= 0 {
			t.Fatalf("same-scope busy error = %v", err)
		}
	}
	releaseSecond, err := manager.acquire(scopes[1])
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	if _, err := manager.acquire(scopes[2]); err == nil {
		t.Fatal("runtime slot limit was not enforced")
	} else {
		var providerErr *domain.ProviderError
		if !errors.As(err, &providerErr) || providerErr.Kind != "quota" || providerErr.RetryAfter <= 0 {
			t.Fatalf("runtime slot error = %v", err)
		}
	}
}

type nativeLoginFixtureHost struct {
	complete chan struct{}
	started  chan struct{}
	closed   atomic.Int32
	start    sync.Once
}

func (h *nativeLoginFixtureHost) Exec(ctx context.Context, _ []string, stdin io.Reader, stdout, _ io.Writer) (int, error) {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 4096), maxPacket)
	write := func(value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, string(raw))
		return err
	}
	for scanner.Scan() {
		var request packet
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return 1, err
		}
		switch request.Method {
		case "initialize":
			if err := write(map[string]any{"id": request.ID, "result": map[string]string{"userAgent": "codex_cli_rs/" + NativeCodexVersion + " (fixture)"}}); err != nil {
				return 1, err
			}
		case "initialized":
		case "account/login/start":
			if err := write(map[string]any{"id": request.ID, "result": map[string]string{"type": "chatgptDeviceCode", "loginId": "fixture-login", "verificationUrl": "https://auth.openai.com/device", "userCode": "ABCD-EFGH"}}); err != nil {
				return 1, err
			}
			h.start.Do(func() { close(h.started) })
			select {
			case <-ctx.Done():
				return 1, ctx.Err()
			case <-h.complete:
			}
			if err := write(map[string]any{"method": "account/login/completed", "params": map[string]any{"loginId": "fixture-login", "success": true}}); err != nil {
				return 1, err
			}
		case "account/read":
			if err := write(map[string]any{"id": request.ID, "result": map[string]any{"account": map[string]string{"type": "chatgpt", "email": "fixture@example.com", "planType": "plus"}, "workspaceRouting": map[string]string{"chatgptAccountId": "fixture-account"}}}); err != nil {
				return 1, err
			}
		default:
			return 1, fmt.Errorf("unexpected fixture RPC method %q", request.Method)
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return 1, err
	}
	return 0, nil
}

func (h *nativeLoginFixtureHost) Close() error {
	h.closed.Add(1)
	return nil
}

func TestNativeCodexLoginOutlivesRequestAndClearsChallenge(t *testing.T) {
	managerCtx, cancelManager := context.WithCancel(context.Background())
	defer cancelManager()
	host := &nativeLoginFixtureHost{complete: make(chan struct{}), started: make(chan struct{})}
	scope := NativeScope{OrgID: domain.NewID(), UserID: domain.NewID(), ConnectionID: domain.NewID(), RunnerID: domain.NewID(), Provider: "codex", AccountID: "fixture-account/fixture@example.com"}
	manager := NewNativeManager(managerCtx, func(context.Context, NativeScope) (NativeHost, error) { return host, nil })
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	state, err := manager.Action(requestCtx, scope, NativeAction{Kind: "login"})
	cancelRequest()
	if err != nil || state.State != "starting" {
		t.Fatalf("login start = %#v, %v", state, err)
	}
	select {
	case <-host.started:
	case <-time.After(3 * time.Second):
		t.Fatal("native login did not reach device-code request")
	}
	pending := waitNativeLoginState(t, manager, scope, "pending")
	if pending.Login == nil || pending.Login.URL != "https://auth.openai.com/device" || pending.Login.Code != "ABCD-EFGH" {
		t.Fatalf("pending login challenge = %#v", pending.Login)
	}
	if host.closed.Load() != 0 {
		t.Fatal("native process closed when the login request context was canceled")
	}
	close(host.complete)
	connected := waitNativeLoginState(t, manager, scope, "connected")
	if connected.Account == nil || connected.Account.ID != scope.AccountID || connected.Login == nil || connected.Login.URL != "" || connected.Login.Code != "" {
		t.Fatalf("completed login retained challenge or missed account: %#v", connected)
	}
	deadline := time.After(3 * time.Second)
	for host.closed.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("native process did not close after successful login")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitNativeLoginState(t *testing.T, manager *NativeManager, scope NativeScope, expected string) NativeState {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		state, err := manager.Action(context.Background(), scope, NativeAction{Kind: "status"})
		if err != nil {
			t.Fatal(err)
		}
		if state.State == expected {
			return state
		}
		select {
		case <-deadline:
			t.Fatalf("login state stayed %q; expected %q", state.State, expected)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
