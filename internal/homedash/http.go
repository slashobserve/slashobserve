package homedash

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const bodyLimit = 1 << 10

const sessionRequiredMessage = "a home dashboard belongs to a signed-in person; service-account keys have none"

type dashboardJSON struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	FolderID    string `json:"folder_id"`
	FolderTitle string `json:"folder_title"`
}

type homeJSON struct {
	Status    Status         `json:"status"`
	Dashboard *dashboardJSON `json:"dashboard"`
	SetAt     *string        `json:"set_at"`
}

type setBody struct {
	DashboardID string `json:"dashboard_id"`
}

type Handler struct {
	home   *Service
	logger *slog.Logger
}

func NewHandler(home *Service, logger *slog.Logger) *Handler {
	return &Handler{home: home, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/me/home-dashboard", auth.RoleViewer, requireSession, h.get)
	api.PUT("/me/home-dashboard", auth.RoleViewer, requireSession, h.set)
	api.DELETE("/me/home-dashboard", auth.RoleViewer, requireSession, h.clear)
}

func requireSession(c *gin.Context) {
	if httpapi.Caller(c).Kind != auth.KindUser {
		httpapi.AbortError(c, http.StatusForbidden, "user_session_required", sessionRequiredMessage)
		return
	}
	c.Next()
}

func bodyOf(h Home) homeJSON {
	body := homeJSON{Status: h.Status}
	if h.Status == StatusNone {
		return body
	}
	setAt := httpapi.FormatTime(h.SetAt)
	body.SetAt = &setAt
	if h.Status == StatusOK {
		body.Dashboard = &dashboardJSON{ID: h.DashboardID, Title: h.Title, FolderID: h.FolderID, FolderTitle: h.FolderTitle}
	}
	return body
}

func (h *Handler) get(c *gin.Context) {
	home, err := h.home.Get(c.Request.Context(), httpapi.Caller(c))
	if err != nil {
		h.fail(c, "get home dashboard", err)
		return
	}
	c.JSON(http.StatusOK, bodyOf(home))
}

func (h *Handler) set(c *gin.Context) {
	var body setBody
	if !httpapi.DecodeBody(c, &body, bodyLimit) {
		return
	}
	id, err := db.ParseUUID(body.DashboardID)
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "dashboard_id must be a UUID")
		return
	}
	err = h.home.Set(c.Request.Context(), httpapi.Caller(c), id)
	if errors.Is(err, ErrNotFound) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "dashboard not found")
		return
	}
	if err != nil {
		h.fail(c, "set home dashboard", err)
		return
	}
	h.get(c)
}

func (h *Handler) clear(c *gin.Context) {
	if err := h.home.Clear(c.Request.Context(), httpapi.Caller(c)); err != nil {
		h.fail(c, "clear home dashboard", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}
