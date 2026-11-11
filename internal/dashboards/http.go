package dashboards

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxBodyBytes     = spec.MaxBytes + 64<<10
	maxMoveBodyBytes = 1 << 10
	defaultLimit     = 50
	maxLimit         = 200
	maxOffset        = 100_000

	mergeAuto = "auto"

	maxDatasourceRunes = 64
	maxQueryRunes      = 200
	maxTags            = 10
)

var tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,39}$`)

type createRequest struct {
	Spec     json.RawMessage `json:"spec"`
	Summary  string          `json:"summary"`
	FolderID *string         `json:"folder_id"`
}

type moveRequest struct {
	FolderID *string `json:"folder_id"`
}

type versionRequest struct {
	Spec          json.RawMessage `json:"spec"`
	Summary       *string         `json:"summary"`
	ParentVersion *int            `json:"parent_version"`

	Merge           *string `json:"merge"`
	ProposeIfNeeded bool    `json:"propose_if_needed"`
}

type restoreRequest struct {
	ParentVersion *int   `json:"parent_version"`
	Summary       string `json:"summary"`
}

func (s *Service) Register(api auth.Routes) {
	g := api.Group("/dashboards")
	g.POST("", auth.RoleViewer, s.createDashboard)
	g.GET("", auth.RoleViewer, s.listDashboards)
	g.GET("/tags", auth.RoleViewer, s.listTags)
	view := s.access.RequireDashboard(folders.NeedView)
	edit := s.access.RequireDashboard(folders.NeedEdit)
	g.GET("/:id", auth.RoleViewer, view, s.getDashboard)
	g.PATCH("/:id", auth.RoleViewer, edit, s.moveDashboard)
	g.DELETE("/:id", auth.RoleViewer, edit, s.deleteDashboard)
	g.GET("/:id/versions", auth.RoleViewer, view, s.listVersions)
	g.GET("/:id/versions/:version", auth.RoleViewer, view, s.getVersion)
	g.POST("/:id/versions", auth.RoleViewer, view, s.addVersion)
	g.POST("/:id/versions/:version/restore", auth.RoleViewer, edit, s.restoreVersion)
	g.POST("/:id/duplicate", auth.RoleViewer, view, s.duplicateDashboard)
	g.GET("/:id/diff", auth.RoleViewer, view, s.diff)
}

func (s *Service) createDashboard(c *gin.Context) {
	var req createRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) || !requireSpec(c, req.Spec) {
		return
	}
	folderID, ok := optionalFolderID(c, req.FolderID)
	if !ok {
		return
	}
	dash, ver, err := s.Create(c.Request.Context(), httpapi.Caller(c), folderID, req.Spec, req.Summary)
	if err != nil {
		s.fail(c, "create dashboard", err)
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+dash.ID)
	c.JSON(http.StatusCreated, NewDashboardVersionJSON(dash, ver))
}

func listPageParams(c *gin.Context) (limit, offset int, cursor string, ok bool) {
	limit, ok = httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return 0, 0, "", false
	}
	offset, ok = httpapi.OffsetParam(c, maxOffset)
	if !ok {
		return 0, 0, "", false
	}
	_, hasCursor := c.GetQuery("cursor")
	_, hasOffset := c.GetQuery("offset")
	if hasCursor && hasOffset {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "use cursor or offset, not both")
		return 0, 0, "", false
	}
	return limit, offset, c.Query("cursor"), true
}

func (s *Service) listDashboards(c *gin.Context) {
	limit, offset, cursor, ok := listPageParams(c)
	if !ok {
		return
	}
	params, ok := s.listFilters(c)
	if !ok {
		return
	}
	params.Limit, params.Offset, params.Cursor = limit, offset, cursor
	page, err := s.List(c.Request.Context(), httpapi.Caller(c), params)
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

type tagCountJSON struct {
	Tag        string `json:"tag"`
	Dashboards int    `json:"dashboards"`
}

func newTagsJSON(tags []TagCount) []tagCountJSON {
	out := make([]tagCountJSON, 0, len(tags))
	for _, t := range tags {
		out = append(out, tagCountJSON(t))
	}
	return out
}

func (s *Service) listTags(c *gin.Context) {
	tags, err := s.Tags(c.Request.Context(), httpapi.Caller(c))
	if err != nil {
		s.fail(c, "list dashboard tags", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tags": newTagsJSON(tags)})
}

func (s *Service) listFilters(c *gin.Context) (ListParams, bool) {
	var params ListParams
	madeBy, ok := httpapi.NonEmptyQuery(c, "made_by")
	if !ok {
		return params, false
	}
	datasource, ok := httpapi.NonEmptyQuery(c, "datasource")
	if !ok {
		return params, false
	}
	query, ok := httpapi.NonEmptyQuery(c, "q")
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
	tags := c.QueryArray("tag")
	if len(tags) > maxTags || slices.ContainsFunc(tags, func(tag string) bool { return !tagPattern.MatchString(tag) }) {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "tag must match the dashboard tag pattern, at most 10")
		return params, false
	}
	params.Tags = tags
	if query != "" {
		trimmed := strings.TrimSpace(query)
		if !cleanText(query) || utf8.RuneCountInString(query) > maxQueryRunes || trimmed == "" {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "q must be 1 to 200 characters")
			return params, false
		}
		params.Query = trimmed
	}
	ok = folderFilters(c, &params)
	return params, ok
}

func folderFilters(c *gin.Context, params *ListParams) bool {
	if raw, present := c.GetQuery("folder"); present {
		folder, err := db.ParseUUID(raw)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "folder must be a UUID")
			return false
		}
		params.Folder = folder
	}
	switch c.DefaultQuery("order", "updated") {
	case "updated":
	case "folder":
		params.ByFolder = true
	default:
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "order must be updated or folder")
		return false
	}
	return true
}

func optionalFolderID(c *gin.Context, raw *string) (pgtype.UUID, bool) {
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
	dash.Abilities = folders.DashboardFrom(c).Abilities
	c.JSON(http.StatusOK, NewDashboardVersionJSON(dash, ver))
}

func (s *Service) moveDashboard(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req moveRequest
	if !httpapi.DecodeBody(c, &req, maxMoveBodyBytes) {
		return
	}
	if req.FolderID == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "folder_id is required")
		return
	}
	folderID, ok := optionalFolderID(c, req.FolderID)
	if !ok {
		return
	}
	dash, ver, err := s.move(c.Request.Context(), httpapi.Caller(c), id, folderID)
	if err != nil {
		s.fail(c, "move dashboard", err)
		return
	}
	c.JSON(http.StatusOK, NewDashboardVersionJSON(dash, ver))
}

func (s *Service) deleteDashboard(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := s.Delete(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		s.fail(c, "delete dashboard", err)
		return
	}
	c.Status(http.StatusNoContent)
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
	dash.Abilities = folders.DashboardFrom(c).Abilities
	c.JSON(http.StatusOK, NewDashboardVersionJSON(dash, ver))
}

func (s *Service) addVersion(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if abilities := folders.DashboardFrom(c).Abilities; !abilities.Edit && !abilities.Comment {
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
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
	if req.Merge != nil && *req.Merge != mergeAuto {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", `merge must be "auto"`)
		return
	}
	res, err := s.Save(c.Request.Context(), httpapi.Caller(c), SaveInput{
		DashboardID:     id,
		Parent:          *req.ParentVersion,
		Spec:            req.Spec,
		Summary:         *req.Summary,
		Merge:           req.Merge != nil,
		ProposeIfNeeded: req.ProposeIfNeeded,
	})
	if err != nil {
		s.fail(c, "add dashboard version", err)
		return
	}
	if res.Proposal != nil {
		c.Header("Location", "/api/v1/proposals/"+res.Proposal.ID)
		c.JSON(http.StatusAccepted, proposalCreatedJSON{ProposalID: res.Proposal.ID, Revision: res.Proposal.Revision, Opened: res.Proposal.Opened})
		return
	}
	body := versionSavedJSON{DashboardVersionJSON: NewDashboardVersionJSON(res.Dashboard, res.Version), MergedFrom: res.MergedFrom}
	if res.Replayed {
		c.Header("Idempotent-Replay", "true")
		c.JSON(http.StatusOK, body)
		return
	}
	c.Header("Location", versionLocation(res.Dashboard.ID, res.Version.Number))
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
	c.JSON(http.StatusCreated, NewDashboardVersionJSON(dash, ver))
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

func (s *Service) Fail(c *gin.Context, action string, err error) {
	s.fail(c, action, err)
}

func (s *Service) fail(c *gin.Context, action string, err error) {
	ctx := c.Request.Context()
	var conflict *ConflictError
	var invalid *InvalidSpecError
	var invalidInput *InvalidInputError
	if clientgone.Handle(c, s.logger, action, err) {
		return
	}
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrFolderNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, folders.ErrNoAccess):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.AccessMessage)
	case errors.Is(err, folders.ErrMoveIntoPersonal):
		httpapi.AbortError(c, http.StatusConflict, "personal_folder", err.Error())
	case errors.Is(err, folders.ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
	case errors.As(err, &conflict):
		c.AbortWithStatusJSON(http.StatusConflict, conflictJSON{Error: "dashboard has a newer version", Code: "version_conflict", LatestVersion: conflict.Latest, Conflicts: conflict.Conflicts})
	case errors.As(err, &invalid):
		httpapi.AbortInvalidSpec(c, "spec is invalid", invalid.Issues)
	case errors.As(err, &invalidInput):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", invalidInput.Message)
	case errors.Is(err, ErrInvalidCursor), errors.Is(err, ErrInvalidRange):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrBeingCreated):
		httpapi.AbortError(c, http.StatusConflict, "dashboard_being_created", "the agent is still creating this dashboard; try again when it finishes")
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
