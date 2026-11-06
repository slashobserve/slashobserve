package proxyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxInFlightPerPrincipal = 8
	maxVersion              = 1<<31 - 1
	lintTimeout             = 5 * time.Second
	maxLintQueryBytes       = 64 << 10
	maxLintBodyBytes        = 512 << 10
)

var (
	errInternal = errors.New("internal error")
	errTooLarge = errors.New("response too large")
)

type SpecSource interface {
	VersionSpec(ctx context.Context, id pgtype.UUID, version int) (*spec.Dashboard, error)
}

type BuildSpecs interface {
	ViewBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (dashboards.Build, error)
}

type Access interface {
	Dashboard(ctx context.Context, p auth.Principal, id pgtype.UUID) (folders.DashboardAccess, error)
	RequireDashboard(n folders.Need) gin.HandlerFunc
}

type Handler struct {
	runner   *queryproxy.Runner
	specs    SpecSource
	builds   BuildSpecs
	access   Access
	registry *datasource.Registry
	logger   *slog.Logger
	caches   *caches
	slots    *slots

	lintTimeout time.Duration
}

func NewHandler(runner *queryproxy.Runner, specs SpecSource, builds BuildSpecs, access Access, registry *datasource.Registry, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		runner:   runner,
		specs:    specs,
		builds:   builds,
		access:   access,
		registry: registry,
		logger:   logger,
		caches:   newCaches(logger),
		slots:    &slots{held: map[string]int{}},

		lintTimeout: lintTimeout,
	}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/dashboards/:id/versions/:version/panels/:panel_id/data", auth.RoleViewer, h.access.RequireDashboard(folders.NeedView), h.data)
	api.GET("/dashboards/:id/versions/:version/variables/:name/values", auth.RoleViewer, h.access.RequireDashboard(folders.NeedView), h.values)
	api.GET("/observe/:id/build/panels/:panel_id/data", auth.RoleViewer, h.buildData)
	api.GET("/observe/:id/build/variables/:name/values", auth.RoleViewer, h.buildValues)
	api.POST("/datasources/:uid/lint", auth.RoleViewer, h.lint)
	api.POST("/datasources/:uid/preview", auth.RoleViewer, h.preview)
	api.POST("/datasources/:uid/query", auth.RoleViewer, h.explore)
	api.POST("/datasources/:uid/log-volume", auth.RoleViewer, h.volume)
	api.POST("/datasources/:uid/log-facets", auth.RoleViewer, h.facets)
	api.GET("/datasources/:uid/metrics", auth.RoleViewer, h.metrics)
	api.GET("/datasources/:uid/labels", auth.RoleViewer, h.labels)
	api.GET("/datasources/:uid/labels/:name/values", auth.RoleViewer, h.labelValues)
}

type dataMeta struct {
	Kind        queryproxy.Kind `json:"kind"`
	From        string          `json:"from"`
	To          string          `json:"to"`
	StepMS      int64           `json:"step_ms"`
	Queries     []queryMeta     `json:"queries"`
	Cached      bool            `json:"cached"`
	ExecutedAt  string          `json:"executed_at"`
	SourcePanel string          `json:"source_panel,omitempty"`
	Truncated   bool            `json:"truncated"`
	Partial     bool            `json:"partial"`
	Rows        int             `json:"rows"`
	RowsTotal   int             `json:"rows_total,omitempty"`
	Oldest      string          `json:"oldest,omitempty"`
	Newest      string          `json:"newest,omitempty"`
}

type resultMeta struct {
	truncated bool
	partial   bool
	rows      int
	rowsTotal int
	oldest    time.Time
	newest    time.Time
}

func resultMetaOf(frames []datasource.Frame) resultMeta {
	merged := datasource.MergeMeta(frames)
	m := resultMeta{truncated: merged.Truncated, partial: merged.Partial, rowsTotal: merged.RowsTotal}
	for _, f := range frames {
		if !f.IsSeries() {
			m.rows += f.Rows()
		}
		i := slices.IndexFunc(f.Fields, func(field datasource.Field) bool { return field.Type == datasource.FieldTime })
		if i < 0 {
			continue
		}
		for _, v := range f.Fields[i].Values {
			at, ok := v.(time.Time)
			if !ok {
				continue
			}
			if m.oldest.IsZero() || at.Before(m.oldest) {
				m.oldest = at
			}
			if m.newest.IsZero() || at.After(m.newest) {
				m.newest = at
			}
		}
	}
	return m
}

func formatEdge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatTime(t)
}

