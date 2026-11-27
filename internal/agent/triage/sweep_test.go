package triage

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

var end = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

type fakeStore struct {
	typ      string
	services []string
	failWith error
	rangeFn  func(q string, r datasource.TimeRange, step time.Duration) []datasource.Frame
	metrics  []string

	mu    sync.Mutex
	calls int
}

func (f *fakeStore) touch() error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.failWith
}

func (f *fakeStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeStore) Type() string                      { return f.typ }
func (f *fakeStore) CheckHealth(context.Context) error { return nil }
func (f *fakeStore) Lint(context.Context, string) ([]datasource.Diagnostic, error) {
	return nil, nil
}
func (f *fakeStore) Close() error { return nil }
func (f *fakeStore) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return nil, nil
}

func (f *fakeStore) Discover(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	if err := f.touch(); err != nil {
		return nil, err
	}
	cat := &datasource.Catalog{}
	for _, m := range f.metrics {
		if strings.Contains(m, req.Search) {
			cat.Metrics = append(cat.Metrics, datasource.Metric{Name: m})
		}
	}
	return cat, nil
}

func (f *fakeStore) Services(context.Context, datasource.ServiceRequest) (datasource.ServiceList, error) {
	if err := f.touch(); err != nil {
		return datasource.ServiceList{}, err
	}
	return datasource.ServiceList{Field: "service_name", Values: f.services}, nil
}

func (f *fakeStore) QueryRange(_ context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
	if err := f.touch(); err != nil {
		return nil, err
	}
	if f.rangeFn == nil {
		return nil, nil
	}
	return f.rangeFn(q.Text, r, step), nil
}

func (f *fakeStore) QueryInstant(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
	return nil, f.touch()
}

func frame(service string, r datasource.TimeRange, step time.Duration, value func(time.Time) float64) datasource.Frame {
	var times, nums []any
	for t := r.From; !t.After(r.To); t = t.Add(step) {
		times = append(times, t)
		nums = append(nums, value(t))
	}
	return datasource.Frame{Labels: map[string]string{"service_name": service}, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime, Values: times},
		{Name: "Value", Type: datasource.FieldNumber, Values: nums},
	}}
}

var faultAt = end.Add(-20 * time.Minute)

func spanMetrics(q string, r datasource.TimeRange, step time.Duration) []datasource.Frame {
	var out []datasource.Frame
	for _, svc := range []string{"cart", "payment"} {
		out = append(out, frame(svc, r, step, func(t time.Time) float64 {
			switch {
			case strings.Contains(q, "histogram_quantile"):
				return 40
			case strings.Contains(q, "STATUS_CODE_ERROR") && svc == "payment" && !t.Before(faultAt):
				return 3
			case strings.Contains(q, "STATUS_CODE_ERROR") && svc == "cart":
				return 0.2
			case strings.Contains(q, "STATUS_CODE_ERROR"):
				return 0
			}
			return 10
		}))
	}
	return out
}

type rig struct {
	tools *tools.Registry
	reg   *datasource.Registry
	prom  *fakeStore
	loki  *fakeStore
	audit *fakeStore
}

func newRig(t *testing.T) rig {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	r := rig{
		prom:  &fakeStore{typ: "prometheus", services: []string{"cart", "payment"}, metrics: []string{"traces_span_metrics_calls_total", "traces_span_metrics_duration_milliseconds_bucket"}, rangeFn: spanMetrics},
		loki:  &fakeStore{typ: "loki", failWith: datasource.Errorf(datasource.ErrUnavailable, "connection refused")},
		audit: &fakeStore{typ: "elasticsearch", services: []string{"slashobserve"}},
	}
	reg, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: "prom", Name: "Prom", Datasource: r.prom},
		datasource.Instance{UID: "loki", Name: "Loki", Datasource: r.loki},
		datasource.Instance{UID: auditsource.UID, Name: "Audit", Datasource: r.audit, Audience: datasource.AudienceAdmins},
	)
	if err != nil {
		t.Fatal(err)
	}
	r.reg = reg
	r.tools = tools.New(tools.Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger})
	return r
}

func runCtx() (context.Context, tools.Run) {
	admin := auth.Principal{Kind: auth.KindUser, Role: auth.RoleAdmin}
	run := tools.Run{Mode: tools.ModeInvestigate, Investigation: true, AsOf: end, Access: datasource.Access{By: admin, Audience: datasource.AudienceEveryone}}
	return auth.WithPrincipal(context.Background(), admin), run
}

func TestSweepThroughTheToolRegistry(t *testing.T) {
	r := newRig(t)
	ctx, run := runCtx()
	out := Service{Tools: r.tools, Datasources: r.reg, Limits: DefaultLimits}.Sweep(ctx, run)
	for _, want := range []string{
		"Windows: 1h, 6h, 24h, each ending 2026-11-20T12:00:00Z",
		"Services on prom (prometheus metrics, in service_name): cart, payment",
		"traces_span_metrics_calls_total and traces_span_metrics_duration_milliseconds_bucket",
		"c1 payment: errors",
		"onset 2026-11-20T11:4",
		"Background, not a candidate: cart errors 2%",
		"Skipped: service map on loki: the read failed",
	} {
		if !strings.Contains(out.Block, want) {
			t.Errorf("block lacks %q:\n%s", want, out.Block)
		}
	}
	if strings.Contains(out.Block, "c2 ") {
		t.Errorf("one fault gave more than one candidate:\n%s", out.Block)
	}
	if n := r.audit.count(); n != 0 {
		t.Errorf("the audit datasource was read %d times", n)
	}
	if strings.Contains(out.Block, auditsource.UID) {
		t.Errorf("the block names the audit datasource:\n%s", out.Block)
	}
	if out.Calls == 0 || out.Calls > DefaultLimits.Calls {
		t.Errorf("calls = %d", out.Calls)
	}
}

