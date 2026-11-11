package revisions

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

var (
	ErrBuildNotFound   = errors.New("build not found")
	ErrBuildEmpty      = errors.New("the build has no panels")
	ErrBuildNotSavable = errors.New("the request is no longer a failed request with an unsaved build")
	ErrBuildRunning    = errors.New("the agent may still change the build")
)

type BuildConflictError struct {
	Latest int
}

func (e *BuildConflictError) Error() string {
	return fmt.Sprintf("dashboard has a newer version %d", e.Latest)
}

type BuildView struct {
	RequestID     string
	DashboardID   string
	BaseVersion   int
	Saves         int
	Panels        int
	PlannedPanels int
	Spec          json.RawMessage
	UpdatedAt     string
}

type SavedBuild struct {
	DashboardID string
	Version     int
	Replayed    bool
}

type Builds interface {
	ViewBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (BuildView, error)
	SaveStoppedBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (SavedBuild, error)
}

type buildProgressJSON struct {
	Saves         int     `json:"saves"`
	Panels        int     `json:"panels"`
	PlannedPanels int     `json:"planned_panels"`
	Title         *string `json:"title"`
	UpdatedAt     string  `json:"updated_at"`
}

type buildJSON struct {
	RequestID     string          `json:"request_id"`
	DashboardID   *string         `json:"dashboard_id"`
	BaseVersion   *int            `json:"base_version"`
	Saves         int             `json:"saves"`
	Panels        int             `json:"panels"`
	PlannedPanels int             `json:"planned_panels"`
	Spec          json.RawMessage `json:"spec"`
	UpdatedAt     string          `json:"updated_at"`
}

type buildEnvelopeJSON struct {
	Build buildJSON `json:"build"`
}

type savedBuildJSON struct {
	DashboardID string `json:"dashboard_id"`
	Version     int    `json:"version"`
	URL         string `json:"url"`
	Replayed    bool   `json:"replayed"`
}

type versionConflictJSON struct {
	Error         string `json:"error"`
	Code          string `json:"code"`
	LatestVersion int    `json:"latest_version"`
}

type buildReply struct {
	status  int
	code    string
	message string
	latest  int
}

func newBuildJSON(v BuildView) buildJSON {
	out := buildJSON{
		RequestID:     v.RequestID,
		Saves:         v.Saves,
		Panels:        v.Panels,
		PlannedPanels: v.PlannedPanels,
		Spec:          v.Spec,
		UpdatedAt:     v.UpdatedAt,
	}
	if v.DashboardID != "" {
		out.DashboardID = &v.DashboardID
		out.BaseVersion = &v.BaseVersion
	}
	return out
}

func buildReplyFor(err error) (buildReply, bool) {
	var conflict *BuildConflictError
	switch {
	case errors.As(err, &conflict):
		return buildReply{status: http.StatusConflict, code: "version_conflict", message: "dashboard has a newer version", latest: conflict.Latest}, true
	case errors.Is(err, ErrBuildNotFound), errors.Is(err, ErrRequestNotFound), errors.Is(err, ErrDashboardNotFound), errors.Is(err, folders.ErrNotFound):
		return buildReply{status: http.StatusNotFound, code: "not_found", message: err.Error()}, true
	case errors.Is(err, ErrBuildRunning):
		return buildReply{status: http.StatusConflict, code: "build_running", message: ErrBuildRunning.Error()}, true
	case errors.Is(err, ErrBuildEmpty):
		return buildReply{status: http.StatusConflict, code: "build_empty", message: ErrBuildEmpty.Error()}, true
	case errors.Is(err, ErrBuildNotSavable):
		return buildReply{status: http.StatusConflict, code: "build_not_savable", message: ErrBuildNotSavable.Error()}, true
	case errors.Is(err, ErrNotContinuable):
		return buildReply{status: http.StatusConflict, code: "not_continuable", message: ErrNotContinuable.Error()}, true
	case errors.Is(err, ErrRequestFinished):
		return buildReply{status: http.StatusConflict, code: "request_finished", message: ErrRequestFinished.Error()}, true
	case errors.Is(err, ErrNotRequester):
		return buildReply{status: http.StatusForbidden, code: "forbidden", message: ErrNotRequester.Error()}, true
	case errors.Is(err, folders.ErrNoAccess):
		return buildReply{status: http.StatusForbidden, code: forbidden.Code, message: forbidden.AccessMessage}, true
	case errors.Is(err, folders.ErrForbidden), errors.Is(err, ErrForbidden):
		return buildReply{status: http.StatusForbidden, code: forbidden.Code, message: forbidden.RoleMessage}, true
	}
	return buildReply{}, false
}

