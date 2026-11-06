package proxyhttp

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/clickhouse"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
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
	StepMS  int64        `json:"step_ms"`
	Total   int64        `json:"total"`
	Buckets []bucketJSON `json:"buckets"`
}

type bucketJSON struct {
	Start   string `json:"start"`
	Error   int64  `json:"error"`
	Warning int64  `json:"warning"`
	Info    int64  `json:"info"`
	Other   int64  `json:"other"`
}

type volumeSource struct {
	kind   queryproxy.Kind
	derive func(query string, step time.Duration) (string, error)
	fold   func(h *logvolume.Histogram, f datasource.Frame, step time.Duration) bool
}

var volumeSources = map[string]volumeSource{
	string(spec.Loki):       {kind: queryproxy.KindRange, derive: loki.VolumeQuery, fold: foldLokiVolume},
	string(spec.ClickHouse): {kind: queryproxy.KindInstant, derive: clickhouse.VolumeQuery, fold: foldClickHouseVolume},
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
	inst, found := h.registry.Get(uid)
	if !found {
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "datasource not found")
		return
	}
	source, supported := volumeSources[inst.Datasource.Type()]
	if !supported {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), "volume reads Loki and ClickHouse datasources")
		return
	}
	p, err := parseParams(req.values(), false)
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	rng, _, err := p.timeRange(exploreDashboard(uid, *req.Query, source.kind), time.Now())
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	al := align(rng, queryproxy.KindRange, req.buckets())
	derived, err := source.derive(*req.Query, al.step)
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	res, err := h.runner.RunPanel(ctx, queryproxy.Request{
		Dashboard:     exploreDashboard(uid, derived, source.kind),
		PanelID:       explorePanelID,
		Range:         al.rng,
		Selections:    map[string]queryproxy.Selection{},
		MaxDataPoints: al.maxDataPoints,
		Limits:        queryproxy.DraftLimits,
		User:          queryproxy.UserFor(principal),
	})
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	hist := logvolume.New(al.rng.From, al.step, al.maxDataPoints)
	for _, q := range res.Queries {
		if source.kind == queryproxy.KindRange && q.Step != al.step {
			h.logger.ErrorContext(ctx, "log volume ran at a step other than its window", slog.String("datasource_uid", uid), slog.Duration("window", al.step), slog.Duration("step", q.Step))
			h.fail(c, errInternal)
			return
		}
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

func foldLokiVolume(h *logvolume.Histogram, f datasource.Frame, step time.Duration) bool {
	times, values := frameField(f, "Time"), frameField(f, "Value")
	if times == nil || values == nil {
		return false
	}
	kind := logvolume.KindOf(loki.VolumeLevel(f.Labels))
	for i, t := range times.Values {
		at, isTime := t.(time.Time)
		v, isNumber := values.Values[i].(float64)
		if isTime && isNumber {
			h.Add(at.Add(-step), kind, int64(math.Round(v)))
		}
	}
	return true
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
	out := volumeJSON{StepMS: v.StepMS, Total: v.Total, Buckets: make([]bucketJSON, len(v.Buckets))}
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
