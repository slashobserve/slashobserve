package grafanaconn

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxBodyBytes      = 64 << 10
	maxChosenFolders  = 1000
	maxFolderUIDBytes = 64
	maxMappingEntries = 200
	maxMappingBytes   = 200
	allFolders        = "all"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/grafana")
	g.GET("", auth.RoleAdmin, h.list)
	g.POST("", auth.RoleAdmin, h.create)
	g.GET("/resolve/:uid", auth.RoleViewer, h.resolve)
	g.GET("/:id", auth.RoleAdmin, h.get)
	g.PATCH("/:id", auth.RoleAdmin, h.update)
	g.DELETE("/:id", auth.RoleAdmin, h.remove)
	g.POST("/:id/test", auth.RoleAdmin, h.test)
	g.GET("/:id/folders", auth.RoleAdmin, h.folders)
	g.GET("/:id/datasources", auth.RoleAdmin, h.datasources)
	g.POST("/:id/imports", auth.RoleAdmin, h.startImport)
	g.GET("/:id/imports/:sync_id", auth.RoleAdmin, h.importStatus)
}

type testJSON struct {
	At      string `json:"at"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type summaryJSON struct {
	ID         string  `json:"id"`
	Status     string  `json:"status"`
	Total      int     `json:"total"`
	Done       int     `json:"done"`
	Failed     int     `json:"failed"`
	Skipped    int     `json:"skipped"`
	Error      *string `json:"error"`
	StartedAt  string  `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
}

type connectionJSON struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	URL       string       `json:"url"`
	CreatedBy *string      `json:"created_by"`
	CreatedAt string       `json:"created_at"`
	UpdatedAt string       `json:"updated_at"`
	LastTest  *testJSON    `json:"last_test"`
	LastSync  *summaryJSON `json:"last_sync"`
}

type resultJSON struct {
	UID               string  `json:"uid"`
	Title             string  `json:"title"`
	Outcome           string  `json:"outcome"`
	DashboardID       *string `json:"dashboard_id"`
	UnverifiedPanels  int     `json:"unverified_panels"`
	PlaceholderPanels int     `json:"placeholder_panels"`
	Error             *string `json:"error"`
}

type syncJSON struct {
	summaryJSON
	ConnectionID string       `json:"connection_id"`
	Results      []resultJSON `json:"results"`
}

type folderJSON struct {
	UID        string `json:"uid"`
	Title      string `json:"title"`
	Dashboards int    `json:"dashboards"`
}

