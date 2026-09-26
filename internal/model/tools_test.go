package model

import (
	"encoding/json"
	"testing"
)

func TestToolSchemasRejectExternalReferencesAndValidateNestedLimits(t *testing.T) {
	for _, ref := range []string{"https://example.test/schema", "file:///etc/passwd"} {
		raw, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{"value": map[string]string{"$ref": ref}}})
		if _, err := CompileTools([]Tool{{Name: "edit", Schema: raw}}); err == nil {
			t.Fatal("external schema accepted")
		}
	}
	schemas, err := CompileTools([]Tool{{Name: "edit", Schema: json.RawMessage(`{"type":"object","required":["files"],"additionalProperties":false,"properties":{"files":{"type":"array","maxItems":1,"items":{"type":"object","required":["path","mode"],"additionalProperties":false,"properties":{"path":{"type":"string","minLength":1},"mode":{"enum":["write","delete"]}}}}}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"files":[{"path":"a","mode":"chmod"}]}`, `{"files":[{"path":"","mode":"write"}]}`, `{"files":[{"path":"a","mode":"write"},{"path":"b","mode":"delete"}]}`, `{"files":[],"extra":1}`} {
		if schemas.Validate(ToolCall{ID: "one", Name: "edit", Arguments: json.RawMessage(raw)}) == nil {
			t.Fatalf("invalid arguments accepted: %s", raw)
		}
	}
	if err := schemas.Validate(ToolCall{ID: "one", Name: "edit", Arguments: json.RawMessage(`{"files":[{"path":"a","mode":"write"}]}`)}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckMarksMalformedArgumentsForTheModel(t *testing.T) {
	schemas, err := CompileTools([]Tool{{Name: "edit", Schema: json.RawMessage(`{"type":"object","required":["path"],"additionalProperties":false,"properties":{"path":{"type":"string"}}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	call := ToolCall{ID: "one", Name: "edit", Arguments: json.RawMessage(`{"file":"a"}`)}
	if err = schemas.Check(&call); err != nil || call.Invalid == "" {
		t.Fatalf("schema mismatch not returned to the model: %v %q", err, call.Invalid)
	}
	call = ToolCall{ID: "two", Name: "edit", Arguments: json.RawMessage(`{"path":`)}
	if err = schemas.Check(&call); err != nil || call.Invalid == "" || string(call.Arguments) != "{}" {
		t.Fatalf("broken JSON not replaced: %v %+v", err, call)
	}
	if schemas.Check(&ToolCall{ID: "three", Name: "shell", Arguments: json.RawMessage(`{}`)}) == nil {
		t.Fatal("unregistered tool accepted")
	}
}
