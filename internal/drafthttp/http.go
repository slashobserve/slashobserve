package drafthttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const maxBodyBytes = 16 << 10

type Drafter interface {
	DraftQuery(ctx context.Context, in tools.DraftInput) (tools.Draft, error)
}

type Handler struct {
	drafter Drafter
	logger  *slog.Logger
}

func NewHandler(d Drafter, logger *slog.Logger) *Handler {
	return &Handler{drafter: d, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.POST("/datasources/:uid/draft-query", auth.RoleViewer, h.draft)
}

type draftRequest struct {
	Question string `json:"question"`
	From     string `json:"from"`
	To       string `json:"to"`
	Signal   string `json:"signal"`
}

type cannotTranslateJSON struct {
	Error  string `json:"error"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

var statusByCode = map[string]int{
	"invalid_request":   http.StatusBadRequest,
	"not_found":         http.StatusNotFound,
	"too_many_requests": http.StatusTooManyRequests,
	"agent_busy":        http.StatusTooManyRequests,
	"provider_error":    http.StatusBadGateway,
	"agent_unavailable": http.StatusServiceUnavailable,
	"draft_timeout":     http.StatusGatewayTimeout,
}

func (h *Handler) draft(c *gin.Context) {
	var body draftRequest
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	uid := c.Param("uid")
	draft, err := h.drafter.DraftQuery(c.Request.Context(), tools.DraftInput{DatasourceUID: uid, Question: body.Question, Signal: body.Signal, From: body.From, To: body.To})
	if err != nil {
		h.fail(c, uid, err)
		return
	}
	c.JSON(http.StatusOK, draft)
}

func (h *Handler) fail(c *gin.Context, uid string, err error) {
	if clientgone.Handle(c, h.logger, "draft query", err) {
		return
	}
	var cannot *tools.CannotTranslate
	if errors.As(err, &cannot) {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, cannotTranslateJSON{Error: cannot.Reason, Code: tools.CodeCannotTranslate, Reason: cannot.Reason})
		return
	}
	code, message, ok := tools.DraftFailure(err)
	if !ok {
		h.logger.ErrorContext(c.Request.Context(), "draft query failed", logattrs.Failure("internal", err, slog.String("datasource_uid", uid))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	status, known := statusByCode[code]
	if !known {
		status = http.StatusBadGateway
	}
	httpapi.AbortError(c, status, code, message)
}
