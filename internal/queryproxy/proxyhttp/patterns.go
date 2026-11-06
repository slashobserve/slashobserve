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

const (
	defaultPatternLimit  = 20
	maxPatternLimit      = 100
	defaultPatternSample = 2000
	minPatternSample     = 100
	maxPatternSample     = 5000
	patternsReadTypes    = "patterns read Loki and ClickHouse datasources"
)

type patternsRequest struct {
	Query    *string `json:"query"`
	From     *string `json:"from"`
	To       *string `json:"to"`
	Timezone *string `json:"timezone"`
	Limit    *int    `json:"limit"`
	Sample   *int    `json:"sample"`
}

func init() {
	registerRoutes(func(h *Handler, api auth.Routes) {
		api.POST("/datasources/:uid/log-patterns", auth.RoleViewer, h.patterns)
	})
}

func (h *Handler) patterns(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	var req patternsRequest
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
	if _, supported := inst.Datasource.(datasource.LineSource); !supported {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), patternsReadTypes)
		return
	}
	p, err := parseParams(req.values(), false)
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	d := exploreDashboard(uid, *req.Query, queryproxy.KindInstant)
	rng, _, err := p.timeRange(d, time.Now())
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	out, err := h.runner.LogPatterns(ctx, inst, queryproxy.PatternsRequest{
		Query:  datasource.Query{Text: *req.Query, Limits: queryproxy.DraftLimits, Dashboard: d.Title, User: queryproxy.UserFor(principal)},
		Range:  rng,
		Limit:  req.limit(),
		Sample: req.sample(),
	})
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (r patternsRequest) validate() (status int, code, message string) {
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
	case r.Limit != nil && (*r.Limit < 1 || *r.Limit > maxPatternLimit):
		return invalid("limit must be from 1 to " + strconv.Itoa(maxPatternLimit))
	case r.Sample != nil && (*r.Sample < minPatternSample || *r.Sample > maxPatternSample):
		return invalid("sample must be from " + strconv.Itoa(minPatternSample) + " to " + strconv.Itoa(maxPatternSample))
	}
	return 0, "", ""
}

func (r patternsRequest) values() url.Values {
	q := url.Values{}
	q.Set(paramFrom, *r.From)
	q.Set(paramTo, *r.To)
	if r.Timezone != nil {
		q.Set(paramTimezone, *r.Timezone)
	}
	return q
}

func (r patternsRequest) limit() int {
	if r.Limit == nil {
		return defaultPatternLimit
	}
	return *r.Limit
}

func (r patternsRequest) sample() int {
	if r.Sample == nil {
		return defaultPatternSample
	}
	return *r.Sample
}
