package tailhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	pollInterval        = time.Second
	pingInterval        = 15 * time.Second
	maxStreamLifetime   = 30 * time.Minute
	retryMilliseconds   = 3000
	maxTailsPerUser     = 5
	maxTailsPerProc     = 200
	maxTransientErrors  = 5
	maxFailedChecks     = 2
	writeTimeout        = 30 * time.Second
	sessionCheckTimeout = 5 * time.Second
	defaultLookback     = 10 * time.Second
	defaultLimit        = 500
	maxLimit            = 1000
	maxQueryBytes       = 64 << 10
	maxTimeParamBytes   = 128
	eventStreamContent  = "text/event-stream"
	frameRef            = "A"
	notLineSource       = "tail reads Loki and ClickHouse datasources"
)

type Sessions interface {
	Valid(r *http.Request) (bool, error)
}

type Handler struct {
	registry     *datasource.Registry
	runner       *queryproxy.Runner
	sessions     Sessions
	logger       *slog.Logger
	pollInterval time.Duration
	pingInterval time.Duration

	mu          sync.Mutex
	total       int
	byPrincipal map[string]int
}

func NewHandler(reg *datasource.Registry, runner *queryproxy.Runner, sessions Sessions, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{registry: reg, runner: runner, sessions: sessions, logger: logger, pollInterval: pollInterval, pingInterval: pingInterval, byPrincipal: map[string]int{}}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/datasources/:uid/tail", auth.RoleViewer, h.serve)
}

type tailRequest struct {
	uid    string
	query  string
	from   time.Time
	limit  int
	user   datasource.QueryUser
	source datasource.LineSource
}

type readyData struct {
	DatasourceUID string `json:"datasource_uid"`
	From          string `json:"from"`
}

type linesData struct {
	Frames    json.RawMessage `json:"frames"`
	Newest    string          `json:"newest"`
	Truncated bool            `json:"truncated"`
}

type errorData struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

func (h *Handler) serve(c *gin.Context) {
	req, ok := h.parse(c)
	if !ok {
		return
	}
	principal, _ := auth.FromContext(c.Request.Context())
	key := string(principal.Kind) + ":" + principal.ID
	if status, code := h.acquire(key); status != 0 {
		abort(c, status, code, "too many open tail streams")
		return
	}
	defer h.release(key)

	t := &tail{h: h, c: c, req: req, last: req.from}
	first, err := t.poll()
	if err != nil {
		h.failBeforeStream(c, req.uid, err)
		return
	}
	header := c.Writer.Header()
	header.Set("Content-Type", eventStreamContent)
	header.Set("Cache-Control", "no-store")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	defer func() { _ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}) }()
	if !write(c, fmt.Sprintf("retry: %d\n\n", retryMilliseconds)) ||
		!writeEvent(c, "ready", readyData{DatasourceUID: req.uid, From: httpapi.FormatTime(req.from)}) ||
		!t.emit(first) {
		return
	}
	t.pump(first.truncated)
}

func (h *Handler) parse(c *gin.Context) (tailRequest, bool) {
	invalid := func(message string) (tailRequest, bool) {
		abort(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message)
		return tailRequest{}, false
	}
	values := c.Request.URL.Query()
	for name := range values {
		switch name {
		case "query", "from", "limit", "timezone":
			if len(values[name]) != 1 {
				return invalid(name + " must be given once")
			}
		default:
			return invalid("unknown query parameter")
		}
	}
	req := tailRequest{uid: c.Param("uid"), query: values.Get("query"), limit: defaultLimit}
	switch {
	case req.query == "":
		return invalid("query must be from 1 byte to 64 KiB")
	case len(req.query) > maxQueryBytes:
		abort(c, http.StatusBadRequest, string(queryproxy.CodeQueryTooLong), "query must be at most 64 KiB")
		return tailRequest{}, false
	}
	if raw := values.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || strconv.Itoa(n) != raw || n < 1 || n > maxLimit {
			return invalid("limit must be an integer from 1 to " + strconv.Itoa(maxLimit))
		}
		req.limit = n
	}
	from, err := parseFrom(values, time.Now())
	if err != nil {
		return invalid(err.Error())
	}
	req.from = from
	inst, found := h.registry.Get(req.uid)
	if !found {
		abort(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "datasource not found")
		return tailRequest{}, false
	}
	source, ok := inst.Datasource.(datasource.LineSource)
	if !ok {
		return invalid(notLineSource)
	}
	req.source = source
	principal, _ := auth.FromContext(c.Request.Context())
	req.user = queryproxy.UserFor(principal)
	return req, true
}

