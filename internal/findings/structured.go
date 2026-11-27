package findings

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxRuledOut  = 5
	MaxNextSteps = 3

	MaxComponentRunes = 120
	MaxSentenceRunes  = 300
	MaxLineRunes      = 200
)

var (
	RootCauseKinds = []string{"deploy", "config_or_flag", "dependency", "resource", "data", "traffic", "infrastructure", "unknown"}
	Confidences    = []string{"high", "medium", "low"}
)

type RootCause struct {
	Component, Kind, Summary string
}

type Structured struct {
	RootCause  *RootCause
	Onset      *time.Time
	Impact     string
	Confidence string
	RuledOut   []string
	NextSteps  []string
	NoIncident bool
	Causes     []Cause
}

func (s Structured) Empty() bool {
	return s.RootCause == nil && len(s.Causes) == 0 && s.Onset == nil && s.Impact == "" && s.Confidence == "" &&
		len(s.RuledOut) == 0 && len(s.NextSteps) == 0 && !s.NoIncident
}

type RootCauseInput struct {
	Component string `json:"component"`
	Kind      string `json:"kind"`
	Summary   string `json:"summary"`
}

type StructuredInput struct {
	RootCause  *RootCauseInput `json:"root_cause"`
	Onset      string          `json:"onset"`
	Impact     string          `json:"impact"`
	Confidence string          `json:"confidence"`
	RuledOut   []string        `json:"ruled_out"`
	NextSteps  []string        `json:"next_steps"`
	NoIncident bool            `json:"no_incident"`
	Causes     []CauseInput    `json:"causes"`

	LegacyFacts []string `json:"-"`
}

func (in StructuredInput) Provided() bool {
	return in.RootCause != nil || len(in.Causes) > 0 || in.Onset != "" || in.Impact != "" || in.Confidence != "" ||
		len(in.RuledOut) > 0 || len(in.NextSteps) > 0 || in.NoIncident
}

func (in StructuredInput) Check() (Structured, error) {
	var out Structured
	var rootCause *RootCause
	if in.RootCause != nil {
		cause, err := checkRootCause(*in.RootCause)
		if err != nil {
			return Structured{}, err
		}
		rootCause = &cause
	}
	if in.NoIncident && rootCause != nil {
		return Structured{}, fmt.Errorf("no_incident is true, so root_cause must be left out")
	}
	out.NoIncident = in.NoIncident
	var err error
	if out.Onset, err = onsetOf("onset", in.Onset); err != nil {
		return Structured{}, err
	}
	if out.Impact, err = sentence("impact", in.Impact, MaxSentenceRunes); err != nil {
		return Structured{}, err
	}
	if out.Confidence = strings.TrimSpace(in.Confidence); out.Confidence != "" && !slices.Contains(Confidences, out.Confidence) {
		return Structured{}, fmt.Errorf("confidence must be one of %s", strings.Join(Confidences, ", "))
	}
	if out.RuledOut, err = lines("ruled_out", in.RuledOut, MaxRuledOut); err != nil {
		return Structured{}, err
	}
	if out.NextSteps, err = lines("next_steps", in.NextSteps, MaxNextSteps); err != nil {
		return Structured{}, err
	}
	if out.Causes, err = in.checkCauses(rootCause); err != nil {
		return Structured{}, err
	}
	out.settle()
	return out, nil
}

func checkRootCause(in RootCauseInput) (RootCause, error) {
	component, err := sentence("root_cause.component", in.Component, MaxComponentRunes)
	if err != nil {
		return RootCause{}, err
	}
	summary, err := sentence("root_cause.summary", in.Summary, MaxSentenceRunes)
	if err != nil {
		return RootCause{}, err
	}
	kind := strings.TrimSpace(in.Kind)
	switch {
	case component == "" || kind == "" || summary == "":
		return RootCause{}, fmt.Errorf("root_cause needs component, kind and summary together; leave root_cause out when no cause was found")
	case !slices.Contains(RootCauseKinds, kind):
		return RootCause{}, fmt.Errorf("root_cause.kind must be one of %s", strings.Join(RootCauseKinds, ", "))
	}
	return RootCause{Component: component, Kind: kind, Summary: summary}, nil
}

func sentence(name, raw string, maxRunes int) (string, error) {
	text := strings.TrimSpace(raw)
	switch {
	case utf8.RuneCountInString(text) > maxRunes:
		return "", fmt.Errorf("%s takes at most %d characters; shorten it", name, maxRunes)
	case strings.ContainsFunc(text, unicode.IsControl):
		return "", fmt.Errorf("%s is one plain line with no line breaks", name)
	}
	return text, nil
}

func lines(name string, raw []string, most int) ([]string, error) {
	if len(raw) > most {
		return nil, fmt.Errorf("%s takes at most %d lines", name, most)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		line, err := sentence(name, item, MaxLineRunes)
		if err != nil {
			return nil, err
		}
		if line == "" {
			return nil, fmt.Errorf("%s takes no empty lines", name)
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
