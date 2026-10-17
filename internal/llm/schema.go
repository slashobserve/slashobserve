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

	if value, ok := full["properties"]; ok {
		if _, ok := value.(map[string]any); !ok {
			return ToolSchema{}, fmt.Errorf("tool %q: input schema properties must be an object", spec.Name)
		}
	}
	if value, ok := full["required"]; ok {
		if err := checkStringList(value); err != nil {
			return ToolSchema{}, fmt.Errorf("tool %q: input schema required %w", spec.Name, err)
		}
	}
	return ToolSchema{Full: full, Normalized: normalized}, nil
}

var errNotStringList = errors.New("must be an array of strings")

func checkStringList(value any) error {
	items, ok := value.([]any)
	if !ok {
		return errNotStringList
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return errNotStringList
		}
	}
	return nil
}
