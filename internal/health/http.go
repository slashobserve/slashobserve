package health

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/scheduler"
)

const (
	maxBodyBytes       = 64 << 10
	maxRepairIDs       = 50
	defaultListLimit   = 50
	maxListLimit       = 100
	invalidRepairsBody = "send dashboard_ids with 1 to 50 dashboard UUIDs"
)

type Jobs interface {
	RunNow(ctx context.Context, name string) error
}

type Handler struct {
	svc     *Service
	repairs *Repairs
	access  *folders.Access
	jobs    Jobs
	life    context.Context
	logger  *slog.Logger
	runs    sync.WaitGroup
}

func NewHandler(life context.Context, svc *Service, repairs *Repairs, jobs Jobs) *Handler {
	return &Handler{svc: svc, repairs: repairs, access: svc.access, jobs: jobs, life: life, logger: svc.logger}
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/health")
	g.GET("", auth.RoleViewer, h.list)
	g.POST("/repairs", auth.RoleEditor, h.repair)
	g.POST("/run", auth.RoleAdmin, h.run)
	api.POST("/dashboards/:id/views", auth.RoleViewer, h.access.RequireDashboard(folders.NeedView), h.view)
}

func (h *Handler) Wait() {
	h.runs.Wait()
}

type folderJSON struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	General  bool   `json:"general"`
	Personal bool   `json:"personal"`
}

type problemJSON struct {
	Kind       Kind    `json:"kind"`
	PanelID    *string `json:"panel_id"`
	PanelTitle *string `json:"panel_title"`
	Variable   *string `json:"variable"`
	Code       *string `json:"code"`
	Message    *string `json:"message"`
}

type itemJSON struct {
	ID             string        `json:"id"`
	Title          string        `json:"title"`
	URL            string        `json:"url"`
	Folder         folderJSON    `json:"folder"`
	CanEdit        bool          `json:"can_edit"`
	CheckedVersion *int          `json:"checked_version"`
	CheckedAt      *string       `json:"checked_at"`
	LastViewedAt   *string       `json:"last_viewed_at"`
	Skipped        *string       `json:"skipped"`
	Problems       []problemJSON `json:"problems"`
}

type jobJSON struct {
	LastStartedAt  *string `json:"last_started_at"`
	LastFinishedAt *string `json:"last_finished_at"`
	LastOutcome    *string `json:"last_outcome"`
	Running        bool    `json:"running"`
}

type listJSON struct {
	Dashboards []itemJSON `json:"dashboards"`
	NextCursor *string    `json:"next_cursor"`
	Job        jobJSON    `json:"job"`
}

type queuedJSON struct {
	DashboardID string `json:"dashboard_id"`
	RequestID   string `json:"request_id"`
}

type skippedJSON struct {
	DashboardID string `json:"dashboard_id"`
	Reason      string `json:"reason"`
}

type outcomeJSON struct {
	Queued  []queuedJSON  `json:"queued"`
	Skipped []skippedJSON `json:"skipped"`
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := httpapi.FormatTime(*t)
	return &s
}

func newItemJSON(it Item) itemJSON {
	out := itemJSON{
		ID: it.ID, Title: it.Title, URL: it.URL,
		Folder:         folderJSON{ID: it.Folder.ID.String(), Title: it.Folder.Title, General: it.Folder.General, Personal: it.Folder.Personal},
		CanEdit:        it.CanEdit,
		CheckedVersion: it.CheckedVersion,
		CheckedAt:      formatTime(it.CheckedAt),
		LastViewedAt:   formatTime(it.LastViewedAt),
		Skipped:        httpapi.OptionalString(it.Skipped),
		Problems:       make([]problemJSON, 0, len(it.Problems)),
	}
	for _, p := range it.Problems {
		out.Problems = append(out.Problems, problemJSON{
			Kind:       p.Kind,
			PanelID:    httpapi.OptionalString(p.PanelID),
			PanelTitle: httpapi.OptionalString(p.PanelTitle),
			Variable:   httpapi.OptionalString(p.Variable),
			Code:       httpapi.OptionalString(p.Code),
			Message:    httpapi.OptionalString(p.Message),
		})
	}
	return out
}

