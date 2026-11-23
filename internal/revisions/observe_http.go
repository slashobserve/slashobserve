package revisions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
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

var listableStates = []State{StateQueued, StateRunning, StateCanceling, StateSucceeded, StateFailed, StateCanceled}

var (
	errWindowPair      = errors.New("from and to go together")
	errWindowTime      = errors.New("from and to must be relative times like now-1h or epoch milliseconds")
	errWindowOrder     = errors.New("from must be before to")
	errWindowTooLong   = errors.New("the time range is too long")
	errWindowAfterAsOf = errors.New("to must not be after as_of")
)

type ObserveOptions struct {
	Provider           string
	Model              string
	Effort             string
	DashboardURL       func(dashboardID, title string) string
	ObservationURL     func(observationID string) string
	Finding            func(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (json.RawMessage, bool, error)
	SaveAnswer         AnswerSaver
	ShareAnswer        AnswerSharer
	LiveText           func(runID string, attempt int) (string, bool)
	ShareTitle         TitleSharer
	ObservationVisible func(ctx context.Context, by auth.Principal, id pgtype.UUID) (bool, error)
}

type ObserveHandler struct {
	service *Service
	opts    ObserveOptions
	runs    *RunDesk
}

func NewObserveHandler(service *Service, opts ObserveOptions) *ObserveHandler {
	return &ObserveHandler{service: service, opts: opts, runs: NewRunDesk(service, opts)}
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
	ID             string             `json:"id"`
	Kind           string             `json:"kind"`
	Intent         Intent             `json:"intent"`
	Investigation  bool               `json:"investigation"`
	Trigger        string             `json:"trigger"`
	Title          string             `json:"title"`
	Prompt         string             `json:"prompt"`
	DashboardID    *string            `json:"dashboard_id"`
	BaseVersion    *int               `json:"base_version"`
	PanelID        *string            `json:"panel_id"`
	CommentID      *string            `json:"comment_id"`
	Status         Status             `json:"status"`
	State          State              `json:"state"`
	ResultKind     ResultKind         `json:"result_kind"`
	Attempts       int                `json:"attempts"`
	Attempt        int                `json:"attempt"`
	ResultVersion  *int               `json:"result_version"`
	URL            *string            `json:"url"`
	Error          *string            `json:"error"`
	FailureCode    *string            `json:"failure_code"`
	Steps          []Step             `json:"steps"`
	LiveText       *string            `json:"live_text"`
	StepsTruncated bool               `json:"steps_truncated,omitempty"`
	Usage          *Usage             `json:"usage"`
	Build          *buildProgressJSON `json:"build"`
	BuildSavedAt   *string            `json:"build_saved_at"`
	RequestedBy    authorJSON         `json:"requested_by"`
	AssignedTo     *assigneeJSON      `json:"assigned_to"`
	Origin         *string            `json:"origin"`
	OriginClient   *string            `json:"origin_client"`
	CreatedAt      string             `json:"created_at"`
	StartedAt      *string            `json:"started_at"`
	FinishedAt     *string            `json:"finished_at"`

	QueuePosition     *int        `json:"queue_position"`
	CancelRequestedAt *string     `json:"cancel_requested_at"`
	CanceledBy        *authorJSON `json:"canceled_by"`
	CanCancel         bool        `json:"can_cancel"`
	ReadOnly          bool        `json:"read_only"`
	Redacted          bool        `json:"redacted"`
	AllowedActions    []string    `json:"allowed_actions"`
	ContinuesID       *string     `json:"continues_id"`
	ContinueNote      *string     `json:"continue_note"`
	ContinuedBy       *string     `json:"continued_by"`
	RetriedBy         *string     `json:"retried_by"`

	PinnedAt      *string         `json:"pinned_at"`
	ArchivedAt    *string         `json:"archived_at"`
	From          *string         `json:"from"`
	To            *string         `json:"to"`
	AsOf          *string         `json:"as_of"`
	ObservationID *string         `json:"observation_id"`
	SubjectID     *string         `json:"subject_observation_id"`
	ChatID        *string         `json:"chat_id"`
	SharedThread  *string         `json:"shared_thread_id"`
	Anchor        *anchor.Anchor  `json:"anchor"`
	BlockID       *string         `json:"block_id"`
	Finding       json.RawMessage `json:"finding"`

	ObservationRename SharedTitle `json:"observation_rename,omitempty"`
	Replayed          bool        `json:"replayed,omitempty"`
}

type assigneeJSON struct {
	Kind AssigneeKind `json:"kind"`
	ID   string       `json:"id"`
	Name *string      `json:"name"`
}

type requestSummaryJSON struct {
	ID             string     `json:"id"`
	Kind           string     `json:"kind"`
	Intent         Intent     `json:"intent"`
	Investigation  bool       `json:"investigation"`
	Trigger        string     `json:"trigger"`
	Title          string     `json:"title"`
	PinnedAt       *string    `json:"pinned_at"`
	Prompt         string     `json:"prompt"`
	Status         Status     `json:"status"`
	State          State      `json:"state"`
	ResultKind     ResultKind `json:"result_kind"`
	DashboardID    *string    `json:"dashboard_id"`
	DashboardTitle *string    `json:"dashboard_title"`
	PanelID        *string    `json:"panel_id"`
	BaseVersion    *int       `json:"base_version"`
	CommentID      *string    `json:"comment_id"`
	ResultVersion  *int       `json:"result_version"`
	URL            *string    `json:"url"`
	Error          *string    `json:"error"`
	Origin         *string    `json:"origin"`
	OriginClient   *string    `json:"origin_client"`
	CreatedAt      string     `json:"created_at"`
	StartedAt      *string    `json:"started_at"`
	FinishedAt     *string    `json:"finished_at"`
	RequestedBy    authorJSON `json:"requested_by"`

	FailureCode  *string            `json:"failure_code"`
	Build        *buildProgressJSON `json:"build"`
	BuildSavedAt *string            `json:"build_saved_at"`
	ContinuesID  *string            `json:"continues_id"`
	ContinuedBy  *string            `json:"continued_by"`
	RetriedBy    *string            `json:"retried_by"`
	ContinueNote *string            `json:"continue_note"`

	ObservationID    *string  `json:"observation_id"`
	ObservationTitle *string  `json:"observation_title"`
	ChatID           *string  `json:"chat_id"`
	AllowedActions   []string `json:"allowed_actions"`

	Anchor  *anchor.Anchor `json:"anchor"`
	BlockID *string        `json:"block_id"`
}

type requestCountJSON struct {
	Total  int64  `json:"total"`
	People *int64 `json:"people"`
}

type requestListJSON struct {
	Requests   []requestSummaryJSON `json:"requests"`
	NextCursor *string              `json:"next_cursor"`
	Summary    *requestCountJSON    `json:"summary"`
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
	api.POST("/observe/:id/observation", auth.RoleViewer, h.saveAnswer)
	api.POST("/observe/:id/share", auth.RoleViewer, h.shareAnswer)
	api.POST("/observe/:id/cancel", auth.RoleViewer, h.cancelObserve)
	api.POST("/observe/:id/continue", auth.RoleViewer, h.continueObserve)
	api.POST("/observe/:id/retry", auth.RoleViewer, h.retryObserve)
	api.GET("/chats", auth.RoleViewer, h.listChats)
	api.GET("/chats/:id", auth.RoleViewer, h.getChat)
	api.DELETE("/chats/:id", auth.RoleViewer, h.archiveChat)
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
	var req StartRequest
	if !httpapi.DecodeBody(c, &req, maxRequestBytes) {
		return
	}
	started, err := h.service.Start(c.Request.Context(), httpapi.Caller(c), req)
	var invalid *StartError
	if errors.As(err, &invalid) {
		httpapi.AbortText(c, http.StatusBadRequest, "invalid_request", invalid)
		return
	}
	if err != nil {
		h.service.fail(c, "enqueue agent request", err, principalAttrs(c)...)
		return
	}
	view, err := h.runs.renderStarted(c.Request.Context(), started, httpapi.Caller(c))
	if err != nil {
		h.service.fail(c, "render agent request", err, slog.String("agent_request_id", started.ID.String()))
		return
	}
	status := http.StatusAccepted
	if started.Replayed {
		status = http.StatusOK
	}
	c.Header("Location", "/api/v1/observe/"+started.ID.String())
	c.JSON(status, agentRequestEnvelopeJSON{Request: view})
}

func investigateWindow(from, to *string, now time.Time, asOf bool) (*time.Time, *time.Time, error) {
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
	case asOf && end.After(now):
		return nil, nil, errWindowAfterAsOf
	}
	start, end = start.UTC(), end.UTC()
	return &start, &end, nil
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
	body, err := h.runs.listBody(c.Request.Context(), httpapi.Caller(c), filter, limit, c.Query("cursor"))
	if err != nil {
		h.service.fail(c, "list agent requests", err, principalAttrs(c)...)
		return
	}
	c.JSON(http.StatusOK, body)
}

