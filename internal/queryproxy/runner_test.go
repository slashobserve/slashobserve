package queryproxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestKindFor(t *testing.T) {
	tests := []struct {
		panelType spec.PanelType
		want      Kind
	}{
		{spec.Timeseries, KindRange},
		{spec.Stat, KindInstant},
		{spec.Table, KindInstant},
		{spec.BarGauge, KindInstant},
		{spec.PanelType("unknown"), KindInstant},
	}
	for _, tt := range tests {
		t.Run(string(tt.panelType), func(t *testing.T) {
			if got := KindFor(tt.panelType); got != tt.want {
				t.Fatalf("KindFor(%q) = %q, want %q", tt.panelType, got, tt.want)
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
		{"stat uses instant", spec.Stat, 200, KindInstant, "instant", 0},
		{"table uses instant", spec.Table, 200, KindInstant, "instant", 0},
		{"bargauge uses instant", spec.BarGauge, 200, KindInstant, "instant", 0},
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
			if len(res.Frames) == 0 {
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
		{
			"datasource type mismatch",
			dashboardOf([]spec.Panel{{ID: "p1", Type: spec.Stat, Datasource: spec.DatasourceRef{Type: spec.ClickHouse, UID: "prom"}, Query: "up"}}),
			"p1", testRange(), ErrDatasourceTypeMismatch,
		},
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
	if res.Frames != nil || res.Kind != "" {
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
			wantStatus: statusClientGone,
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
	_, _, err := runner.VariableValues(ctx, d, "v", testRange(), nil)
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
	if _, _, err := runner.VariableValues(t.Context(), d, "job", testRange(), nil); err == nil {
		t.Fatal("VariableValues returned nil error")
	}
	if !strings.Contains(logs.String(), "subject=variable:job") {
		t.Fatalf("log = %q", logs.String())
	}
}
