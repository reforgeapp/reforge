package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/model"
	"google.golang.org/genai"
)

const (
	GatewayV1Profile = "gateway_v1"
	defaultProfile   = "gemini"
	maxResponseBody  = 8 << 20
)

type Provider struct {
	config model.Config
	client *genai.Client
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
		return nil, &domain.ProviderError{Kind: "configuration", Message: "a Gemini API key is required"}
	}
	if requireModel && strings.TrimSpace(config.Model) == "" {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "a model is required"}
	}
	profile := config.Profile
	if profile == "" {
		profile = defaultProfile
	}
	if profile != defaultProfile && profile != "gemini_api" && profile != GatewayV1Profile {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "unsupported Google profile; Vertex credentials require project and location fields"}
	}
	base, err := url.Parse(config.Endpoint)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.Opaque != "" || base.RawPath != "" {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "endpoint must be a fixed HTTP origin"}
	}
	base.Path = strings.TrimRight(base.Path, "/")
	for _, suffix := range []string{"/v1beta", "/v1"} {
		if strings.HasSuffix(base.Path, suffix) {
			base.Path = strings.TrimSuffix(base.Path, suffix)
		}
	}
	injected := config.Client
	if httpClient, ok := config.Client.(*http.Client); ok {
		clone := *httpClient
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		injected = &clone
	}
	transport := &clientTransport{client: injected}
	httpClient := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	apiVersion := "v1beta"
	if profile == GatewayV1Profile {
		apiVersion = "v1"
	}
	options := genai.HTTPOptions{
		BaseURL:      strings.TrimRight(base.String(), "/") + "/",
		APIVersion:   apiVersion,
		RetryOptions: &genai.HTTPRetryOptions{Attempts: genai.Ptr(int32(1))},
	}
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      config.APIKey,
		Backend:     genai.BackendGeminiAPI,
		HTTPClient:  httpClient,
		HTTPOptions: options,
	})
	if err != nil {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "could not initialize Google client"}
	}
	config.Profile = profile
	return &Provider{config: config, client: client, base: base}, nil
}

func (p *Provider) Probe(ctx context.Context) (model.Capabilities, error) {
	models, err := p.list(ctx)
	if err != nil {
		return model.Capabilities{}, err
	}
	entry := findModel(models, p.config.Model)
	if entry == nil {
		return model.Capabilities{}, &domain.ProviderError{Kind: "model_unsupported", Message: "configured model was not returned by Google"}
	}
	features := map[string]domain.Capability{
		"generate_content":   capability(domain.Unknown, "model metadata does not advertise generateContent", "model metadata"),
		"streaming":          capability(domain.Unknown, "model metadata does not verify streaming", "model metadata"),
		"tool_calling":       capability(domain.Unknown, "model metadata does not verify function calling", "model metadata"),
		"thought_signatures": capability(domain.Unknown, "model metadata does not verify thought signatures", "model metadata"),
		"usage":              capability(domain.Unknown, "reported only by a completed response", "model metadata"),
	}
	for _, action := range entry.SupportedActions {
		if action == "generateContent" {
			features["generate_content"] = capability(domain.Supported, "model metadata advertises generateContent", "model metadata")
		}
	}
	return model.Capabilities{Provider: "google", Model: modelID(entry.Name), BillingRoute: "google.gemini_api", Features: features}, nil
}

func (p *Provider) ListModels(ctx context.Context) ([]model.Model, error) {
	entries, err := p.list(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]model.Model, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Name == "" {
			continue
		}
		result = append(result, model.Model{ID: modelID(entry.Name), Name: entry.DisplayName, ContextLimit: int(entry.InputTokenLimit), OutputLimit: int(entry.OutputTokenLimit)})
	}
	return result, nil
}

func (p *Provider) list(ctx context.Context) ([]*genai.Model, error) {
	page, err := p.client.Models.List(ctx, &genai.ListModelsConfig{PageSize: 100, HTTPOptions: noRetryOptions()})
	if err != nil {
		return nil, classifyError(ctx, err, false)
	}
	result := make([]*genai.Model, 0, len(page.Items))
	seen := make(map[string]struct{})
	for pages := 0; ; pages++ {
		result = append(result, page.Items...)
		if page.NextPageToken == "" {
			return result, nil
		}
		if pages >= 100 {
			return nil, &domain.ProviderError{Kind: "protocol", Message: "Google model pagination exceeded limit"}
		}
		if _, ok := seen[page.NextPageToken]; ok {
			return nil, &domain.ProviderError{Kind: "protocol", Message: "Google model pagination repeated a cursor"}
		}
		seen[page.NextPageToken] = struct{}{}
		page, err = page.Next(ctx)
		if err != nil {
			return nil, classifyError(ctx, err, false)
		}
	}
}

