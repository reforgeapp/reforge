package agent

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/reforgeapp/reforge/pkg/model"
)

const nativeInstructions = "You are the coding agent for Reforge. Reforge executes repository tools in a separate isolated workspace and independently verifies changes. Return only the requested structured response. Request tools using tool_calls; arguments must be a JSON-encoded string matching that tool's schema. Never invoke native tools, browse the host, read credentials, or claim a change was applied before receiving its tool result. Return an empty tool_calls array when finished."

const nativeReplySchema = `{"type":"object","properties":{"text":{"type":"string"},"tool_calls":{"type":"array","maxItems":16,"items":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"},"arguments":{"type":"string"}},"required":["id","name","arguments"],"additionalProperties":false}}},"required":["text","tool_calls"],"additionalProperties":false}`

func nativePrompt(in model.Turn) (string, error) {
	raw, err := json.Marshal(struct {
		Instructions string          `json:"instructions"`
		System       string          `json:"system"`
		Messages     []model.Message `json:"messages"`
		Tools        []model.Tool    `json:"tools"`
	}{nativeInstructions, in.System, in.Messages, in.Tools})
	if err != nil || len(raw) > model.MaxRequestBytes/2 {
		return "", ErrDenied
	}
	return string(raw), nil
}

func nativeReply(in model.Turn, raw []byte, usage model.Usage) (model.TurnResult, error) {
	var response struct {
		Text  *string `json:"text"`
		Calls []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"tool_calls"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 512<<10 || decoder.Decode(&response) != nil || response.Text == nil || response.Calls == nil || len(response.Calls) > 16 || len(*response.Text) > 256<<10 || decoder.Decode(new(any)) != io.EOF {
		return model.TurnResult{}, nativeFailure("provider", true)
	}
	schemas, err := model.CompileTools(in.Tools)
	if err != nil {
		return model.TurnResult{}, err
	}
	out := model.TurnResult{Text: *response.Text, Usage: usage}
	ids := map[string]bool{}
	for _, item := range response.Calls {
		call := model.ToolCall{ID: item.ID, Name: item.Name, Arguments: json.RawMessage(item.Arguments)}
		if call.ID == "" || len(call.ID) > 200 || ids[call.ID] || schemas.Validate(call) != nil {
			return model.TurnResult{}, nativeFailure("provider", true)
		}
		ids[call.ID] = true
		out.ToolCalls = append(out.ToolCalls, call)
	}
	if len(out.ToolCalls) == 0 {
		out.FinishReason = "stop"
	} else {
		out.FinishReason = "tool_calls"
	}
	return out, nil
}
