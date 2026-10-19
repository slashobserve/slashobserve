package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrNoPanel = errors.New("spec: panel not found")

func WithPanel(d *Dashboard, panelID string, panel json.RawMessage) (*Dashboard, error) {
	entry, ok := d.FindPanel(panelID)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoPanel, panelID)
	}
	base, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	tree, err := decodeNumbers(base)
	if err != nil {
		return nil, err
	}
	draft, err := decodeNumbers(panel)
	if err != nil {
		return nil, &ValidationError{Issues: []Issue{{Path: entry.Pointer, Code: CodeSchema, Message: "the panel is not valid JSON"}}}
	}
	if err := replaceAt(tree, entry.Pointer, draft); err != nil {
		return nil, err
	}
	out, err := json.Marshal(tree)
	if err != nil {
		return nil, err
	}
	return Parse(out)
}

func decodeNumbers(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err == nil {
		return nil, errors.New("spec: trailing data after JSON value")
	}
	return v, nil
}

func replaceAt(root any, ptr string, value any) error {
	tokens := strings.Split(strings.TrimPrefix(ptr, "/"), "/")
	node := root
	for i, token := range tokens {
		last := i == len(tokens)-1
		switch n := node.(type) {
		case map[string]any:
			next, ok := n[token]
			if !ok {
				return fmt.Errorf("spec: no node at %s", ptr)
			}
			if last {
				n[token] = value
				return nil
			}
			node = next
		case []any:
			idx, err := strconv.Atoi(token)
			if err != nil || idx < 0 || idx >= len(n) {
				return fmt.Errorf("spec: no node at %s", ptr)
			}
			if last {
				n[idx] = value
				return nil
			}
			node = n[idx]
		default:
			return fmt.Errorf("spec: no node at %s", ptr)
		}
	}
	return nil
}
