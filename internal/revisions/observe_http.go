package revisions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	defaultObserveLimit = 20
	maxObserveLimit     = 100
	maxQueryRunes       = 200
)

var (
	errWindowPair    = errors.New("from and to go together")
	errWindowTime    = errors.New("from and to must be relative times like now-1h or epoch milliseconds")
	errWindowOrder   = errors.New("from must be before to")
	errWindowTooLong = errors.New("the time range is too long")
)

type ObserveOptions struct {
	Provider     string
	Model        string
	Effort       string
	DashboardURL func(dashboardID, title string) string
	FindingURL   func(findingID string) string
	Finding      func(ctx context.Context, requestID pgtype.UUID) (json.RawMessage, bool, error)
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
	Kind        *string `json:"kind"`
	PanelID     *string `json:"panel_id"`
	From        *string `json:"from"`
	To          *string `json:"to"`
}

type patchObserveRequest struct {
	Title  json.RawMessage `json:"title"`
	Pinned *bool           `json:"pinned"`
}

type agentInfoJSON struct {
	Available bool    `json:"available"`
	Provider  *string `json:"provider"`
	Model     *string `json:"model"`
	Effort    *string `json:"effort"`
}

type agentRequestJSON struct {
	ID            string             `json:"id"`
	Kind          string             `json:"kind"`
	Source        Source             `json:"source"`
	Title         string             `json:"title"`
	Prompt        string             `json:"prompt"`
	DashboardID   *string            `json:"dashboard_id"`
	BaseVersion   *int               `json:"base_version"`
	PanelID       *string            `json:"panel_id"`
	CommentID     *string            `json:"comment_id"`
	Status        Status             `json:"status"`
	Attempts      int                `json:"attempts"`
	ResultVersion *int               `json:"result_version"`
	URL           *string            `json:"url"`
	Error         *string            `json:"error"`
	FailureCode   *string            `json:"failure_code"`
	Steps         []Step             `json:"steps"`
	Usage         *Usage             `json:"usage"`
	Build         *buildProgressJSON `json:"build"`
	BuildSavedAt  *string            `json:"build_saved_at"`
	RequestedBy   authorJSON         `json:"requested_by"`
	Origin        *string            `json:"origin"`
	OriginClient  *string            `json:"origin_client"`
	CreatedAt     string             `json:"created_at"`
	StartedAt     *string            `json:"started_at"`
	FinishedAt    *string            `json:"finished_at"`

	QueuePosition     *int        `json:"queue_position"`
	CancelRequestedAt *string     `json:"cancel_requested_at"`
	CanceledBy        *authorJSON `json:"canceled_by"`
	CanCancel         bool        `json:"can_cancel"`
	ContinuesID       *string     `json:"continues_id"`
	ContinueNote      *string     `json:"continue_note"`
	ContinuedBy       *string     `json:"continued_by"`

	PinnedAt   *string         `json:"pinned_at"`
	ArchivedAt *string         `json:"archived_at"`
	From       *string         `json:"from"`
	To         *string         `json:"to"`
	Finding    json.RawMessage `json:"finding"`
}

type requestSummaryJSON struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	Source         Source  `json:"source"`
	Title          string  `json:"title"`
	PinnedAt       *string `json:"pinned_at"`
	Prompt         string  `json:"prompt"`
	Status         Status  `json:"status"`
	DashboardID    *string `json:"dashboard_id"`
	DashboardTitle *string `json:"dashboard_title"`
	PanelID        *string `json:"panel_id"`
	ResultVersion  *int    `json:"result_version"`
	URL            *string `json:"url"`
	Error          *string `json:"error"`
	Origin         *string `json:"origin"`
	OriginClient   *string `json:"origin_client"`
	CreatedAt      string  `json:"created_at"`
	StartedAt      *string `json:"started_at"`
	FinishedAt     *string `json:"finished_at"`

	FailureCode  *string            `json:"failure_code"`
	Build        *buildProgressJSON `json:"build"`
	BuildSavedAt *string            `json:"build_saved_at"`
	ContinuesID  *string            `json:"continues_id"`
	ContinuedBy  *string            `json:"continued_by"`
	ContinueNote *string            `json:"continue_note"`
}

type requestListJSON struct {
	Requests   []requestSummaryJSON `json:"requests"`
	NextCursor *string              `json:"next_cursor"`
}

