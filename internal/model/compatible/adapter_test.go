package compatible

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"reforge/internal/domain"
	"reforge/internal/model"
)

type fixtureClient struct {
	responses []*http.Response
	requests  []*http.Request
	err       error
}

func (c *fixtureClient) Do(request *http.Request) (*http.Response, error) {
	c.requests = append(c.requests, request)
	if c.err != nil {
		return nil, c.err
	}
	if len(c.responses) == 0 {
		return nil, io.EOF
	}
	response := c.responses[0]
	c.responses = c.responses[1:]
	return response, nil
}

func fixtureResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func fixtureProvider(t *testing.T, client *fixtureClient) *Provider {
	t.Helper()
	provider, err := New(model.Config{Endpoint: "https://private.example/v1", APIKey: "fixture-key", Model: "local-model", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestChatStreamToolsUsageAndContinuation(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chat-1","choices":[{"index":0,"delta":{"role":"assistant","content":"checking"},"finish_reason":null}]}`,
		`data: {"id":"chat-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-a","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"a\"}"}},{"index":1,"id":"call-b","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"b\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chat-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"id":"chat-1","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19,"prompt_tokens_details":{"cached_tokens":3}}}`,
		`data: [DONE]`,
	}, "\n\n")
	client := &fixtureClient{responses: []*http.Response{fixtureResponse(http.StatusOK, stream)}}
	provider := fixtureProvider(t, client)
	tools := []model.Tool{{Name: "lookup", Schema: []byte(`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"],"additionalProperties":false}`)}}
	var events []model.Event
	err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "find"}}, Tools: tools, MaxOutputTokens: 64}, func(event model.Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[0].Type != "text_delta" || events[1].Type != "tool_call" || events[2].Type != "tool_call" || events[3].Type != "completed" {
		t.Fatalf("unexpected events: %#v", events)
	}
	if events[3].Usage == nil || !events[3].Usage.Known || events[3].Usage.InputTokens != 12 || events[3].Usage.OutputTokens != 7 || events[3].Usage.CacheTokens != 3 {
		t.Fatalf("unexpected usage: %#v", events[3].Usage)
	}
	if !strings.Contains(string(events[3].Continuation), `"tool_calls"`) {
		t.Fatalf("continuation lost tool calls: %s", events[3].Continuation)
	}
	client.responses = []*http.Response{fixtureResponse(http.StatusOK, strings.Join([]string{
		`data: {"id":"chat-2","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}`,
		`data: {"id":"chat-2","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":1,"total_tokens":21}}`,
		`data: [DONE]`,
	}, "\n\n"))}
	err = provider.StreamTurn(context.Background(), model.TurnRequest{Continuation: events[3].Continuation, Messages: []model.Message{{Role: "tool", ToolCallID: "call-a", Text: `{"value":1}`}}, MaxOutputTokens: 64}, func(model.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(client.requests[1].Body)
	if err != nil || !strings.Contains(string(body), `"content":"find"`) || !strings.Contains(string(body), `"tool_call_id":"call-a"`) || !strings.Contains(string(body), `"call-a"`) {
		t.Fatalf("continuation request missing history: %s %v", body, err)
	}
}

func TestMalformedIncompleteAndEmptyTools(t *testing.T) {
	client := &fixtureClient{responses: []*http.Response{fixtureResponse(http.StatusOK, `data: {"id":"chat-1","choices":[{"delta":{"content":"partial"},"finish_reason":"stop"}]}`+"\n")}}
	provider := fixtureProvider(t, client)
	err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, MaxOutputTokens: 32}, func(model.Event) error { return nil })
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Uncertain {
		t.Fatalf("unexpected incomplete stream error: %#v", err)
	}
	client.responses = []*http.Response{fixtureResponse(http.StatusOK, strings.Join([]string{
		`data: {"id":"chat-2","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-a","type":"function","function":{"name":"lookup","arguments":"not-json"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, "\n\n"))}
	err = provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, Tools: []model.Tool{{Name: "lookup", Schema: []byte(`{"type":"object"}`)}}, MaxOutputTokens: 32}, func(model.Event) error { return nil })
	if !errors.As(err, &providerErr) || providerErr.Kind != "protocol" || !providerErr.Uncertain {
		t.Fatalf("unexpected malformed tool error: %#v", err)
	}
	client.responses = []*http.Response{fixtureResponse(http.StatusOK, strings.Join([]string{
		`data: {"id":"chat-3","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}, "\n\n"))}
	if err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, MaxOutputTokens: 32}, func(model.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestCancellationAndUnavailableModelList(t *testing.T) {
	client := &fixtureClient{err: context.Canceled}
	provider := fixtureProvider(t, client)
	err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, MaxOutputTokens: 32}, func(model.Event) error { return nil })
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "canceled" || !providerErr.Uncertain {
		t.Fatalf("unexpected cancellation: %#v", err)
	}
	client = &fixtureClient{responses: []*http.Response{fixtureResponse(http.StatusNotFound, `{}`)}}
	provider = fixtureProvider(t, client)
	_, err = provider.ListModels(context.Background())
	if !errors.As(err, &providerErr) || providerErr.Kind != "capability_unknown" {
		t.Fatalf("unexpected model list error: %#v", err)
	}
}

func TestProbeRecordsModelCaps(t *testing.T) {
	client := &fixtureClient{responses: []*http.Response{fixtureResponse(http.StatusOK, `{"data":[{"id":"local-model","owned_by":"vllm","context_length":4096,"max_output_tokens":256}]}`)}}
	provider := fixtureProvider(t, client)
	var providerErr *domain.ProviderError
	caps, err := provider.Probe(context.Background())
	if err != nil || caps.Features["chat_completions"].State != domain.Unknown {
		t.Fatalf("unexpected probe: %#v %v", caps, err)
	}
	client.responses = []*http.Response{fixtureResponse(http.StatusOK, strings.Join([]string{
		`data: {"id":"chat-1","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}, "\n\n"))}
	err = provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, MaxOutputTokens: 257}, func(model.Event) error { return nil })
	if !errors.As(err, &providerErr) || providerErr.Kind != "invalid_request" {
		t.Fatalf("unexpected output cap result: %#v", err)
	}
}

func TestStreamRejectsUnboundedOrAmbiguousToolCalls(t *testing.T) {
	for _, value := range []string{
		`{"index":-1,"id":"a","function":{"name":"lookup","arguments":"{}"}}`,
		`{"index":1000000000,"id":"a","function":{"name":"lookup","arguments":"{}"}}`,
		`{"index":0,"id":"a","function":{"name":"lookup","arguments":"{}"}},{"index":1,"id":"a","function":{"name":"lookup","arguments":"{}"}}`,
	} {
		stream := `data: {"id":"x","choices":[{"index":0,"delta":{"tool_calls":[` + value + `]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
		client := &fixtureClient{responses: []*http.Response{fixtureResponse(200, stream)}}
		provider := fixtureProvider(t, client)
		tools := 0
		err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, Tools: []model.Tool{{Name: "lookup", Schema: []byte(`{"type":"object"}`)}}, MaxOutputTokens: 32}, func(e model.Event) error {
			if e.Type == "tool_call" {
				tools++
			}
			return nil
		})
		var typed *domain.ProviderError
		if !errors.As(err, &typed) || !typed.Uncertain || tools != 0 {
			t.Fatalf("ambiguous tool emitted: %d %v", tools, err)
		}
	}
	for _, finish := range []string{"length", "content_filter", "unknown"} {
		stream := `data: {"id":"x","choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"` + finish + `"}]}` + "\n\ndata: [DONE]\n\n"
		provider := fixtureProvider(t, &fixtureClient{responses: []*http.Response{fixtureResponse(200, stream)}})
		err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, Tools: []model.Tool{{Name: "lookup", Schema: []byte(`{"type":"object"}`)}}, MaxOutputTokens: 32}, func(e model.Event) error {
			if e.Type == "tool_call" || e.Type == "completed" {
				t.Error("nonterminal tool executed")
			}
			return nil
		})
		if err == nil {
			t.Fatal("incomplete finish accepted")
		}
	}
}

func TestPinnedLimitsAndOpaqueHistoryRemainExact(t *testing.T) {
	client := &fixtureClient{responses: []*http.Response{fixtureResponse(200, `{"data":[{"id":"local-model","context_length":4096,"max_output_tokens":256},{"id":"other","context_length":100,"max_output_tokens":1}]}`)}}
	provider := fixtureProvider(t, client)
	if _, err := provider.ListModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if provider.outputLimit != 256 || provider.contextLimit != 4096 {
		t.Fatal("unrelated model changed pinned limits")
	}
	body, history, err := provider.requestBody(model.TurnRequest{System: "fixed", Continuation: []byte(`[{"role":"assistant","content":"old","opaque":{"integer":9007199254740993}}]`), Messages: []model.Message{{Role: "user", Text: "new"}}, MaxOutputTokens: 64}, "local-model")
	if err != nil || !strings.Contains(string(body), "9007199254740993") || len(history) != 2 || !strings.Contains(string(history[1]), "new") {
		t.Fatal("native continuation lost precision or new input")
	}
}
