package proxyhttp

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/clickhouse"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logvolume"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	defaultVolumeBuckets = 60
	maxVolumeBuckets     = 200
)

type volumeRequest struct {
	Query   *string `json:"query"`
	From    *string `json:"from"`
	To      *string `json:"to"`
	Buckets *int    `json:"buckets"`
}

type volumeJSON struct {
	StepMS      int64        `json:"step_ms"`
	Total       int64        `json:"total"`
	Buckets     []bucketJSON `json:"buckets"`
	Partial     bool         `json:"partial"`
	CountedFrom string       `json:"counted_from,omitempty"`
}

type bucketJSON struct {
	Start   string `json:"start"`
	Error   int64  `json:"error"`
	Warning int64  `json:"warning"`
	Info    int64  `json:"info"`
	Other   int64  `json:"other"`
}

type volumeSource struct {
	derive func(query string, step time.Duration) (string, error)
	fold   func(h *logvolume.Histogram, f datasource.Frame, step time.Duration) bool
}

var volumeSources = map[string]volumeSource{
	string(spec.ClickHouse): {derive: clickhouse.VolumeQuery, fold: foldClickHouseVolume},
}

func init() {
	registerRoutes(func(h *Handler, api auth.Routes) {
		api.POST("/datasources/:uid/log-volume", auth.RoleViewer, h.volume)
	})
}

func (h *Handler) volume(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	var req volumeRequest
	if !httpapi.DecodeBody(c, &req, maxExploreBodyBytes) {
		return
	}
	if status, code, message := req.validate(); status != 0 {
		abortError(c, status, code, message)
		return
	}
	uid := c.Param("uid")
	inst, found := h.datasourceFor(c)
	if !found {
		return
	}
	if capability, ok := inst.Datasource.(datasource.LogVolumeSource); ok {
		h.volumeFromSource(c, uid, capability, req)
		return
	}
	source, supported := volumeSources[inst.Datasource.Type()]
	if !supported {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), "volume reads Loki, ClickHouse, Elasticsearch and OpenSearch datasources")
		return
	}
	al, ok := h.volumeWindow(c, uid, req, exploreDashboard(uid, *req.Query, queryproxy.KindInstant))
	if !ok {
		return
	}
	derived, err := source.derive(*req.Query, al.step)
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	res, err := h.runner.RunPanel(ctx, queryproxy.Request{
		Dashboard:     exploreDashboard(uid, derived, queryproxy.KindInstant),
		PanelID:       explorePanelID,
		Range:         al.counted,
		Selections:    map[string]queryproxy.Selection{},
		MaxDataPoints: al.maxDataPoints,
		Limits:        queryproxy.DraftLimits,
		User:          queryproxy.UserFor(principal),
	})
	if err != nil {
		h.exploreFail(c, uid, volumeProblem(err))
		return
	}
	hist := logvolume.New(al.rng.From, al.step, al.maxDataPoints)
	for _, q := range res.Queries {
		for _, f := range q.Frames {
			if !source.fold(hist, f, al.step) {
				h.logger.ErrorContext(ctx, "log volume frame lacks the Time, Level and Count fields", slog.String("datasource_uid", uid))
				h.fail(c, errInternal)
				return
			}
		}
	}
	c.JSON(http.StatusOK, newVolumeJSON(hist.Volume()))
}

var (
	pipelineFailure = regexp.MustCompile(`pipeline error: '([A-Za-z]+)'`)
	wrapperPosition = regexp.MustCompile(`parse error at line \d+, col \d+|failed (?:at (?:position \d+|line \d+, column \d+ of the query|the end of the query)|to parse the query)|fromUnixTimestamp64Milli`)
)

func volumeProblem(err error) error {
	var upstream *datasource.UpstreamError
	if !errors.As(err, &upstream) || !errors.Is(upstream.Kind, datasource.ErrInvalidQuery) {
		return err
	}
	if match := pipelineFailure.FindStringSubmatch(upstream.Message); match != nil {
		return datasource.Errorf(datasource.ErrInvalidQuery, "a parser stage failed on some lines (%s), so the volume cannot count them; add | __error__=\"\" after the parser to skip those lines", match[1])
	}
	if wrapperPosition.MatchString(upstream.Message) {
		return datasource.Errorf(datasource.ErrInvalidQuery, "the volume could not read the query as written; fix the query itself and the volume follows")
	}
	return err
}

