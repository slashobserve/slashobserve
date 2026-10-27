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

type Handler struct {
	runner   *queryproxy.Runner
	specs    SpecSource
	registry *datasource.Registry
	logger   *slog.Logger
	caches   *caches
	slots    *slots

	lintTimeout time.Duration
}

func NewHandler(runner *queryproxy.Runner, specs SpecSource, registry *datasource.Registry, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		runner:   runner,
		specs:    specs,
		registry: registry,
		logger:   logger,
		caches:   newCaches(),
		slots:    &slots{held: map[string]int{}},

		lintTimeout: lintTimeout,
	}
}

func (h *Handler) Register(api *gin.RouterGroup) {
	api.GET("/dashboards/:id/versions/:version/panels/:panel_id/data", h.data)
	api.GET("/dashboards/:id/versions/:version/variables/:name/values", h.values)
	api.POST("/datasources/:uid/lint", h.lint)
	api.POST("/datasources/:uid/preview", h.preview)
	api.GET("/datasources/:uid/metrics", h.metrics)
	api.GET("/datasources/:uid/labels", h.labels)
	api.GET("/datasources/:uid/labels/:name/values", h.labelValues)
}

type dataMeta struct {
	Kind       queryproxy.Kind `json:"kind"`
	From       string          `json:"from"`
	To         string          `json:"to"`
	StepMS     int64           `json:"step_ms"`
	Queries    []queryMeta     `json:"queries"`
	Cached     bool            `json:"cached"`
	ExecutedAt string          `json:"executed_at"`
}

type dataResponse struct {
	Frames json.RawMessage `json:"frames"`
	Meta   dataMeta        `json:"meta"`
}

type valuesResponse struct {
	Values    []string `json:"values"`
	Truncated bool     `json:"truncated"`
	Cached    bool     `json:"cached"`
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
	d := v.dashboard
	panelID := c.Param("panel_id")
	found, ok := d.FindDataPanel(panelID)
	if !ok {
		h.fail(c, fmt.Errorf("%w: %q", queryproxy.ErrPanelNotFound, panelID))
		return
	}
	now := time.Now()
	rng, err := p.timeRange(d, now)
	if err == nil {
		rng, err = panelTimeRange(found.Panel, rng, now)
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
	key := dataKey(id, version, panelID, kind, al, selections, scope)
	if entry, at, ok := h.caches.data.Get(key); ok {
		writeData(c, entry, true, at)
		return
	}
	entry, err := h.caches.dataFlights.Do(ctx, string(key[:]), func(ctx context.Context) (dataEntry, error) {
		return h.execute(ctx, key, d, panelID, al, selections, user)
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	writeData(c, entry, false, entry.executedAt)
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
	entry, err := h.render(ctx, queryproxy.Request{
		Dashboard:     d,
		PanelID:       panelID,
		Range:         al.rng,
		Selections:    selections,
		MaxDataPoints: al.maxDataPoints,
		User:          user,
	})
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
		mu.Lock()
		defer mu.Unlock()
		parts[index] = part
		if total += len(part); total > maxResponseBytes {
			return &queryproxy.AbortError{Err: errTooLarge}
		}
		return nil
	}
	res, err := h.runner.RunPanel(ctx, req)
	if err != nil {
		return dataEntry{}, err
	}
	queries := make([]queryMeta, len(res.Queries))
	for i, q := range res.Queries {
		queries[i] = queryMeta{Ref: q.Ref, Kind: q.Kind, StepMS: q.Step.Milliseconds()}
	}
	return dataEntry{frames: joinFrames(parts), queries: queries, kind: res.Kind, from: res.Range.From, to: res.Range.To, step: res.Step, executedAt: time.Now()}, nil
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

func writeData(c *gin.Context, e dataEntry, cached bool, executedAt time.Time) {
	c.JSON(http.StatusOK, dataResponse{
		Frames: e.frames,
		Meta: dataMeta{
			Kind:       e.kind,
			From:       formatTime(e.from),
			To:         formatTime(e.to),
			StepMS:     e.step.Milliseconds(),
			Queries:    e.queries,
			Cached:     cached,
			ExecutedAt: formatTime(executedAt),
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
	d := v.dashboard
	rng, err := p.timeRange(d, time.Now())
	if err != nil {
		h.fail(c, err)
		return
	}
	name := c.Param("name")
	rng = align(rng, queryproxy.KindRange, defaultMaxDataPoints).rng
	user, scope := cacheScope(ctx, v)
	key := valuesKey(id, version, name, rng.From, rng.To, p.selections, scope)
	if entry, _, ok := h.caches.values.Get(key); ok {
		c.JSON(http.StatusOK, valuesResponse{Values: entry.values, Truncated: entry.truncated, Cached: true})
		return
	}
	values, truncated, err := h.runner.VariableValues(ctx, d, name, rng, p.selections, user)
	if err != nil {
		h.fail(c, err)
		return
	}
	if values == nil {
		values = []string{}
	}
	if i := slices.IndexFunc(d.Variables, func(v spec.Variable) bool { return v.Name == name }); i >= 0 && d.Variables[i].Type == spec.QueryVariable {
		if entry := (valuesEntry{values: values, truncated: truncated}); entry.cost() <= valuesMaxEntry {
			h.caches.values.Put(key, entry, entry.cost())
		}
	}
	c.JSON(http.StatusOK, valuesResponse{Values: values, Truncated: truncated})
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
	release, ok = h.slots.acquire(principal.ID)
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
