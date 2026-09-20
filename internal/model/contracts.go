package model

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"reforge/internal/domain"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}
type Config struct {
	Endpoint string
	APIKey   string `json:"-"`
	Model    string
	Client   HTTPClient `json:"-"`
	Profile  string
}

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(slog.String("model", c.Model), slog.String("profile", c.Profile))
}

func (c Config) String() string   { return "model connection " + c.Model }
func (c Config) GoString() string { return c.String() }

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
type Message struct {
	Role       string          `json:"role"`
	Text       string          `json:"text"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Opaque     json.RawMessage `json:"-"`
}
type TurnRequest struct {
	OperationID     string
	Model           string
	System          string
	Messages        []Message
	Tools           []Tool
	MaxOutputTokens int
	Continuation    json.RawMessage
}
type Usage struct {
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheTokens  int64  `json:"cache_tokens"`
	Known        bool   `json:"known"`
	Source       string `json:"source"`
}
type Event struct {
	Type         string          `json:"type"`
	ID           string          `json:"id"`
	Text         string          `json:"text,omitempty"`
	ToolCall     *ToolCall       `json:"tool_call,omitempty"`
	Usage        *Usage          `json:"usage,omitempty"`
	Continuation json.RawMessage `json:"-"`
	FinishReason string          `json:"finish_reason,omitempty"`
}
type Capabilities struct {
	Provider     string                       `json:"provider"`
	Model        string                       `json:"model"`
	Features     map[string]domain.Capability `json:"features"`
	BillingRoute string                       `json:"billing_route"`
}
type Model struct {
	ID           string `json:"id"`
	ContextLimit int    `json:"context_limit"`
	OutputLimit  int    `json:"output_limit"`
}
type ModelProvider interface {
	Probe(context.Context) (Capabilities, error)
	ListModels(context.Context) ([]Model, error)
	StreamTurn(context.Context, TurnRequest, func(Event) error) error
	EstimateUsage(TurnRequest) Usage
}
