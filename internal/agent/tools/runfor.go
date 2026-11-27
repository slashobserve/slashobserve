package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const CodeAdminsOnlyDatasource = "admins_only_datasource"

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
	Brief   *RunBrief      `json:"brief"`
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

type RunWrites struct {
	Shared     bool
	AdminsOnly bool
	Folders    []pgtype.UUID
}

func RunAudience(w RunWrites) datasource.Audience {
	switch {
	case !w.Shared:
		return datasource.AudienceCaller
	case w.AdminsOnly:
		return datasource.AudienceAdmins
	}
	return datasource.AudienceEveryone
}

func RunAccess(req revisions.Request, w RunWrites) datasource.Access {
	return datasource.Access{By: req.RequestedBy, Audience: RunAudience(w)}
}

func Withheld(a datasource.Access) bool {
	return a.By.Role == auth.RoleAdmin && a.Audience == datasource.AudienceEveryone
}

func RunFor(req revisions.Request) (Run, error) {
	run := Run{RequestID: req.ID, ClaimEpoch: req.ClaimEpoch, Prompt: req.Prompt, TimeZone: req.TimeZone, Investigation: req.Investigation, ChatID: req.ChatID, ThreadAsk: req.ThreadAsk}
	switch {
	case req.Intent == revisions.IntentSuggest:
		run = Run{Mode: ModeSuggest, RequestID: req.ID, PanelID: req.PanelID}
	case req.Intent == revisions.IntentApply:
		run.Mode, run.Build = ModeRevise, true
	case req.Intent != revisions.IntentAsk:
		return Run{}, fmt.Errorf("%w: run intent %q", ErrInvariant, req.Intent)
	case req.Source == revisions.SourceObserve:
		run.Mode, run.Build = ModeCreate, true
	default:
		run.Mode, run.PanelID = ModeInvestigate, req.PanelID
		if req.From != nil && req.To != nil {
			run.From, run.To = *req.From, *req.To
		}
	}
	if req.AsOf != nil {
		run.AsOf = *req.AsOf
	}
	if req.Investigation && req.ObservationID != "" {
		run.Observations = append(run.Observations, req.ObservationID)
	}
	if req.SubjectObservationID != "" {
		run.Observations = append(run.Observations, req.SubjectObservationID)
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
