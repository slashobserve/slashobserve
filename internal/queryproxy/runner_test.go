package queryproxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logtest"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestQueryKind(t *testing.T) {
	tests := []struct {
		panelType spec.PanelType
		instant   bool
		want      Kind
	}{
		{spec.Timeseries, false, KindRange},
		{spec.Heatmap, false, KindRange},
		{spec.StateTimeline, false, KindRange},
		{spec.Histogram, false, KindRange},
		{spec.Stat, false, KindRange},
		{spec.Gauge, false, KindRange},
		{spec.BarGauge, false, KindRange},
		{spec.PieChart, false, KindRange},
		{spec.Table, false, KindInstant},
		{spec.Logs, false, KindInstant},
		{spec.NodeGraph, false, KindInstant},
		{spec.Traces, false, KindInstant},
		{spec.FlameGraph, false, KindInstant},
		{spec.Geomap, false, KindInstant},
		{spec.AlertList, false, KindInstant},
		{spec.BarChart, false, KindRange},
		{spec.StatusHistory, false, KindRange},
		{spec.Trend, false, KindRange},
		{spec.XYChart, false, KindRange},
		{spec.Candlestick, false, KindRange},
		{spec.Canvas, false, KindRange},
		{spec.BarChart, true, KindInstant},
		{spec.PanelType("unknown"), false, KindRange},
		{spec.Stat, true, KindInstant},
		{spec.Table, true, KindInstant},
		{spec.Timeseries, true, KindInstant},
		{spec.Heatmap, true, KindInstant},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s instant=%v", tt.panelType, tt.instant), func(t *testing.T) {
			p := &spec.Panel{Type: tt.panelType}
			if got := QueryKind(p, spec.Query{Instant: tt.instant}); got != tt.want {
				t.Fatalf("QueryKind = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPanelKind(t *testing.T) {
	tests := []struct {
		name  string
		panel spec.Panel
		want  Kind
	}{
		{"range query", spec.Panel{Type: spec.Timeseries, Queries: []spec.Query{{Query: "a"}}}, KindRange},
		{"instant only", spec.Panel{Type: spec.Timeseries, Queries: []spec.Query{{Query: "a", Instant: true}}}, KindInstant},
		{"one range among instants", spec.Panel{Type: spec.Timeseries, Queries: []spec.Query{{Query: "a", Instant: true}, {Query: "b"}}}, KindRange},
		{"hidden range ignored", spec.Panel{Type: spec.Timeseries, Queries: []spec.Query{{Query: "a", Instant: true}, {Query: "b", Hide: true}}}, KindInstant},
		{"all hidden", spec.Panel{Type: spec.Timeseries, Queries: []spec.Query{{Query: "a", Hide: true}}}, KindInstant},
		{"table", spec.Panel{Type: spec.Table, Queries: []spec.Query{{Query: "a"}}}, KindInstant},
		{"stat", spec.Panel{Type: spec.Stat, Queries: []spec.Query{{Query: "a"}}}, KindRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PanelKind(&tt.panel); got != tt.want {
				t.Fatalf("PanelKind = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQueryStep(t *testing.T) {
	base := 22 * time.Second
	tests := []struct {
		name    string
		minimum time.Duration
		factor  int
		want    time.Duration
	}{
		{"no minimum and no factor", 0, 0, base},
		{"minimum below base", 5 * time.Second, 0, base},
		{"minimum above base", time.Minute, 0, time.Minute},
		{"factor one changes nothing", 0, 1, base},
		{"factor doubles the base", 0, 2, 44 * time.Second},
		{"factor applies after the minimum", time.Minute, 2, 2 * time.Minute},
		{"factor applies to a base that beat the minimum", 5 * time.Second, 5, 110 * time.Second},
		{"negative factor counts as one", 0, -3, base},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QueryStep(base, tt.minimum, tt.factor); got != tt.want {
				t.Fatalf("QueryStep = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStepFor(t *testing.T) {
	span := func(d time.Duration) datasource.TimeRange {
		return datasource.TimeRange{From: testNow.Add(-d), To: testNow}
	}
	tests := []struct {
		name          string
		rng           datasource.TimeRange
		maxDataPoints int
		want          time.Duration
	}{
		{"6h at 1000 rounds 21.6s up", span(6 * time.Hour), 1000, 22 * time.Second},
		{"zero uses default 1000", span(6 * time.Hour), 0, 22 * time.Second},
		{"save uses 200", span(6 * time.Hour), 200, 108 * time.Second},
		{"below minimum clamps to 10", span(6 * time.Hour), 5, 2160 * time.Second},
		{"negative clamps to 10", span(6 * time.Hour), -3, 2160 * time.Second},
		{"exactly minimum", span(100 * time.Second), 10, 10 * time.Second},
		{"above maximum clamps to 11000", span(6 * time.Hour), 20000, 2 * time.Second},
		{"exactly maximum", span(11000 * time.Second), 11000, time.Second},
		{"just above maximum clamps", span(11000 * time.Second), 11001, time.Second},
		{"just under maximum rounds up", span(11000 * time.Second), 10999, 2 * time.Second},
		{"exact division stays whole", span(1000 * time.Second), 1000, time.Second},
		{"one nanosecond over rounds up", span(1000*time.Second + time.Nanosecond), 1000, 2 * time.Second},
		{"sub-second raw step floors to 1s", span(time.Hour), 11000, time.Second},
		{"tiny span floors to 1s", span(time.Millisecond), 1000, time.Second},
		{"zero span", datasource.TimeRange{From: testNow, To: testNow}, 1000, time.Second},
		{"inverted span", datasource.TimeRange{From: testNow, To: testNow.Add(-time.Hour)}, 1000, time.Second},
		{"30d at 200", span(30 * 24 * time.Hour), 200, 12960 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StepFor(tt.rng, tt.maxDataPoints)
			if got != tt.want {
				t.Fatalf("StepFor = %v, want %v", got, tt.want)
			}
			if got%time.Second != 0 || got < time.Second {
				t.Fatalf("step %v is not a whole number of seconds >= 1s", got)
			}
		})
	}
}

func TestRunPanelDispatch(t *testing.T) {
	tests := []struct {
		name          string
		panelType     spec.PanelType
		maxDataPoints int
		wantKind      Kind
		wantMethod    string
		wantStep      time.Duration
	}{
		{"timeseries uses range with step", spec.Timeseries, 200, KindRange, "range", 108 * time.Second},
		{"timeseries default points", spec.Timeseries, 0, KindRange, "range", 22 * time.Second},
		{"stat uses range with step", spec.Stat, 200, KindRange, "range", 108 * time.Second},
		{"table uses instant", spec.Table, 200, KindInstant, "instant", 0},
		{"bargauge uses range with step", spec.BarGauge, 200, KindRange, "range", 108 * time.Second},
		{"logs uses instant", spec.Logs, 200, KindInstant, "instant", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			d := dashboardOf([]spec.Panel{panelOf("p1", tt.panelType, "prom", "up")})
			res, err := runner.RunPanel(t.Context(), Request{
				Dashboard:     d,
				PanelID:       "p1",
				Range:         testRange(),
				MaxDataPoints: tt.maxDataPoints,
			})
			if err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			if res.Kind != tt.wantKind || res.Step != tt.wantStep || res.Range != testRange() {
				t.Fatalf("result kind=%q step=%v range=%v, want kind=%q step=%v", res.Kind, res.Step, res.Range, tt.wantKind, tt.wantStep)
			}
			if len(res.Frames()) == 0 {
				t.Fatal("no frames returned")
			}
			calls := fake.recorded()
			if len(calls) != 1 || calls[0].method != tt.wantMethod || calls[0].step != tt.wantStep {
				t.Fatalf("calls = %+v, want one %s call with step %v", calls, tt.wantMethod, tt.wantStep)
			}
			if calls[0].query.Text != "up" || calls[0].rng != testRange() {
				t.Fatalf("query/range not forwarded: %+v", calls[0])
			}
		})
	}
}

func TestRunPanelProxyPassesZeroLimits(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{panelOf("p1", spec.Timeseries, "prom", "up")})
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if got := fake.recorded()[0].query.Limits; got != (datasource.Limits{}) {
		t.Fatalf("limits = %+v, want zero", got)
	}
}

func TestRunPanelRunsRowChildren(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{
		panelOf("p0", spec.Stat, "prom", "up"),
		{ID: "r1", Type: spec.Row, Panels: []spec.Panel{panelOf("child", spec.Stat, "prom", "child_query")}},
	})
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "child", Range: testRange()}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if got := fake.recorded(); len(got) != 1 || got[0].query.Text != "child_query" {
		t.Fatalf("calls = %+v, want one call with the row child's query", got)
	}
}

func TestRunPanelErrors(t *testing.T) {
	tests := []struct {
		name    string
		d       *spec.Dashboard
		panelID string
		rng     datasource.TimeRange
		want    error
	}{
		{"nil dashboard", nil, "p1", testRange(), ErrPanelNotFound},
		{"unknown panel", dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "prom", "up")}), "nope", testRange(), ErrPanelNotFound},
		{"unknown datasource", dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "missing", "up")}), "p1", testRange(), ErrUnknownDatasource},
		{"text panel", dashboardOf([]spec.Panel{{ID: "t", Type: spec.Text}}), "t", testRange(), ErrPanelNotFound},
		{"row", dashboardOf([]spec.Panel{{ID: "r", Type: spec.Row}}), "r", testRange(), ErrPanelNotFound},
		{"zero range", dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "prom", "up")}), "p1", datasource.TimeRange{}, ErrInvalidRequest},
		{
			"inverted range",
			dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "prom", "up")}), "p1",
			datasource.TimeRange{From: testNow, To: testNow.Add(-time.Hour)}, ErrInvalidRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			_, err := runner.RunPanel(t.Context(), Request{Dashboard: tt.d, PanelID: tt.panelID, Range: tt.rng})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if errors.Is(tt.want, ErrInvalidRequest) && (errors.Is(err, datasource.ErrInvalidQuery) || strings.Contains(err.Error(), "invalid query")) {
				t.Fatalf("err = %v, want no invalid query text", err)
			}
			if fake.callCount() != 0 {
				t.Fatalf("upstream called %d times", fake.callCount())
			}
		})
	}
}

