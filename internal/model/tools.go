package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"reforge/internal/domain"
)

const MaxRequestBytes = 1 << 20
const MaxToolArguments = 256 << 10

var toolName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type ToolSchemas map[string]*jsonschema.Schema
type offlineLoader struct{}

func (offlineLoader) Load(string) (any, error) {
	return nil, errors.New("external schemas are not permitted")
}

func CompileTools(tools []Tool) (ToolSchemas, error) {
	if len(tools) > 64 {
		return nil, &domain.ProviderError{Kind: "invalid_request", Message: "too many tools"}
	}
	result := ToolSchemas{}
	for _, tool := range tools {
		if !toolName.MatchString(tool.Name) || result[tool.Name] != nil || len(tool.Schema) > 64<<10 || len(tool.Description) > 8192 {
			return nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid tool definition"}
		}
		value, err := decodeJSON(tool.Schema)
		if err != nil {
			return nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid tool schema"}
		}
		object, ok := value.(map[string]any)
		if !ok || object["type"] != "object" {
			return nil, &domain.ProviderError{Kind: "invalid_request", Message: "tool schema must describe an object"}
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.UseLoader(offlineLoader{})
		compiler.AssertFormat()
		if err = compiler.AddResource("https://reforge.invalid/tool.json", value); err != nil {
			return nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid tool schema"}
		}
		schema, err := compiler.Compile("https://reforge.invalid/tool.json")
		if err != nil {
			return nil, &domain.ProviderError{Kind: "invalid_request", Message: "invalid or externally referenced tool schema"}
		}
		result[tool.Name] = schema
	}
	return result, nil
}

func (s ToolSchemas) Registered(call ToolCall) error {
	if call.ID == "" || len(call.ID) > 256 || s[call.Name] == nil {
		return &domain.ProviderError{Kind: "protocol", Message: "unregistered tool or invalid call identity", Uncertain: true}
	}
	if len(call.Arguments) > MaxToolArguments {
		return &domain.ProviderError{Kind: "protocol", Message: "tool arguments exceed limit", Uncertain: true}
	}
	return nil
}

func (s ToolSchemas) Check(call *ToolCall) error {
	if err := s.Registered(*call); err != nil {
		return err
	}
	value, err := decodeJSON(call.Arguments)
	if err != nil {
		call.Arguments, call.Invalid = json.RawMessage(`{}`), "arguments are not valid JSON"
		return nil
	}
	if err = s[call.Name].Validate(value); err != nil {
		call.Invalid = err.Error()
		if len(call.Invalid) > 500 {
			call.Invalid = call.Invalid[:500]
		}
	}
	return nil
}

func (s ToolSchemas) Validate(call ToolCall) error {
	if err := s.Registered(call); err != nil {
		return err
	}
	value, err := decodeJSON(call.Arguments)
	if err != nil || s[call.Name].Validate(value) != nil {
		return &domain.ProviderError{Kind: "protocol", Message: "tool arguments do not match the registered schema", Uncertain: true}
	}
	return nil
}

func decodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	return value, nil
}
