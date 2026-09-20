package compatible

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"reforge/internal/domain"
	"reforge/internal/model"
	"reforge/internal/network"
)

func TestRealLocalOllamaProfiles(t *testing.T) {
	if os.Getenv("REFORGE_LOCAL_OLLAMA_TEST") != "1" {
		t.Skip("explicit local Ollama fixture required")
	}
	for _, profile := range []string{"ollama", "responses"} {
		t.Run(profile, func(t *testing.T) {
			runnerID := domain.NewID()
			client, err := network.NewClient("http://127.0.0.1:55435", network.Options{Development: true, RunnerID: runnerID, PrivateRoute: &network.PrivateRoute{OrgID: domain.NewID(), ConnectionID: domain.NewID(), RunnerID: runnerID, Host: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}}})
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			provider, err := New(model.Config{Endpoint: "http://127.0.0.1:55435", Model: "qwen3:0.6b", APIKey: "local-ollama", Profile: profile, Client: client})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			if _, err = provider.ListModels(ctx); err != nil {
				t.Fatal(err)
			}
			tools := []model.Tool{{Name: "lookup", Description: "Read the named local fixture value.", Schema: json.RawMessage(`{"type":"object","properties":{"key":{"type":"string","enum":["reforge"]}},"required":["key"],"additionalProperties":false}`)}}
			var calls []model.ToolCall
			var completed model.Event
			request := model.TurnRequest{OperationID: "local-qualification", Model: "qwen3:0.6b", Messages: []model.Message{{Role: "user", Text: "Call lookup exactly once with key reforge to read its value. Do not answer without using lookup. /no_think"}}, Tools: tools, MaxOutputTokens: 512}
			err = provider.StreamTurn(ctx, request, func(event model.Event) error {
				if event.ToolCall != nil {
					calls = append(calls, *event.ToolCall)
				}
				if event.Type == "completed" {
					completed = event
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(calls) != 1 || completed.Type != "completed" || len(completed.Continuation) == 0 || completed.Usage == nil || !completed.Usage.Known {
				t.Fatalf("incomplete real model turn: tools=%d finish=%s usage=%+v", len(calls), completed.FinishReason, completed.Usage)
			}
			continuation := completed.Continuation
			completed = model.Event{}
			text := ""
			err = provider.StreamTurn(ctx, model.TurnRequest{Model: "qwen3:0.6b", Continuation: continuation, Messages: []model.Message{{Role: "tool", ToolCallID: calls[0].ID, Text: `{"value":"local fixture verified"}`}}, MaxOutputTokens: 512}, func(event model.Event) error {
				if event.Type == "text_delta" {
					text += event.Text
				}
				if event.Type == "completed" {
					completed = event
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if text == "" || completed.Type != "completed" || completed.Usage == nil || !completed.Usage.Known {
				t.Fatalf("real continuation missing text or usage: finish=%s usage=%+v", completed.FinishReason, completed.Usage)
			}
			t.Logf("real Ollama profile=%s tools=1 continuation=true usage=input:%d output:%d", profile, completed.Usage.InputTokens, completed.Usage.OutputTokens)
		})
	}
}