func TestRunPanelPropagatesUpstreamError(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = failWith(datasource.Errorf(datasource.ErrUnavailable, "connection refused"))
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{panelOf("p1", spec.Timeseries, "prom", "up")})
	res, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
	if !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if res.Queries != nil || res.Kind != "" {
		t.Fatalf("result on error = %+v, want zero", res)
	}
}

func TestRunPanelSelectionValidation(t *testing.T) {
	strings101 := make([]string, 101)
	for i := range strings101 {
		strings101[i] = "v"
	}
	strings100 := strings101[:100]
	tests := []struct {
		name    string
		sel     Selection
		wantErr bool
	}{
		{"plain value", Selection{Values: []string{"checkout-0"}}, false},
		{"non-ascii value", Selection{Values: []string{"café"}}, false},
		{"value with tab-free punctuation", Selection{Values: []string{`a"b'c\d`}}, false},
		{"exactly 100 values", Selection{Values: strings100}, false},
		{"exactly 1024 bytes", Selection{Values: []string{strings.Repeat("a", 1024)}}, false},
		{"all selection", Selection{All: true}, false},
		{"nul byte", Selection{Values: []string{"a\x00b"}}, true},
		{"newline", Selection{Values: []string{"a\nb"}}, true},
		{"tab", Selection{Values: []string{"a\tb"}}, true},
		{"escape", Selection{Values: []string{"\x1b[0m"}}, true},
		{"delete", Selection{Values: []string{"a\x7fb"}}, true},
		{"control in second value", Selection{Values: []string{"ok", "bad\x01"}}, true},
		{"101 values", Selection{Values: strings101}, true},
		{"1025 bytes", Selection{Values: []string{strings.Repeat("a", 1025)}}, true},
		{"control byte with all set", Selection{Values: []string{"a\x00"}, All: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			v := queryVariable("pod", "prom", "label_values(pod)")
			v.Multi, v.IncludeAll = true, true
			d := dashboardOf([]spec.Panel{panelOf("p1", spec.Timeseries, "prom", "up{pod=~$pod}")}, v)
			_, err := runner.RunPanel(t.Context(), Request{
				Dashboard:  d,
				PanelID:    "p1",
				Range:      testRange(),
				Selections: map[string]Selection{"pod": tt.sel},
			})
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("RunPanel: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidRequest) || errors.Is(err, datasource.ErrInvalidQuery) {
				t.Fatalf("err = %v, want only ErrInvalidRequest", err)
			}
			if fake.callCount() != 0 {
				t.Fatalf("upstream called %d times before validation failed", fake.callCount())
			}
		})
	}
}

func TestRunPanelPassesSelectionsInDeclarationOrder(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	a, b := customVariable("a", "a1"), customVariable("b", "b1", "b2")
	b.Multi = true
	d := dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "prom", "up")}, a, b)
	_, err := runner.RunPanel(t.Context(), Request{
		Dashboard: d,
		PanelID:   "p1",
		Range:     testRange(),
		Selections: map[string]Selection{
			"b": {Values: []string{"b1", "b2"}},
			"a": {Values: []string{"a1"}},
		},
	})
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	got := fake.recorded()[0].query.Variables
	want := []datasource.Variable{{Name: "a", Values: []string{"a1"}}, {Name: "b", Values: []string{"b1", "b2"}}}
	if !equalVariables(got, want) {
		t.Fatalf("variables = %+v, want %+v", got, want)
	}
}

