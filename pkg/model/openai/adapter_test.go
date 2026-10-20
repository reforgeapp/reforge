package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

func testProvider(t *testing.T, client model.HTTPClient) *Provider {
	t.Helper()
	provider, err := New(model.Config{Endpoint: "https://api.openai.test/v1", APIKey: "sk-fixture", Model: "gpt-fixture", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func sse(event string, payload string) string {
	return "event: " + event + "\ndata: " + payload + "\n\n"
}

func TestStreamTurnAssemblesParallelToolCalls(t *testing.T) {
	stream := strings.Join([]string{
		sse("response.output_text.delta", `{"type":"response.output_text.delta","item_id":"msg_1","delta":"checking"}`),
		sse("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"item_a","call_id":"call_a","name":"lookup","arguments":""}}`),
		sse("response.output_item.added", `{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"item_b","call_id":"call_b","name":"lookup","arguments":""}}`),
		sse("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","item_id":"item_b","output_index":1,"delta":"{\"query\":"}`),
		sse("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","item_id":"item_a","output_index":0,"delta":"{\"query\":"}`),
		sse("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","item_id":"item_b","output_index":1,"delta":"\"second\"}"}`),
		sse("response.function_call_arguments.done", `{"type":"response.function_call_arguments.done","item_id":"item_b","output_index":1,"arguments":"{\"query\":\"second\"}"}`),
		sse("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","item_id":"item_a","output_index":0,"delta":"\"first\"}"}`),
		sse("response.function_call_arguments.done", `{"type":"response.function_call_arguments.done","item_id":"item_a","output_index":0,"arguments":"{\"query\":\"first\"}"}`),
		sse("response.completed", `{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"reasoning","encrypted_content":"opaque"}],"usage":{"input_tokens":11,"output_tokens":7,"input_tokens_details":{"cached_tokens":2}}}}`),
		"data: [DONE]\n\n",
	}, "")
	provider := testProvider(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		return response(http.StatusOK, stream), nil
	}))
	var events []model.Event
	err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "find"}}, Tools: []model.Tool{{Name: "lookup", Schema: []byte(`{"type":"object","required":["query"],"properties":{"query":{"type":"string"}},"additionalProperties":false}`)}}, MaxOutputTokens: 20}, func(event model.Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[0].Type != "text_delta" || events[1].Type != "tool_call" || events[2].Type != "tool_call" || events[3].Type != "completed" {
		t.Fatalf("unexpected event sequence: %+v", events)
	}
	if string(events[1].ToolCall.Arguments) != `{"query":"second"}` || string(events[2].ToolCall.Arguments) != `{"query":"first"}` {
		t.Fatalf("unexpected tool arguments: %+v %+v", events[1].ToolCall, events[2].ToolCall)
	}
	if events[3].Usage == nil || !events[3].Usage.Known || events[3].Usage.CacheTokens != 2 {
		t.Fatalf("missing authoritative usage: %+v", events[3].Usage)
	}
	if !strings.Contains(string(events[3].Continuation), "opaque") {
		t.Fatalf("opaque continuation was not retained: %s", events[3].Continuation)
	}
}

func TestStreamTurnRejectsMalformedOrUnknownToolWithoutPartialEvent(t *testing.T) {
	cases := []struct {
		name   string
		stream string
	}{
		{name: "malformed", stream: sse("response.output_item.added", `{"type":"response.output_item.added","item":{"type":"function_call","id":"item","call_id":"call","name":"lookup"}}`) + sse("response.function_call_arguments.done", `{"type":"response.function_call_arguments.done","item_id":"item","arguments":"{bad"}`)},
		{name: "unknown", stream: sse("response.output_item.added", `{"type":"response.output_item.added","item":{"type":"function_call","id":"item","call_id":"call","name":"other"}}`) + sse("response.function_call_arguments.done", `{"type":"response.function_call_arguments.done","item_id":"item","arguments":"{}"}`)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			provider := testProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return response(http.StatusOK, test.stream), nil }))
			var toolEvents int
			err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 20, Tools: []model.Tool{{Name: "lookup", Schema: []byte(`{"type":"object"}`)}}}, func(event model.Event) error {
				if event.Type == "tool_call" {
					toolEvents++
				}
				return nil
			})
			if err == nil || toolEvents != 0 {
				t.Fatalf("expected rejection without tool event, err=%v events=%d", err, toolEvents)
			}
		})
	}
}

func TestStreamTurnInterruptionHasNoUsage(t *testing.T) {
	stream := sse("response.output_text.delta", `{"type":"response.output_text.delta","delta":"partial"}`) + sse("response.incomplete", `{"type":"response.incomplete","response":{"id":"resp_1","usage":{"input_tokens":4,"output_tokens":1}}}`) + sse("response.completed", `{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":4,"output_tokens":1}}}`)
	provider := testProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return response(http.StatusOK, stream), nil }))
	var events []model.Event
	err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 20}, func(event model.Event) error { events = append(events, event); return nil })
	if err == nil {
		t.Fatal("expected interruption error")
	}
	for _, event := range events {
		if event.Usage != nil || event.Type == "completed" {
			t.Fatalf("interrupted stream emitted final usage: %+v", event)
		}
	}
}

