package proxyhttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxExploreBodyBytes = maxLintQueryBytes + 4<<10
	explorePanelID      = "explore"
	exploreQueryRef     = "A"
)

type exploreRequest struct {
	Query         *string `json:"query"`
	Kind          string  `json:"kind"`
	From          *string `json:"from"`
	To            *string `json:"to"`
	Timezone      *string `json:"timezone"`
	MaxDataPoints *int    `json:"max_data_points"`
	Limit         *int    `json:"limit"`
	PageSize      *int    `json:"page_size"`
	Page          *int    `json:"page"`
	Cursor        *string `json:"cursor"`
}

func init() {
	registerRoutes(func(h *Handler, api auth.Routes) {
		api.POST("/datasources/:uid/query", auth.RoleViewer, h.explore)
	})
}

func (h *Handler) explore(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	var req exploreRequest
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
	kind := queryproxy.Kind(req.Kind)
	d := exploreDashboard(uid, *req.Query, kind)
	p, err := parseParams(req.values(), true)
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	rng, _, err := p.timeRange(d, time.Now())
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	if req.PageSize != nil {
		h.explorePage(c, inst, rng, req)
		return
	}
	al := alignment{rng: rng}
	if kind == queryproxy.KindRange {
		al = align(rng, kind, p.maxDataPoints)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	entry, err := h.render(ctx, queryproxy.Request{
		Dashboard:     d,
		PanelID:       explorePanelID,
		Range:         al.rng,
		Selections:    map[string]queryproxy.Selection{},
		MaxDataPoints: al.maxDataPoints,
		Limits:        req.limits(),
		User:          queryproxy.UserFor(principal),
	})
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	writeData(c, entry, false, entry.executedAt, "")
}

func exploreDashboard(uid, text string, kind queryproxy.Kind) *spec.Dashboard {
	panelType := spec.Timeseries
	if kind == queryproxy.KindInstant {
		panelType = spec.Table
	}
	return &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "Explore",
		Timezone:    spec.TimezoneBrowser,
		Time:        spec.TimeRange{From: "now-1h", To: "now"},
		Panels: []spec.Panel{{
			ID:         explorePanelID,
			Title:      "Explore",
			Type:       panelType,
			Datasource: uid,
			GridPos:    spec.GridPos{W: 24, H: 8},
			Queries:    []spec.Query{{Ref: exploreQueryRef, Query: text, Instant: kind == queryproxy.KindInstant}},
		}},
	}
}

func (r exploreRequest) validate() (status int, code, message string) {
	invalid := func(message string) (int, string, string) {
		return http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message
	}
	switch {
	case r.Query == nil || *r.Query == "":
		return invalid("query must be from 1 byte to 64 KiB")
	case len(*r.Query) > maxLintQueryBytes:
		return http.StatusBadRequest, string(queryproxy.CodeQueryTooLong), "query must be at most 64 KiB"
	case r.Kind != string(queryproxy.KindRange) && r.Kind != string(queryproxy.KindInstant):
		return invalid("kind must be range or instant")
	case r.From == nil || r.To == nil:
		return invalid("from and to are required")
	case r.Limit != nil && (*r.Limit < 1 || *r.Limit > queryproxy.DraftLimits.MaxRows):
		return invalid("limit must be from 1 to " + strconv.Itoa(queryproxy.DraftLimits.MaxRows))
	case r.MaxDataPoints != nil && r.Kind == string(queryproxy.KindInstant):
		return invalid("max_data_points applies to range queries only")
	}
	if message := r.pagingRule(); message != "" {
		return invalid(message)
	}
	return 0, "", ""
}

