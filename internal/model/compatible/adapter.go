package compatible

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"reforge/internal/domain"
	"reforge/internal/model"
	openaimodel "reforge/internal/model/openai"
)

const (
	defaultProfile  = "chat_completions"
	maxResponseBody = 8 << 20
	maxStreamBytes  = 16 << 20
	maxStreamLine   = 1 << 20
)

type Provider struct {
	config       model.Config
	client       model.HTTPClient
	base         *url.URL
	delegate     model.ModelProvider
	gateway      bool
	mu           sync.RWMutex
	contextLimit int
	outputLimit  int
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
	if requireModel && strings.TrimSpace(config.Model) == "" {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "a model is required"}
	}
	profile := config.Profile
	if profile == "" {
		profile = defaultProfile
	}
	if profile != defaultProfile && profile != "ollama" && profile != "vllm" && profile != "responses" && !IsOpenCode(profile) {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "unsupported compatible profile"}
	}
	base, err := url.Parse(config.Endpoint)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.Opaque != "" || base.RawPath != "" {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "endpoint must be a fixed HTTP origin"}
	}
	base.Path = strings.TrimRight(base.Path, "/")
	if !strings.HasSuffix(base.Path, "/v1") {
		base.Path += "/v1"
	}
	injected := config.Client
	if httpClient, ok := config.Client.(*http.Client); ok {
		clone := *httpClient
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		injected = &clone
	}
	config.Profile = profile
	if IsOpenCode(profile) {
		return openCode(config, &Provider{config: config, base: base, client: injected, gateway: true}, requireModel)
	}
	if profile == "responses" && requireModel {
		delegateConfig := config
		delegateConfig.Profile = "responses"
		delegateConfig.Endpoint = strings.TrimSuffix(strings.TrimRight(delegateConfig.Endpoint, "/"), "/v1")
		delegate, err := openaimodel.New(delegateConfig)
		if err != nil {
			return nil, err
		}
		return &Provider{config: config, delegate: delegate, base: base, client: injected}, nil
	}
	return &Provider{config: config, base: base, client: injected}, nil
}

func (p *Provider) Probe(ctx context.Context) (model.Capabilities, error) {
	if p.gateway {
		return p.gatewayProbe(ctx)
	}
	if p.delegate != nil {
		caps, err := p.delegate.Probe(ctx)
		caps.Provider, caps.BillingRoute = "compatible", "customer_endpoint"
		return caps, err
	}
	response, err := p.do(ctx, http.MethodGet, "/models", nil, "")
	if err != nil {
		return model.Capabilities{}, classifyTransport(ctx, err, false)
	}
	if response == nil || response.Body == nil {
		return model.Capabilities{}, &domain.ProviderError{Kind: "protocol", Message: "provider returned no response body"}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return p.unknownCapabilities("model listing is unavailable; configured model requires explicit endpoint qualification"), nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return model.Capabilities{}, classifyHTTP(response, false)
	}
	var payload modelListResponse
	if err := decodeBounded(response.Body, &payload); err != nil || !validModelList(payload.Data) {
		return p.unknownCapabilities("model listing response is not understood"), nil
	}
	entry := findModel(payload.Data, p.config.Model)
	if entry == nil && payload.Data != nil {
		return model.Capabilities{}, &domain.ProviderError{Kind: "model_unsupported", Message: "configured model was not returned by compatible endpoint"}
	}
	if entry != nil {
		p.recordLimits(entry)
	}
	features := map[string]domain.Capability{
		"chat_completions":    capability(domain.Unknown, "model listing does not qualify the configured Chat Completions protocol", "model metadata"),
		"streaming":           capability(domain.Unknown, "streaming requires endpoint qualification", "model metadata"),
		"tool_calling":        capability(domain.Unknown, "tool calling requires endpoint qualification", "model metadata"),
		"parallel_tool_calls": capability(domain.Unknown, "parallel tools require endpoint qualification", "model metadata"),
		"usage":               capability(domain.Unknown, "reported only by a completed stream", "model metadata"),
	}
	return model.Capabilities{Provider: "compatible", Model: p.config.Model, BillingRoute: "customer_endpoint", Features: features}, nil
}

