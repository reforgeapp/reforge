package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"reforge/internal/domain"
	"reforge/internal/model"
)

const (
	defaultProfile = "messages"
	maxPage        = 1000
)

type Provider struct {
	config model.Config
	client anthropic.Client
}

func New(config model.Config) (*Provider, error) {
	return build(config, true)
}

func NewCatalog(config model.Config) (model.ModelLister, error) {
	p, err := build(config, false)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func build(config model.Config, requireModel bool) (*Provider, error) {
	if config.Client == nil {
		return nil, providerError("configuration", "an HTTP client is required", false)
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, providerError("configuration", "an API key is required", false)
	}
	if requireModel && strings.TrimSpace(config.Model) == "" {
		return nil, providerError("configuration", "a model is required", false)
	}
	profile := config.Profile
	if profile == "" {
		profile = defaultProfile
	}
	if profile != defaultProfile {
		return nil, providerError("configuration", "unsupported Anthropic profile", false)
	}
	base, err := fixedBaseURL(config.Endpoint)
	if err != nil {
		return nil, err
	}
	if client, ok := config.Client.(*http.Client); ok {
		clone := *client
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		config.Client = &clone
	}
	client := anthropic.NewClient(
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(config.APIKey),
		option.WithBaseURL(base),
		option.WithHTTPClient(config.Client),
		option.WithMaxRetries(0),
		option.WithMiddleware(limitResponseBody),
	)
	config.Profile = profile
	return &Provider{config: config, client: client}, nil
}

func (p *Provider) Probe(ctx context.Context) (model.Capabilities, error) {
	metadata, err := p.client.Models.Get(ctx, p.config.Model, anthropic.ModelGetParams{})
	if err != nil {
		return model.Capabilities{}, normalizeError(ctx, err, false)
	}
	if metadata == nil || metadata.ID != p.config.Model {
		return model.Capabilities{}, providerError("protocol", "Anthropic returned no model metadata", false)
	}
	features := map[string]domain.Capability{
		"messages":          capability(domain.Supported, "configured Messages API", "adapter"),
		"streaming":         capability(domain.Supported, "Messages SSE contract", "adapter"),
		"tool_calling":      capability(domain.Unknown, "model metadata has no general tool-call flag", "model metadata"),
		"thinking":          capability(domain.Unknown, "model capability has not been verified", "model metadata"),
		"structured_output": capability(domain.Unknown, "model capability has not been verified", "model metadata"),
	}
	if metadata.Capabilities.Thinking.JSON.Supported.Valid() {
		features["thinking"] = capabilityFromBool(metadata.Capabilities.Thinking.Supported, "model metadata", "thinking capability")
	}
	if metadata.Capabilities.StructuredOutputs.JSON.Supported.Valid() {
		features["structured_output"] = capabilityFromBool(metadata.Capabilities.StructuredOutputs.Supported, "model metadata", "structured output capability")
	}
	return model.Capabilities{Provider: "anthropic", Model: metadata.ID, BillingRoute: "anthropic.messages", Features: features}, nil
}

func (p *Provider) ListModels(ctx context.Context) ([]model.Model, error) {
	page, err := p.client.Models.List(ctx, anthropic.ModelListParams{Limit: anthropic.Int(maxPage)})
	if err != nil {
		return nil, normalizeError(ctx, err, false)
	}
	if page == nil {
		return nil, providerError("protocol", "Anthropic returned no model list", false)
	}
	result := []model.Model{}
	seen := map[string]bool{}
	for pages := 0; pages < 20; pages++ {
		for _, item := range page.Data {
			if item.ID == "" || seen[item.ID] {
				return nil, providerError("protocol", "invalid or repeated model identity", false)
			}
			seen[item.ID] = true
			result = append(result, model.Model{ID: item.ID, Name: item.DisplayName, ContextLimit: int(item.MaxInputTokens), OutputLimit: int(item.MaxTokens)})
		}
		if !page.HasMore {
			return result, nil
		}
		page, err = page.GetNextPage()
		if err != nil {
			return nil, normalizeError(ctx, err, false)
		}
		if page == nil {
			return nil, providerError("protocol", "incomplete model pagination", false)
		}
	}
	return nil, providerError("protocol", "model pagination exceeds limit", false)
}

func (p *Provider) StreamTurn(ctx context.Context, request model.TurnRequest, emit func(model.Event) error) error {
	if emit == nil {
		return providerError("configuration", "an event callback is required", false)
	}
	if err := ctx.Err(); err != nil {
		return normalizeError(ctx, err, false)
	}
	modelID := request.Model
	if modelID == "" {
		modelID = p.config.Model
	}
	if modelID != p.config.Model {
		return providerError("model_unsupported", "requested model is not the configured model", false)
	}
	if request.MaxOutputTokens <= 0 || request.MaxOutputTokens > 1<<20 {
		return providerError("invalid_request", "max output tokens must be positive", false)
	}
	params, err := messageParams(request, modelID)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(params)
	if err != nil || len(encoded) > model.MaxRequestBytes {
		return providerError("invalid_request", "Anthropic request exceeds the size limit", false)
	}
	stream := p.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	var pending []model.Event
	deliver := emit
	emit = func(event model.Event) error {
		if event.Type == "tool_call" {
			pending = append(pending, event)
			return nil
		}
		if event.Type == "completed" {
			for _, tool := range pending {
				if err := deliver(tool); err != nil {
					return err
				}
			}
		}
		return deliver(event)
	}
	message := anthropic.Message{}
	stopped := make(map[int64]bool)
	emitted := make(map[int64]bool)
	toolStates := make(map[int64]*streamTool)
	schemas, err := model.CompileTools(request.Tools)
	if err != nil {
		return err
	}
	inputKnown, outputKnown := false, false
	started := true
	completed := false
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return normalizeError(ctx, err, true)
		}
		switch variant := event.AsAny().(type) {
		case anthropic.MessageStartEvent:
			inputKnown = variant.Message.Usage.JSON.InputTokens.Valid()
		case anthropic.ContentBlockStartEvent:
			if variant.ContentBlock.Type == "tool_use" {
				state := &streamTool{ID: variant.ContentBlock.ID, Name: variant.ContentBlock.Name}
				if raw, err := json.Marshal(variant.ContentBlock.Input); err == nil && string(raw) != "{}" && string(raw) != "null" {
					state.Arguments.Write(raw)
				}
				toolStates[variant.Index] = state
			}
		case anthropic.ContentBlockDeltaEvent:
			switch variant.Delta.Type {
			case "text_delta":
				if variant.Delta.Text != "" {
					if err := emitOrCancel(ctx, emit, model.Event{Type: "text_delta", ID: message.ID, Text: variant.Delta.Text}); err != nil {
						return normalizeError(ctx, err, true)
					}
				}
			case "input_json_delta":
				if state := toolStates[variant.Index]; state != nil {
					state.Arguments.WriteString(variant.Delta.PartialJSON)
				}
			}
		case anthropic.ContentBlockStopEvent:
			stopped[variant.Index] = true
			if state := toolStates[variant.Index]; state != nil {
				if err := emitTool(state, schemas, emitted, variant.Index, emit, ctx); err != nil {
					return normalizeError(ctx, err, true)
				}
			}
		case anthropic.MessageDeltaEvent:
			inputKnown = inputKnown || variant.Usage.JSON.InputTokens.Valid()
			outputKnown = outputKnown || variant.Usage.JSON.OutputTokens.Valid()
		case anthropic.MessageStopEvent:
			completed = true
			for index := range message.Content {
				if !stopped[int64(index)] {
					return providerError("protocol", "tool content block did not complete", true)
				}
			}
			if message.Usage.InputTokens < 0 || message.Usage.OutputTokens < 0 || message.Usage.CacheCreationInputTokens < 0 || message.Usage.CacheReadInputTokens < 0 || message.Usage.InputTokens > 1<<40 || message.Usage.OutputTokens > 1<<40 || message.Usage.CacheCreationInputTokens > 1<<40 || message.Usage.CacheReadInputTokens > 1<<40 {
				return providerError("protocol", "invalid provider usage", true)
			}
			usage := &model.Usage{Known: inputKnown && outputKnown, Source: "completed_message", InputTokens: message.Usage.InputTokens + message.Usage.CacheCreationInputTokens + message.Usage.CacheReadInputTokens, OutputTokens: message.Usage.OutputTokens, CacheTokens: message.Usage.CacheReadInputTokens, CacheCreationTokens: message.Usage.CacheCreationInputTokens}
			var wire struct {
				Content json.RawMessage `json:"content"`
			}
			if json.Unmarshal([]byte(message.RawJSON()), &wire) != nil || len(wire.Content) == 0 {
				return providerError("protocol", "invalid continuation", true)
			}
			history, err := buildMessages(request)
			if err != nil {
				return err
			}
			history = append(history, map[string]any{"role": "assistant", "content": wire.Content})
			continuation, err := json.Marshal(history)
			if err != nil || len(continuation) > model.MaxRequestBytes {
				return providerError("protocol", "continuation exceeds limit", true)
			}
			if err := emitOrCancel(ctx, emit, model.Event{Type: "completed", ID: message.ID, Usage: usage, Continuation: continuation, FinishReason: string(message.StopReason)}); err != nil {
				return normalizeError(ctx, err, true)
			}
			return nil
		}
	}
	if err := stream.Err(); err != nil {
		return normalizeError(ctx, err, started)
	}
	if completed {
		return nil
	}
	if ctx.Err() != nil {
		return normalizeError(ctx, ctx.Err(), started)
	}
	return providerError("protocol", "Anthropic stream ended before message_stop", started)
}

