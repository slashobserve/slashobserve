package proxyhttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	maxTraceLogsBodyBytes = 4 << 10
	defaultTraceLogsLimit = 500
	traceLogsRef          = "A"
)

type traceLogsRequest struct {
	TraceID  *string `json:"trace_id"`
	SpanID   *string `json:"span_id"`
	Service  *string `json:"service"`
	From     *string `json:"from"`
	To       *string `json:"to"`
	Timezone *string `json:"timezone"`
	Limit    *int    `json:"limit"`
}

func init() {
	registerRoutes(func(h *Handler, api auth.Routes) {
		api.POST("/datasources/:uid/trace-logs", auth.RoleViewer, h.traceLogs)
	})
}

func (h *Handler) traceLogs(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	var req traceLogsRequest
	if !httpapi.DecodeBody(c, &req, maxTraceLogsBodyBytes) {
		return
	}
	if status, code, message := req.validate(); status != 0 {
		abortError(c, status, code, message)
		return
	}
	uid := c.Param("uid")
	inst, found := h.registry.Get(uid)
	if !found {
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "datasource not found")
		return
	}
	source, supported := inst.Datasource.(datasource.TraceLogSource)
	if !supported {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), "trace logs read Loki and ClickHouse datasources")
		return
	}
	p, err := parseParams(req.values(), false)
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	rng, _, err := p.timeRange(exploreDashboard(uid, "", queryproxy.KindInstant), time.Now())
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	releaseUpstream, ok := h.acquireUpstream(c)
	if !ok {
		return
	}
	defer releaseUpstream()
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	frames, err := h.safeTraceLogs(ctx, uid, source, datasource.TraceLogsRequest{
		TraceID: *req.TraceID,
		SpanID:  optional(req.SpanID),
		Service: optional(req.Service),
		Range:   rng,
		Limit:   req.limit(),
		User:    queryproxy.UserFor(principal),
	})
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	entry, err := traceLogsEntry(rng, frames)
	if err != nil {
		h.logger.ErrorContext(ctx, "convert trace log frames", append([]any{slog.String("datasource_uid", uid)}, failureAttrs(err)...)...)
		h.fail(c, errInternal)
		return
	}
	writeData(c, entry, false, entry.executedAt, "")
}

func (h *Handler) safeTraceLogs(ctx context.Context, uid string, source datasource.TraceLogSource, req datasource.TraceLogsRequest) (frames []datasource.Frame, err error) {
	defer func() {
		if p := recover(); p != nil {
			h.logger.ErrorContext(ctx, "datasource trace logs panicked",
				slog.String("datasource_uid", uid),
				slog.String("panic_type", fmt.Sprintf("%T", p)),
				slog.String("stack", string(debug.Stack())))
			frames, err = nil, errInternal
		}
	}()
	return source.TraceLogs(ctx, req)
}

func traceLogsEntry(rng datasource.TimeRange, frames []datasource.Frame) (dataEntry, error) {
	part, err := marshalFrames([]queryproxy.QueryResult{{Ref: traceLogsRef, Kind: queryproxy.KindInstant, Frames: frames}})
	if err != nil {
		return dataEntry{}, err
	}
	return dataEntry{
		frames:     part,
		queries:    []queryMeta{{Ref: traceLogsRef, Kind: queryproxy.KindInstant}},
		kind:       queryproxy.KindInstant,
		from:       rng.From,
		to:         rng.To,
		meta:       resultMetaOf(frames),
		executedAt: time.Now(),
	}, nil
}

func (r traceLogsRequest) validate() (status int, code, message string) {
	invalid := func(message string) (int, string, string) {
		return http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message
	}
	switch {
	case r.TraceID == nil || !datasource.ValidTraceID(*r.TraceID):
		return invalid("trace_id must be 16 or 32 hex characters")
	case r.SpanID != nil && !datasource.ValidSpanID(*r.SpanID):
		return invalid("span_id must be 16 hex characters")
	case r.Service != nil && (*r.Service == "" || len(*r.Service) > datasource.MaxTraceLogsServiceBytes):
		return invalid("service must be from 1 to " + strconv.Itoa(datasource.MaxTraceLogsServiceBytes) + " bytes")
	case r.From == nil || r.To == nil:
		return invalid("from and to are required")
	case r.Limit != nil && (*r.Limit < 1 || *r.Limit > queryproxy.DraftLimits.MaxRows):
		return invalid("limit must be from 1 to " + strconv.Itoa(queryproxy.DraftLimits.MaxRows))
	}
	return 0, "", ""
}

func (r traceLogsRequest) values() url.Values {
	q := url.Values{}
	q.Set(paramFrom, *r.From)
	q.Set(paramTo, *r.To)
	if r.Timezone != nil {
		q.Set(paramTimezone, *r.Timezone)
	}
	return q
}

func (r traceLogsRequest) limit() int {
	if r.Limit == nil {
		return defaultTraceLogsLimit
	}
	return *r.Limit
}

func optional(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