type dataResponse struct {
	Frames    json.RawMessage `json:"frames"`
	Exemplars json.RawMessage `json:"exemplars,omitempty"`
	Meta      dataMeta        `json:"meta"`
}

type valuesResponse struct {
	Values    []string `json:"values"`
	Texts     []string `json:"texts,omitempty"`
	Truncated bool     `json:"truncated"`
	Cached    bool     `json:"cached"`
}

func (e valuesEntry) response(cached bool) valuesResponse {
	return valuesResponse{Values: e.values, Texts: e.texts, Truncated: e.truncated, Cached: cached}
}

type lintRequest struct {
	Query *string `json:"query"`
}

type lintResponse struct {
	Diagnostics []datasource.Diagnostic `json:"diagnostics"`
}

func (h *Handler) data(c *gin.Context) {
	ctx := c.Request.Context()
	query, ok := h.readQuery(c)
	if !ok {
		return
	}
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	id, version, err := address(c)
	if err != nil {
		h.fail(c, err)
		return
	}
	p, err := parseParams(query, true)
	if err != nil {
		h.fail(c, err)
		return
	}
	v, err := h.dashboard(ctx, id, version)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.serveData(c, p, v, c.Param("panel_id"), &cachedVersion{id: id, version: version})
}

type cachedVersion struct {
	id      pgtype.UUID
	version int
}