func (p *Provider) EstimateUsage(request model.TurnRequest) model.Usage {
	bytesUsed := len(request.System) + 1024
	for _, message := range request.Messages {
		bytesUsed += len(message.Role) + len(message.Text) + len(message.ToolCallID) + len(message.Opaque) + 256
		for _, call := range message.ToolCalls {
			bytesUsed += len(call.ID) + len(call.Name) + len(call.Arguments) + 128
		}
	}
	for _, tool := range request.Tools {
		bytesUsed += len(tool.Name) + len(tool.Description) + len(tool.Schema) + 256
	}
	bytesUsed += len(request.Continuation) + 256
	return model.Usage{InputTokens: int64(bytesUsed), OutputTokens: int64(request.MaxOutputTokens), Known: false, Source: "conservative_byte_upper_bound"}
}

func messageParams(request model.TurnRequest, modelID string) (anthropic.MessageNewParams, error) {
	if _, err := model.CompileTools(request.Tools); err != nil {
		return anthropic.MessageNewParams{}, err
	}
	input, err := buildMessages(request)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	tools := make([]any, 0, len(request.Tools))
	for _, tool := range request.Tools {
		var schema map[string]any
		if json.Unmarshal(tool.Schema, &schema) != nil || schema == nil {
			return anthropic.MessageNewParams{}, providerError("invalid_request", "tool schema must be an object", false)
		}
		tools = append(tools, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": schema})
	}
	body := map[string]any{"model": modelID, "max_tokens": request.MaxOutputTokens, "messages": input, "stream": true}
	if request.System != "" {
		body["system"] = []any{map[string]any{"type": "text", "text": request.System}}
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return anthropic.MessageNewParams{}, providerError("invalid_request", "could not encode message request", false)
	}
	var params anthropic.MessageNewParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return anthropic.MessageNewParams{}, providerError("invalid_request", "could not encode message request", false)
	}
	return params, nil
}

