package replay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type fixture struct {
	File   string `json:"-"`
	Prompt string `json:"prompt"`
	Turns  []turn `json:"turns"`
}

type turn struct {
	Text      string        `json:"text"`
	ToolCalls []fixtureCall `json:"tool_calls"`
	Expect    []expectation `json:"expect"`
}

type fixtureCall struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type expectation struct {
	IsError  *bool  `json:"is_error"`
	Contains string `json:"contains"`
}

func promptKey(prompt string) string {
	return strings.Join(strings.Fields(strings.ToLower(prompt)), " ")
}

func parseFixture(file string, content []byte) (*fixture, error) {
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.DisallowUnknownFields()
	f := &fixture{File: file}
	if err := dec.Decode(f); err != nil {
		return nil, fmt.Errorf("replay fixture %s: %w", file, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("replay fixture %s: unexpected content after the fixture object", file)
	}
	if err := f.validate(); err != nil {
		return nil, fmt.Errorf("replay fixture %s: %w", file, err)
	}
	return f, nil
}

func (f *fixture) validate() error {
	if promptKey(f.Prompt) == "" {
		return errors.New("prompt is required")
	}
	if len(f.Turns) == 0 {
		return errors.New("turns needs at least one turn")
	}
	for i := range f.Turns {
		previousCalls := 0
		if i > 0 {
			previousCalls = len(f.Turns[i-1].ToolCalls)
			if previousCalls == 0 {
				return fmt.Errorf("turn %d: unreachable because turn %d has no tool_calls and ends the run", i, i-1)
			}
		}
		if err := f.Turns[i].validate(i, previousCalls); err != nil {
			return fmt.Errorf("turn %d: %w", i, err)
		}
	}
	return nil
}

func (t *turn) validate(index, previousCalls int) error {
	if t.Text == "" && len(t.ToolCalls) == 0 {
		return errors.New("needs text or tool_calls")
	}
	for i := range t.ToolCalls {
		if err := t.ToolCalls[i].validate(); err != nil {
			return fmt.Errorf("tool_calls[%d]: %w", i, err)
		}
	}
	if t.Expect == nil {
		return nil
	}
	if index == 0 {
		return errors.New("expect is not allowed on the first turn")
	}
	if len(t.Expect) != previousCalls {
		return fmt.Errorf("expect has %d entries but the previous turn has %d tool calls", len(t.Expect), previousCalls)
	}
	for i, e := range t.Expect {
		if e.IsError == nil {
			return fmt.Errorf("expect[%d]: is_error is required", i)
		}
	}
	return nil
}

func (c *fixtureCall) validate() error {
	if c.Name == "" {
		return errors.New("name is required")
	}
	if len(bytes.TrimSpace(c.Input)) == 0 {
		c.Input = json.RawMessage(`{}`)
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(c.Input, &object); err != nil || object == nil {
		return errors.New("input must be a JSON object")
	}
	return nil
}