func parseFrom(values url.Values, now time.Time) (time.Time, error) {
	var opts spec.TimeOptions
	if zone := values.Get("timezone"); zone != "" {
		loc, err := spec.LoadZone(zone)
		if err != nil {
			return time.Time{}, errors.New("timezone is not a known time zone")
		}
		opts.Location = loc
	}
	raw := values.Get("from")
	if raw == "" {
		return now.Add(-defaultLookback), nil
	}
	if len(raw) > maxTimeParamBytes {
		return time.Time{}, fmt.Errorf("from must be a time expression of 1 to %d bytes", maxTimeParamBytes)
	}
	from, err := spec.ResolveTime(raw, now, false, opts)
	if err != nil {
		return time.Time{}, errors.New("from is not a valid time")
	}
	if !from.Before(now) {
		return time.Time{}, errors.New("from must be before now")
	}
	return from, nil
}

type tail struct {
	h    *Handler
	c    *gin.Context
	req  tailRequest
	last time.Time
}

type pollResult struct {
	frames    []datasource.Frame
	newest    time.Time
	truncated bool
}

func (t *tail) poll() (pollResult, error) {
	ctx, cancel := context.WithTimeout(t.c.Request.Context(), queryproxy.DraftLimits.Timeout)
	defer cancel()
	release, err := t.h.runner.AcquireUpstream(ctx)
	if err != nil {
		if t.c.Request.Context().Err() != nil {
			return pollResult{}, context.Canceled
		}
		return pollResult{}, queryproxy.ErrUpstreamBusy
	}
	defer release()
	now := time.Now()
	if !t.last.Before(now) {
		return pollResult{}, nil
	}
	frames, err := t.req.source.Lines(ctx, datasource.LinesRequest{
		Query:     datasource.Query{Text: t.req.query, Limits: queryproxy.DraftLimits, User: t.req.user},
		Range:     datasource.TimeRange{From: t.last, To: now},
		Direction: datasource.Forward,
		Limit:     t.req.limit,
	})
	if err != nil {
		return pollResult{}, err
	}
	res := pollResult{frames: frames, truncated: datasource.MergeMeta(frames).Truncated}
	if newest, ok := newestOf(frames); ok {
		res.newest = newest
		t.last = newest.Add(time.Nanosecond)
	}
	return res, nil
}

func newestOf(frames []datasource.Frame) (time.Time, bool) {
	var newest time.Time
	found := false
	for _, f := range frames {
		for row := range f.Rows() {
			at, ok := rowTime(f, row)
			if ok && (!found || at.After(newest)) {
				newest, found = at, true
			}
		}
	}
	return newest, found
}

func rowTime(f datasource.Frame, row int) (time.Time, bool) {
	if ns, ok := datasource.TimeNsOf(f, row); ok {
		return time.Unix(0, ns).UTC(), true
	}
	for _, field := range f.Fields {
		if field.Type != datasource.FieldTime || row >= len(field.Values) {
			continue
		}
		if at, ok := field.Values[row].(time.Time); ok {
			return at, true
		}
	}
	return time.Time{}, false
}

func (t *tail) emit(res pollResult) bool {
	if datasource.Points(res.frames) == 0 {
		return true
	}
	frames, err := marshalFrames(frameRef, res.frames)
	if err != nil {
		t.h.logger.ErrorContext(t.c.Request.Context(), "convert tail frames", logattrs.Failure(string(queryproxy.CodeInternal), err, slog.String("datasource_uid", t.req.uid))...)
		writeEvent(t.c, "error", errorData{Code: string(queryproxy.CodeInternal), Error: "internal error"})
		return false
	}
	return writeEvent(t.c, "lines", linesData{Frames: frames, Newest: httpapi.FormatTime(res.newest), Truncated: res.truncated})
}

