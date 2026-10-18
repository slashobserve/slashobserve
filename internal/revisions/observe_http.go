package revisions

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type ObserveOptions struct {
	Provider     string
	Model        string
	Effort       string
	DashboardURL func(dashboardID string) string
}

type ObserveHandler struct {
	service *Service
	opts    ObserveOptions
}

func NewObserveHandler(service *Service, opts ObserveOptions) *ObserveHandler {
	return &ObserveHandler{service: service, opts: opts}
}

type observeRequest struct {
	Prompt      *string `json:"prompt"`
	DashboardID *string `json:"dashboard_id"`
	BaseVersion *int    `json:"base_version"`
}

type agentInfoJSON struct {
	Available bool    `json:"available"`
	Provider  *string `json:"provider"`
	Model     *string `json:"model"`
	Effort    *string `json:"effort"`
}

type agentRequestJSON struct {
	ID            string     `json:"id"`
	Source        Source     `json:"source"`
	Prompt        string     `json:"prompt"`
	DashboardID   *string    `json:"dashboard_id"`
	BaseVersion   *int       `json:"base_version"`
	PanelID       *string    `json:"panel_id"`
	CommentID     *string    `json:"comment_id"`
	Status        Status     `json:"status"`
	Attempts      int        `json:"attempts"`
	ResultVersion *int       `json:"result_version"`
	URL           *string    `json:"url"`
	Error         *string    `json:"error"`
	Steps         []Step     `json:"steps"`
	Usage         *Usage     `json:"usage"`
	RequestedBy   authorJSON `json:"requested_by"`
	CreatedAt     string     `json:"created_at"`
	StartedAt     *string    `json:"started_at"`
	FinishedAt    *string    `json:"finished_at"`
}

type agentRequestEnvelopeJSON struct {
	Request agentRequestJSON `json:"request"`
}

func (h *ObserveHandler) Register(api *gin.RouterGroup) {
	api.GET("/agent", h.getAgent)
	api.POST("/observe", h.createObserve)
	api.GET("/observe/:id", h.getObserve)
}

func (h *ObserveHandler) getAgent(c *gin.Context) {
	info := agentInfoJSON{Available: h.service.Available()}
	if info.Available {
		info.Provider, info.Model = &h.opts.Provider, &h.opts.Model
		if h.opts.Effort != "" {
			info.Effort = &h.opts.Effort
		}
	}
	c.JSON(http.StatusOK, info)
}

func (h *ObserveHandler) createObserve(c *gin.Context) {
	var req observeRequest
	if !httpapi.DecodeBody(c, &req, maxRequestBytes) {
		return
	}
	if req.Prompt == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "prompt is required")
		return
	}
	prompt, ok := CleanPrompt(*req.Prompt)
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", fmt.Sprintf("prompt must be 1 to %d characters without control or bidirectional override characters", MaxPromptRunes))
		return
	}
	if req.DashboardID == nil && req.BaseVersion != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "base_version needs dashboard_id")
		return
	}
	var dashboardID pgtype.UUID
	if req.DashboardID != nil {
		id, err := db.ParseUUID(*req.DashboardID)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "dashboard_id must be a UUID")
			return
		}
		dashboardID = id
	}
	if req.BaseVersion != nil && (*req.BaseVersion < 1 || *req.BaseVersion > math.MaxInt32) {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "base_version must be a positive 32-bit integer")
		return
	}
	if !h.service.Available() {
		httpapi.AbortError(c, http.StatusServiceUnavailable, "agent_unavailable", ErrUnavailable.Error())
		return
	}
	row, queued := h.enqueue(c, dashboardID, req.BaseVersion, prompt)
	if !queued {
		return
	}
	c.Header("Location", "/api/v1/observe/"+row.ID.String())
	c.JSON(http.StatusAccepted, agentRequestEnvelopeJSON{Request: h.view(c, row)})
}

func (h *ObserveHandler) enqueue(c *gin.Context, dashboardID pgtype.UUID, baseVersion *int, prompt string) (dbsqlc.AgentRequest, bool) {
	var row dbsqlc.AgentRequest
	var err error
	if dashboardID.Valid {
		row, err = h.service.SubmitFollowUp(c.Request.Context(), dashboardID, baseVersion, prompt, httpapi.Caller(c))
	} else {
		row, err = h.service.SubmitObserve(c.Request.Context(), httpapi.Caller(c), prompt)
	}
	if err != nil {
		var idAttrs []any
		if dashboardID.Valid {
			idAttrs = append(idAttrs, dashboardAttr(dashboardID))
		}
		h.service.fail(c, "enqueue observe request", err, idAttrs...)
		return dbsqlc.AgentRequest{}, false
	}
	return row, true
}

func (h *ObserveHandler) getObserve(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	row, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		h.service.fail(c, "get agent request", err, slog.String("agent_request_id", id.String()))
		return
	}
	c.JSON(http.StatusOK, agentRequestEnvelopeJSON{Request: h.view(c, row)})
}

func (h *ObserveHandler) view(c *gin.Context, row dbsqlc.AgentRequest) agentRequestJSON {
	out := agentRequestJSON{
		ID:       row.ID.String(),
		Source:   Source(row.Source),
		Prompt:   row.Prompt,
		Status:   Status(row.Status),
		Attempts: int(row.Attempts),
		Error:    httpapi.OptionalString(row.Error.String),
		Steps:    []Step{},
		RequestedBy: authorJSON{
			Kind: row.RequestedByKind, ID: row.RequestedByID.String(), Name: row.RequestedByName, Email: row.RequestedByEmail.String,
		},
		CreatedAt:  httpapi.FormatTime(row.CreatedAt.Time),
		StartedAt:  formatOptionalTime(optionalTime(row.StartedAt)),
		FinishedAt: formatOptionalTime(optionalTime(row.FinishedAt)),
	}
	if row.DashboardID.Valid {
		id := row.DashboardID.String()
		out.DashboardID = &id
		if out.Status == StatusSucceeded && h.opts.DashboardURL != nil {
			url := h.opts.DashboardURL(id)
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
			h.service.logger.ErrorContext(c.Request.Context(), "decode agent request steps", logattrs.Failure("agent_request_steps_decode_failed", err, slog.String("agent_request_id", out.ID))...)
		}
		out.Steps = []Step{}
	}
	if len(row.RunUsage) > 0 {
		var usage Usage
		if err := json.Unmarshal(row.RunUsage, &usage); err != nil {
			h.service.logger.ErrorContext(c.Request.Context(), "decode agent request usage", logattrs.Failure("agent_request_usage_decode_failed", err, slog.String("agent_request_id", out.ID))...)
		} else {
			out.Usage = &usage
		}
	}
	return out
}