type slowReader struct {
	calls   atomic.Int64
	delay   time.Duration
	failing string
}

func (s *slowReader) wait(ctx context.Context) error {
	s.calls.Add(1)
	select {
	case <-time.After(s.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *slowReader) Services(ctx context.Context, uid, _ string) (string, []string, error) {
	if uid == s.failing {
		s.calls.Add(1)
		return "", nil, errors.New("boom")
	}
	return "service_name", []string{"a", "b"}, s.wait(ctx)
}

func (s *slowReader) MetricNames(ctx context.Context, _, search string) ([]string, error) {
	return []string{"traces_span_metrics_calls_total", "traces_span_metrics_duration_seconds_bucket"}, s.wait(ctx)
}

func (s *slowReader) Range(ctx context.Context, _, _ string, _ Window, _ int) ([]Series, error) {
	return nil, s.wait(ctx)
}

func (s *slowReader) TraceStats(ctx context.Context, _ string, _ TraceStatsInput) (TraceStats, error) {
	return TraceStats{Groups: []TraceGroup{{Value: "k", Spans: 1}}}, s.wait(ctx)
}

func (s *slowReader) ErrorOrigins(ctx context.Context, _ string, _ Window) ([]Origin, error) {
	return nil, s.wait(ctx)
}

func (s *slowReader) Patterns(ctx context.Context, _, _ string, _ Window) ([]PatternTrend, error) {
	return nil, s.wait(ctx)
}

func (s *slowReader) Annotations(ctx context.Context, _ Window) ([]Event, error) {
	return nil, s.wait(ctx)
}

func manySources() []Source {
	var out []Source
	for _, t := range []string{"prometheus", "loki", "tempo", "clickhouse", "jaeger", "elasticsearch", "opensearch"} {
		for _, n := range []string{"1", "2", "3"} {
			out = append(out, Source{UID: t + n, Type: t})
		}
	}
	return out
}

func TestSweepKeepsToItsCallBudget(t *testing.T) {
	reader := &slowReader{}
	lim := Limits{Calls: 9, Time: 5 * time.Second, Concurrency: 3}
	out := Sweeper{Reader: reader, Limits: lim}.Sweep(context.Background(), Input{Sources: manySources(), End: end})
	if got := reader.calls.Load(); got > int64(lim.Calls) {
		t.Fatalf("the reader was called %d times over a budget of %d", got, lim.Calls)
	}
	if out.Calls != lim.Calls {
		t.Errorf("outcome calls = %d, want %d", out.Calls, lim.Calls)
	}
	if !strings.Contains(out.Block, "call budget was spent first") {
		t.Errorf("the block does not say the budget ran out:\n%s", out.Block)
	}
}

func TestSweepKeepsToItsTimeLimit(t *testing.T) {
	reader := &slowReader{delay: time.Hour}
	lim := Limits{Calls: 40, Time: 200 * time.Millisecond, Concurrency: 4}
	start := time.Now()
	out := Sweeper{Reader: reader, Limits: lim}.Sweep(context.Background(), Input{Sources: manySources(), End: end})
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the sweep took %s over a %s limit", took, lim.Time)
	}
	if !strings.Contains(out.Block, "time limit") {
		t.Errorf("the block does not say the time limit stopped it:\n%s", out.Block)
	}
	if !strings.HasSuffix(out.Block, "</triage>") {
		t.Errorf("block is not closed:\n%s", out.Block)
	}
}

func TestSweepSkipsAFailingStoreAndContinues(t *testing.T) {
	reader := &slowReader{failing: "loki1"}
	out := Sweeper{Reader: reader, Limits: DefaultLimits}.Sweep(context.Background(), Input{Sources: []Source{{"loki1", "loki"}, {"prometheus1", "prometheus"}}, End: end})
	for _, want := range []string{"Skipped: service map on loki1: the read failed: boom.", "Services on prometheus1"} {
		if !strings.Contains(out.Block, want) {
			t.Errorf("block lacks %q:\n%s", want, out.Block)
		}
	}
}

func TestSourcesOfLeavesOutTheAuditAndAdminOnlyStores(t *testing.T) {
	got := SourcesOf([]datasource.Instance{
		{UID: "prom", Datasource: &fakeStore{typ: "prometheus"}},
		{UID: auditsource.UID, Datasource: &fakeStore{typ: "elasticsearch"}},
		{UID: "secret-logs", Datasource: &fakeStore{typ: "loki"}, Audience: datasource.AudienceAdmins},
		{UID: "audit-like", Datasource: &fakeStore{typ: "audit"}},
	})
	if len(got) != 1 || got[0].UID != "prom" {
		t.Fatalf("sources = %+v", got)
	}
}
