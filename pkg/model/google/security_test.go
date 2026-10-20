package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
)

func lookupTools() []model.Tool {
	return []model.Tool{{Name: "lookup", Schema: json.RawMessage(`{"type":"object","properties":{"key":{"type":"integer"}},"required":["key"],"additionalProperties":false}`)}}
}
func streamFixture(body string) *fixtureClient {
	return &fixtureClient{responses: []*http.Response{response(http.StatusOK, body)}}
}
func collect(t *testing.T, p *Provider, request model.TurnRequest) ([]model.Event, error) {
	t.Helper()
	var events []model.Event
	err := p.StreamTurn(context.Background(), request, func(event model.Event) error { events = append(events, event); return nil })
	return events, err
}
func assertNoExecution(t *testing.T, events []model.Event, err error) {
	t.Helper()
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Uncertain {
		t.Fatalf("expected uncertain protocol rejection: %v", err)
	}
	for _, event := range events {
		if event.Type == "tool_call" || event.Type == "completed" {
			t.Fatalf("executed rejected response: %s", event.Type)
		}
	}
}
func TestUnsafeTerminalAndFragmentedCallsCannotExecute(t *testing.T) {
	parts := `{"functionCall":{"id":"call","name":"lookup","args":{"key":1}}}`
	for _, finish := range []string{"MAX_TOKENS", "SAFETY", "RECITATION", "MALFORMED_FUNCTION_CALL", "OTHER", ""} {
		t.Run(finish, func(t *testing.T) {
			body := fmt.Sprintf("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[%s]},\"finishReason\":%q}]}\n\n", parts, finish)
			events, err := collect(t, newFixtureProvider(t, streamFixture(body)), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, Tools: lookupTools(), MaxOutputTokens: 16})
			assertNoExecution(t, events, err)
		})
	}
	for _, call := range []string{
		`{"id":"call","name":"lookup","args":{"key":1},"partialArgs":[]}`,
		`{"id":"call","name":"lookup","args":{"key":1},"willContinue":false}`,
	} {
		body := `data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":` + call + `}]},"finishReason":"STOP"}]}` + "\n\n"
		events, err := collect(t, newFixtureProvider(t, streamFixture(body)), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, Tools: lookupTools(), MaxOutputTokens: 16})
		assertNoExecution(t, events, err)
	}
	for _, name := range []string{"lookup", "changed"} {
		body := `data: {"candidates":[{"content":{"role":"model","parts":[` + parts + `,{"functionCall":{"id":"call","name":"` + name + `","args":{"key":2}}}]},"finishReason":"STOP"}]}` + "\n\n"
		events, err := collect(t, newFixtureProvider(t, streamFixture(body)), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, Tools: lookupTools(), MaxOutputTokens: 16})
		assertNoExecution(t, events, err)
	}
}
func TestNativeOrderMissingIDsAndIntegerPrecisionRoundTrip(t *testing.T) {
	body := `data: {"responseId":"native","candidates":[{"content":{"role":"model","parts":[{"text":"before","thoughtSignature":"YWJj"},{"functionCall":{"name":"lookup","args":{"key":9007199254740993}},"thoughtSignature":"ZGVm","opaqueFuture":{"n":9007199254740995}},{"text":"between"},{"functionCall":{"id":"native-second","name":"lookup","args":{"key":9007199254740997}},"thoughtSignature":"Z2hp"},{"text":"after"}]},"finishReason":"STOP"}]}` + "\n\n"
	client := streamFixture(body)
	provider := newFixtureProvider(t, client)
	events, err := collect(t, provider, model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "find"}}, Tools: lookupTools(), MaxOutputTokens: 16})
	if err != nil {
		t.Fatal(err)
	}
	var calls []model.ToolCall
	var continuation json.RawMessage
	for _, event := range events {
		if event.ToolCall != nil {
			calls = append(calls, *event.ToolCall)
		}
		if event.Type == "completed" {
			continuation = event.Continuation
		}
	}
	if len(calls) != 2 || !strings.HasPrefix(calls[0].ID, "reforge_gemini_") || calls[1].ID != "native-second" || string(calls[0].Arguments) != `{"key":9007199254740993}` {
		t.Fatalf("incorrect call association or precision: %#v", calls)
	}
	var history []json.RawMessage
	if json.Unmarshal(continuation, &history) != nil || len(history) != 2 {
		t.Fatal("invalid continuation")
	}
	native, err := readContent(history[1])
	if err != nil || len(native.Parts) != 5 {
		t.Fatalf("lost ordered parts: %v", err)
	}
	for index, want := range []string{`"before"`, `9007199254740993`, `"between"`, `"native-second"`, `"after"`} {
		if !strings.Contains(string(native.Parts[index]), want) {
			t.Fatalf("part%d reordered", index)
		}
	}
	if strings.Contains(string(history[1]), calls[0].ID) || !strings.Contains(string(history[1]), `9007199254740995`) {
		t.Fatal("native history was changed")
	}
	client.responses = []*http.Response{response(http.StatusOK, `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`+"\n\n")}
	events, err = collect(t, provider, model.TurnRequest{Continuation: continuation, Messages: []model.Message{{Role: "tool", ToolCallID: calls[0].ID, Text: `{"exact":9007199254740999}`}, {Role: "tool", ToolCallID: calls[1].ID, Text: `{"exact":9007199254740997}`}}, Tools: lookupTools(), MaxOutputTokens: 16})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(client.requests[1].Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`9007199254740993`, `9007199254740995`, `9007199254740997`, `9007199254740999`, `"thoughtSignature":"ZGVm"`, `"opaqueFuture"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("wire lost %s", want)
		}
	}
	var request struct {
		Contents []json.RawMessage `json:"contents"`
	}
	if json.Unmarshal(raw, &request) != nil {
		t.Fatal("bad wire JSON")
	}
	var answer struct {
		Parts []struct {
			Response map[string]json.RawMessage `json:"functionResponse"`
		} `json:"parts"`
	}
	if json.Unmarshal(request.Contents[2], &answer) != nil || len(answer.Parts) != 2 {
		t.Fatal("missing response")
	}
	if _, exists := answer.Parts[0].Response["id"]; exists {
		t.Fatal("fabricated native ID for legacy call")
	}
	if events[len(events)-1].Type != "completed" {
		t.Fatal("round trip failed")
	}
}
func TestUsagePresenceAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		known  bool
		output int64
	}{
		{`{}`, false, 0}, {`{"promptTokenCount":4}`, false, 0}, {`{"promptTokenCount":null,"candidatesTokenCount":0}`, false, 0},
		{`{"promptTokenCount":0,"candidatesTokenCount":0}`, true, 0},
		{`{"promptTokenCount":1,"candidatesTokenCount":2147483647,"thoughtsTokenCount":2147483647}`, true, 4294967294},
		{`{"promptTokenCount":4,"candidatesTokenCount":3,"totalTokenCount":6}`, false, 0},
		{`{"promptTokenCount":4,"candidatesTokenCount":3,"thoughtsTokenCount":2,"cachedContentTokenCount":2,"totalTokenCount":9}`, true, 5},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			body := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}],"usageMetadata":` + tc.raw + "}\n\n"
			events, err := collect(t, newFixtureProvider(t, streamFixture(body)), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, MaxOutputTokens: 16})
			if err != nil {
				t.Fatal(err)
			}
			usage := events[len(events)-1].Usage
			if usage.Known != tc.known || usage.OutputTokens != tc.output {
				t.Fatalf("incorrect usage: %#v", usage)
			}
		})
	}
}
func TestContinuationNullAndCallbackFailure(t *testing.T) {
	client := streamFixture("")
	provider := newFixtureProvider(t, client)
	for _, raw := range []string{`[null]`, `[{"role":"model","parts":[null]}]`, `[{"role":"model","parts":null}]`} {
		_, err := collect(t, provider, model.TurnRequest{Continuation: json.RawMessage(raw), MaxOutputTokens: 16})
		if err == nil || len(client.requests) != 0 {
			t.Fatal("invalid continuation dispatched")
		}
	}
	client.responses = []*http.Response{response(http.StatusOK, `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`+"\n\n")}
	err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, MaxOutputTokens: 16}, func(model.Event) error { return errors.New("consumer failed with secret") })
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Uncertain || strings.Contains(err.Error(), "secret") {
		t.Fatalf("callback uncertainty/redaction failed: %v", err)
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }
func TestCompletePrefixOversizedTailCannotReleaseTools(t *testing.T) {
	prefix := `data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call","name":"lookup","args":{"key":1}}}]},"finishReason":"STOP"}]}` + "\n\n"
	body := &trackedBody{Reader: strings.NewReader(prefix + strings.Repeat("\n", maxResponseBody))}
	client := &fixtureClient{responses: []*http.Response{{StatusCode: 200, Header: make(http.Header), Body: body}}}
	events, err := collect(t, newFixtureProvider(t, client), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, Tools: lookupTools(), MaxOutputTokens: 16})
	assertNoExecution(t, events, err)
	if !body.closed {
		t.Fatal("oversized response body not closed")
	}
}
func TestExplicitKeyAndBoundedProfile(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "environment-key")
	t.Setenv("GEMINI_API_KEY", "environment-key")
	t.Setenv("GOOGLE_GENAI_USE_VERTEXAI", "true")
	body := &trackedBody{Reader: strings.NewReader(`{"models":[{"name":"models/gemini-test"}]}`)}
	client := &fixtureClient{responses: []*http.Response{{StatusCode: 200, Header: make(http.Header), Body: body}}}
	provider := newFixtureProvider(t, client)
	if _, err := provider.ListModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := client.requests[0]
	if request.Header.Get("x-goog-api-key") != "fixture-key" || request.Header.Get("Authorization") != "" || request.URL.Host != "example.test" || !body.closed {
		t.Fatal("explicit credential, endpoint or close contract failed")
	}
}