func (p *Provider) unknownCapabilities(reason string) model.Capabilities {
	features := map[string]domain.Capability{}
	for _, name := range []string{"chat_completions", "streaming", "tool_calling", "parallel_tool_calls", "usage"} {
		features[name] = capability(domain.Unknown, reason, "endpoint metadata")
	}
	return model.Capabilities{Provider: "compatible", Model: p.config.Model, BillingRoute: "customer_endpoint", Features: features}
}

func (p *Provider) ListModels(ctx context.Context) ([]model.Model, error) {
	if p.delegate != nil && !p.gateway {
		return p.delegate.ListModels(ctx)
	}
	response, err := p.do(ctx, http.MethodGet, "/models", nil, "")
	if err != nil {
		return nil, classifyTransport(ctx, err, false)
	}
	if response == nil || response.Body == nil {
		return nil, &domain.ProviderError{Kind: "protocol", Message: "provider returned no response body"}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, &domain.ProviderError{Kind: "capability_unknown", Message: "model listing is unavailable; configure and qualify the pinned model explicitly"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, classifyHTTP(response, false)
	}
	var payload modelListResponse
	if err := decodeBounded(response.Body, &payload); err != nil || !validModelList(payload.Data) {
		return nil, &domain.ProviderError{Kind: "capability_unknown", Message: "model listing response is not understood"}
	}
	models := make([]model.Model, 0, len(payload.Data))
	for _, entry := range payload.Data {
		if entry.ID == "" {
			continue
		}
		p.recordLimits(&entry)
		models = append(models, model.Model{ID: entry.ID, ContextLimit: entry.contextLimit(), OutputLimit: entry.outputLimit()})
	}
	return models, nil
}

func validModelList(entries []modelEntry) bool {
	if entries == nil || len(entries) > 10000 {
		return false
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" || len(entry.ID) > 512 || seen[entry.ID] {
			return false
		}
		seen[entry.ID] = true
	}
	return true
}

func (p *Provider) StreamTurn(ctx context.Context, request model.TurnRequest, emit func(model.Event) error) error {
	if p.delegate != nil {
		return p.delegate.StreamTurn(ctx, request, emit)
	}
	if emit == nil {
		return &domain.ProviderError{Kind: "configuration", Message: "an event callback is required"}
	}
	if err := ctx.Err(); err != nil {
		return canceledError(err, false)
	}
	modelID := request.Model
	if modelID == "" {
		modelID = p.config.Model
	}
	if modelID != p.config.Model {
		return &domain.ProviderError{Kind: "model_unsupported", Message: "requested model is not the configured model"}
	}
	if request.MaxOutputTokens < 1 || request.MaxOutputTokens > 1<<20 {
		return &domain.ProviderError{Kind: "invalid_request", Message: "an explicit bounded output token limit is required"}
	}
	p.mu.RLock()
	outputLimit, contextLimit := p.outputLimit, p.contextLimit
	p.mu.RUnlock()
	if outputLimit > 0 && request.MaxOutputTokens > outputLimit {
		return &domain.ProviderError{Kind: "invalid_request", Message: "requested output exceeds the qualified model limit"}
	}
	schemas, err := model.CompileTools(request.Tools)
	if err != nil {
		return err
	}
	body, history, err := p.requestBody(request, modelID)
	if err != nil {
		return err
	}
	if contextLimit > 0 && estimateInputTokens(body)+request.MaxOutputTokens > contextLimit {
		return &domain.ProviderError{Kind: "invalid_request", Message: "request exceeds the qualified model context limit"}
	}
	response, err := p.do(ctx, http.MethodPost, "/chat/completions", body, request.Session)
	if err != nil {
		return classifyTransport(ctx, err, true)
	}
	if response == nil || response.Body == nil {
		return &domain.ProviderError{Kind: "protocol", Message: "provider returned no response body", Uncertain: true}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return classifyHTTP(response, false)
	}
	state := &streamState{schemas: schemas}
	if err := p.readStream(ctx, response.Body, state, emit); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return canceledError(err, true)
	}
	if state.completed && state.finishReason == "length" && state.usageValue().Known {
		return emit(model.Event{Type: "completed", ID: state.id, FinishReason: "length", Usage: state.usageValue()})
	}
	if !state.completed || (state.finishReason != "stop" && state.finishReason != "tool_calls") || (state.finishReason == "tool_calls" && len(state.calls) == 0) {
		return &domain.ProviderError{Kind: "protocol", Message: "compatible response ended before completion", Uncertain: true}
	}
	if err := state.validateTools(); err != nil {
		return err
	}
	continuation, err := json.Marshal(append(history, state.assistantMessage()))
	if err != nil || len(continuation) > model.MaxRequestBytes {
		return &domain.ProviderError{Kind: "protocol", Message: "compatible continuation exceeds limit", Uncertain: true}
	}
	for _, call := range state.calls {
		if err := ctx.Err(); err != nil {
			return canceledError(err, true)
		}
		if err := emit(model.Event{Type: "tool_call", ID: call.ID, ToolCall: &model.ToolCall{ID: call.ID, Name: call.Name, Arguments: json.RawMessage([]byte(call.Arguments.String()))}}); err != nil {
			return normalizeCallback(ctx, err)
		}
	}
	if err := emit(model.Event{Type: "completed", ID: state.id, FinishReason: state.finishReason, Usage: state.usageValue(), Continuation: continuation}); err != nil {
		return normalizeCallback(ctx, err)
	}
	return nil
}

