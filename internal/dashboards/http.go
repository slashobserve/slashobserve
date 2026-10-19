package dashboards

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxBodyBytes = spec.MaxBytes + 64<<10
	defaultLimit = 50
	maxLimit     = 200

	maxDatasourceRunes = 64
	maxQueryRunes      = 200

	statusClientClosedRequest = 499
)

type createRequest struct {
	Spec    json.RawMessage `json:"spec"`
	Summary string          `json:"summary"`
}

type versionRequest struct {
	Spec          json.RawMessage `json:"spec"`
	Summary       *string         `json:"summary"`
	ParentVersion *int            `json:"parent_version"`
}

type restoreRequest struct {
	ParentVersion *int   `json:"parent_version"`
	Summary       string `json:"summary"`
}

func (s *Service) Register(api *gin.RouterGroup) {
	g := api.Group("/dashboards")
	g.POST("", s.createDashboard)
	g.GET("", s.listDashboards)
	g.GET("/:id", s.getDashboard)
	g.GET("/:id/versions", s.listVersions)
	g.GET("/:id/versions/:version", s.getVersion)
	g.POST("/:id/versions", s.addVersion)
	g.POST("/:id/versions/:version/restore", s.restoreVersion)
	g.GET("/:id/diff", s.diff)
}

func (s *Service) createDashboard(c *gin.Context) {
	var req createRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) || !requireSpec(c, req.Spec) {
		return
	}
	dash, ver, err := s.Create(c.Request.Context(), httpapi.Caller(c), req.Spec, req.Summary)
	if err != nil {
		s.fail(c, "create dashboard", err)
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+dash.ID)
	c.JSON(http.StatusCreated, newDashboardVersionJSON(dash, ver))
}

func (s *Service) listDashboards(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	params, ok := s.listFilters(c)
	if !ok {
		return
	}
	params.Limit, params.Cursor = limit, c.Query("cursor")
	page, err := s.List(c.Request.Context(), params)
	if err != nil {
		s.fail(c, "list dashboards", err)
		return
	}
	body := dashboardListJSON{Dashboards: make([]dashboardListItemJSON, 0, len(page.Items)), NextCursor: httpapi.OptionalString(page.NextCursor), TotalCount: page.Total}
	for _, item := range page.Items {
		body.Dashboards = append(body.Dashboards, newDashboardListItemJSON(item))
	}
	c.JSON(http.StatusOK, body)
}

func (s *Service) listFilters(c *gin.Context) (ListParams, bool) {
	var params ListParams
	madeBy, ok := nonEmptyQuery(c, "made_by")
	if !ok {
		return params, false
	}
	datasource, ok := nonEmptyQuery(c, "datasource")
	if !ok {
		return params, false
	}
	query, ok := nonEmptyQuery(c, "q")
	if !ok {
		return params, false
	}
	if madeBy != "" {
		parsed, err := ParseMadeBy(madeBy, httpapi.Caller(c))
		if err != nil {
			s.fail(c, "list dashboards", err)
			return params, false
		}
		params.MadeBy = parsed
	}
	if !cleanText(datasource) || utf8.RuneCountInString(datasource) > maxDatasourceRunes {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "datasource must be 1 to 64 characters")
		return params, false
	}
	params.Datasource = datasource
	if query != "" {
		trimmed := strings.TrimSpace(query)
		if !cleanText(query) || utf8.RuneCountInString(query) > maxQueryRunes || trimmed == "" {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "q must be 1 to 200 characters")
			return params, false
		}
		params.Query = trimmed
	}
	return params, true
}

func cleanText(v string) bool {
	return utf8.ValidString(v) && !strings.ContainsRune(v, 0)
}

func (s *Service) getDashboard(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	dash, ver, err := s.Get(c.Request.Context(), id)
	if err != nil {
		s.fail(c, "get dashboard", err)
		return
	}
	c.JSON(http.StatusOK, newDashboardVersionJSON(dash, ver))
}

func (s *Service) listVersions(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	items, next, err := s.ListVersions(c.Request.Context(), id, limit, c.Query("cursor"))
	if err != nil {
		s.fail(c, "list dashboard versions", err)
		return
	}
	body := versionListJSON{Versions: make([]versionMetaJSON, 0, len(items)), NextCursor: httpapi.OptionalString(next)}
	for _, v := range items {
		body.Versions = append(body.Versions, newVersionMetaJSON(v))
	}
	c.JSON(http.StatusOK, body)
}

func (s *Service) getVersion(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	number, ok := versionParam(c, "version")
	if !ok {
		return
	}
	dash, ver, err := s.GetVersion(c.Request.Context(), id, number)
	if err != nil {
		s.fail(c, "get dashboard version", err)
		return
	}
	c.JSON(http.StatusOK, newDashboardVersionJSON(dash, ver))
}