func (h *Handler) volumeWindow(c *gin.Context, uid string, req volumeRequest, d *spec.Dashboard) (volumeWindowAlignment, bool) {
	p, err := parseParams(req.values(), false)
	if err != nil {
		h.exploreFail(c, uid, err)
		return volumeWindowAlignment{}, false
	}
	rng, _, err := p.timeRange(d, time.Now())
	if err != nil {
		h.exploreFail(c, uid, err)
		return volumeWindowAlignment{}, false
	}
	return volumeAlignment(rng, req.buckets()), true
}

func volumeAlignment(rng datasource.TimeRange, buckets int) volumeWindowAlignment {
	al := align(rng, queryproxy.KindRange, buckets)
	if rng.To.After(al.rng.To) {
		al.rng.To = al.rng.To.Add(al.step)
		al.maxDataPoints++
	}
	return volumeWindowAlignment{alignment: al, counted: rng}
}

type volumeWindowAlignment struct {
	alignment
	counted datasource.TimeRange
}

func (h *Handler) volumeFromSource(c *gin.Context, uid string, source datasource.LogVolumeSource, req volumeRequest) {
	d := exploreDashboard(uid, *req.Query, queryproxy.KindRange)
	al, ok := h.volumeWindow(c, uid, req, d)
	if !ok {
		return
	}
	releaseUpstream, ok := h.acquireUpstream(c, uid)
	if !ok {
		return
	}
	defer releaseUpstream()
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	hist, err := source.LogVolume(ctx, datasource.VolumeRequest{
		Query:   datasource.Query{Text: *req.Query, Limits: queryproxy.DraftLimits, Dashboard: d.Title, User: queryproxy.UserFor(principal)},
		Range:   al.rng,
		Counted: al.counted,
		Step:    al.step,
	})
	if err != nil {
		datasource.NoteQuery(ctx, uid, *req.Query, al.rng, 0)
		h.exploreFail(c, uid, volumeProblem(err))
		return
	}
	volume := hist.Volume()
	datasource.NoteQuery(ctx, uid, *req.Query, al.rng, len(volume.Buckets))
	c.JSON(http.StatusOK, newVolumeJSON(volume))
}

func foldClickHouseVolume(h *logvolume.Histogram, f datasource.Frame, _ time.Duration) bool {
	times, levels, counts := frameField(f, "Time"), frameField(f, "Level"), frameField(f, "Count")
	if times == nil || levels == nil || counts == nil {
		return false
	}
	for i, t := range times.Values {
		at, isTime := t.(time.Time)
		level, _ := levels.Values[i].(string)
		n, isNumber := counts.Values[i].(float64)
		if isTime && isNumber {
			h.Add(at, logvolume.Kind(level), int64(math.Round(n)))
		}
	}
	return true
}

func frameField(f datasource.Frame, name string) *datasource.Field {
	for i := range f.Fields {
		if strings.EqualFold(f.Fields[i].Name, name) {
			return &f.Fields[i]
		}
	}
	return nil
}

func newVolumeJSON(v logvolume.Volume) volumeJSON {
	out := volumeJSON{StepMS: v.StepMS, Total: v.Total, Buckets: make([]bucketJSON, len(v.Buckets)), Partial: v.Partial()}
	if v.Partial() {
		out.CountedFrom = httpapi.FormatTime(v.CountedFrom)
	}
	for i, b := range v.Buckets {
		out.Buckets[i] = bucketJSON{Start: httpapi.FormatTime(b.Start), Error: b.Error, Warning: b.Warning, Info: b.Info, Other: b.Other}
	}
	return out
}

func (r volumeRequest) validate() (status int, code, message string) {
	invalid := func(message string) (int, string, string) {
		return http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message
	}
	switch {
	case r.Query == nil || *r.Query == "":
		return invalid("query must be from 1 byte to 64 KiB")
	case len(*r.Query) > maxLintQueryBytes:
		return http.StatusBadRequest, string(queryproxy.CodeQueryTooLong), "query must be at most 64 KiB"
	case r.From == nil || r.To == nil:
		return invalid("from and to are required")
	case r.Buckets != nil && (*r.Buckets < 1 || *r.Buckets > maxVolumeBuckets):
		return invalid("buckets must be from 1 to " + strconv.Itoa(maxVolumeBuckets))
	}
	return 0, "", ""
}

func (r volumeRequest) values() url.Values {
	q := url.Values{}
	q.Set(paramFrom, *r.From)
	q.Set(paramTo, *r.To)
	return q
}

func (r volumeRequest) buckets() int {
	if r.Buckets == nil {
		return defaultVolumeBuckets
	}
	return *r.Buckets
}