func (h *Handler) serveData(c *gin.Context, p params, v loadedVersion, panelID string, cached *cachedVersion) {
	ctx := c.Request.Context()
	if cached == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
		defer cancel()
	}
	d := v.dashboard
	found, ok := d.FindDataPanel(panelID)
	if !ok {
		h.fail(c, fmt.Errorf("%w: %q", queryproxy.ErrPanelNotFound, panelID))
		return
	}
	sourcePanel, hidden := "", false
	if source, reuses := d.SourceOf(panelID); reuses {
		hidden = found.Panel.Queries[0].Hide
		found, panelID, sourcePanel = source, source.Panel.ID, source.Panel.ID
	}
	now := time.Now()
	rng, opts, err := p.timeRange(d, now)
	if err == nil {
		rng, err = panelTimeRange(found.Panel, rng, now, opts)
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	maxDataPoints := p.maxDataPoints
	if found.Panel.MaxDataPoints > 0 {
		maxDataPoints = min(maxDataPoints, found.Panel.MaxDataPoints)
	}
	kind := queryproxy.PanelKind(found.Panel)
	al := align(rng, kind, maxDataPoints)
	user, scope := cacheScope(ctx, v)
	selections, err := h.runner.Defaults(ctx, d, al.rng, p.selections, user)
	if err != nil {
		h.fail(c, err)
		return
	}
	if hidden {
		writeData(c, noQueriesEntry(kind, al, now), false, now, sourcePanel)
		return
	}
	if cached == nil {
		req := dataRequest(d, panelID, al, selections, user)
		req.Limits = queryproxy.DraftLimits
		entry, err := h.render(ctx, req)
		if err != nil {
			h.fail(c, err)
			return
		}
		writeData(c, entry, false, entry.executedAt, sourcePanel)
		return
	}
	key := dataKey(cached.id, cached.version, panelID, kind, al, selections, scope)
	entry, at, hit := h.caches.data.Get(key)
	h.runner.RecordCacheLookup(hit)
	if hit {
		writeData(c, entry, true, at, sourcePanel)
		return
	}
	entry, err = h.caches.dataFlights.Do(ctx, string(key[:]), func(ctx context.Context) (dataEntry, error) {
		return h.execute(ctx, key, d, panelID, al, selections, user)
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	writeData(c, entry, false, entry.executedAt, sourcePanel)
}

func dataRequest(d *spec.Dashboard, panelID string, al alignment, selections map[string]queryproxy.Selection, user datasource.QueryUser) queryproxy.Request {
	return queryproxy.Request{
		Dashboard:     d,
		PanelID:       panelID,
		Range:         al.rng,
		Selections:    selections,
		MaxDataPoints: al.maxDataPoints,
		Exemplars:     true,
		User:          user,
	}
}

func noQueriesEntry(kind queryproxy.Kind, al alignment, now time.Time) dataEntry {
	entry := dataEntry{frames: json.RawMessage("[]"), queries: []queryMeta{}, kind: kind, from: al.rng.From, to: al.rng.To, executedAt: now}
	if kind == queryproxy.KindRange {
		entry.step = queryproxy.StepFor(al.rng, al.maxDataPoints)
	}
	return entry
}

func (h *Handler) execute(ctx context.Context, key cacheKey, d *spec.Dashboard, panelID string, al alignment, selections map[string]queryproxy.Selection, user datasource.QueryUser) (result dataEntry, err error) {
	defer func() {
		if p := recover(); p != nil {
			h.logPanic(ctx, panelID, p)
			result, err = dataEntry{}, errInternal
		}
	}()
	if cached, stored, ok := h.caches.data.Get(key); ok {
		cached.executedAt = stored
		return cached, nil
	}
	entry, err := h.render(ctx, dataRequest(d, panelID, al, selections, user))
	if err != nil {
		return dataEntry{}, err
	}
	if entry.cost() <= resultMaxEntry {
		entry.executedAt = h.caches.data.Put(key, entry, entry.cost())
	}
	return entry, nil
}

func (h *Handler) render(ctx context.Context, req queryproxy.Request) (entry dataEntry, err error) {
	defer func() {
		if p := recover(); p != nil {
			h.logPanic(ctx, req.PanelID, p)
			entry, err = dataEntry{}, errInternal
		}
	}()
	found, _ := req.Dashboard.FindDataPanel(req.PanelID)
	parts := make([][]byte, len(found.Panel.Queries))
	exemplarParts := make([][]byte, len(found.Panel.Queries))
	var (
		mu    sync.Mutex
		total int
	)
	req.Admit = func(index int, q queryproxy.QueryResult) error {
		part, err := marshalFrames([]queryproxy.QueryResult{q})
		if err != nil {
			h.logger.ErrorContext(ctx, "convert panel frames", append([]any{slog.String("panel_id", req.PanelID)}, failureAttrs(err)...)...)
			return errInternal
		}
		var exemplars []byte
		if q.ExemplarStatus != "" {
			if exemplars, err = marshalExemplars(q); err != nil {
				h.logger.ErrorContext(ctx, "convert panel exemplars", append([]any{slog.String("panel_id", req.PanelID)}, failureAttrs(err)...)...)
				return errInternal
			}
		}
		mu.Lock()
		defer mu.Unlock()
		parts[index], exemplarParts[index] = part, exemplars
		if total += len(part) + len(exemplars); total > maxResponseBytes {
			return &queryproxy.AbortError{Err: errTooLarge}
		}
		return nil
	}
	res, err := h.runner.RunPanel(ctx, req)
	if err != nil {
		return dataEntry{}, err
	}
	queries := make([]queryMeta, len(res.Queries))
	asked := false
	for i, q := range res.Queries {
		queries[i] = queryMeta{Ref: q.Ref, Kind: q.Kind, StepMS: q.Step.Milliseconds(), Exemplars: q.ExemplarStatus}
		asked = asked || q.ExemplarStatus != ""
	}
	var exemplars json.RawMessage
	if asked {
		exemplars = joinFrames(exemplarParts)
	}
	return dataEntry{frames: joinFrames(parts), exemplars: exemplars, queries: queries, kind: res.Kind, from: res.Range.From, to: res.Range.To, step: res.Step, meta: resultMetaOf(res.Frames()), executedAt: time.Now()}, nil
}

func (h *Handler) logPanic(ctx context.Context, panelID string, p any) {
	h.logger.ErrorContext(ctx, "query proxy panicked",
		slog.String("panel_id", panelID),
		slog.String("panic_type", fmt.Sprintf("%T", p)),
		slog.String("stack", string(debug.Stack())))
}

func cacheScope(ctx context.Context, v loadedVersion) (user datasource.QueryUser, scope string) {
	principal, _ := auth.FromContext(ctx)
	user = queryproxy.UserFor(principal)
	if v.usesUser {
		scope = principalScope(principal.Kind, user)
	}
	return user, scope
}

func writeData(c *gin.Context, e dataEntry, cached bool, executedAt time.Time, sourcePanel string) {
	c.JSON(http.StatusOK, dataResponse{
		Frames:    e.frames,
		Exemplars: e.exemplars,
		Meta: dataMeta{
			Kind:        e.kind,
			From:        formatTime(e.from),
			To:          formatTime(e.to),
			StepMS:      e.step.Milliseconds(),
			Queries:     e.queries,
			Cached:      cached,
			ExecutedAt:  formatTime(executedAt),
			SourcePanel: sourcePanel,
			Truncated:   e.meta.truncated,
			Partial:     e.meta.partial,
			Rows:        e.meta.rows,
			RowsTotal:   e.meta.rowsTotal,
			Oldest:      formatEdge(e.meta.oldest),
			Newest:      formatEdge(e.meta.newest),
		},
	})
}

func (h *Handler) values(c *gin.Context) {
	ctx := c.Request.Context()
	query, ok := h.readQuery(c)
	if !ok {
		return
	}
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	id, version, err := address(c)
	if err != nil {
		h.fail(c, err)
		return
	}
	p, err := parseParams(query, false)
	if err != nil {
		h.fail(c, err)
		return
	}
	v, err := h.dashboard(ctx, id, version)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.serveValues(c, p, v, c.Param("name"), &cachedVersion{id: id, version: version})
}

func (h *Handler) serveValues(c *gin.Context, p params, v loadedVersion, name string, cached *cachedVersion) {
	ctx := c.Request.Context()
	if cached == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
		defer cancel()
	}
	d := v.dashboard
	rng, _, err := p.timeRange(d, time.Now())
	if err != nil {
		h.fail(c, err)
		return
	}
	entry, hit, err := h.variableOptions(ctx, v, cached, name, valuesRange(rng), p.selections)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, entry.response(hit))
}

