package proxyhttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	traceRef             = "A"
	traceDefaultFrom     = "now-24h"
	paramTraceTimezone   = "timezone"
	traceNotFoundMessage = "trace not found in the range; widen from and to"
	traceSourcesMessage  = "traces read Tempo and ClickHouse datasources"
	traceIDMessage       = "trace_id must be 16 or 32 hex characters"
)

func (h *Handler) trace(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	id := strings.ToLower(c.Param("trace_id"))
	if !datasource.ValidTraceID(id) {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), traceIDMessage)
		return
	}
	uid := c.Param("uid")
	inst, found := h.registry.Get(uid)
	if !found {
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "datasource not found")
		return
	}
	source, ok := inst.Datasource.(datasource.TraceSource)
	if !ok {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), traceSourcesMessage)
		return
	}
	p, err := parseParams(traceParams(c.Request.URL.Query()), false)
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	rng, _, err := p.timeRange(traceDashboard(uid), time.Now())
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	releaseUpstream, ok := h.acquireUpstream(c)
	if !ok {
		return
	}
	defer releaseUpstream()
	f, err := source.Trace(ctx, id, rng, datasource.TraceSpanBudget)
	switch {
	case errors.Is(err, datasource.ErrNotFound):
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), traceNotFoundMessage)
		return
	case err != nil:
		h.exploreFail(c, uid, err)
		return
	}
	frames, err := marshalFrames([]queryproxy.QueryResult{{Ref: traceRef, Kind: queryproxy.KindInstant, Frames: []datasource.Frame{f}}})
	if err != nil {
		h.logger.ErrorContext(ctx, "convert trace frames", append([]any{slog.String("datasource_uid", uid)}, failureAttrs(err)...)...)
		h.fail(c, errInternal)
		return
	}
	if len(frames) > maxResponseBytes {
		h.fail(c, errTooLarge)
		return
	}
	entry := dataEntry{
		frames:     frames,
		queries:    []queryMeta{{Ref: traceRef, Kind: queryproxy.KindInstant}},
		kind:       queryproxy.KindInstant,
		from:       rng.From,
		to:         rng.To,
		meta:       resultMetaOf([]datasource.Frame{f}),
		executedAt: time.Now(),
	}
	writeData(c, entry, false, entry.executedAt, "")
}

func traceParams(q url.Values) url.Values {
	if zone, ok := q[paramTraceTimezone]; ok {
		delete(q, paramTraceTimezone)
		q[paramTimezone] = zone
	}
	return q
}

func traceDashboard(uid string) *spec.Dashboard {
	d := exploreDashboard(uid, "", queryproxy.KindInstant)
	d.Time = spec.TimeRange{From: traceDefaultFrom, To: "now"}
	return d
}