func (r exploreRequest) pagingRule() string {
	switch {
	case r.PageSize == nil && (r.Page != nil || r.Cursor != nil):
		return queryproxy.PageNeedsSize
	case r.PageSize == nil:
		return ""
	case r.Kind == string(queryproxy.KindRange):
		return queryproxy.PagingIsInstant
	case r.Limit != nil:
		return queryproxy.LimitWithPage
	case *r.PageSize < datasource.MinPageSize || *r.PageSize > datasource.MaxPageSize:
		return queryproxy.PageSizeOutside
	case r.Page != nil && *r.Page < 1:
		return queryproxy.PageBelowOne
	case r.Cursor != nil && len(*r.Cursor) > datasource.MaxCursorBytes:
		return queryproxy.CursorTooLong
	}
	return ""
}

func (r exploreRequest) pageParams(uid string, rng datasource.TimeRange, user datasource.QueryUser) queryproxy.PageParams {
	p := queryproxy.PageParams{
		UID:      uid,
		Query:    datasource.Query{Text: *r.Query, Limits: queryproxy.DraftLimits, User: user},
		Range:    rng,
		Page:     1,
		PageSize: *r.PageSize,
	}
	if r.Page != nil {
		p.Page = *r.Page
	}
	if r.Cursor != nil {
		p.Cursor = *r.Cursor
	}
	return p
}

func (h *Handler) explorePage(c *gin.Context, inst datasource.Instance, rng datasource.TimeRange, req exploreRequest) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	started := time.Now()
	res, err := h.runner.RunPage(ctx, inst, req.pageParams(inst.UID, rng, queryproxy.UserFor(principal)))
	if err != nil {
		h.exploreFail(c, inst.UID, err)
		return
	}
	frames, err := marshalFrames([]queryproxy.QueryResult{{Ref: exploreQueryRef, Kind: queryproxy.KindInstant, Frames: res.Frames}})
	if err != nil {
		h.logger.ErrorContext(ctx, "convert page frames", append([]any{slog.String("datasource_uid", inst.UID)}, failureAttrs(err)...)...)
		h.fail(c, errInternal)
		return
	}
	if len(frames) > maxResponseBytes {
		h.fail(c, errTooLarge)
		return
	}
	meta := resultMetaOf(res.Frames).took(time.Since(started))
	meta.truncated, meta.rowsTotal = false, 0
	meta.page = pageMetaOf(res)
	entry := dataEntry{frames: frames, queries: []queryMeta{{Ref: exploreQueryRef, Kind: queryproxy.KindInstant}}, kind: queryproxy.KindInstant, from: res.Range.From, to: res.Range.To, meta: meta, executedAt: time.Now()}
	writeData(c, entry, false, entry.executedAt, "")
}

func pageMetaOf(res queryproxy.PageResult) *pageMeta {
	m := &pageMeta{
		Page:          res.Info.Page,
		PageSize:      res.Info.PageSize,
		TotalRelation: string(res.Info.TotalRelation),
		HasNext:       res.Info.HasNext,
		HasPrev:       res.HasPrev,
		Next:          res.Next,
		Prev:          res.Prev,
		Window:        res.Info.Window,
	}
	if res.Info.TotalRelation != datasource.TotalUnknown {
		total := res.Info.Total
		m.Total = &total
	}
	return m
}

func (r exploreRequest) values() url.Values {
	q := url.Values{}
	q.Set(paramFrom, *r.From)
	q.Set(paramTo, *r.To)
	if r.Timezone != nil {
		q.Set(paramTimezone, *r.Timezone)
	}
	if r.MaxDataPoints != nil {
		q.Set(paramMaxDataPoints, strconv.Itoa(*r.MaxDataPoints))
	}
	return q
}

func (r exploreRequest) limits() datasource.Limits {
	limits := queryproxy.DraftLimits
	if r.Limit != nil {
		limits.MaxRows = *r.Limit
	}
	return limits
}

func (h *Handler) exploreFail(c *gin.Context, uid string, err error) {
	if status, code, _ := queryproxy.Classify(err); status > http.StatusInternalServerError {
		h.logger.ErrorContext(c.Request.Context(), "explore query failed", logattrs.Failure(code, err, slog.String("datasource_uid", uid))...)
	}
	var qe *queryproxy.QueryError
	if errors.As(err, &qe) {
		err = qe.Err
	}
	h.fail(c, err)
}
