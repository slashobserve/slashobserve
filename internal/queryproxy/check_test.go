package queryproxy

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type wantIssue struct {
	path    string
	code    spec.IssueCode
	message string
}

func assertIssues(t *testing.T, got []spec.Issue, want []wantIssue) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("issues = %+v, want %d issues %+v", got, len(want), want)
	}
	for i, w := range want {
		if got[i].Path != w.path || got[i].Code != w.code {
			t.Fatalf("issue[%d] = {%s %s}, want {%s %s}; all: %+v", i, got[i].Path, got[i].Code, w.path, w.code, got)
		}
		if w.message != "" && got[i].Message != w.message {
			t.Fatalf("issue[%d] message = %q, want %q", i, got[i].Message, w.message)
		}
		if got[i].Message == "" {
			t.Fatalf("issue[%d] has empty message", i)
		}
	}
}

func TestCheckSpecIssues(t *testing.T) {
	badRange := func(from, to string) func(*spec.Dashboard) {
		return func(d *spec.Dashboard) { d.Time = spec.TimeRange{From: from, To: to} }
	}
	emptyFrames := func(frames []datasource.Frame) func(*fakeDatasource) {
		return func(f *fakeDatasource) {
			f.onRange = func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
				return frames, nil
			}
		}
	}
	failRange := func(err error) func(*fakeDatasource) {
		return func(f *fakeDatasource) { f.onRange = failWith(err) }
	}
	failVariable := func(err error) func(*fakeDatasource) {
		return func(f *fakeDatasource) {
			f.onVariable = func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) { return nil, err }
		}
	}
	oneTimeseries := func(query string) []spec.Panel { return []spec.Panel{panelOf("p0", spec.Timeseries, "prom", query)} }

	tests := []struct {
		name      string
		panels    []spec.Panel
		variables []spec.Variable
		mutate    func(*spec.Dashboard)
		setup     func(*fakeDatasource)
		want      []wantIssue
		wantCalls int
	}{
		{
			name:      "clean dashboard",
			panels:    []spec.Panel{panelOf("p0", spec.Timeseries, "prom", "up"), panelOf("p1", spec.Stat, "prom", "up")},
			wantCalls: 2,
		},
		{
			name:   "unknown panel datasource",
			panels: []spec.Panel{panelOf("p0", spec.Timeseries, "missing", "up")},
			want:   []wantIssue{{"/panels/0/datasource", CodeUnknownDatasource, `datasource "missing" is not configured`}},
		},
		{
			name:   "macro of another datasource type",
			panels: []spec.Panel{panelOf("p0", spec.Timeseries, "prom", "up[$__timeFilter(ts)]")},
			want:   []wantIssue{{"/panels/0/queries/0/query", spec.CodeUnknownMacro, "macro $__timeFilter is not available for prometheus queries"}},
		},
		{
			name: "query level datasource is checked at the query",
			panels: []spec.Panel{{
				ID: "p0", Type: spec.Timeseries, Datasource: "prom",
				Queries: []spec.Query{{Query: "up", Datasource: "missing"}},
			}},
			want: []wantIssue{{"/panels/0/queries/0/datasource", CodeUnknownDatasource, `datasource "missing" is not configured`}},
		},
		{
			name: "row child runs and text panel does not",
			panels: []spec.Panel{
				{ID: "t", Type: spec.Text},
				{ID: "r", Type: spec.Row, Panels: []spec.Panel{panelOf("c", spec.Stat, "prom", "up")}},
			},
			wantCalls: 1,
		},
		{
			name:      "unknown variable datasource blocks dependent panel only",
			panels:    []spec.Panel{panelOf("p0", spec.Timeseries, "prom", `up{job="$job"}`), panelOf("p1", spec.Timeseries, "prom", "up")},
			variables: []spec.Variable{queryVariable("job", "missing", "label_values(job)")},
			want: []wantIssue{
				{"/panels/0/queries/0/query", CodeVariableUnresolved, `variable "job" could not be resolved`},
				{"/variables/0/datasource", CodeUnknownDatasource, ""},
			},
			wantCalls: 1,
		},
		{
			name:      "empty result",
			panels:    oneTimeseries("up"),
			setup:     emptyFrames(nil),
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeQueryEmpty, emptyPanelMessage}},
			wantCalls: 1,
		},
		{
			name:   "only time field is empty",
			panels: oneTimeseries("up"),
			setup: emptyFrames([]datasource.Frame{{Fields: []datasource.Field{
				{Name: "time", Type: datasource.FieldTime, Values: []any{1.0}},
			}}}),
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeQueryEmpty, emptyPanelMessage}},
			wantCalls: 1,
		},
		{
			name:   "all values nil is empty",
			panels: oneTimeseries("up"),
			setup: emptyFrames([]datasource.Frame{{Fields: []datasource.Field{
				{Name: "time", Type: datasource.FieldTime, Values: []any{1.0, 2.0}},
				{Name: "value", Type: datasource.FieldNumber, Values: []any{nil, nil}},
			}}}),
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeQueryEmpty, emptyPanelMessage}},
			wantCalls: 1,
		},
		{
			name:   "non-time field with a value is not empty",
			panels: oneTimeseries("up"),
			setup: emptyFrames([]datasource.Frame{{Fields: []datasource.Field{
				{Name: "label", Type: datasource.FieldString, Values: []any{"a"}},
			}}}),
			wantCalls: 1,
		},
		{
			name:      "query invalid",
			panels:    oneTimeseries("up{"),
			setup:     failRange(datasource.Errorf(datasource.ErrInvalidQuery, "parse error")),
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeQueryInvalid, "invalid query: parse error"}},
			wantCalls: 1,
		},
		{
			name:      "query forbidden",
			panels:    oneTimeseries("DROP"),
			setup:     failRange(datasource.Errorf(datasource.ErrForbiddenQuery, "only SELECT")),
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeQueryForbidden, "query not allowed: only SELECT"}},
			wantCalls: 1,
		},
		{
			name:      "limit exceeded",
			panels:    oneTimeseries("up"),
			setup:     failRange(datasource.Errorf(datasource.ErrLimitExceeded, "too many series")),
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeLimitExceeded, "query limit exceeded: too many series"}},
			wantCalls: 1,
		},
		{
			name:      "query timeout from datasource",
			panels:    oneTimeseries("up"),
			setup:     failRange(datasource.Errorf(datasource.ErrTimeout, "after 15s")),
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeQueryTimeout, "query timed out: after 15s"}},
			wantCalls: 1,
		},
		{
			name:      "query timeout from context deadline",
			panels:    oneTimeseries("up"),
			setup:     failRange(context.DeadlineExceeded),
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeQueryTimeout, "query timed out"}},
			wantCalls: 1,
		},
		{
			name:      "datasource unavailable",
			panels:    oneTimeseries("up"),
			setup:     failRange(datasource.Errorf(datasource.ErrUnavailable, "refused")),
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeDatasourceDown, "datasource unavailable: refused"}},
			wantCalls: 1,
		},
		{
			name:      "unexpected error is hidden",
			panels:    oneTimeseries("up"),
			setup:     failRange(errors.New("password=hunter2")),
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeInternal, "internal error"}},
			wantCalls: 1,
		},
		{
			name:   "panicking datasource is hidden",
			panels: oneTimeseries("up"),
			setup: func(f *fakeDatasource) {
				f.onRange = func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
					panic("hunter2")
				}
			},
			want:      []wantIssue{{"/panels/0/queries/0/query", CodeInternal, "internal error"}},
			wantCalls: 1,
		},
		{
			name:      "variable failure maps to variable_failed",
			panels:    oneTimeseries("up"),
			variables: []spec.Variable{queryVariable("job", "prom", "label_values(job)")},
			setup:     failVariable(datasource.Errorf(datasource.ErrUnavailable, "down")),
			want:      []wantIssue{{"/variables/0", CodeVariableFailed, "datasource unavailable: down"}},
			wantCalls: 2,
		},
		{
			name:      "variable invalid query still variable_failed",
			panels:    oneTimeseries("up"),
			variables: []spec.Variable{queryVariable("job", "prom", "bad")},
			setup:     failVariable(datasource.Errorf(datasource.ErrInvalidQuery, "no label_values")),
			want:      []wantIssue{{"/variables/0", CodeVariableFailed, "invalid query: no label_values"}},
			wantCalls: 2,
		},
		{
			name:      "variable timeout is variable_failed",
			panels:    oneTimeseries("up"),
			variables: []spec.Variable{queryVariable("job", "prom", "x")},
			setup:     failVariable(context.DeadlineExceeded),
			want:      []wantIssue{{"/variables/0", CodeVariableFailed, "query timed out"}},
			wantCalls: 2,
		},
		{
			name:      "variable internal error is variable_failed",
			panels:    oneTimeseries("up"),
			variables: []spec.Variable{queryVariable("job", "prom", "x")},
			setup:     failVariable(errors.New("hunter2")),
			want:      []wantIssue{{"/variables/0", CodeVariableFailed, "internal error"}},
			wantCalls: 2,
		},
		{
			name:      "variable resolves to nothing",
			panels:    oneTimeseries("up"),
			variables: []spec.Variable{queryVariable("job", "prom", "x")},
			setup: func(f *fakeDatasource) {
				f.onVariable = func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) { return nil, nil }
			},
			want:      []wantIssue{{"/variables/0", CodeVariableEmpty, `variable has no values: "job"`}},
			wantCalls: 2,
		},
		{
			name:      "custom variable with no options is empty",
			panels:    oneTimeseries("up"),
			variables: []spec.Variable{customVariable("env")},
			want:      []wantIssue{{"/variables/0", CodeVariableEmpty, `variable has no values: "env"`}},
			wantCalls: 1,
		},
		{
			name:   "failed variable cascades to later variables and panels",
			panels: []spec.Panel{panelOf("p0", spec.Timeseries, "prom", `up{job="$a"}`), panelOf("p1", spec.Timeseries, "prom", "up")},
			variables: []spec.Variable{
				queryVariable("a", "prom", "x"),
				queryVariable("b", "prom", "label_values($a, pod)"),
				customVariable("c", "ok"),
			},
			setup: failVariable(datasource.Errorf(datasource.ErrUnavailable, "down")),
			want: []wantIssue{
				{"/panels/0/queries/0/query", CodeVariableUnresolved, `variable "a" could not be resolved`},
				{"/variables/0", CodeVariableFailed, ""},
				{"/variables/1", CodeVariableUnresolved, `variable "a" could not be resolved`},
			},
			wantCalls: 2,
		},
		{
			name:   "panel referencing variable that was only resolved transitively still runs",
			panels: []spec.Panel{panelOf("p0", spec.Timeseries, "prom", `up{pod="$b"}`)},
			variables: []spec.Variable{
				customVariable("a", "a1"),
				queryVariable("b", "prom", "label_values($a, pod)"),
			},
			wantCalls: 2,
		},
		{
			name:   "issues across panels are ordered by path",
			panels: []spec.Panel{panelOf("p0", spec.Timeseries, "prom", "up"), panelOf("p1", spec.Timeseries, "prom", "bad"), panelOf("p2", spec.Stat, "gone", "up")},
			setup: func(f *fakeDatasource) {
				f.onRange = func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
					if q.Text == "bad" {
						return nil, datasource.Errorf(datasource.ErrInvalidQuery, "nope")
					}
					return dataFrames(), nil
				}
			},
			want: []wantIssue{
				{"/panels/1/queries/0/query", CodeQueryInvalid, ""},
				{"/panels/2/datasource", CodeUnknownDatasource, ""},
			},
			wantCalls: 2,
		},
		{
			name:   "time range garbage",
			panels: oneTimeseries("up"),
			mutate: badRange("garbage", "now"),
			want:   []wantIssue{{"/time", CodeTimeRangeInvalid, `time.from "garbage" is not a valid time`}},
		},
		{
			name:   "time range to garbage",
			panels: oneTimeseries("up"),
			mutate: badRange("now-1h", "later"),
			want:   []wantIssue{{"/time", CodeTimeRangeInvalid, `time.to "later" is not a valid time`}},
		},
		{
			name:   "time range from after to",
			panels: oneTimeseries("up"),
			mutate: badRange("now", "now-1h"),
			want:   []wantIssue{{"/time", CodeTimeRangeInvalid, "time.from must be before time.to"}},
		},
		{
			name:      "time range invalid reports datasource issues and skips upstream work",
			panels:    []spec.Panel{panelOf("p0", spec.Timeseries, "missing", "up")},
			variables: []spec.Variable{queryVariable("v", "missing", "x")},
			mutate:    badRange("now", "now"),
			want: []wantIssue{
				{"/panels/0/datasource", CodeUnknownDatasource, ""},
				{"/time", CodeTimeRangeInvalid, ""},
				{"/variables/0/datasource", CodeUnknownDatasource, ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			if tt.setup != nil {
				tt.setup(fake)
			}
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			d := dashboardOf(tt.panels, tt.variables...)
			if tt.mutate != nil {
				tt.mutate(d)
			}
			issues, err := runner.CheckSpec(t.Context(), d, testNow)
			if err != nil {
				t.Fatalf("CheckSpec error: %v", err)
			}
			if len(tt.want) == 0 && issues != nil {
				t.Fatalf("issues = %+v, want nil", issues)
			}
			assertIssues(t, issues, tt.want)
			for _, i := range issues {
				if strings.Contains(i.Message, "hunter2") {
					t.Fatalf("issue message leaked detail: %+v", i)
				}
			}
			if got := fake.callCount(); got != tt.wantCalls {
				t.Fatalf("upstream calls = %d, want %d: %+v", got, tt.wantCalls, fake.recorded())
			}
		})
	}
}