func valuesRange(rng datasource.TimeRange) datasource.TimeRange {
	return align(rng, queryproxy.KindRange, defaultMaxDataPoints).rng
}

func (h *Handler) variableOptions(ctx context.Context, v loadedVersion, cached *cachedVersion, name string, rng datasource.TimeRange, selections map[string]queryproxy.Selection) (entry valuesEntry, hit bool, err error) {
	d := v.dashboard
	user, scope := cacheScope(ctx, v)
	if cached == nil {
		options, truncated, err := h.runner.VariableValues(ctx, d, name, rng, selections, user)
		return newValuesEntry(options, truncated), false, err
	}
	key := valuesKey(cached.id, cached.version, name, rng.From, rng.To, selections, scope)
	if entry, _, ok := h.caches.values.Get(key); ok {
		return entry, true, nil
	}
	options, truncated, err := h.runner.VariableValues(ctx, d, name, rng, selections, user)
	if err != nil {
		return valuesEntry{}, false, err
	}
	entry = newValuesEntry(options, truncated)
	if i := slices.IndexFunc(d.Variables, func(v spec.Variable) bool { return v.Name == name }); i >= 0 && d.Variables[i].Type == spec.QueryVariable {
		if entry.cost() <= valuesMaxEntry {
			h.caches.values.Put(key, entry, entry.cost())
		}
	}
	return entry, false, nil
}

func (h *Handler) lint(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	inst, found := h.registry.Get(c.Param("uid"))
	if !found {
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "datasource not found")
		return
	}
	var req lintRequest
	if status, code, msg := decodeLint(c, &req); status != 0 {
		abortError(c, status, code, msg)
		return
	}
	releaseUpstream, ok := h.acquireUpstream(c)
	if !ok {
		return
	}
	defer releaseUpstream()
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.lintTimeout)
	defer cancel()
	diagnostics, err := h.safeLint(ctx, inst, *req.Query)
	if err != nil {
		h.fail(c, err)
		return
	}
	if err := c.Request.Context().Err(); err != nil {
		h.fail(c, err)
		return
	}
	if diagnostics == nil {
		diagnostics = []datasource.Diagnostic{}
	}
	c.JSON(http.StatusOK, lintResponse{Diagnostics: diagnostics})
}

func (h *Handler) acquireUpstream(c *gin.Context) (release func(), ok bool) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.lintTimeout)
	defer cancel()
	release, err := h.runner.AcquireUpstream(ctx)
	switch {
	case err == nil:
		return release, true
	case c.Request.Context().Err() == nil:
		tooManyRequests(c)
	default:
		h.fail(c, c.Request.Context().Err())
	}
	return nil, false
}

func (h *Handler) safeLint(ctx context.Context, inst datasource.Instance, text string) (diagnostics []datasource.Diagnostic, err error) {
	defer func() {
		if p := recover(); p != nil {
			h.logger.ErrorContext(ctx, "datasource lint panicked",
				slog.String("datasource", inst.UID),
				slog.String("panic_type", fmt.Sprintf("%T", p)),
				slog.String("stack", string(debug.Stack())))
			diagnostics, err = nil, errInternal
		}
	}()
	return inst.Datasource.Lint(ctx, text)
}

