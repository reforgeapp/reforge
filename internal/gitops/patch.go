package gitops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

const maxManifestSize = 1 << 20

func ReadManifestField(content []byte, pointer string) (string, error) {
	_, node, err := parseManifest(content, pointer)
	if err != nil {
		return "", err
	}
	if _, _, err := scalarRange(content, node.GetToken()); err != nil {
		return "", err
	}
	value, ok := node.GetValue().(string)
	if !ok || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("manifest target must be a single-line string")
	}
	return value, nil
}

func PatchManifest(content []byte, pointer, expected, desired string) ([]byte, error) {
	if len(content) > maxManifestSize {
		return nil, fmt.Errorf("manifest exceeds 1 MiB")
	}
	if strings.ContainsAny(desired, "\r\n") {
		return nil, fmt.Errorf("desired value must be single-line")
	}
	isJSON := isJSONDocument(content)
	beforeFile, node, err := parseManifest(content, pointer)
	if err != nil {
		return nil, err
	}
	value, ok := node.GetValue().(string)
	if !ok || strings.ContainsAny(value, "\r\n") {
		return nil, fmt.Errorf("manifest target must be a single-line string")
	}
	if value != expected {
		return nil, fmt.Errorf("manifest target does not match expected value")
	}
	start, end, err := scalarRange(content, node.GetToken())
	if err != nil {
		return nil, err
	}
	replacement, err := formatScalar(desired, node.GetToken(), isJSON)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(content)-end+start+len(replacement))
	out = append(out, content[:start]...)
	out = append(out, replacement...)
	out = append(out, content[end:]...)
	afterFile, afterNode, err := parseManifest(out, pointer)
	if err != nil {
		return nil, fmt.Errorf("patched manifest failed validation: %w", err)
	}
	got, ok := afterNode.GetValue().(string)
	if !ok || got != desired {
		return nil, fmt.Errorf("patched manifest target mismatch")
	}
	segments, _ := pointerSegments(pointer)
	beforeTree, err := semantic(beforeFile.Docs[0].Body)
	if err != nil {
		return nil, err
	}
	afterTree, err := semantic(afterFile.Docs[0].Body)
	if err != nil {
		return nil, err
	}
	if !replaceSemantic(beforeTree, segments, desired) || !reflect.DeepEqual(beforeTree, afterTree) {
		return nil, fmt.Errorf("patched manifest changed unrelated fields")
	}
	return out, nil
}

func parseManifest(content []byte, pointer string) (*ast.File, ast.ScalarNode, error) {
	if len(content) == 0 || len(content) > maxManifestSize || !utf8.Valid(content) {
		return nil, nil, fmt.Errorf("manifest is empty or exceeds 1 MiB")
	}
	segments, err := pointerSegments(pointer)
	if err != nil {
		return nil, nil, err
	}
	tokens := lexer.Tokenize(string(content))
	if len(tokens) > 100000 {
		return nil, nil, fmt.Errorf("manifest exceeds token limit")
	}
	depth := 0
	for _, item := range tokens {
		switch item.Type {
		case token.SequenceStartType, token.MappingStartType:
			depth++
		case token.SequenceEndType, token.MappingEndType:
			depth--
		}
		if depth > 128 || item.Position != nil && item.Position.IndentLevel > 128 {
			return nil, nil, fmt.Errorf("manifest exceeds nesting limit")
		}
	}
	file, err := parser.Parse(tokens, 0)
	if err != nil || len(file.Docs) != 1 {
		if err != nil {
			return nil, nil, fmt.Errorf("invalid manifest: %w", err)
		}
		return nil, nil, fmt.Errorf("manifest must contain exactly one document")
	}
	if err := validateNode(file.Docs[0].Body); err != nil {
		return nil, nil, err
	}
	node, err := selectNode(file.Docs[0].Body, segments)
	if err != nil {
		return nil, nil, err
	}
	value, ok := node.(ast.ScalarNode)
	if !ok {
		return nil, nil, fmt.Errorf("manifest target must be scalar string")
	}
	return file, value, nil
}