func (p *Provider) EstimateUsage(request model.TurnRequest) model.Usage {
	bytesUsed := len(request.System) + len(request.Continuation)
	for _, message := range request.Messages {
		bytesUsed += len(message.Role) + len(message.Text) + len(message.ToolCallID) + len(message.Opaque)
		for _, call := range message.ToolCalls {
			bytesUsed += len(call.ID) + len(call.Name) + len(call.Arguments)
		}
	}
	for _, tool := range request.Tools {
		bytesUsed += len(tool.Name) + len(tool.Description) + len(tool.Schema)
	}
	return model.Usage{InputTokens: int64(bytesUsed + 256*(len(request.Messages)+len(request.Tools)) + 512), OutputTokens: int64(request.MaxOutputTokens), Known: false, Source: "estimate"}
}

func (p *Provider) requestBody(request model.TurnRequest, modelID string) ([]byte, []json.RawMessage, error) {
	messages, history, err := buildMessages(request)
	if err != nil {
		return nil, nil, err
	}
	tools := make([]map[string]any, 0, len(request.Tools))
	for _, tool := range request.Tools {
		if !json.Valid(tool.Schema) {
			return nil, nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid tool schema"}
		}
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": json.RawMessage(tool.Schema)}})
	}
	bodyValue := map[string]any{"model": modelID, "messages": messages, "stream": true, "stream_options": map[string]any{"include_usage": true}, "max_tokens": request.MaxOutputTokens}
	if len(tools) > 0 {
		bodyValue["tools"] = tools
		bodyValue["parallel_tool_calls"] = true
		if p.config.Profile == "ollama" {
			bodyValue["reasoning_effort"] = "none"
			bodyValue["temperature"] = 0
		}
	}
	body, err := json.Marshal(bodyValue)
	if err != nil {
		return nil, nil, &domain.ProviderError{Kind: "invalid_request", Message: "could not encode compatible request"}
	}
	if len(body) > model.MaxRequestBytes {
		return nil, nil, &domain.ProviderError{Kind: "invalid_request", Message: "request exceeds limit"}
	}
	return body, history, nil
}

