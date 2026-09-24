package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"reforge/internal/domain"
	"reforge/internal/model"
)

const (
	defaultProfile  = "responses"
	maxResponseBody = 8 << 20
	maxStreamEvent  = 1 << 20
	maxStreamBytes  = 16 << 20
)

type Provider struct {
	config model.Config
	base   *url.URL
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
		return nil, &domain.ProviderError{Kind: "configuration", Message: "an HTTP client is required"}
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "an API key is required"}
	}
	if requireModel && strings.TrimSpace(config.Model) == "" {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "a model is required"}
	}
	profile := config.Profile
	if profile == "" {
		profile = defaultProfile
	}
	if profile != defaultProfile {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "unsupported OpenAI profile"}
	}
	base, err := url.Parse(config.Endpoint)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "endpoint must be a fixed HTTP origin"}
	}
	base.Path = strings.TrimRight(base.Path, "/")
	if !strings.HasSuffix(base.Path, "/v1") {
		base.Path += "/v1"
	}
	if client, ok := config.Client.(*http.Client); ok {
		clone := *client
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		config.Client = &clone
	}
	config.Profile = profile
	return &Provider{config: config, base: base}, nil
}

func (p *Provider) Probe(ctx context.Context) (model.Capabilities, error) {
	response, err := p.do(ctx, http.MethodGet, "/models/"+url.PathEscape(p.config.Model), nil)
	if err != nil {
		return model.Capabilities{}, classifyTransport(ctx, err, false)
	}
	if response == nil || response.Body == nil {
		return model.Capabilities{}, &domain.ProviderError{Kind: "protocol", Message: "provider returned no response body"}
	}
	defer response.Body.Close()
	if err := checkResponse(response); err != nil {
		return model.Capabilities{}, err
	}
	var metadata struct {
		ID string `json:"id"`
	}
	if err := decodeBounded(response.Body, &metadata); err != nil {
		return model.Capabilities{}, &domain.ProviderError{Kind: "protocol", Message: "invalid model metadata"}
	}
	modelID := metadata.ID
	if modelID != p.config.Model {
		return model.Capabilities{}, &domain.ProviderError{Kind: "protocol", Message: "model metadata identity mismatch"}
	}
	return model.Capabilities{
		Provider:     "openai",
		Model:        modelID,
		BillingRoute: "openai.responses",
		Features: map[string]domain.Capability{
			"responses":    capability(domain.Supported, "configured Responses contract", "adapter"),
			"streaming":    capability(domain.Unknown, "model metadata does not verify streaming", "model metadata"),
			"tool_calling": capability(domain.Unknown, "requires a contract capability profile", "model metadata"),
			"usage":        capability(domain.Unknown, "reported only by a completed response", "model metadata"),
		},
	}, nil
}