func TestRunPanelRecoversFromPanic(t *testing.T) {
	const secret = "s3cret-token-value"
	for _, kind := range []string{"range", "instant"} {
		t.Run(kind, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onRange = func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
				panic(secret)
			}
			fake.onInstant = func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
				panic(errors.New(secret))
			}
			var logs bytes.Buffer
			runner := newTestRunner(t, slog.New(slog.NewTextHandler(&logs, nil)), map[string]*fakeDatasource{"prom": fake})
			panelType := spec.Timeseries
			if kind == "instant" {
				panelType = spec.Stat
			}
			d := dashboardOf([]spec.Panel{panelOf("p1", panelType, "prom", "up")})
			_, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
			if err == nil {
				t.Fatal("RunPanel returned nil error after panic")
			}
			status, code, message := Classify(err)
			if status != 500 || code != "internal" || message != "internal error" {
				t.Fatalf("Classify = %d %q %q, want 500 internal %q", status, code, message, "internal error")
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(message, secret) {
				t.Fatalf("panic value leaked into error: %v / %q", err, message)
			}
			if !strings.Contains(logs.String(), "datasource call panicked") {
				t.Fatalf("panic not logged: %q", logs.String())
			}
			if strings.Contains(logs.String(), secret) {
				t.Fatalf("panic value leaked into logs: %q", logs.String())
			}
		})
	}
}

func TestPanicReleasesSemaphoreSlot(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		panic("boom")
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{panelOf("p1", spec.Timeseries, "prom", "up")})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for i := range maxUpstreamCalls + 8 {
		_, err := runner.RunPanel(ctx, Request{Dashboard: d, PanelID: "p1", Range: testRange()})
		if !errors.Is(err, errInternal) {
			t.Fatalf("call %d: err = %v, want internal error", i, err)
		}
	}
}

func holdUpstreamSlots(t *testing.T, runner *Runner, fake *fakeDatasource, d *spec.Dashboard, n int) (release func(), wait func()) {
	t.Helper()
	var wg sync.WaitGroup
	gate := make(chan struct{})
	fake.onRange = func(ctx context.Context, _ datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return dataFrames(), nil
	}
	selections := map[string]Selection{}
	for _, v := range d.Variables {
		selections[v.Name] = Selection{Values: []string{"x"}}
	}
	for range n {
		wg.Go(func() {
			if _, err := runner.RunPanel(context.Background(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: selections}); err != nil {
				t.Errorf("blocked RunPanel: %v", err)
			}
		})
	}
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(func() { release(); wg.Wait() })
	return release, wg.Wait
}

func TestUpstreamConcurrencyNeverExceeds32(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{panelOf("p1", spec.Timeseries, "prom", "up")})

	release, wait := holdUpstreamSlots(t, runner, fake, d, maxUpstreamCalls+16)
	waitFor(t, "32 concurrent upstream calls", func() bool { return fake.inflightNow() == maxUpstreamCalls })
	time.Sleep(50 * time.Millisecond)
	if got := fake.inflightNow(); got != maxUpstreamCalls {
		t.Fatalf("inflight = %d while saturated, want exactly %d", got, maxUpstreamCalls)
	}
	if got := fake.callCount(); got != maxUpstreamCalls {
		t.Fatalf("upstream calls started = %d while saturated, want %d", got, maxUpstreamCalls)
	}
	release()
	wait()
	if got := fake.peakInflight(); got != maxUpstreamCalls {
		t.Fatalf("peak inflight = %d, want %d", got, maxUpstreamCalls)
	}
	if got := fake.callCount(); got != maxUpstreamCalls+16 {
		t.Fatalf("total calls = %d, want %d", got, maxUpstreamCalls+16)
	}
}

func TestUpstreamSemaphoreWaitHonorsContext(t *testing.T) {
	tests := []struct {
		name       string
		makeCtx    func(t *testing.T) (context.Context, func())
		wantErr    error
		wantStatus int
		wantCode   string
	}{
		{
			name: "canceled",
			makeCtx: func(t *testing.T) (context.Context, func()) {
				ctx, cancel := context.WithCancel(t.Context())
				return ctx, func() { time.Sleep(20 * time.Millisecond); cancel() }
			},
			wantErr:    context.Canceled,
			wantStatus: statusCanceled,
			wantCode:   "canceled",
		},
		{
			name: "deadline",
			makeCtx: func(t *testing.T) (context.Context, func()) {
				ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
				t.Cleanup(cancel)
				return ctx, func() {}
			},
			wantErr:    context.DeadlineExceeded,
			wantStatus: 504,
			wantCode:   "query_timeout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			d := dashboardOf([]spec.Panel{panelOf("p1", spec.Timeseries, "prom", "up")})
			holdUpstreamSlots(t, runner, fake, d, maxUpstreamCalls)
			waitFor(t, "saturated semaphore", func() bool { return fake.inflightNow() == maxUpstreamCalls })

			ctx, trigger := tt.makeCtx(t)
			done := make(chan error, 1)
			go func() {
				_, err := runner.RunPanel(ctx, Request{Dashboard: d, PanelID: "p1", Range: testRange()})
				done <- err
			}()
			trigger()
			select {
			case err := <-done:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				status, code, _ := Classify(err)
				if status != tt.wantStatus || code != tt.wantCode {
					t.Fatalf("Classify = %d %q, want %d %q", status, code, tt.wantStatus, tt.wantCode)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("RunPanel did not return after context ended while waiting for a slot")
			}
			if got := fake.callCount(); got != maxUpstreamCalls {
				t.Fatalf("upstream calls = %d, want %d (waiter must not reach upstream)", got, maxUpstreamCalls)
			}
		})
	}
}

