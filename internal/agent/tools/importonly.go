package tools

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	codePlaceholderNotAllowed spec.IssueCode = "placeholder_not_allowed"
	codeHTMLNotAllowed        spec.IssueCode = "html_not_allowed"
	codeActionNotAllowed      spec.IssueCode = "action_not_allowed"
)

var htmlMarkup = regexp.MustCompile(`<(?:[A-Za-z][A-Za-z0-9-]*(?:[\s/>]|$)|/[A-Za-z]|!--|![A-Za-z\[]|\?)`)

func containsHTML(s string) bool {
	return htmlMarkup.MatchString(s)
}

func importOnlyIssues(next, parent *spec.Dashboard) []spec.Issue {
	var before map[string]*spec.Panel
	if parent != nil {
		before = panelsByID(parent)
	}
	var issues []spec.Issue
	for _, e := range next.AllPanels() {
		issues = append(issues, panelImportOnlyIssues(e, before, parent != nil)...)
	}
	return issues
}

func panelImportOnlyIssues(e spec.PanelEntry, before map[string]*spec.Panel, hasParent bool) []spec.Issue {
	previous, existed := before[e.Panel.ID]
	if e.Panel.Type == spec.Placeholder {
		return placeholderViolations(e, previous, existed && previous.Type == spec.Placeholder, hasParent)
	}
	var issues []spec.Issue
	if hasHTML(e.Panel) && (!existed || !sameText(previous, e.Panel)) {
		issues = append(issues, spec.Issue{
			Path:    e.Pointer,
			Code:    codeHTMLNotAllowed,
			Message: "the agent cannot write HTML; write Markdown without tags, or leave this imported panel unchanged",
		})
	}
	return append(issues, actionViolations(e, previous, existed)...)
}

func placeholderViolations(e spec.PanelEntry, previous *spec.Panel, keptPlaceholder, hasParent bool) []spec.Issue {
	switch {
	case !hasParent:
		return []spec.Issue{{
			Path:    e.Pointer,
			Code:    codePlaceholderNotAllowed,
			Message: "the agent cannot create placeholder panels; remove this panel",
		}}
	case !keptPlaceholder || !samePlaceholder(previous, e.Panel):
		return []spec.Issue{{
			Path:    e.Pointer,
			Code:    codePlaceholderNotAllowed,
			Message: fmt.Sprintf("placeholder %q can only be moved or deleted; keep its id, title, description, reason and sourceType", e.Panel.ID),
		}}
	}
	return nil
}

func actionViolations(e spec.PanelEntry, previous *spec.Panel, existed bool) []spec.Issue {
	return append(canvasActionViolations(e, previous, existed), fieldActionViolations(e, previous, existed)...)
}

func canvasActionViolations(e spec.PanelEntry, previous *spec.Panel, existed bool) []spec.Issue {
	if e.Panel.Type != spec.Canvas || e.Panel.Display == nil {
		return nil
	}
	var issues []spec.Issue
	for i, el := range e.Panel.Display.Elements {
		if len(el.Actions) == 0 || (existed && sameActions(previous, el)) {
			continue
		}
		issues = append(issues, spec.Issue{
			Path:    e.Pointer + "/display/elements/" + strconv.Itoa(i) + "/actions",
			Code:    codeActionNotAllowed,
			Message: "the agent cannot add or change canvas actions; keep them as they are or remove the element",
		})
	}
	return issues
}

func fieldActionViolations(e spec.PanelEntry, previous *spec.Panel, existed bool) []spec.Issue {
	var issues []spec.Issue
	if len(e.Panel.Actions) > 0 && (!existed || !reflect.DeepEqual(previous.Actions, e.Panel.Actions)) {
		issues = append(issues, fieldActionIssue(e.Pointer+"/actions"))
	}
	for i, o := range e.Panel.Overrides {
		if len(o.Actions) > 0 && (!existed || !sameOverrideActions(previous, o)) {
			issues = append(issues, fieldActionIssue(e.Pointer+"/overrides/"+strconv.Itoa(i)+"/actions"))
		}
	}
	return issues
}

func fieldActionIssue(path string) spec.Issue {
	return spec.Issue{
		Path:    path,
		Code:    codeActionNotAllowed,
		Message: "the agent cannot add or change field actions; keep them as they are or remove them",
	}
}

func needsImportOnlyCheck(d *spec.Dashboard) bool {
	for _, e := range d.AllPanels() {
		if e.Panel.Type == spec.Placeholder || hasHTML(e.Panel) || hasActions(e.Panel) {
			return true
		}
	}
	return false
}

func hasHTML(p *spec.Panel) bool {
	if p.Type != spec.Text {
		return false
	}
	return p.Mode == "html" || (p.Content != nil && containsHTML(*p.Content))
}

func hasActions(p *spec.Panel) bool {
	if len(p.Actions) > 0 || slices.ContainsFunc(p.Overrides, func(o spec.Override) bool { return len(o.Actions) > 0 }) {
		return true
	}
	if p.Type != spec.Canvas || p.Display == nil {
		return false
	}
	return slices.ContainsFunc(p.Display.Elements, func(el spec.CanvasElement) bool { return len(el.Actions) > 0 })
}

func panelsByID(d *spec.Dashboard) map[string]*spec.Panel {
	byID := map[string]*spec.Panel{}
	for _, e := range d.AllPanels() {
		byID[e.Panel.ID] = e.Panel
	}
	return byID
}

func samePlaceholder(a, b *spec.Panel) bool {
	return a.Title == b.Title && a.Description == b.Description && a.Reason == b.Reason && a.SourceType == b.SourceType
}

func sameText(previous, next *spec.Panel) bool {
	return previous.Type == spec.Text &&
		reflect.DeepEqual(previous.Content, next.Content) &&
		previous.Mode == next.Mode &&
		reflect.DeepEqual(previous.Code, next.Code)
}

func sameActions(previous *spec.Panel, el spec.CanvasElement) bool {
	if previous.Type != spec.Canvas || previous.Display == nil {
		return false
	}
	for _, old := range previous.Display.Elements {
		if old.ID == el.ID {
			return reflect.DeepEqual(old.Actions, el.Actions)
		}
	}
	return false
}

func sameOverrideActions(previous *spec.Panel, o spec.Override) bool {
	return slices.ContainsFunc(previous.Overrides, func(old spec.Override) bool {
		return reflect.DeepEqual(old.Match, o.Match) && reflect.DeepEqual(old.Actions, o.Actions)
	})
}