func (p *Provider) StreamTurn(ctx context.Context, request model.TurnRequest, emit func(model.Event) error) error {
	if emit == nil {
		return &domain.ProviderError{Kind: "configuration", Message: "an event callback is required"}
	}
	if err := ctx.Err(); err != nil {
		return canceledError(err, false)
	}
	modelIDValue := request.Model
	if modelIDValue == "" {
		modelIDValue = p.config.Model
	}
	if modelIDValue != p.config.Model {
		return &domain.ProviderError{Kind: "model_unsupported", Message: "requested model is not the configured model"}
	}
	if request.MaxOutputTokens < 1 || request.MaxOutputTokens > 1<<20 {
		return &domain.ProviderError{Kind: "invalid_request", Message: "an explicit bounded output token limit is required"}
	}
	schemas, err := model.CompileTools(request.Tools)
	if err != nil {
		return err
	}
	contents, config, err := buildRequest(request)
	if err != nil {
		return err
	}
	if size, _ := json.Marshal(struct {
		Contents []json.RawMessage
		Config   *genai.GenerateContentConfig
		Tools    []model.Tool
	}{contents, config, request.Tools}); len(size) > model.MaxRequestBytes {
		return &domain.ProviderError{Kind: "invalid_request", Message: "request exceeds limit"}
	}
	capture := &bytes.Buffer{}
	ctx = context.WithValue(ctx, captureKey{}, capture)
	stream := p.client.Models.GenerateContentStream(ctx, modelIDValue, nil, config)
	for response, streamErr := range stream {
		if streamErr != nil {
			return classifyError(ctx, streamErr, true)
		}
		if response == nil {
			continue
		}
		for _, candidate := range response.Candidates {
			if candidate == nil || candidate.Content == nil {
				continue
			}
			for _, part := range candidate.Content.Parts {
				if part == nil || part.Thought || part.Text == "" {
					continue
				}
				if err := emit(model.Event{Type: "text_delta", ID: response.ResponseID, Text: part.Text}); err != nil {
					return normalizeCallback(ctx, err)
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return canceledError(err, true)
	}
	state, err := parseStream(capture.Bytes(), schemas, len(contents))
	if err != nil {
		return err
	}
	content, err := state.content()
	if err != nil {
		return err
	}
	history := append(append([]json.RawMessage(nil), contents...), content)
	continuation, err := json.Marshal(history)
	if err != nil || len(continuation) > model.MaxRequestBytes {
		return protocolError("Google continuation exceeds limit")
	}
	for _, call := range state.calls {
		if err := ctx.Err(); err != nil {
			return canceledError(err, true)
		}
		if err := emit(model.Event{Type: "tool_call", ID: call.ID, ToolCall: &model.ToolCall{ID: call.ID, Name: call.Name, Arguments: append(json.RawMessage(nil), call.Arguments...)}}); err != nil {
			return normalizeCallback(ctx, err)
		}
	}
	usage := state.usageValue()
	if err := emit(model.Event{Type: "completed", ID: state.responseID, FinishReason: state.finish, Usage: usage, Continuation: continuation}); err != nil {
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

type clientTransport struct{ client model.HTTPClient }

func (t *clientTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.client.Do(request)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	body := &boundedBody{reader: response.Body, closer: response.Body, remaining: maxResponseBody}
	if capture, ok := request.Context().Value(captureKey{}).(*bytes.Buffer); ok && response.StatusCode == http.StatusOK {
		body.capture = capture
	}
	response.Body = body
	return response, nil
}

type boundedBody struct {
	reader    io.Reader
	closer    io.Closer
	remaining int64
	capture   *bytes.Buffer
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, errors.New("Google response exceeds limit")
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.reader.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return 0, errors.New("Google response exceeds limit")
	}
	if b.capture != nil && n > 0 {
		b.capture.Write(p[:n])
	}
	return n, err
}
func (b *boundedBody) Close() error { return b.closer.Close() }

func noRetryOptions() *genai.HTTPOptions {
	return &genai.HTTPOptions{RetryOptions: &genai.HTTPRetryOptions{Attempts: genai.Ptr(int32(1))}}
}

func findModel(models []*genai.Model, configured string) *genai.Model {
	for _, entry := range models {
		if entry != nil && modelID(entry.Name) == modelID(configured) {
			return entry
		}
	}
	return nil
}

func modelID(name string) string { return strings.TrimPrefix(name, "models/") }

func classifyError(ctx context.Context, err error, uncertain bool) error {
	if ctx.Err() != nil {
		return canceledError(ctx.Err(), uncertain)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return canceledError(err, uncertain)
	}
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		kind := "provider"
		switch apiErr.Code {
		case http.StatusUnauthorized, http.StatusForbidden:
			kind = "auth"
		case http.StatusNotFound:
			kind = "model_unsupported"
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			kind = "quota"
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			kind = "invalid_request"
		}
		return &domain.ProviderError{Kind: kind, Message: "Google request failed", Uncertain: uncertain || apiErr.Code >= 500 || apiErr.Code == http.StatusTooManyRequests}
	}
	return &domain.ProviderError{Kind: "transport", Message: "Google request could not be completed", Uncertain: uncertain}
}

func canceledError(err error, uncertain bool) error {
	message := "Google request canceled"
	if errors.Is(err, context.DeadlineExceeded) {
		message = "Google request deadline exceeded"
	}
	return &domain.ProviderError{Kind: "canceled", Message: message, Uncertain: uncertain}
}

func normalizeCallback(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if ctx.Err() != nil {
			return canceledError(ctx.Err(), true)
		}
		return canceledError(err, true)
	}
	return &domain.ProviderError{Kind: "callback", Message: "Google event delivery failed", Uncertain: true}
}

func capability(state domain.CapabilityState, reason, source string) domain.Capability {
	return domain.Capability{State: state, Scope: "configured model", Reason: reason, Source: source, LastChecked: time.Now().UTC()}
}

var _ model.ModelProvider = (*Provider)(nil)