func TestUpstreamSemaphoreSharedByVariableLookups(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{panelOf("p1", spec.Timeseries, "prom", "up")}, queryVariable("v", "prom", "label_values(job)"))
	holdUpstreamSlots(t, runner, fake, d, maxUpstreamCalls)
	waitFor(t, "saturated semaphore", func() bool { return fake.inflightNow() == maxUpstreamCalls })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	_, _, err := runner.VariableValues(ctx, d, "v", testRange(), nil, datasource.QueryUser{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if fake.callsTo("variable") != nil {
		t.Fatal("variable lookup reached upstream while semaphore was full")
	}
}

func equalVariables(a, b []datasource.Variable) bool {
	return slices.EqualFunc(a, b, func(x, y datasource.Variable) bool {
		return x.Name == y.Name && x.All == y.All && slices.Equal(x.Values, y.Values)
	})
}

func TestUpstreamLogsInternalErrorsOnceWithoutSecrets(t *testing.T) {
	const secret = "hunter2-token"
	tests := []struct {
		name    string
		fail    error
		wantLog bool
	}{
		{"unclassified error", errors.New("dial tcp 10.0.0.5:9090: password=" + secret), true},
		{"postgres error", fmt.Errorf("query: %w", &pgconn.PgError{Code: "57014", Message: "canceling statement for password=" + secret, Detail: "secret_query"}), true},
		{"classified error", datasource.Errorf(datasource.ErrUnavailable, "connection refused"), false},
		{"invalid query", datasource.Errorf(datasource.ErrInvalidQuery, "bad syntax"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onRange = failWith(tt.fail)
			var logs bytes.Buffer
			runner := newTestRunner(t, slog.New(slog.NewTextHandler(&logs, nil)), map[string]*fakeDatasource{"prom": fake})
			d := dashboardOf([]spec.Panel{panelOf("p1", spec.Timeseries, "prom", "up{secret_query=1}")})
			_, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
			if err == nil {
				t.Fatal("RunPanel returned nil error")
			}
			if got := strings.Count(logs.String(), "datasource call failed"); (got == 1) != tt.wantLog || got > 1 {
				t.Fatalf("log count = %d, want logged=%v: %q", got, tt.wantLog, logs.String())
			}
			if tt.wantLog {
				wantType := "error_type=*errors.errorString"
				wantExtra := "error_code=internal"
				if strings.HasPrefix(tt.name, "postgres") {
					wantType = "error_type=*pgconn.PgError"
					wantExtra = "sqlstate=57014"
				}
				for _, want := range []string{"subject=p1", "datasource=prom", "datasource_type=prometheus", "error_code=internal", wantType, wantExtra} {
					if !strings.Contains(logs.String(), want) {
						t.Fatalf("log missing %q: %q", want, logs.String())
					}
				}
				if strings.Contains(logs.String(), " error=") {
					t.Fatalf("error text logged: %q", logs.String())
				}
			}
			for _, leak := range []string{"secret_query", secret, "10.0.0.5", "canceling statement"} {
				if strings.Contains(logs.String(), leak) {
					t.Fatalf("log leaks %q: %q", leak, logs.String())
				}
			}
			if _, _, message := Classify(err); strings.Contains(message, secret) {
				t.Fatalf("message leaked detail: %q", message)
			}
		})
	}
}

func TestUpstreamLogsVariableLookupSubject(t *testing.T) {
	fake := newFake("prometheus")
	fake.onVariable = func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
		return nil, errors.New("boom")
	}
	var logs bytes.Buffer
	runner := newTestRunner(t, slog.New(slog.NewTextHandler(&logs, nil)), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf(nil, queryVariable("job", "prom", "label_values(job)"))
	if _, _, err := runner.VariableValues(t.Context(), d, "job", testRange(), nil, datasource.QueryUser{}); err == nil {
		t.Fatal("VariableValues returned nil error")
	}
	if !strings.Contains(logs.String(), "subject=variable:job") {
		t.Fatalf("log = %q", logs.String())
	}
}

func multiPanel(typ spec.PanelType, queries ...spec.Query) spec.Panel {
	return spec.Panel{ID: "p1", Type: typ, Title: "p1", Datasource: "prom", Queries: queries}
}

func TestRunPanelRunsEveryVisibleQuery(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		return []datasource.Frame{{Name: q.Text, Fields: dataFrames()[0].Fields}}, nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Query: "a"},
		spec.Query{Ref: "Z", Query: "b"},
		spec.Query{Query: "hidden", Hide: true},
		spec.Query{Query: "c"},
	)})
	res, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), MaxDataPoints: 200})
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	var refs, names []string
	for _, q := range res.Queries {
		refs = append(refs, q.Ref)
	}
	for _, f := range res.Frames() {
		names = append(names, f.Name)
	}
	if !slices.Equal(refs, []string{"A", "Z", "D"}) || !slices.Equal(names, []string{"a", "b", "c"}) {
		t.Fatalf("refs = %v, frames = %v; want refs [A Z D] in panel order and frames [a b c]", refs, names)
	}
	if fake.callCount() != 3 {
		t.Fatalf("upstream calls = %d, want 3 (the hidden query never runs)", fake.callCount())
	}
	if res.Kind != KindRange || res.Step != 108*time.Second {
		t.Fatalf("kind = %q, step = %v", res.Kind, res.Step)
	}
}

func TestRunPanelIncludeHiddenRunsHiddenQueriesWithoutReportingThem(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries, spec.Query{Query: "a"}, spec.Query{Query: "h", Hide: true})})
	res, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), IncludeHidden: true})
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if fake.callCount() != 2 || len(res.Queries) != 1 || res.Queries[0].Ref != "A" {
		t.Fatalf("calls = %d, queries = %+v; want 2 calls and only query A reported", fake.callCount(), res.Queries)
	}
}

func TestRunPanelAllHiddenReturnsNoQueries(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries, spec.Query{Query: "a", Hide: true})})
	res, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if len(res.Queries) != 0 || res.Kind != KindInstant || res.Step != 0 || fake.callCount() != 0 {
		t.Fatalf("result = %+v, calls = %d; want no queries, instant, step 0, no calls", res, fake.callCount())
	}
}

func TestRunPanelPerQueryKindAndStep(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	p := multiPanel(spec.Timeseries,
		spec.Query{Query: "plain"},
		spec.Query{Query: "slow", Interval: "5m"},
		spec.Query{Query: "point", Instant: true},
	)
	p.Interval = "1m"
	res, err := runner.RunPanel(t.Context(), Request{Dashboard: dashboardOf([]spec.Panel{p}), PanelID: "p1", Range: testRange(), MaxDataPoints: 200})
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	want := []struct {
		kind Kind
		step time.Duration
	}{{KindRange, 108 * time.Second}, {KindRange, 5 * time.Minute}, {KindInstant, 0}}
	for i, q := range res.Queries {
		if q.Kind != want[i].kind || q.Step != want[i].step {
			t.Fatalf("query %d kind=%q step=%v, want kind=%q step=%v", i, q.Kind, q.Step, want[i].kind, want[i].step)
		}
	}
	steps := map[string]time.Duration{}
	for _, c := range fake.callsTo("range") {
		steps[c.query.Text] = c.step
	}
	if steps["plain"] != 108*time.Second || steps["slow"] != 5*time.Minute || len(fake.callsTo("instant")) != 1 {
		t.Fatalf("range steps = %v, instant calls = %d", steps, len(fake.callsTo("instant")))
	}
}

func TestRunPanelGivesInstantQueriesTheirMinimumInterval(t *testing.T) {
	fake := newFake("clickhouse")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	p := multiPanel(spec.Table,
		spec.Query{Query: "panel interval"},
		spec.Query{Query: "own interval", Interval: "5m"},
	)
	p.Interval = "1m"
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: dashboardOf([]spec.Panel{p}), PanelID: "p1", Range: testRange(), MaxDataPoints: 200}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	got := map[string]time.Duration{}
	for _, c := range fake.callsTo("instant") {
		got[c.query.Text] = c.query.MinInterval
	}
	want := map[string]time.Duration{"panel interval": time.Minute, "own interval": 5 * time.Minute}
	if !maps.Equal(got, want) {
		t.Fatalf("instant minimum intervals = %v, want %v", got, want)
	}
}

func TestRunPanelLeavesTheMinimumIntervalEmptyWithoutAnInterval(t *testing.T) {
	fake := newFake("clickhouse")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	p := multiPanel(spec.Table, spec.Query{Query: "plain"})
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: dashboardOf([]spec.Panel{p}), PanelID: "p1", Range: testRange(), MaxDataPoints: 200}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	calls := fake.callsTo("instant")
	if len(calls) != 1 || calls[0].query.MinInterval != 0 {
		t.Fatalf("instant calls = %+v, want one with no minimum interval", calls)
	}
}

