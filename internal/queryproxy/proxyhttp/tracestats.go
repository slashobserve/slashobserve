package proxyhttp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const traceStatsSourcesMessage = "trace stats read Tempo and ClickHouse datasources"

type traceStatsRequest struct {
	Query   *string `json:"query"`
	From    *string `json:"from"`
	To      *string `json:"to"`
	Buckets *int    `json:"buckets"`
}

type traceStatsJSON struct {
	StepMS    int64                `json:"step_ms"`
	Partial   bool                 `json:"partial"`
	Spans     int64                `json:"spans"`
	Errors    int64                `json:"errors"`
	Buckets   []traceBucketJSON    `json:"buckets"`
	Histogram []durationBucketJSON `json:"histogram"`
}

type traceBucketJSON struct {
	Start  string   `json:"start"`
	Spans  int64    `json:"spans"`
	Errors int64    `json:"errors"`
	P50MS  *float64 `json:"p50_ms"`
	P95MS  *float64 `json:"p95_ms"`
	P99MS  *float64 `json:"p99_ms"`
}

type durationBucketJSON struct {
	LeMS  float64 `json:"le_ms"`
	Count int64   `json:"count"`
}

func init() {
	registerRoutes(func(h *Handler, api auth.Routes) {
		api.POST("/datasources/:uid/trace-stats", auth.RoleViewer, h.traceStats)
	})
}

func (h *Handler) traceStats(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	var req traceStatsRequest
	if !httpapi.DecodeBody(c, &req, maxExploreBodyBytes) {
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
	source, ok := inst.Datasource.(datasource.TraceStatsSource)
	if !ok {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), traceStatsSourcesMessage)
		return
	}
	p, err := parseParams(req.values(), false)
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	rng, _, err := p.timeRange(exploreDashboard(uid, *req.Query, queryproxy.KindRange), time.Now())
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	al := align(rng, queryproxy.KindRange, req.buckets())
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	releaseUpstream, ok := h.acquireUpstream(c, uid)
	if !ok {
		return
	}
	defer releaseUpstream()
	stats, err := source.TraceStats(ctx, datasource.TraceStatsRequest{
		Query:  *req.Query,
		Range:  al.rng,
		Step:   al.step,
		Limits: queryproxy.DraftLimits,
		User:   queryproxy.UserFor(principal),
	})
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	c.JSON(http.StatusOK, newTraceStatsJSON(stats))
}

func newTraceStatsJSON(s datasource.TraceStats) traceStatsJSON {
	out := traceStatsJSON{
		StepMS:    s.Step.Milliseconds(),
		Partial:   s.Partial,
		Spans:     s.Spans,
		Errors:    s.Errors,
		Buckets:   make([]traceBucketJSON, len(s.Buckets)),
		Histogram: make([]durationBucketJSON, len(s.Histogram)),
	}
	for i, b := range s.Buckets {
		out.Buckets[i] = traceBucketJSON{Start: httpapi.FormatTime(b.Start), Spans: b.Spans, Errors: b.Errors, P50MS: b.P50, P95MS: b.P95, P99MS: b.P99}
	}
	for i, b := range s.Histogram {
		out.Histogram[i] = durationBucketJSON{LeMS: b.LeMS, Count: b.Count}
	}
	return out
}

func (r traceStatsRequest) validate() (status int, code, message string) {
	invalid := func(message string) (int, string, string) {
		return http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message
	}
	switch {
	case r.Query == nil:
		return invalid("query is required")
	case len(*r.Query) > maxLintQueryBytes:
		return http.StatusBadRequest, string(queryproxy.CodeQueryTooLong), "query must be at most 64 KiB"
	case r.From == nil || r.To == nil:
		return invalid("from and to are required")
	case r.Buckets != nil && (*r.Buckets < 1 || *r.Buckets > maxVolumeBuckets):
		return invalid("buckets must be from 1 to " + strconv.Itoa(maxVolumeBuckets))
	}
	return 0, "", ""
}

func (r traceStatsRequest) values() url.Values {
	q := url.Values{}
	q.Set(paramFrom, *r.From)
	q.Set(paramTo, *r.To)
	return q
}

func (r traceStatsRequest) buckets() int {
	if r.Buckets == nil {
		return defaultVolumeBuckets
	}
	return *r.Buckets
}
