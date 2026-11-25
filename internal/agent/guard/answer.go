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
}

var (
	countClaim = regexp.MustCompile(`(?i)\b\d[\d,]*(?:\.\d+)?\s?(?:%|ms\b|errors?\b|failures?\b|spans?\b|requests?\b|calls?\b|traces?\b|lines?\b|logs?\b|rows?\b|series\b|services?\b|pods?\b)`)
)

const (
	faultNoEvidence = "the finding gives counts but no evidence panel: add 1 to 4 evidence panels whose queries you ran and that returned data, so the numbers can be checked; if no query returned data, send it again unchanged"
)

func (s *State) noteQueried(c Call, res tools.Result) {
	if tools.Phase(c.Name) == tools.PhaseQuery && !res.IsError {
		s.Answer.Queried = true
	}
}

func (s *State) takeAnswerFaults(input json.RawMessage) string {
	var in struct {
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
	return strings.Join(faults, "; and ")
}