func stepsByQuery(fake *fakeDatasource) map[string]time.Duration {
	steps := map[string]time.Duration{}
	for _, c := range fake.callsTo("range") {
		steps[c.query.Text] = c.step
	}
	return steps
}

func TestRunPanelResolvesAMinStepFromAnIntervalVariable(t *testing.T) {
	tests := []struct {
		name      string
		selection string
		panelIv   string
		query     spec.Query
		want      time.Duration
	}{
		{"query names the variable", "5m", "", spec.Query{Query: "q", Interval: "${iv}"}, 5 * time.Minute},
		{"panel names the variable", "5m", "${iv}", spec.Query{Query: "q"}, 5 * time.Minute},
		{"query wins over the panel", "5m", "1m", spec.Query{Query: "q", Interval: "${iv}"}, 5 * time.Minute},
		{"selection below the base step", "1ms", "", spec.Query{Query: "q", Interval: "${iv}"}, 108 * time.Second},
		{"auto follows the range", "$__auto", "", spec.Query{Query: "q", Interval: "${iv}"}, 15 * time.Minute},
		{"factor multiplies the selected step", "5m", "", spec.Query{Query: "q", Interval: "${iv}", IntervalFactor: 2}, 10 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			p := multiPanel(spec.Timeseries, tt.query)
			p.Interval = tt.panelIv
			iv := intervalVariable("iv", "1ms", "1m", "5m")
			iv.Auto = true
			req := Request{
				Dashboard:     dashboardOf([]spec.Panel{p}, iv),
				PanelID:       "p1",
				Range:         testRange(),
				MaxDataPoints: 200,
				Selections:    map[string]Selection{"iv": {Values: []string{tt.selection}}},
			}
			res, err := runner.RunPanel(t.Context(), req)
			if err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			if got := stepsByQuery(fake)["q"]; got != tt.want {
				t.Fatalf("step = %v, want %v", got, tt.want)
			}
			if res.Queries[0].Step != tt.want {
				t.Fatalf("result step = %v, want %v", res.Queries[0].Step, tt.want)
			}
		})
	}
}

func TestRunPanelAppliesTheResolutionFactorToTheBaseStep(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	p := multiPanel(spec.Timeseries,
		spec.Query{Query: "plain"},
		spec.Query{Query: "doubled", IntervalFactor: 2},
		spec.Query{Query: "literal", Interval: "5m", IntervalFactor: 3},
	)
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: dashboardOf([]spec.Panel{p}), PanelID: "p1", Range: testRange(), MaxDataPoints: 200}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	want := map[string]time.Duration{"plain": 108 * time.Second, "doubled": 216 * time.Second, "literal": 15 * time.Minute}
	if got := stepsByQuery(fake); !maps.Equal(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
}

func TestRunPanelFailsAQueryWhoseMinStepVariableHasNoUsableSelection(t *testing.T) {
	tests := []struct {
		name       string
		variables  []spec.Variable
		selections map[string]Selection
		interval   string
	}{
		{"not selected", []spec.Variable{intervalVariable("iv", "1m")}, nil, "${iv}"},
		{"not an interval", []spec.Variable{intervalVariable("iv", "1m"), customVariable("c", "30s")}, map[string]Selection{"iv": {Values: []string{"1m"}}, "c": {Values: []string{"30s"}}}, "${c}"},
		{"undeclared", nil, nil, "${missing}"},
		{"not a duration", []spec.Variable{intervalVariable("iv", "1m", "soon")}, map[string]Selection{"iv": {Values: []string{"soon"}}}, "${iv}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			p := multiPanel(spec.Timeseries, spec.Query{Query: "q", Interval: tt.interval})
			_, _, errs, err := runner.runQueries(t.Context(), Request{Dashboard: dashboardOf([]spec.Panel{p}, tt.variables...), PanelID: "p1", Range: testRange(), Selections: tt.selections}, newVarState(), false)
			if err != nil {
				t.Fatalf("runQueries: %v", err)
			}
			var qe *QueryError
			if !errors.As(errs[0], &qe) || !errors.Is(qe.Err, ErrInvalidRequest) {
				t.Fatalf("query error = %v, want an invalid request", errs[0])
			}
			if status, code, _ := Classify(errs[0]); status != 400 || code != string(CodeInvalidRequest) {
				t.Fatalf("Classify = %d %s, want 400 invalid_request", status, code)
			}
			if len(fake.recorded()) != 0 {
				t.Fatalf("datasource calls = %+v, want none", fake.recorded())
			}
		})
	}
}

func TestRunPanelInstantQueryTakesTheSelectedMinStepAndIgnoresTheFactor(t *testing.T) {
	fake := newFake("clickhouse")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	p := multiPanel(spec.Table, spec.Query{Query: "q", Interval: "${iv}", IntervalFactor: 4})
	req := Request{
		Dashboard:  dashboardOf([]spec.Panel{p}, intervalVariable("iv", "1m", "5m")),
		PanelID:    "p1",
		Range:      testRange(),
		Selections: map[string]Selection{"iv": {Values: []string{"5m"}}},
	}
	if _, err := runner.RunPanel(t.Context(), req); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	calls := fake.callsTo("instant")
	if len(calls) != 1 || calls[0].query.MinInterval != 5*time.Minute {
		t.Fatalf("instant calls = %+v, want one with a 5m minimum interval", calls)
	}
}

func TestRunPanelReportsLowestIndexFailureAfterAllQueriesFinish(t *testing.T) {
	fake := newFake("prometheus")
	release := make(chan struct{})
	fake.onRange = func(ctx context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		switch q.Text {
		case "late":
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "late failure")
		case "early":
			close(release)
			return nil, datasource.Errorf(datasource.ErrLimitExceeded, "early failure")
		}
		return dataFrames(), nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries, spec.Query{Query: "late"}, spec.Query{Query: "ok"}, spec.Query{Query: "early"})})
	_, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
	var qe *QueryError
	if !errors.As(err, &qe) || qe.Index != 0 || qe.Ref != "A" || !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v (%+v), want the lowest-index failure, query A", err, qe)
	}
	if fake.callCount() != 3 {
		t.Fatalf("upstream calls = %d, want 3: every query runs to completion", fake.callCount())
	}
}

func TestRunPanelQueryErrorNamesTheQueryRef(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		if q.Text == "bad" {
			return nil, datasource.Errorf(datasource.ErrLimitExceeded, "too many series")
		}
		return dataFrames(), nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries, spec.Query{Query: "ok"}, spec.Query{Ref: "errs", Query: "bad"})})
	_, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
	var qe *QueryError
	if !errors.As(err, &qe) || qe.Ref != "errs" || qe.Index != 1 {
		t.Fatalf("err = %v, want a QueryError for ref errs at index 1", err)
	}
	if status, code, _ := Classify(err); status != 422 || code != string(CodeLimitExceeded) {
		t.Fatalf("Classify = %d %s", status, code)
	}
}

