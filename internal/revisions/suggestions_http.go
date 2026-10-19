package revisions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxSuggestionBodyBytes = MaxDraftBytes + 32<<10

type suggestionRequest struct {
	Version *int            `json:"version"`
	PanelID *string         `json:"panel_id"`
	Prompt  *string         `json:"prompt"`
	Panel   json.RawMessage `json:"panel"`
}

type suggestionJSON struct {
	ID           string          `json:"id"`
	DashboardID  string          `json:"dashboard_id"`
	PanelID      string          `json:"panel_id"`
	BaseVersion  int             `json:"base_version"`
	Prompt       string          `json:"prompt"`
	Status       Status          `json:"status"`
	Steps        []Step          `json:"steps"`
	Panel        json.RawMessage `json:"panel"`
	Error        *string         `json:"error"`
	RequestedBy  authorJSON      `json:"requested_by"`
	Origin       *string         `json:"origin"`
	OriginClient *string         `json:"origin_client"`
	CreatedAt    string          `json:"created_at"`
	StartedAt    *string         `json:"started_at"`
	FinishedAt   *string         `json:"finished_at"`
}

type suggestionEnvelopeJSON struct {
	Suggestion suggestionJSON `json:"suggestion"`
}

type draftErrorJSON struct {
	Error  string       `json:"error"`
	Code   string       `json:"code"`
	Issues []spec.Issue `json:"issues"`
}

func (s *Service) createSuggestion(c *gin.Context) {
	dashboardID, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req suggestionRequest
	if !httpapi.DecodeBody(c, &req, maxSuggestionBodyBytes) {
		return
	}
	if req.Version == nil || req.PanelID == nil || req.Prompt == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "version, panel_id and prompt are required")
		return
	}
	if *req.Version < 1 || *req.Version > math.MaxInt32 {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "version must be a positive 32-bit integer")
		return
	}
	if err := ValidPanelID(*req.PanelID); err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	prompt, ok := CleanPrompt(*req.Prompt)
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", fmt.Sprintf("prompt must be 1 to %d characters without control or bidirectional override characters", MaxPromptRunes))
		return
	}
	draft, ok := draftOf(req.Panel)
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "panel must be a JSON object or null")
		return
	}
	ctx := c.Request.Context()
	row, err := s.SubmitSuggestion(ctx, dashboardID, *req.Version, *req.PanelID, prompt, draft, httpapi.Caller(c))
	if err != nil {
		s.failSuggestion(c, "enqueue suggestion", err, dashboardAttr(dashboardID))
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+dashboardID.String()+"/suggestions/"+row.ID.String())
	c.JSON(http.StatusAccepted, suggestionEnvelopeJSON{Suggestion: s.newSuggestionJSON(c, row)})
}

func (s *Service) getSuggestion(c *gin.Context) {
	dashboardID, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	id, ok := httpapi.UUIDParam(c, "suggestion_id")
	if !ok {
		return
	}
	row, err := s.GetSuggestion(c.Request.Context(), dashboardID, id, httpapi.Caller(c))
	if err != nil {
		s.fail(c, "get suggestion", err, dashboardAttr(dashboardID), slog.String("agent_request_id", id.String()))
		return
	}
	c.JSON(http.StatusOK, suggestionEnvelopeJSON{Suggestion: s.newSuggestionJSON(c, row)})
}

func (s *Service) failSuggestion(c *gin.Context, action string, err error, idAttrs ...any) {
	var invalid *DraftError
	switch {
	case errors.As(err, &invalid):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, draftErrorJSON{Error: invalid.Error(), Code: "invalid_spec", Issues: invalid.Issues})
	case errors.Is(err, ErrDraftTooLarge):
		httpapi.AbortError(c, http.StatusRequestEntityTooLarge, "too_large", err.Error())
	case errors.Is(err, ErrTooManySuggestions):
		httpapi.AbortError(c, http.StatusTooManyRequests, "too_many_suggestions", err.Error())
	default:
		s.fail(c, action, err, idAttrs...)
	}
}

func draftOf(raw json.RawMessage) (json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(raw)
	switch {
	case len(trimmed) == 0, bytes.Equal(trimmed, []byte("null")):
		return nil, true
	case trimmed[0] == '{':
		return trimmed, true
	}
	return nil, false
}

func (s *Service) newSuggestionJSON(c *gin.Context, row dbsqlc.AgentRequest) suggestionJSON {
	out := suggestionJSON{
		ID:           row.ID.String(),
		DashboardID:  row.DashboardID.String(),
		PanelID:      row.PanelID.String,
		BaseVersion:  int(row.BaseVersion.Int32),
		Prompt:       row.Prompt,
		Status:       Status(row.Status),
		Steps:        []Step{},
		RequestedBy:  authorJSON{Kind: row.RequestedByKind, ID: row.RequestedByID.String(), Name: row.RequestedByName, Email: row.RequestedByEmail.String},
		Origin:       httpapi.OptionalString(row.Origin.String),
		OriginClient: httpapi.OptionalString(row.OriginClient.String),
		CreatedAt:    httpapi.FormatTime(row.CreatedAt.Time),
		StartedAt:    formatOptionalTime(optionalTime(row.StartedAt)),
		FinishedAt:   formatOptionalTime(optionalTime(row.FinishedAt)),
	}
	switch out.Status {
	case StatusSucceeded:
		out.Panel = row.Suggestion
	case StatusFailed:
		out.Error = httpapi.OptionalString(row.Error.String)
	}
	if err := json.Unmarshal(row.Steps, &out.Steps); err != nil || out.Steps == nil {
		if err != nil {
			s.logger.ErrorContext(c.Request.Context(), "decode agent request steps", logattrs.Failure("agent_request_steps_decode_failed", err, slog.String("agent_request_id", out.ID))...)
		}
		out.Steps = []Step{}
	}
	return out
}
