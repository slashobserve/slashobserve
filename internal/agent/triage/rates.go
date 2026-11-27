package triage

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const maxRateRows = 10

func (sw *sweep) metricRates(ctx context.Context) {
	srcs := sw.sources(datasource.SignalMetrics, "prometheus")
	if len(srcs) == 0 {
		sw.skip("metric rates", "", "no Prometheus store lists services")
		return
	}
	for _, src := range srcs[:min(len(srcs), maxMetricStores)] {
		plan, ok := sw.metricPlan(ctx, src)
		if !ok {
			continue
		}
		sw.signal("requests, errors and p95 latency per service on " + src.UID + " from " + plan.basis + " " + plan.names())
		var tasks []func(context.Context)
		for _, w := range Rungs {
			tasks = append(tasks, func(ctx context.Context) { sw.metricRung(ctx, src, plan, w) })
		}
		sw.parallel(ctx, tasks)
	}
}

func (sw *sweep) metricPlan(ctx context.Context, src Source) (metricPlan, bool) {
	var calls, durations []string
	var mu sync.Mutex
	okCalls, okDurations := false, false
	sw.parallel(ctx, []func(context.Context){
		func(ctx context.Context) {
			okCalls = sw.call(ctx, "metric names", src.UID, func(ctx context.Context) error {
				names, err := sw.r.MetricNames(ctx, src.UID, "calls_total")
				mu.Lock()
				calls = names
				mu.Unlock()
				return err
			})
		},
		func(ctx context.Context) {
			okDurations = sw.call(ctx, "metric names", src.UID, func(ctx context.Context) error {
				names, err := sw.r.MetricNames(ctx, src.UID, "duration")
				mu.Lock()
				durations = names
				mu.Unlock()
				return err
			})
		},
	})
	if !okCalls && !okDurations {
		return metricPlan{}, false
	}
	plan, ok := pickMetrics(calls, durations, sw.field(src.UID, datasource.SignalMetrics))
	if !ok {
		sw.skip("metric rates", src.UID, "no span-metric calls_total counter or HTTP server duration histogram was found")
	}
	return plan, ok
}

func (sw *sweep) metricRung(ctx context.Context, src Source, plan metricPlan, w time.Duration) {
	rng, windowStart := sw.rung(w)
	rate := rateWindow(2 * w / rungPoints)
	var req, errs, lat []Series
	reads := []struct {
		query string
		into  *[]Series
	}{{plan.requestQuery(rate), &req}, {plan.errorQuery(rate), &errs}}
	if plan.bucket != "" {
		reads = append(reads, struct {
			query string
			into  *[]Series
		}{plan.latencyQuery(rate), &lat})
	}
	step := "metric rates over " + short(w)
	var tasks []func(context.Context)
	for _, read := range reads {
		tasks = append(tasks, func(ctx context.Context) {
			sw.call(ctx, step, src.UID, func(ctx context.Context) (err error) {
				*read.into, err = sw.r.Range(ctx, src.UID, read.query, rng, rungPoints)
				return err
			})
		})
	}
	sw.parallel(ctx, tasks)
	base := Row{Rung: w, Source: src.UID, Basis: plan.basis}
	sw.addRows(metricRows(base, windowStart, byService(req, plan.label), byService(errs, plan.label), byService(lat, plan.label), plan.scale))
}

func byService(series []Series, label string) map[string][]Point {
	out := map[string][]Point{}
	for _, s := range series {
		if name := s.Labels[label]; name != "" {
			out[name] = append(out[name], s.Points...)
		}
	}
	for name := range out {
		slices.SortFunc(out[name], func(a, b Point) int { return a.At.Compare(b.At) })
	}
	return out
}