func TestRunPanelQueryDatasourceOverride(t *testing.T) {
	main, other := newFake("prometheus"), newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": main, "other": other})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries, spec.Query{Query: "a"}, spec.Query{Query: "b", Datasource: "other"})})
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if main.callCount() != 1 || other.callCount() != 1 {
		t.Fatalf("calls main=%d other=%d, want 1 each", main.callCount(), other.callCount())
	}
	missing := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries, spec.Query{Query: "a"}, spec.Query{Ref: "B", Query: "b", Datasource: "gone"})})
	_, err := runner.RunPanel(t.Context(), Request{Dashboard: missing, PanelID: "p1", Range: testRange()})
	var qe *QueryError
	if !errors.Is(err, ErrUnknownDatasource) || !errors.As(err, &qe) || qe.Ref != "B" {
		t.Fatalf("err = %v, want ErrUnknownDatasource for query B", err)
	}
}

func TestRunPanelQueriesRunConcurrentlyUpToFour(t *testing.T) {
	const total = 10
	fake := newFake("prometheus")
	entered := make(chan struct{}, total)
	release := make(chan struct{})
	fake.onRange = func(ctx context.Context, _ datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return dataFrames(), nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	queries := make([]spec.Query, total)
	for i := range queries {
		queries[i] = spec.Query{Query: fmt.Sprintf("q%d", i)}
	}
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries, queries...)})
	done := make(chan error, 1)
	go func() {
		_, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
		done <- err
	}()
	for range queryParallelism {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for four queries to run at once")
		}
	}
	if got := fake.inflightNow(); got != queryParallelism {
		t.Fatalf("in flight = %d, want %d while the first four are blocked", got, queryParallelism)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if got := fake.peakInflight(); got != queryParallelism {
		t.Fatalf("peak concurrent queries = %d, want exactly %d", got, queryParallelism)
	}
	if got := fake.callCount(); got != total {
		t.Fatalf("upstream calls = %d, want %d", got, total)
	}
}

func TestFloorHandlesTimesBeforeTheEpoch(t *testing.T) {
	got := Floor(time.UnixMilli(-1500), time.Second)
	if want := time.UnixMilli(-2000); !got.Equal(want) {
		t.Fatalf("Floor = %v, want %v", got, want)
	}
}

func TestRunPanelAlignsAQueryWithALargerStepToItsOwnGrid(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Query: "plain"},
		spec.Query{Query: "slow", Interval: "5m"},
	)})
	rng := datasource.TimeRange{From: testNow.Add(-6*time.Hour + 37*time.Second), To: testNow.Add(37 * time.Second)}
	res, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: rng, MaxDataPoints: 200})
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	ranges := map[string]datasource.TimeRange{}
	for _, c := range fake.callsTo("range") {
		ranges[c.query.Text] = c.rng
	}
	if ranges["plain"] != rng {
		t.Fatalf("plain range = %v, want the request range %v", ranges["plain"], rng)
	}
	wantSlow := datasource.TimeRange{From: testNow.Add(-6 * time.Hour), To: testNow}
	if !ranges["slow"].From.Equal(wantSlow.From) || !ranges["slow"].To.Equal(wantSlow.To) {
		t.Fatalf("slow range = %v, want %v", ranges["slow"], wantSlow)
	}
	if res.Range != rng {
		t.Fatalf("result range = %v, want the request range %v", res.Range, rng)
	}
}

func TestRunPanelKeepsARangeNonEmptyWhenAQueryStepIsLongerThanIt(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries, spec.Query{Query: "daily", Interval: "1d"})})
	rng := datasource.TimeRange{From: testNow.Add(-time.Hour), To: testNow}
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: rng, MaxDataPoints: 200}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	got := fake.callsTo("range")[0].rng
	if !got.To.After(got.From) || got.To.Sub(got.From) != 24*time.Hour {
		t.Fatalf("range = %v, want one day wide", got)
	}
}

func TestRunPanelStopsAtThePanelDeadlineAndNamesTheLowestUnfinishedQuery(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = func(ctx context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		if q.Text == "fast" {
			return dataFrames(), nil
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	runner.panelDeadline = 50 * time.Millisecond
	queries := []spec.Query{{Query: "fast"}}
	for i := range 9 {
		queries = append(queries, spec.Query{Query: fmt.Sprintf("slow%d", i)})
	}
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries, queries...)})
	start := time.Now()
	_, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("RunPanel took %v, want it to stop at the panel deadline", elapsed)
	}
	var qe *QueryError
	if !errors.As(err, &qe) || qe.Index != 1 || qe.Ref != "B" {
		t.Fatalf("err = %v (%+v), want a QueryError for the lowest unfinished query B", err, qe)
	}
	if status, code, _ := Classify(err); status != 504 || code != string(CodeQueryTimeout) {
		t.Fatalf("Classify = %d %s, want 504 query_timeout", status, code)
	}
}

func TestRunPanelAdmitSeesEachVisibleQueryAndCanRefuseOne(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Query: "a"},
		spec.Query{Query: "hidden", Hide: true},
		spec.Query{Ref: "big", Query: "b"},
	)})
	refused := errors.New("too big")
	var mu sync.Mutex
	admitted := map[int]string{}
	_, err := runner.RunPanel(t.Context(), Request{
		Dashboard: d, PanelID: "p1", Range: testRange(), IncludeHidden: true,
		Admit: func(index int, q QueryResult) error {
			mu.Lock()
			defer mu.Unlock()
			admitted[index] = q.Ref
			if q.Ref == "big" {
				return refused
			}
			return nil
		},
	})
	var qe *QueryError
	if !errors.As(err, &qe) || qe.Index != 2 || qe.Ref != "big" || !errors.Is(err, refused) {
		t.Fatalf("err = %v (%+v), want the refusal as a QueryError for big", err, qe)
	}
	if len(admitted) != 2 || admitted[0] != "A" || admitted[2] != "big" {
		t.Fatalf("admitted = %v, want only the visible queries 0 and 2", admitted)
	}
}

func TestRunPanelAbortFromAdmitCancelsTheOthersAndOutranksQueryErrors(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = func(ctx context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		switch q.Text {
		case "stuck":
			<-ctx.Done()
			return nil, ctx.Err()
		case "bad":
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "nope")
		}
		return dataFrames(), nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Query: "bad"},
		spec.Query{Query: "stuck"},
		spec.Query{Query: "big"},
	)})
	stop := errors.New("stop")
	_, err := runner.RunPanel(t.Context(), Request{
		Dashboard: d, PanelID: "p1", Range: testRange(),
		Admit: func(index int, _ QueryResult) error {
			if index == 2 {
				return &AbortError{Err: stop}
			}
			return nil
		},
	})
	var qe *QueryError
	if !errors.Is(err, stop) || errors.As(err, &qe) {
		t.Fatalf("err = %v, want the abort error alone, without a query ref", err)
	}
}