type agentRequestEnvelopeJSON struct {
	Request agentRequestJSON `json:"request"`
}

func (h *ObserveHandler) Register(api auth.Routes) {
	api.GET("/agent", auth.RoleViewer, h.getAgent)
	api.POST("/observe", auth.RoleViewer, h.createObserve)
	api.GET("/observe", auth.RoleViewer, h.listObserve)
	api.GET("/observe/:id", auth.RoleViewer, h.getObserve)
	api.PATCH("/observe/:id", auth.RoleViewer, h.patchObserve)
	api.DELETE("/observe/:id", auth.RoleViewer, h.archiveObserve)
	api.GET("/observe/:id/build", auth.RoleViewer, h.getBuild)
	api.POST("/observe/:id/build/save", auth.RoleViewer, h.saveBuild)
	api.POST("/observe/:id/cancel", auth.RoleViewer, h.cancelObserve)
	api.POST("/observe/:id/continue", auth.RoleViewer, h.continueObserve)
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
	switch {
	case req.Kind == nil || *req.Kind == kindBuild:
		if req.PanelID != nil || req.From != nil || req.To != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "panel_id, from and to need kind investigate")
			return
		}
	case *req.Kind == kindInvestigate:
		h.createInvestigation(c, req, prompt)
		return
	default:
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "kind must be build or investigate")
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
	h.replyRequest(c, http.StatusAccepted, row)
}

func (h *ObserveHandler) createInvestigation(c *gin.Context, req observeRequest, prompt string) {
	if req.BaseVersion != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "base_version is not allowed for kind investigate")
		return
	}
	params := InvestigateParams{Prompt: prompt, By: httpapi.Caller(c)}
	if req.DashboardID != nil {
		id, err := db.ParseUUID(*req.DashboardID)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "dashboard_id must be a UUID")
			return
		}
		params.DashboardID = id
	}
	if req.PanelID != nil {
		if !params.DashboardID.Valid {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "panel_id needs dashboard_id")
			return
		}
		if err := ValidPanelID(*req.PanelID); err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		params.PanelID = *req.PanelID
	}
	var err error
	if params.From, params.To, err = investigateWindow(req.From, req.To, time.Now()); err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !h.service.Available() {
		httpapi.AbortError(c, http.StatusServiceUnavailable, "agent_unavailable", ErrUnavailable.Error())
		return
	}
	row, err := h.service.SubmitInvestigate(c.Request.Context(), params)
	if err != nil {
		var idAttrs []any
		if params.DashboardID.Valid {
			idAttrs = append(idAttrs, dashboardAttr(params.DashboardID))
		}
		h.service.fail(c, "enqueue investigate request", err, idAttrs...)
		return
	}
	c.Header("Location", "/api/v1/observe/"+row.ID.String())
	h.replyRequest(c, http.StatusAccepted, row)
}

func investigateWindow(from, to *string, now time.Time) (*time.Time, *time.Time, error) {
	if from == nil && to == nil {
		return nil, nil, nil
	}
	if from == nil || to == nil {
		return nil, nil, errWindowPair
	}
	start, err := spec.ResolveTime(*from, now, false, spec.TimeOptions{})
	if err != nil {
		return nil, nil, errWindowTime
	}
	end, err := spec.ResolveTime(*to, now, true, spec.TimeOptions{})
	if err != nil {
		return nil, nil, errWindowTime
	}
	switch {
	case !start.Before(end):
		return nil, nil, errWindowOrder
	case end.Sub(start) > queryproxy.MaxRange:
		return nil, nil, errWindowTooLong
	}
	start, end = start.UTC(), end.UTC()
	return &start, &end, nil
}

