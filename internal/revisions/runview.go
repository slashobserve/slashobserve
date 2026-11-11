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

type RunReader struct {
	service *Service
	opts    ObserveOptions
}

func NewRunReader(service *Service, opts ObserveOptions) *RunReader {
	return &RunReader{service: service, opts: opts}
}

func (r *RunReader) Run(ctx context.Context, id pgtype.UUID, caller auth.Principal, maxSteps int) (json.RawMessage, error) {
	row, err := r.service.GetObserve(ctx, id, caller)
	if err == nil {
		err = r.service.checkVisible(ctx, row.DashboardID, caller)
	}
	if err != nil {
		return nil, err
	}
	out, err := r.render(ctx, row, caller)
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

func (r *RunReader) render(ctx context.Context, row dbsqlc.AgentRequest, caller auth.Principal) (agentRequestJSON, error) {
	states, err := r.service.buildStates(ctx, []pgtype.UUID{row.ID})
	if err != nil {
		return agentRequestJSON{}, err
	}
	position, err := r.service.queuePosition(ctx, row)
	if err != nil {
		return agentRequestJSON{}, err
	}
	mayStart, err := r.service.mayStart(ctx, row, caller)
	if err != nil {
		return agentRequestJSON{}, err
	}
	allowed := allowedActions(factsOf(row, states[row.ID], caller, mayStart))
	out := r.view(ctx, row, caller, states[row.ID], allowed, position)
	if mayHaveFinding(row) && r.opts.Finding != nil {
		if err := r.attachFinding(ctx, row, &out); err != nil {
			return agentRequestJSON{}, err
		}
	}
	return out, nil
}

func mayHaveFinding(row dbsqlc.AgentRequest) bool {
	return row.Source != string(SourceSuggest) && !row.ResultVersion.Valid
}

func (r *RunReader) attachFinding(ctx context.Context, row dbsqlc.AgentRequest, out *agentRequestJSON) error {
	raw, found, err := r.opts.Finding(ctx, row.ID)
	if err != nil || !found {
		return err
	}
	out.Finding = raw
	if out.Status != StatusSucceeded || r.opts.FindingURL == nil {
		return nil
	}
	var finding struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &finding); err != nil {
		return fmt.Errorf("read finding id: %w", err)
	}
	url := r.opts.FindingURL(finding.ID)
	out.URL = &url
	return nil
}

func (r *RunReader) view(ctx context.Context, row dbsqlc.AgentRequest, caller auth.Principal, state buildState, allowed []Action, position *int) agentRequestJSON {
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

		PinnedAt:   formatOptionalTime(optionalTime(row.PinnedAt)),
		ArchivedAt: formatOptionalTime(optionalTime(row.ArchivedAt)),
		From:       formatOptionalTime(optionalTime(row.ContextFrom)),
		To:         formatOptionalTime(optionalTime(row.ContextTo)),
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
	return out
}
