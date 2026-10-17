package dashboards

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxBodyBytes  = spec.MaxBytes + 64<<10
	defaultLimit  = 50
	maxLimit      = 200
	uuidTextBytes = 36
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
	if !decodeBody(c, &req) || !requireSpec(c, req.Spec) {
		return
	}
	dash, ver, err := s.Create(c.Request.Context(), caller(c), req.Spec, req.Summary)
	if err != nil {
		s.fail(c, "create dashboard", err)
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+dash.ID)
	c.JSON(http.StatusCreated, newDashboardVersionJSON(dash, ver))
}

func (s *Service) listDashboards(c *gin.Context) {
	limit, ok := limitParam(c)
	if !ok {
		return
	}
	items, next, err := s.List(c.Request.Context(), limit, c.Query("cursor"))
	if err != nil {
		s.fail(c, "list dashboards", err)
		return
	}
	body := dashboardListJSON{Dashboards: make([]dashboardJSON, 0, len(items)), NextCursor: optionalString(next)}
	for _, d := range items {
		body.Dashboards = append(body.Dashboards, newDashboardJSON(d))
	}
	c.JSON(http.StatusOK, body)
}

func (s *Service) getDashboard(c *gin.Context) {
	id, ok := uuidParam(c, "id")
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
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	limit, ok := limitParam(c)
	if !ok {
		return
	}
	items, next, err := s.ListVersions(c.Request.Context(), id, limit, c.Query("cursor"))
	if err != nil {
		s.fail(c, "list dashboard versions", err)
		return
	}
	body := versionListJSON{Versions: make([]versionMetaJSON, 0, len(items)), NextCursor: optionalString(next)}
	for _, v := range items {
		body.Versions = append(body.Versions, newVersionMetaJSON(v))
	}
	c.JSON(http.StatusOK, body)
}

func (s *Service) getVersion(c *gin.Context) {
	id, ok := uuidParam(c, "id")
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
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req versionRequest
	if !decodeBody(c, &req) || !requireSpec(c, req.Spec) {
		return
	}
	if req.Summary == nil || req.ParentVersion == nil {
		abortError(c, http.StatusBadRequest, "invalid_request", "spec, summary and parent_version are required")
		return
	}
	if !validVersion(*req.ParentVersion) {
		abortError(c, http.StatusBadRequest, "invalid_request", "parent_version must be a positive 32-bit integer")
		return
	}
	dash, ver, replayed, err := s.AddVersion(c.Request.Context(), caller(c), id, *req.ParentVersion, req.Spec, *req.Summary)
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
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	from, ok := versionParam(c, "version")
	if !ok {
		return
	}
	var req restoreRequest
	if !decodeBody(c, &req) {
		return
	}
	if req.ParentVersion == nil || !validVersion(*req.ParentVersion) {
		abortError(c, http.StatusBadRequest, "invalid_request", "parent_version is required and must be a positive 32-bit integer")
		return
	}
	dash, ver, err := s.Restore(c.Request.Context(), caller(c), id, from, *req.ParentVersion, req.Summary)
	if err != nil {
		s.fail(c, "restore dashboard version", err)
		return
	}
	c.Header("Location", versionLocation(dash.ID, ver.Number))
	c.JSON(http.StatusCreated, newDashboardVersionJSON(dash, ver))
}

func (s *Service) diff(c *gin.Context) {
	id, ok := uuidParam(c, "id")
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
	switch {
	case errors.Is(err, context.Canceled):
		s.logger.DebugContext(ctx, action+": client went away")
		c.Abort()
	case errors.Is(err, ErrNotFound):
		abortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.As(err, &conflict):
		c.AbortWithStatusJSON(http.StatusConflict, conflictJSON{Error: "dashboard has a newer version", Code: "version_conflict", LatestVersion: conflict.Latest})
	case errors.As(err, &invalid):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, invalidSpecJSON{Error: "spec is invalid", Code: "invalid_spec", Issues: invalid.Issues})
	case errors.Is(err, ErrInvalidCursor), errors.Is(err, ErrInvalidRange):
		abortError(c, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		s.logger.ErrorContext(ctx, action, "error", err)
		abortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}

func abortError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message, "code": code})
}

func decodeBody(c *gin.Context, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil {
		_, err = dec.Token()
		if errors.Is(err, io.EOF) {
			return true
		}
		if err == nil {
			abortError(c, http.StatusBadRequest, "invalid_request", "request body must be a single JSON object")
			return false
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		abortError(c, http.StatusRequestEntityTooLarge, "too_large", "request body is too large")
		return false
	}
	abortError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON with known fields")
	return false
}

func requireSpec(c *gin.Context, raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		abortError(c, http.StatusBadRequest, "invalid_request", "spec is required")
		return false
	}
	return true
}

func caller(c *gin.Context) auth.Principal {
	p, _ := auth.FromContext(c.Request.Context())
	return p
}

func uuidParam(c *gin.Context, name string) (pgtype.UUID, bool) {
	raw := c.Param(name)
	id, err := parseUUID(raw)
	if err != nil || len(raw) != uuidTextBytes {
		abortError(c, http.StatusBadRequest, "invalid_request", name+" must be a UUID")
		return pgtype.UUID{}, false
	}
	return id, true
}

func versionParam(c *gin.Context, name string) (int, bool) {
	n, ok := parseVersion(c.Param(name))
	if !ok {
		abortError(c, http.StatusBadRequest, "invalid_request", name+" must be a positive 32-bit integer")
		return 0, false
	}
	return n, true
}

func limitParam(c *gin.Context) (int, bool) {
	raw, present := c.GetQuery("limit")
	if !present {
		return defaultLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxLimit || strconv.Itoa(n) != raw {
		abortError(c, http.StatusBadRequest, "invalid_request", "limit must be an integer from 1 to 200")
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
		abortError(c, http.StatusBadRequest, "invalid_request", name+" must be an integer of at least "+strconv.Itoa(lowest))
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

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
