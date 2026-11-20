package revisions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type RunDesk struct {
	service *Service
	opts    ObserveOptions
}

func NewRunDesk(service *Service, opts ObserveOptions) *RunDesk {
	return &RunDesk{service: service, opts: opts}
}

func (r *RunDesk) Run(ctx context.Context, id pgtype.UUID, caller auth.Principal, maxSteps int) (json.RawMessage, error) {
	row, readOnly, err := r.service.ReadObserve(ctx, id, caller)
	if err != nil {
		return nil, err
	}
	out, err := r.render(ctx, row, caller, readOnly)
	if err != nil {
		return nil, err
	}
	if maxSteps > 0 && len(out.Steps) > maxSteps {
		out.Steps = out.Steps[len(out.Steps)-maxSteps:]
		out.StepsTruncated = true
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode agent request: %w", err)
	}
	return raw, nil
}

func (r *RunDesk) List(ctx context.Context, caller auth.Principal, filter ListFilter, limit int, cursor string) (json.RawMessage, error) {
	body, err := r.listBody(ctx, caller, filter, limit, cursor)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode agent request list: %w", err)
	}
	return raw, nil
}

func (r *RunDesk) Update(ctx context.Context, id pgtype.UUID, caller auth.Principal, title *string, clearTitle bool, pinned *bool) (json.RawMessage, error) {
	row, err := r.service.UpdateListFields(ctx, id, caller, title, clearTitle, pinned)
	return r.answer(ctx, row, caller, err)
}

func (r *RunDesk) Archive(ctx context.Context, id pgtype.UUID, caller auth.Principal) error {
	return r.service.Archive(ctx, id, caller)
}

func (r *RunDesk) listBody(ctx context.Context, caller auth.Principal, filter ListFilter, limit int, cursor string) (requestListJSON, error) {
	rows, next, err := r.service.ListRequests(ctx, caller, filter, limit, cursor)
	if err != nil {
		return requestListJSON{}, err
	}
	ids := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	states, err := r.service.buildStates(ctx, ids)
	if err != nil {
		return requestListJSON{}, fmt.Errorf("list agent request build state: %w", err)
	}
	starts := r.service.newStartChecks(caller)
	body := requestListJSON{Requests: make([]requestSummaryJSON, len(rows))}
	for i, row := range rows {
		requester := auth.Principal{Kind: auth.PrincipalKind(row.RequestedByKind), ID: row.RequestedByID.String()}
		var allowed []Action
		if mine := sameActor(requester, caller); mine || row.DashboardTitle.Valid {
			mayStart, err := starts.may(ctx, State(row.State), Source(row.Source), row.DashboardID)
			if err != nil {
				return requestListJSON{}, fmt.Errorf("check who may start a listed agent request again: %w", err)
			}
			mayEdit, err := starts.mayEdit(ctx, State(row.State), row.DashboardID)
			if err != nil {
				return requestListJSON{}, fmt.Errorf("check who may keep a listed agent request's build: %w", err)
			}
			allowed = allowedActions(newFacts(State(row.State), Source(row.Source), row.DashboardID, row.BuildSavedAt.Valid, states[row.ID], mine, mayStart, mayEdit))
		}
		state := states[row.ID]
		state.build = state.progressFor(requester, caller)
		body.Requests[i] = r.summary(row, state, allowed)
	}
	if next != "" {
		body.NextCursor = &next
	}
	if cursor == "" {
		count, err := r.service.CountRequests(ctx, caller, filter)
		if err != nil {
			return requestListJSON{}, fmt.Errorf("count agent requests: %w", err)
		}
		body.Summary = &requestCountJSON{Total: count.Total, People: count.People}
	}
	return body, nil
}

func (r *RunDesk) Start(ctx context.Context, caller auth.Principal, req StartRequest) (json.RawMessage, error) {
	row, err := r.service.Start(ctx, caller, req)
	return r.answer(ctx, row, caller, err)
}

func (r *RunDesk) Cancel(ctx context.Context, id pgtype.UUID, caller auth.Principal) (json.RawMessage, error) {
	row, err := r.service.Cancel(ctx, id, caller)
	return r.answer(ctx, row, caller, err)
}

func (r *RunDesk) Retry(ctx context.Context, id pgtype.UUID, caller auth.Principal) (json.RawMessage, error) {
	row, err := r.service.Retry(ctx, id, caller)
	return r.answer(ctx, row, caller, err)
}

func (r *RunDesk) Build(ctx context.Context, id pgtype.UUID, caller auth.Principal) (json.RawMessage, error) {
	row, _, err := r.service.ReadObserve(ctx, id, caller)
	if err != nil {
		return nil, err
	}
	view, err := r.service.builds.ViewBuild(ctx, caller, row.ID)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(newBuildJSON(view))
	if err != nil {
		return nil, fmt.Errorf("encode build: %w", err)
	}
	return raw, nil
}

