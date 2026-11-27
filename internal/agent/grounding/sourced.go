package grounding

import (
	"encoding/json"
	"regexp"
	"slices"
)

const (
	maxSourcedOfRead = 500
	maxSourced       = 2000
)

var sourceName = regexp.MustCompile(`(?:^|[^A-Za-z0-9])([A-Za-z][A-Za-z0-9_.-]*[A-Za-z0-9])`)

// MCP source text gives names only, never a number, a time or a trace id: the
// server cannot vouch for what an outside tool computed.
func sourceNames(text string) []string {
	var out []string
	for _, m := range sourceName.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if len(name) < minNameRunes || len(name) > maxNameBytes || slices.Contains(out, name) {
			continue
		}
		out = append(out, name)
		if len(out) == maxSourcedOfRead {
			break
		}
	}
	return out
}

func (r *Record) AddSourced(content []byte) {
	if r == nil {
		return
	}
	var result struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &result) != nil {
		return
	}
	for _, name := range sourceNames(result.Text) {
		if i := slices.Index(r.Sourced, name); i >= 0 {
			r.Sourced = slices.Delete(r.Sourced, i, i+1)
		}
		r.Sourced = append(r.Sourced, name)
	}
	if over := len(r.Sourced) - maxSourced; over > 0 {
		r.Sourced = slices.Delete(r.Sourced, 0, over)
	}
}
