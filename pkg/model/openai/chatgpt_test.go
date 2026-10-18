package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
)

func TestChatGPTPlanCatalogAndRequestContract(t *testing.T) {
	calls := 0
	provider, err := New(model.Config{Endpoint: "https://api.openai.com/v1", APIKey: "oauth-fixture", Model: "eligible-model", Profile: ChatGPTProfile, Client: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Header.Get("Authorization") != "Bearer oauth-fixture" {
			t.Fatal("missing subscription credential")
		}
		if request.Method == http.MethodGet {
			if request.URL.Path != "/v1/models" {
				t.Fatal("subscription probe must use account catalog")
			}
			return response(200, `{"models":[{"slug":"hidden","visibility":"hide"},{"slug":"eligible-model","display_name":"Eligible model","visibility":"list"}],"data":[{"id":"paid-model"}]}`), nil
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(request.Body).Decode(&body) != nil {
			t.Fatal("invalid body")
		}
		for _, field := range []string{"max_output_tokens", "service_tier", "previous_response_id", "conversation", "background", "metadata", "temperature", "max_tool_calls", "user"} {
			if _, ok := body[field]; ok {
				t.Fatalf("unsupported plan field %s", field)
			}
		}
		if string(body["stream"]) != "true" || string(body["store"]) != "false" {
			t.Fatal("subscription privacy contract lost")
		}
		var tools []struct {
			Type, Name string
			Tools      []struct{ Name string }
		}
		if json.Unmarshal(body["tools"], &tools) != nil || len(tools) != 1 || tools[0].Type != "namespace" || tools[0].Name != "reforge" || len(tools[0].Tools) != 1 || tools[0].Tools[0].Name != "lookup" {
			t.Fatal("repair tools missing from namespace")
		}
		var input []map[string]any
		if json.Unmarshal(body["input"], &input) != nil || len(input) != 2 || input[0]["type"] != "reasoning" || input[1]["role"] != "user" {
			t.Fatal("full turn history lost")
		}
		return response(200, sse("response.completed", `{"response":{"id":"resp_plan","output":[],"usage":{"input_tokens":3,"output_tokens":2}}}`)), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	models, err := provider.ListModels(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "eligible-model" || models[0].Name != "Eligible model" {
		t.Fatalf("account catalog wrong: %+v %v", models, err)
	}
	caps, err := provider.Probe(context.Background())
	if err != nil || caps.BillingRoute != "openai.chatgpt_plan" {
		t.Fatalf("probe failed: %+v %v", caps, err)
	}
	completed := false
	err = provider.StreamTurn(context.Background(), model.TurnRequest{Messages: []model.Message{{Role: "user", Text: "fix"}}, Continuation: json.RawMessage(`[{"type":"reasoning","encrypted_content":"opaque"}]`), Tools: []model.Tool{{Name: "lookup", Schema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)}}, MaxOutputTokens: 4096}, func(event model.Event) error { completed = event.Type == "completed"; return nil })
	if err != nil || !completed || calls != 3 {
		t.Fatalf("subscription inference failed: %v", err)
	}
}

func TestChatGPTLateQuotaNeverDispatchesRepairTools(t *testing.T) {
	provider, err := New(model.Config{Endpoint: "https://api.openai.com/v1", APIKey: "oauth-fixture", Model: "eligible-model", Profile: ChatGPTProfile, Client: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(200, sse("response.output_item.done", `{"output_index":0,"item":{"type":"function_call","id":"item","call_id":"call","name":"lookup","namespace":"reforge","arguments":"{}"}}`)+sse("response.failed", `{"response":{"error":{"code":"subscription_sharing_usage_limit_exceeded","message":"private provider diagnostic"}}}`)), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	events := 0
	err = provider.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 4096, Tools: []model.Tool{{Name: "lookup", Schema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)}}}, func(model.Event) error { events++; return nil })
	var failure *domain.ProviderError
	if !errors.As(err, &failure) || failure.Kind != "quota" || !failure.Uncertain || events != 0 {
		t.Fatalf("late failure dispatched tools or lost accounting: events=%d error=%v", events, err)
	}
}

func TestChatGPTPlanRejectsAlternateBillingEndpoint(t *testing.T) {
	for _, endpoint := range []string{"https://chatgpt.com/backend-api", "https://gateway.example/v1", "http://api.openai.com/v1"} {
		if _, err := New(model.Config{Endpoint: endpoint, APIKey: "oauth-fixture", Model: "eligible-model", Profile: ChatGPTProfile, Client: roundTripFunc(nil)}); err == nil {
			t.Fatalf("accepted unsupported endpoint %s", endpoint)
		}
	}
}