type concurrentClient func(*http.Request) (*http.Response, error)

func (c concurrentClient) Do(request *http.Request) (*http.Response, error) { return c(request) }
func TestConcurrentTurnsKeepRawCaptureSeparate(t *testing.T) {
	client := concurrentClient(func(request *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		var body struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err = json.Unmarshal(raw, &body); err != nil {
			return nil, err
		}
		tag := body.Contents[0].Parts[0].Text
		return response(200, fmt.Sprintf("data: {\"responseId\":%q,\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":%q}]},\"finishReason\":\"STOP\"}]}\n\n", tag, tag)), nil
	})
	provider, err := New(model.Config{Endpoint: "https://example.test", APIKey: "fixture-key", Model: "gemini-test", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func(index int) {
			tag := fmt.Sprintf("turn-%d", index)
			completed := false
			err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: tag}}, MaxOutputTokens: 16}, func(event model.Event) error {
				if event.Type == "completed" {
					completed = true
					if event.ID != tag || strings.Count(string(event.Continuation), tag) != 2 {
						return fmt.Errorf("crossed turn capture")
					}
				}
				return nil
			})
			if err == nil && !completed {
				err = fmt.Errorf("missing completion")
			}
			done <- err
		}(i)
	}
	for i := 0; i < 8; i++ {
		if err = <-done; err != nil {
			t.Fatal(err)
		}
	}
}