func scopeAndStates(c *gin.Context, filter *ListFilter) bool {
	if raw, present := c.GetQuery("scope"); present {
		switch ListScope(raw) {
		case ScopeMine, ScopeAll:
			filter.Scope = ListScope(raw)
		default:
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "scope must be mine or all")
			return false
		}
	}
	for _, raw := range c.QueryArray("state") {
		state := State(raw)
		if !slices.Contains(listableStates, state) {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "state must be queued, running, canceling, succeeded, failed or canceled")
			return false
		}
		filter.States = append(filter.States, state)
	}
	return true
}

func listFilter(c *gin.Context) (ListFilter, bool) {
	filter := ListFilter{Scope: ScopeMine}
	if !scopeAndStates(c, &filter) {
		return filter, false
	}
	if raw, present := c.GetQuery("q"); present {
		q, err := CheckQuery(raw)
		if err != nil {
			httpapi.AbortText(c, http.StatusBadRequest, "invalid_request", err)
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
	if raw, present := c.GetQuery("observation_id"); present {
		id, err := db.ParseUUID(raw)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "observation_id must be a UUID")
			return filter, false
		}
		filter.ObservationID = id
	}
	if raw, present := c.GetQuery("anchored"); present {
		switch raw {
		case "true", "false":
			filter.Anchored = raw == "true"
		default:
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "anchored must be true or false")
			return filter, false
		}
	}
	if raw, present := c.GetQuery("chat_id"); present {
		id, err := db.ParseUUID(raw)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "chat_id must be a UUID")
			return filter, false
		}
		filter.ChatID = id
	}
	return filter, true
}

func requestTitle(title pgtype.Text, prompt string) string {
	if title.Valid {
		return title.String
	}
	return DefaultTitle(prompt)
}

func optionalUUID(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	s := id.String()
	return &s
}

func optionalInt(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
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
	row, readOnly, err := h.service.ReadObserve(c.Request.Context(), id, httpapi.Caller(c))
	if err != nil {
		h.service.fail(c, "get agent request", err, slog.String("agent_request_id", id.String()))
		return
	}
	h.replyRequest(c, http.StatusOK, row, readOnly)
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
		if json.Unmarshal(req.Title, &raw) != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "title must be a string or null")
			return
		}
		cleaned, err := CheckTitle(raw)
		if err != nil {
			httpapi.AbortText(c, http.StatusBadRequest, "invalid_request", err)
			return
		}
		title = &cleaned
	}
	ctx, caller := c.Request.Context(), httpapi.Caller(c)
	out, err := h.runs.update(ctx, id, caller, title, clearTitle, req.Pinned)
	if err != nil {
		h.failBuild(c, "update agent request", err, slog.String("agent_request_id", id.String()))
		return
	}
	c.JSON(http.StatusOK, agentRequestEnvelopeJSON{Request: out})
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
