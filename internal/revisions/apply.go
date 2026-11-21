package revisions

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const appliesFindingIndex = "agent_requests_applies_finding_idx"

var ErrFindingApplied = errors.New("the suggested change of this answer is already being applied")

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
