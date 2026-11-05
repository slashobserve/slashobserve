package provision

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type service interface {
	Run(ctx context.Context) error
	Report(ctx context.Context) (Report, error)
}

type Handler struct {
	service service
	logger  *slog.Logger
}

func NewHandler(s service, logger *slog.Logger) *Handler {
	return &Handler{service: s, logger: logger}
}

type statusJSON struct {
	Dir        *string         `json:"dir"`
	Dashboards []dashboardJSON `json:"dashboards"`
	Errors     []fileErrorJSON `json:"errors"`
}

type dashboardJSON struct {
	Path        string  `json:"path"`
	DashboardID string  `json:"dashboard_id"`
	Folder      string  `json:"folder"`
	UpdatedAt   string  `json:"updated_at"`
	LastError   *string `json:"last_error"`
}

type fileErrorJSON struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	SeenAt  string `json:"seen_at"`
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/provision")
	g.GET("", auth.RoleAdmin, h.status)
	g.POST("/run", auth.RoleAdmin, h.run)
}

func (h *Handler) status(c *gin.Context) {
	h.respond(c, "read provisioning status")
}

func (h *Handler) run(c *gin.Context) {
	err := h.service.Run(c.Request.Context())
	switch {
	case errors.Is(err, ErrOff):
		httpapi.AbortError(c, http.StatusConflict, "provisioning_off", "provisioning is off; set SLASHOBSERVE_PROVISION_DIR to turn it on")
		return
	case errors.Is(err, ErrDirUnreadable):
		h.logger.ErrorContext(c.Request.Context(), "run provisioning", logattrs.Failure("provision_dir_unreadable", err)...)
		httpapi.AbortError(c, http.StatusInternalServerError, "provision_dir_unreadable", "the provisioning directory cannot be read")
		return
	case err != nil:
		h.fail(c, "run provisioning", err)
		return
	}
	h.respond(c, "read provisioning status")
}

func (h *Handler) respond(c *gin.Context, action string) {
	report, err := h.service.Report(c.Request.Context())
	if err != nil {
		h.fail(c, action, err)
		return
	}
	c.JSON(http.StatusOK, statusOf(report))
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}

func statusOf(r Report) statusJSON {
	out := statusJSON{Dir: httpapi.OptionalString(r.Dir), Dashboards: make([]dashboardJSON, 0, len(r.Dashboards)), Errors: make([]fileErrorJSON, 0, len(r.Errors))}
	for _, d := range r.Dashboards {
		out.Dashboards = append(out.Dashboards, dashboardJSON{Path: d.Path, DashboardID: d.DashboardID, Folder: d.Folder, UpdatedAt: httpapi.FormatTime(d.UpdatedAt), LastError: httpapi.OptionalString(d.LastError)})
	}
	for _, e := range r.Errors {
		out.Errors = append(out.Errors, fileErrorJSON{Path: e.Path, Message: e.Message, SeenAt: httpapi.FormatTime(e.SeenAt)})
	}
	return out
}