func (s *Service) addVersion(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req versionRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) || !requireSpec(c, req.Spec) {
		return
	}
	if req.Summary == nil || req.ParentVersion == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "spec, summary and parent_version are required")
		return
	}
	if !validVersion(*req.ParentVersion) {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "parent_version must be a positive 32-bit integer")
		return
	}
	dash, ver, replayed, err := s.AddVersion(c.Request.Context(), httpapi.Caller(c), id, *req.ParentVersion, req.Spec, *req.Summary)
	if err != nil {
		s.fail(c, "add dashboard version", err)
		return
	}
	body := newDashboardVersionJSON(dash, ver)
	if replayed {
		c.Header("Idempotent-Replay", "true")
		c.JSON(http.StatusOK, body)
		return
	}
	c.Header("Location", versionLocation(dash.ID, ver.Number))
	c.JSON(http.StatusCreated, body)
}

func (s *Service) restoreVersion(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	from, ok := versionParam(c, "version")
	if !ok {
		return
	}
	var req restoreRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	if req.ParentVersion == nil || !validVersion(*req.ParentVersion) {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "parent_version is required and must be a positive 32-bit integer")
		return
	}
	dash, ver, err := s.Restore(c.Request.Context(), httpapi.Caller(c), id, from, *req.ParentVersion, req.Summary)
	if err != nil {
		s.fail(c, "restore dashboard version", err)
		return
	}
	c.Header("Location", versionLocation(dash.ID, ver.Number))
	c.JSON(http.StatusCreated, newDashboardVersionJSON(dash, ver))
}

func (s *Service) diff(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	from, ok := optionalIntQuery(c, "from", 0)
	if !ok {
		return
	}
	to, ok := optionalIntQuery(c, "to", 1)
	if !ok {
		return
	}
	changes, fromN, toN, err := s.Diff(c.Request.Context(), id, from, to)
	if err != nil {
		s.fail(c, "diff dashboard versions", err)
		return
	}
	c.JSON(http.StatusOK, diffJSON{From: fromN, To: toN, Changes: changes})
}

func (s *Service) fail(c *gin.Context, action string, err error) {
	ctx := c.Request.Context()
	var conflict *ConflictError
	var invalid *InvalidSpecError
	var invalidInput *InvalidInputError
	switch {
	case errors.Is(err, context.Canceled):
		s.logger.DebugContext(ctx, action+": client went away")
		c.AbortWithStatus(statusClientClosedRequest)
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.As(err, &conflict):
		c.AbortWithStatusJSON(http.StatusConflict, conflictJSON{Error: "dashboard has a newer version", Code: "version_conflict", LatestVersion: conflict.Latest})
	case errors.As(err, &invalid):
		httpapi.AbortInvalidSpec(c, "spec is invalid", invalid.Issues)
	case errors.As(err, &invalidInput):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", invalidInput.Message)
	case errors.Is(err, ErrInvalidCursor), errors.Is(err, ErrInvalidRange):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		principal := httpapi.Caller(c)
		attrs := []any{slog.String("principal_kind", string(principal.Kind)), slog.String("principal_id", principal.ID)}
		if id, parseErr := db.ParseUUID(c.Param("id")); parseErr == nil {
			attrs = append(attrs, slog.String("dashboard_id", id.String()))
		}
		s.logger.ErrorContext(ctx, action, logattrs.Failure("internal", err, attrs...)...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}

func requireSpec(c *gin.Context, raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "spec is required")
		return false
	}
	return true
}

func versionParam(c *gin.Context, name string) (int, bool) {
	n, ok := parseVersion(c.Param(name))
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", name+" must be a positive 32-bit integer")
		return 0, false
	}
	return n, true
}

func optionalIntQuery(c *gin.Context, name string, lowest int) (*int, bool) {
	raw, present := c.GetQuery(name)
	if !present {
		return nil, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lowest || n > maxInt32 || strconv.Itoa(n) != raw {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", name+" must be an integer of at least "+strconv.Itoa(lowest))
		return nil, false
	}
	return &n, true
}

func parseVersion(raw string) (int, bool) {
	n, err := strconv.Atoi(raw)
	if err != nil || !validVersion(n) || strconv.Itoa(n) != raw {
		return 0, false
	}
	return n, true
}

func validVersion(n int) bool {
	return n >= 1 && n <= maxInt32
}

func versionLocation(id string, number int) string {
	return "/api/v1/dashboards/" + id + "/versions/" + strconv.Itoa(number)
}
