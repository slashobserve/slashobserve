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
	maxFacetFields     = 50
	maxFacetFieldBytes = 200
	defaultFacetLimit  = 10
	maxFacetLimit      = 50
	defaultFacetSample = 1000
	minFacetSample     = 100
	maxFacetSample     = 5000
	facetsReadsTypes   = "facets read Loki and ClickHouse datasources"
)

type facetsRequest struct {
	Query    *string  `json:"query"`
	From     *string  `json:"from"`
	To       *string  `json:"to"`
	Timezone *string  `json:"timezone"`
	Fields   []string `json:"fields"`
	Limit    *int     `json:"limit"`
	Sample   *int     `json:"sample"`
}

func (h *Handler) facets(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	var req facetsRequest
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
	source, supported := inst.Datasource.(datasource.LogFacetSource)
	if !supported {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), facetsReadsTypes)
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
	releaseUpstream, ok := h.acquireUpstream(c)
	if !ok {
		return
	}
	defer releaseUpstream()
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	facets, err := source.LogFacets(ctx, datasource.FacetRequest{
		Query:  datasource.Query{Text: *req.Query, Limits: queryproxy.DraftLimits, Dashboard: d.Title, User: queryproxy.UserFor(principal)},
		Range:  rng,
		Fields: req.Fields,
		Limit:  req.limit(),
		Sample: req.sample(),
	})
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	datasource.SortFacets(&facets)
	datasource.TrimFacets(&facets, maxFacetFields, req.limit())
	if facets.Fields == nil {
		facets.Fields = []datasource.FacetField{}
	}
	c.JSON(http.StatusOK, facets)
}

func (r facetsRequest) validate() (status int, code, message string) {
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
	case r.Limit != nil && (*r.Limit < 1 || *r.Limit > maxFacetLimit):
		return invalid("limit must be from 1 to " + strconv.Itoa(maxFacetLimit))
	case r.Sample != nil && (*r.Sample < minFacetSample || *r.Sample > maxFacetSample):
		return invalid("sample must be from " + strconv.Itoa(minFacetSample) + " to " + strconv.Itoa(maxFacetSample))
	}
	if message, ok := validFacetFields(r.Fields); !ok {
		return invalid(message)
	}
	return 0, "", ""
}

func validFacetFields(fields []string) (message string, ok bool) {
	if fields != nil && (len(fields) < 1 || len(fields) > maxFacetFields) {
		return "fields must name from 1 to " + strconv.Itoa(maxFacetFields) + " fields", false
	}
	for _, name := range fields {
		if name == "" || len(name) > maxFacetFieldBytes {
			return "each field must be from 1 to " + strconv.Itoa(maxFacetFieldBytes) + " bytes", false
		}
	}
	return "", true
}

func (r facetsRequest) values() url.Values {
	q := url.Values{}
	q.Set(paramFrom, *r.From)
	q.Set(paramTo, *r.To)
	if r.Timezone != nil {
		q.Set(paramTimezone, *r.Timezone)
	}
	return q
}

func (r facetsRequest) limit() int {
	if r.Limit == nil {
		return defaultFacetLimit
	}
	return *r.Limit
}

func (r facetsRequest) sample() int {
	if r.Sample == nil {
		return defaultFacetSample
	}
	return *r.Sample
}
