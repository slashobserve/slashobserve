package guard

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

const (
	toolListMetrics     = "list_metrics"
	toolListAnnotations = "list_annotations"
)

type Absence struct {
	ScopedMetrics bool `json:"scoped_metrics,omitempty"`
	Annotations   bool `json:"annotations,omitempty"`
	Asked         bool `json:"asked,omitempty"`
}

var (
	noMetricsClaim = regexp.MustCompile(`(?i)\b(?:no|not any|without)\b[^.;:\n]{0,40}\bmetrics?\b|\bmetrics?\b[^.;:\n]{0,40}\b(?:do not|don't|does not|doesn't) exist|\bmetrics?\b[^.;:\n]{0,20}\b(?:are|is) missing`)
	noDeployClaim  = regexp.MustCompile(`(?i)\b(?:no|not any|without)\b[^.;:\n]{0,30}\b(?:deploy(?:s|ment|ments)?|releases?|rollouts?)\b|\bdeploy(?:s|ment|ments)?\b[^.;:\n]{0,30}\b(?:not found|none|do not exist|does not exist)`)
)

func (s *State) noteAbsenceRead(c Call, res tools.Result) {
	switch c.Name {
	case toolListMetrics:
		var in struct {
			Scope string `json:"scope"`
		}
		if json.Unmarshal(c.Input, &in) == nil && strings.TrimSpace(in.Scope) != "" {
			s.Absence.ScopedMetrics = true
		}
	case toolListAnnotations:
		if !res.IsError {
			s.Absence.Annotations = true
		}
	}
}

func (s *State) unbackedAbsence(input json.RawMessage) string {
	if s.Absence.Asked {
		return ""
	}
	var in struct {
		Markdown string `json:"markdown"`
	}
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	switch {
	case !s.Absence.ScopedMetrics && noMetricsClaim.MatchString(in.Markdown):
		return "the finding says a metric is missing, but no list_metrics call in this run set scope: a service, job or pod is a label value, so list metrics with scope set to its selector, such as {service=\"checkout\"}, before saying so, or drop the claim"
	case !s.Absence.Annotations && noDeployClaim.MatchString(in.Markdown) && s.Mode == tools.ModeInvestigate:
		return "the finding says there was no deploy, but this run never read list_annotations, where deploy markers live: read it with tag deploy before saying so, or drop the claim"
	case !s.Absence.Annotations && noDeployClaim.MatchString(in.Markdown):
		return "the finding says there was no deploy, but this run cannot read annotations, where deploy markers live: say deploys were not checked here and that an investigation reads them, instead of saying none exists"
	}
	return ""
}
