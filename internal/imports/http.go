package imports

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/pkg/grafana"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxMappingBytes = 200

type Handler struct {
	svc    *Service
	access *folders.Access
}

func NewHandler(svc *Service, access *folders.Access) *Handler {
	return &Handler{svc: svc, access: access}
}

func (h *Handler) Register(api auth.Routes) {
	api.POST("/imports/preview", auth.RoleEditor, h.preview)
	api.POST("/imports", auth.RoleEditor, h.create)
	api.GET("/imports", auth.RoleViewer, h.list)
	api.GET("/dashboards/:id/import", auth.RoleViewer, h.access.RequireDashboard(folders.NeedView), h.forDashboard)
}

type importRequest struct {
	JSON    *string           `json:"json"`
	Mapping map[string]string `json:"mapping"`
	Folder  *string           `json:"folder"`
}

type authorJSON struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type metaAuthorJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type previewJSON struct {
	Summary     grafana.Summary            `json:"summary"`
	Datasources []grafana.SourceDatasource `json:"datasources"`
	Ready       bool                       `json:"ready"`
	Report      grafana.Report             `json:"report"`
	Spec        *spec.Dashboard            `json:"spec"`
}

type importJSON struct {
	ID              string                     `json:"id"`
	DashboardID     string                     `json:"dashboard_id"`
	Version         int                        `json:"version"`
	Source          string                     `json:"source"`
	ImporterVersion int                        `json:"importer_version"`
	Title           string                     `json:"title"`
	Mapping         map[string]string          `json:"mapping"`
	Summary         grafana.Summary            `json:"summary"`
	Datasources     []grafana.SourceDatasource `json:"datasources"`
	Report          grafana.Report             `json:"report"`
	CreatedBy       authorJSON                 `json:"created_by"`
	CreatedAt       string                     `json:"created_at"`
}

type importedJSON struct {
	dashboards.DashboardVersionJSON
	Import     importJSON `json:"import"`
	FolderNote *string    `json:"folder_note"`
}

type metaJSON struct {
	ID              string         `json:"id"`
	DashboardID     string         `json:"dashboard_id"`
	Version         int            `json:"version"`
	ImporterVersion int            `json:"importer_version"`
	Title           string         `json:"title"`
	Counts          grafana.Counts `json:"counts"`
	CreatedBy       metaAuthorJSON `json:"created_by"`
	CreatedAt       string         `json:"created_at"`
}

type importListJSON struct {
	Imports    []metaJSON `json:"imports"`
	NextCursor *string    `json:"next_cursor"`
}

type unresolvedJSON struct {
	Error       string                     `json:"error"`
	Code        string                     `json:"code"`
	Datasources []grafana.SourceDatasource `json:"datasources"`
}

func newImportJSON(r Record) importJSON {
	return importJSON{
		ID:              r.ID,
		DashboardID:     r.DashboardID,
		Version:         r.Version,
		Source:          sourceGrafana,
		ImporterVersion: r.ImporterVersion,
		Title:           r.Title,
		Mapping:         r.Mapping,
		Summary:         r.Summary,
		Datasources:     r.Datasources,
		Report:          r.Report,
		CreatedBy:       authorJSON(r.CreatedBy),
		CreatedAt:       httpapi.FormatTime(r.CreatedAt),
	}
}

func newMetaJSON(m Meta) metaJSON {
	return metaJSON{
		ID:              m.ID,
		DashboardID:     m.DashboardID,
		Version:         m.Version,
		ImporterVersion: m.ImporterVersion,
		Title:           m.Title,
		Counts:          m.Counts,
		CreatedBy:       metaAuthorJSON{Kind: m.CreatedBy.Kind, ID: m.CreatedBy.ID, Name: m.CreatedBy.Name},
		CreatedAt:       httpapi.FormatTime(m.CreatedAt),
	}
}

var errAnswered = errors.New("the request was already answered")

func decodeImport(c *gin.Context) LoadFunc {
	return func() (Request, error) {
		req, ok := decodeImportBody(c)
		if !ok {
			return Request{}, errAnswered
		}
		return req, nil
	}
}

func decodeImportBody(c *gin.Context) (Request, bool) {
	var body importRequest
	if !httpapi.DecodeBody(c, &body, MaxBodyBytes) {
		return Request{}, false
	}
	if body.JSON == nil || *body.JSON == "" {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "json is required")
		return Request{}, false
	}
	if len(body.Mapping) > MaxMappingEntries {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "mapping can have at most 50 entries")
		return Request{}, false
	}
	for key, value := range body.Mapping {
		if key == "" || len(key) > maxMappingBytes || value == "" || len(value) > maxMappingBytes {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "mapping keys and values must be 1 to 200 bytes")
			return Request{}, false
		}
	}
	req := Request{JSON: []byte(*body.JSON), Mapping: body.Mapping}
	if body.Folder != nil {
		title, err := folders.CleanTitle(*body.Folder)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "folder: "+err.Error())
			return Request{}, false
		}
		req.Folder = title
	}
	return req, true
}