func (p *Provider) ListModels(ctx context.Context) ([]model.Model, error) {
	response, err := p.do(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, classifyTransport(ctx, err, false)
	}
	if response == nil || response.Body == nil {
		return nil, &domain.ProviderError{Kind: "protocol", Message: "provider returned no response body"}
	}
	defer response.Body.Close()
	if err := checkResponse(response); err != nil {
		return nil, err
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := decodeBounded(response.Body, &payload); err != nil {
		return nil, &domain.ProviderError{Kind: "protocol", Message: "invalid model list"}
	}
	models := make([]model.Model, 0, len(payload.Data))
	for _, entry := range payload.Data {
		if entry.ID != "" {
			models = append(models, model.Model{ID: entry.ID})
		}
	}
	return models, nil
}

func (p *Provider) StreamTurn(ctx context.Context, request model.TurnRequest, emit func(model.Event) error) error {
	if emit == nil {
		return &domain.ProviderError{Kind: "configuration", Message: "an event callback is required"}
	}
	if err := ctx.Err(); err != nil {
		return canceledError(err)
	}
	modelID := request.Model
	if modelID == "" {
		modelID = p.config.Model
	}
	if modelID != p.config.Model {
		return &domain.ProviderError{Kind: "model_unsupported", Message: "requested model is not the configured model"}
	}
	validators, err := model.CompileTools(request.Tools)
	if err != nil {
		return err
	}
	body, err := p.turnBody(request, modelID)
	if err != nil {
		return err
	}
	response, err := p.do(ctx, http.MethodPost, "/responses", body)
	if err != nil {
		return classifyTransport(ctx, err, true)
	}
	if response == nil || response.Body == nil {
		return &domain.ProviderError{Kind: "protocol", Message: "provider returned no response body"}
	}
	defer response.Body.Close()
	if err := checkResponse(response); err != nil {
		return err
	}
	var sent struct {
		Input []json.RawMessage `json:"input"`
	}
	_ = json.Unmarshal(body, &sent)
	err = p.readStream(ctx, response.Body, validators, func(event model.Event) error {
		if event.Type == "completed" {
			var output []json.RawMessage
			if json.Unmarshal(event.Continuation, &output) != nil {
				return &domain.ProviderError{Kind: "protocol", Message: "invalid completed response output", Uncertain: true}
			}
			history := append(sent.Input, output...)
			event.Continuation, _ = json.Marshal(history)
			if len(event.Continuation) > model.MaxRequestBytes {
				return &domain.ProviderError{Kind: "protocol", Message: "continuation exceeds limit", Uncertain: true}
			}
		}
		return emit(event)
	})
	if err != nil {
		var provider *domain.ProviderError
		if errors.As(err, &provider) {
			safe := *provider
			safe.Uncertain = true
			return &safe
		}
		return &domain.ProviderError{Kind: "interrupted", Message: "response consumer stopped", Uncertain: true}
	}
	return nil
}

func (p *Provider) EstimateUsage(request model.TurnRequest) model.Usage {
	bytesUsed := len(request.System)
	for _, message := range request.Messages {
		bytesUsed += len(message.Role) + len(message.Text) + len(message.ToolCallID)
		for _, call := range message.ToolCalls {
			bytesUsed += len(call.ID) + len(call.Name) + len(call.Arguments)
		}
		bytesUsed += len(message.Opaque)
	}
	for _, tool := range request.Tools {
		bytesUsed += len(tool.Name) + len(tool.Description) + len(tool.Schema)
	}
	bytesUsed += len(request.Continuation) + 512
	input := int64(bytesUsed + 256*len(request.Messages) + 256*len(request.Tools))
	return model.Usage{InputTokens: input, OutputTokens: int64(request.MaxOutputTokens), Known: false, Source: "estimate"}
}

func (p *Provider) turnBody(request model.TurnRequest, modelID string) ([]byte, error) {
	if request.MaxOutputTokens < 1 || request.MaxOutputTokens > 1<<20 {
		return nil, &domain.ProviderError{Kind: "invalid_request", Message: "an explicit bounded output token limit is required"}
	}
	input, err := buildInput(request)
	if err != nil {
		return nil, err
	}
	tools := make([]map[string]any, 0, len(request.Tools))
	for _, tool := range request.Tools {
		if !validName(tool.Name) || len(tool.Schema) == 0 || !json.Valid(tool.Schema) {
			return nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid tool definition"}
		}
		var schema map[string]any
		if json.Unmarshal(tool.Schema, &schema) != nil || schema == nil {
			return nil, &domain.ProviderError{Kind: "invalid_request", Message: "tool schema must be an object"}
		}
		tools = append(tools, map[string]any{"type": "function", "name": tool.Name, "description": tool.Description, "parameters": schema, "strict": true})
	}
	body := map[string]any{"model": modelID, "input": input, "stream": true, "store": false}
	if request.System != "" {
		body["instructions"] = request.System
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	if request.MaxOutputTokens > 0 {
		body["max_output_tokens"] = request.MaxOutputTokens
	}
	body["include"] = []string{"reasoning.encrypted_content"}
	raw, err := json.Marshal(body)
	if len(raw) > model.MaxRequestBytes {
		return nil, &domain.ProviderError{Kind: "invalid_request", Message: "request exceeds limit"}
	}
	return raw, err
}

func buildInput(request model.TurnRequest) ([]any, error) {
	input := make([]any, 0, len(request.Messages)+4)
	if len(request.Continuation) > 0 {
		var items []json.RawMessage
		if len(request.Continuation) > model.MaxRequestBytes || json.Unmarshal(request.Continuation, &items) != nil {
			return nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid continuation"}
		}
		for _, item := range items {
			input = append(input, item)
		}
	}
	for _, message := range request.Messages {
		if len(message.Opaque) > 0 {
			if !json.Valid(message.Opaque) {
				return nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid opaque continuation item"}
			}
			input = append(input, append(json.RawMessage(nil), message.Opaque...))
			continue
		}
		if message.ToolCallID != "" {
			input = append(input, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": message.Text})
			continue
		}
		if len(message.ToolCalls) > 0 {
			for _, call := range message.ToolCalls {
				if !validName(call.Name) || !json.Valid(call.Arguments) {
					return nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid tool call"}
				}
				input = append(input, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": string(call.Arguments)})
			}
			continue
		}
		if message.Role != "user" && message.Role != "assistant" {
			return nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid message role"}
		}
		if message.Text == "" {
			continue
		}
		input = append(input, map[string]any{"role": message.Role, "content": message.Text})
	}

	return input, nil
}

type toolState struct {
	callID  string
	itemID  string
	index   int
	name    string
	args    strings.Builder
	emitted bool
}

func (p *Provider) readStream(ctx context.Context, reader io.Reader, validators model.ToolSchemas, emit func(model.Event) error) error {
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
	states := make(map[string]*toolState)
	var responseID string
	var continuationItems []json.RawMessage
	completed := false
	scanner := bufio.NewScanner(io.LimitReader(reader, maxStreamBytes))
	scanner.Buffer(make([]byte, 4096), maxStreamEvent)
	var eventType string
	var data bytes.Buffer
	total := 0
	dispatch := func() error {
		if data.Len() == 0 {
			eventType = ""
			return nil
		}
		payload := append([]byte(nil), data.Bytes()...)
		data.Reset()
		typeName, err := processStreamEvent(ctx, eventType, payload, &responseID, states, validators, &continuationItems, emit)
		if err != nil {
			return err
		}
		if typeName == "completed" {
			completed = true
			return nil
		}
		eventType = ""
		return nil
	}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return canceledError(err)
		}
		line := scanner.Bytes()
		line = bytes.TrimSuffix(line, []byte("\r"))
		total += len(line)
		if total > maxStreamBytes {
			return &domain.ProviderError{Kind: "protocol", Message: "stream exceeded size limit"}
		}
		switch {
		case len(line) == 0:
			if err := dispatch(); err != nil {
				return err
			}
			if completed {
				return nil
			}
		case bytes.HasPrefix(line, []byte("event:")):
			eventType = strings.TrimSpace(string(line[len("event:"):]))
		case bytes.HasPrefix(line, []byte("data:")):
			part := bytes.TrimSpace(line[len("data:"):])
			if bytes.Equal(part, []byte("[DONE]")) {
				if err := dispatch(); err != nil {
					return err
				}
				if !completed {
					return &domain.ProviderError{Kind: "provider", Message: "OpenAI response ended before completion"}
				}
				return nil
			}
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			if data.Len()+len(part) > maxStreamEvent {
				return &domain.ProviderError{Kind: "protocol", Message: "stream event exceeds limit", Uncertain: true}
			}
			data.Write(part)
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return &domain.ProviderError{Kind: "protocol", Message: "stream event exceeded size limit"}
		}
		return classifyTransport(ctx, err, true)
	}
	if err := dispatch(); err != nil {
		return err
	}
	if !completed {
		return &domain.ProviderError{Kind: "provider", Message: "OpenAI response ended before completion"}
	}
	return nil
}

type streamEnvelope struct {
	Type        string          `json:"type"`
	ID          string          `json:"id"`
	ItemID      string          `json:"item_id"`
	OutputIndex int             `json:"output_index"`
	Delta       string          `json:"delta"`
	Arguments   string          `json:"arguments"`
	Response    json.RawMessage `json:"response"`
	Item        json.RawMessage `json:"item"`
	Usage       json.RawMessage `json:"usage"`
	Error       json.RawMessage `json:"error"`
}

func processStreamEvent(ctx context.Context, eventType string, payload []byte, responseID *string, states map[string]*toolState, validators model.ToolSchemas, continuationItems *[]json.RawMessage, emit func(model.Event) error) (string, error) {
	var event streamEnvelope
	if err := json.Unmarshal(payload, &event); err != nil {
		return "", &domain.ProviderError{Kind: "protocol", Message: "invalid stream event"}
	}
	if event.Type != "" {
		eventType = event.Type
	}
	if event.ID != "" && strings.HasPrefix(event.ID, "resp_") {
		*responseID = event.ID
	}
	if eventType == "response.output_item.done" && len(event.Item) > 0 {
		*continuationItems = append(*continuationItems, append(json.RawMessage(nil), event.Item...))
	}
	switch eventType {
	case "response.output_text.delta":
		if event.Delta == "" {
			return "", nil
		}
		if err := emit(model.Event{Type: "text_delta", ID: event.ItemID, Text: event.Delta}); err != nil {
			return "", normalizeCallbackError(ctx, err)
		}
	case "response.output_item.added":
		var item struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if json.Unmarshal(event.Item, &item) == nil && item.Type == "function_call" {
			state := stateFor(states, item.CallID, item.ID, event.OutputIndex)
			state.callID, state.itemID, state.name = item.CallID, item.ID, item.Name
			if item.Arguments != "" {
				state.args.WriteString(item.Arguments)
			}
		}
	case "response.function_call_arguments.delta":
		state := stateFor(states, "", event.ItemID, event.OutputIndex)
		state.args.WriteString(event.Delta)
	case "response.function_call_arguments.done":
		state := stateFor(states, "", event.ItemID, event.OutputIndex)
		if event.Arguments != "" {
			state.args.Reset()
			state.args.WriteString(event.Arguments)
		}
		if err := finalizeTool(ctx, state, validators, emit); err != nil {
			return "", err
		}
	case "response.output_item.done":
		var item struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if json.Unmarshal(event.Item, &item) == nil && item.Type == "function_call" {
			state := stateFor(states, item.CallID, item.ID, event.OutputIndex)
			state.callID, state.itemID, state.name = item.CallID, item.ID, item.Name
			if item.Arguments != "" {
				state.args.Reset()
				state.args.WriteString(item.Arguments)
			}
			if err := finalizeTool(ctx, state, validators, emit); err != nil {
				return "", err
			}
		}
	case "response.completed":
		var completed struct {
			ID    string `json:"id"`
			Usage *struct {
				InputTokens        *int64 `json:"input_tokens"`
				OutputTokens       *int64 `json:"output_tokens"`
				InputTokensDetails struct {
					CachedTokens int64 `json:"cached_tokens"`
				} `json:"input_tokens_details"`
			} `json:"usage"`
			Output json.RawMessage `json:"output"`
		}
		if json.Unmarshal(event.Response, &completed) != nil {
			json.Unmarshal(payload, &completed)
		}
		if completed.ID != "" {
			*responseID = completed.ID
		}
		usage := &model.Usage{Known: false, Source: "completed_response"}
		if completed.Usage != nil && completed.Usage.InputTokens != nil && completed.Usage.OutputTokens != nil {
			if *completed.Usage.InputTokens < 0 || *completed.Usage.OutputTokens < 0 || completed.Usage.InputTokensDetails.CachedTokens < 0 || completed.Usage.InputTokensDetails.CachedTokens > *completed.Usage.InputTokens {
				return "", &domain.ProviderError{Kind: "protocol", Message: "invalid usage", Uncertain: true}
			}
			usage.Known = true
			usage.InputTokens = *completed.Usage.InputTokens
			usage.OutputTokens = *completed.Usage.OutputTokens
			usage.CacheTokens = completed.Usage.InputTokensDetails.CachedTokens
		}
		for _, state := range states {
			if !state.emitted {
				return "", &domain.ProviderError{Kind: "protocol", Message: "response completed with partial tool call", Uncertain: true}
			}
		}
		continuation := completed.Output
		if len(continuation) == 0 {
			continuation, _ = json.Marshal(*continuationItems)
		}
		if err := emit(model.Event{Type: "completed", ID: *responseID, Usage: usage, Continuation: continuation}); err != nil {
			return "", normalizeCallbackError(ctx, err)
		}
		return "completed", nil
	case "response.failed", "response.incomplete", "error":
		return "", &domain.ProviderError{Kind: "provider", Message: "OpenAI response did not complete"}
	}
	return "", nil
}

func stateFor(states map[string]*toolState, callID, itemID string, index int) *toolState {
	key := callID
	if key == "" {
		key = itemID
	}
	if key == "" {
		key = strconv.Itoa(index)
	}
	if state := states[key]; state != nil {
		return state
	}
	for _, state := range states {
		if (callID != "" && state.callID == callID) || (itemID != "" && state.itemID == itemID) {
			return state
		}
	}
	state := &toolState{callID: callID, itemID: itemID, index: index}
	states[key] = state
	return state
}

func finalizeTool(ctx context.Context, state *toolState, validators model.ToolSchemas, emit func(model.Event) error) error {
	if state.emitted {
		return nil
	}
	if state.callID == "" || state.name == "" {
		return &domain.ProviderError{Kind: "protocol", Message: "incomplete tool call identity"}
	}
	arguments := []byte(state.args.String())
	if err := validators.Validate(model.ToolCall{ID: state.callID, Name: state.name, Arguments: arguments}); err != nil {
		return err
	}
	state.emitted = true
	if err := emit(model.Event{Type: "tool_call", ID: state.callID, ToolCall: &model.ToolCall{ID: state.callID, Name: state.name, Arguments: json.RawMessage(append([]byte(nil), arguments...))}}); err != nil {
		return normalizeCallbackError(ctx, err)
	}
	return nil
}

func (p *Provider) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	u := *p.base
	u.Path = strings.TrimRight(p.base.Path, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "invalid provider request"}
	}
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		if method == http.MethodPost {
			req.Header.Set("Accept", "text/event-stream")
		}
	}
	return p.config.Client.Do(req)
}

