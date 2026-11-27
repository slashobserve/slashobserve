package tools

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	traceSampleLimit     = 200
	traceReadConcurrency = 4
	traceReadMargin      = time.Hour
	maxSampledRoutes     = 10
	traceAnalyticsText   = "reads Tempo, Jaeger and ClickHouse datasources"

	propTraceService   = `"service":{"type":"string","maxLength":256,"description":"Only traces with a span from this service. Required on Jaeger."}`
	propTraceRoute     = `"route":{"type":"string","maxLength":512,"description":"Only traces whose root span has this name, such as GET /api/cart."}`
	propTraceOperation = `"operation":{"type":"string","maxLength":512,"description":"Only traces with a span of this name."}`
)

type sampledTrace struct {
	summary   datasource.TraceSummary
	spans     []traceSpan
	truncated bool
}

type traceSampler interface {
	datasource.TraceSource
	datasource.TraceListSource
}

func traceFilterOf(service, route, operation string) datasource.TraceFilter {
	return datasource.TraceFilter{Service: strings.TrimSpace(service), Route: strings.TrimSpace(route), Operation: strings.TrimSpace(operation)}
}

func filterText(f datasource.TraceFilter, extra ...string) string {
	parts := extra
	for _, p := range [][2]string{{"service", f.Service}, {"route", f.Route}, {"operation", f.Operation}} {
		if p[1] != "" {
			parts = append(parts, p[0]+"="+p[1])
		}
	}
	return strings.Join(parts, " ")
}

// pickSample spreads the picks evenly over the newest-first listing, and with byRoute takes them from each route in turn, so a rare route is read too.
func pickSample(traces []datasource.TraceSummary, limit int, byRoute bool) []datasource.TraceSummary {
	if len(traces) <= limit {
		return traces
	}
	if !byRoute {
		return spread(traces, limit)
	}
	routes := routesOf(traces)
	quota := make(map[string]int, len(routes))
	for left := limit; left > 0; {
		gave := false
		for _, r := range routes {
			if left > 0 && quota[r.name] < len(r.traces) {
				quota[r.name]++
				left--
				gave = true
			}
		}
		if !gave {
			break
		}
	}
	var picks []datasource.TraceSummary
	for _, r := range routes {
		picks = append(picks, spread(r.traces, quota[r.name])...)
	}
	return picks
}

func spread(traces []datasource.TraceSummary, n int) []datasource.TraceSummary {
	if n >= len(traces) {
		return traces
	}
	picks := make([]datasource.TraceSummary, 0, n)
	for i := range n {
		picks = append(picks, traces[i*len(traces)/n])
	}
	return picks
}

type routeTraces struct {
	name   string
	traces []datasource.TraceSummary
}

func routesOf(traces []datasource.TraceSummary) []routeTraces {
	at := map[string]int{}
	var routes []routeTraces
	for _, t := range traces {
		i, ok := at[t.Route]
		if !ok {
			i = len(routes)
			at[t.Route] = i
			routes = append(routes, routeTraces{name: t.Route})
		}
		routes[i].traces = append(routes[i].traces, t)
	}
	slices.SortStableFunc(routes, func(a, b routeTraces) int {
		return cmp.Or(cmp.Compare(len(b.traces), len(a.traces)), cmp.Compare(a.name, b.name))
	})
	return routes
}

func (r *Registry) readSample(ctx context.Context, uid string, source datasource.TraceSource, picks []datasource.TraceSummary, rng datasource.TimeRange) ([]sampledTrace, bool, error) {
	readCtx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout)
	defer cancel()
	wide := datasource.TimeRange{From: rng.From.Add(-traceReadMargin), To: minTime(rng.To.Add(traceReadMargin), time.Now())}
	found := make([]*sampledTrace, len(picks))
	group, groupCtx := errgroup.WithContext(readCtx)
	group.SetLimit(traceReadConcurrency)
	for i, pick := range picks {
		if groupCtx.Err() != nil {
			break
		}
		group.Go(func() error {
			f, err := source.Trace(groupCtx, pick.ID, wide, datasource.TraceSpanBudget)
			switch {
			case errors.Is(err, datasource.ErrNotFound):
				return nil
			case err != nil:
				return err
			}
			found[i] = &sampledTrace{summary: pick, spans: spansOf(f), truncated: f.Meta != nil && f.Meta.Truncated}
			return nil
		})
	}
	err := group.Wait()
	read := make([]sampledTrace, 0, len(found))
	ids := make([]string, 0, len(found))
	for _, t := range found {
		if t != nil {
			read = append(read, *t)
			ids = append(ids, t.summary.ID)
		}
	}
	datasource.NoteQuery(ctx, uid, "traces "+strings.Join(ids, ","), wide, len(read))
	cut := len(read) < len(picks)
	if err != nil && (readCtx.Err() == nil || len(read) == 0) {
		return nil, false, err
	}
	return read, cut, nil
}
