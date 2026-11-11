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
	"github.com/slashobserve/slashobserve/internal/tracecompare"
)

const (
	traceCompareSourcesMessage = "trace compare reads Tempo and ClickHouse datasources"
	defaultTraceCompareLimit   = 20
	maxTraceCompareLimit       = 100
)

type traceCompareRequest struct {
	Selected []string `json:"selected"`
	Baseline []string `json:"baseline"`
	From     *string  `json:"from"`
	To       *string  `json:"to"`
	Timezone *string  `json:"timezone"`
	Limit    *int     `json:"limit"`
}

type traceCompareJSON struct {
	Selected    traceSetJSON     `json:"selected"`
	Baseline    traceSetJSON     `json:"baseline"`
	Attributes  int              `json:"attributes"`
	Partial     bool             `json:"partial"`
	Differences []differenceJSON `json:"differences"`
	Truncated   bool             `json:"truncated"`
}

type traceSetJSON struct {
	Requested int `json:"requested"`
	Read      int `json:"read"`
}

type differenceJSON struct {
	Attribute     string  `json:"attribute"`
	Value         string  `json:"value"`
	SelectedShare float64 `json:"selected_share"`
	BaselineShare float64 `json:"baseline_share"`
	SelectedCount int     `json:"selected_count"`
	BaselineCount int     `json:"baseline_count"`
}

func init() {
	registerRoutes(func(h *Handler, api auth.Routes) {
		api.POST("/datasources/:uid/trace-compare", auth.RoleViewer, h.traceCompare)
	})
}

func (h *Handler) traceCompare(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	var req traceCompareRequest
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
	source, ok := inst.Datasource.(datasource.TraceAttributeSource)
	if !ok {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), traceCompareSourcesMessage)
		return
	}
	p, err := parseParams(req.values(), false)
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
	releaseUpstream, ok := h.acquireUpstream(c, uid)
	if !ok {
		return
	}
	defer releaseUpstream()
	selected, err := source.TraceAttributes(ctx, datasource.TraceAttributesRequest{IDs: req.Selected, Range: rng, MaxSpans: datasource.CompareSpanBudget})
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	baseline, err := source.TraceAttributes(ctx, datasource.TraceAttributesRequest{IDs: req.Baseline, Range: rng, MaxSpans: datasource.CompareSpanBudget})
	if err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	c.JSON(http.StatusOK, newTraceCompareJSON(len(req.Selected), selected, len(req.Baseline), baseline, tracecompare.Compare(selected, baseline, req.limit())))
}

func newTraceCompareJSON(selectedRequested int, selected []datasource.TraceAttributeSet, baselineRequested int, baseline []datasource.TraceAttributeSet, result tracecompare.Result) traceCompareJSON {
	out := traceCompareJSON{
		Selected:    traceSetJSON{Requested: selectedRequested, Read: len(selected)},
		Baseline:    traceSetJSON{Requested: baselineRequested, Read: len(baseline)},
		Attributes:  result.Attributes,
		Partial:     len(selected) < selectedRequested || len(baseline) < baselineRequested,
		Differences: make([]differenceJSON, len(result.Differences)),
		Truncated:   result.Truncated,
	}
	for i, d := range result.Differences {
		out.Differences[i] = differenceJSON{
			Attribute:     d.Attribute,
			Value:         d.Value,
			SelectedShare: d.SelectedShare,
			BaselineShare: d.BaselineShare,
			SelectedCount: d.SelectedCount,
			BaselineCount: d.BaselineCount,
		}
	}
	return out
}

func (r *traceCompareRequest) validate() (status int, code, message string) {
	invalid := func(message string) (int, string, string) {
		return http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message
	}
	switch {
	case r.From == nil || r.To == nil:
		return invalid("from and to are required")
	case r.Limit != nil && (*r.Limit < 1 || *r.Limit > maxTraceCompareLimit):
		return invalid("limit must be from 1 to " + strconv.Itoa(maxTraceCompareLimit))
	}
	selected, baseline, err := datasource.CompareTraceIDs(r.Selected, r.Baseline)
	if err != nil {
		return invalid(err.Error())
	}
	r.Selected, r.Baseline = selected, baseline
	return 0, "", ""
}

func (r traceCompareRequest) values() url.Values {
	q := url.Values{}
	q.Set(paramFrom, *r.From)
	q.Set(paramTo, *r.To)
	if r.Timezone != nil {
		q.Set(paramTimezone, *r.Timezone)
	}
	return q
}

func (r traceCompareRequest) limit() int {
	if r.Limit == nil {
		return defaultTraceCompareLimit
	}
	return *r.Limit
}
