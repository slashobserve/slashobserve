package tools

import (
	"fmt"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const codePlaceholderNotAllowed spec.IssueCode = "placeholder_not_allowed"

func placeholderIssues(next, parent *spec.Dashboard) []spec.Issue {
	var kept map[string]*spec.Panel
	if parent != nil {
		kept = placeholdersByID(parent)
	}
	var issues []spec.Issue
	for _, e := range next.AllPanels() {
		if e.Panel.Type != spec.Placeholder {
			continue
		}
		switch before, ok := kept[e.Panel.ID]; {
		case parent == nil:
			issues = append(issues, spec.Issue{
				Path:    e.Pointer,
				Code:    codePlaceholderNotAllowed,
				Message: "the agent cannot create placeholder panels; remove this panel",
			})
		case !ok || !samePlaceholder(before, e.Panel):
			issues = append(issues, spec.Issue{
				Path:    e.Pointer,
				Code:    codePlaceholderNotAllowed,
				Message: fmt.Sprintf("placeholder %q can only be moved or deleted; keep its id, title, description, reason and sourceType", e.Panel.ID),
			})
		}
	}
	return issues
}

func hasPlaceholder(d *spec.Dashboard) bool {
	for _, e := range d.AllPanels() {
		if e.Panel.Type == spec.Placeholder {
			return true
		}
	}
	return false
}

func placeholdersByID(d *spec.Dashboard) map[string]*spec.Panel {
	byID := map[string]*spec.Panel{}
	for _, e := range d.AllPanels() {
		if e.Panel.Type == spec.Placeholder {
			byID[e.Panel.ID] = e.Panel
		}
	}
	return byID
}

func samePlaceholder(a, b *spec.Panel) bool {
	return a.Title == b.Title && a.Description == b.Description && a.Reason == b.Reason && a.SourceType == b.SourceType
}