func (h *ObserveHandler) enqueue(c *gin.Context, dashboardID pgtype.UUID, baseVersion *int, prompt string) (dbsqlc.AgentRequest, bool) {
	var row dbsqlc.AgentRequest
	var err error
	if dashboardID.Valid {
		row, err = h.service.SubmitFollowUp(c.Request.Context(), dashboardID, baseVersion, "", prompt, httpapi.Caller(c))
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

func (h *ObserveHandler) listObserve(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultObserveLimit, maxObserveLimit)
	if !ok {
		return
	}
	filter, ok := listFilter(c)
	if !ok {
		return
	}
	rows, next, err := h.service.ListMine(c.Request.Context(), httpapi.Caller(c), filter, limit, c.Query("cursor"))
	if err != nil {
		h.service.fail(c, "list agent requests", err, principalAttrs(c)...)
		return
	}
	ids := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	states, err := h.service.buildStates(c.Request.Context(), ids)
	if err != nil {
		h.service.fail(c, "list agent request build state", err, principalAttrs(c)...)
		return
	}
	body := requestListJSON{Requests: make([]requestSummaryJSON, len(rows))}
	for i, row := range rows {
		body.Requests[i] = h.summary(row, states[row.ID])
	}
	if next != "" {
		body.NextCursor = &next
	}
	c.JSON(http.StatusOK, body)
}

func listFilter(c *gin.Context) (ListFilter, bool) {
	var filter ListFilter
	if raw, present := c.GetQuery("source"); present {
		switch Source(raw) {
		case SourceObserve, SourcePrompt, SourceComment, SourceInvestigate:
			filter.Source = raw
		default:
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "source must be observe, prompt, comment or investigate")
			return filter, false
		}
	}
	if raw, present := c.GetQuery("q"); present {
		q := strings.TrimSpace(raw)
		if n := utf8.RuneCountInString(q); n < 1 || n > maxQueryRunes || !utf8.ValidString(q) || strings.ContainsRune(q, 0) {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", fmt.Sprintf("q must be 1 to %d characters", maxQueryRunes))
			return filter, false
		}
		filter.Query = q
	}
	if raw, present := c.GetQuery("pinned"); present {
		switch raw {
		case "true", "false":
			pinned := raw == "true"
			filter.Pinned = &pinned
		default:
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "pinned must be true or false")
			return filter, false
		}
	}
	if raw, present := c.GetQuery("dashboard_id"); present {
		id, err := db.ParseUUID(raw)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "dashboard_id must be a UUID")
			return filter, false
		}
		filter.DashboardID = id
	}
	return filter, true
}

func requestTitle(title pgtype.Text, prompt string) string {
	if title.Valid {
		return title.String
	}
	return DefaultTitle(prompt)
}

func (h *ObserveHandler) summary(row dbsqlc.ListAgentRequestsByRow, state buildState) requestSummaryJSON {
	out := requestSummaryJSON{
		ID:           row.ID.String(),
		Kind:         Kind(Source(row.Source)),
		Source:       Source(row.Source),
		Title:        requestTitle(row.Title, row.Prompt),
		PinnedAt:     formatOptionalTime(optionalTime(row.PinnedAt)),
		Prompt:       row.Prompt,
		Status:       Status(row.Status),
		PanelID:      optionalText(row.PanelID),
		Error:        httpapi.OptionalString(row.Error.String),
		Origin:       httpapi.OptionalString(row.Origin.String),
		OriginClient: httpapi.OptionalString(row.OriginClient.String),
		CreatedAt:    httpapi.FormatTime(row.CreatedAt.Time),
		StartedAt:    formatOptionalTime(optionalTime(row.StartedAt)),
		FinishedAt:   formatOptionalTime(optionalTime(row.FinishedAt)),

		FailureCode:  httpapi.OptionalString(row.FailureCode.String),
		Build:        state.build,
		BuildSavedAt: formatOptionalTime(optionalTime(row.BuildSavedAt)),
		ContinuesID:  optionalUUID(row.ContinuesID),
		ContinuedBy:  state.continuedBy,
		ContinueNote: httpapi.OptionalString(row.ContinueNote.String),
	}
	if row.DashboardID.Valid {
		id := row.DashboardID.String()
		out.DashboardID = &id
		out.DashboardTitle = optionalText(row.DashboardTitle)
		if out.Status == StatusSucceeded && out.Kind == kindBuild && h.opts.DashboardURL != nil {
			url := h.opts.DashboardURL(id, row.DashboardTitle.String)
			out.URL = &url
		}
	}
	if out.Status == StatusSucceeded && row.FindingID.Valid && h.opts.FindingURL != nil {
		url := h.opts.FindingURL(row.FindingID.String())
		out.URL = &url
	}
	if row.ResultVersion.Valid {
		n := int(row.ResultVersion.Int32)
		out.ResultVersion = &n
	}
	return out
}

func optionalUUID(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	s := id.String()
	return &s
}

