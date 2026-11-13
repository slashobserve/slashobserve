package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var (
	ErrInvalidReason      = errors.New("reason is not one the brief knows")
	ErrInvalidFailureCode = errors.New("failure_code is not one the agent reports")
	ErrCommitNeedsSuccess = errors.New("commit applies only to a succeeded outcome")
	ErrRunEnded           = errors.New("the run has ended")
)

type BuildProgress struct {
	Saves         int `json:"saves"`
	Panels        int `json:"panels"`
	PlannedPanels int `json:"planned_panels"`
}

type BriefAnswer struct {
	System  string         `json:"system"`
	Message string         `json:"message"`
	Reason  string         `json:"reason"`
	Build   *BuildProgress `json:"build"`
}

type RunFinish struct {
	Outcome     string
	FailureCode string
	Message     string
	Commit      bool
	Usage       *revisions.Usage
}

type RunFinished struct {
	State         string  `json:"state"`
	ResultKind    string  `json:"result_kind"`
	ResultVersion *int    `json:"result_version"`
	FailureCode   *string `json:"failure_code"`
	Restarted     bool    `json:"restarted"`
}

type RunSessions interface {
	Brief(ctx context.Context, req revisions.Request, reason string) (BriefAnswer, error)
	NoteUsage(ctx context.Context, req revisions.Request, u revisions.Usage) (stop bool, err error)
	Finish(ctx context.Context, req revisions.Request, in RunFinish) (RunFinished, error)
	Progress(ctx context.Context, req revisions.Request) (*BuildProgress, error)
}

func RunFor(req revisions.Request) (Run, error) {
	run := Run{RequestID: req.ID, Attempt: req.Attempt, Prompt: req.Prompt}
	switch {
	case req.Source == revisions.SourceSuggest:
		run = Run{Mode: ModeSuggest, RequestID: req.ID, PanelID: req.PanelID}
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
	if _, err := db.ParseUUID(req.DashboardID); err != nil {
		return Run{}, fmt.Errorf("%w: run dashboard id: %w", ErrInvariant, err)
	}
	run.DashboardID = req.DashboardID
	return run, nil
}
