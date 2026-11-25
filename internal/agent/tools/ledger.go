package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	nameNoteHypotheses = "note_hypotheses"

	MaxHypotheses        = 6
	maxHypothesisID      = 16
	maxHypothesisText    = 160
	maxHypothesisEvLines = 3
	maxHypothesisEvLine  = 200
	maxHypothesisNext    = 160
	MaxLedgerBytes       = 4 << 10

	descNoteHypotheses = "Write down your working list of candidate causes, with what each read showed. Send the whole list every time, at most 6 hypotheses, in the same call as your next reads. A hypothesis is open, supported or ruled_out; mark one ruled_out or supported only with an evidence line saying what you read and what it showed. The server keeps the latest list and gives it back if your conversation is shortened. It changes nothing and is not a read."

	schemaNoteHypotheses = `{"type":"object","properties":{"hypotheses":{"type":"array","minItems":1,"maxItems":6,"description":"The whole ledger, replacing the last one.","items":{"type":"object","properties":{` +
		`"id":{"type":"string","maxLength":16,"description":"A short id such as h1 or db-pool, kept the same across updates."},` +
		`"text":{"type":"string","maxLength":160,"description":"One sentence saying what may have failed."},` +
		`"status":{"type":"string","enum":["open","supported","ruled_out"]},` +
		`"evidence":{"type":"array","maxItems":3,"items":{"type":"string","maxLength":200},"description":"What you read and what it showed; required for supported and ruled_out."},` +
		`"next_check":{"type":"string","maxLength":160,"description":"For an open hypothesis: the read that would settle it."}` +
		`},"required":["id","text","status"],"additionalProperties":false}}},"required":["hypotheses"],"additionalProperties":false}`
)

var hypothesisID = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func init() { registerEntries((*Registry).ledgerEntries) }

func (r *Registry) ledgerEntries() []entry {
	if !r.deps.LedgerTool {
		return nil
	}
	return []entry{
		{Tool{Name: nameNoteHypotheses, Description: descNoteHypotheses, InputSchema: json.RawMessage(schemaNoteHypotheses), AgentOnly: true, ReadOnly: true, OnlyIn: ModeInvestigate, MinRole: auth.RoleViewer}, r.noteHypotheses},
	}
}

type hypothesisInput struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Status   string   `json:"status"`
	Evidence []string `json:"evidence"`
	Next     string   `json:"next_check"`
}

type ledgerInput struct {
	Hypotheses []hypothesisInput `json:"hypotheses"`
}

type ledgerOut struct {
	Hypotheses int `json:"hypotheses"`
	Open       int `json:"open"`
	Supported  int `json:"supported"`
	RuledOut   int `json:"ruled_out"`
}

func (r *Registry) noteHypotheses(_ context.Context, input json.RawMessage) (Result, error) {
	var in ledgerInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	ledger, problem := checkLedger(in.Hypotheses)
	if problem != "" {
		return invalid("%s", problem), nil
	}
	res := success(LedgerStep(ledger), countLedger(ledger))
	res.Ledger = ledger
	return res, nil
}

func countLedger(ledger []revisions.Hypothesis) ledgerOut {
	out := ledgerOut{Hypotheses: len(ledger)}
	for _, h := range ledger {
		switch h.Status {
		case revisions.HypothesisOpen:
			out.Open++
		case revisions.HypothesisSupport:
			out.Supported++
		case revisions.HypothesisRuledOut:
			out.RuledOut++
		}
	}
	return out
}

func LedgerStep(ledger []revisions.Hypothesis) string {
	c := countLedger(ledger)
	return fmt.Sprintf("%d hypotheses: %d supported, %d ruled out, %d open", c.Hypotheses, c.Supported, c.RuledOut, c.Open)
}

func checkLedger(in []hypothesisInput) ([]revisions.Hypothesis, string) {
	if len(in) == 0 || len(in) > MaxHypotheses {
		return nil, fmt.Sprintf("send 1 to %d hypotheses, the whole ledger each time", MaxHypotheses)
	}
	ledger := make([]revisions.Hypothesis, 0, len(in))
	seen := map[string]bool{}
	for i, h := range in {
		item, problem := checkHypothesis(i, h)
		if problem != "" {
			return nil, problem
		}
		if seen[item.ID] {
			return nil, fmt.Sprintf("hypotheses[%d].id %q is used twice", i, item.ID)
		}
		seen[item.ID] = true
		ledger = append(ledger, item)
	}
	size := 0
	for _, h := range ledger {
		size += h.Size()
	}
	if size > MaxLedgerBytes {
		return nil, fmt.Sprintf("the ledger is %d bytes, over the %d byte limit; shorten the evidence lines or drop a ruled out hypothesis's detail", size, MaxLedgerBytes)
	}
	return ledger, ""
}

func checkHypothesis(i int, in hypothesisInput) (revisions.Hypothesis, string) {
	h := revisions.Hypothesis{ID: strings.TrimSpace(in.ID), Text: oneLine(in.Text), Status: in.Status}
	switch {
	case h.ID == "" || utf8.RuneCountInString(h.ID) > maxHypothesisID || !hypothesisID.MatchString(h.ID):
		return h, fmt.Sprintf("hypotheses[%d].id must be 1 to %d characters of letters, digits, - _ or .", i, maxHypothesisID)
	case h.Text == "" || utf8.RuneCountInString(h.Text) > maxHypothesisText:
		return h, fmt.Sprintf("hypotheses[%d].text must be one sentence of at most %d characters", i, maxHypothesisText)
	case h.Status != revisions.HypothesisOpen && h.Status != revisions.HypothesisSupport && h.Status != revisions.HypothesisRuledOut:
		return h, fmt.Sprintf("hypotheses[%d].status must be open, supported or ruled_out", i)
	}
	evidence, problem := checkEvidence(i, in.Evidence)
	if problem != "" {
		return h, problem
	}
	h.Evidence = evidence
	if h.Status != revisions.HypothesisOpen && len(h.Evidence) == 0 {
		return h, fmt.Sprintf("hypotheses[%d] is %s with no evidence line; say what you read and what it showed, or keep it open", i, h.Status)
	}
	if h.Status == revisions.HypothesisOpen {
		h.Next = oneLine(in.Next)
		if utf8.RuneCountInString(h.Next) > maxHypothesisNext {
			return h, fmt.Sprintf("hypotheses[%d].next_check must be at most %d characters", i, maxHypothesisNext)
		}
	}
	return h, ""
}

func checkEvidence(i int, in []string) ([]string, string) {
	if len(in) > maxHypothesisEvLines {
		return nil, fmt.Sprintf("hypotheses[%d].evidence takes at most %d lines", i, maxHypothesisEvLines)
	}
	var lines []string
	for j, line := range in {
		line = oneLine(line)
		if line == "" || utf8.RuneCountInString(line) > maxHypothesisEvLine {
			return nil, fmt.Sprintf("hypotheses[%d].evidence[%d] must be 1 to %d characters", i, j, maxHypothesisEvLine)
		}
		lines = append(lines, line)
	}
	return lines, ""
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