func optionalText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func (h *ObserveHandler) getObserve(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	row, err := h.service.GetObserve(c.Request.Context(), id, httpapi.Caller(c))
	if err == nil {
		err = h.service.checkVisible(c.Request.Context(), row.DashboardID, httpapi.Caller(c))
	}
	if err != nil {
		h.service.fail(c, "get agent request", err, slog.String("agent_request_id", id.String()))
		return
	}
	h.replyRequest(c, http.StatusOK, row)
}

func (h *ObserveHandler) patchObserve(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req patchObserveRequest
	if !httpapi.DecodeBody(c, &req, maxRequestBytes) {
		return
	}
	if req.Title == nil && req.Pinned == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "send title or pinned")
		return
	}
	var title *string
	clearTitle := bytes.Equal(req.Title, []byte("null"))
	if req.Title != nil && !clearTitle {
		var raw string
		cleaned, valid := "", json.Unmarshal(req.Title, &raw) == nil
		if valid {
			cleaned, valid = CleanTitle(raw)
		}
		if !valid {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", fmt.Sprintf("title must be 1 to %d characters on one line", MaxTitleRunes))
			return
		}
		title = &cleaned
	}
	row, err := h.service.UpdateListFields(c.Request.Context(), id, httpapi.Caller(c), title, clearTitle, req.Pinned)
	if err != nil {
		h.failBuild(c, "update agent request", err, slog.String("agent_request_id", id.String()))
		return
	}
	h.replyRequest(c, http.StatusOK, row)
}

func (h *ObserveHandler) archiveObserve(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.Archive(c.Request.Context(), id, httpapi.Caller(c)); err != nil {
		h.failBuild(c, "archive agent request", err, slog.String("agent_request_id", id.String()))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ObserveHandler) render(c *gin.Context, row dbsqlc.AgentRequest) (agentRequestJSON, error) {
	ctx := c.Request.Context()
	states, err := h.service.buildStates(ctx, []pgtype.UUID{row.ID})
	if err != nil {
		return agentRequestJSON{}, err
	}
	position, err := h.service.queuePosition(ctx, row)
	if err != nil {
		return agentRequestJSON{}, err
	}
	out := h.view(c, row, states[row.ID], position)
	if row.Source == string(SourceInvestigate) && h.opts.Finding != nil {
		if err := h.attachFinding(ctx, row, &out); err != nil {
			return agentRequestJSON{}, err
		}
	}
	return out, nil
}

func (h *ObserveHandler) attachFinding(ctx context.Context, row dbsqlc.AgentRequest, out *agentRequestJSON) error {
	raw, found, err := h.opts.Finding(ctx, row.ID)
	if err != nil || !found {
		return err
	}
	out.Finding = raw
	if out.Status != StatusSucceeded || h.opts.FindingURL == nil {
		return nil
	}
	var finding struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &finding); err != nil {
		return fmt.Errorf("read finding id: %w", err)
	}
	url := h.opts.FindingURL(finding.ID)
	out.URL = &url
	return nil
}

func (h *ObserveHandler) view(c *gin.Context, row dbsqlc.AgentRequest, state buildState, position *int) agentRequestJSON {
	out := agentRequestJSON{
		ID:          row.ID.String(),
		Kind:        Kind(Source(row.Source)),
		Source:      Source(row.Source),
		Title:       requestTitle(row.Title, row.Prompt),
		Prompt:      row.Prompt,
		Status:      Status(row.Status),
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
		Build:        state.progressFor(requesterOf(row), httpapi.Caller(c)),
		BuildSavedAt: formatOptionalTime(optionalTime(row.BuildSavedAt)),

		QueuePosition:     position,
		CancelRequestedAt: formatOptionalTime(optionalTime(row.CancelRequestedAt)),
		CanCancel:         canCancel(row, httpapi.Caller(c)),
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
		if out.Status == StatusSucceeded && out.Kind == kindBuild && h.opts.DashboardURL != nil {
			url := h.opts.DashboardURL(id, "")
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
	out.Usage = h.service.decodeUsage(c.Request.Context(), out.ID, row.RunUsage)
	return out
}

func (s *Service) decodeUsage(ctx context.Context, requestID string, raw []byte) *Usage {
	if len(raw) == 0 {
		return nil
	}
	var usage Usage
	if err := json.Unmarshal(raw, &usage); err != nil {
		s.logger.ErrorContext(ctx, "decode agent request usage", logattrs.Failure("agent_request_usage_decode_failed", err, slog.String("agent_request_id", requestID))...)
		return nil
	}
	return &usage
}