func TestUserForPrefersTheEmailForTheLogin(t *testing.T) {
	tests := []struct {
		name string
		in   auth.Principal
		want datasource.QueryUser
	}{
		{"user", auth.Principal{Kind: auth.KindUser, ID: "u1", Email: "ada@example.com", Name: "Ada"}, datasource.QueryUser{ID: "u1", Login: "ada@example.com", Email: "ada@example.com", Name: "Ada"}},
		{"service account", auth.Principal{Kind: auth.KindServiceAccount, ID: "k1", Name: "ci"}, datasource.QueryUser{ID: "k1", Login: "ci", Name: "ci"}},
		{"nobody", auth.Principal{}, datasource.QueryUser{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UserFor(tt.in); got != tt.want {
				t.Fatalf("UserFor = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRunPanelPassesTheDashboardTitleAndCallerToEveryQuery(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{
		multiPanel(spec.Timeseries, spec.Query{Query: "a"}, spec.Query{Query: "b"}),
		{ID: "p2", Type: spec.Table, Title: "p2", Datasource: "prom", Queries: []spec.Query{{Query: "c"}}},
	}, queryVariable("job", "prom", "label_values(job)"))
	d.Title = "Checkout health"
	user := datasource.QueryUser{ID: "u1", Login: "ada@example.com", Email: "ada@example.com", Name: "Ada"}

	for _, panelID := range []string{"p1", "p2"} {
		if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: panelID, Range: testRange(), Selections: map[string]Selection{"job": {Values: []string{"v1"}}}, User: user}); err != nil {
			t.Fatalf("RunPanel %s: %v", panelID, err)
		}
	}
	if _, _, err := runner.VariableValues(t.Context(), d, "job", testRange(), nil, user); err != nil {
		t.Fatalf("VariableValues: %v", err)
	}
	if _, err := runner.Defaults(t.Context(), d, testRange(), nil, user); err != nil {
		t.Fatalf("Defaults: %v", err)
	}

	calls := fake.recorded()
	if len(calls) != 5 {
		t.Fatalf("%d upstream calls, want 5: %+v", len(calls), calls)
	}
	for _, c := range calls {
		if c.query.Dashboard != "Checkout health" || c.query.User != user {
			t.Errorf("%s call %q saw dashboard %q and user %+v", c.method, c.query.Text, c.query.Dashboard, c.query.User)
		}
	}
}

func exemplarFrameOf(labels map[string]string, times []time.Time, values []any) datasource.Frame {
	timeValues := make([]any, len(times))
	traces := make([]any, len(times))
	for i, at := range times {
		timeValues[i] = at
		traces[i] = fmt.Sprintf("trace-%d", at.UnixMilli())
	}
	return datasource.Frame{Labels: labels, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime, Values: timeValues},
		{Name: "Value", Type: datasource.FieldNumber, Values: values},
		{Name: "trace_id", Type: datasource.FieldString, Values: traces},
	}}
}

func exemplarRequest(d *spec.Dashboard) Request {
	return Request{Dashboard: d, PanelID: "p1", Range: testRange(), Exemplars: true}
}

func exemplarPanel(queries ...spec.Query) *spec.Dashboard {
	return dashboardOf([]spec.Panel{multiPanel(spec.Timeseries, queries...)})
}

func TestRunPanelFetchesExemplarsForTheRangeCallsStepAndRange(t *testing.T) {
	fake := newFake("prometheus")
	at := testRange().From.Add(time.Hour)
	fake.onExemplar = func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return []datasource.Frame{exemplarFrameOf(map[string]string{"job": "api"}, []time.Time{at}, []any{0.5})}, nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := exemplarPanel(spec.Query{Query: "up", Exemplar: true, IntervalFactor: 2})
	res, err := runner.RunPanel(t.Context(), exemplarRequest(d))
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	q := res.Queries[0]
	if q.ExemplarStatus != ExemplarsOK || len(q.Exemplars) != 1 || q.Exemplars[0].Labels["job"] != "api" {
		t.Fatalf("exemplars = %+v status %q, want one ok frame for job api", q.Exemplars, q.ExemplarStatus)
	}
	if len(q.Frames) == 0 {
		t.Fatal("the range frames are missing")
	}
	ranges, exemplars := fake.callsTo("range"), fake.callsTo("exemplars")
	if len(ranges) != 1 || len(exemplars) != 1 {
		t.Fatalf("calls = %+v, want one range and one exemplars call", fake.recorded())
	}
	if exemplars[0].step != ranges[0].step || exemplars[0].rng != ranges[0].rng || exemplars[0].query.Text != "up" {
		t.Fatalf("exemplars call %+v differs from range call %+v", exemplars[0], ranges[0])
	}
}

func TestRunPanelDoesNotFetchExemplarsUnlessAsked(t *testing.T) {
	tests := []struct {
		name    string
		request bool
		query   spec.Query
	}{
		{"request does not ask", false, spec.Query{Query: "up", Exemplar: true}},
		{"query does not ask", true, spec.Query{Query: "up"}},
		{"hidden query never runs", true, spec.Query{Query: "up", Exemplar: true, Hide: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			req := exemplarRequest(exemplarPanel(tt.query, spec.Query{Ref: "other", Query: "other"}))
			req.Exemplars = tt.request
			res, err := runner.RunPanel(t.Context(), req)
			if err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			if n := len(fake.callsTo("exemplars")); n != 0 {
				t.Fatalf("%d exemplars calls, want none", n)
			}
			for _, q := range res.Queries {
				if q.ExemplarStatus != "" || q.Exemplars != nil {
					t.Fatalf("query %s has exemplar status %q, want none", q.Ref, q.ExemplarStatus)
				}
			}
		})
	}
}

func TestRunPanelMarksOnlyTheQueriesThatAskedForExemplars(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := exemplarPanel(spec.Query{Ref: "asks", Query: "a", Exemplar: true}, spec.Query{Ref: "quiet", Query: "b"})
	res, err := runner.RunPanel(t.Context(), exemplarRequest(d))
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if res.Queries[0].ExemplarStatus != ExemplarsOK || res.Queries[1].ExemplarStatus != "" {
		t.Fatalf("statuses = %q and %q, want ok and empty", res.Queries[0].ExemplarStatus, res.Queries[1].ExemplarStatus)
	}
}

func TestRunPanelReportsExemplarsUnavailableWithoutFailingThePanel(t *testing.T) {
	const secret = "hunter2-token"
	fake := newFake("prometheus")
	fake.onExemplar = func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return nil, datasource.Errorf(datasource.ErrTimeout, "exemplars slow, password="+secret)
	}
	logger, logs := logtest.New()
	runner := newTestRunner(t, logger, map[string]*fakeDatasource{"prom": fake})
	d := exemplarPanel(spec.Query{Ref: "asks", Query: "a", Exemplar: true})
	res, err := runner.RunPanel(t.Context(), exemplarRequest(d))
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	q := res.Queries[0]
	if q.ExemplarStatus != ExemplarsUnavailable || len(q.Exemplars) != 0 || len(q.Frames) == 0 {
		t.Fatalf("query = %+v, want unavailable exemplars next to the range frames", q)
	}
	record := logs.Find(t, "exemplar fetch failed")
	if record["level"] != "WARN" || record["query_ref"] != "asks" {
		t.Fatalf("log record = %v, want a WARN naming query_ref asks", record)
	}
	logs.AssertSafeFailure(t, record, "query_timeout", secret)
}

