package guard

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

type Answer struct {
	Queried       bool `json:"queried,omitempty"`
	EvidenceAsked bool `json:"evidence_asked,omitempty"`
	TitleAsked    bool `json:"title_asked,omitempty"`
}

var (
	countClaim  = regexp.MustCompile(`(?i)\b\d[\d,]*(?:\.\d+)?(?:(?-i:[kM])\b|x\b|\s?(?:%|ms\b|s\b|sec\b|seconds?\b|rps\b|req/s\b|errors?\b|failures?\b|spans?\b|requests?\b|calls?\b|traces?\b|lines?\b|logs?\b|rows?\b|series\b|services?\b|pods?\b))`)
	causeTitle  = regexp.MustCompile(`(?i)\b(?:caused by|causes|due to|because of|tied to|linked to|driven by|blamed on)\b`)
	hedgedText  = regexp.MustCompile(`(?i)\b(?:most likely|probably|possibly|unconfirmed|not confirmed|(?:could not|cannot|can't) confirm|could not be confirmed|appears to|seems to|may have|might have)\b`)
	sentenceEnd = regexp.MustCompile(`[.!?]+\s+|\n+`)
	wordToken   = regexp.MustCompile(`[A-Za-z0-9]+`)
	wordPart    = regexp.MustCompile(`[A-Z]?[a-z]+|[A-Z]+|[0-9]+`)
)

var plainWords = map[string]bool{
	"errors": true, "error": true, "failures": true, "failure": true, "failing": true, "service": true, "services": true,
	"requests": true, "latency": true, "with": true, "from": true, "that": true, "this": true, "after": true, "during": true,
}

var dataReads = map[string]bool{
	"trace_stats": true, "sample_rows": true, "get_trace": true, "compare_traces": true,
	"log_context": true, "log_patterns": true, "log_facets": true, "logs_for_trace": true, "call_mcp_tool": true,
}

const sentBackStep = "The answer was sent back for a fix: "

type fault struct{ model, people string }

const (
	faultNoEvidence = "the finding gives counts but no evidence panel: add 1 to 4 evidence panels whose queries you ran and that returned data, so the numbers can be checked; if no query returned data, send it again unchanged"
	faultTitle      = "the title names a cause, but the text calls it likely or unconfirmed: title the symptom you saw instead, such as the service and what failed, or send it again unchanged when you did confirm the cause"
)

func ReadsData(name string) bool {
	return tools.Phase(name) == tools.PhaseQuery || dataReads[name]
}

func (s *State) noteQueried(c Call, res tools.Result) {
	if !res.IsError && ReadsData(c.Name) {
		s.Answer.Queried = true
	}
}

func (s *State) unbackedFinding(c Call) []fault {
	input := c.Input
	var faults []fault
	if f, ok := s.unbackedAbsence(input); ok {
		s.Absence.Asked = true
		faults = append(faults, f)
	}
	faults = append(faults, s.takeAnswerFaults(input)...)
	if f, ok := s.ledgerBehind(input); ok {
		faults = append(faults, f)
	}
	if f, ok := s.groundingBehind(c); ok {
		faults = append(faults, f)
	}
	return faults
}

func (s *State) takeAnswerFaults(input json.RawMessage) []fault {
	var in struct {
		Title    string            `json:"title"`
		Markdown string            `json:"markdown"`
		Evidence []json.RawMessage `json:"evidence"`
	}
	if json.Unmarshal(input, &in) != nil {
		return nil
	}
	var faults []fault
	if !s.Answer.EvidenceAsked && s.Answer.Queried && len(in.Evidence) == 0 && countClaim.MatchString(in.Markdown) {
		s.Answer.EvidenceAsked = true
		faults = append(faults, fault{faultNoEvidence, "it gave counts without an evidence panel"})
	}
	if !s.Answer.TitleAsked && doubtsTitleCause(in.Title, in.Markdown) {
		s.Answer.TitleAsked = true
		faults = append(faults, fault{faultTitle, "its title named a cause that the text calls unconfirmed"})
	}
	return faults
}

func doubtsTitleCause(title, markdown string) bool {
	loc := causeTitle.FindStringIndex(title)
	if loc == nil {
		return false
	}
	cause := wordsOf(title[loc[1]:])
	if len(cause) == 0 {
		return false
	}
	for _, sentence := range sentenceEnd.Split(markdown, -1) {
		if !hedgedText.MatchString(sentence) {
			continue
		}
		for w := range wordsOf(sentence) {
			if cause[w] {
				return true
			}
		}
	}
	return false
}

func wordsOf(text string) map[string]bool {
	words := map[string]bool{}
	add := func(w string) {
		if w = strings.ToLower(w); len(w) >= 4 && !plainWords[w] {
			words[w] = true
		}
	}
	for _, token := range wordToken.FindAllString(text, -1) {
		add(token)
		for _, part := range wordPart.FindAllString(token, -1) {
			add(part)
		}
	}
	return words
}
