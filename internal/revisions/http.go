package revisions

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxRequestBytes = 64 << 10
	defaultLimit    = 50
	maxLimit        = 200
)

type createRequest struct {
	Prompt      *string `json:"prompt"`
	BaseVersion *int    `json:"base_version"`
	PanelID     *string `json:"panel_id"`
	CommentID   *string `json:"comment_id"`
}

type authorJSON struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type revisionJSON struct {
	ID            string     `json:"id"`
	DashboardID   string     `json:"dashboard_id"`
	Source        Source     `json:"source"`
	CommentID     *string    `json:"comment_id"`
	PanelID       *string    `json:"panel_id"`
	Prompt        string     `json:"prompt"`
	BaseVersion   *int       `json:"base_version"`
	Status        Status     `json:"status"`
	Attempts      int        `json:"attempts"`
	ResultVersion *int       `json:"result_version"`
	Error         *string    `json:"error"`
	RequestedBy   authorJSON `json:"requested_by"`
	Origin        *string    `json:"origin"`
	OriginClient  *string    `json:"origin_client"`
	CreatedAt     string     `json:"created_at"`
	StartedAt     *string    `json:"started_at"`
	FinishedAt    *string    `json:"finished_at"`
}

type envelopeJSON struct {
	Revision revisionJSON `json:"revision"`
}

type listJSON struct {
	Revisions  []revisionJSON `json:"revisions"`
	NextCursor *string        `json:"next_cursor"`
}

func (s *Service) Register(api *gin.RouterGroup) {
	g := api.Group("/dashboards/:id/revisions")
	g.POST("", s.createRevision)
	g.GET("", s.listRevisions)
	g.GET("/:revision_id", s.getRevision)
}

func (s *Service) createRevision(c *gin.Context) {
	dashboardID, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req createRequest
	if !httpapi.DecodeBody(c, &req, maxRequestBytes) {
		return
	}
	if req.Prompt == nil || req.BaseVersion == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "prompt and base_version are required")
		return
	}
	prompt, ok := CleanPrompt(*req.Prompt)
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", fmt.Sprintf("prompt must be 1 to %d characters without control or bidirectional override characters", MaxPromptRunes))
		return
	}
	if *req.BaseVersion < 1 || *req.BaseVersion > math.MaxInt32 {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "base_version must be a positive 32-bit integer")
		return
	}
	panelID := ""
	if req.PanelID != nil {
		panelID = *req.PanelID
		if err := ValidPanelID(panelID); err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	commentID, ok := threadCommentID(c, req)
	if !ok {
		return
	}
	if !s.Available() {
		httpapi.AbortError(c, http.StatusServiceUnavailable, "agent_unavailable", "no agent is configured")
		return
	}
	var row dbsqlc.AgentRequest
	var err error
	if commentID.Valid {
		row, err = s.SubmitForThread(c.Request.Context(), dashboardID, commentID, req.BaseVersion, prompt, httpapi.Caller(c))
	} else {
		row, err = s.SubmitFollowUp(c.Request.Context(), dashboardID, req.BaseVersion, panelID, prompt, httpapi.Caller(c))
	}
	if err != nil {
		s.fail(c, "enqueue revision", err, dashboardAttr(dashboardID))
		return
	}
	rev := revisionFromRow(row)
	c.Header("Location", "/api/v1/dashboards/"+rev.DashboardID+"/revisions/"+rev.ID)
	c.JSON(http.StatusAccepted, envelopeJSON{Revision: newRevisionJSON(rev)})
}

func threadCommentID(c *gin.Context, req createRequest) (pgtype.UUID, bool) {
	if req.CommentID == nil {
		return pgtype.UUID{}, true
	}
	id, err := db.ParseUUID(*req.CommentID)
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "comment_id must be a UUID")
		return pgtype.UUID{}, false
	}
	if req.PanelID != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "send comment_id or panel_id, not both; a thread request uses its thread's panel")
		return pgtype.UUID{}, false
	}
	return id, true
}

