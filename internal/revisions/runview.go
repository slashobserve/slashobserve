package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/anchor"
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
	out, err := r.update(ctx, id, caller, title, clearTitle, pinned)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode agent request: %w", err)
	}
	return raw, nil
}

func (r *RunDesk) update(ctx context.Context, id pgtype.UUID, caller auth.Principal, title *string, clearTitle bool, pinned *bool) (agentRequestJSON, error) {
	row, err := r.service.UpdateListFields(ctx, id, caller, title, clearTitle, pinned)
	if err != nil {
		return agentRequestJSON{}, err
	}
	var shared SharedTitle
	if (title != nil || clearTitle) && r.opts.ShareTitle != nil {
		if shared, err = r.opts.ShareTitle(ctx, caller, row.ID, requestTitle(row.Title, row.Prompt)); err != nil {
			return agentRequestJSON{}, fmt.Errorf("share the chat title with its observation: %w", err)
		}
	}
	out, err := r.render(ctx, row, caller, false)
	out.ObservationRename = shared
	return out, err
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
			mayStart, err := starts.may(ctx, State(row.State), listedSource(row), listedShape(row).target())
			if err != nil {
				return requestListJSON{}, fmt.Errorf("check who may start a listed agent request again: %w", err)
			}
			mayEdit, err := starts.mayEdit(ctx, State(row.State), row.DashboardID)
			if err != nil {
				return requestListJSON{}, fmt.Errorf("check who may keep a listed agent request's build: %w", err)
			}
			allowed = allowedActions(newFacts(State(row.State), listedSource(row), row.DashboardID, row.BuildSavedAt.Valid, states[row.ID], mine, mayStart, mayEdit, asOfBars(row.AsOf, caller)))
		}
		state := states[row.ID]
		state.build = state.progressFor(requester, caller)
		body.Requests[i] = r.summary(row, caller, state, allowed)
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
	started, err := r.service.Start(ctx, caller, req)
	if err != nil {
		return nil, err
	}
	out, err := r.renderStarted(ctx, started, caller)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode agent request: %w", err)
	}
	return raw, nil
}