type datasourceJSON struct {
	UID        string   `json:"uid"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Kind       *string  `json:"kind"`
	Candidates []string `json:"candidates"`
	MatchedUID *string  `json:"matched_uid"`
}

type resolvedJSON struct {
	DashboardID string `json:"dashboard_id"`
	URL         string `json:"url"`
}

func optionalTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := httpapi.FormatTime(*t)
	return &s
}

func newSummaryJSON(s SyncSummary) summaryJSON {
	return summaryJSON{
		ID: s.ID, Status: s.Status, Total: s.Total, Done: s.Done, Failed: s.Failed, Skipped: s.Skipped,
		Error: httpapi.OptionalString(s.Error), StartedAt: httpapi.FormatTime(s.StartedAt), FinishedAt: optionalTime(s.FinishedAt),
	}
}

func newConnectionJSON(c Connection) connectionJSON {
	out := connectionJSON{
		ID: c.ID, Name: c.Name, URL: c.URL, CreatedBy: httpapi.OptionalString(c.CreatedBy),
		CreatedAt: httpapi.FormatTime(c.CreatedAt), UpdatedAt: httpapi.FormatTime(c.UpdatedAt),
	}
	if c.LastTest != nil {
		out.LastTest = &testJSON{At: httpapi.FormatTime(c.LastTest.At), OK: c.LastTest.OK, Message: c.LastTest.Message}
	}
	if c.LastSync != nil {
		summary := newSummaryJSON(*c.LastSync)
		out.LastSync = &summary
	}
	return out
}

func newSyncJSON(s Sync) syncJSON {
	out := syncJSON{summaryJSON: newSummaryJSON(s.SyncSummary), ConnectionID: s.ConnectionID, Results: make([]resultJSON, 0, len(s.Results))}
	for _, r := range s.Results {
		out.Results = append(out.Results, resultJSON{
			UID: r.UID, Title: r.Title, Outcome: r.Outcome, DashboardID: httpapi.OptionalString(r.DashboardID),
			UnverifiedPanels: r.UnverifiedPanels, PlaceholderPanels: r.PlaceholderPanels, Error: httpapi.OptionalString(r.Error),
		})
	}
	return out
}

func (h *Handler) list(c *gin.Context) {
	list, err := h.svc.List(c.Request.Context())
	if err != nil {
		h.fail(c, "list grafana connections", err)
		return
	}
	out := make([]connectionJSON, 0, len(list))
	for _, conn := range list {
		out = append(out, newConnectionJSON(conn))
	}
	c.JSON(http.StatusOK, gin.H{"connections": out})
}

func (h *Handler) create(c *gin.Context) {
	var body struct {
		Name  *string `json:"name"`
		URL   *string `json:"url"`
		Token *string `json:"token"`
	}
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	if body.Name == nil || body.URL == nil || body.Token == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "name, url and token are required")
		return
	}
	created, err := h.svc.Create(c.Request.Context(), httpapi.Caller(c), Input{Name: *body.Name, URL: *body.URL, Token: *body.Token})
	if err != nil {
		h.fail(c, "create grafana connection", err)
		return
	}
	c.Header("Location", "/api/v1/grafana/"+created.ID)
	c.JSON(http.StatusCreated, newConnectionJSON(created))
}

func (h *Handler) get(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	conn, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		h.fail(c, "get grafana connection", err)
		return
	}
	c.JSON(http.StatusOK, newConnectionJSON(conn))
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var body struct {
		Name  *string `json:"name"`
		URL   *string `json:"url"`
		Token *string `json:"token"`
	}
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	updated, err := h.svc.Update(c.Request.Context(), httpapi.Caller(c), id, Patch{Name: body.Name, URL: body.URL, Token: body.Token})
	if err != nil {
		h.fail(c, "update grafana connection", err)
		return
	}
	c.JSON(http.StatusOK, newConnectionJSON(updated))
}

func (h *Handler) remove(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		h.fail(c, "delete grafana connection", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) test(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	tested, err := h.svc.Test(c.Request.Context(), id)
	if err != nil {
		h.fail(c, "test grafana connection", err)
		return
	}
	c.JSON(http.StatusOK, newConnectionJSON(tested))
}

func (h *Handler) folders(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	list, err := h.svc.Folders(c.Request.Context(), id)
	if err != nil {
		h.fail(c, "list grafana folders", err)
		return
	}
	out := make([]folderJSON, 0, len(list))
	for _, f := range list {
		out = append(out, folderJSON(f))
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) datasources(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	list, err := h.svc.Datasources(c.Request.Context(), id)
	if err != nil {
		h.fail(c, "list grafana datasources", err)
		return
	}
	out := make([]datasourceJSON, 0, len(list))
	for _, d := range list {
		out = append(out, datasourceJSON{UID: d.UID, Name: d.Name, Type: d.Type, Kind: httpapi.OptionalString(d.Kind), Candidates: d.Candidates, MatchedUID: httpapi.OptionalString(d.MatchedUID)})
	}
	c.JSON(http.StatusOK, out)
}

func decodeSyncRequest(c *gin.Context) (SyncRequest, bool) {
	var body struct {
		Folders json.RawMessage   `json:"folders"`
		Mapping map[string]string `json:"mapping"`
	}
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return SyncRequest{}, false
	}
	req := SyncRequest{Mapping: body.Mapping}
	problem := readFolders(body.Folders, &req)
	if problem == "" {
		problem = mappingProblem(body.Mapping)
	}
	if problem != "" {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", problem)
		return SyncRequest{}, false
	}
	return req, true
}

func readFolders(raw json.RawMessage, req *SyncRequest) string {
	var all string
	if json.Unmarshal(raw, &all) == nil && all == allFolders {
		req.All = true
		return ""
	}
	if json.Unmarshal(raw, &req.Folders) != nil || len(req.Folders) == 0 || len(req.Folders) > maxChosenFolders {
		return `folders must be "all" or a list of 1 to 1000 folder uids`
	}
	for _, uid := range req.Folders {
		if uid == "" || len(uid) > maxFolderUIDBytes {
			return "folder uids must be 1 to 64 bytes"
		}
	}
	return ""
}

func mappingProblem(mapping map[string]string) string {
	if len(mapping) > maxMappingEntries {
		return "mapping can have at most 200 entries"
	}
	for key, value := range mapping {
		if key == "" || len(key) > maxMappingBytes || value == "" || len(value) > maxMappingBytes {
			return "mapping keys and values must be 1 to 200 bytes"
		}
	}
	return ""
}

func (h *Handler) startImport(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	req, ok := decodeSyncRequest(c)
	if !ok {
		return
	}
	started, err := h.svc.StartSync(c.Request.Context(), httpapi.Caller(c), id, req)
	if err != nil {
		h.fail(c, "start grafana import", err)
		return
	}
	c.Header("Location", "/api/v1/grafana/"+started.ConnectionID+"/imports/"+started.ID)
	c.JSON(http.StatusAccepted, newSyncJSON(started))
}

func (h *Handler) importStatus(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	got, err := h.svc.SyncStatus(id, c.Param("sync_id"))
	if err != nil {
		h.fail(c, "get grafana import", err)
		return
	}
	c.JSON(http.StatusOK, newSyncJSON(got))
}

func (h *Handler) resolve(c *gin.Context) {
	id, err := h.svc.Resolve(c.Request.Context(), httpapi.Caller(c), c.Param("uid"))
	if err != nil {
		h.fail(c, "resolve grafana uid", err)
		return
	}
	c.JSON(http.StatusOK, resolvedJSON{DashboardID: id.String(), URL: dashboardPath(id)})
}

func dashboardPath(id pgtype.UUID) string {
	return "/dashboards/" + id.String()
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	var invalid *InvalidError
	var hostErr *egress.HostError
	if clientgone.Handle(c, h.svc.logger, action, err) {
		return
	}
	switch {
	case errors.As(err, &invalid):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", invalid.Message)
	case errors.Is(err, ErrTokenRequired):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "token is required when the url changes")
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrUnknownUID), errors.Is(err, ErrSyncNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrNoSecretKey):
		httpapi.AbortError(c, http.StatusConflict, "secret_key_missing", err.Error())
	case errors.Is(err, ErrUnreadableToken):
		httpapi.AbortError(c, http.StatusConflict, "token_unreadable", err.Error())
	case errors.Is(err, ErrSyncRunning):
		httpapi.AbortError(c, http.StatusConflict, "import_running", err.Error())
	case errors.Is(err, ErrTooManyDomains):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "too_many_domains", "at most 100 outbound domains can be allowed")
	case errors.Is(err, egress.ErrNotAllowed) && errors.As(err, &hostErr):
		httpapi.AbortError(c, http.StatusForbidden, "outbound_domain_not_allowed", hostErr.Host+" is not on the outbound allowlist.")
	default:
		if message, ok := describeFetch(err); ok {
			httpapi.AbortError(c, http.StatusBadGateway, "grafana_unavailable", message)
			return
		}
		h.svc.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