func (t *tail) pump(immediate bool) {
	poll := time.NewTimer(t.h.pollDelay(immediate))
	defer poll.Stop()
	ping := time.NewTicker(t.h.pingInterval)
	defer ping.Stop()
	lifetime := time.NewTimer(maxStreamLifetime)
	defer lifetime.Stop()
	var transient, failedChecks int
	for {
		select {
		case <-poll.C:
			immediate, alive := t.step(&transient)
			if !alive {
				return
			}
			poll.Reset(t.h.pollDelay(immediate))
		case <-ping.C:
			if !t.ping(&failedChecks) {
				return
			}
		case <-lifetime.C:
			return
		case <-t.c.Request.Context().Done():
			return
		}
	}
}

func (t *tail) step(transient *int) (immediate, alive bool) {
	res, err := t.poll()
	switch {
	case t.c.Request.Context().Err() != nil:
		return false, false
	case err == nil:
		*transient = 0
		return res.truncated, t.emit(res)
	case transientError(err) && *transient+1 < maxTransientErrors:
		*transient++
		return false, true
	default:
		t.fail(err)
		return false, false
	}
}

func (t *tail) ping(failedChecks *int) bool {
	valid, failed := t.h.credentialsValid(t.c)
	if failed {
		*failedChecks++
	} else {
		*failedChecks = 0
	}
	return valid && *failedChecks < maxFailedChecks && write(t.c, ": ping\n\n")
}

func (h *Handler) pollDelay(immediate bool) time.Duration {
	if immediate {
		return 0
	}
	return h.pollInterval
}

func transientError(err error) bool {
	return datasource.IsTransient(err) || errors.Is(err, queryproxy.ErrUpstreamBusy)
}

func (t *tail) fail(err error) {
	status, code, message := queryproxy.Classify(err)
	if status == 0 || status >= http.StatusInternalServerError {
		t.h.logger.ErrorContext(t.c.Request.Context(), "tail poll failed", logattrs.Failure(code, err, slog.String("datasource_uid", t.req.uid))...)
	}
	writeEvent(t.c, "error", errorData{Code: code, Error: message})
}

func (h *Handler) failBeforeStream(c *gin.Context, uid string, err error) {
	if clientgone.Handle(c, h.logger, "tail", err) {
		return
	}
	status, code, message := queryproxy.Classify(err)
	if status == 0 {
		status, code, message = http.StatusInternalServerError, string(queryproxy.CodeInternal), "internal error"
	}
	if status >= http.StatusInternalServerError {
		h.logger.ErrorContext(c.Request.Context(), "tail poll failed", logattrs.Failure(code, err, slog.String("datasource_uid", uid))...)
	}
	abort(c, status, code, message)
}

func (h *Handler) credentialsValid(c *gin.Context) (valid, failed bool) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sessionCheckTimeout)
	defer cancel()
	ok, err := h.sessions.Valid(c.Request.WithContext(ctx))
	if err != nil {
		if c.Request.Context().Err() == nil {
			h.logger.WarnContext(c.Request.Context(), "re-check tail credentials", logattrs.Failure("tail_auth_check_failed", err)...)
		}
		return c.Request.Context().Err() == nil, true
	}
	return ok, false
}

func (h *Handler) acquire(key string) (int, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byPrincipal[key] >= maxTailsPerUser {
		return http.StatusTooManyRequests, "too_many_streams"
	}
	if h.total >= maxTailsPerProc {
		return http.StatusServiceUnavailable, "too_many_streams"
	}
	h.byPrincipal[key]++
	h.total++
	return 0, ""
}

func (h *Handler) release(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.total--
	if h.byPrincipal[key]--; h.byPrincipal[key] <= 0 {
		delete(h.byPrincipal, key)
	}
}

func writeEvent(c *gin.Context, name string, data any) bool {
	body, err := json.Marshal(data)
	if err != nil {
		return false
	}
	return write(c, fmt.Sprintf("event: %s\ndata: %s\n\n", name, body))
}

func write(c *gin.Context, s string) bool {
	rc := http.NewResponseController(c.Writer)
	if err := rc.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return false
	}
	if _, err := c.Writer.WriteString(s); err != nil {
		return false
	}
	c.Writer.Flush()
	return true
}

func abort(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message, "code": code})
}
