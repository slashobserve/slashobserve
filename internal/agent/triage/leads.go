package triage

import (
	"fmt"
	"slices"
	"strings"
)

const (
	MaxLeads = 5

	maxLeadRows  = 2
	maxLeadRunes = 160
)

type Lead struct {
	ID      string `json:"id"`
	Service string `json:"service"`
	Text    string `json:"text"`
	Next    string `json:"next"`
}

func Leads(candidates []Candidate) []Lead {
	var out []Lead
	for _, c := range candidates[:min(len(candidates), MaxLeads)] {
		var moved, signals []string
		for _, row := range c.Rows {
			if len(moved) < maxLeadRows {
				moved = append(moved, fmt.Sprintf("%s %s from %s (%s)", row.Signal, figure(row.Window, row.Unit), figure(row.Baseline, row.Unit), short(row.Rung)))
			}
			if !slices.Contains(signals, row.Signal) {
				signals = append(signals, row.Signal)
			}
		}
		text := fmt.Sprintf("%s: %s; onset %s", c.Service, strings.Join(moved, "; "), onsetText(c.Onset))
		next := fmt.Sprintf("Read %s %s before and after the onset, then close it as cause, knock_on, background, transient or unrelated", c.Service, strings.Join(signals, " and "))
		out = append(out, Lead{ID: c.ID, Service: c.Service, Text: clipRunes(text, maxLeadRunes), Next: clipRunes(next, maxLeadRunes)})
	}
	return out
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}