func (s *Service) listRevisions(c *gin.Context) {
	dashboardID, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if _, err := s.db.Queries().GetDashboardLatestVersion(ctx, dashboardID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpapi.AbortError(c, http.StatusNotFound, "not_found", "dashboard not found")
			return
		}
		s.fail(c, "read latest dashboard version", err, dashboardAttr(dashboardID))
		return
	}
	items, next, err := s.List(ctx, dashboardID, limit, c.Query("cursor"))
	if err != nil {
		s.fail(c, "list revisions", err, dashboardAttr(dashboardID))
		return
	}
	body := listJSON{Revisions: make([]revisionJSON, len(items))}
	for i, item := range items {
		body.Revisions[i] = newRevisionJSON(item)
	}
	if next != "" {
		body.NextCursor = &next
	}
	c.JSON(http.StatusOK, body)
}

func (s *Service) getRevision(c *gin.Context) {
	dashboardID, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	id, ok := httpapi.UUIDParam(c, "revision_id")
	if !ok {
		return
	}
	rev, err := s.Get(c.Request.Context(), dashboardID, id)
	if err != nil {
		s.fail(c, "get revision", err, dashboardAttr(dashboardID), slog.String("agent_request_id", id.String()))
		return
	}
	c.JSON(http.StatusOK, envelopeJSON{Revision: newRevisionJSON(rev)})
}

func (s *Service) fail(c *gin.Context, action string, err error, idAttrs ...any) {
	ctx := c.Request.Context()
	if clientgone.Handle(c, s.logger, action, err) {
		return
	}
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrDashboardNotFound), errors.Is(err, ErrDashboardVersionNotFound), errors.Is(err, ErrCommentNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrUnknownPanel):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "unknown_panel", err.Error())
	case errors.Is(err, ErrTooManyRevisions):
		httpapi.AbortError(c, http.StatusTooManyRequests, "too_many_revisions", err.Error())
	case errors.Is(err, ErrTooManyRequests):
		httpapi.AbortError(c, http.StatusTooManyRequests, "too_many_requests", err.Error())
	case errors.Is(err, ErrAgentBusy):
		httpapi.AbortError(c, http.StatusTooManyRequests, "agent_busy", err.Error())
	case errors.Is(err, ErrRequestNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrUnavailable):
		httpapi.AbortError(c, http.StatusServiceUnavailable, "agent_unavailable", err.Error())
	case errors.Is(err, ErrInvalidCursor):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		s.logger.ErrorContext(ctx, action, logattrs.Failure("internal", err, append(idAttrs, principalAttrs(c)...)...)...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}

func principalAttrs(c *gin.Context) []any {
	principal := httpapi.Caller(c)
	return []any{slog.String("principal_kind", string(principal.Kind)), slog.String("principal_id", principal.ID)}
}

func dashboardAttr(id pgtype.UUID) any {
	return slog.String("dashboard_id", id.String())
}

func newRevisionJSON(r Revision) revisionJSON {
	out := revisionJSON{
		ID:            r.ID,
		DashboardID:   r.DashboardID,
		Source:        r.Source,
		CommentID:     httpapi.OptionalString(r.CommentID),
		PanelID:       httpapi.OptionalString(r.PanelID),
		Prompt:        r.Prompt,
		BaseVersion:   r.BaseVersion,
		Status:        r.Status,
		Attempts:      r.Attempts,
		ResultVersion: r.ResultVersion,
		Error:         httpapi.OptionalString(r.Error),
		RequestedBy:   authorJSON{Kind: string(r.RequestedBy.Kind), ID: r.RequestedBy.ID, Name: r.RequestedBy.Name, Email: r.RequestedBy.Email},
		Origin:        httpapi.OptionalString(r.Origin),
		OriginClient:  httpapi.OptionalString(r.OriginClient),
		CreatedAt:     httpapi.FormatTime(r.CreatedAt),
		StartedAt:     formatOptionalTime(r.StartedAt),
		FinishedAt:    formatOptionalTime(r.FinishedAt),
	}
	return out
}

func formatOptionalTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := httpapi.FormatTime(*t)
	return &s
}
