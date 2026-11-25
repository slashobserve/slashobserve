package dashboards

import (
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func acceptUnchangedFailures(prev, next *spec.Dashboard, issues []spec.Issue) ([]spec.Issue, Unverified) {
	if prev == nil {
		return issues, Unverified{}
	}
	failing, _ := UnverifiedOf(next, issues)
	accepted, slow := acceptedVariables(prev, next, failing.Variables)
	explained := []spec.IssueCode{queryproxy.CodeQueryTimeout}
	if rangeGrew(prev, next) {
		explained = append(explained, queryproxy.CodeQueryEmpty)
	}
	for _, p := range failing.Panels {
		if panelUnchanged(prev, next, p.PanelID) && explainedOrSlowVariables(next, p, slow, explained) {
			accepted.Panels = append(accepted.Panels, p)
		}
	}
	if accepted.Empty() {
		return issues, accepted
	}
	return withoutAccepted(next, issues, accepted), accepted
}

func acceptedVariables(prev, next *spec.Dashboard, failing []UnverifiedVariable) (Unverified, map[string]bool) {
	var accepted Unverified
	slow := map[string]bool{}
	for _, v := range failing {
		if onlyCodes(v.Issues, queryproxy.CodeQueryTimeout) && variableUnchanged(prev, next, v.Name) {
			accepted.Variables = append(accepted.Variables, v)
			slow[v.Name] = true
		}
	}
	for _, v := range failing {
		if !slow[v.Name] && reachesAny(next, v.Name, slow) &&
			onlyCodes(v.Issues, queryproxy.CodeQueryTimeout, queryproxy.CodeVariableUnresolved) && variableUnchanged(prev, next, v.Name) {
			accepted.Variables = append(accepted.Variables, v)
		}
	}
	return accepted, slow
}

func withoutAccepted(next *spec.Dashboard, issues []spec.Issue, accepted Unverified) []spec.Issue {
	entries := next.AllPanels()
	return slices.DeleteFunc(slices.Clone(issues), func(is spec.Issue) bool {
		if i, ok := variableIndex(is.Path); ok && i < len(next.Variables) {
			_, taken := accepted.Variable(next.Variables[i].Name)
			return taken
		}
		id, ok := owningPanel(entries, is.Path)
		if !ok {
			return false
		}
		_, taken := accepted.Panel(id)
		return taken
	})
}

func rangeGrew(prev, next *spec.Dashboard) bool {
	now := time.Now()
	span := func(d *spec.Dashboard) (time.Duration, bool) {
		opts, err := spec.TimeOptionsFor(d, "")
		if err != nil {
			return 0, false
		}
		from, to, err := spec.ResolveTimeRange(d.Time, now, opts)
		return to.Sub(from), err == nil
	}
	before, okBefore := span(prev)
	after, okAfter := span(next)
	return okBefore && okAfter && after > before
}

func explainedOrSlowVariables(next *spec.Dashboard, p UnverifiedPanel, slow map[string]bool, explained []spec.IssueCode) bool {
	if onlyCodes(p.Issues, explained...) {
		return true
	}
	if !onlyCodes(p.Issues, queryproxy.CodeQueryTimeout, queryproxy.CodeVariableUnresolved, queryproxy.CodeVariableFailed) {
		return false
	}
	entry, ok := next.FindDataPanel(p.PanelID)
	if !ok {
		return false
	}
	reachedNames, ok := reachedByPanel(next, entry.Panel)
	if !ok {
		return false
	}
	for name := range reachedNames {
		if slow[name] {
			return true
		}
	}
	return false
}

func reachesAny(d *spec.Dashboard, name string, among map[string]bool) bool {
	names, ok := reached(d, []string{name}, nil)
	if !ok {
		return false
	}
	for other := range names {
		if other != name && among[other] {
			return true
		}
	}
	return false
}

func onlyCodes(issues []spec.Issue, codes ...spec.IssueCode) bool {
	return !slices.ContainsFunc(issues, func(is spec.Issue) bool { return !slices.Contains(codes, is.Code) })
}