func TestCheckSpecResolvesRangeFromNow(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{panelOf("p0", spec.Timeseries, "prom", "up")})
	if issues, err := runner.CheckSpec(t.Context(), d, testNow); err != nil || issues != nil {
		t.Fatalf("issues = %+v, err = %v", issues, err)
	}
	if got := fake.recorded()[0].rng; got != testRange() {
		t.Fatalf("range = %v, want %v", got, testRange())
	}
}

func TestCheckSpecPanelCallShape(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{
		panelOf("series", spec.Timeseries, "prom", "up"),
		panelOf("single", spec.Stat, "prom", "up"),
	})
	before := time.Now()
	if issues, err := runner.CheckSpec(t.Context(), d, testNow); err != nil || issues != nil {
		t.Fatalf("issues = %+v, err = %v", issues, err)
	}
	after := time.Now()

	ranges, instants := fake.callsTo("range"), fake.callsTo("instant")
	if len(ranges) != 1 || len(instants) != 1 {
		t.Fatalf("calls: %d range, %d instant", len(ranges), len(instants))
	}
	if ranges[0].step != 108*time.Second {
		t.Fatalf("step = %v, want 108s (maxDataPoints 200)", ranges[0].step)
	}
	for _, c := range []call{ranges[0], instants[0]} {
		if c.query.Limits != (datasource.Limits{Timeout: 15 * time.Second}) {
			t.Fatalf("%s limits = %+v, want only Timeout 15s", c.method, c.query.Limits)
		}
		if !c.hasDeadline || c.deadline.Before(before.Add(15*time.Second)) || c.deadline.After(after.Add(15*time.Second)) {
			t.Fatalf("%s deadline = %v, want 15s from the call (between %v and %v)", c.method, c.deadline, before, after)
		}
	}
}