func buildMessages(request model.TurnRequest) ([]json.RawMessage, []json.RawMessage, error) {
	history := []json.RawMessage{}
	if len(request.Continuation) > 0 {
		if len(request.Continuation) > model.MaxRequestBytes || json.Unmarshal(request.Continuation, &history) != nil || history == nil {
			return nil, nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid compatible continuation"}
		}
	}
	validate := func(raw json.RawMessage) error {
		var value struct {
			Role string `json:"role"`
		}
		if json.Unmarshal(raw, &value) != nil || (value.Role != "user" && value.Role != "assistant" && value.Role != "tool") {
			return &domain.ProviderError{Kind: "invalid_request", Message: "invalid compatible history role"}
		}
		return nil
	}
	for _, raw := range history {
		if err := validate(raw); err != nil {
			return nil, nil, err
		}
	}
	for _, message := range request.Messages {
		if len(message.Opaque) > 0 {
			if err := validate(message.Opaque); err != nil {
				return nil, nil, err
			}
			history = append(history, append(json.RawMessage(nil), message.Opaque...))
			continue
		}
		if message.Role != "user" && message.Role != "assistant" && message.Role != "tool" {
			return nil, nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid message role"}
		}
		value := map[string]any{"role": message.Role, "content": message.Text}
		if message.Role == "tool" {
			if message.ToolCallID == "" {
				return nil, nil, &domain.ProviderError{Kind: "invalid_request", Message: "tool result requires call identity"}
			}
			value["tool_call_id"] = message.ToolCallID
		} else if message.ToolCallID != "" {
			return nil, nil, &domain.ProviderError{Kind: "invalid_request", Message: "unexpected call identity"}
		}
		if len(message.ToolCalls) > 0 {
			if message.Role != "assistant" {
				return nil, nil, &domain.ProviderError{Kind: "invalid_request", Message: "tool call must be an assistant message"}
			}
			calls := []map[string]any{}
			for _, call := range message.ToolCalls {
				if call.ID == "" || call.Name == "" || !json.Valid(call.Arguments) {
					return nil, nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid compatible tool call"}
				}
				calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": string(call.Arguments)}})
			}
			value["tool_calls"] = calls
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, nil, err
		}
		history = append(history, raw)
	}
	if len(history) == 0 {
		return nil, nil, &domain.ProviderError{Kind: "invalid_request", Message: "messages are required"}
	}
	messages := make([]json.RawMessage, 0, len(history)+1)
	if request.System != "" {
		raw, _ := json.Marshal(map[string]string{"role": "system", "content": request.System})
		messages = append(messages, raw)
	}
	messages = append(messages, history...)
	return messages, history, nil
}

type callState struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

type streamState struct {
	schemas      model.ToolSchemas
	id           string
	finishReason string
	completed    bool
	usage        *chatUsage
	text         strings.Builder
	calls        []*callState
}

func (p *Provider) readStream(ctx context.Context, reader io.Reader, state *streamState, emit func(model.Event) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxStreamLine)
	total := 0
	done := false
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return canceledError(err, true)
		}
		line := scanner.Bytes()
		total += len(line) + 1
		if total > maxStreamBytes {
			return &domain.ProviderError{Kind: "protocol", Message: "compatible stream exceeded size limit", Uncertain: true}
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if bytes.Equal(payload, []byte("[DONE]")) {
			done = true
			break
		}
		var chunk chatChunk
		if json.Unmarshal(payload, &chunk) != nil {
			return &domain.ProviderError{Kind: "protocol", Message: "invalid compatible stream event", Uncertain: true}
		}
		if err := state.accept(chunk); err != nil {
			return err
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				state.text.WriteString(choice.Delta.Content)
			}
			if choice.Delta.Content != "" {
				if err := emit(model.Event{Type: "text_delta", ID: chunk.ID, Text: choice.Delta.Content}); err != nil {
					return normalizeCallback(ctx, err)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return &domain.ProviderError{Kind: "protocol", Message: "compatible stream event exceeded limit", Uncertain: true}
		}
		return classifyTransport(ctx, err, true)
	}
	if !done {
		return &domain.ProviderError{Kind: "protocol", Message: "compatible stream ended before done marker", Uncertain: true}
	}
	state.completed = true
	return nil
}

