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
)

type exploreRequest struct {
	Query         *string `json:"query"`
	Kind          string  `json:"kind"`
	From          *string `json:"from"`
	To            *string `json:"to"`
	Timezone      *string `json:"timezone"`
	MaxDataPoints *int    `json:"max_data_points"`
	Limit         *int    `json:"limit"`
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
	if _, found := h.registry.Get(uid); !found {
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
			Queries:    []spec.Query{{Ref: "A", Query: text, Instant: kind == queryproxy.KindInstant}},
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
	return 0, "", ""
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