func (r *RunDesk) renderStarted(ctx context.Context, started Started, caller auth.Principal) (agentRequestJSON, error) {
	out, err := r.render(ctx, started.AgentRequest, caller, false)
	out.Replayed = started.Replayed
	return out, err
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
	view := r.service.viewOf(ctx, row, caller)
	if view == viewNone {
		return agentRequestJSON{}, ErrRequestNotFound
	}
	readOnly = readOnly || view == viewRedacted
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
	if view == viewRedacted {
		out.redact()
	}
	out.ReadOnly = readOnly
	if out.ObservationID, err = r.shownObservation(ctx, caller, out.ObservationID, row.ObservationID); err != nil {
		return agentRequestJSON{}, err
	}
	if out.SubjectID, err = r.shownObservation(ctx, caller, out.SubjectID, row.SubjectObservationID); err != nil {
		return agentRequestJSON{}, err
	}
	if out.AssignedTo, err = r.service.assignee(ctx, row); err != nil {
		return agentRequestJSON{}, err
	}
	if out.SharedThread, err = r.service.sharedThreadShownTo(ctx, row, caller); err != nil {
		return agentRequestJSON{}, err
	}
	if findingShown(view, row) && r.opts.Finding != nil {
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

func (s *Service) sharedThreadShownTo(ctx context.Context, row dbsqlc.AgentRequest, caller auth.Principal) (*string, error) {
	if !row.ChatID.Valid || !row.ThreadMessageID.Valid || !sameActor(requesterOf(row), caller) {
		return nil, nil
	}
	thread, err := s.db.Queries().GetThreadOfMessage(ctx, row.ThreadMessageID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("read the thread a run is shared in: %w", err)
	}
	return optionalUUID(thread.ID), nil
}

func mayHaveFinding(row dbsqlc.AgentRequest) bool {
	return SourceOf(row) != SourceSuggest && !row.ResultVersion.Valid
}

func findingShown(view runView, row dbsqlc.AgentRequest) bool {
	return view != viewNone && mayHaveFinding(row)
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

func (r *RunDesk) shownObservation(ctx context.Context, caller auth.Principal, shown *string, id pgtype.UUID) (*string, error) {
	if shown == nil || r.opts.ObservationVisible == nil {
		return shown, nil
	}
	visible, err := r.opts.ObservationVisible(ctx, caller, id)
	switch {
	case err != nil:
		return nil, fmt.Errorf("check the observation a run names: %w", err)
	case !visible:
		return nil, nil
	}
	return shown, nil
}

func (r *RunDesk) view(ctx context.Context, row dbsqlc.AgentRequest, caller auth.Principal, state buildState, allowed []Action, position *int) agentRequestJSON {
	out := agentRequestJSON{
		ID:            row.ID.String(),
		Kind:          Kind(SourceOf(row)),
		Intent:        Intent(row.Intent),
		Investigation: row.Investigation,
		Trigger:       row.Trigger,
		Title:         titleShownTo(row, caller),
		Prompt:        row.Prompt,
		Status:        Status(row.Status),
		State:         State(row.State),
		ResultKind:    ReportedResultKind(row.ResultKind, row.ObservationID),
		Attempt:       int(row.Attempts),
		Attempts:      int(row.Attempts),
		Error:         httpapi.OptionalString(row.Error.String),
		FailureCode:   httpapi.OptionalString(row.FailureCode.String),
		Steps:         []Step{},
		RequestedBy: authorJSON{
			Kind: row.RequestedByKind, ID: row.RequestedByID.String(), Name: row.RequestedByName, Email: row.RequestedByEmail.String,
		},
		Origin:       httpapi.OptionalString(row.Origin.String),
		OriginClient: httpapi.OptionalString(row.OriginClient.String),
		CreatedAt:    httpapi.FormatTime(row.CreatedAt.Time),
		StartedAt:    httpapi.FormatOptionalTime(optionalTime(row.StartedAt)),
		FinishedAt:   httpapi.FormatOptionalTime(optionalTime(row.FinishedAt)),
		Build:        state.progressFor(requesterOf(row), caller),
		BuildSavedAt: httpapi.FormatOptionalTime(optionalTime(row.BuildSavedAt)),
		BuildState:   state.derived(),

		QueuePosition:     position,
		CancelRequestedAt: httpapi.FormatOptionalTime(optionalTime(row.CancelRequestedAt)),
		CanCancel:         slices.Contains(allowed, ActionCancel),
		AllowedActions:    actionStrings(allowed),
		ContinuesID:       optionalUUID(row.ContinuesID),
		ContinueNote:      httpapi.OptionalString(row.ContinueNote.String),
		ContinuedBy:       state.continuedBy,
		RetriedBy:         state.retriedBy,

		PinnedAt:   httpapi.FormatOptionalTime(optionalTime(row.PinnedAt)),
		ArchivedAt: httpapi.FormatOptionalTime(optionalTime(row.ArchivedAt)),
		From:       httpapi.FormatOptionalTime(optionalTime(row.ContextFrom)),
		To:         httpapi.FormatOptionalTime(optionalTime(row.ContextTo)),
		AsOf:       httpapi.FormatOptionalTime(optionalTime(row.AsOf)),

		ObservationID: optionalUUID(row.ObservationID),
		SubjectID:     optionalUUID(row.SubjectObservationID),
		ChatID:        chatShownTo(row, caller),
	}
	out.Anchor, out.BlockID = pointShownTo(row.RequestedByKind, row.RequestedByID, row.Anchor, row.BlockID, caller)
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
	out.Usage = usageShownTo(r.service.decodeUsage(ctx, out.ID, row.RunUsage), row, caller)
	out.LiveText = r.liveText(row)
	return out
}

func pointShownTo(requesterKind string, requesterID pgtype.UUID, raw []byte, block pgtype.UUID, caller auth.Principal) (*anchor.Anchor, *string) {
	if !sameActor(auth.Principal{Kind: auth.PrincipalKind(requesterKind), ID: requesterID.String()}, caller) {
		return nil, nil
	}
	return anchorOf(raw), optionalUUID(block)
}

func (out *agentRequestJSON) redact() {
	out.Redacted = true
	out.Title, out.Prompt, out.LiveText, out.ContinueNote, out.SubjectID, out.Error = "", "", nil, nil, nil, nil
	for i := range out.Steps {
		out.Steps[i].Message = ""
	}
}

type runView int

const (
	viewNone runView = iota
	viewRedacted
	viewFull
)

type runAudience struct {
	requester     auth.Principal
	chat          bool
	privateAsk    bool
	thread        pgtype.UUID
	investigation bool
}

func audienceOf(row dbsqlc.AgentRequest) runAudience {
	return runAudience{
		requester:     requesterOf(row),
		chat:          row.ChatID.Valid,
		privateAsk:    PrivateAsk(row),
		thread:        row.ThreadMessageID,
		investigation: row.Investigation,
	}
}

func (s *Service) viewOf(ctx context.Context, row dbsqlc.AgentRequest, caller auth.Principal) runView {
	return s.viewFor(ctx, audienceOf(row), caller)
}

func (s *Service) viewFor(ctx context.Context, a runAudience, caller auth.Principal) runView {
	return a.viewFor(caller, func() bool {
		return s.threadReads != nil && s.threadReads(ctx, caller, a.thread) == nil
	})
}

func (a runAudience) readsTheChat(caller auth.Principal) bool {
	return sameActor(a.requester, caller) || caller.Role.AtLeast(auth.RoleAdmin)
}

func (a runAudience) viewFor(caller auth.Principal, readsThread func() bool) runView {
	switch {
	case a.readsTheChat(caller), !a.chat, a.investigation:
		return viewFull
	case a.thread.Valid && readsThread():
		return viewFull
	case a.privateAsk:
		return viewNone
	}
	return viewRedacted
}

func readsTheChat(row dbsqlc.AgentRequest, caller auth.Principal) bool {
	return audienceOf(row).readsTheChat(caller)
}

func chatShownTo(row dbsqlc.AgentRequest, caller auth.Principal) *string {
	if !row.ChatID.Valid || !readsTheChat(row, caller) {
		return nil
	}
	chat := row.ChatID.String()
	return &chat
}

func usageShownTo(usage *Usage, row dbsqlc.AgentRequest, caller auth.Principal) *Usage {
	if usage == nil || len(usage.Hypotheses) == 0 || readsTheChat(row, caller) {
		return usage
	}
	shown := *usage
	shown.Hypotheses = nil
	return &shown
}

func titleShownTo(row dbsqlc.AgentRequest, caller auth.Principal) string {
	if row.ChatID == row.ID && !readsTheChat(row, caller) {
		return requestTitle(pgtype.Text{}, row.Prompt)
	}
	return requestTitle(row.Title, row.Prompt)
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

func (r *RunDesk) summary(row dbsqlc.ListAgentRequestsByRow, caller auth.Principal, state buildState, allowed []Action) requestSummaryJSON {
	out := requestSummaryJSON{
		ID:            row.ID.String(),
		Kind:          Kind(listedSource(row)),
		Intent:        Intent(row.Intent),
		Investigation: row.Investigation,
		Trigger:       row.Trigger,
		Title:         requestTitle(row.Title, row.Prompt),
		PinnedAt:      httpapi.FormatOptionalTime(optionalTime(row.PinnedAt)),
		Prompt:        row.Prompt,
		Status:        Status(row.Status),
		State:         State(row.State),
		ResultKind:    ReportedResultKind(row.ResultKind, row.ObservationID),
		PanelID:       optionalText(row.PanelID),
		BaseVersion:   optionalInt(row.BaseVersion),
		CommentID:     optionalUUID(row.CommentID),
		Error:         httpapi.OptionalString(row.Error.String),
		Origin:        httpapi.OptionalString(row.Origin.String),
		OriginClient:  httpapi.OptionalString(row.OriginClient.String),
		CreatedAt:     httpapi.FormatTime(row.CreatedAt.Time),
		StartedAt:     httpapi.FormatOptionalTime(optionalTime(row.StartedAt)),
		FinishedAt:    httpapi.FormatOptionalTime(optionalTime(row.FinishedAt)),
		RequestedBy:   authorJSON{Kind: row.RequestedByKind, ID: row.RequestedByID.String(), Name: row.RequestedByName},

		FailureCode:  httpapi.OptionalString(row.FailureCode.String),
		Build:        state.build,
		BuildSavedAt: httpapi.FormatOptionalTime(optionalTime(row.BuildSavedAt)),
		BuildState:   state.derived(),
		ContinuesID:  optionalUUID(row.ContinuesID),
		ContinuedBy:  state.continuedBy,
		RetriedBy:    state.retriedBy,
		ContinueNote: httpapi.OptionalString(row.ContinueNote.String),

		ObservationID:    visibleObservationID(row),
		ObservationTitle: optionalText(row.ObservationTitle),
		ChatID:           optionalUUID(row.ChatID),
		AllowedActions:   actionStrings(allowed),
	}
	out.Anchor, out.BlockID = pointShownTo(row.RequestedByKind, row.RequestedByID, row.Anchor, row.BlockID, caller)
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

func visibleObservationID(row dbsqlc.ListAgentRequestsByRow) *string {
	if !row.ObservationTitle.Valid {
		return nil
	}
	return optionalUUID(row.ObservationID)
}