func (s *streamState) accept(chunk chatChunk) error {
	invalid := func() error {
		return &domain.ProviderError{Kind: "protocol", Message: "invalid compatible stream identity or tool fragments", Uncertain: true}
	}
	if chunk.ID != "" {
		if s.id != "" && s.id != chunk.ID {
			return invalid()
		}
		s.id = chunk.ID
	}
	if chunk.Usage != nil {
		s.usage = chunk.Usage
	}
	if len(chunk.Choices) > 1 {
		return invalid()
	}
	for _, choice := range chunk.Choices {
		if choice.Index != 0 || s.finishReason != "" {
			return invalid()
		}
		for _, delta := range choice.Delta.ToolCalls {
			if delta.Index < 0 || delta.Index >= 64 || (delta.Type != "" && delta.Type != "function") {
				return invalid()
			}
			for len(s.calls) <= delta.Index {
				s.calls = append(s.calls, &callState{})
			}
			call := s.calls[delta.Index]
			if delta.ID != "" {
				if call.ID != "" && call.ID != delta.ID {
					return invalid()
				}
				call.ID = delta.ID
			}
			if delta.Function.Name != "" {
				if call.Name != "" && call.Name != delta.Function.Name {
					return invalid()
				}
				call.Name = delta.Function.Name
			}
			if call.Arguments.Len()+len(delta.Function.Arguments) > 256<<10 {
				return invalid()
			}
			call.Arguments.WriteString(delta.Function.Arguments)
		}
		if choice.FinishReason != "" {
			s.finishReason = choice.FinishReason
		}
	}
	return nil
}

func (s *streamState) validateTools() error {
	seen := map[string]bool{}
	for _, call := range s.calls {
		if seen[call.ID] {
			return &domain.ProviderError{Kind: "protocol", Message: "duplicate tool call identity", Uncertain: true}
		}
		seen[call.ID] = true
		if call.ID == "" || call.Name == "" || call.Arguments.Len() == 0 {
			return &domain.ProviderError{Kind: "protocol", Message: "incomplete compatible tool call", Uncertain: true}
		}
		if err := s.schemas.Validate(model.ToolCall{ID: call.ID, Name: call.Name, Arguments: json.RawMessage(call.Arguments.String())}); err != nil {
			return err
		}
	}
	return nil
}

func (s *streamState) assistantMessage() json.RawMessage {
	message := map[string]any{"role": "assistant", "content": s.text.String()}
	if len(s.calls) > 0 {
		calls := make([]map[string]any, 0, len(s.calls))
		for _, call := range s.calls {
			calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": call.Arguments.String()}})
		}
		message["tool_calls"] = calls
	}
	raw, _ := json.Marshal(message)
	return raw
}

func (s *streamState) usageValue() *model.Usage {
	if s.usage == nil || s.usage.PromptTokens == nil || s.usage.CompletionTokens == nil || *s.usage.PromptTokens < 0 || *s.usage.CompletionTokens < 0 || *s.usage.PromptTokens > 1<<40 || *s.usage.CompletionTokens > 1<<40 {
		return &model.Usage{Known: false, Source: "completed_response"}
	}
	cache := int64(0)
	if s.usage.PromptTokensDetails != nil && s.usage.PromptTokensDetails.CachedTokens != nil {
		cache = *s.usage.PromptTokensDetails.CachedTokens
	}
	if cache < 0 || cache > *s.usage.PromptTokens || s.usage.TotalTokens != nil && *s.usage.TotalTokens != *s.usage.PromptTokens+*s.usage.CompletionTokens {
		return &model.Usage{Known: false, Source: "completed_response"}
	}
	return &model.Usage{InputTokens: *s.usage.PromptTokens, OutputTokens: *s.usage.CompletionTokens, CacheTokens: cache, Known: true, Source: "completed_response"}
}

type chatChunk struct {
	ID      string       `json:"id"`
	Choices []chatChoice `json:"choices"`
	Usage   *chatUsage   `json:"usage"`
}

type chatChoice struct {
	Index        int       `json:"index"`
	Delta        chatDelta `json:"delta"`
	FinishReason string    `json:"finish_reason"`
}

type chatDelta struct {
	Content   string              `json:"content"`
	ToolCalls []chatToolCallDelta `json:"tool_calls"`
}

