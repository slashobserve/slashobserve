package dashboards

import (
	"slices"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func acceptTimeouts(prev, next *spec.Dashboard, issues []spec.Issue) ([]spec.Issue, Unverified) {
	if prev == nil {
		return issues, Unverified{}
	}
	failing, _ := UnverifiedOf(next, issues)
	var accepted Unverified
	for _, p := range failing.Panels {
		if onlyTimeouts(p.Issues) && panelUnchanged(prev, next, p.PanelID) {
			accepted.Panels = append(accepted.Panels, p)
		}
	}
	if accepted.Empty() {
		return issues, accepted
	}
	entries := next.AllPanels()
	blocking := slices.DeleteFunc(slices.Clone(issues), func(is spec.Issue) bool {
		id, ok := owningPanel(entries, is.Path)
		if !ok {
			return false
		}
		_, taken := accepted.Panel(id)
		return taken
	})
	return blocking, accepted
}

func onlyTimeouts(issues []spec.Issue) bool {
	return !slices.ContainsFunc(issues, func(is spec.Issue) bool { return is.Code != queryproxy.CodeQueryTimeout })
}