func pointerSegments(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, fmt.Errorf("manifest pointer must identify a field")
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("manifest pointer must use RFC6901 syntax")
	}
	parts := strings.Split(pointer[1:], "/")
	for i, part := range parts {
		var b strings.Builder
		for j := 0; j < len(part); j++ {
			if part[j] != '~' {
				b.WriteByte(part[j])
				continue
			}
			if j+1 >= len(part) || (part[j+1] != '0' && part[j+1] != '1') {
				return nil, fmt.Errorf("invalid RFC6901 escape in segment %d", i)
			}
			if part[j+1] == '0' {
				b.WriteByte('~')
			} else {
				b.WriteByte('/')
			}
			j++
		}
		parts[i] = b.String()
	}
	return parts, nil
}

func validateNode(node ast.Node) error {
	switch value := node.(type) {
	case *ast.DocumentNode:
		return validateNode(value.Body)
	case *ast.MappingNode:
		seen := map[string]struct{}{}
		for _, item := range value.Values {
			if item.Key.IsMergeKey() {
				return fmt.Errorf("manifest merge keys are unsupported")
			}
			keyNode, ok := item.Key.(ast.ScalarNode)
			if !ok {
				return fmt.Errorf("manifest mapping keys must be strings")
			}
			key, ok := keyNode.GetValue().(string)
			if !ok {
				return fmt.Errorf("manifest mapping keys must be strings")
			}
			if _, ok := seen[key]; ok {
				return fmt.Errorf("manifest contains duplicate mapping key %q", key)
			}
			seen[key] = struct{}{}
			if err := validateNode(item.Value); err != nil {
				return err
			}
		}
	case *ast.SequenceNode:
		for _, item := range value.Values {
			if err := validateNode(item); err != nil {
				return err
			}
		}
	case *ast.AnchorNode, *ast.AliasNode, *ast.LiteralNode, *ast.TagNode:
		return fmt.Errorf("manifest aliases and anchors are unsupported")
	}
	return nil
}

