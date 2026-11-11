package claims

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const timeLimitMessage = "the run passed its time limit"

type ReportInput struct {
	RunID   string `json:"run_id"`
	Attempt int    `json:"attempt"`
	Note    string `json:"note"`
}

type ReportAnswer struct {
	State      string  `json:"state"`
	Stop       bool    `json:"stop"`
	LeaseUntil *string `json:"lease_until"`
}

func (s *Service) Report(ctx context.Context, c Claimer, in ReportInput) (ReportAnswer, error) {
	row, err := s.claimedRun(ctx, c, in.RunID, in.Attempt)
	if err != nil {
		return ReportAnswer{}, err
	}
	s.touch(ctx, c)
	if revisions.State(row.State).Terminal() {
		return ReportAnswer{State: row.State, Stop: true}, nil
	}
	passed, err := s.deadlinePassed(ctx, row)
	if err != nil {
		return ReportAnswer{}, err
	}
	if passed {
		return s.timeOut(ctx, row)
	}
	canceled, err := s.runs.RenewLease(ctx, row)
	if errors.Is(err, revisions.ErrLeaseLost) {
		return s.stopped(ctx, row)
	}
	if err != nil {
		return ReportAnswer{}, err
	}
	s.recordNote(ctx, row, in.Note)
	if canceled {
		return ReportAnswer{State: string(revisions.StateCanceling), Stop: true}, nil
	}
	until := time.Now().Add(revisions.LeaseTTL).UTC().Format(time.RFC3339)
	return ReportAnswer{State: string(revisions.StateRunning), LeaseUntil: &until}, nil
}

func (s *Service) claimedRun(ctx context.Context, c Claimer, runID string, attempt int) (dbsqlc.AgentRequest, error) {
	id, err := db.ParseUUID(runID)
	if err != nil || attempt < 1 || attempt > math.MaxInt32 {
		return dbsqlc.AgentRequest{}, ErrNotYourRun
	}
	row, err := s.db.Queries().GetClaimedAgentRequest(ctx, dbsqlc.GetClaimedAgentRequestParams{
		ID: id, Attempts: int32(attempt), ClaimedBy: textOf(c.claimant().Key()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.AgentRequest{}, ErrNotYourRun
	}
	return row, err
}

func (s *Service) deadlinePassed(ctx context.Context, row dbsqlc.AgentRequest) (bool, error) {
	deadline, err := s.db.Queries().GetRunTokenDeadline(ctx, dbsqlc.GetRunTokenDeadlineParams{AgentRequestID: row.ID, Attempt: row.Attempts})
	if err != nil {
		return false, err
	}
	return !deadline.Valid || !deadline.Time.After(time.Now()), nil
}

func (s *Service) timeOut(ctx context.Context, row dbsqlc.AgentRequest) (ReportAnswer, error) {
	ended, err := s.runs.End(ctx, row, revisions.Ending{Failure: revisions.FailureRunTimeout, Message: timeLimitMessage})
	if errors.Is(err, revisions.ErrLeaseLost) {
		return s.stopped(ctx, row)
	}
	if err != nil {
		return ReportAnswer{}, err
	}
	s.dropTokens(ctx, row)
	return ReportAnswer{State: ended.State, Stop: true}, nil
}

func (s *Service) stopped(ctx context.Context, row dbsqlc.AgentRequest) (ReportAnswer, error) {
	stored, err := s.runs.GetByID(ctx, row.ID)
	if err != nil {
		return ReportAnswer{}, err
	}
	return ReportAnswer{State: stored.State, Stop: true}, nil
}

func (s *Service) dropTokens(ctx context.Context, row dbsqlc.AgentRequest) {
	if err := s.db.Queries().DeleteRunTokens(ctx, row.ID); err != nil {
		s.logger.WarnContext(ctx, "delete run sessions; prune will remove them", logattrs.Failure("run_session_delete_failed", err, slog.String("agent_request_id", row.ID.String()))...)
	}
}

func (s *Service) recordNote(ctx context.Context, row dbsqlc.AgentRequest, note string) {
	note = cleanText(note, maxNoteRunes)
	if note == "" {
		return
	}
	s.RecordStep(ctx, revisions.RequestOf(row), revisions.Step{At: time.Now(), Phase: "discover", Tool: "report_run", Status: "ok", Message: note})
}
