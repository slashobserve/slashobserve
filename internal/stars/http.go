package stars

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const sessionRequiredMessage = "stars belong to signed-in people; service-account keys cannot star dashboards"

type starJSON struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	FolderID    string `json:"folder_id"`
	FolderTitle string `json:"folder_title"`
	StarredAt   string `json:"starred_at"`
}

type listJSON struct {
	Items []starJSON `json:"items"`
}

type Handler struct {
	stars       *Service
	requireView gin.HandlerFunc
	logger      *slog.Logger
}

func NewHandler(stars *Service, requireView gin.HandlerFunc, logger *slog.Logger) *Handler {
	return &Handler{stars: stars, requireView: requireView, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.PUT("/dashboards/:id/star", auth.RoleViewer, requireSession, h.requireView, h.star)
	api.DELETE("/dashboards/:id/star", auth.RoleViewer, requireSession, h.requireView, h.unstar)
	api.GET("/stars", auth.RoleViewer, requireSession, h.list)
}

func requireSession(c *gin.Context) {
	if httpapi.Caller(c).Kind != auth.KindUser {
		httpapi.AbortError(c, http.StatusForbidden, "user_session_required", sessionRequiredMessage)
		return
	}
	c.Next()
}

func (h *Handler) star(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	err := h.stars.Star(c.Request.Context(), httpapi.Caller(c), id)
	if errors.Is(err, ErrNotFound) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "dashboard not found")
		return
	}
	if err != nil {
		h.fail(c, "star dashboard", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) unstar(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.stars.Unstar(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		h.fail(c, "unstar dashboard", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) list(c *gin.Context) {
	stars, err := h.stars.List(c.Request.Context(), httpapi.Caller(c))
	if err != nil {
		h.fail(c, "list stars", err)
		return
	}
	body := listJSON{Items: make([]starJSON, 0, len(stars))}
	for _, s := range stars {
		body.Items = append(body.Items, starJSON{
			ID:          s.DashboardID,
			Title:       s.Title,
			FolderID:    s.FolderID,
			FolderTitle: s.FolderTitle,
			StarredAt:   httpapi.FormatTime(s.StarredAt),
		})
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}