func TestStreamTurnCallbackCancellationIsTyped(t *testing.T) {
	stream := sse("response.output_text.delta", `{"type":"response.output_text.delta","delta":"stop"}`)
	provider := testProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return response(http.StatusOK, stream), nil }))
	err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 20}, func(model.Event) error { return context.Canceled })
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "canceled" {
		t.Fatalf("expected typed cancellation: %v", err)
	}
}

func TestSafeErrorsAndReadOnlyProbe(t *testing.T) {
	provider := testProvider(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/models/gpt-fixture" {
			t.Fatalf("probe must be read-only: %s %s", request.Method, request.URL.Path)
		}
		return response(http.StatusUnauthorized, `{"error":{"message":"sk-fixture leaked"}}`), nil
	}))
	_, err := provider.Probe(context.Background())
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "auth" || strings.Contains(err.Error(), "sk-fixture") {
		t.Fatalf("unsafe auth error: %v", err)
	}
}

func TestQuotaErrorIsTypedAndDoesNotExposeBody(t *testing.T) {
	provider := testProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		result := response(http.StatusTooManyRequests, `{"error":{"message":"secret quota details"}}`)
		result.Header.Set("Retry-After", "3")
		return result, nil
	}))
	err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 20}, func(model.Event) error { return nil })
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "quota" || providerErr.RetryAfter != 3*time.Second || strings.Contains(err.Error(), "secret quota") {
		t.Fatalf("unexpected quota error: %v", err)
	}
}

func TestNewRejectsModelFallbackAndEstimateIsUnknown(t *testing.T) {
	provider := testProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return response(http.StatusOK, ""), nil }))
	err := provider.StreamTurn(context.Background(), model.TurnRequest{Model: "other"}, func(model.Event) error { return nil })
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "model_unsupported" {
		t.Fatalf("expected pinned model rejection: %v", err)
	}
	usage := provider.EstimateUsage(model.TurnRequest{System: "system", Messages: []model.Message{{Role: "user", Text: "hello"}}, MaxOutputTokens: 8})
	if usage.Known || usage.InputTokens <= 0 || usage.OutputTokens != 8 {
		t.Fatalf("unexpected estimate: %+v", usage)
	}
}

func TestStatelessContinuationReplaysHistoryBeforeNewToolResult(t *testing.T) {
	turn := 0
	provider := testProvider(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		turn++
		var body struct {
			Store   bool                         `json:"store"`
			Include []string                     `json:"include"`
			Input   []map[string]json.RawMessage `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Store || len(body.Include) != 1 {
			t.Fatal("stateless reasoning preservation missing")
		}
		if turn == 2 {
			if len(body.Input) != 4 || string(body.Input[0]["content"]) != `"inspect"` || string(body.Input[1]["encrypted_content"]) != `"opaque-token"` || string(body.Input[2]["type"]) != `"function_call"` || string(body.Input[3]["type"]) != `"function_call_output"` {
				t.Fatalf("invalid replay order: %+v", body.Input)
			}
			return response(200, sse("response.completed", `{"response":{"id":"resp_2","output":[],"usage":{"input_tokens":12,"output_tokens":2}}}`)), nil
		}
		return response(200, sse("response.completed", `{"response":{"id":"resp_1","output":[{"type":"reasoning","id":"rs_1","encrypted_content":"opaque-token"},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read","arguments":"{}"}],"usage":{"input_tokens":10,"output_tokens":2}}}`)), nil
	}))
	var continuation json.RawMessage
	if err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 20, Messages: []model.Message{{Role: "user", Text: "inspect"}}}, func(event model.Event) error { continuation = event.Continuation; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 20, Continuation: continuation, Messages: []model.Message{{Role: "tool", ToolCallID: "call_1", Text: "file content"}}}, func(model.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestIncompleteToolAndCancellationRetainUncertainUsage(t *testing.T) {
	stream := sse("response.output_item.added", `{"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read"}}`) + sse("response.completed", `{"response":{"output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`)
	provider := testProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return response(200, stream), nil }))
	err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 10}, func(model.Event) error { return nil })
	var pe *domain.ProviderError
	if !errors.As(err, &pe) || !pe.Uncertain {
		t.Fatalf("partial tool did not retain usage uncertainty: %v", err)
	}
	provider = testProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.Canceled }))
	err = provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 10}, func(model.Event) error { return nil })
	if !errors.As(err, &pe) || !pe.Uncertain {
		t.Fatalf("dispatch cancellation lost usage uncertainty: %v", err)
	}
}
