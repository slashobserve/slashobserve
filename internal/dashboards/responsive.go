package dashboards

import "github.com/slashobserve/slashobserve/pkg/spec"

func freezeResized(prev, next *spec.Dashboard) bool {
	stored := map[string]spec.GridPos{}
	for _, entry := range prev.AllPanels() {
		if entry.Panel.Type != spec.Row {
			stored[entry.Panel.ID] = entry.Panel.GridPos
		}
	}
	changed := false
	for _, entry := range next.AllPanels() {
		panel := entry.Panel
		if panel.Type == spec.Row || panel.Responsive != nil {
			continue
		}
		before, ok := stored[panel.ID]
		if ok && (before.W != panel.GridPos.W || before.H != panel.GridPos.H) {
			panel.Responsive = new(false)
			changed = true
		}
	}
	return changed
}
