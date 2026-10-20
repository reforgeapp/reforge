package google

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
	"google.golang.org/genai"
)

type captureKey struct{}
type nativeContent struct {
	Role  string            `json:"role"`
	Parts []json.RawMessage `json:"parts"`
}
type nativePart struct {
	FunctionCall *nativeCall `json:"functionCall"`
	Text         string      `json:"text"`
}
type nativeCall struct {
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name"`
	Args         json.RawMessage `json:"args,omitempty"`
	PartialArgs  json.RawMessage `json:"partialArgs"`
	WillContinue json.RawMessage `json:"willContinue"`
}
type callReference struct{ ID, Name string }
type callState struct {
	ID, Name  string
	Arguments json.RawMessage
}
type streamState struct {
	responseID, finish string
	fields             map[string]json.RawMessage
	parts              []json.RawMessage
	calls              []*callState
	usage              map[string]json.RawMessage
}

func protocolError(message string) error {
	return &domain.ProviderError{Kind: "protocol", Message: message, Uncertain: true}
}
func invalidRequest(message string) error {
	return &domain.ProviderError{Kind: "invalid_request", Message: message}
}
func decodeNumber(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return fmt.Errorf("extra JSON data")
	}
	return nil
}
func readContent(raw json.RawMessage) (nativeContent, error) {
	var content nativeContent
	if json.Unmarshal(raw, &content) != nil || (content.Role != "model" && content.Role != "user") || len(content.Parts) == 0 {
		return content, invalidRequest("invalid native Google content")
	}
	for _, part := range content.Parts {
		if len(part) == 0 || part[0] != '{' {
			return content, invalidRequest("invalid native Google part")
		}
	}
	return content, nil
}
func localCallPrefix(content json.RawMessage, historyIndex int) string {
	var compact bytes.Buffer
	_ = json.Compact(&compact, content)
	hash := sha256.Sum256(compact.Bytes())
	return "reforge_gemini_" + hex.EncodeToString(hash[:]) + fmt.Sprintf("_%d_", historyIndex)
}
func buildRequest(request model.TurnRequest) ([]json.RawMessage, *genai.GenerateContentConfig, error) {
	contents := make([]json.RawMessage, 0, len(request.Messages)+2)
	if len(request.Continuation) > 0 {
		if len(request.Continuation) > model.MaxRequestBytes || json.Unmarshal(request.Continuation, &contents) != nil || len(contents) == 0 {
			return nil, nil, invalidRequest("invalid Google continuation")
		}
	}
	refs := map[string]callReference{}
	for historyIndex, raw := range contents {
		content, err := readContent(raw)
		if err != nil {
			return nil, nil, err
		}
		prefix := localCallPrefix(raw, historyIndex)
		for index, rawPart := range content.Parts {
			var part nativePart
			if json.Unmarshal(rawPart, &part) != nil {
				return nil, nil, invalidRequest("invalid Google part")
			}
			if call := part.FunctionCall; call != nil {
				if call.Name == "" || len(call.PartialArgs) > 0 || len(call.WillContinue) > 0 {
					return nil, nil, invalidRequest("invalid Google call history")
				}
				id := call.ID
				if id == "" {
					id = prefix + fmt.Sprint(index)
				} else if strings.HasPrefix(id, "reforge_gemini_") {
					return nil, nil, invalidRequest("reserved Google call identity")
				}
				if _, exists := refs[id]; exists {
					return nil, nil, invalidRequest("ambiguous Google call identity")
				}
				refs[id] = callReference{ID: call.ID, Name: call.Name}
			}
		}
	}
	responded := map[string]bool{}
	toolGroup := false
	for _, message := range request.Messages {
		if len(message.Opaque) > 0 {
			if _, err := readContent(message.Opaque); err != nil {
				return nil, nil, err
			}
			contents = append(contents, append(json.RawMessage(nil), message.Opaque...))
			toolGroup = false
			continue
		}
		role := "user"
		if message.Role == "assistant" {
			role = "model"
		} else if message.Role != "user" && message.Role != "tool" {
			return nil, nil, invalidRequest("invalid message role")
		}
		if message.Role == "tool" && message.ToolCallID == "" {
			return nil, nil, invalidRequest("tool response identity is required")
		}
		parts := []any{}
		if message.ToolCallID != "" {
			ref, exists := refs[message.ToolCallID]
			if !exists || responded[message.ToolCallID] || message.Role != "tool" {
				return nil, nil, invalidRequest("tool response requires unique native call history")
			}
			responded[message.ToolCallID] = true
			output := map[string]any{"output": message.Text}
			var object map[string]any
			if decodeNumber([]byte(message.Text), &object) == nil && object != nil {
				output = object
			}
			response := map[string]any{"name": ref.Name, "response": output}
			if ref.ID != "" {
				response["id"] = ref.ID
			}
			parts = append(parts, map[string]any{"functionResponse": response})
		} else if len(message.ToolCalls) > 0 {
			if role != "model" {
				return nil, nil, invalidRequest("invalid tool call role")
			}
			for _, call := range message.ToolCalls {
				var args map[string]any
				if call.ID == "" || call.Name == "" || decodeNumber(call.Arguments, &args) != nil || args == nil {
					return nil, nil, invalidRequest("invalid tool call")
				}
				parts = append(parts, map[string]any{"functionCall": map[string]any{"id": call.ID, "name": call.Name, "args": json.RawMessage(call.Arguments)}})
			}
		} else if message.Text != "" {
			parts = append(parts, map[string]any{"text": message.Text})
		}
		if len(parts) > 0 {
			raw, _ := json.Marshal(map[string]any{"role": role, "parts": parts})
			if toolGroup && message.ToolCallID != "" {
				var previous nativeContent
				var next nativeContent
				_ = json.Unmarshal(contents[len(contents)-1], &previous)
				_ = json.Unmarshal(raw, &next)
				previous.Parts = append(previous.Parts, next.Parts...)
				contents[len(contents)-1], _ = json.Marshal(previous)
			} else {
				contents = append(contents, raw)
			}
		}
		toolGroup = message.ToolCallID != ""
	}
	if len(contents) == 0 {
		return nil, nil, invalidRequest("Google content is required")
	}
	config := &genai.GenerateContentConfig{MaxOutputTokens: int32(request.MaxOutputTokens), HTTPOptions: noRetryOptions()}
	if request.System != "" {
		config.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: request.System}}}
	}
	declarations := []map[string]any{}
	for _, tool := range request.Tools {
		declarations = append(declarations, map[string]any{"name": tool.Name, "description": tool.Description, "parametersJsonSchema": json.RawMessage(tool.Schema)})
	}
	config.HTTPOptions.ExtrasRequestProvider = func(body map[string]any) map[string]any {
		body["contents"] = contents
		if len(declarations) > 0 {
			body["tools"] = []any{map[string]any{"functionDeclarations": declarations}}
		}
		return body
	}
	return contents, config, nil
}
func parseStream(raw []byte, schemas model.ToolSchemas, historyIndex int) (*streamState, error) {
	state := &streamState{fields: map[string]json.RawMessage{}}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), maxResponseBody)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			return nil, protocolError("invalid Google stream frame")
		}
		var frame struct {
			ResponseID string `json:"responseId"`
			Candidates []struct {
				Index        *int            `json:"index"`
				Content      json.RawMessage `json:"content"`
				FinishReason string          `json:"finishReason"`
			} `json:"candidates"`
			Usage map[string]json.RawMessage `json:"usageMetadata"`
		}
		if json.Unmarshal(bytes.TrimSpace(line[5:]), &frame) != nil {
			return nil, protocolError("invalid Google stream JSON")
		}
		if frame.ResponseID != "" {
			if state.responseID != "" && state.responseID != frame.ResponseID {
				return nil, protocolError("Google response identity changed")
			}
			state.responseID = frame.ResponseID
		}
		if frame.Usage != nil {
			state.usage = frame.Usage
		}
		if len(frame.Candidates) > 1 {
			return nil, protocolError("multiple Google candidates are unsupported")
		}
		for _, candidate := range frame.Candidates {
			if candidate.Index != nil && *candidate.Index != 0 {
				return nil, protocolError("unexpected Google candidate")
			}
			if state.finish != "" {
				return nil, protocolError("Google candidate continued after completion")
			}
			if len(candidate.Content) > 0 && string(candidate.Content) != "null" {
				var fields map[string]json.RawMessage
				var content nativeContent
				if json.Unmarshal(candidate.Content, &fields) != nil || json.Unmarshal(candidate.Content, &content) != nil || content.Role != "" && content.Role != "model" {
					return nil, protocolError("invalid Google content")
				}
				for key, value := range fields {
					if key == "parts" {
						continue
					}
					if previous, exists := state.fields[key]; exists && !bytes.Equal(previous, value) {
						return nil, protocolError("Google content metadata changed")
					}
					state.fields[key] = value
				}
				for _, part := range content.Parts {
					if len(part) == 0 || part[0] != '{' {
						return nil, protocolError("invalid Google part")
					}
					state.parts = append(state.parts, part)
				}
			}
			if candidate.FinishReason != "" {
				state.finish = candidate.FinishReason
			}
		}
	}
	if scanner.Err() != nil || state.finish == "" || len(state.parts) == 0 {
		return nil, protocolError("Google response ended before completion")
	}
	content, err := state.content()
	if err != nil {
		return nil, err
	}
	prefix := localCallPrefix(content, historyIndex)
	seen := map[string]bool{}
	for index, rawPart := range state.parts {
		var part nativePart
		if json.Unmarshal(rawPart, &part) != nil {
			return nil, protocolError("invalid Google part")
		}
		call := part.FunctionCall
		if call == nil {
			continue
		}
		if state.finish != "STOP" || part.Text != "" || call.Name == "" || len(call.PartialArgs) > 0 || len(call.WillContinue) > 0 {
			return nil, protocolError("Google tool call did not complete normally")
		}
		var args map[string]any
		if len(call.Args) == 0 {
			call.Args = json.RawMessage(`{}`)
		}
		if decodeNumber(call.Args, &args) != nil || args == nil {
			return nil, protocolError("invalid Google tool arguments")
		}
		id := call.ID
		if id == "" {
			id = prefix + fmt.Sprint(index)
		} else if strings.HasPrefix(id, "reforge_gemini_") {
			return nil, protocolError("reserved Google call identity")
		}
		if seen[id] {
			return nil, protocolError("duplicate Google call identity")
		}
		seen[id] = true
		if err := schemas.Registered(model.ToolCall{ID: id, Name: call.Name, Arguments: call.Args}); err != nil {
			return nil, protocolError("Google tool call is not registered")
		}
		state.calls = append(state.calls, &callState{ID: id, Name: call.Name, Arguments: call.Args})
	}
	return state, nil
}
func (s *streamState) content() (json.RawMessage, error) {
	fields := make(map[string]json.RawMessage, len(s.fields)+2)
	for key, value := range s.fields {
		fields[key] = value
	}
	fields["role"] = json.RawMessage(`"model"`)
	fields["parts"], _ = json.Marshal(s.parts)
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, protocolError("invalid Google continuation")
	}
	return raw, nil
}
func (s *streamState) usageValue() *model.Usage {
	unknown := &model.Usage{Known: false, Source: "completed_response"}
	values := map[string]int64{}
	for _, field := range []string{"promptTokenCount", "candidatesTokenCount", "thoughtsTokenCount", "cachedContentTokenCount", "totalTokenCount", "toolUsePromptTokenCount"} {
		raw, present := s.usage[field]
		if !present {
			if field == "promptTokenCount" || field == "candidatesTokenCount" {
				return unknown
			}
			continue
		}
		var n int64
		if string(raw) == "null" || json.Unmarshal(raw, &n) != nil || n < 0 || n > 1<<40 {
			return unknown
		}
		values[field] = n
	}
	input, output, thought, cache := values["promptTokenCount"], values["candidatesTokenCount"], values["thoughtsTokenCount"], values["cachedContentTokenCount"]
	if output > math.MaxInt64-thought || cache > input || values["toolUsePromptTokenCount"] != 0 {
		return unknown
	}
	output += thought
	if _, present := s.usage["totalTokenCount"]; present && (input > math.MaxInt64-output || values["totalTokenCount"] != input+output) {
		return unknown
	}
	return &model.Usage{InputTokens: input, OutputTokens: output, CacheTokens: cache, Known: true, Source: "completed_response"}
}
