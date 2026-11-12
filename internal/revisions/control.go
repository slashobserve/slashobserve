package revisions

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
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

func newFacts(state State, source Source, dashboardID pgtype.UUID, buildSaved bool, build buildState, requester, mayStart, mayEdit bool) runFacts {
	f := runFacts{
		state:       state,
		source:      source,
		onDashboard: dashboardID.Valid,
		requester:   requester,
		mayStart:    mayStart,
		mayEdit:     mayEdit,
		buildSaved:  buildSaved,
		continued:   build.continuedBy != nil,
	}
	if build.build != nil {
		f.buildPanels = build.build.Panels
	}
	return f
}

func factsOf(row dbsqlc.AgentRequest, build buildState, by auth.Principal, mayStart, mayEdit bool) runFacts {
	return newFacts(State(row.State), Source(row.Source), row.DashboardID, row.BuildSavedAt.Valid, build, isRequester(row, by), mayStart, mayEdit)
}

type startChecks struct {
	service *Service
	by      auth.Principal
	seen    map[startKey]bool
	edits   map[pgtype.UUID]bool
}

type startKey struct {
	source    Source
	dashboard pgtype.UUID
}

func (s *Service) newStartChecks(by auth.Principal) *startChecks {
	return &startChecks{service: s, by: by, seen: map[startKey]bool{}, edits: map[pgtype.UUID]bool{}}
}

func (c *startChecks) mayEdit(ctx context.Context, state State, dashboardID pgtype.UUID) (bool, error) {
	if !dashboardID.Valid || !slices.Contains(RunMachine.From(ActionKeepBuild), string(state)) {
		return true, nil
	}
	if may, ok := c.edits[dashboardID]; ok {
		return may, nil
	}
	access, err := c.service.access.Dashboard(ctx, c.by, dashboardID)
	switch {
	case err == nil:
		c.edits[dashboardID] = access.Abilities.Edit
		return access.Abilities.Edit, nil
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden):
		c.edits[dashboardID] = false
		return false, nil
	}
	return false, err
}

func (c *startChecks) may(ctx context.Context, state State, source Source, dashboardID pgtype.UUID) (bool, error) {
	if !state.Terminal() || !(runFacts{source: source, onDashboard: dashboardID.Valid}).retryable() {
		return false, nil
	}
	if source == SourceObserve {
		dashboardID = pgtype.UUID{}
	}
	key := startKey{source: source, dashboard: dashboardID}
	if may, ok := c.seen[key]; ok {
		return may, nil
	}
	err := c.service.checkRun(ctx, source, dashboardID, c.by)
	may := err == nil
	switch {
	case err == nil,
		errors.Is(err, ErrForbidden), errors.Is(err, ErrDashboardNotFound),
		errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden):
		c.seen[key] = may
		return may, nil
	}
	return false, err
}

func (s *Service) factsFor(ctx context.Context, row dbsqlc.AgentRequest, build buildState, by auth.Principal) (runFacts, error) {
	checks := s.newStartChecks(by)
	mayStart, err := checks.may(ctx, State(row.State), Source(row.Source), row.DashboardID)
	if err != nil {
		return runFacts{}, err
	}
	mayEdit, err := checks.mayEdit(ctx, State(row.State), row.DashboardID)
	if err != nil {
		return runFacts{}, err
	}
	return factsOf(row, build, by, mayStart, mayEdit), nil
}

func (s *Service) continueBlocker(ctx context.Context, row dbsqlc.AgentRequest, by auth.Principal) error {
	states, err := s.buildStates(ctx, []pgtype.UUID{row.ID})
	if err != nil {
		return err
	}
	if !slices.Contains(allowedActions(factsOf(row, states[row.ID], by, false, true)), ActionContinue) {
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
	if State(row.State) == StateCanceled || State(row.State) == StateCanceling {
		return row, nil
	}
	return dbsqlc.AgentRequest{}, ErrRequestFinished
}

func (s *Service) Cancel(ctx context.Context, id pgtype.UUID, by auth.Principal) (dbsqlc.AgentRequest, error) {
	row, err := s.requesterRow(ctx, id, by)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if !slices.Contains(allowedActions(factsOf(row, buildState{}, by, false, true)), ActionCancel) {
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
			FromStates:     RunMachine.From(ActionCancel),
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
		out = updated.AgentRequest
		if updated.AgentRequest.State == updated.PriorState {
			return nil
		}
		return s.recordEnded(ctx, q, savepoint, updated.AgentRequest, ActionCancel)
	})
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	return out, nil
}

