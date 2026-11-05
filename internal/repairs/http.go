package repairs

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	maxBodyBytes       = 64 << 10
	maxDashboardIDs    = 100
	defaultListLimit   = 50
	maxListLimit       = 100
	invalidRequestBody = "send dashboard_ids with 1 to 100 dashboard UUIDs, or all set to true with an optional folder_id"
)

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc, logger: svc.logger}
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/repairs")
	g.GET("", auth.RoleEditor, h.list)
	g.POST("", auth.RoleEditor, h.request)
}

type folderJSON struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	General  bool   `json:"general"`
	Personal bool   `json:"personal"`
}

type repairJSON struct {
	ID         string  `json:"id"`
	Status     string  `json:"status"`
	FinishedAt *string `json:"finished_at"`
}

type itemJSON struct {
	ID                  string      `json:"id"`
	Title               string      `json:"title"`
	URL                 string      `json:"url"`
	Folder              folderJSON  `json:"folder"`
	UnverifiedPanels    int         `json:"unverified_panels"`
	UnverifiedVariables int         `json:"unverified_variables"`
	PlaceholderPanels   int         `json:"placeholder_panels"`
	LastImportAt        *string     `json:"last_import_at"`
	Repair              *repairJSON `json:"repair"`
}

type listJSON struct {
	Dashboards []itemJSON `json:"dashboards"`
	NextCursor *string    `json:"next_cursor"`
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
		Folder:           folderJSON{ID: it.Folder.ID.String(), Title: it.Folder.Title, General: it.Folder.General, Personal: it.Folder.Personal},
		UnverifiedPanels: it.UnverifiedPanels, UnverifiedVariables: it.UnverifiedVariables, PlaceholderPanels: it.PlaceholderPanels,
		LastImportAt: formatTime(it.LastImportAt),
	}
	if it.Repair != nil {
		out.Repair = &repairJSON{ID: it.Repair.ID, Status: it.Repair.Status, FinishedAt: formatTime(it.Repair.FinishedAt)}
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
	page, err := h.svc.List(c.Request.Context(), httpapi.Caller(c), params)
	if err != nil {
		h.fail(c, "list repairs", err)
		return
	}
	out := listJSON{Dashboards: make([]itemJSON, 0, len(page.Items)), NextCursor: httpapi.OptionalString(page.NextCursor)}
	for _, it := range page.Items {
		out.Dashboards = append(out.Dashboards, newItemJSON(it))
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) request(c *gin.Context) {
	target, ok := decodeTarget(c)
	if !ok {
		return
	}
	outcome, err := h.svc.Request(c.Request.Context(), httpapi.Caller(c), target)
	if err != nil {
		h.fail(c, "request repairs", err)
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

func decodeTarget(c *gin.Context) (Target, bool) {
	var body struct {
		DashboardIDs []string `json:"dashboard_ids"`
		All          *bool    `json:"all"`
		FolderID     *string  `json:"folder_id"`
	}
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return Target{}, false
	}
	target, ok := targetOf(body.DashboardIDs, body.All, body.FolderID)
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", invalidRequestBody)
	}
	return target, ok
}

func targetOf(ids []string, all *bool, folderID *string) (Target, bool) {
	switch {
	case all != nil && ids == nil:
		if !*all {
			return Target{}, false
		}
		target := Target{All: true}
		if folderID != nil {
			folder, err := db.ParseUUID(*folderID)
			if err != nil {
				return Target{}, false
			}
			target.FolderID = folder
		}
		return target, true
	case all == nil && folderID == nil && len(ids) >= 1 && len(ids) <= maxDashboardIDs:
		target := Target{DashboardIDs: make([]pgtype.UUID, 0, len(ids))}
		for _, raw := range ids {
			id, err := db.ParseUUID(raw)
			if err != nil {
				return Target{}, false
			}
			target.DashboardIDs = append(target.DashboardIDs, id)
		}
		return target, true
	}
	return Target{}, false
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
