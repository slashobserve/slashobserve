package health

import (
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type Kind string

const (
	KindNoData         Kind = "no_data"
	KindQueryFailed    Kind = "query_failed"
	KindVariableFailed Kind = "variable_failed"
	KindDuplicateTitle Kind = "duplicate_title"
	KindUnopened       Kind = "unopened"
)

var Kinds = []Kind{KindNoData, KindQueryFailed, KindVariableFailed, KindDuplicateTitle, KindUnopened}

func (k Kind) checked() bool {
	return k == KindNoData || k == KindQueryFailed || k == KindVariableFailed
}

const unopenedAfter = 90 * 24 * time.Hour

const variablesPointer = "/variables/"

type Problem struct {
	Kind       Kind   `json:"kind"`
	PanelID    string `json:"panel_id,omitempty"`
	PanelTitle string `json:"panel_title,omitempty"`
	Variable   string `json:"variable,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
}

func classify(d *spec.Dashboard, issues []spec.Issue) []Problem {
	panels := d.AllPanels()
	problems := make([]Problem, 0, len(issues))
	seen := map[Problem]bool{}
	for _, issue := range issues {
		p := locate(d, panels, issue.Path)
		p.Code = string(issue.Code)
		switch {
		case p.Variable != "":
			p.Kind = KindVariableFailed
		case issue.Code == queryproxy.CodeQueryEmpty:
			p.Kind = KindNoData
		default:
			p.Kind = KindQueryFailed
		}
		key := Problem{Kind: p.Kind, PanelID: p.PanelID, Variable: p.Variable}
		if seen[key] {
			continue
		}
		seen[key] = true
		p.Message = issue.Message
		problems = append(problems, p)
	}
	return problems
}

func locate(d *spec.Dashboard, panels []spec.PanelEntry, path string) Problem {
	if rest, ok := strings.CutPrefix(path, variablesPointer); ok {
		index, _, _ := strings.Cut(rest, "/")
		if i, err := strconv.Atoi(index); err == nil && i >= 0 && i < len(d.Variables) {
			return Problem{Variable: d.Variables[i].Name}
		}
		return Problem{}
	}
	var best *spec.PanelEntry
	for i := range panels {
		e := &panels[i]
		if path != e.Pointer && !strings.HasPrefix(path, e.Pointer+"/") {
			continue
		}
		if best == nil || len(e.Pointer) > len(best.Pointer) {
			best = e
		}
	}
	if best == nil {
		return Problem{}
	}
	return Problem{PanelID: best.Panel.ID, PanelTitle: best.Panel.Title}
}

func unopened(lastView *time.Time, created, now time.Time) bool {
	seen := created
	if lastView != nil {
		seen = *lastView
	}
	return seen.Before(unopenedBefore(now))
}

func unopenedBefore(now time.Time) time.Time {
	return now.Add(-unopenedAfter)
}