func (r *RunDesk) answer(ctx context.Context, row dbsqlc.AgentRequest, caller auth.Principal, err error) (json.RawMessage, error) {
	if err != nil {
		return nil, err
	}
	out, err := r.render(ctx, row, caller, false)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode agent request: %w", err)
	}
	return raw, nil
}

func (r *RunDesk) render(ctx context.Context, row dbsqlc.AgentRequest, caller auth.Principal, readOnly bool) (agentRequestJSON, error) {
	states, err := r.service.buildStates(ctx, []pgtype.UUID{row.ID})
	if err != nil {
		return agentRequestJSON{}, err
	}
	position, err := r.service.queuePosition(ctx, row)
	if err != nil {
		return agentRequestJSON{}, err
	}
	var allowed []Action
	if !readOnly {
		facts, err := r.service.factsFor(ctx, row, states[row.ID], caller)
		if err != nil {
			return agentRequestJSON{}, err
		}
		allowed = allowedActions(facts)
	}
	out := r.view(ctx, row, caller, states[row.ID], allowed, position)
	out.ReadOnly = readOnly
	if out.AssignedTo, err = r.service.assignee(ctx, row); err != nil {
		return agentRequestJSON{}, err
	}
	if mayHaveFinding(row) && r.opts.Finding != nil {
		if err := r.attachFinding(ctx, caller, row, &out); err != nil {
			return agentRequestJSON{}, err
		}
	}
	return out, nil
}

func (s *Service) assignee(ctx context.Context, row dbsqlc.AgentRequest) (*assigneeJSON, error) {
	kind := AssigneeKind(row.AssignedKind)
	if kind == AssignBuiltin {
		return nil, nil
	}
	name, err := s.db.Queries().GetAssigneeName(ctx, dbsqlc.GetAssigneeNameParams{Kind: row.AssignedKind, ID: row.AssignedID})
	if err != nil {
		return nil, fmt.Errorf("get assignee name: %w", err)
	}
	return &assigneeJSON{Kind: kind, ID: row.AssignedID.String(), Name: httpapi.OptionalString(name)}, nil
}

func mayHaveFinding(row dbsqlc.AgentRequest) bool {
	return row.Source != string(SourceSuggest) && !row.ResultVersion.Valid
}

func (r *RunDesk) attachFinding(ctx context.Context, caller auth.Principal, row dbsqlc.AgentRequest, out *agentRequestJSON) error {
	raw, found, err := r.opts.Finding(ctx, caller, row.ID)
	if err != nil || !found {
		return err
	}
	out.Finding = raw
	if out.Status != StatusSucceeded || !row.ObservationID.Valid || r.opts.ObservationURL == nil {
		return nil
	}
	url := r.opts.ObservationURL(row.ObservationID.String())
	out.URL = &url
	return nil
}

func (r *RunDesk) view(ctx context.Context, row dbsqlc.AgentRequest, caller auth.Principal, state buildState, allowed []Action, position *int) agentRequestJSON {
	out := agentRequestJSON{
		ID:          row.ID.String(),
		Kind:        Kind(Source(row.Source)),
		Source:      Source(row.Source),
		Trigger:     row.Trigger,
		Title:       requestTitle(row.Title, row.Prompt),
		Prompt:      row.Prompt,
		Status:      Status(row.Status),
		State:       State(row.State),
		ResultKind:  ResultKind(row.ResultKind),
		Attempt:     int(row.Attempts),
		Attempts:    int(row.Attempts),
		Error:       httpapi.OptionalString(row.Error.String),
		FailureCode: httpapi.OptionalString(row.FailureCode.String),
		Steps:       []Step{},
		RequestedBy: authorJSON{
			Kind: row.RequestedByKind, ID: row.RequestedByID.String(), Name: row.RequestedByName, Email: row.RequestedByEmail.String,
		},
		Origin:       httpapi.OptionalString(row.Origin.String),
		OriginClient: httpapi.OptionalString(row.OriginClient.String),
		CreatedAt:    httpapi.FormatTime(row.CreatedAt.Time),
		StartedAt:    formatOptionalTime(optionalTime(row.StartedAt)),
		FinishedAt:   formatOptionalTime(optionalTime(row.FinishedAt)),
		Build:        state.progressFor(requesterOf(row), caller),
		BuildSavedAt: formatOptionalTime(optionalTime(row.BuildSavedAt)),

		QueuePosition:     position,
		CancelRequestedAt: formatOptionalTime(optionalTime(row.CancelRequestedAt)),
		CanCancel:         slices.Contains(allowed, ActionCancel),
		AllowedActions:    actionStrings(allowed),
		ContinuesID:       optionalUUID(row.ContinuesID),
		ContinueNote:      httpapi.OptionalString(row.ContinueNote.String),
		ContinuedBy:       state.continuedBy,
		RetriedBy:         state.retriedBy,

		PinnedAt:   formatOptionalTime(optionalTime(row.PinnedAt)),
		ArchivedAt: formatOptionalTime(optionalTime(row.ArchivedAt)),
		From:       formatOptionalTime(optionalTime(row.ContextFrom)),
		To:         formatOptionalTime(optionalTime(row.ContextTo)),

		ObservationID: optionalUUID(row.ObservationID),
		ChatID:        chatOf(row.ID, row.ChatID),
	}
	if row.CanceledByKind.Valid {
		out.CanceledBy = &authorJSON{Kind: row.CanceledByKind.String, ID: row.CanceledByID.String(), Name: row.CanceledByName.String}
	}
	if row.DashboardID.Valid {
		id := row.DashboardID.String()
		out.DashboardID = &id
		if out.Status == StatusSucceeded && out.Kind == KindBuild && r.opts.DashboardURL != nil {
			url := r.opts.DashboardURL(id, "")
			out.URL = &url
		}
	}
	if row.BaseVersion.Valid {
		n := int(row.BaseVersion.Int32)
		out.BaseVersion = &n
	}
	if row.PanelID.Valid {
		out.PanelID = &row.PanelID.String
	}
	if row.CommentID.Valid {
		id := row.CommentID.String()
		out.CommentID = &id
	}
	if row.ResultVersion.Valid {
		n := int(row.ResultVersion.Int32)
		out.ResultVersion = &n
	}
	if err := json.Unmarshal(row.Steps, &out.Steps); err != nil || out.Steps == nil {
		if err != nil {
			r.service.logger.ErrorContext(ctx, "decode agent request steps", logattrs.Failure("agent_request_steps_decode_failed", err, slog.String("agent_request_id", out.ID))...)
		}
		out.Steps = []Step{}
	}
	out.Usage = r.service.decodeUsage(ctx, out.ID, row.RunUsage)
	out.LiveText = r.liveText(row)
	return out
}

