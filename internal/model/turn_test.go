package model

import (
	"context"
	"encoding/json"
	"testing"
)

type turnFixture struct{ events []Event }

func (turnFixture) Probe(context.Context) (Capabilities, error) { return Capabilities{}, nil }
func (turnFixture) ListModels(context.Context) ([]Model, error) { return nil, nil }
func (turnFixture) EstimateUsage(TurnRequest) Usage             { return Usage{} }
func (p turnFixture) StreamTurn(_ context.Context, _ TurnRequest, emit func(Event) error) error {
	for _, e := range p.events {
		if err := emit(e); err != nil {
			return err
		}
	}
	return nil
}
func TestTurnRequiresKnownCompletionAndPreservesContinuation(t *testing.T) {
	in := Turn{OperationID: "operation", Model: "example", Messages: []Message{{Role: "user", Text: "test"}}, TimeoutMS: 1000, MaxOutputTokens: 20}
	usage := &Usage{Known: true, InputTokens: 4, OutputTokens: 3}
	completed := Event{Type: "completed", ID: "native", Usage: usage, Continuation: json.RawMessage(`{"signature":"opaque"}`)}
	for _, events := range [][]Event{{{Type: "text_delta", Text: "partial"}}, {{Type: "completed", Usage: &Usage{Known: false}}}, {completed, {Type: "text_delta", Text: "late"}}, {{Type: "tool_call", ToolCall: &ToolCall{ID: "unexpected", Name: "unknown", Arguments: json.RawMessage(`{}`)}}}} {
		if _, err := CollectTurn(context.Background(), turnFixture{events}, in); err == nil {
			t.Fatal("incomplete or invalid provider result accepted")
		}
	}
	got, err := CollectTurn(context.Background(), turnFixture{[]Event{{Type: "text_delta", Text: "answer"}, completed}}, in)
	if err != nil || got.Text != "answer" || string(got.Continuation) != string(completed.Continuation) {
		t.Fatalf("continuation lost: %v", err)
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TurnResult
	if err = json.Unmarshal(body, &decoded); err != nil || string(decoded.Continuation) != string(got.Continuation) {
		t.Fatal("wire lost continuation")
	}
}