func TestRunPanelReportsExemplarsUnsupported(t *testing.T) {
	tests := []struct {
		name  string
		query spec.Query
		inst  func(*fakeDatasource) datasource.Datasource
	}{
		{"datasource without exemplars", spec.Query{Query: "up", Exemplar: true}, func(f *fakeDatasource) datasource.Datasource { return withoutExemplars{f} }},
		{"instant query", spec.Query{Query: "up", Exemplar: true, Instant: true}, func(f *fakeDatasource) datasource.Datasource { return f }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newRunnerOf(t, discardLogger(), datasource.Instance{UID: "prom", Datasource: tt.inst(fake)})
			res, err := runner.RunPanel(t.Context(), exemplarRequest(exemplarPanel(tt.query)))
			if err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			q := res.Queries[0]
			if q.ExemplarStatus != ExemplarsUnsupported || len(q.Exemplars) != 0 || len(q.Frames) == 0 {
				t.Fatalf("query = %+v, want unsupported with its frames", q)
			}
			if n := len(fake.callsTo("exemplars")); n != 0 {
				t.Fatalf("%d exemplars calls, want none", n)
			}
		})
	}
}

func TestRunPanelSkipsExemplarsWhenTheRangeQueryFails(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = failWith(datasource.Errorf(datasource.ErrUnavailable, "down"))
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	_, err := runner.RunPanel(t.Context(), exemplarRequest(exemplarPanel(spec.Query{Query: "up", Exemplar: true})))
	if err == nil {
		t.Fatal("RunPanel succeeded, want the range error")
	}
	if n := len(fake.callsTo("exemplars")); n != 0 {
		t.Fatalf("%d exemplars calls after a failed range query, want none", n)
	}
}

func TestRunPanelKeepsTheHighestExemplarPerStepBucket(t *testing.T) {
	from := testRange().From
	step := 22 * time.Second
	at := func(bucket int, offset time.Duration) time.Time { return from.Add(time.Duration(bucket)*step + offset) }
	fake := newFake("prometheus")
	fake.onExemplar = func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return []datasource.Frame{
			exemplarFrameOf(map[string]string{"job": "a"},
				[]time.Time{at(3, time.Second), at(3, 5*time.Second), at(3, 9*time.Second), at(4, 0), at(7, time.Second)},
				[]any{1.0, 9.0, 4.0, nil, 2.0}),
			exemplarFrameOf(map[string]string{"job": "b"}, []time.Time{at(3, 2*time.Second)}, []any{0.1}),
		}, nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	res, err := runner.RunPanel(t.Context(), exemplarRequest(exemplarPanel(spec.Query{Query: "up", Exemplar: true})))
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	q := res.Queries[0]
	if q.ExemplarStatus != ExemplarsOK || len(q.Exemplars) != 2 {
		t.Fatalf("status %q with %d frames, want ok and 2", q.ExemplarStatus, len(q.Exemplars))
	}
	wantTimes := []any{at(3, 5*time.Second), at(4, 0), at(7, time.Second)}
	wantValues := []any{9.0, nil, 2.0}
	if got := q.Exemplars[0].Fields[0].Values; !slices.Equal(got, wantTimes) {
		t.Fatalf("job a times = %v, want %v", got, wantTimes)
	}
	if got := q.Exemplars[0].Fields[1].Values; !slices.Equal(got, wantValues) {
		t.Fatalf("job a values = %v, want %v", got, wantValues)
	}
	if got := q.Exemplars[0].Fields[2].Values; got[0] != fmt.Sprintf("trace-%d", at(3, 5*time.Second).UnixMilli()) {
		t.Fatalf("job a trace_id = %v, want the trace of the kept exemplar", got)
	}
	if q.Exemplars[1].Labels["job"] != "b" {
		t.Fatalf("second frame = %v, want job b kept apart from job a", q.Exemplars[1].Labels)
	}
}

func TestRunPanelCapsExemplarsAtTheNewest2000(t *testing.T) {
	const step = 2 * time.Second
	from := testRange().From
	build := func(n int) []datasource.Frame {
		var evens, odds []time.Time
		var evenValues, oddValues []any
		for i := 0; i < n; i++ {
			at := from.Add(time.Duration(i) * step)
			if i%2 == 0 {
				evens, evenValues = append(evens, at), append(evenValues, 1.0)
			} else {
				odds, oddValues = append(odds, at), append(oddValues, 1.0)
			}
		}
		return []datasource.Frame{
			exemplarFrameOf(map[string]string{"parity": "even"}, evens, evenValues),
			exemplarFrameOf(map[string]string{"parity": "odd"}, odds, oddValues),
		}
	}
	tests := []struct {
		name       string
		count      int
		wantStatus string
		wantOldest time.Time
	}{
		{"2000 fit", 2000, ExemplarsOK, from},
		{"2001 truncate to the newest", 2001, ExemplarsTruncated, from.Add(step)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onExemplar = func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
				return build(tt.count), nil
			}
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			req := exemplarRequest(exemplarPanel(spec.Query{Query: "up", Exemplar: true}))
			req.MaxDataPoints = 11000
			res, err := runner.RunPanel(t.Context(), req)
			if err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			q := res.Queries[0]
			if q.Step != step {
				t.Fatalf("step = %v, want %v for this test's buckets", q.Step, step)
			}
			total, oldest := 0, time.Time{}
			for _, f := range q.Exemplars {
				for _, v := range f.Fields[0].Values {
					total++
					if at := v.(time.Time); oldest.IsZero() || at.Before(oldest) {
						oldest = at
					}
				}
			}
			if q.ExemplarStatus != tt.wantStatus || total != 2000 || !oldest.Equal(tt.wantOldest) {
				t.Fatalf("status %q with %d exemplars from %v, want %q with 2000 from %v", q.ExemplarStatus, total, oldest, tt.wantStatus, tt.wantOldest)
			}
		})
	}
}

func TestRunPanelHoldsTheUpstreamSlotDuringTheExemplarCall(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	var allFree bool
	fake.onExemplar = func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		if allFree = runner.sem.TryAcquire(maxUpstreamCalls); allFree {
			runner.sem.Release(maxUpstreamCalls)
		}
		return nil, nil
	}
	if _, err := runner.RunPanel(t.Context(), exemplarRequest(exemplarPanel(spec.Query{Query: "up", Exemplar: true}))); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if allFree {
		t.Fatal("every upstream slot was free during the exemplars call, want the query's slot held")
	}
}