func checkResponse(response *http.Response) error {
	if response == nil {
		return &domain.ProviderError{Kind: "protocol", Message: "provider returned no response"}
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	retryAfter := time.Duration(0)
	if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 && seconds < 3600 {
		retryAfter = time.Duration(seconds) * time.Second
	}
	kind := "provider"
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		kind = "auth"
	case http.StatusNotFound:
		kind = "model_unsupported"
	case http.StatusTooManyRequests:
		kind = "quota"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		kind = "invalid_request"
	}
	return &domain.ProviderError{Kind: kind, Message: "OpenAI request failed", RetryAfter: retryAfter, Uncertain: response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests}
}

func decodeBounded(reader io.Reader, target any) error {
	body, err := io.ReadAll(io.LimitReader(reader, maxResponseBody+1))
	if err != nil || len(body) > maxResponseBody {
		return errors.New("bounded response read failed")
	}
	return json.Unmarshal(body, target)
}

func classifyTransport(ctx context.Context, err error, uncertain bool) error {
	if ctx.Err() != nil {
		e := canceledError(ctx.Err()).(*domain.ProviderError)
		e.Uncertain = uncertain
		return e
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		e := canceledError(err).(*domain.ProviderError)
		e.Uncertain = uncertain
		return e
	}
	return &domain.ProviderError{Kind: "transport", Message: "OpenAI request could not be completed", Uncertain: uncertain}
}

func canceledError(err error) error {
	message := "OpenAI request canceled"
	if errors.Is(err, context.DeadlineExceeded) {
		message = "OpenAI request deadline exceeded"
	}
	return &domain.ProviderError{Kind: "canceled", Message: message}
}

func normalizeCallbackError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if ctx.Err() != nil {
			return canceledError(ctx.Err())
		}
		return canceledError(err)
	}
	return err
}

func capability(state domain.CapabilityState, reason, source string) domain.Capability {
	return domain.Capability{State: state, Scope: "configured model", Reason: reason, Source: source, LastChecked: time.Now().UTC()}
}

func validName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
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
