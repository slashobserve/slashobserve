package activity

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

type Handler struct {
	store  *Store
	logger *slog.Logger
}

type listJSON struct {
	Activity   []EventJSON `json:"activity"`
	NextCursor *string     `json:"next_cursor"`
}

func NewHandler(store *Store, logger *slog.Logger) *Handler {
	return &Handler{store: store, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/activity", auth.RoleViewer, h.list)
}

func (h *Handler) list(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	filter, err := ParseFilter(c.Request.URL.Query())
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	ctx := c.Request.Context()
	audience, err := h.store.Audience(ctx, httpapi.Caller(c))
	if err != nil {
		h.fail(c, "read activity audience", err)
		return
	}
	page, next, err := h.store.List(ctx, audience, filter, c.Query("cursor"), limit)
	if err != nil {
		h.fail(c, "list activity", err)
		return
	}
	body := listJSON{Activity: make([]EventJSON, 0, len(page))}
	for _, e := range page {
		body.Activity = append(body.Activity, JSONOf(e))
	}
	if next != "" {
		body.NextCursor = &next
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	switch {
	case errors.Is(err, folders.ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.AccessMessage)
	case errors.Is(err, ErrInvalidCursor):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "cursor is not valid")
	case clientgone.Handle(c, h.logger, action, err):
	default:
		h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
