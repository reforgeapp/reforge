package model

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/reforgeapp/reforge/internal/skills"
)

const MaxTurnTimeout = 10 * time.Minute

type Turn struct {
	OperationID     string          `json:"operation_id"`
	Model           string          `json:"model"`
	System          string          `json:"system"`
	Messages        []Message       `json:"messages"`
	Tools           []Tool          `json:"tools"`
	MaxOutputTokens int             `json:"max_output_tokens"`
	Continuation    json.RawMessage `json:"continuation,omitempty"`
	TimeoutMS       int64           `json:"timeout_ms"`
	Session         string          `json:"-"`
}

func (Turn) String() string         { return "model turn [redacted]" }
func (t Turn) GoString() string     { return t.String() }
func (t Turn) LogValue() slog.Value { return slog.StringValue(t.String()) }
func (t Turn) Request() TurnRequest {
	return TurnRequest{OperationID: t.OperationID, Model: t.Model, System: t.System, Messages: t.Messages, Tools: t.Tools, MaxOutputTokens: t.MaxOutputTokens, Continuation: t.Continuation, Session: t.Session}
}
func (t Turn) WithSkills() (Turn, error) {
	var err error
	t.System, err = skills.Apply(t.System)
	if err != nil {
		return Turn{}, err
	}
	tools := make([]Tool, 0, len(t.Tools)+1)
	for _, tool := range t.Tools {
		if tool.Name != skills.ToolName {
			tools = append(tools, tool)
		}
	}
	t.Tools = append(tools, Tool{Name: skills.ToolName, Description: skills.ToolDescription, Schema: json.RawMessage(skills.ToolSchema)})
	if !t.Valid() {
		return Turn{}, errors.New("invalid model turn after loading required skills")
	}
	return t, nil
}
func (t Turn) Valid() bool {
	if t.OperationID == "" || t.Model == "" || len(t.Model) > 200 || t.MaxOutputTokens < 1 || t.MaxOutputTokens > 131072 || t.TimeoutMS < 1000 || t.TimeoutMS > MaxTurnTimeout.Milliseconds() || len(t.Messages) == 0 || len(t.Messages) > 400 || len(t.Tools) > 32 {
		return false
	}
	b, err := json.Marshal(t)
	if err != nil || len(b) > MaxRequestBytes/2 {
		return false
	}
	for _, m := range t.Messages {
		if m.Role != "user" && m.Role != "assistant" && m.Role != "tool" {
			return false
		}
		if m.Role == "tool" && m.ToolCallID == "" {
			return false
		}
		if len(m.Opaque) > 0 {
			return false
		}
	}
	_, err = CompileTools(t.Tools)
	return err == nil
}

type TurnResult struct {
	ProviderID   string          `json:"provider_id"`
	Text         string          `json:"text"`
	ToolCalls    []ToolCall      `json:"tool_calls"`
	Usage        Usage           `json:"usage"`
	Continuation json.RawMessage `json:"continuation,omitempty"`
	FinishReason string          `json:"finish_reason"`
}

func (TurnResult) String() string         { return "model result [redacted]" }
func (t TurnResult) GoString() string     { return t.String() }
func (t TurnResult) LogValue() slog.Value { return slog.StringValue(t.String()) }
func CollectTurn(ctx context.Context, p ModelProvider, t Turn) (TurnResult, error) {
	out := TurnResult{ToolCalls: []ToolCall{}}
	t, err := t.WithSkills()
	if err != nil {
		return out, err
	}
	schemas, _ := CompileTools(t.Tools)
	completed := false
	bytes := 0
	seen := map[string]bool{}
	err = p.StreamTurn(ctx, t.Request(), func(e Event) error {
		if completed {
			return errors.New("model emitted after completion")
		}
		bytes += len(e.Text) + len(e.Continuation)
		if e.ToolCall != nil {
			bytes += len(e.ToolCall.Arguments)
		}
		if bytes > 2<<20 {
			return errors.New("model response limit exceeded")
		}
		switch e.Type {
		case "text_delta":
			out.Text += e.Text
		case "tool_call":
			if e.ToolCall == nil || e.ToolCall.ID == "" || seen[e.ToolCall.ID] || len(out.ToolCalls) >= 32 {
				return errors.New("invalid model tool result")
			}
			if err := schemas.Check(e.ToolCall); err != nil {
				return err
			}
			seen[e.ToolCall.ID] = true
			out.ToolCalls = append(out.ToolCalls, *e.ToolCall)
		case "completed":
			if e.Usage == nil || !e.Usage.Known || e.Usage.InputTokens < 0 || e.Usage.OutputTokens < 0 || e.Usage.CacheTokens < 0 || e.Usage.CacheCreationTokens < 0 {
				return errors.New("model completed without known usage")
			}
			completed = true
			out.ProviderID = e.ID
			out.FinishReason = e.FinishReason
			out.Usage = *e.Usage
			out.Continuation = append(json.RawMessage(nil), e.Continuation...)
		}
		return nil
	})
	if err == nil && !completed {
		err = errors.New("model stream ended without completion")
	}
	if err != nil {
		return TurnResult{}, err
	}
	return out, nil
}
