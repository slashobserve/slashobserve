package triage

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	rungPoints      = 120
	eventPoints     = 144
	maxMetricStores = 2
	maxFocus        = 3
	maxFlagKeys     = 3
	maxFlagPairs    = 4
	traceBuckets    = 60
	minNewPattern   = 3
	minGrownPattern = 10
	minBaselineObs  = 3
)

type Limits struct {
	Calls       int
	Time        time.Duration
	Concurrency int
}

var DefaultLimits = Limits{Calls: 40, Time: 20 * time.Second, Concurrency: 4}

type Source struct{ UID, Type string }

type Input struct {
	Sources  []Source
	End      time.Time
	Question *Window
}

type Outcome struct {
	Block    string
	Calls    int
	Duration time.Duration
}

type Sweeper struct {
	Reader Reader
	Limits Limits
}

type sweep struct {
	r   Reader
	lim Limits
	in  Input

	mu       sync.Mutex
	calls    int
	fields   map[string]string
	services []ServiceSource
	signals  []string
	skipped  []Skip
	findings Findings
	rates    []Row
}

func (s Sweeper) Sweep(ctx context.Context, in Input) Outcome {
	start := time.Now()
	lim := s.Limits
	if lim.Calls <= 0 || lim.Time <= 0 || lim.Concurrency <= 0 {
		lim = DefaultLimits
	}
	ctx, cancel := context.WithTimeout(ctx, lim.Time)
	defer cancel()
	sw := &sweep{r: s.Reader, lim: lim, in: in, fields: map[string]string{}}
	sw.serviceMap(ctx)
	sw.measure(ctx)
	sw.context(ctx)
	report := sw.report(time.Since(start))
	return Outcome{Block: Render(report, MaxBlockBytes), Calls: report.Calls, Duration: report.Duration}
}

func (sw *sweep) report(took time.Duration) Report {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	candidates, background := Cluster(sw.findings)
	slices.SortFunc(sw.services, func(a, b ServiceSource) int {
		return cmp.Or(cmp.Compare(a.Source, b.Source), cmp.Compare(a.Signal, b.Signal))
	})
	slices.Sort(sw.signals)
	slices.SortFunc(sw.skipped, func(a, b Skip) int {
		return cmp.Or(cmp.Compare(a.Step, b.Step), cmp.Compare(a.Source, b.Source), cmp.Compare(a.Reason, b.Reason))
	})
	return Report{
		End: sw.in.End, Question: sw.in.Question, Services: sw.services, Signals: sw.signals, Rates: topRates(sw.rates),
		Candidates: candidates, Background: background, Skipped: sw.skipped, Calls: sw.calls, Duration: took,
	}
}

func (sw *sweep) take() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.calls >= sw.lim.Calls {
		return false
	}
	sw.calls++
	return true
}

func (sw *sweep) call(ctx context.Context, step, uid string, read func(context.Context) error) bool {
	if ctx.Err() != nil {
		sw.skip(step, uid, "the sweep's time limit was reached first")
		return false
	}
	if !sw.take() {
		sw.skip(step, uid, "the sweep's call budget was spent first")
		return false
	}
	err := read(ctx)
	switch {
	case err == nil:
		return true
	case ctx.Err() != nil && errors.Is(err, context.DeadlineExceeded):
		sw.skip(step, uid, "the sweep's time limit ran out during the read")
	default:
		sw.skip(step, uid, "the read failed: "+clipText(err.Error()))
	}
	return false
}

func (sw *sweep) skip(step, uid, reason string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.skipped = append(sw.skipped, Skip{Step: step, Source: uid, Reason: reason})
}

func (sw *sweep) signal(text string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.signals = append(sw.signals, text)
}

func (sw *sweep) parallel(ctx context.Context, tasks []func(context.Context)) {
	var group errgroup.Group
	group.SetLimit(sw.lim.Concurrency)
	for _, task := range tasks {
		group.Go(func() error {
			task(ctx)
			return nil
		})
	}
	_ = group.Wait()
}

func (sw *sweep) serviceMap(ctx context.Context) {
	var tasks []func(context.Context)
	for _, src := range sw.in.Sources {
		for _, signal := range datasource.ServiceSignals(src.Type) {
			tasks = append(tasks, func(ctx context.Context) {
				var field string
				var names []string
				if !sw.call(ctx, "service map", src.UID, func(ctx context.Context) (err error) {
					field, names, err = sw.r.Services(ctx, src.UID, signal)
					return err
				}) {
					return
				}
				sw.mu.Lock()
				defer sw.mu.Unlock()
				sw.fields[src.UID+"/"+signal] = field
				sw.services = append(sw.services, ServiceSource{Source: src.UID, Type: src.Type, Signal: signal, Field: field, Services: slices.Sorted(slices.Values(names))})
			})
		}
	}
	sw.parallel(ctx, tasks)
}

func (sw *sweep) field(uid, signal string) string {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return cmp.Or(sw.fields[uid+"/"+signal], defaultLabel)
}

func (sw *sweep) holds(src Source, signal string) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return slices.ContainsFunc(sw.services, func(s ServiceSource) bool {
		return s.Source == src.UID && s.Signal == signal && len(s.Services) > 0
	})
}

