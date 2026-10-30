package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type suggestedOut struct {
	Suggested bool            `json:"suggested"`
	PanelID   string          `json:"panel_id"`
	Panel     json.RawMessage `json:"panel"`
}

func (r *Registry) suggestPanel(ctx context.Context, principal auth.Principal, run Run, rawSpec json.RawMessage) (Result, error) {
	parsed, err := spec.Parse(rawSpec)
	if err != nil {
		return suggestionFailure(err)
	}
	entry, ok := parsed.FindPanel(run.PanelID)
	if !ok {
		return invalid("keep panel %q in the Spec; this run changes only that panel", run.PanelID), nil
	}
	if entry.Panel.Type == spec.Row {
		return invalid("panel %q must stay a panel, not a row", run.PanelID), nil
	}
	raw, err := json.Marshal(entry.Panel)
	if err != nil {
		return Result{}, err
	}
	if len(raw) > spec.MaxSuggestedPanelBytes {
		return toolError(codeTooLarge, fmt.Sprintf("the panel is %d bytes, over the %d byte limit", len(raw), spec.MaxSuggestedPanelBytes)), nil
	}
	candidate, err := spec.WithPanel(run.Base, run.PanelID, raw)
	if err != nil {
		return suggestionFailure(err)
	}
	if issues := importOnlyIssues(candidate, run.Base); len(issues) > 0 {
		return suggestionRejected(issues), nil
	}
	canonical, err := json.Marshal(candidate)
	if err != nil {
		return Result{}, err
	}
	if issues := spec.NULIssues(canonical); len(issues) > 0 {
		return suggestionRejected(issues), nil
	}
	if entry.Panel.Type.IsData() {
		issues, err := r.deps.Runner.CheckPanels(ctx, candidate, time.Now(), queryproxy.UserFor(principal), []string{run.PanelID})
		if err != nil {
			return Result{}, err
		}
		if len(issues) > 0 {
			blocking, err := r.blockingSuggestionIssues(ctx, run, candidate, issues)
			if err != nil {
				return Result{}, err
			}
			if len(blocking) > 0 {
				return suggestionRejected(blocking), nil
			}
		}
	}
	return success("Proposed a change to panel "+clip(run.PanelID, maxStepNameBytes), suggestedOut{Suggested: true, PanelID: run.PanelID, Panel: raw}), nil
}

func (r *Registry) blockingSuggestionIssues(ctx context.Context, run Run, candidate *spec.Dashboard, issues []spec.Issue) ([]spec.Issue, error) {
	id, err := db.ParseUUID(run.DashboardID)
	if err != nil {
		return nil, fmt.Errorf("run dashboard id: %w", err)
	}
	_, base, err := r.deps.Dashboards.GetVersion(ctx, id, run.ParentVersion)
	if errors.Is(err, dashboards.ErrNotFound) {
		return issues, nil
	}
	if err != nil {
		return nil, err
	}
	_, blocking := dashboards.CarryUnverified(base.Spec, base.Unverified, candidate, issues)
	return blocking, nil
}

func suggestionFailure(err error) (Result, error) {
	var invalidSpec *spec.ValidationError
	if errors.As(err, &invalidSpec) {
		return suggestionRejected(invalidSpec.Issues), nil
	}
	return Result{}, err
}

func suggestionRejected(issues []spec.Issue) Result {
	step := "Suggestion rejected: " + plural(len(issues), "issue", "issues") + issueCodes(issues)
	kept, truncated := fit(slices.Clone(issues), listBudget)
	return failure(step, invalidSpecOut{Code: codeInvalidSpec, Issues: kept, Truncated: truncated})
}