func (r *RunDesk) liveText(row dbsqlc.AgentRequest) *string {
	if State(row.State) != StateRunning || r.opts.LiveText == nil {
		return nil
	}
	text, ok := r.opts.LiveText(row.ID.String(), int(row.Attempts))
	if !ok {
		return nil
	}
	return &text
}

func (r *RunDesk) summary(row dbsqlc.ListAgentRequestsByRow, state buildState, allowed []Action) requestSummaryJSON {
	out := requestSummaryJSON{
		ID:           row.ID.String(),
		Kind:         Kind(Source(row.Source)),
		Source:       Source(row.Source),
		Trigger:      row.Trigger,
		Title:        requestTitle(row.Title, row.Prompt),
		PinnedAt:     formatOptionalTime(optionalTime(row.PinnedAt)),
		Prompt:       row.Prompt,
		Status:       Status(row.Status),
		State:        State(row.State),
		ResultKind:   ResultKind(row.ResultKind),
		PanelID:      optionalText(row.PanelID),
		Error:        httpapi.OptionalString(row.Error.String),
		Origin:       httpapi.OptionalString(row.Origin.String),
		OriginClient: httpapi.OptionalString(row.OriginClient.String),
		CreatedAt:    httpapi.FormatTime(row.CreatedAt.Time),
		StartedAt:    formatOptionalTime(optionalTime(row.StartedAt)),
		FinishedAt:   formatOptionalTime(optionalTime(row.FinishedAt)),
		RequestedBy:  authorJSON{Kind: row.RequestedByKind, ID: row.RequestedByID.String(), Name: row.RequestedByName},

		FailureCode:  httpapi.OptionalString(row.FailureCode.String),
		Build:        state.build,
		BuildSavedAt: formatOptionalTime(optionalTime(row.BuildSavedAt)),
		ContinuesID:  optionalUUID(row.ContinuesID),
		ContinuedBy:  state.continuedBy,
		RetriedBy:    state.retriedBy,
		ContinueNote: httpapi.OptionalString(row.ContinueNote.String),

		ObservationID:    optionalUUID(row.ObservationID),
		ObservationTitle: optionalText(row.ObservationTitle),
		ChatID:           chatOf(row.ID, row.ChatID),
		AllowedActions:   actionStrings(allowed),
	}
	if row.DashboardID.Valid {
		id := row.DashboardID.String()
		out.DashboardID = &id
		out.DashboardTitle = optionalText(row.DashboardTitle)
		if out.Status == StatusSucceeded && out.Kind == KindBuild && r.opts.DashboardURL != nil {
			url := r.opts.DashboardURL(id, row.DashboardTitle.String)
			out.URL = &url
		}
	}
	if out.Status == StatusSucceeded && row.FindingID.Valid && row.ObservationID.Valid && r.opts.ObservationURL != nil {
		url := r.opts.ObservationURL(row.ObservationID.String())
		out.URL = &url
	}
	if row.ResultVersion.Valid {
		n := int(row.ResultVersion.Int32)
		out.ResultVersion = &n
	}
	return out
}