func buildMessages(request model.TurnRequest) ([]any, error) {
	messages := make([]any, 0, len(request.Messages)+2)
	if len(request.Continuation) > 0 {
		var prior []json.RawMessage
		if len(request.Continuation) > model.MaxRequestBytes || json.Unmarshal(request.Continuation, &prior) != nil {
			return nil, providerError("invalid_request", "invalid opaque history", false)
		}
		for _, item := range prior {
			messages = append(messages, item)
		}
	}
	for _, message := range request.Messages {
		if message.ToolCallID != "" && message.Role == "tool" {
			message.Role = "user"
		}
		if message.Role != "user" && message.Role != "assistant" {
			return nil, providerError("invalid_request", "Anthropic messages only support user and assistant roles", false)
		}
		if len(message.Opaque) > 0 {
			if !json.Valid(message.Opaque) {
				return nil, providerError("invalid_request", "invalid opaque continuation", false)
			}
			var blocks any
			if err := json.Unmarshal(message.Opaque, &blocks); err != nil {
				return nil, providerError("invalid_request", "invalid opaque continuation", false)
			}
			messages = append(messages, map[string]any{"role": message.Role, "content": blocks})
			continue
		}
		content := make([]any, 0, 1+len(message.ToolCalls))
		if message.Text != "" && message.ToolCallID == "" {
			content = append(content, map[string]any{"type": "text", "text": message.Text})
		}
		for _, call := range message.ToolCalls {
			if call.ID == "" || !validName(call.Name) || !json.Valid(call.Arguments) {
				return nil, providerError("invalid_request", "invalid tool call", false)
			}
			var input any
			if err := json.Unmarshal(call.Arguments, &input); err != nil {
				return nil, providerError("invalid_request", "invalid tool call arguments", false)
			}
			content = append(content, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": input})
		}
		if message.ToolCallID != "" {
			if message.Role != "user" {
				return nil, providerError("invalid_request", "tool results must be user messages", false)
			}
			content = append(content, map[string]any{"type": "tool_result", "tool_use_id": message.ToolCallID, "content": message.Text})
		}
		if len(content) == 0 {
			return nil, providerError("invalid_request", "message has no content", false)
		}
		messages = append(messages, map[string]any{"role": message.Role, "content": content})
	}
	if len(messages) == 0 {
		return nil, providerError("invalid_request", "at least one message is required", false)
	}
	return messages, nil
}

type streamTool struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

func emitTool(state *streamTool, schemas model.ToolSchemas, emitted map[int64]bool, index int64, emit func(model.Event) error, ctx context.Context) error {
	args := state.Arguments.String()
	if args == "" {
		args = "{}"
	}
	call := model.ToolCall{ID: state.ID, Name: state.Name, Arguments: json.RawMessage(args)}
	if err := schemas.Validate(call); err != nil {
		return err
	}
	if emitted[index] {
		return nil
	}
	emitted[index] = true
	return emitOrCancel(ctx, emit, model.Event{Type: "tool_call", ID: state.ID, ToolCall: &call})
}

func fixedBaseURL(endpoint string) (string, error) {
	base, err := url.Parse(endpoint)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return "", providerError("configuration", "endpoint must be a fixed HTTP origin", false)
	}
	base.Path = strings.TrimRight(base.Path, "/")
	if strings.HasSuffix(base.Path, "/v1") {
		base.Path = strings.TrimSuffix(base.Path, "/v1")
	}
	return strings.TrimRight(base.String(), "/") + "/", nil
}

