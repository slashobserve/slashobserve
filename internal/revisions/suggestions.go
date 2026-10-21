package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const codeInvalidDraft spec.IssueCode = "invalid_request"

func (s *Service) SubmitSuggestion(ctx context.Context, dashboardID pgtype.UUID, version int, panelID, prompt string, draft json.RawMessage, by auth.Principal) (dbsqlc.AgentRequest, error) {
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("requester id: %w", err)
	}
	if err := s.checkSuggestion(ctx, dashboardID, version, panelID, draft); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	originKind, originClient := origin.From(ctx).Columns()
	var row dbsqlc.AgentRequest
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockSuggestRequests(ctx); err != nil {
			return fmt.Errorf("lock suggest requests: %w", err)
		}
		mine, err := q.CountActiveSuggestRequestsBy(ctx, requesterID)
		if err != nil {
			return fmt.Errorf("count active suggest requests of requester: %w", err)
		}
		if mine >= MaxActiveSuggestPerPrincipal {
			return ErrTooManySuggestions
		}
		all, err := q.CountActiveSuggestRequests(ctx)
		if err != nil {
			return fmt.Errorf("count active suggest requests: %w", err)
		}
		if all >= MaxActiveSuggest {
			return ErrAgentBusy
		}
		row, err = q.EnqueueSuggestRequest(ctx, dbsqlc.EnqueueSuggestRequestParams{
			DashboardID:      dashboardID,
			Prompt:           prompt,
			PanelID:          pgtype.Text{String: panelID, Valid: true},
			BaseVersion:      httpapi.Int32(version),
			Draft:            draft,
			RequestedByKind:  string(by.Kind),
			RequestedByID:    requesterID,
			RequestedByName:  by.Name,
			RequestedByEmail: pgtype.Text{String: by.Email, Valid: by.Email != ""},
			Origin:           originKind,
			OriginClient:     originClient,
		})
		if err != nil {
			return fmt.Errorf("enqueue suggest request: %w", err)
		}
		if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: WakeChannel, Payload: row.ID.String()}); err != nil {
			return fmt.Errorf("notify workers: %w", err)
		}
		return nil
	})
	return row, err
}

func (s *Service) checkSuggestion(ctx context.Context, dashboardID pgtype.UUID, version int, panelID string, draft json.RawMessage) error {
	if s.specs == nil {
		return fmt.Errorf("read version spec: no spec reader configured")
	}
	if len(draft) > MaxDraftBytes {
		return ErrDraftBodyTooLarge
	}
	if _, _, err := s.baseVersionFor(ctx, dashboardID, &version); err != nil {
		return err
	}
	base, err := s.specs.VersionSpec(ctx, dashboardID, version)
	if err != nil {
		return fmt.Errorf("read version spec: %w", err)
	}
	entry, ok := base.FindPanel(panelID)
	if !ok || !suggestible(entry.Panel.Type) {
		return ErrNotSuggestible
	}
	if len(draft) == 0 {
		return nil
	}
	return s.checkDraft(base, entry, draft)
}

func (s *Service) GetSuggestion(ctx context.Context, dashboardID, id pgtype.UUID, by auth.Principal) (dbsqlc.AgentRequest, error) {
	row, err := s.db.Queries().GetSuggestRequest(ctx, dbsqlc.GetSuggestRequestParams{ID: id, DashboardID: dashboardID})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.AgentRequest{}, ErrRequestNotFound
	}
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("get suggest request: %w", err)
	}
	if row.RequestedByKind != string(by.Kind) || row.RequestedByID.String() != by.ID {
		return dbsqlc.AgentRequest{}, ErrRequestNotFound
	}
	return row, nil
}

func (s *Service) checkDraft(base *spec.Dashboard, entry spec.PanelEntry, draft json.RawMessage) error {
	candidate, err := spec.WithPanel(base, entry.Panel.ID, draft)
	var invalid *spec.ValidationError
	switch {
	case errors.As(err, &invalid):
		return &DraftError{Issues: invalid.Issues}
	case err != nil:
		return fmt.Errorf("place draft panel: %w", err)
	}
	placed, ok := placedPanel(candidate, entry)
	if !ok {
		return &DraftError{Issues: []spec.Issue{{
			Path:    entry.Pointer,
			Code:    codeInvalidDraft,
			Message: fmt.Sprintf("the draft must keep id %q and stay a data or text panel", entry.Panel.ID),
		}}}
	}
	raw, err := json.Marshal(placed)
	if err != nil {
		return fmt.Errorf("marshal draft panel: %w", err)
	}
	if len(raw) > spec.MaxSuggestedPanelBytes {
		return ErrDraftTooLarge
	}
	canonical, err := json.Marshal(candidate)
	if err != nil {
		return fmt.Errorf("marshal draft dashboard: %w", err)
	}
	if issues := spec.NULIssues(canonical); len(issues) > 0 {
		return &DraftError{Issues: issues}
	}
	return nil
}

func placedPanel(candidate *spec.Dashboard, entry spec.PanelEntry) (*spec.Panel, bool) {
	for _, e := range candidate.AllPanels() {
		if e.Pointer == entry.Pointer {
			return e.Panel, e.Panel.ID == entry.Panel.ID && suggestible(e.Panel.Type)
		}
	}
	return nil, false
}

func suggestible(t spec.PanelType) bool {
	return t.IsData() || t == spec.Text
}
