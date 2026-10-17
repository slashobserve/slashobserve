package queryproxy

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var testNow = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func testRange() datasource.TimeRange {
	return datasource.TimeRange{From: testNow.Add(-6 * time.Hour), To: testNow}
}

type call struct {
	method      string
	query       datasource.Query
	rng         datasource.TimeRange
	step        time.Duration
	deadline    time.Time
	hasDeadline bool
}

type fakeDatasource struct {
	typ string

	onRange    func(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error)
	onInstant  func(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error)
	onVariable func(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]string, error)

	mu       sync.Mutex
	calls    []call
	inflight int
	peak     int
}

func newFake(typ string) *fakeDatasource { return &fakeDatasource{typ: typ} }

func (f *fakeDatasource) enter(ctx context.Context, c call) func() {
	c.deadline, c.hasDeadline = ctx.Deadline()
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.inflight++
	f.peak = max(f.peak, f.inflight)
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		f.inflight--
		f.mu.Unlock()
	}
}

func (f *fakeDatasource) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeDatasource) inflightNow() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inflight
}

func (f *fakeDatasource) peakInflight() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.peak
}

func (f *fakeDatasource) recorded() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

func (f *fakeDatasource) callsTo(method string) []call {
	var out []call
	for _, c := range f.recorded() {
		if c.method == method {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeDatasource) Type() string                                         { return f.typ }
func (f *fakeDatasource) CheckHealth(context.Context) error                    { return nil }
func (f *fakeDatasource) Lint(context.Context, string) []datasource.Diagnostic { return nil }
func (f *fakeDatasource) Close() error                                         { return nil }
func (f *fakeDatasource) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}

func (f *fakeDatasource) VariableValues(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]string, error) {
	defer f.enter(ctx, call{method: "variable", query: q, rng: r})()
	if f.onVariable != nil {
		return f.onVariable(ctx, q, r)
	}
	return []string{"v1"}, nil
}

func (f *fakeDatasource) QueryRange(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
	defer f.enter(ctx, call{method: "range", query: q, rng: r, step: step})()
	if f.onRange != nil {
		return f.onRange(ctx, q, r, step)
	}
	return dataFrames(), nil
}

func (f *fakeDatasource) QueryInstant(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	defer f.enter(ctx, call{method: "instant", query: q, rng: r})()
	if f.onInstant != nil {
		return f.onInstant(ctx, q, r)
	}
	return dataFrames(), nil
}

func dataFrames() []datasource.Frame {
	return []datasource.Frame{{Fields: []datasource.Field{
		{Name: "time", Type: datasource.FieldTime, Values: []any{1.0}},
		{Name: "value", Type: datasource.FieldNumber, Values: []any{2.0}},
	}}}
}

func failWith(err error) func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
	return func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return nil, err
	}
}

func newTestRunner(t *testing.T, logger *slog.Logger, fakes map[string]*fakeDatasource) *Runner {
	t.Helper()
	instances := make([]datasource.Instance, 0, len(fakes))
	for uid, f := range fakes {
		instances = append(instances, datasource.Instance{UID: uid, Datasource: f})
	}
	reg, err := datasource.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)), instances...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return NewRunner(reg, logger)
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func promRef(uid string) spec.DatasourceRef {
	return spec.DatasourceRef{Type: spec.Prometheus, UID: uid}
}

func panelOf(id string, typ spec.PanelType, uid, query string) spec.Panel {
	return spec.Panel{ID: id, Type: typ, Title: id, Datasource: promRef(uid), Query: query}
}

func queryVariable(name, uid, query string) spec.Variable {
	ref := promRef(uid)
	return spec.Variable{Name: name, Type: spec.QueryVariable, Datasource: &ref, Query: query}
}

func customVariable(name string, options ...string) spec.Variable {
	return spec.Variable{Name: name, Type: spec.CustomVariable, Options: options}
}

func dashboardOf(panels []spec.Panel, variables ...spec.Variable) *spec.Dashboard {
	return &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "test",
		TimeRange:   spec.TimeRange{From: "now-6h", To: "now"},
		Variables:   variables,
		Panels:      panels,
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}