func decodeLint(c *gin.Context, req *lintRequest) (status int, code, message string) {
	invalid := string(queryproxy.CodeInvalidRequest)
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxLintBodyBytes))
	dec.DisallowUnknownFields()
	err := dec.Decode(req)
	if err == nil {
		if _, err = dec.Token(); !errors.Is(err, io.EOF) {
			return http.StatusBadRequest, invalid, "request body must be a single JSON object"
		}
	} else {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return http.StatusRequestEntityTooLarge, "too_large", "request body is too large"
		}
		return http.StatusBadRequest, invalid, "request body must be valid JSON with known fields"
	}
	if req.Query != nil && len(*req.Query) > maxLintQueryBytes {
		return http.StatusBadRequest, string(queryproxy.CodeQueryTooLong), "query must be at most 64 KiB"
	}
	if req.Query == nil || len(*req.Query) == 0 {
		return http.StatusBadRequest, invalid, "query must be from 1 byte to 64 KiB"
	}
	return 0, "", ""
}

func (h *Handler) dashboard(ctx context.Context, id pgtype.UUID, version int) (loadedVersion, error) {
	key := specKey{id: id, version: version}
	if v, _, ok := h.caches.specs.Get(key); ok {
		return v, nil
	}
	d, err := h.specs.VersionSpec(ctx, id, version)
	switch {
	case err == nil:
		v := loadedVersion{dashboard: d, usesUser: queryproxy.DashboardUsesUser(d)}
		h.caches.specs.Put(key, v, 1)
		return v, nil
	case errors.Is(err, dashboards.ErrNotFound), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return loadedVersion{}, err
	default:
		h.logger.ErrorContext(ctx, "load dashboard version", failureAttrs(err)...)
		return loadedVersion{}, errInternal
	}
}

func failureAttrs(err error) []any {
	_, code, _ := queryproxy.Classify(err)
	return append([]any{slog.String("error_code", code)}, logattrs.Safe(err)...)
}

func (h *Handler) fail(c *gin.Context, err error) {
	if clientgone.Handle(c, h.logger, "query proxy", err) {
		return
	}
	if errors.Is(err, dashboards.ErrNotFound) {
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), err.Error())
		return
	}
	if errors.Is(err, errTooLarge) {
		abortError(c, http.StatusUnprocessableEntity, string(queryproxy.CodeLimitExceeded), errTooLarge.Error())
		return
	}
	status, code, message := queryproxy.Classify(err)
	if status == 0 {
		h.logger.ErrorContext(c.Request.Context(), "query proxy: canceled while the client is still connected", failureAttrs(err)...)
		status, code, message = queryproxy.Classify(errInternal)
	}
	var qe *queryproxy.QueryError
	if errors.As(err, &qe) {
		c.AbortWithStatusJSON(status, gin.H{"error": message, "code": code, "ref": qe.Ref})
		return
	}
	abortError(c, status, code, message)
}

func (h *Handler) readQuery(c *gin.Context) (url.Values, bool) {
	query, err := parseQuery(c.Request.URL.RawQuery)
	switch {
	case err == nil:
		return query, true
	case errors.Is(err, errQueryNotInSpec):
		forbidQuery(c)
	default:
		h.fail(c, err)
	}
	return nil, false
}

func (h *Handler) acquire(c *gin.Context) (release func(), ok bool) {
	principal, _ := auth.FromContext(c.Request.Context())
	return h.acquireKey(c, principal.ID)
}

func (h *Handler) acquireKey(c *gin.Context, key string) (release func(), ok bool) {
	release, ok = h.slots.acquire(key)
	if !ok {
		tooManyRequests(c)
	}
	return release, ok
}

func tooManyRequests(c *gin.Context) {
	c.Header("Retry-After", "1")
	abortError(c, http.StatusTooManyRequests, "too_many_requests", "too many in-flight query requests")
}

func forbidQuery(c *gin.Context) {
	abortError(c, http.StatusForbidden, "query_not_in_spec", "the query proxy only runs saved panel queries")
}

func abortError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message, "code": code})
}

func address(c *gin.Context) (id pgtype.UUID, version int, err error) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		return id, 0, invalidf("id must be a UUID")
	}
	n, convErr := strconv.Atoi(c.Param("version"))
	if convErr != nil || !validVersion(n) || strconv.Itoa(n) != c.Param("version") {
		return id, 0, invalidf("version must be a positive 32-bit integer")
	}
	return id, n, nil
}

func parseID(raw string) (id pgtype.UUID, ok bool) {
	id, err := db.ParseUUID(raw)
	return id, err == nil
}

func validVersion(n int) bool {
	return n >= 1 && n <= maxVersion
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

type slots struct {
	mu   sync.Mutex
	held map[string]int
}

func (s *slots) acquire(principalID string) (release func(), ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[principalID] >= maxInFlightPerPrincipal {
		return nil, false
	}
	s.held[principalID]++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.held[principalID]--; s.held[principalID] <= 0 {
				delete(s.held, principalID)
			}
		})
	}, true
}