func (s *Service) Continue(ctx context.Context, id pgtype.UUID, by auth.Principal, note string) (dbsqlc.AgentRequest, error) {
	row, err := s.requesterRow(ctx, id, by)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	assignee := assigneeOf(row)
	if assignee.builtin() && !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	if err := s.continueBlocker(ctx, row, by); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := s.checkRun(ctx, Source(row.Source), row.DashboardID, by); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	var next dbsqlc.AgentRequest
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := checkContinuable(ctx, q, row); err != nil {
			return err
		}
		if err := checkAssignee(ctx, q, assignee, by); err != nil {
			return err
		}
		var err error
		threadMessage, threadContext := threadOf(row)
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

				Assignee:        assignee,
				ThreadMessageID: threadMessage,
				ThreadContext:   threadContext,
			})
		} else {
			next, err = s.enqueueObserve(ctx, q, by, row.Prompt, row.ID, note)
		}
		if err != nil {
			return err
		}
		moved, err := q.MoveAgentRequestBuild(ctx, dbsqlc.MoveAgentRequestBuildParams{FromStates: RunMachine.From(ActionContinue), FromID: row.ID, ToID: next.ID})
		if err != nil {
			return fmt.Errorf("move agent request build: %w", err)
		}
		if moved != 1 {
			return ErrNotContinuable
		}
		return notifyRevision(ctx, q, row)
	})
	if err != nil {
		return dbsqlc.AgentRequest{}, continueFailure(err)
	}
	return next, nil
}

func continueFailure(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == continuesKeyIndex:
		return ErrNotContinuable
	case db.IsForeignKeyViolation(err):
		return ErrDashboardNotFound
	}
	return err
}

func (s *Service) Retry(ctx context.Context, id pgtype.UUID, by auth.Principal) (dbsqlc.AgentRequest, error) {
	row, err := s.GetObserve(ctx, id, by)
	if err == nil {
		err = s.checkVisible(ctx, row.DashboardID, by)
	}
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	assignee := assigneeOf(row)
	if assignee.builtin() && !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	states, err := s.buildStates(ctx, []pgtype.UUID{row.ID})
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	build := states[row.ID]
	if !slices.Contains(allowedActions(factsOf(row, build, by, true, true)), ActionRetry) {
		facts, err := s.factsFor(ctx, row, build, by)
		if err != nil {
			return dbsqlc.AgentRequest{}, err
		}
		return dbsqlc.AgentRequest{}, &lifecycle.Refusal{
			Machine: RunMachine.Name,
			State:   row.State,
			Action:  string(ActionRetry),
			Allowed: actionStrings(allowedActions(facts)),
		}
	}
	if err := checkAssignee(ctx, s.db.Queries(), assignee, by); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	threadMessage, threadContext := threadOf(row)
	switch Source(row.Source) {
	case SourceObserve:
		return s.SubmitObserve(ctx, by, row.Prompt)
	case SourceInvestigate:
		return s.SubmitInvestigate(ctx, InvestigateParams{
			Prompt:      row.Prompt,
			By:          by,
			DashboardID: row.DashboardID,
			PanelID:     row.PanelID.String,
			From:        optionalTime(row.ContextFrom),
			To:          optionalTime(row.ContextTo),

			ObservationID:    row.ObservationID,
			ReuseObservation: true,

			Assignee:        assignee,
			ThreadMessageID: threadMessage,
			ThreadContext:   threadContext,
		})
	default:
		return s.submitFollowUp(ctx, EnqueueParams{
			DashboardID:     row.DashboardID,
			Prompt:          row.Prompt,
			By:              by,
			Assignee:        assignee,
			ThreadMessageID: threadMessage,
			ThreadContext:   threadContext,
		}, nil)
	}
}

func threadOf(row dbsqlc.AgentRequest) (message pgtype.UUID, transcript string) {
	if !row.ThreadMessageID.Valid {
		return pgtype.UUID{}, ""
	}
	return row.ThreadMessageID, row.ThreadContext.String
}

func checkContinuable(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) error {
	build, err := q.GetAgentRequestBuild(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotContinuable
	}
	if err != nil {
		return fmt.Errorf("get agent request build: %w", err)
	}
	if !row.DashboardID.Valid {
		return nil
	}
	latest, err := q.GetDashboardLatestVersion(ctx, row.DashboardID)
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
	if State(row.State) != StateQueued {
		return nil, nil
	}
	before, err := s.db.Queries().CountQueuedAgentRequestsBefore(ctx, dbsqlc.CountQueuedAgentRequestsBeforeParams{CreatedAt: row.CreatedAt, ID: row.ID})
	if err != nil {
		return nil, fmt.Errorf("count queued agent requests before: %w", err)
	}
	position := int(before) + 1
	return &position, nil
}