func metricRows(base Row, windowStart time.Time, req, errs, lat map[string][]Point, scale float64) []Row {
	var rows []Row
	for _, name := range slices.Sorted(maps.Keys(req)) {
		rows = append(rows, requestRow(base, name, windowStart, req[name]))
		if row, ok := errorRow(base, name, windowStart, req[name], errs[name]); ok {
			rows = append(rows, row)
		}
		if row, ok := latencyRow(base, name, windowStart, lat[name], scale); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func requestRow(base Row, name string, windowStart time.Time, points []Point) Row {
	before, during := split(points, windowStart)
	row := base
	row.Service, row.Signal, row.Unit = name, SignalRequests, "/s"
	row.Window, row.Baseline = meanOf(during), meanOf(before)
	return row
}

func errorRow(base Row, name string, windowStart time.Time, req, errs []Point) (Row, bool) {
	ratios := ratioSeries(req, errs)
	if len(ratios) == 0 {
		return Row{}, false
	}
	beforeReq, duringReq := split(req, windowStart)
	beforeErr, duringErr := split(errs, windowStart)
	row := base
	row.Service, row.Signal, row.Unit = name, SignalErrors, "ratio"
	row.Window, row.Baseline = share(duringErr, duringReq), share(beforeErr, beforeReq)
	row.Moved = sumOf(duringErr) > 0 && row.Window > ErrorRule.threshold(row.Baseline)
	if row.Moved {
		row.Onset = onsetPtr(ratios, windowStart, ErrorRule)
	}
	return row, true
}

func latencyRow(base Row, name string, windowStart time.Time, points []Point, scale float64) (Row, bool) {
	scaled := make([]Point, len(points))
	for i, p := range points {
		scaled[i] = Point{At: p.At, Value: p.Value * scale}
	}
	before, during := split(scaled, windowStart)
	if len(during) == 0 {
		return Row{}, false
	}
	row := base
	row.Service, row.Signal, row.Unit = name, SignalLatency, "ms"
	row.Window, row.Baseline = median(values(during)), median(values(before))
	row.Moved = len(before) >= minBaselineObs && row.Window > LatencyRule.threshold(row.Baseline)
	if row.Moved {
		row.Onset = onsetPtr(scaled, windowStart, LatencyRule)
	}
	return row, true
}

func onsetPtr(points []Point, windowStart time.Time, r Rule) *time.Time {
	if at, ok := Onset(points, windowStart, r); ok {
		return &at
	}
	return nil
}

func ratioSeries(req, errs []Point) []Point {
	errAt := make(map[int64]float64, len(errs))
	for _, p := range errs {
		errAt[p.At.UnixNano()] = p.Value
	}
	var out []Point
	for _, p := range req {
		if p.Value > 0 {
			out = append(out, Point{At: p.At, Value: errAt[p.At.UnixNano()] / p.Value})
		}
	}
	return out
}

func share(errs, req []Point) float64 {
	total := sumOf(req)
	if total <= 0 {
		return 0
	}
	return sumOf(errs) / total
}

func sumOf(points []Point) float64 {
	s := 0.0
	for _, p := range points {
		s += p.Value
	}
	return s
}

func meanOf(points []Point) float64 {
	if len(points) == 0 {
		return 0
	}
	return sumOf(points) / float64(len(points))
}

func topRates(rows []Row) []Row {
	var req []Row
	for _, r := range rows {
		if r.Signal == SignalRequests && r.Rung == Rungs[0] {
			req = append(req, r)
		}
	}
	slices.SortStableFunc(req, func(a, b Row) int { return cmp.Or(cmp.Compare(b.Window, a.Window), cmp.Compare(a.Service, b.Service)) })
	req = req[:min(len(req), maxRateRows)]
	var out []Row
	for _, r := range req {
		out = append(out, r)
		for _, other := range rows {
			if other.Service == r.Service && other.Source == r.Source && other.Rung == r.Rung && other.Signal != SignalRequests {
				out = append(out, other)
			}
		}
	}
	return out
}

func (sw *sweep) traceRates(ctx context.Context) {
	srcs := sw.sources(datasource.SignalTraces, "tempo", "clickhouse")
	if len(srcs) == 0 {
		sw.skip("trace rates", "", "no metric rates were read and no Tempo or ClickHouse traces store lists services")
		return
	}
	src := srcs[0]
	sw.signal("span counts, errors and p95 duration per service on " + src.UID + " from " + basisTraceSpans)
	var tasks []func(context.Context)
	for _, w := range Rungs {
		tasks = append(tasks, func(ctx context.Context) { sw.traceRung(ctx, src, w) })
	}
	sw.parallel(ctx, tasks)
	sw.traceOnsets(ctx, src)
}

func (sw *sweep) traceRung(ctx context.Context, src Source, w time.Duration) {
	var now, before TraceStats
	step := "trace rates over " + short(w)
	sw.parallel(ctx, []func(context.Context){
		func(ctx context.Context) {
			sw.call(ctx, step, src.UID, func(ctx context.Context) (err error) {
				now, err = sw.r.TraceStats(ctx, src.UID, TraceStatsInput{Query: traceAll(src.Type), GroupBy: traceServiceKey, Range: Window{From: sw.in.End.Add(-w), To: sw.in.End}, Buckets: 1})
				return err
			})
		},
		func(ctx context.Context) {
			sw.call(ctx, step, src.UID, func(ctx context.Context) (err error) {
				before, err = sw.r.TraceStats(ctx, src.UID, TraceStatsInput{Query: traceAll(src.Type), GroupBy: traceServiceKey, Range: Window{From: sw.in.End.Add(-2 * w), To: sw.in.End.Add(-w)}, Buckets: 1})
				return err
			})
		},
	})
	sw.addRows(traceRows(Row{Rung: w, Source: src.UID, Basis: basisTraceSpans}, now.Groups, before.Groups))
}

func traceRows(base Row, now, before []TraceGroup) []Row {
	prior := map[string]TraceGroup{}
	for _, g := range before {
		prior[g.Value] = g
	}
	seconds := base.Rung.Seconds()
	var rows []Row
	for _, g := range now {
		if g.Value == "" || g.Spans == 0 {
			continue
		}
		b := prior[g.Value]
		req := base
		req.Service, req.Signal, req.Unit = g.Value, SignalRequests, "spans/s"
		req.Window, req.Baseline = float64(g.Spans)/seconds, float64(b.Spans)/seconds
		errs := base
		errs.Service, errs.Signal, errs.Unit = g.Value, SignalErrors, "ratio"
		errs.Window, errs.Baseline = ratio(g.Errors, g.Spans), ratio(b.Errors, b.Spans)
		errs.Moved = g.Errors > 0 && errs.Window > ErrorRule.threshold(errs.Baseline)
		rows = append(rows, req, errs)
		if g.P95MS != nil && b.P95MS != nil {
			lat := base
			lat.Service, lat.Signal, lat.Unit = g.Value, SignalLatency, "ms"
			lat.Window, lat.Baseline = *g.P95MS, *b.P95MS
			lat.Moved = b.Spans > 0 && lat.Window > LatencyRule.threshold(lat.Baseline)
			rows = append(rows, lat)
		}
	}
	return rows
}

func ratio(n, d int64) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

func (sw *sweep) traceOnsets(ctx context.Context, src Source) {
	var tasks []func(context.Context)
	for _, m := range sw.focus() {
		w := finest(m)
		rng, windowStart := sw.rung(w)
		tasks = append(tasks, func(ctx context.Context) {
			var stats TraceStats
			if !sw.call(ctx, "trace onsets", src.UID, func(ctx context.Context) (err error) {
				stats, err = sw.r.TraceStats(ctx, src.UID, TraceStatsInput{Query: traceService(src.Type, m.service), Range: rng, Buckets: traceBuckets})
				return err
			}) {
				return
			}
			sw.setOnsets(m.service, w, bucketOnsets(stats.Buckets, windowStart))
		})
	}
	sw.parallel(ctx, tasks)
}

func bucketOnsets(buckets []TraceBucket, windowStart time.Time) map[string]*time.Time {
	var ratios, lat []Point
	for _, b := range buckets {
		if b.Spans > 0 {
			ratios = append(ratios, Point{At: b.Start, Value: ratio(b.Errors, b.Spans)})
		}
		if b.P95MS != nil {
			lat = append(lat, Point{At: b.Start, Value: *b.P95MS})
		}
	}
	return map[string]*time.Time{
		SignalErrors:  onsetPtr(ratios, windowStart, ErrorRule),
		SignalLatency: onsetPtr(lat, windowStart, LatencyRule),
	}
}

func (sw *sweep) setOnsets(service string, w time.Duration, onsets map[string]*time.Time) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	for i, r := range sw.findings.Rows {
		if r.Service == service && r.Rung == w && r.Moved {
			sw.findings.Rows[i].Onset = onsets[r.Signal]
		}
	}
}

func (sw *sweep) logRates(ctx context.Context) {
	srcs := sw.sources(datasource.SignalLogs, "loki")
	if len(srcs) == 0 {
		sw.skip("log rates", "", "no metric or trace rates were read and no Loki store lists services")
		return
	}
	src := srcs[0]
	label := sw.field(src.UID, datasource.SignalLogs)
	if !labelName.MatchString(label) {
		sw.skip("log rates", src.UID, "the service field is not a stream label")
		return
	}
	sw.signal("error-level log lines per service on " + src.UID + " from " + basisLogLevels + " in " + levelLabel)
	var tasks []func(context.Context)
	for _, w := range Rungs {
		tasks = append(tasks, func(ctx context.Context) {
			rng, windowStart := sw.rung(w)
			step := 2 * w / rungPoints
			var got []Series
			if sw.call(ctx, "log rates over "+short(w), src.UID, func(ctx context.Context) (err error) {
				got, err = sw.r.Range(ctx, src.UID, logLevelQuery(label, step), rng, rungPoints)
				return err
			}) {
				sw.addRows(logRows(Row{Rung: w, Source: src.UID, Basis: basisLogLevels}, windowStart, errorLines(got, label), step))
			}
		})
	}
	sw.parallel(ctx, tasks)
}

func errorLines(series []Series, label string) map[string][]Point {
	grid := map[int64]time.Time{}
	sums := map[string]map[int64]float64{}
	for _, srs := range series {
		name := srs.Labels[label]
		if name == "" {
			continue
		}
		if sums[name] == nil {
			sums[name] = map[int64]float64{}
		}
		isError := slices.Contains(errorLevels, strings.ToLower(srs.Labels[levelLabel]))
		for _, p := range srs.Points {
			grid[p.At.UnixNano()] = p.At
			if isError {
				sums[name][p.At.UnixNano()] += p.Value
			}
		}
	}
	steps := slices.Sorted(maps.Keys(grid))
	out := map[string][]Point{}
	for name, at := range sums {
		points := make([]Point, len(steps))
		for i, k := range steps {
			points[i] = Point{At: grid[k], Value: at[k]}
		}
		out[name] = points
	}
	return out
}

func logRows(base Row, windowStart time.Time, lines map[string][]Point, step time.Duration) []Row {
	var rows []Row
	for _, name := range slices.Sorted(maps.Keys(lines)) {
		perSecond := make([]Point, len(lines[name]))
		for i, p := range lines[name] {
			perSecond[i] = Point{At: p.At, Value: p.Value / step.Seconds()}
		}
		before, during := split(lines[name], windowStart)
		row := base
		row.Service, row.Signal, row.Unit = name, SignalErrors, "error lines/s"
		row.Window, row.Baseline = sumOf(during)/base.Rung.Seconds(), sumOf(before)/base.Rung.Seconds()
		row.Moved = row.Window > LogRule.threshold(row.Baseline)
		if row.Moved {
			row.Onset = onsetPtr(perSecond, windowStart, LogRule)
		}
		rows = append(rows, row)
	}
	return rows
}
