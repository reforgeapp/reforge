package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"reforge/internal/domain"
	"reforge/internal/model"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

func testProvider(t *testing.T, client model.HTTPClient) *Provider {
	t.Helper()
	provider, err := New(model.Config{Endpoint: "https://api.anthropic.test/v1", APIKey: "sk-ant-api-fixture", Model: "claude-fixture", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func response(status int, body string) *http.Response {
	headers := make(http.Header)
	if strings.HasPrefix(body, "event:") {
		headers.Set("Content-Type", "text/event-stream")
	} else {
		headers.Set("Content-Type", "application/json")
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: headers}
}

func event(name, payload string) string {
	return "event: " + name + "\ndata: " + payload + "\n\n"
}

func messageStart() string {
	return event("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-fixture","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":0}}}`)
}

func messageStop() string { return event("message_stop", `{"type":"message_stop"}`) }

func TestStreamTurnAccumulatesIndexedParallelTools(t *testing.T) {
	stream := strings.Join([]string{
		messageStart(),
		event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ready"}}`),
		event("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_a","name":"lookup","input":{}}}`),
		event("content_block_start", `{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tool_b","name":"lookup","input":{}}}`),
		event("content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}`),
		event("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}`),
		event("content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"second\"}"}}`),
		event("content_block_stop", `{"type":"content_block_stop","index":2}`),
		event("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"first\"}"}}`),
		event("content_block_stop", `{"type":"content_block_stop","index":1}`),
		event("content_block_stop", `{"type":"content_block_stop","index":0}`),
		event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"input_tokens":5,"output_tokens":9,"cache_creation_input_tokens":2,"cache_read_input_tokens":3}}`),
		messageStop(),
	}, "")
	provider := testProvider(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		return response(http.StatusOK, stream), nil
	}))
	tools := []model.Tool{{Name: "lookup", Schema: []byte(`{"type":"object","required":["query"],"properties":{"query":{"type":"string"}},"additionalProperties":false}`)}}
	var events []model.Event
	err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 32, Messages: []model.Message{{Role: "user", Text: "find"}}, Tools: tools}, func(item model.Event) error {
		events = append(events, item)
		return nil
	})
	if err != nil {
		t.Fatalf("%v events=%+v", err, events)
	}
	if len(events) != 4 || events[0].Type != "text_delta" || events[1].Type != "tool_call" || events[2].Type != "tool_call" || events[3].Type != "completed" {
		t.Fatalf("unexpected events: %+v", events)
	}
	if string(events[1].ToolCall.Arguments) != `{"query":"second"}` || string(events[2].ToolCall.Arguments) != `{"query":"first"}` {
		t.Fatalf("unexpected tool calls: %+v %+v", events[1].ToolCall, events[2].ToolCall)
	}
	if events[3].Usage == nil || !events[3].Usage.Known || events[3].Usage.CacheTokens != 3 || events[3].Usage.CacheCreationTokens != 2 || events[3].Usage.InputTokens != 10 || events[3].Usage.OutputTokens != 9 {
		t.Fatalf("unexpected usage: %+v", events[3].Usage)
	}
}

func TestStreamTurnRejectsMalformedToolInputWithoutPartialEvent(t *testing.T) {
	stream := strings.Join([]string{
		messageStart(),
		event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_a","name":"lookup","input":{}}}`),
		event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{bad"}}`),
		event("content_block_stop", `{"type":"content_block_stop","index":0}`),
	}, "")
	provider := testProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return response(http.StatusOK, stream), nil }))
	var calls int
	err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 8, Messages: []model.Message{{Role: "user", Text: "inspect"}}, Tools: []model.Tool{{Name: "lookup", Schema: []byte(`{"type":"object"}`)}}}, func(item model.Event) error {
		if item.Type == "tool_call" {
			calls++
		}
		return nil
	})
	if err == nil || calls != 0 {
		t.Fatalf("expected malformed input failure without tool event, err=%v calls=%d", err, calls)
	}
}