func (sw *sweep) sources(signal string, types ...string) []Source {
	var out []Source
	for _, t := range types {
		for _, src := range sw.in.Sources {
			if src.Type == t && sw.holds(src, signal) {
				out = append(out, src)
			}
		}
	}
	return out
}

func (sw *sweep) addRows(rows []Row) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	for _, r := range rows {
		if r.Signal == SignalRequests {
			sw.rates = append(sw.rates, r)
			continue
		}
		sw.findings.Rows = append(sw.findings.Rows, r)
		if r.Rung == Rungs[0] {
			sw.rates = append(sw.rates, r)
		}
	}
}

func (sw *sweep) hasRows() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return len(sw.findings.Rows) > 0
}

func (sw *sweep) rung(w time.Duration) (Window, time.Time) {
	return Window{From: sw.in.End.Add(-2 * w), To: sw.in.End}, sw.in.End.Add(-w)
}

func (sw *sweep) measure(ctx context.Context) {
	sw.metricRates(ctx)
	if !sw.hasRows() {
		sw.traceRates(ctx)
	}
	if !sw.hasRows() {
		sw.logRates(ctx)
	}
}

func (sw *sweep) focus() []move {
	sw.mu.Lock()
	moves := movesOf(sw.findings.Rows)
	sw.mu.Unlock()
	return moves[:min(len(moves), maxFocus)]
}

func finest(m move) time.Duration {
	w := Rungs[len(Rungs)-1]
	for _, r := range m.rows {
		w = min(w, r.Rung)
	}
	return w
}

func (sw *sweep) context(ctx context.Context) {
	focus := sw.focus()
	tasks := []func(context.Context){
		func(ctx context.Context) { sw.origins(ctx, focus) },
		sw.annotations,
		sw.deployLines,
		sw.logFlags,
		func(ctx context.Context) { sw.patterns(ctx, focus) },
		sw.traceFlags,
	}
	sw.parallel(ctx, tasks)
}

func (sw *sweep) origins(ctx context.Context, focus []move) {
	srcs := sw.sources(datasource.SignalTraces, "tempo", "clickhouse", "jaeger")
	if len(srcs) == 0 {
		sw.skip("error attribution", "", "no traces store lists services")
		return
	}
	w := Rungs[0]
	if len(focus) > 0 {
		w = finest(focus[0])
	}
	src := srcs[0]
	var origins []Origin
	if sw.call(ctx, "error attribution", src.UID, func(ctx context.Context) (err error) {
		origins, err = sw.r.ErrorOrigins(ctx, src.UID, Window{From: sw.in.End.Add(-w), To: sw.in.End})
		return err
	}) {
		sw.signal("errors counted at the deepest erroring span on " + src.UID + " over the last " + short(w))
		sw.mu.Lock()
		sw.findings.Origins = append(sw.findings.Origins, origins...)
		sw.mu.Unlock()
	}
}

func (sw *sweep) annotations(ctx context.Context) {
	rng := Window{From: sw.in.End.Add(-2 * Rungs[len(Rungs)-1]), To: sw.in.End}
	var events []Event
	if sw.call(ctx, "annotations", "", func(ctx context.Context) (err error) {
		events, err = sw.r.Annotations(ctx, rng)
		return err
	}) {
		sw.addEvents(events)
	}
}

func (sw *sweep) addEvents(events []Event) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.findings.Events = append(sw.findings.Events, events...)
}

func (sw *sweep) eventRange() (Window, time.Duration) {
	w := Rungs[len(Rungs)-1]
	return Window{From: sw.in.End.Add(-w), To: sw.in.End}, w / eventPoints
}

func (sw *sweep) lokiSource(step string) (Source, bool) {
	srcs := sw.sources(datasource.SignalLogs, "loki")
	if len(srcs) == 0 {
		sw.skip(step, "", "no Loki store lists services, and this step reads Loki only")
		return Source{}, false
	}
	return srcs[0], true
}

func (sw *sweep) deployLines(ctx context.Context) {
	src, ok := sw.lokiSource("deploy, rollout, version and restart lines")
	if !ok {
		return
	}
	label := sw.field(src.UID, datasource.SignalLogs)
	rng, step := sw.eventRange()
	var got []Series
	if sw.call(ctx, "deploy, rollout, version and restart lines", src.UID, func(ctx context.Context) (err error) {
		got, err = sw.r.Range(ctx, src.UID, deployLineQuery(label, step), rng, eventPoints)
		return err
	}) {
		sw.signal("deploy, rollout, version and restart log lines at every level on " + src.UID)
		sw.addEvents(burstStarts(got, label, src.UID))
	}
}

func (sw *sweep) logFlags(ctx context.Context) {
	src, ok := sw.lokiSource("feature flag evaluations in logs")
	if !ok {
		return
	}
	label := sw.field(src.UID, datasource.SignalLogs)
	rng, step := sw.eventRange()
	var got []Series
	if sw.call(ctx, "feature flag evaluations in logs", src.UID, func(ctx context.Context) (err error) {
		got, err = sw.r.Range(ctx, src.UID, logFlagQuery(label, step), rng, eventPoints)
		return err
	}) {
		events := flagFirstSeen(got, src.UID, rng.From)
		if len(events) > 0 {
			sw.signal("feature flag evaluations in log records on " + src.UID)
		}
		sw.addEvents(events)
	}
}
