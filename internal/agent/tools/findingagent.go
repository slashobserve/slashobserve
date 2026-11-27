package tools

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/findings"
)

const (
	maxGivenReadsRefused = 1000
	maxGivenUnchecked    = 8
	maxUnverifiedRunes   = 300

	agentFindingProps = `,"unverified":{"type":"array","maxItems":20,"items":{"type":"object","properties":{"kind":{"type":"string","enum":["figure","component","onset","fact","trace_id","trigger","candidate"]},"text":{"type":"string","maxLength":300}},"required":["kind","text"],"additionalProperties":false},"description":"Built-in agent only: what the finding states that the reads do not back."},` +
		`"reads_refused":{"type":"integer","minimum":0,"maximum":1000,"description":"Built-in agent only: reads refused at the call limit."},` +
		`"unchecked":{"type":"array","maxItems":8,"items":{"type":"object","properties":{"datasource":{"type":"string"},"code":{"type":"string"}},"required":["datasource","code"],"additionalProperties":false},"description":"Built-in agent only: reads that did not finish."}`

	agentCauseProps = `,"signals":{"type":"object","properties":{"kinds":{"type":"array","items":{"type":"string","enum":["metrics","logs","traces"]}},"component":{"type":"boolean"},"onset_read":{"type":"boolean"},"onset_fact":{"type":"boolean"},"trigger_read":{"type":"boolean"},"ruled_out":{"type":"boolean"}},"additionalProperties":false},` +
		`"score":{"type":"number","minimum":0,"maximum":1},"band":{"type":"string","enum":["high","medium","low"]},` +
		`"queries":{"type":"array","maxItems":6,"items":{"type":"string","maxLength":400}}`
)

var unverifiedKinds = []string{
	findings.UnverifiedFigure, findings.UnverifiedComponent, findings.UnverifiedOnset, findings.UnverifiedFact,
	findings.UnverifiedTraceID, findings.UnverifiedTrigger, findings.UnverifiedCandidate,
}

type givenUnchecked struct {
	Datasource string `json:"datasource"`
	Code       string `json:"code"`
}

type agentFinding struct {
	Unverified   *[]findings.Unverified `json:"unverified"`
	ReadsRefused *int                   `json:"reads_refused"`
	Unchecked    *[]givenUnchecked      `json:"unchecked"`
}

type agentFindingFields struct {
	given        bool
	unverified   []findings.Unverified
	hasUnverify  bool
	readsRefused *int
	unchecked    *[]Unchecked
	causes       []findings.CauseGiven
}

func agentFindingOf(in agentFinding, causes []findings.CauseInput, run Run) agentFindingFields {
	if !run.BuiltIn {
		return agentFindingFields{}
	}
	out := agentFindingFields{given: true}
	if in.Unverified != nil {
		out.hasUnverify = true
		for _, item := range *in.Unverified {
			text := strings.TrimSpace(strings.ToValidUTF8(item.Text, ""))
			if utf8.RuneCountInString(text) > maxUnverifiedRunes {
				text = string([]rune(text)[:maxUnverifiedRunes])
			}
			if text != "" && slices.Contains(unverifiedKinds, item.Kind) && len(out.unverified) < findings.MaxUnverified {
				out.unverified = append(out.unverified, findings.Unverified{Kind: item.Kind, Text: text})
			}
		}
	}
	if in.ReadsRefused != nil {
		n := min(max(*in.ReadsRefused, 0), maxGivenReadsRefused)
		out.readsRefused = &n
	}
	if in.Unchecked != nil {
		list := []Unchecked{}
		for _, u := range *in.Unchecked {
			if u.Datasource != "" && u.Code != "" && len(list) < maxGivenUnchecked {
				list = append(list, Unchecked{Datasource: clip(u.Datasource, maxStepNameBytes), Code: clip(u.Code, maxStepNameBytes)})
			}
		}
		out.unchecked = &list
	}
	for _, c := range causes {
		out.causes = append(out.causes, c.CauseGiven)
	}
	return out
}

func (f agentFindingFields) into(ctx context.Context) context.Context {
	if f.readsRefused != nil {
		ctx = WithReadsRefused(ctx, *f.readsRefused)
	}
	if f.unchecked != nil {
		ctx = WithUnchecked(ctx, *f.unchecked)
	}
	return ctx
}

func (f agentFindingFields) scoring(s *findings.Structured) ([]findings.Signals, []findings.GivenScore, bool) {
	if !f.given || len(f.causes) == 0 {
		return nil, nil, false
	}
	merged := make([]findings.Signals, len(s.Causes))
	scored := false
	given := make([]findings.GivenScore, len(s.Causes))
	for i := range s.Causes {
		if i >= len(f.causes) {
			break
		}
		c := f.causes[i]
		if c.Signals != nil {
			merged[i], scored = c.Signals.Signals(), true
		}
		if c.Score != nil {
			given[i], scored = findings.GivenScore{Score: c.Score, Band: c.Band}, true
		}
		if len(c.Queries) > 0 {
			s.GiveQueries(i, c.Queries)
		}
	}
	return merged, given, scored
}
