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
	countClaim = regexp.MustCompile(`(?i)\b\d[\d,]*(?:\.\d+)?\s?(?:%|ms\b|errors?\b|failures?\b|spans?\b|requests?\b|calls?\b|traces?\b|lines?\b|logs?\b|rows?\b|series\b|services?\b|pods?\b)`)
	causeTitle = regexp.MustCompile(`(?i)\b(?:caus(?:e|es|ed|ing)|due to|because of|tied to|linked to|driven by|blamed on)\b`)
	hedgedText = regexp.MustCompile(`(?i)\b(?:most likely|probably|possibly|unconfirmed|not confirmed|(?:could not|cannot|can't) confirm|could not be confirmed|appears to|seems to|may have|might have)\b`)
)

const (
	faultNoEvidence = "the finding gives counts but no evidence panel: add 1 to 4 evidence panels whose queries you ran and that returned data, so the numbers can be checked; if no query returned data, send it again unchanged"
	faultTitle      = "the title names a cause, but the text calls it likely or unconfirmed: title the symptom you saw instead, such as the service and what failed, or send it again unchanged when you did confirm the cause"
)

func (s *State) noteQueried(c Call, res tools.Result) {
	if tools.Phase(c.Name) == tools.PhaseQuery && !res.IsError {
		s.Answer.Queried = true
	}
}

func (s *State) takeAnswerFaults(input json.RawMessage) string {
	var in struct {
		Title    string            `json:"title"`
		Markdown string            `json:"markdown"`
		Evidence []json.RawMessage `json:"evidence"`
	}
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	var faults []string
	if !s.Answer.EvidenceAsked && s.Answer.Queried && len(in.Evidence) == 0 && countClaim.MatchString(in.Markdown) {
		s.Answer.EvidenceAsked = true
		faults = append(faults, faultNoEvidence)
	}
	if !s.Answer.TitleAsked && causeTitle.MatchString(in.Title) && hedgedText.MatchString(in.Markdown) {
		s.Answer.TitleAsked = true
		faults = append(faults, faultTitle)
	}
	return strings.Join(faults, "; and ")
}
