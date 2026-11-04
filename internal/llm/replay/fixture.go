package replay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type fixture struct {
	File             string             `json:"-"`
	Prompt           string             `json:"prompt"`
	Turns            []turn             `json:"turns"`
	Resumes          map[string]segment `json:"resumes"`
	ResumeAfterSaves bool               `json:"resume_after_saves"`
}

type segment struct {
	Turns []turn `json:"turns"`
}

type turn struct {
	Text      string        `json:"text"`
	ToolCalls []fixtureCall `json:"tool_calls"`
	Expect    []expectation `json:"expect"`
	Fault     *fault        `json:"fault"`
}

type fault struct {
	Kind    string `json:"kind"`
	DelayMS int    `json:"delay_ms"`
}

const (
	faultRateLimited = "rate_limited"
	faultServerError = "server_error"
	faultCallTimeout = "call_timeout"
	faultMaxTokens   = "max_tokens"
	faultRefusal     = "refusal"
	faultDelay       = "delay"
)

var saveTools = []string{"create_dashboard", "update_dashboard", "update_panels"}

var faultKinds = []string{faultRateLimited, faultServerError, faultCallTimeout, faultMaxTokens, faultRefusal, faultDelay}

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
	if err := validateTurns("turns", f.Turns); err != nil {
		return err
	}
	if f.ResumeAfterSaves {
		if err := f.validateResumeAfterSaves(); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(f.Resumes))
	for key := range f.Resumes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if n, err := strconv.Atoi(key); err != nil || n < 1 || strconv.Itoa(n) != key {
			return fmt.Errorf("resumes key %q is not a positive integer", key)
		}
		if err := validateTurns(fmt.Sprintf("resumes.%q.turns", key), f.Resumes[key].Turns); err != nil {
			return err
		}
	}
	return nil
}

func (f *fixture) validateResumeAfterSaves() error {
	if len(f.Resumes) > 0 {
		return errors.New("resume_after_saves cannot be combined with resumes")
	}
	for i, t := range f.Turns {
		if saves := t.saveCalls(); saves > 1 {
			return fmt.Errorf("turns: turn %d has %d save calls; resume_after_saves needs at most one per turn", i, saves)
		}
		if i > 0 && t.Expect != nil && f.Turns[i-1].saveCalls() > 0 {
			return fmt.Errorf("turns: turn %d: expect is not allowed on the turn after a save when resume_after_saves is set, because a resume starts there", i)
		}
	}
	return nil
}

func (t *turn) saveCalls() int {
	n := 0
	for _, c := range t.ToolCalls {
		if slices.Contains(saveTools, c.Name) {
			n++
		}
	}
	return n
}

func (f *fixture) turnsFor(buildSaves int) ([]turn, bool) {
	if buildSaves == 0 {
		return f.Turns, true
	}
	if f.ResumeAfterSaves {
		return f.turnsAfterSave(buildSaves)
	}
	seg, ok := f.Resumes[strconv.Itoa(buildSaves)]
	return seg.Turns, ok
}

func (f *fixture) turnsAfterSave(k int) ([]turn, bool) {
	seen := 0
	for i, t := range f.Turns {
		if t.saveCalls() == 0 {
			continue
		}
		if seen++; seen == k {
			return f.Turns[i+1:], true
		}
	}
	return nil, false
}

func validateTurns(label string, turns []turn) error {
	if len(turns) == 0 {
		return fmt.Errorf("%s needs at least one turn", label)
	}
	for i := range turns {
		previousCalls := 0
		if i > 0 {
			previousCalls = len(turns[i-1].ToolCalls)
			if previousCalls == 0 {
				return fmt.Errorf("%s: turn %d: unreachable because turn %d has no tool_calls and ends the run", label, i, i-1)
			}
		}
		if err := turns[i].validate(i, previousCalls); err != nil {
			return fmt.Errorf("%s: turn %d: %w", label, i, err)
		}
	}
	return nil
}

func (t *turn) validate(index, previousCalls int) error {
	if t.Fault != nil {
		if err := t.Fault.validate(); err != nil {
			return fmt.Errorf("fault: %w", err)
		}
	} else if t.Text == "" && len(t.ToolCalls) == 0 {
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

func (f *fault) validate() error {
	if !slices.Contains(faultKinds, f.Kind) {
		return fmt.Errorf("kind %q is not one of %s", f.Kind, strings.Join(faultKinds, ", "))
	}
	if f.DelayMS < 0 {
		return errors.New("delay_ms must not be negative")
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