func (h *Handler) list(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultListLimit, maxListLimit)
	if !ok {
		return
	}
	params := ListParams{Cursor: c.Query("cursor"), Limit: limit}
	if raw, present := c.GetQuery("folder_id"); present {
		folder, err := db.ParseUUID(raw)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "folder_id must be a UUID")
			return
		}
		params.FolderID = folder
	}
	if raw, present := c.GetQuery("kind"); present {
		kind, valid := ValidKind(raw)
		if !valid {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "kind must be no_data, query_failed, variable_failed, duplicate_title or unopened")
			return
		}
		params.Kind = kind
	}
	page, err := h.svc.List(c.Request.Context(), httpapi.Caller(c), params)
	if err != nil {
		h.fail(c, "list dashboard health", err)
		return
	}
	out := listJSON{
		Dashboards: make([]itemJSON, 0, len(page.Items)),
		NextCursor: httpapi.OptionalString(page.NextCursor),
		Job: jobJSON{
			LastStartedAt:  formatTime(page.Job.LastStartedAt),
			LastFinishedAt: formatTime(page.Job.LastFinishedAt),
			LastOutcome:    httpapi.OptionalString(page.Job.LastOutcome),
			Running:        page.Job.Running,
		},
	}
	for _, it := range page.Items {
		out.Dashboards = append(out.Dashboards, newItemJSON(it))
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) repair(c *gin.Context) {
	var body struct {
		DashboardIDs []string `json:"dashboard_ids"`
	}
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	ids, ok := repairIDs(body.DashboardIDs)
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", invalidRepairsBody)
		return
	}
	outcome, err := h.repairs.Repair(c.Request.Context(), httpapi.Caller(c), ids)
	if err != nil {
		h.fail(c, "request health repairs", err)
		return
	}
	out := outcomeJSON{Queued: make([]queuedJSON, 0, len(outcome.Queued)), Skipped: make([]skippedJSON, 0, len(outcome.Skipped))}
	for _, q := range outcome.Queued {
		out.Queued = append(out.Queued, queuedJSON(q))
	}
	for _, s := range outcome.Skipped {
		out.Skipped = append(out.Skipped, skippedJSON(s))
	}
	c.JSON(http.StatusAccepted, out)
}

func repairIDs(raw []string) ([]pgtype.UUID, bool) {
	if len(raw) < 1 || len(raw) > maxRepairIDs {
		return nil, false
	}
	ids := make([]pgtype.UUID, 0, len(raw))
	for _, r := range raw {
		id, err := db.ParseUUID(r)
		if err != nil {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

func (h *Handler) run(c *gin.Context) {
	h.runs.Add(1)
	go func() {
		defer h.runs.Done()
		err := h.jobs.RunNow(h.life, h.svc.job)
		switch {
		case err == nil, h.life.Err() != nil:
		case errors.Is(err, scheduler.ErrJobLeased):
			h.logger.InfoContext(h.life, "dashboard health run is already running")
		default:
			h.logger.WarnContext(h.life, "start a dashboard health run", logattrs.Failure("health_run_failed", err)...)
		}
	}()
	c.Status(http.StatusAccepted)
}

func (h *Handler) view(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.RecordView(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		if clientgone.Handle(c, h.logger, "record dashboard view", err) {
			return
		}
		h.logger.WarnContext(c.Request.Context(), "record dashboard view", logattrs.Failure("internal", err, slog.String("dashboard_id", id.String()))...)
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	switch {
	case errors.Is(err, ErrInvalidCursor):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrFolderNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, revisions.ErrUnavailable):
		httpapi.AbortError(c, http.StatusServiceUnavailable, "agent_unavailable", err.Error())
	default:
		h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