func TestCheckSpecVariableCallShape(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{panelOf("p0", spec.Stat, "prom", "up")}, queryVariable("job", "prom", "label_values(job)"))
	before := time.Now()
	if issues, err := runner.CheckSpec(t.Context(), d, testNow); err != nil || issues != nil {
		t.Fatalf("issues = %+v, err = %v", issues, err)
	}
	after := time.Now()
	c := fake.callsTo("variable")[0]
	if c.query.Limits != (datasource.Limits{}) {
		t.Fatalf("variable limits = %+v, want zero", c.query.Limits)
	}
	if !c.hasDeadline || c.deadline.Before(before.Add(10*time.Second)) || c.deadline.After(after.Add(10*time.Second)) {
		t.Fatalf("variable deadline = %v, want 10s from the call", c.deadline)
	}
	if c.rng != testRange() {
		t.Fatalf("variable range = %v", c.rng)
	}
}

func TestCheckSpecPanelParallelismBound(t *testing.T) {
	fake := newFake("prometheus")
	gate := make(chan struct{})
	var once sync.Once
	open := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(open)
	fake.onRange = func(ctx context.Context, _ datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return dataFrames(), nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	panels := make([]spec.Panel, 10)
	for i := range panels {
		panels[i] = panelOf("p"+string(rune('a'+i)), spec.Timeseries, "prom", "up")
	}
	type checked struct {
		issues []spec.Issue
		err    error
	}
	done := make(chan checked, 1)
	go func() {
		issues, err := runner.CheckSpec(t.Context(), dashboardOf(panels), testNow)
		done <- checked{issues, err}
	}()

	waitFor(t, "4 concurrent panel checks", func() bool { return fake.inflightNow() == checkPanelParallelism })
	time.Sleep(50 * time.Millisecond)
	if got := fake.inflightNow(); got != checkPanelParallelism {
		t.Fatalf("inflight = %d while gated, want exactly %d", got, checkPanelParallelism)
	}
	open()
	select {
	case got := <-done:
		if got.err != nil || got.issues != nil {
			t.Fatalf("issues = %+v, err = %v", got.issues, got.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("CheckSpec did not finish")
	}
	if got := fake.peakInflight(); got != checkPanelParallelism {
		t.Fatalf("peak inflight = %d, want %d", got, checkPanelParallelism)
	}
	if got := fake.callCount(); got != len(panels) {
		t.Fatalf("calls = %d, want %d", got, len(panels))
	}
}

func TestCheckSpecVariablesResolveSequentially(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf(nil,
		queryVariable("a", "prom", "a"),
		queryVariable("b", "prom", "b"),
		queryVariable("c", "prom", "c"),
	)
	if issues, err := runner.CheckSpec(t.Context(), d, testNow); err != nil || issues != nil {
		t.Fatalf("issues = %+v, err = %v", issues, err)
	}
	if got := fake.peakInflight(); got != 1 {
		t.Fatalf("peak inflight = %d, want 1", got)
	}
	var order []string
	for _, c := range fake.recorded() {
		order = append(order, c.query.Text)
	}
	if !reflect.DeepEqual(order, []string{"a", "b", "c"}) {
		t.Fatalf("resolution order = %v", order)
	}
}

func TestCheckSpecVariableValuesNeverInterpolated(t *testing.T) {
	const hostile = `x"} or vector(1) # '; DROP TABLE t; --`
	fake := newFake("prometheus")
	fake.onVariable = func(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]string, error) {
		if q.Text == "label_values(env)" {
			return []string{hostile}, nil
		}
		return []string{"second"}, nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf(
		[]spec.Panel{
			panelOf("series", spec.Timeseries, "prom", `up{env="$env", pod="$pod"}`),
			panelOf("single", spec.Stat, "prom", `up{env="$env"}`),
		},
		queryVariable("env", "prom", "label_values(env)"),
		queryVariable("pod", "prom", "label_values(up{env=\"$env\"}, pod)"),
	)
	if issues, err := runner.CheckSpec(t.Context(), d, testNow); err != nil || issues != nil {
		t.Fatalf("issues = %+v, err = %v", issues, err)
	}
	wantText := map[string]string{
		"series": `up{env="$env", pod="$pod"}`,
		"single": `up{env="$env"}`,
	}
	calls := fake.recorded()
	if len(calls) != 4 {
		t.Fatalf("calls = %d, want 4", len(calls))
	}
	sawHostile := 0
	for _, c := range calls {
		if strings.Contains(c.query.Text, hostile) || strings.Contains(c.query.Text, "second") {
			t.Fatalf("variable value interpolated into Query.Text: %q", c.query.Text)
		}
		for _, v := range c.query.Variables {
			if v.Name == "env" {
				if !reflect.DeepEqual(v.Values, []string{hostile}) || v.All {
					t.Fatalf("env variable = %+v", v)
				}
				sawHostile++
			}
		}
		if c.method != "variable" {
			found := false
			for _, text := range wantText {
				found = found || c.query.Text == text
			}
			if !found {
				t.Fatalf("panel Query.Text = %q, want the Spec text unchanged", c.query.Text)
			}
		}
	}
	if sawHostile != 3 {
		t.Fatalf("env reached the datasource as a Variable %d times, want 3 (pod lookup, two panels)", sawHostile)
	}
	for _, c := range fake.callsTo("range") {
		want := []datasource.Variable{{Name: "env", Values: []string{hostile}}, {Name: "pod", Values: []string{"second"}}}
		if !equalVariables(c.query.Variables, want) {
			t.Fatalf("panel variables = %+v, want %+v", c.query.Variables, want)
		}
	}
}

func TestCheckSpecIncludeAllVariableIsPassedWithItsValues(t *testing.T) {
	fake := newFake("prometheus")
	fake.onVariable = func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
		return []string{"a", "b"}, nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	v := queryVariable("job", "prom", "label_values(job)")
	v.IncludeAll, v.Multi = true, true
	d := dashboardOf([]spec.Panel{panelOf("p0", spec.Timeseries, "prom", `up{job=~"$job"}`)}, v)
	if issues, err := runner.CheckSpec(t.Context(), d, testNow); err != nil || issues != nil {
		t.Fatalf("issues = %+v, err = %v", issues, err)
	}
	if got := len(fake.callsTo("variable")); got != 1 {
		t.Fatalf("variable lookups = %d, want 1 (memoized across variable check and panel)", got)
	}
	got := fake.callsTo("range")[0].query.Variables
	if !equalVariables(got, []datasource.Variable{{Name: "job", Values: []string{"a", "b"}, All: true}}) {
		t.Fatalf("variables = %+v", got)
	}
}

func TestCheckSpecIncludeAllVariableProblems(t *testing.T) {
	fake := newFake("prometheus")
	fake.onVariable = func(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]string, error) {
		switch q.Text {
		case "ghost":
			return nil, nil
		case "bad":
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "parse error")
		}
		return []string{"v1"}, nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	ghost, bad := queryVariable("ghost", "prom", "ghost"), queryVariable("bad", "prom", "bad")
	ghost.IncludeAll, bad.IncludeAll = true, true
	d := dashboardOf([]spec.Panel{
		panelOf("p0", spec.Timeseries, "prom", `up{job=~"$ghost"}`),
		panelOf("p1", spec.Timeseries, "prom", "up"),
	}, ghost, bad)
	issues, err := runner.CheckSpec(t.Context(), d, testNow)
	if err != nil {
		t.Fatalf("CheckSpec error: %v", err)
	}
	assertIssues(t, issues, []wantIssue{
		{"/panels/0/queries/0/query", CodeVariableUnresolved, ""},
		{"/variables/0", CodeVariableEmpty, ""},
		{"/variables/1", CodeVariableFailed, ""},
	})
	if got := len(fake.callsTo("range")); got != 1 {
		t.Fatalf("panel queries = %d, want 1 (only the panel that does not need a failed variable)", got)
	}
}

func TestCheckSpecBadCustomDefault(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{panelOf("p0", spec.Timeseries, "prom", "up$env")}, customVariable("env", "a\x00b"))
	issues, err := runner.CheckSpec(t.Context(), d, testNow)
	if err != nil {
		t.Fatalf("CheckSpec error: %v", err)
	}
	assertIssues(t, issues, []wantIssue{
		{"/panels/0/queries/0/query", CodeVariableUnresolved, ""},
		{"/variables/0", CodeVariableFailed, ""},
	})
	if fake.callCount() != 0 {
		t.Fatalf("upstream called %d times", fake.callCount())
	}
}

func TestCheckSpecCanceledDuringVariablePhase(t *testing.T) {
	fake := newFake("prometheus")
	fake.onVariable = func(ctx context.Context, _ datasource.Query, _ datasource.TimeRange) ([]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	d := dashboardOf([]spec.Panel{panelOf("p0", spec.Timeseries, "prom", "up$v")}, queryVariable("v", "prom", "x"))
	issues, err := runner.CheckSpec(ctx, d, testNow)
	if !errors.Is(err, context.Canceled) || issues != nil {
		t.Fatalf("issues = %+v, err = %v, want nil and context.Canceled", issues, err)
	}
}

func TestCheckSpecParentContextEnds(t *testing.T) {
	blockUntilDone := func(f *fakeDatasource) {
		f.onRange = func(ctx context.Context, _ datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}
	panels := []spec.Panel{panelOf("p0", spec.Timeseries, "prom", "up"), panelOf("p1", spec.Timeseries, "prom", "up")}

	t.Run("deadline", func(t *testing.T) {
		fake := newFake("prometheus")
		blockUntilDone(fake)
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		issues, err := runner.CheckSpec(ctx, dashboardOf(panels), testNow)
		if err != nil {
			t.Fatalf("CheckSpec error: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("CheckSpec took %v", elapsed)
		}
		assertIssues(t, issues, []wantIssue{
			{"/panels/0/queries/0/query", CodeQueryTimeout, "query timed out"},
			{"/panels/1/queries/0/query", CodeQueryTimeout, "query timed out"},
		})
		for _, c := range fake.recorded() {
			if d, _ := ctx.Deadline(); c.deadline.After(d) {
				t.Fatalf("panel deadline %v is later than the caller's %v", c.deadline, d)
			}
		}
	})

	t.Run("canceled", func(t *testing.T) {
		fake := newFake("prometheus")
		blockUntilDone(fake)
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		issues, err := runner.CheckSpec(ctx, dashboardOf(panels), testNow)
		if !errors.Is(err, context.Canceled) || issues != nil {
			t.Fatalf("issues = %+v, err = %v, want nil and context.Canceled", issues, err)
		}
	})

	t.Run("deadline during variable lookup", func(t *testing.T) {
		fake := newFake("prometheus")
		fake.onVariable = func(ctx context.Context, _ datasource.Query, _ datasource.TimeRange) ([]string, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		d := dashboardOf([]spec.Panel{panelOf("p0", spec.Timeseries, "prom", "up$v")}, queryVariable("v", "prom", "x"))
		issues, err := runner.CheckSpec(ctx, d, testNow)
		if err != nil {
			t.Fatalf("CheckSpec error: %v", err)
		}
		assertIssues(t, issues, []wantIssue{
			{"/panels/0/queries/0/query", CodeVariableUnresolved, ""},
			{"/variables/0", CodeVariableFailed, "query timed out"},
		})
	})
}

func TestCheckDatasources(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf(
		[]spec.Panel{
			panelOf("ok", spec.Timeseries, "prom", "up"),
			panelOf("wrong", spec.Stat, "prom", "$__timeFilter(x)"),
			panelOf("gone", spec.Stat, "missing", "x"),
		},
		customVariable("env", "a"),
		spec.Variable{Name: "noDatasource", Type: spec.QueryVariable, Query: "x"},
		spec.Variable{Name: "qv", Type: spec.QueryVariable, Datasource: "missing", Query: "x"},
	)
	assertIssues(t, runner.CheckDatasources(d), []wantIssue{
		{"/panels/1/queries/0/query", spec.CodeUnknownMacro, ""},
		{"/panels/2/datasource", CodeUnknownDatasource, ""},
		{"/variables/2/datasource", CodeUnknownDatasource, ""},
	})
	if fake.callCount() != 0 {
		t.Fatalf("CheckDatasources called upstream %d times", fake.callCount())
	}
	if got := runner.CheckDatasources(dashboardOf([]spec.Panel{panelOf("ok", spec.Stat, "prom", "up")})); got != nil {
		t.Fatalf("clean dashboard issues = %+v", got)
	}
}
