package tools

import (
	"context"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *Registry) RunFor(ctx context.Context, req revisions.Request) (Run, error) {
	run := Run{RequestID: req.ID, Attempt: req.Attempt, Prompt: req.Prompt}
	switch {
	case req.Source == revisions.SourceSuggest:
		return Run{}, fmt.Errorf("%w: suggest runs have no outside session", ErrInvariant)
	case req.Source == revisions.SourceInvestigate:
		run.Mode, run.PanelID = ModeInvestigate, req.PanelID
		if req.From != nil && req.To != nil {
			run.From, run.To = *req.From, *req.To
		}
	case req.DashboardID == "":
		run.Mode, run.Build = ModeCreate, true
	default:
		run.Mode, run.Build = ModeRevise, true
	}
	if req.DashboardID == "" {
		return run, nil
	}
	id, err := db.ParseUUID(req.DashboardID)
	if err != nil {
		return Run{}, fmt.Errorf("%w: run dashboard id: %w", ErrInvariant, err)
	}
	version, err := r.deps.Dashboards.LatestVersion(ctx, id)
	if err != nil {
		return Run{}, err
	}
	run.DashboardID, run.ParentVersion = req.DashboardID, version
	return run, nil
}
