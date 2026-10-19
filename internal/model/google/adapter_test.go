package google

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/model"
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

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func newFixtureProvider(t *testing.T, client *fixtureClient) *Provider {
	t.Helper()
	provider, err := New(model.Config{Endpoint: "https://example.test", APIKey: "fixture-key", Model: "gemini-test", Client: client})
	if err != nil {
		t.Fatalf("%v requests=%d", err, len(client.requests))
	}
	return provider
}

func TestStreamTurnPreservesNativeHistoryAndParallelCalls(t *testing.T) {
	client := &fixtureClient{responses: []*http.Response{response(http.StatusOK, strings.Join([]string{
		`data: {"responseId":"resp-1","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"checking"}]}}]}`,
		`data: {"responseId":"resp-1","candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"id":"call-a","name":"lookup","args":{"key":"a"}},"thoughtSignature":"c2lnLWE="},{"functionCall":{"id":"call-b","name":"lookup","args":{"key":"b"}},"thoughtSignature":"c2lnLWI="}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7,"thoughtsTokenCount":3,"cachedContentTokenCount":4}}`}, "\n\n"))}}
	provider := newFixtureProvider(t, client)
	tools := []model.Tool{{Name: "lookup", Schema: []byte(`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"],"additionalProperties":false}`)}}
	var events []model.Event
	err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "find"}}, Tools: tools, MaxOutputTokens: 128}, func(event model.Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("%v requests=%d", err, len(client.requests))
	}
	if len(events) != 4 || events[0].Type != "text_delta" || events[1].Type != "tool_call" || events[2].Type != "tool_call" || events[3].Type != "completed" {
		t.Fatalf("unexpected events: %#v", events)
	}
	if events[3].Usage == nil || !events[3].Usage.Known || events[3].Usage.InputTokens != 12 || events[3].Usage.OutputTokens != 10 || events[3].Usage.CacheTokens != 4 {
		t.Fatalf("unexpected usage: %#v", events[3].Usage)
	}
	if !strings.Contains(string(events[3].Continuation), `"thoughtSignature":"c2lnLWE="`) {
		t.Fatalf("continuation lost thought signature: %s", events[3].Continuation)
	}
	client.responses = []*http.Response{response(http.StatusOK, `data: {"responseId":"resp-2","candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`+"\n\n")}
	var second []model.Event
	err = provider.StreamTurn(context.Background(), model.TurnRequest{Continuation: events[3].Continuation, Messages: []model.Message{{Role: "tool", ToolCallID: "call-a", Text: `{"value":1}`}}, MaxOutputTokens: 64}, func(event model.Event) error {
		second = append(second, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 || second[1].Type != "completed" {
		t.Fatalf("unexpected continuation events: %#v", second)
	}
	body, err := io.ReadAll(client.requests[1].Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"thoughtSignature":"c2lnLWE="`) || strings.Index(string(body), `"functionResponse"`) < strings.Index(string(body), `"thoughtSignature":"c2lnLWE="`) {
		t.Fatalf("continuation request did not preserve ordering/signature: %s", body)
	}
}

func TestProbeAndListModels(t *testing.T) {
	client := &fixtureClient{responses: []*http.Response{response(http.StatusOK, `{"models":[{"name":"models/gemini-test","inputTokenLimit":1000,"outputTokenLimit":200,"supportedActions":["generateContent"]}]}`), response(http.StatusOK, `{"models":[{"name":"models/gemini-test","inputTokenLimit":1000,"outputTokenLimit":200}]}`)}}
	provider := newFixtureProvider(t, client)
	caps, err := provider.Probe(context.Background())
	if err != nil || caps.Features["generate_content"].State != domain.Unknown || caps.Features["tool_calling"].State != domain.Unknown {
		t.Fatalf("unexpected probe: %#v %v", caps, err)
	}
	models, err := provider.ListModels(context.Background())
	if err != nil || len(models) != 1 || models[0].ContextLimit != 1000 {
		t.Fatalf("unexpected model list: %#v %v", models, err)
	}
}

func TestErrorsAreBoundedAndCancellationIsUncertain(t *testing.T) {
	client := &fixtureClient{responses: []*http.Response{response(http.StatusTooManyRequests, `{"error":{"code":429,"message":"quota"}}`)}}
	provider := newFixtureProvider(t, client)
	err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, MaxOutputTokens: 8}, func(model.Event) error { return nil })
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "quota" || !providerErr.Uncertain {
		t.Fatalf("unexpected quota error: %#v", err)
	}
}

func TestCancellationAfterDispatchIsUncertain(t *testing.T) {
	client := &fixtureClient{err: context.Canceled}
	provider := newFixtureProvider(t, client)
	err := provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "x"}}, MaxOutputTokens: 8}, func(model.Event) error { return nil })
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "canceled" || !providerErr.Uncertain {
		t.Fatalf("unexpected cancellation: %#v", err)
	}
}

func TestProbeRejectsUnlistedModel(t *testing.T) {
	client := &fixtureClient{responses: []*http.Response{response(http.StatusOK, `{"models":[{"name":"models/other"}]}`)}}
	provider := newFixtureProvider(t, client)
	_, err := provider.Probe(context.Background())
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "model_unsupported" {
		t.Fatalf("unexpected unsupported model error: %#v", err)
	}
}
