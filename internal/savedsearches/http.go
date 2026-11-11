package savedsearches

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	defaultLimit     = 20
	maxLimit         = 100
	maxBodyBytes     = 64 << 10
	moveIntoPersonal = "a saved search cannot move into My Dashboards; save it there instead"
)

type folderJSON struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	General  bool   `json:"general"`
	Personal bool   `json:"personal"`
}

type SavedSearchJSON struct {
	ID            string              `json:"id"`
	Kind          Kind                `json:"kind"`
	Title         string              `json:"title"`
	DatasourceUID string              `json:"datasource_uid"`
	Query         *string             `json:"query"`
	Filters       Filters             `json:"filters"`
	From          string              `json:"from"`
	To            string              `json:"to"`
	Folder        folderJSON          `json:"folder"`
	Permissions   folders.Permissions `json:"permissions"`
	Actor         actor.JSON          `json:"actor"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
	URL           string              `json:"url"`
}

type listJSON struct {
	SavedSearches []SavedSearchJSON `json:"saved_searches"`
	NextCursor    *string           `json:"next_cursor"`
}

func JSONOf(s SavedSearch) SavedSearchJSON {
	return SavedSearchJSON{
		ID:            s.ID,
		Kind:          s.Kind,
		Title:         s.Title,
		DatasourceUID: s.DatasourceUID,
		Query:         s.Query,
		Filters:       s.Filters,
		From:          s.From,
		To:            s.To,
		Folder:        folderJSON{ID: s.Folder.ID.String(), Title: s.Folder.Title, General: s.Folder.General, Personal: s.Folder.Personal},
		Permissions:   folders.PermissionsJSON(s.Abilities),
		Actor:         s.Actor.JSON(),
		CreatedAt:     httpapi.FormatTime(s.CreatedAt),
		UpdatedAt:     httpapi.FormatTime(s.UpdatedAt),
		URL:           s.URL,
	}
}

type createRequest struct {
	Kind          string          `json:"kind"`
	Title         string          `json:"title"`
	DatasourceUID string          `json:"datasource_uid"`
	Query         *string         `json:"query"`
	Filters       json.RawMessage `json:"filters"`
	From          string          `json:"from"`
	To            string          `json:"to"`
	FolderID      *string         `json:"folder_id"`
}

type updateRequest struct {
	Title    *string `json:"title"`
	FolderID *string `json:"folder_id"`
}

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/saved-searches", auth.RoleViewer, h.list)
	api.POST("/saved-searches", auth.RoleViewer, h.create)
	api.PATCH("/saved-searches/:id", auth.RoleViewer, h.update)
	api.DELETE("/saved-searches/:id", auth.RoleViewer, h.remove)
}

func optionalFolder(c *gin.Context, raw *string) (pgtype.UUID, bool) {
	if raw == nil {
		return pgtype.UUID{}, true
	}
	id, err := db.ParseUUID(*raw)
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "folder_id must be a UUID")
		return pgtype.UUID{}, false
	}
	return id, true
}

func (h *Handler) list(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	var filter ListFilter
	if kind, present := c.GetQuery("kind"); present {
		if filter.Kind = Kind(kind); filter.Kind != KindLogs && filter.Kind != KindTraces {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "kind must be logs or traces")
			return
		}
	}
	if raw, present := c.GetQuery("folder_id"); present {
		var ok bool
		if filter.FolderID, ok = optionalFolder(c, &raw); !ok {
			return
		}
	}
	rows, next, err := h.service.List(c.Request.Context(), httpapi.Caller(c), filter, limit, c.Query("cursor"))
	if err != nil {
		h.fail(c, "list saved searches", err)
		return
	}
	body := listJSON{SavedSearches: make([]SavedSearchJSON, len(rows))}
	for i, s := range rows {
		body.SavedSearches[i] = JSONOf(s)
	}
	if next != "" {
		body.NextCursor = &next
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) create(c *gin.Context) {
	var req createRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	folderID, ok := optionalFolder(c, req.FolderID)
	if !ok {
		return
	}
	saved, err := h.service.Create(c.Request.Context(), httpapi.Caller(c), CreateInput{
		Kind:          Kind(req.Kind),
		Title:         req.Title,
		DatasourceUID: req.DatasourceUID,
		Query:         req.Query,
		Filters:       req.Filters,
		From:          req.From,
		To:            req.To,
		FolderID:      folderID,
	})
	if err != nil {
		h.fail(c, "create saved search", err)
		return
	}
	c.JSON(http.StatusCreated, JSONOf(saved))
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req updateRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	folderID, ok := optionalFolder(c, req.FolderID)
	if !ok {
		return
	}
	saved, err := h.service.Update(c.Request.Context(), httpapi.Caller(c), id, UpdateInput{Title: req.Title, FolderID: folderID})
	if err != nil {
		h.fail(c, "update saved search", err)
		return
	}
	c.JSON(http.StatusOK, JSONOf(saved))
}

func (h *Handler) remove(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		h.fail(c, "delete saved search", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	var invalid *InvalidInputError
	switch {
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", ErrNotFound.Error())
	case errors.Is(err, ErrFolderNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", ErrFolderNotFound.Error())
	case errors.Is(err, ErrInvalidCursor):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "cursor is not valid")
	case errors.Is(err, ErrNothingToUpdate):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", ErrNothingToUpdate.Error())
	case errors.Is(err, folders.ErrMoveIntoPersonal):
		httpapi.AbortError(c, http.StatusConflict, "personal_folder", moveIntoPersonal)
	case errors.Is(err, folders.ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
	case errors.Is(err, folders.ErrNoAccess):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.AccessMessage)
	case errors.As(err, &invalid):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, invalid.Code, invalid.Message)
	default:
		principal := httpapi.Caller(c)
		attrs := []any{slog.String("principal_kind", string(principal.Kind)), slog.String("principal_id", principal.ID)}
		if id, parseErr := db.ParseUUID(c.Param("id")); parseErr == nil {
			attrs = append(attrs, slog.String("saved_search_id", id.String()))
		}
		h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, attrs...)...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