type chatToolCallDelta struct {
	Type     string `json:"type"`
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatUsage struct {
	PromptTokens        *int64 `json:"prompt_tokens"`
	CompletionTokens    *int64 `json:"completion_tokens"`
	TotalTokens         *int64 `json:"total_tokens"`
	PromptTokensDetails *struct {
		CachedTokens *int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type modelListResponse struct {
	Data []modelEntry `json:"data"`
}

type modelEntry struct {
	ID            string `json:"id"`
	OwnedBy       string `json:"owned_by"`
	ContextLength int    `json:"context_length"`
	ContextWindow int    `json:"context_window"`
	MaxModelLen   int    `json:"max_model_len"`
	MaxOutput     int    `json:"max_output_tokens"`
	MaxTokens     int    `json:"max_tokens"`
}

func (entry modelEntry) contextLimit() int {
	for _, value := range []int{entry.ContextLength, entry.ContextWindow, entry.MaxModelLen} {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (entry modelEntry) outputLimit() int {
	for _, value := range []int{entry.MaxOutput, entry.MaxTokens} {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (p *Provider) do(ctx context.Context, method, path string, body []byte, session string) (*http.Response, error) {
	u := *p.base
	u.Path = strings.TrimRight(p.base.Path, "/") + path
	request, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "invalid compatible request"}
	}
	if p.config.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "reforge/1.0")
	if session != "" && IsOpenCode(p.config.Profile) {
		request.Header.Set("x-opencode-session", session)
	}
	if method == http.MethodPost {
		request.Header.Set("Accept", "text/event-stream")
		request.Header.Set("Content-Type", "application/json")
	}
	return p.client.Do(request)
}

func (p *Provider) recordLimits(entry *modelEntry) {
	if entry.ID != p.config.Model {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry.contextLimit() > 0 {
		p.contextLimit = entry.contextLimit()
	}
	if entry.outputLimit() > 0 {
		p.outputLimit = entry.outputLimit()
	}
}

func findModel(entries []modelEntry, id string) *modelEntry {
	for index := range entries {
		if entries[index].ID == id {
			return &entries[index]
		}
	}
	return nil
}

func decodeBounded(reader io.Reader, target any) error {
	body, err := io.ReadAll(io.LimitReader(reader, maxResponseBody+1))
	if err != nil || len(body) > maxResponseBody {
		return errors.New("bounded response read failed")
	}
	return json.Unmarshal(body, target)
}

func estimateInputTokens(body []byte) int { return len(body) + 512 }

func classifyHTTP(response *http.Response, uncertain bool) error {
	kind := "provider"
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		kind = "auth"
	case http.StatusNotFound:
		kind = "model_unsupported"
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		kind = "quota"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		kind = "invalid_request"
	}
	message := "compatible endpoint request failed"
	if response.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		var body struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		detail := string(raw)
		if json.Unmarshal(raw, &body) == nil && body.Error.Message != "" {
			detail = body.Error.Message
		}
		if detail = strings.TrimSpace(detail); detail != "" {
			message += ": " + strings.Map(func(r rune) rune {
				if unicode.IsPrint(r) {
					return r
				}
				return -1
			}, detail[:min(len(detail), 300)])
		}
	}
	return &domain.ProviderError{Kind: kind, Message: message, Uncertain: uncertain || response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests}
}

func classifyTransport(ctx context.Context, err error, uncertain bool) error {
	if ctx.Err() != nil {
		return canceledError(ctx.Err(), uncertain)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return canceledError(err, uncertain)
	}
	return &domain.ProviderError{Kind: "transport", Message: "compatible endpoint could not be reached", Uncertain: uncertain}
}

func canceledError(err error, uncertain bool) error {
	message := "compatible request canceled"
	if errors.Is(err, context.DeadlineExceeded) {
		message = "compatible request deadline exceeded"
	}
	return &domain.ProviderError{Kind: "canceled", Message: message, Uncertain: uncertain}
}

func normalizeCallback(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return canceledError(ctx.Err(), true)
	}
	return &domain.ProviderError{Kind: "consumer", Message: "stream consumer stopped after dispatch", Uncertain: true}
}

func capability(state domain.CapabilityState, reason, source string) domain.Capability {
	return domain.Capability{State: state, Scope: "configured model", Reason: reason, Source: source, LastChecked: time.Now().UTC()}
}

var _ model.ModelProvider = (*Provider)(nil)