func semantic(node ast.Node) (any, error) {
	switch value := node.(type) {
	case *ast.DocumentNode:
		return semantic(value.Body)
	case ast.ScalarNode:
		return value.GetValue(), nil
	case *ast.MappingNode:
		result := make(map[string]any, len(value.Values))
		for _, item := range value.Values {
			key, ok := item.Key.(ast.ScalarNode)
			if !ok {
				return nil, fmt.Errorf("manifest mapping key is not scalar")
			}
			name, ok := key.GetValue().(string)
			if !ok {
				return nil, fmt.Errorf("manifest mapping key is not string")
			}
			child, err := semantic(item.Value)
			if err != nil {
				return nil, err
			}
			result[name] = child
		}
		return result, nil
	case *ast.SequenceNode:
		result := make([]any, len(value.Values))
		for i, item := range value.Values {
			child, err := semantic(item)
			if err != nil {
				return nil, err
			}
			result[i] = child
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported manifest node")
	}
}

func replaceSemantic(node any, segments []string, desired string) bool {
	if len(segments) == 0 {
		return false
	}
	if len(segments) == 1 {
		if list, ok := node.([]any); ok {
			index, err := strconv.Atoi(segments[0])
			if err != nil || index < 0 || index >= len(list) {
				return false
			}
			list[index] = desired
			return true
		}
		object, ok := node.(map[string]any)
		if !ok {
			return false
		}
		if _, exists := object[segments[0]]; !exists {
			return false
		}
		object[segments[0]] = desired
		return true
	}
	if object, ok := node.(map[string]any); ok {
		child, exists := object[segments[0]]
		return exists && replaceSemantic(child, segments[1:], desired)
	}
	if list, ok := node.([]any); ok {
		index, err := strconv.Atoi(segments[0])
		if err != nil || index < 0 || index >= len(list) {
			return false
		}
		return replaceSemantic(list[index], segments[1:], desired)
	}
	return false
}

func selectNode(node ast.Node, segments []string) (ast.Node, error) {
	if len(segments) == 0 {
		return node, nil
	}
	switch value := node.(type) {
	case *ast.DocumentNode:
		return selectNode(value.Body, segments)
	case *ast.MappingNode:
		for _, item := range value.Values {
			keyNode, _ := item.Key.(ast.ScalarNode)
			key, _ := keyNode.GetValue().(string)
			if key == segments[0] {
				return selectNode(item.Value, segments[1:])
			}
		}
		return nil, fmt.Errorf("manifest pointer segment %q not found", segments[0])
	case *ast.SequenceNode:
		index, err := strconv.Atoi(segments[0])
		if err != nil || index < 0 || strconv.Itoa(index) != segments[0] || index >= len(value.Values) {
			return nil, fmt.Errorf("invalid manifest array index %q", segments[0])
		}
		return selectNode(value.Values[index], segments[1:])
	default:
		return nil, fmt.Errorf("manifest pointer traverses non-container")
	}
}

func scalarRange(content []byte, value *token.Token) (int, int, error) {
	start := positionOffset(content, value)
	if start < 0 || start >= len(content) {
		return 0, 0, fmt.Errorf("manifest scalar position is invalid")
	}
	if value.Type != token.DoubleQuoteType && value.Type != token.SingleQuoteType {
		for start < len(content) && (content[start] == ' ' || content[start] == '\t') {
			start++
		}
	}
	if start >= len(content) {
		return 0, 0, fmt.Errorf("manifest scalar range is invalid")
	}
	if value.Type == token.DoubleQuoteType || value.Type == token.SingleQuoteType {
		quote := content[start]
		if quote != '\'' && quote != '"' {
			return 0, 0, fmt.Errorf("manifest quoted scalar position does not match")
		}
		for i := start + 1; i < len(content); i++ {
			if content[i] != quote {
				continue
			}
			if quote == '\'' && i+1 < len(content) && content[i+1] == quote {
				i++
				continue
			}
			if quote == '"' {
				escapes := 0
				for j := i - 1; j >= start && content[j] == '\\'; j-- {
					escapes++
				}
				if escapes%2 == 1 {
					continue
				}
			}
			if bytes.ContainsAny(content[start:i+1], "\r\n") {
				return 0, 0, fmt.Errorf("multiline manifest scalar is unsupported")
			}
			return start, i + 1, nil
		}
		return 0, 0, fmt.Errorf("manifest quoted scalar is unterminated")
	}
	end := start + len(value.Value)
	if end > len(content) || string(content[start:end]) != value.Value {
		return 0, 0, fmt.Errorf("manifest scalar range is invalid")
	}
	return start, end, nil
}

func positionOffset(content []byte, value *token.Token) int {
	line, column := 1, 1
	index := 0
	for index < len(content) && line < value.Position.Line {
		if content[index] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
		index++
	}
	for column < value.Position.Column && index < len(content) {
		_, size := utf8.DecodeRune(content[index:])
		index += size
		column++
	}
	return index
}

func formatScalar(value string, source *token.Token, isJSON bool) (string, error) {
	if isJSON || source.Type == token.DoubleQuoteType {
		encoded, err := json.Marshal(value)
		return string(encoded), err
	}
	if source.Type == token.SingleQuoteType {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
	}
	if value == "" {
		return "''", nil
	}
	if strings.TrimSpace(value) != value || strings.Contains(value, " #") || strings.Contains(value, ": ") || strings.ContainsAny(value, "{}[]&,*!|>'\"%@`\r\n") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "?") || strings.HasPrefix(value, ":") || strings.HasPrefix(value, "#") {
		encoded, err := json.Marshal(value)
		return string(encoded), err
	}
	return value, nil
}

func isJSONDocument(content []byte) bool {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return false
	}
	return (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(content)
}