func (h *Handler) preview(c *gin.Context) {
	result, err := h.svc.PreviewFrom(c.Request.Context(), httpapi.Caller(c), decodeImport(c))
	if errors.Is(err, errAnswered) {
		return
	}
	if err != nil {
		h.fail(c, "preview import", err)
		return
	}
	c.JSON(http.StatusOK, previewJSON{
		Summary:     result.Summary,
		Datasources: result.Datasources,
		Ready:       result.Ready(),
		Report:      result.Report,
		Spec:        result.Spec,
	})
}

func (h *Handler) create(c *gin.Context) {
	imported, err := h.svc.ImportFrom(c.Request.Context(), httpapi.Caller(c), decodeImport(c))
	if errors.Is(err, errAnswered) {
		return
	}
	if err != nil {
		h.fail(c, "import dashboard", err)
		return
	}
	respondImported(c, imported)
}

func respondImported(c *gin.Context, imported Imported) {
	saved := dashboards.NewDashboardVersionJSON(imported.Dashboard, imported.Version)
	if imported.Record.ID == "" {
		c.JSON(http.StatusOK, saved)
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+imported.Dashboard.ID)
	c.JSON(http.StatusCreated, importedJSON{DashboardVersionJSON: saved, Import: newImportJSON(imported.Record), FolderNote: httpapi.OptionalString(imported.FolderNote)})
}

func (h *Handler) list(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	params, err := listParamsOf(c.Request.URL.Query(), httpapi.Caller(c))
	if err != nil {
		h.fail(c, "list imports", err)
		return
	}
	params.Limit = limit
	params.Cursor = c.Query("cursor")
	metas, next, err := h.svc.List(c.Request.Context(), httpapi.Caller(c), params)
	if err != nil {
		h.fail(c, "list imports", err)
		return
	}
	body := importListJSON{Imports: make([]metaJSON, 0, len(metas)), NextCursor: httpapi.OptionalString(next)}
	for _, m := range metas {
		body.Imports = append(body.Imports, newMetaJSON(m))
	}
	c.JSON(http.StatusOK, body)
}

func listParamsOf(query url.Values, caller auth.Principal) (ListParams, error) {
	var params ListParams
	if values, present := query["made_by"]; present {
		if len(values) != 1 || values[0] != "me" {
			return ListParams{}, fmt.Errorf("%w: made_by must be me", ErrInvalidFilter)
		}
		params.MadeBy = &caller
	}
	if values, present := query["dashboard_id"]; present {
		id, err := db.ParseUUID(values[0])
		if len(values) != 1 || err != nil {
			return ListParams{}, fmt.Errorf("%w: dashboard_id must be a UUID", ErrInvalidFilter)
		}
		params.DashboardID = id
	}
	return params, nil
}

func (h *Handler) forDashboard(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	record, err := h.svc.ForDashboard(c.Request.Context(), id)
	if err != nil {
		h.fail(c, "get dashboard import", err)
		return
	}
	c.JSON(http.StatusOK, newImportJSON(record))
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	var input *grafana.InputError
	var unresolved *UnresolvedError
	var invalid *dashboards.InvalidSpecError
	if clientgone.Handle(c, h.svc.logger, action, err) {
		return
	}
	switch {
	case errors.As(err, &input):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, input.Code, input.Message)
	case errors.As(err, &unresolved):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, unresolvedJSON{Error: unresolved.Error(), Code: "datasources_unresolved", Datasources: unresolved.Datasources})
	case errors.As(err, &invalid):
		httpapi.AbortInvalidSpec(c, "spec is invalid", invalid.Issues)
	case errors.Is(err, ErrBusy):
		httpapi.AbortError(c, http.StatusTooManyRequests, "too_many_imports", err.Error())
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_imported", "the dashboard was not imported")
	case errors.Is(err, dashboards.ErrNotFound), errors.Is(err, dashboards.ErrFolderNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, folders.ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
	case errors.Is(err, ErrInvalidCursor), errors.Is(err, ErrInvalidFilter):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		principal := httpapi.Caller(c)
		attrs := []any{slog.String("principal_kind", string(principal.Kind)), slog.String("principal_id", principal.ID)}
		if id, parseErr := db.ParseUUID(c.Param("id")); parseErr == nil {
			attrs = append(attrs, slog.String("dashboard_id", id.String()))
		}
		h.svc.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, attrs...)...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