func (h *ObserveHandler) failBuild(c *gin.Context, action string, err error, idAttrs ...any) {
	if reply, ok := buildReplyFor(err); ok {
		if reply.latest > 0 {
			c.AbortWithStatusJSON(reply.status, versionConflictJSON{Error: reply.message, Code: reply.code, LatestVersion: reply.latest})
			return
		}
		httpapi.AbortError(c, reply.status, reply.code, reply.message)
		return
	}
	h.service.fail(c, action, err, idAttrs...)
}

func isRequester(row dbsqlc.AgentRequest, by auth.Principal) bool {
	return sameActor(requesterOf(row), by)
}

func sameActor(a, b auth.Principal) bool {
	return a.Kind == b.Kind && a.ID == b.ID
}

func (h *ObserveHandler) getBuild(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	ctx, caller := c.Request.Context(), httpapi.Caller(c)
	idAttr := slog.String("agent_request_id", id.String())
	row, err := h.service.GetObserve(ctx, id, caller)
	if err == nil {
		err = h.service.checkVisible(ctx, row.DashboardID, caller)
	}
	if err != nil {
		h.failBuild(c, "get agent request build", err, idAttr)
		return
	}
	view, err := h.service.builds.ViewBuild(ctx, caller, row.ID)
	if err != nil {
		h.failBuild(c, "get agent request build", err, idAttr)
		return
	}
	c.JSON(http.StatusOK, buildEnvelopeJSON{Build: newBuildJSON(view)})
}

func (h *ObserveHandler) saveBuild(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	ctx, caller := c.Request.Context(), httpapi.Caller(c)
	idAttr := slog.String("agent_request_id", id.String())
	row, err := h.service.GetObserve(ctx, id, caller)
	if err == nil {
		err = h.service.checkVisible(ctx, row.DashboardID, caller)
	}
	switch {
	case err == nil && !isRequester(row, caller):
		err = ErrRequestNotFound
	case err == nil && !State(row.State).Terminal():
		err = ErrBuildRunning
	}
	if err != nil {
		h.failBuild(c, "save stopped agent request build", err, idAttr)
		return
	}
	saved, err := h.service.builds.SaveStoppedBuild(ctx, caller, row.ID)
	if err != nil {
		h.failBuild(c, "save stopped agent request build", err, idAttr)
		return
	}
	status := http.StatusCreated
	if saved.Replayed {
		status = http.StatusOK
	}
	body := savedBuildJSON{DashboardID: saved.DashboardID, Version: saved.Version, Replayed: saved.Replayed}
	if h.opts.DashboardURL != nil {
		body.URL = h.opts.DashboardURL(saved.DashboardID, "")
	}
	c.JSON(status, body)
}

type continueRequest struct {
	Note *string `json:"note"`
}

func decodeOptionalBody(c *gin.Context, dst any, limit int64) bool {
	body := bufio.NewReader(c.Request.Body)
	if _, err := body.Peek(1); errors.Is(err, io.EOF) {
		return true
	}
	c.Request.Body = io.NopCloser(body)
	return httpapi.DecodeBody(c, dst, limit)
}

func (h *ObserveHandler) replyRequest(c *gin.Context, status int, row dbsqlc.AgentRequest) {
	view, err := h.runs.render(c.Request.Context(), row, httpapi.Caller(c))
	if err != nil {
		h.service.fail(c, "render agent request", err, slog.String("agent_request_id", row.ID.String()))
		return
	}
	c.JSON(status, agentRequestEnvelopeJSON{Request: view})
}

func (h *ObserveHandler) cancelObserve(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	row, err := h.service.Cancel(c.Request.Context(), id, httpapi.Caller(c))
	if err != nil {
		h.failBuild(c, "cancel agent request", err, slog.String("agent_request_id", id.String()))
		return
	}
	h.replyRequest(c, http.StatusOK, row)
}

func (h *ObserveHandler) retryObserve(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	row, err := h.service.Retry(c.Request.Context(), id, httpapi.Caller(c))
	var refusal *lifecycle.Refusal
	switch {
	case errors.As(err, &refusal):
		lifecycle.AbortRefusal(c, refusal)
		return
	case err != nil:
		h.failBuild(c, "retry agent request", err, slog.String("agent_request_id", id.String()))
		return
	}
	c.Header("Location", "/api/v1/observe/"+row.ID.String())
	h.replyRequest(c, http.StatusAccepted, row)
}

func (h *ObserveHandler) continueObserve(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req continueRequest
	if !decodeOptionalBody(c, &req, maxRequestBytes) {
		return
	}
	note, ok := CleanContinueNote(req.Note)
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", fmt.Sprintf("note must be 1 to %d characters without control or bidirectional override characters", MaxPromptRunes))
		return
	}
	row, err := h.service.Continue(c.Request.Context(), id, httpapi.Caller(c), note)
	if err != nil {
		h.failBuild(c, "continue agent request", err, slog.String("agent_request_id", id.String()))
		return
	}
	c.Header("Location", "/api/v1/observe/"+row.ID.String())
	h.replyRequest(c, http.StatusCreated, row)
}
