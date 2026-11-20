package viewers

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	ActiveWindow = 45 * time.Second
	maxViewers   = 50
)

type Viewer struct {
	ID   string
	Name string
}

type Service struct {
	db *db.DB
}

func New(database *db.DB) *Service {
	return &Service{db: database}
}

func (s *Service) Record(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	if by.Kind != auth.KindUser {
		return nil
	}
	userID, err := db.ParseUUID(by.ID)
	if err != nil {
		return fmt.Errorf("viewer id: %w", err)
	}
	q := s.db.Queries()
	if err := q.PruneDashboardViewers(ctx); err != nil {
		return fmt.Errorf("prune dashboard viewers: %w", err)
	}
	err = q.RecordDashboardViewer(ctx, dbsqlc.RecordDashboardViewerParams{DashboardID: dashboardID, UserID: userID})
	if db.IsForeignKeyViolation(err) {
		return nil
	}
	return err
}

func (s *Service) Others(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) ([]Viewer, error) {
	rows, err := s.db.Queries().ListDashboardViewers(ctx, dbsqlc.ListDashboardViewersParams{DashboardID: dashboardID, WindowSeconds: int32(ActiveWindow / time.Second)})
	if err != nil {
		return nil, fmt.Errorf("list dashboard viewers: %w", err)
	}
	out := make([]Viewer, 0, len(rows))
	for _, r := range rows {
		if by.Kind == auth.KindUser && r.ID.String() == by.ID {
			continue
		}
		out = append(out, Viewer{ID: r.ID.String(), Name: cmp.Or(r.Name, r.Email)})
		if len(out) == maxViewers {
			break
		}
	}
	return out, nil
}

type Handler struct {
	service *Service
	access  *folders.Access
	logger  *slog.Logger
}

func NewHandler(service *Service, access *folders.Access, logger *slog.Logger) *Handler {
	return &Handler{service: service, access: access, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/dashboards/:id/viewers", auth.RoleViewer, h.access.RequireDashboard(folders.NeedView), h.list)
	api.POST("/dashboards/:id/viewers", auth.RoleViewer, h.access.RequireDashboard(folders.NeedView), h.report)
}

type viewerJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type viewersJSON struct {
	Viewers []viewerJSON `json:"viewers"`
}

func (h *Handler) list(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	h.respond(c, id, "list dashboard viewers")
}

func (h *Handler) report(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.Record(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		h.fail(c, "record a dashboard viewer", err)
		return
	}
	h.respond(c, id, "list dashboard viewers")
}

func (h *Handler) respond(c *gin.Context, id pgtype.UUID, action string) {
	others, err := h.service.Others(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, action, err)
		return
	}
	body := viewersJSON{Viewers: make([]viewerJSON, len(others))}
	for i, v := range others {
		body.Viewers[i] = viewerJSON(v)
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err)...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}
