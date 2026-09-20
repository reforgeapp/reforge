package agent

import (
	"context"
	"encoding/json"
	"io/fs"
	"time"

	"reforge/internal/domain"
	"reforge/internal/model"
	"reforge/internal/sandbox"
)

func brokerTools() []model.Tool {
	return []model.Tool{
		{Name: "reforge_read", Description: "Read a bounded file from the isolated repository workspace", Schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1,"maxLength":1024}},"required":["path"],"additionalProperties":false}`)},
		{Name: "reforge_patch", Description: "Apply bounded file changes in the isolated repository workspace", Schema: json.RawMessage(`{"type":"object","properties":{"patches":{"type":"array","minItems":1,"maxItems":64,"items":{"type":"object","properties":{"path":{"type":"string","minLength":1,"maxLength":1024},"content":{"type":"string","maxLength":262144},"delete":{"type":"boolean"}},"required":["path"],"additionalProperties":false}}},"required":["patches"],"additionalProperties":false}`)},
		{Name: "reforge_exec", Description: "Execute bounded argv without network inside the isolated repository workspace", Schema: json.RawMessage(`{"type":"object","properties":{"args":{"type":"array","minItems":1,"maxItems":64,"items":{"type":"string","maxLength":4096}},"directory":{"type":"string","maxLength":1024},"timeoutMillis":{"type":"integer","minimum":1,"maximum":60000},"maxOutputBytes":{"type":"integer","minimum":1,"maximum":262144}},"required":["args","timeoutMillis","maxOutputBytes"],"additionalProperties":false}`)},
	}
}
func toolDefinitions() []map[string]any {
	var result []map[string]any
	for _, tool := range brokerTools() {
		result = append(result, map[string]any{"type": "function", "name": tool.Name, "description": tool.Description, "inputSchema": tool.Schema})
	}
	return result
}
func (c *Codex) tool(ctx context.Context, p packet) error {
	var params struct {
		ThreadID  string          `json:"threadId"`
		TurnID    string          `json:"turnId"`
		CallID    string          `json:"callId"`
		Tool      string          `json:"tool"`
		Namespace *string         `json:"namespace"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(p.Params, &params) != nil || params.Namespace != nil && *params.Namespace != "" || params.CallID == "" || len(params.CallID) > 256 {
		return ErrProtocol
	}
	c.mu.Lock()
	ready := c.ready
	c.mu.Unlock()
	if ready == nil {
		return ErrDenied
	}
	select {
	case <-ready:
	case <-ctx.Done():
		return ErrUncertain
	}
	c.mu.Lock()
	if params.ThreadID != c.thread || params.TurnID != c.turn || c.state != "running" || c.calls[params.CallID] || c.toolCount >= c.config.MaxToolCalls {
		c.mu.Unlock()
		return ErrDenied
	}
	c.calls[params.CallID] = true
	c.toolCount++
	request := c.request
	c.mu.Unlock()
	schemas, err := model.CompileTools(brokerTools())
	if err != nil {
		return err
	}
	if err = schemas.Validate(model.ToolCall{ID: params.CallID, Name: params.Tool, Arguments: params.Arguments}); err != nil {
		return ErrDenied
	}

	var result any
	var perform func() error
	switch params.Tool {
	case "reforge_read":
		var input struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(params.Arguments, &input)
		if !fs.ValidPath(input.Path) || input.Path == "." {
			return ErrDenied
		}
		perform = func() error {
			artifact, err := c.config.Sandbox.CollectArtifact(ctx, request.Workspace, input.Path)
			if err != nil {
				return err
			}
			if len(artifact.Data) > 256<<10 {
				return ErrDenied
			}
			result = map[string]any{"content": artifact.Data, "sha256": artifact.SHA256}
			return nil
		}
	case "reforge_patch":
		var input struct {
			Patches []struct {
				Path    string `json:"path"`
				Content string `json:"content"`
				Delete  bool   `json:"delete"`
			} `json:"patches"`
		}
		_ = json.Unmarshal(params.Arguments, &input)
		patches := make([]sandbox.Patch, 0, len(input.Patches))
		total := 0
		for _, patch := range input.Patches {
			total += len(patch.Content)
			if !fs.ValidPath(patch.Path) || patch.Path == "." || total > 256<<10 {
				return ErrDenied
			}
			patches = append(patches, sandbox.Patch{Path: patch.Path, Content: []byte(patch.Content), Delete: patch.Delete})
		}
		perform = func() error {
			if err := c.config.Sandbox.ApplyPatch(ctx, request.Workspace, patches); err != nil {
				return err
			}
			result = map[string]any{"applied": true}
			return nil
		}
	case "reforge_exec":
		var input struct {
			Args           []string `json:"args"`
			Directory      string   `json:"directory"`
			TimeoutMillis  int64    `json:"timeoutMillis"`
			MaxOutputBytes int64    `json:"maxOutputBytes"`
		}
		_ = json.Unmarshal(params.Arguments, &input)
		command := sandbox.Command{Args: input.Args, Directory: input.Directory, Timeout: time.Duration(input.TimeoutMillis) * time.Millisecond, MaxOutputBytes: input.MaxOutputBytes, NetworkProfile: "none"}
		if err := validateCommand(command); err != nil {
			return err
		}
		perform = func() error {
			output, err := c.config.Sandbox.ExecuteBoundedCommand(ctx, request.Workspace, command)
			if err != nil {
				return err
			}
			if len(output.Output) > 256<<10 {
				return ErrDenied
			}
			result = output
			return nil
		}
	default:
		return ErrDenied
	}
	c.mu.Lock()
	if c.state != "running" {
		c.mu.Unlock()
		return ErrDenied
	}
	c.activeEffects++
	c.mu.Unlock()
	err = c.authorize(ctx, Effect{Binding: c.config.Binding, Request: request, OperationID: domain.NewID(), Kind: params.Tool, CallID: params.CallID, Arguments: append(json.RawMessage(nil), params.Arguments...)}, perform)
	c.mu.Lock()
	c.activeEffects--
	c.mu.Unlock()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > 512<<10 {
		return ErrProtocol
	}
	return c.reply(ctx, p.ID, map[string]any{"success": true, "contentItems": []any{map[string]any{"type": "inputText", "text": string(raw)}}})
}
