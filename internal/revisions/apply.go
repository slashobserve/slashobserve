package revisions

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

const (
	appliesFindingIndex = "agent_requests_applies_finding_idx"
	CodeAlreadyApplied  = "already_applied"
)

var (
	ErrFindingApplied  = errors.New("the suggested change of this answer is already being applied")
	ErrFindingNotFound = errors.New("finding not found")
)

type ApplyParams struct {
	FindingID       pgtype.UUID
	DashboardID     pgtype.UUID
	Answered        dbsqlc.AgentRequest
	Change          string
	CommentID       pgtype.UUID
	PanelID         string
	ThreadMessageID pgtype.UUID
	ThreadContext   string
	Point           Point
	By              auth.Principal
}

func (s *Service) EnqueueApply(ctx context.Context, q *dbsqlc.Queries, p ApplyParams) (dbsqlc.AgentRequest, error) {
	assignee := assigneeOf(p.Answered)
	if assignee.builtin() && !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	if err := checkAssignee(ctx, q, assignee, p.By); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	latest, err := q.GetDashboardLatestVersion(ctx, p.DashboardID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dbsqlc.AgentRequest{}, ErrDashboardNotFound
	case err != nil:
		return dbsqlc.AgentRequest{}, fmt.Errorf("read latest dashboard version: %w", err)
	}
	return s.enqueue(ctx, q, EnqueueParams{
		DashboardID:      p.DashboardID,
		CommentID:        p.CommentID,
		Prompt:           p.Change,
		PanelID:          p.PanelID,
		BaseVersion:      int(latest),
		By:               p.By,
		Assignee:         assignee,
		ThreadMessageID:  p.ThreadMessageID,
		ThreadContext:    p.ThreadContext,
		Point:            p.Point,
		AppliesFindingID: p.FindingID,
	})
}

func (s *Service) RunApplying(ctx context.Context, findingID pgtype.UUID) (dbsqlc.AgentRequest, bool, error) {
	row, err := s.db.Queries().GetRunApplyingFinding(ctx, findingID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dbsqlc.AgentRequest{}, false, nil
	case err != nil:
		return dbsqlc.AgentRequest{}, false, fmt.Errorf("read the run applying a finding: %w", err)
	}
	return row, true, nil
}

func (s *Service) suggestedChange(ctx context.Context, by auth.Principal, req StartRequest) (pgtype.UUID, string, error) {
	switch {
	case req.Kind == nil || *req.Kind != KindRevise:
		return pgtype.UUID{}, "", refused("applies_finding_id needs kind revise")
	case req.DashboardID == nil:
		return pgtype.UUID{}, "", refused("kind revise needs dashboard_id")
	}
	findingID, err := uuidField("applies_finding_id", req.AppliesFindingID)
	if err != nil {
		return pgtype.UUID{}, "", err
	}
	dashboardID, err := uuidField("dashboard_id", req.DashboardID)
	if err != nil {
		return pgtype.UUID{}, "", err
	}
	answered, change, err := s.answerSuggesting(ctx, by, findingID)
	if err != nil {
		return pgtype.UUID{}, "", err
	}
	switch {
	case answered.DashboardID != dashboardID:
		return pgtype.UUID{}, "", refused("applies_finding_id must be an answer about dashboard_id")
	case !change.Valid:
		return pgtype.UUID{}, "", refused("the answer applies_finding_id names suggests no change to the dashboard")
	}
	prompt, err := CheckPrompt("suggested_change", change.String)
	if err != nil {
		return pgtype.UUID{}, "", &StartError{Message: err.Error(), Cause: err}
	}
	return findingID, prompt, nil
}

func (s *Service) answerSuggesting(ctx context.Context, by auth.Principal, findingID pgtype.UUID) (dbsqlc.AgentRequest, pgtype.Text, error) {
	found, err := s.db.Queries().GetFinding(ctx, findingID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dbsqlc.AgentRequest{}, pgtype.Text{}, ErrFindingNotFound
	case err != nil:
		return dbsqlc.AgentRequest{}, pgtype.Text{}, fmt.Errorf("read finding: %w", err)
	case !found.Finding.AgentRequestID.Valid:
		return dbsqlc.AgentRequest{}, pgtype.Text{}, ErrFindingNotFound
	}
	answered, err := s.GetObserve(ctx, found.Finding.AgentRequestID, by)
	if err == nil {
		err = s.checkVisible(ctx, answered.DashboardID, by)
	}
	switch {
	case errors.Is(err, ErrRequestNotFound), errors.Is(err, folders.ErrNoAccess):
		return dbsqlc.AgentRequest{}, pgtype.Text{}, ErrFindingNotFound
	case err != nil:
		return dbsqlc.AgentRequest{}, pgtype.Text{}, err
	}
	return answered, found.Finding.SuggestedChange, nil
}

func (s *Service) applyingRun(ctx context.Context, by auth.Principal, findingID pgtype.UUID) (Started, bool, error) {
	row, found, err := s.RunApplying(ctx, findingID)
	switch {
	case err != nil || !found:
		return Started{}, found, err
	case !sameActor(requesterOf(row), by):
		return Started{}, true, ErrFindingApplied
	}
	return Started{AgentRequest: row, Replayed: true}, true, nil
}