func normalizeError(ctx context.Context, err error, uncertain bool) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return providerError("canceled", "Anthropic request canceled", uncertain)
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		kind := "provider"
		switch apiErr.StatusCode {
		case 401:
			kind = "auth"
		case 404:
			kind = "model_unsupported"
		case 408, 409, 429, 529:
			kind = "rate_limit"
		case 400, 413, 422:
			kind = "invalid_request"
		case 403:
			kind = "auth"
		}
		return providerError(kind, "Anthropic API request failed", uncertain || apiErr.StatusCode >= 500 || apiErr.StatusCode == 429 || apiErr.StatusCode == 529)
	}
	return providerError("transport", "Anthropic request failed", uncertain)
}

func providerError(kind, message string, uncertain bool) *domain.ProviderError {
	return &domain.ProviderError{Kind: kind, Message: message, Uncertain: uncertain}
}

type limitedBody struct {
	io.Reader
	io.Closer
}

func limitResponseBody(request *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	response, err := next(request)
	if response != nil && response.Body != nil {
		response.Body = &limitedBody{Reader: io.LimitReader(response.Body, 16<<20), Closer: response.Body}
	}
	return response, err
}

func emitOrCancel(ctx context.Context, emit func(model.Event) error, event model.Event) error {
	if err := emit(event); err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return providerError("canceled", "Anthropic request canceled", true)
		}
		return err
	}
	return nil
}

func capability(state domain.CapabilityState, reason, source string) domain.Capability {
	return domain.Capability{State: state, Scope: "configured model", Reason: reason, Source: source, LastChecked: time.Now().UTC()}
}

func capabilityFromBool(supported bool, source, reason string) domain.Capability {
	state := domain.Unsupported
	if supported {
		state = domain.Supported
	}
	return capability(state, reason, source)
}

func validName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

var _ model.ModelProvider = (*Provider)(nil)
