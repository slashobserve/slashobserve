package revisions

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const continuesKeyIndex = "agent_requests_continues_key"

var ErrNotRequester = errors.New("only the person or key that asked can do this")

type buildState struct {
	build       *buildProgressJSON
	continuedBy *string
}

func newBuildState(row dbsqlc.ListAgentRequestBuildStateRow) buildState {
	var state buildState
	if row.Saves.Valid {
		state.build = &buildProgressJSON{
			Saves:         int(row.Saves.Int32),
			Panels:        int(row.Panels.Int32),
			PlannedPanels: int(row.PlannedPanels.Int32),
			Title:         &row.Title,
			UpdatedAt:     httpapi.FormatTime(row.UpdatedAt.Time),
		}
	}
	if row.ContinuedBy.Valid {
		id := row.ContinuedBy.String()
		state.continuedBy = &id
	}
	return state
}

func (s buildState) progressFor(requester, caller auth.Principal) *buildProgressJSON {
	if s.build == nil || sameActor(requester, caller) {
		return s.build
	}
	hidden := *s.build
	hidden.Title = nil
	return &hidden
}

func (s *Service) buildStates(ctx context.Context, ids []pgtype.UUID) (map[pgtype.UUID]buildState, error) {
	states := make(map[pgtype.UUID]buildState, len(ids))
	if len(ids) == 0 {
		return states, nil
	}
	rows, err := s.db.Queries().ListAgentRequestBuildState(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list agent request build state: %w", err)
	}
	for _, row := range rows {
		states[row.ID] = newBuildState(row)
	}
	return states, nil
}

func canCancel(row dbsqlc.AgentRequest, by auth.Principal) bool {
	active := row.Status == string(StatusQueued) || row.Status == string(StatusRunning)
	return active && isRequester(row, by)
}

func continueBlocker(row dbsqlc.AgentRequest) error {
	if row.Status != string(StatusFailed) || row.BuildSavedAt.Valid {
		return ErrNotContinuable
	}
	return nil
}

func (s *Service) requesterRow(ctx context.Context, id pgtype.UUID, by auth.Principal) (dbsqlc.AgentRequest, error) {
	row, err := s.GetObserve(ctx, id, by)
	if err == nil {
		err = s.checkVisible(ctx, row.DashboardID, by)
	}
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if !isRequester(row, by) {
		return dbsqlc.AgentRequest{}, ErrNotRequester
	}
	return row, nil
}

func canceledOrFinished(row dbsqlc.AgentRequest) (dbsqlc.AgentRequest, error) {
	if row.Status == string(StatusFailed) && row.FailureCode.String == string(FailureCanceled) {
		return row, nil
	}
	return dbsqlc.AgentRequest{}, ErrRequestFinished
}

func (s *Service) Cancel(ctx context.Context, id pgtype.UUID, by auth.Principal) (dbsqlc.AgentRequest, error) {
	row, err := s.requesterRow(ctx, id, by)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if !canCancel(row, by) {
		return canceledOrFinished(row)
	}
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("requester id: %w", err)
	}
	var out dbsqlc.AgentRequest
	err = s.db.InTxWithSavepoints(ctx, func(q *dbsqlc.Queries, savepoint db.Savepoint) error {
		updated, err := q.CancelAgentRequest(ctx, dbsqlc.CancelAgentRequestParams{
			CanceledByKind: pgtype.Text{String: string(by.Kind), Valid: true},
			CanceledByID:   requesterID,
			CanceledByName: pgtype.Text{String: by.Name, Valid: true},
			Message:        CanceledMessage,
			ID:             row.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			current, err := q.GetAgentRequestByID(ctx, row.ID)
			if err != nil {
				return fmt.Errorf("get agent request: %w", err)
			}
			out, err = canceledOrFinished(current)
			return err
		}
		if err != nil {
			return fmt.Errorf("cancel agent request: %w", err)
		}
		out = updated
		if updated.Status == string(StatusRunning) {
			return nil
		}
		if err := notifyRevision(ctx, q, updated); err != nil {
			return err
		}
		if updated.Status == string(StatusSucceeded) {
			s.resolveThread(ctx, savepoint, updated)
		}
		return nil
	})
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	return out, nil
}

func (s *Service) Continue(ctx context.Context, id pgtype.UUID, by auth.Principal, note string) (dbsqlc.AgentRequest, error) {
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	row, err := s.requesterRow(ctx, id, by)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := continueBlocker(row); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := s.checkRun(ctx, Source(row.Source), row.DashboardID, by); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := s.checkContinuable(ctx, row); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	var next dbsqlc.AgentRequest
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		if row.DashboardID.Valid {
			next, err = s.Enqueue(ctx, q, EnqueueParams{
				DashboardID:  row.DashboardID,
				Source:       Source(row.Source),
				CommentID:    row.CommentID,
				Prompt:       row.Prompt,
				PanelID:      row.PanelID.String,
				BaseVersion:  int(row.BaseVersion.Int32),
				By:           by,
				ContinuesID:  row.ID,
				ContinueNote: note,
			})
		} else {
			next, err = s.enqueueObserve(ctx, q, by, row.Prompt, row.ID, note)
		}
		if err != nil {
			return err
		}
		moved, err := q.MoveAgentRequestBuild(ctx, dbsqlc.MoveAgentRequestBuildParams{FromID: row.ID, ToID: next.ID})
		if err != nil {
			return fmt.Errorf("move agent request build: %w", err)
		}
		if moved != 1 {
			return ErrNotContinuable
		}
		return notifyRevision(ctx, q, row)
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == continuesKeyIndex:
		return dbsqlc.AgentRequest{}, ErrNotContinuable
	case db.IsForeignKeyViolation(err):
		return dbsqlc.AgentRequest{}, ErrDashboardNotFound
	case err != nil:
		return dbsqlc.AgentRequest{}, err
	}
	return next, nil
}

func (s *Service) checkContinuable(ctx context.Context, row dbsqlc.AgentRequest) error {
	build, err := s.db.Queries().GetAgentRequestBuild(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotContinuable
	}
	if err != nil {
		return fmt.Errorf("get agent request build: %w", err)
	}
	if !row.DashboardID.Valid {
		return nil
	}
	latest, err := s.db.Queries().GetDashboardLatestVersion(ctx, row.DashboardID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrDashboardNotFound
	case err != nil:
		return fmt.Errorf("read latest dashboard version: %w", err)
	case latest > build.BaseVersion.Int32:
		return &BuildConflictError{Latest: int(latest)}
	}
	return nil
}

func (s *Service) queuePosition(ctx context.Context, row dbsqlc.AgentRequest) (*int, error) {
	if row.Status != string(StatusQueued) {
		return nil, nil
	}
	before, err := s.db.Queries().CountQueuedAgentRequestsBefore(ctx, dbsqlc.CountQueuedAgentRequestsBeforeParams{CreatedAt: row.CreatedAt, ID: row.ID})
	if err != nil {
		return nil, fmt.Errorf("count queued agent requests before: %w", err)
	}
	position := int(before) + 1
	return &position, nil
}
