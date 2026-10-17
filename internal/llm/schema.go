package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type ToolSchema struct {
	Full       map[string]any
	Normalized json.RawMessage
	Properties map[string]any
	Required   []string
	Extra      map[string]any
}

func ParseToolSchema(spec ToolSpec) (ToolSchema, error) {
	full := map[string]any{}
	if len(bytes.TrimSpace(spec.InputSchema)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(spec.InputSchema))
		if err := dec.Decode(&full); err != nil {
			return ToolSchema{}, fmt.Errorf("tool %q: input schema is not a JSON object: %w", spec.Name, err)
		}
		if _, err := dec.Token(); !errors.Is(err, io.EOF) {
			return ToolSchema{}, fmt.Errorf("tool %q: input schema has content after the JSON object", spec.Name)
		}
		if full == nil {
			full = map[string]any{}
		}
	}
	if t, ok := full["type"]; ok && t != "object" {
		return ToolSchema{}, fmt.Errorf("tool %q: input schema type must be \"object\", got %v", spec.Name, t)
	}
	full["type"] = "object"

	normalized, err := json.Marshal(full)
	if err != nil {
		return ToolSchema{}, fmt.Errorf("tool %q: input schema cannot be encoded: %w", spec.Name, err)
	}

	schema := ToolSchema{Full: full, Normalized: normalized, Extra: map[string]any{}}
	for key, value := range full {
		switch key {
		case "type":
		case "properties":
			props, ok := value.(map[string]any)
			if !ok {
				return ToolSchema{}, fmt.Errorf("tool %q: input schema properties must be an object", spec.Name)
			}
			schema.Properties = props
		case "required":
			required, err := stringList(value)
			if err != nil {
				return ToolSchema{}, fmt.Errorf("tool %q: input schema required %w", spec.Name, err)
			}
			schema.Required = required
		default:
			schema.Extra[key] = value
		}
	}
	return schema, nil
}

var errNotStringList = errors.New("must be an array of strings")

func stringList(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, errNotStringList
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, errNotStringList
		}
		out = append(out, s)
	}
	return out, nil
}