func TestStreamTurnInterruptionHasUnknownUsageAndUncertainError(t *testing.T) {
	stream := messageStart() + event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`)
	provider := testProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return response(http.StatusOK, stream), nil }))
	var events []model.Event
	err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 8, Messages: []model.Message{{Role: "user", Text: "hello"}}}, func(item model.Event) error {
		events = append(events, item)
		return nil
	})
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Uncertain {
		t.Fatalf("expected uncertain interruption: %v", err)
	}
	for _, item := range events {
		if item.Usage != nil || item.Type == "completed" {
			t.Fatalf("interrupted stream emitted terminal event: %+v", item)
		}
	}
}

func TestContinuationIsPrependedAndThinkingOpaque(t *testing.T) {
	provider := testProvider(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if !strings.Contains(text, `"data":"opaque"`) || strings.Index(text, `"role":"assistant"`) > strings.Index(text, `"tool_result"`) {
			t.Fatalf("continuation was not preserved before new result: %s", text)
		}
		return response(http.StatusOK, messageStart()+messageStop()), nil
	}))
	continuation := []byte(`[{"role":"user","content":"inspect"},{"role":"assistant","content":[{"type":"thinking","thinking":"hidden","signature":"sig"},{"type":"redacted_thinking","data":"opaque","encrypted":"opaque"},{"type":"tool_use","id":"tool_a","name":"lookup","input":{"query":"x"}}]}]`)
	err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 8, Continuation: continuation, Messages: []model.Message{{Role: "user", ToolCallID: "tool_a", Text: "done"}}}, func(model.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestProbeListAndSafeRateLimit(t *testing.T) {
	provider := testProvider(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/models/claude-fixture":
			return response(http.StatusOK, `{"type":"model","id":"claude-fixture","display_name":"Fixture","created_at":"2025-01-01T00:00:00Z","max_input_tokens":1000,"max_tokens":200,"capabilities":{"batch":{"supported":false},"citations":{"supported":false},"code_execution":{"supported":false},"context_management":{"supported":false},"effort":{"supported":false},"image_input":{"supported":false},"pdf_input":{"supported":false},"structured_outputs":{"supported":false},"thinking":{"supported":false,"types":{"adaptive":{"supported":false},"enabled":{"supported":false}}}}}`), nil
		case request.Method == http.MethodGet && request.URL.Path == "/v1/models":
			return response(http.StatusTooManyRequests, `{"error":{"message":"fixture secret"}}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
			return nil, nil
		}
	}))
	capabilities, err := provider.Probe(context.Background())
	if err != nil || capabilities.Model != "claude-fixture" || capabilities.Features["tool_calling"].State != domain.Unknown {
		t.Fatalf("unexpected probe: %+v %v", capabilities, err)
	}
	_, err = provider.ListModels(context.Background())
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "rate_limit" || strings.Contains(err.Error(), "fixture secret") {
		t.Fatalf("unexpected safe rate-limit error: %v", err)
	}
}

func TestCancellationAndConservativeEstimate(t *testing.T) {
	provider := testProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.Canceled }))
	err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 8, Messages: []model.Message{{Role: "user", Text: "hello"}}}, func(model.Event) error { return nil })
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "canceled" {
		t.Fatalf("expected cancellation: %v", err)
	}
	usage := provider.EstimateUsage(model.TurnRequest{System: "system", MaxOutputTokens: 8, Messages: []model.Message{{Role: "user", Text: "hello"}}})
	if usage.Known || usage.InputTokens <= int64(len("system")) || usage.OutputTokens != 8 || usage.Source != "conservative_byte_upper_bound" {
		t.Fatalf("unexpected estimate: %+v", usage)
	}
}

type countedBody struct {
	io.Reader
	closed *int
}

func (b countedBody) Close() error { *b.closed++; return nil }

func TestSDKHistoryRoundTripPreservesSignatureAndClosesStream(t *testing.T) {
	turn, closed := 0, 0
	provider := testProvider(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		turn++
		var sent struct {
			Messages []json.RawMessage `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&sent); err != nil {
			t.Fatal(err)
		}
		stream := messageStart() + event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`) + messageStop()
		if turn == 1 {
			stream = messageStart() + event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`) + event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"private reasoning"}}`) + event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"native-signature"}}`) + event("content_block_stop", `{"type":"content_block_stop","index":0}`) + event("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_1","name":"read","input":{}}}`) + event("content_block_stop", `{"type":"content_block_stop","index":1}`) + event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`) + messageStop()
		} else if len(sent.Messages) != 3 || !strings.Contains(string(sent.Messages[0]), "inspect") || !strings.Contains(string(sent.Messages[1]), "native-signature") || strings.Count(string(sent.Messages[2]), "result text") != 1 {
			t.Fatalf("invalid native continuation: %s", sent.Messages)
		}
		result := response(200, stream)
		result.Body = countedBody{Reader: strings.NewReader(stream), closed: &closed}
		return result, nil
	}))
	tools := []model.Tool{{Name: "read", Schema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}
	var continuation json.RawMessage
	if err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 20, Tools: tools, Messages: []model.Message{{Role: "user", Text: "inspect"}}}, func(e model.Event) error {
		if e.Type == "text_delta" {
			t.Fatal("thinking exposed")
		}
		if e.Type == "completed" {
			continuation = e.Continuation
			if !e.Usage.Known {
				t.Fatal("start input plus final output usage lost")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 20, Tools: tools, Continuation: continuation, Messages: []model.Message{{Role: "tool", ToolCallID: "call_1", Text: "result text"}}}, func(model.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if closed != 2 {
		t.Fatalf("SDK response bodies leaked: closed=%d", closed)
	}
}
