package dashboards

import (
	"strconv"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxPlannedPanels = 200

type BuildProgress struct {
	Panels        int
	PlannedPanels int
}

func (p BuildProgress) Pending() int {
	return max(0, p.PlannedPanels-p.Panels)
}

func validatePlannedPanels(n int) []spec.Issue {
	if n >= 1 && n <= maxPlannedPanels {
		return nil
	}
	return []spec.Issue{{
		Path:    "/planned_panels",
		Code:    spec.CodeSchema,
		Message: "planned_panels must be between 1 and " + strconv.Itoa(maxPlannedPanels),
	}}
}

func buildProgress(d *spec.Dashboard, planned int) BuildProgress {
	progress := BuildProgress{PlannedPanels: planned}
	for _, p := range d.Panels {
		if p.Type != spec.Row {
			progress.Panels++
			continue
		}
		progress.Panels += len(p.Panels)
	}
	return progress
}

func stoppedSummary(p BuildProgress) string {
	const prefix = "Saved what the agent built before it stopped: "
	switch {
	case p.PlannedPanels >= p.Panels && p.Panels > 0:
		return prefix + strconv.Itoa(p.Panels) + " of " + strconv.Itoa(p.PlannedPanels) + " panels"
	case p.Panels == 1:
		return prefix + "1 panel"
	}
	return prefix + strconv.Itoa(p.Panels) + " panels"
}
