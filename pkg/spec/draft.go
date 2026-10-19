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
	if issues := byteIssues(panel, strings.Count(entry.Pointer, "/")); len(issues) > 0 {
		for i := range issues {
			issues[i].Path = entry.Pointer + issues[i].Path
		}
		return nil, newValidationError(issues)
	}
	dec := json.NewDecoder(bytes.NewReader(panel))
	dec.UseNumber()
	var draft any
	if err := dec.Decode(&draft); err != nil {
		return nil, newValidationError([]Issue{{Path: entry.Pointer, Code: CodeSchema, Message: "the panel is not valid JSON"}})
	}
	tree := decodeTree(d)
	if err := replaceAt(tree, entry.Pointer, draft); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(tree); err != nil {
		return nil, err
	}
	return Parse(out.Bytes())
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
