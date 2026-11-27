package runsession

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const stepRouted = "Routed the question to the %s task type"

func routes(run tools.Run) bool {
	return run.Mode == tools.ModeInvestigate
}

func (s *Service) routed(ctx context.Context, run tools.Run, req revisions.Request) route.Task {
	if !routes(run) {
		return route.Full
	}
	return route.Classify(route.Question{
		Investigation: run.Investigation,
		Dashboard:     run.DashboardID != "",
		Panel:         run.PanelID != "",
		Observation:   len(run.Observations) > 0,
		Types:         s.datasourceTypes(ctx),
		Text:          req.Prompt,
	})
}

func (s *Service) datasourceTypes(ctx context.Context) []string {
	if s.datasources == nil {
		return nil
	}
	var types []string
	for _, in := range s.datasources.Visible(ctx) {
		if t := in.Datasource.Type(); !slices.Contains(types, t) {
			types = append(types, t)
		}
	}
	return types
}

func (sess *session) recordRoute() {
	if sess.state.Guard.Mode != tools.ModeInvestigate {
		return
	}
	sess.record(revisions.Step{At: time.Now(), Phase: tools.PhaseDiscover, Tool: stopTool, Status: "ok", Message: fmt.Sprintf(stepRouted, sess.state.Task.Name())})
}
