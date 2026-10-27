package queryproxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func values(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("v%04d", i)
	}
	return out
}

func answer(vals ...string) func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
		return vals, nil
	}
}

func TestDefaults(t *testing.T) {
	includeAll := queryVariable("all", "prom", "label_values(job)")
	includeAll.IncludeAll = true
	customAll := customVariable("customAll", "x", "y")
	customAll.IncludeAll = true

	tests := []struct {
		name      string
		variables []spec.Variable
		answers   []string
		given     map[string]Selection
		want      map[string]Selection
		wantCalls int
	}{
		{
			name:      "no variables",
			variables: nil,
			want:      map[string]Selection{},
		},
		{
			name:      "includeAll selects All without upstream",
			variables: []spec.Variable{includeAll},
			answers:   []string{"never"},
			want:      map[string]Selection{"all": {All: true}},
		},
		{
			name:      "includeAll wins over custom options",
			variables: []spec.Variable{customAll},
			want:      map[string]Selection{"customAll": {All: true}},
		},
		{
			name:      "custom picks first option",
			variables: []spec.Variable{customVariable("env", "prod", "staging")},
			want:      map[string]Selection{"env": {Values: []string{"prod"}}},
		},
		{
			name:      "query picks first resolved value",
			variables: []spec.Variable{queryVariable("job", "prom", "label_values(job)")},
			answers:   []string{"api", "web"},
			want:      map[string]Selection{"job": {Values: []string{"api"}}},
			wantCalls: 1,
		},
		{
			name:      "given selection is kept and not resolved",
			variables: []spec.Variable{queryVariable("job", "prom", "label_values(job)")},
			answers:   []string{"api"},
			given:     map[string]Selection{"job": {Values: []string{"web"}}},
			want:      map[string]Selection{"job": {Values: []string{"web"}}},
		},
		{
			name: "mixed in Spec order",
			variables: []spec.Variable{
				customVariable("env", "prod"),
				queryVariable("job", "prom", "label_values(job)"),
				includeAll,
			},
			answers: []string{"api"},
			want: map[string]Selection{
				"env": {Values: []string{"prod"}},
				"job": {Values: []string{"api"}},
				"all": {All: true},
			},
			wantCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onVariable = answer(tt.answers...)
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			d := dashboardOf(nil, tt.variables...)
			got, err := runner.Defaults(t.Context(), d, testRange(), tt.given, datasource.QueryUser{})
			if err != nil {
				t.Fatalf("Defaults: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Defaults = %+v, want %+v", got, tt.want)
			}
			if fake.callCount() != tt.wantCalls {
				t.Fatalf("upstream calls = %d, want %d", fake.callCount(), tt.wantCalls)
			}
		})
	}
}

func TestDefaultsDoesNotMutateGiven(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	given := map[string]Selection{"a": {Values: []string{"a2"}}}
	d := dashboardOf(nil, customVariable("a", "a1", "a2"), customVariable("b", "b1"))
	got, err := runner.Defaults(t.Context(), d, testRange(), given, datasource.QueryUser{})
	if err != nil {
		t.Fatalf("Defaults: %v", err)
	}
	if len(given) != 1 || len(got) != 2 {
		t.Fatalf("given = %+v, result = %+v", given, got)
	}
}

func TestDefaultsPassesOnlyEarlierSelections(t *testing.T) {
	fake := newFake("prometheus")
	fake.onVariable = func(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]string, error) {
		return []string{"resolved-" + q.Text}, nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf(nil,
		customVariable("a", "a1", "a2"),
		queryVariable("b", "prom", "b"),
		queryVariable("c", "prom", "c"),
		queryVariable("d", "prom", "d"),
	)
	given := map[string]Selection{"c": {Values: []string{"c-given"}}}
	got, err := runner.Defaults(t.Context(), d, testRange(), given, datasource.QueryUser{})
	if err != nil {
		t.Fatalf("Defaults: %v", err)
	}
	wantSel := map[string]Selection{
		"a": {Values: []string{"a1"}},
		"b": {Values: []string{"resolved-b"}},
		"c": {Values: []string{"c-given"}},
		"d": {Values: []string{"resolved-d"}},
	}
	if !reflect.DeepEqual(got, wantSel) {
		t.Fatalf("Defaults = %+v, want %+v", got, wantSel)
	}

	byQuery := map[string][]datasource.Variable{}
	for _, c := range fake.recorded() {
		byQuery[c.query.Text] = c.query.Variables
	}
	want := map[string][]datasource.Variable{
		"b": {{Name: "a", Values: []string{"a1"}}},
		"d": {
			{Name: "a", Values: []string{"a1"}},
			{Name: "b", Values: []string{"resolved-b"}},
			{Name: "c", Values: []string{"c-given"}},
		},
	}
	if len(byQuery) != len(want) {
		t.Fatalf("upstream queries = %v, want only b and d", byQuery)
	}
	for q, vars := range want {
		if !equalVariables(byQuery[q], vars) {
			t.Fatalf("variables passed for %q = %+v, want %+v", q, byQuery[q], vars)
		}
	}
}

func TestDefaultsErrors(t *testing.T) {
	single := customVariable("single", "a", "b")
	allowAll := customVariable("allowAll", "a")
	allowAll.IncludeAll = true
	multi := customVariable("multi", "a", "b")
	multi.Multi = true

	tests := []struct {
		name      string
		variables []spec.Variable
		answers   []string
		given     map[string]Selection
		want      error
	}{
		{"given unknown variable", []spec.Variable{single}, nil, map[string]Selection{"nope": {Values: []string{"a"}}}, ErrUnknownVariable},
		{"All on variable without includeAll", []spec.Variable{single}, nil, map[string]Selection{"single": {All: true}}, ErrInvalidRequest},
		{"multiple values on single variable", []spec.Variable{single}, nil, map[string]Selection{"single": {Values: []string{"a", "b"}}}, ErrInvalidRequest},
		{"query variable resolves nothing", []spec.Variable{queryVariable("q", "prom", "x")}, nil, nil, ErrVariableEmpty},
		{"custom variable with no options", []spec.Variable{customVariable("c")}, nil, nil, ErrVariableEmpty},
		{"query variable on unknown datasource", []spec.Variable{queryVariable("q", "missing", "x")}, []string{"a"}, nil, ErrUnknownDatasource},
		{"query variable without datasource", []spec.Variable{{Name: "q", Type: spec.QueryVariable}}, []string{"a"}, nil, datasource.ErrInvalidQuery},
		{"interval variable with no options", []spec.Variable{{Name: "iv", Type: spec.IntervalVariable}}, nil, nil, ErrVariableEmpty},
		{
			"given control byte reaches a dependent variable",
			[]spec.Variable{multi, queryVariable("q", "prom", "x")}, []string{"a"},
			map[string]Selection{"multi": {Values: []string{"a\x00"}}}, ErrInvalidRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onVariable = answer(tt.answers...)
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			_, err := runner.Defaults(t.Context(), dashboardOf(nil, tt.variables...), testRange(), tt.given, datasource.QueryUser{})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestDefaultsAcceptsShapeThatIncludeAllAndMultiAllow(t *testing.T) {
	v := customVariable("v", "a", "b")
	v.IncludeAll, v.Multi = true, true
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": newFake("prometheus")})
	for _, sel := range []Selection{{All: true}, {Values: []string{"a", "b"}}} {
		got, err := runner.Defaults(t.Context(), dashboardOf(nil, v), testRange(), map[string]Selection{"v": sel}, datasource.QueryUser{})
		if err != nil {
			t.Fatalf("Defaults(%+v): %v", sel, err)
		}
		if !reflect.DeepEqual(got["v"], sel) {
			t.Fatalf("selection = %+v, want %+v", got["v"], sel)
		}
	}
}

func TestVariableValues(t *testing.T) {
	tests := []struct {
		name          string
		answers       []string
		wantLen       int
		wantTruncated bool
	}{
		{"empty", nil, 0, false},
		{"few", []string{"a", "b"}, 2, false},
		{"exactly 1000", values(1000), 1000, false},
		{"1001 truncates", values(1001), 1000, true},
		{"1500 truncates", values(1500), 1000, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onVariable = answer(tt.answers...)
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			d := dashboardOf(nil, queryVariable("v", "prom", "label_values(job)"))
			got, truncated, err := runner.VariableValues(t.Context(), d, "v", testRange(), nil, datasource.QueryUser{})
			if err != nil {
				t.Fatalf("VariableValues: %v", err)
			}
			if len(got) != tt.wantLen || truncated != tt.wantTruncated {
				t.Fatalf("len=%d truncated=%v, want len=%d truncated=%v", len(got), truncated, tt.wantLen, tt.wantTruncated)
			}
			if tt.wantLen > 0 && !reflect.DeepEqual(got, tt.answers[:tt.wantLen]) {
				t.Fatal("values are not the first 1000 in upstream order")
			}
		})
	}
}

func TestVariableValuesCustomAndUnknown(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	options := []string{"a", "b"}
	d := dashboardOf(nil, customVariable("env", options...))

	got, truncated, err := runner.VariableValues(t.Context(), d, "env", testRange(), nil, datasource.QueryUser{})
	if err != nil || truncated || !reflect.DeepEqual(got, options) {
		t.Fatalf("got %v truncated=%v err=%v", got, truncated, err)
	}
	got[0] = "mutated"
	if d.Variables[0].Options[0] != "a" {
		t.Fatal("returned slice aliases the Spec options")
	}
	if fake.callCount() != 0 {
		t.Fatal("custom variable reached upstream")
	}

	if _, _, err := runner.VariableValues(t.Context(), d, "nope", testRange(), nil, datasource.QueryUser{}); !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("unknown variable err = %v", err)
	}
	if _, _, err := runner.VariableValues(t.Context(), nil, "nope", testRange(), nil, datasource.QueryUser{}); !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("nil dashboard err = %v", err)
	}
}

func TestVariableValuesPassesOnlyEarlierVariables(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf(nil,
		customVariable("a", "a1"),
		queryVariable("b", "prom", "label_values(job)"),
		customVariable("c", "c1"),
	)
	earlier := map[string]Selection{"a": {Values: []string{"a1"}}}
	if _, _, err := runner.VariableValues(t.Context(), d, "b", testRange(), earlier, datasource.QueryUser{}); err != nil {
		t.Fatalf("VariableValues: %v", err)
	}
	got := fake.recorded()[0].query
	if got.Text != "label_values(job)" || !equalVariables(got.Variables, []datasource.Variable{{Name: "a", Values: []string{"a1"}}}) {
		t.Fatalf("query = %+v, want only variable a", got)
	}
	if got.Limits != (datasource.Limits{}) {
		t.Fatalf("limits = %+v, want zero", got.Limits)
	}
}

func TestVariableValuesSelectionValidation(t *testing.T) {
	tooMany := make([]string, 101)
	for i := range tooMany {
		tooMany[i] = "v"
	}
	tests := []struct {
		name string
		sel  Selection
	}{
		{"control byte", Selection{Values: []string{"a\x00b"}}},
		{"delete", Selection{Values: []string{"a\x7f"}}},
		{"101 values", Selection{Values: tooMany}},
		{"1025 bytes", Selection{Values: []string{strings.Repeat("x", 1025)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			a := queryVariable("a", "prom", "label_values(a)")
			a.Multi = true
			d := dashboardOf(nil, a, queryVariable("b", "prom", "label_values($a, job)"))
			_, _, err := runner.VariableValues(t.Context(), d, "b", testRange(), map[string]Selection{"a": tt.sel}, datasource.QueryUser{})
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("err = %v, want ErrInvalidRequest", err)
			}
			if fake.callCount() != 0 {
				t.Fatalf("upstream called %d times", fake.callCount())
			}
		})
	}
}

func TestVariableValuesErrors(t *testing.T) {
	tests := []struct {
		name string
		v    spec.Variable
		rng  datasource.TimeRange
		want error
	}{
		{"no datasource", spec.Variable{Name: "v", Type: spec.QueryVariable}, testRange(), datasource.ErrInvalidQuery},
		{"unknown datasource", queryVariable("v", "missing", "x"), testRange(), ErrUnknownDatasource},
		{"invalid range", queryVariable("v", "prom", "x"), datasource.TimeRange{}, ErrInvalidRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			_, _, err := runner.VariableValues(t.Context(), dashboardOf(nil, tt.v), "v", tt.rng, nil, datasource.QueryUser{})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if fake.callCount() != 0 {
				t.Fatalf("upstream called %d times", fake.callCount())
			}
		})
	}
}

func TestVariableValuesUpstreamErrorAndPanic(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf(nil, queryVariable("v", "prom", "x"))

	fake.onVariable = func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
		return []string{"partial"}, datasource.Errorf(datasource.ErrUnavailable, "down")
	}
	got, truncated, err := runner.VariableValues(t.Context(), d, "v", testRange(), nil, datasource.QueryUser{})
	if !errors.Is(err, datasource.ErrUnavailable) || got != nil || truncated {
		t.Fatalf("got %v truncated=%v err=%v", got, truncated, err)
	}

	fake.onVariable = func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
		panic("kaboom-secret")
	}
	_, _, err = runner.VariableValues(t.Context(), d, "v", testRange(), nil, datasource.QueryUser{})
	status, code, message := Classify(err)
	if status != 500 || code != "internal" || message != "internal error" || strings.Contains(err.Error(), "kaboom-secret") {
		t.Fatalf("err = %v, Classify = %d %q %q", err, status, code, message)
	}
}

func includeAllQuery(name, uid, query string) spec.Variable {
	v := queryVariable(name, uid, query)
	v.IncludeAll, v.Multi = true, true
	return v
}

func runAll(t *testing.T, runner *Runner, d *spec.Dashboard, panelID string, selections map[string]Selection) error {
	t.Helper()
	_, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: panelID, Range: testRange(), Selections: selections})
	return err
}

func TestRunPanelAllExpandsToFullValueList(t *testing.T) {
	kib := strings.Repeat("a", 1024)
	repeated := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = kib
		}
		return out
	}
	tests := []struct {
		name       string
		answers    []string
		wantValues []string
		wantErr    error
	}{
		{"few values", []string{"a", "b"}, []string{"a", "b"}, nil},
		{"exactly 1000 values", values(1000), values(1000), nil},
		{"1001 values is unlistable", values(1001), nil, nil},
		{"exactly 64 KiB", repeated(64), repeated(64), nil},
		{"over 64 KiB is unlistable", repeated(65), nil, nil},
		{"value over 1024 bytes is unlistable", []string{"ok", kib + "a"}, nil, nil},
		{"control byte is unlistable", []string{"ok", "a\nb"}, nil, nil},
		{"delete byte is unlistable", []string{"ok", "a\x7fb"}, nil, nil},
		{"no values", nil, nil, ErrVariableEmpty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onVariable = answer(tt.answers...)
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			d := dashboardOf([]spec.Panel{panelOf("p1", spec.Timeseries, "prom", `up{job=~"$job"}`)}, includeAllQuery("job", "prom", "label_values(job)"))
			err := runAll(t, runner, d, "p1", map[string]Selection{"job": {All: true}})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if fake.callsTo("range") != nil {
					t.Fatal("panel query ran although the All list was empty")
				}
				return
			}
			if err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			got := fake.callsTo("range")[0].query.Variables
			if !equalVariables(got, []datasource.Variable{{Name: "job", Values: tt.wantValues, All: true}}) {
				t.Fatalf("variables = %d entries, first = %+v", len(got), got[0].All)
			}
		})
	}
}

func TestRunPanelAllListSources(t *testing.T) {
	t.Run("custom variable uses its options without upstream", func(t *testing.T) {
		fake := newFake("prometheus")
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
		v := customVariable("env", "prod", "staging")
		v.IncludeAll, v.Multi = true, true
		d := dashboardOf([]spec.Panel{panelOf("p1", spec.Table, "prom", `up{env=~"$env"}`)}, v)
		if err := runAll(t, runner, d, "p1", map[string]Selection{"env": {All: true}}); err != nil {
			t.Fatalf("RunPanel: %v", err)
		}
		if fake.callsTo("variable") != nil {
			t.Fatal("custom variable reached upstream")
		}
		got := fake.callsTo("instant")[0].query.Variables
		if !equalVariables(got, []datasource.Variable{{Name: "env", Values: []string{"prod", "staging"}, All: true}}) {
			t.Fatalf("variables = %+v", got)
		}
	})

	t.Run("only variables the query references are resolved", func(t *testing.T) {
		fake := newFake("prometheus")
		fake.onVariable = answer("a", "b")
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
		d := dashboardOf([]spec.Panel{panelOf("p1", spec.Table, "prom", "up")}, includeAllQuery("job", "prom", "label_values(job)"))
		if err := runAll(t, runner, d, "p1", map[string]Selection{"job": {All: true}}); err != nil {
			t.Fatalf("RunPanel: %v", err)
		}
		if fake.callsTo("variable") != nil {
			t.Fatal("unreferenced All variable was resolved")
		}
		got := fake.callsTo("instant")[0].query.Variables
		if !equalVariables(got, []datasource.Variable{{Name: "job", All: true}}) {
			t.Fatalf("variables = %+v", got)
		}
	})

	t.Run("a list is resolved once per call", func(t *testing.T) {
		fake := newFake("prometheus")
		fake.onVariable = answer("a", "b")
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
		d := dashboardOf([]spec.Panel{panelOf("p1", spec.Table, "prom", `up{a=~"$job", b=~"${job:pipe}"}`)}, includeAllQuery("job", "prom", "label_values(job)"))
		if err := runAll(t, runner, d, "p1", map[string]Selection{"job": {All: true}}); err != nil {
			t.Fatalf("RunPanel: %v", err)
		}
		if got := len(fake.callsTo("variable")); got != 1 {
			t.Fatalf("variable lookups = %d, want 1", got)
		}
	})

	t.Run("a later variable query sees the earlier All list", func(t *testing.T) {
		fake := newFake("prometheus")
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
		env := customVariable("env", "prod", "staging")
		env.IncludeAll, env.Multi = true, true
		d := dashboardOf(nil, env, queryVariable("pod", "prom", `label_values(up{env=~"$env"}, pod)`))
		if _, _, err := runner.VariableValues(t.Context(), d, "pod", testRange(), map[string]Selection{"env": {All: true}}, datasource.QueryUser{}); err != nil {
			t.Fatalf("VariableValues: %v", err)
		}
		got := fake.callsTo("variable")[0].query.Variables
		if !equalVariables(got, []datasource.Variable{{Name: "env", Values: []string{"prod", "staging"}, All: true}}) {
			t.Fatalf("variables = %+v", got)
		}
	})

	t.Run("variable lookups finish before the semaphore is taken for the panel", func(t *testing.T) {
		fake := newFake("prometheus")
		fake.onVariable = answer("a", "b")
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
		runner.sem = semaphore.NewWeighted(1)
		d := dashboardOf([]spec.Panel{panelOf("p1", spec.Table, "prom", `up{job=~"$job"}`)}, includeAllQuery("job", "prom", "label_values(job)"))
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		_, err := runner.RunPanel(ctx, Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: map[string]Selection{"job": {All: true}}})
		if err != nil {
			t.Fatalf("RunPanel with a one-slot semaphore: %v", err)
		}
	})
}

func TestRunPanelAllThroughRealMacros(t *testing.T) {
	clickHouse := func(query string) (spec.Panel, spec.Variable) {
		v := includeAllQuery("svc", "ch", "SELECT DISTINCT ServiceName FROM otel_logs")
		return spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "ch", Queries: []spec.Query{{Query: query}}}, v
	}
	tests := []struct {
		name    string
		query   string
		answers []string
		want    string
		wantErr bool
	}{
		{"default sqlstring in code", "SELECT count() FROM t WHERE ServiceName IN (${svc})", []string{"cart", "it's"}, "SELECT count() FROM t WHERE ServiceName IN ('cart','it''s')", false},
		{"explicit sqlstring", "SELECT count() FROM t WHERE ServiceName IN (${svc:sqlstring})", []string{"cart", "pay"}, "SELECT count() FROM t WHERE ServiceName IN ('cart','pay')", false},
		{"singlequote", "SELECT count() FROM t WHERE ServiceName IN (${svc:singlequote})", []string{"cart", "pay"}, "SELECT count() FROM t WHERE ServiceName IN ('cart','pay')", false},
		{"csv of safe tokens", "SELECT count() FROM t WHERE ServiceName IN (${svc:csv})", []string{"cart", "pay"}, "SELECT count() FROM t WHERE ServiceName IN (cart,pay)", false},
		{"sqlstring with an unlistable list fails closed", "SELECT count() FROM t WHERE ServiceName IN (${svc})", values(1001), "", true},
		{"csv with a value that cannot be a token fails closed", "SELECT count() FROM t WHERE ServiceName IN (${svc:csv})", []string{"cart", "it's"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("clickhouse")
			fake.onVariable = answer(tt.answers...)
			var expanded string
			fake.onInstant = func(_ context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
				exp, err := macros.Expand(macros.ClickHouse, q.Text, macros.Env{Range: r, Interval: time.Minute, Variables: q.Variables})
				if err != nil {
					return nil, err
				}
				expanded = exp.Text
				return dataFrames(), nil
			}
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": fake})
			panel, v := clickHouse(tt.query)
			err := runAll(t, runner, dashboardOf([]spec.Panel{panel}, v), "p1", map[string]Selection{"svc": {All: true}})
			if tt.wantErr {
				if !errors.Is(err, datasource.ErrInvalidQuery) {
					t.Fatalf("err = %v, want ErrInvalidQuery", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			if expanded != tt.want {
				t.Fatalf("expanded = %q, want %q", expanded, tt.want)
			}
		})
	}

	t.Run("promql regex matcher is dot star whether or not the list is listable", func(t *testing.T) {
		for name, answers := range map[string][]string{"listable": {"a", "b"}, "unlistable": values(1001)} {
			t.Run(name, func(t *testing.T) {
				fake := newFake("prometheus")
				fake.onVariable = answer(answers...)
				var expanded string
				fake.onInstant = func(_ context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
					exp, err := macros.Expand(macros.PromQL, q.Text, macros.Env{Range: r, Interval: time.Minute, Variables: q.Variables})
					if err != nil {
						return nil, err
					}
					expanded = exp.Text
					return dataFrames(), nil
				}
				runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
				d := dashboardOf([]spec.Panel{panelOf("p1", spec.Table, "prom", `up{job=~"$job"}`)}, includeAllQuery("job", "prom", "label_values(job)"))
				if err := runAll(t, runner, d, "p1", map[string]Selection{"job": {All: true}}); err != nil {
					t.Fatalf("RunPanel: %v", err)
				}
				if expanded != `up{job=~".*"}` {
					t.Fatalf("expanded = %q", expanded)
				}
			})
		}
	})
}

func TestRunPanelSelectionRules(t *testing.T) {
	multi := customVariable("env", "prod", "staging")
	multi.Multi = true
	tests := []struct {
		name       string
		variables  []spec.Variable
		selections map[string]Selection
		want       error
	}{
		{"missing selection", []spec.Variable{multi}, nil, ErrInvalidRequest},
		{"empty selection", []spec.Variable{multi}, map[string]Selection{"env": {}}, ErrInvalidRequest},
		{"undeclared name", []spec.Variable{multi}, map[string]Selection{"env": {Values: []string{"prod"}}, "nope": {Values: []string{"x"}}}, ErrUnknownVariable},
		{"custom value outside the options", []spec.Variable{multi}, map[string]Selection{"env": {Values: []string{"dev"}}}, ErrInvalidRequest},
		{"one value outside the options among valid ones", []spec.Variable{multi}, map[string]Selection{"env": {Values: []string{"prod", "dev"}}}, ErrInvalidRequest},
		{"All on a variable without includeAll", []spec.Variable{multi}, map[string]Selection{"env": {All: true}}, ErrInvalidRequest},
		{"All combined with a value", []spec.Variable{includeAllQuery("job", "prom", "x")}, map[string]Selection{"job": {All: true, Values: []string{"a"}}}, ErrInvalidRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			d := dashboardOf([]spec.Panel{panelOf("p1", spec.Table, "prom", "up")}, tt.variables...)
			err := runAll(t, runner, d, "p1", tt.selections)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if fake.callCount() != 0 {
				t.Fatalf("upstream called %d times", fake.callCount())
			}
		})
	}
}

func TestVariableValuesSelectionsMustBeEarlier(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf(nil,
		customVariable("a", "a1"),
		queryVariable("b", "prom", "label_values(job)"),
		customVariable("c", "c1"),
	)
	for name, sel := range map[string]Selection{"b": {Values: []string{"x"}}, "c": {Values: []string{"c1"}}} {
		_, _, err := runner.VariableValues(t.Context(), d, "b", testRange(), map[string]Selection{name: sel}, datasource.QueryUser{})
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("selection for %q: err = %v, want ErrInvalidRequest", name, err)
		}
	}
	if fake.callCount() != 0 {
		t.Fatalf("upstream called %d times", fake.callCount())
	}

	if _, _, err := runner.VariableValues(t.Context(), d, "b", testRange(), nil, datasource.QueryUser{}); err != nil {
		t.Fatalf("VariableValues without earlier selections: %v", err)
	}
	got := fake.callsTo("variable")[0].query.Variables
	if !equalVariables(got, []datasource.Variable{{Name: "a", Values: []string{"a1"}}}) {
		t.Fatalf("missing earlier selection was not defaulted: %+v", got)
	}
}

func TestDefaultsRejectsBadDefaults(t *testing.T) {
	tests := []struct {
		name string
		v    spec.Variable
	}{
		{"control byte in first option", customVariable("v", "a\x00b", "ok")},
		{"first option over 1024 bytes", customVariable("v", strings.Repeat("a", 1025))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": newFake("prometheus")})
			_, err := runner.Defaults(t.Context(), dashboardOf(nil, tt.v), testRange(), nil, datasource.QueryUser{})
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("err = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestRunPanelConditionalAllThroughRealMacros(t *testing.T) {
	const query = "SELECT count() FROM t WHERE $__conditionalAll(svc IN (${svc:singlequote}), $svc)"
	tests := []struct {
		name          string
		selection     Selection
		want          string
		wantListCalls int
	}{
		{"All", Selection{All: true}, "SELECT count() FROM t WHERE 1=1", 0},
		{"one value", Selection{Values: []string{"cart"}}, "SELECT count() FROM t WHERE (svc IN ('cart'))", 0},
		{"two values", Selection{Values: []string{"cart", "it's"}}, `SELECT count() FROM t WHERE (svc IN ('cart','it\'s'))`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("clickhouse")
			fake.onVariable = answer("cart", "pay")
			var expanded string
			fake.onInstant = func(_ context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
				exp, err := macros.Expand(macros.ClickHouse, q.Text, macros.Env{Range: r, Interval: time.Minute, Variables: q.Variables})
				if err != nil {
					return nil, err
				}
				expanded = exp.Text
				return dataFrames(), nil
			}
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": fake})
			panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "ch", Queries: []spec.Query{{Query: query}}}
			d := dashboardOf([]spec.Panel{panel}, includeAllQuery("svc", "ch", "SELECT DISTINCT ServiceName FROM otel_logs"))
			if err := runAll(t, runner, d, "p1", map[string]Selection{"svc": tt.selection}); err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			if expanded != tt.want {
				t.Fatalf("expanded = %q, want %q", expanded, tt.want)
			}
			if got := len(fake.callsTo("variable")); got != tt.wantListCalls {
				t.Fatalf("variable list calls = %d, want %d", got, tt.wantListCalls)
			}
		})
	}
}

func TestRunPanelConditionalAllGuardOnlyNeedsNoAllList(t *testing.T) {
	failing := func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "down")
	}
	tests := []struct {
		name          string
		query         string
		onVariable    func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error)
		want          string
		wantErr       bool
		wantListCalls int
	}{
		{"failing list", "SELECT 1 WHERE $__conditionalAll(x > 1, $svc)", failing, "SELECT 1 WHERE 1=1", false, 0},
		{"empty list", "SELECT 1 WHERE $__conditionalAll(x > 1, ${svc})", answer(), "SELECT 1 WHERE 1=1", false, 0},
		{"two conditions on the same guard", "SELECT 1 WHERE $__conditionalAll(x > 1, $svc) AND $__conditionalAll(y > 1, $svc)", failing, "SELECT 1 WHERE 1=1 AND 1=1", false, 0},
		{"guard also used in the condition", "SELECT 1 WHERE $__conditionalAll(svc IN (${svc:singlequote}), $svc)", failing, "SELECT 1 WHERE 1=1", false, 0},
		{"guard also used outside the condition", "SELECT ${svc:singlequote} WHERE $__conditionalAll(x > 1, $svc)", failing, "", true, 1},
		{"guard in a nested position of another condition", "SELECT 1 WHERE $__conditionalAll(x IN (${svc:singlequote}), $other) AND $__conditionalAll(y > 1, $svc)", failing, "", true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("clickhouse")
			fake.onVariable = tt.onVariable
			var expanded string
			fake.onInstant = func(_ context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
				exp, err := macros.Expand(macros.ClickHouse, q.Text, macros.Env{Range: r, Interval: time.Minute, Variables: q.Variables})
				if err != nil {
					return nil, err
				}
				expanded = exp.Text
				return dataFrames(), nil
			}
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": fake})
			panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "ch", Queries: []spec.Query{{Query: tt.query}}}
			other := customVariable("other", "a")
			other.IncludeAll = true
			d := dashboardOf([]spec.Panel{panel}, includeAllQuery("svc", "ch", "SELECT DISTINCT ServiceName FROM otel_logs"), other)
			err := runAll(t, runner, d, "p1", map[string]Selection{"svc": {All: true}, "other": {All: true}})
			if (err != nil) != tt.wantErr {
				t.Fatalf("RunPanel error = %v, want error %v", err, tt.wantErr)
			}
			if expanded != tt.want {
				t.Fatalf("expanded = %q, want %q", expanded, tt.want)
			}
			if got := len(fake.callsTo("variable")); got != tt.wantListCalls {
				t.Fatalf("variable list calls = %d, want %d", got, tt.wantListCalls)
			}
		})
	}
}

func TestQueryUseRecordsTheGuardOfAReferenceWithoutNestedReferences(t *testing.T) {
	use := queryUse{variables: map[string]bool{}, listed: map[string]bool{}}
	use.add(spec.Reference{Kind: spec.MacroRef, Name: "__conditionalAll", HasArgs: true, Guard: "svc"}, "")
	if !use.variables["svc"] || use.listed["svc"] {
		t.Fatalf("variables = %v, listed = %v, want svc used and not listed", use.variables, use.listed)
	}
}

func constantVariable(name, value string) spec.Variable {
	return spec.Variable{Name: name, Type: spec.ConstantVariable, Value: value}
}

func intervalVariable(name string, options ...string) spec.Variable {
	return spec.Variable{Name: name, Type: spec.IntervalVariable, Options: options}
}

func textboxVariable(name string) spec.Variable {
	return spec.Variable{Name: name, Type: spec.TextboxVariable}
}

func datasourceVariable(name string, typ spec.DatasourceType) spec.Variable {
	return spec.Variable{Name: name, Type: spec.DatasourceVariable, DatasourceType: typ}
}

func adhocVariable(name, uid string) spec.Variable {
	return spec.Variable{Name: name, Type: spec.AdhocVariable, Datasource: uid}
}

func withDefault(v spec.Variable, values ...string) spec.Variable {
	v.Default = &spec.Selection{Values: values, List: len(values) > 1}
	return v
}

func newTypesRunner(t *testing.T) *Runner {
	t.Helper()
	return newTestRunner(t, discardLogger(), map[string]*fakeDatasource{
		"prom":  newFake("prometheus"),
		"prom2": newFake("prometheus"),
		"ch":    newFake("clickhouse"),
	})
}

func TestNewVariableTypeValuesAndDefaults(t *testing.T) {
	auto := intervalVariable("iv", "1m", "5m")
	auto.Auto = true
	pickable := queryVariable("q", "prom", "label_values(job)")
	pickable.IncludeAll, pickable.Multi = true, true
	filtered := datasourceVariable("dsf", spec.Prometheus)
	filtered.Regex = "/^prom2$/"
	captured := datasourceVariable("dsc", spec.Prometheus)
	captured.Regex = "/^(.*)2$/"

	tests := []struct {
		name        string
		variable    spec.Variable
		wantValues  []string
		wantDefault Selection
		wantErr     error
	}{
		{"constant", constantVariable("c", "shop"), []string{"shop"}, Selection{Values: []string{"shop"}}, nil},
		{"interval without auto", intervalVariable("iv", "1m", "5m"), []string{"1m", "5m"}, Selection{Values: []string{"1m"}}, nil},
		{"interval with auto lists it first and defaults to it", auto, []string{"$__auto", "1m", "5m"}, Selection{Values: []string{"$__auto"}}, nil},
		{"interval with auto and a default keeps the default", withDefault(auto, "5m"), []string{"$__auto", "1m", "5m"}, Selection{Values: []string{"5m"}}, nil},
		{"interval with auto and no options defaults to auto", spec.Variable{Name: "iv", Type: spec.IntervalVariable, Auto: true}, []string{"$__auto"}, Selection{Values: []string{"$__auto"}}, nil},
		{"interval default", withDefault(intervalVariable("iv", "1m", "5m"), "5m"), []string{"1m", "5m"}, Selection{Values: []string{"5m"}}, nil},
		{"textbox lists nothing and defaults to an empty string", textboxVariable("tb"), []string{}, Selection{Values: []string{""}}, nil},
		{"textbox default", withDefault(textboxVariable("tb"), "hello"), []string{}, Selection{Values: []string{"hello"}}, nil},
		{"datasource lists the configured uids of its type", datasourceVariable("ds", spec.Prometheus), []string{"prom", "prom2"}, Selection{Values: []string{"prom"}}, nil},
		{"datasource of another type", datasourceVariable("ds", spec.ClickHouse), []string{"ch"}, Selection{Values: []string{"ch"}}, nil},
		{"datasource default", withDefault(datasourceVariable("ds", spec.Prometheus), "prom2"), []string{"prom", "prom2"}, Selection{Values: []string{"prom2"}}, nil},
		{"datasource default that is not a value", withDefault(datasourceVariable("ds", spec.Prometheus), "ch"), []string{"prom", "prom2"}, Selection{}, ErrVariableEmpty},
		{"datasource regex", filtered, []string{"prom2"}, Selection{Values: []string{"prom2"}}, nil},
		{"datasource regex only filters and never rewrites a uid through a capture group", captured, []string{"prom2"}, Selection{Values: []string{"prom2"}}, nil},
		{"datasource with nothing configured", datasourceVariable("ds", spec.DatasourceType("loki")), []string{}, Selection{}, ErrVariableEmpty},
		{"adhoc has no list and no filters by default", adhocVariable("f", "prom"), nil, Selection{}, nil},
		{"query default as one value", withDefault(pickable, "web"), nil, Selection{Values: []string{"web"}}, nil},
		{"query default as an array", withDefault(pickable, "web", "api"), nil, Selection{Values: []string{"web", "api"}}, nil},
		{"query default All", withDefault(pickable, "$__all"), nil, Selection{All: true}, nil},
		{"custom default", withDefault(customVariable("cu", "a", "b"), "b"), []string{"a", "b"}, Selection{Values: []string{"b"}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newTypesRunner(t)
			d := dashboardOf(nil, tt.variable)
			if tt.variable.Type == spec.AdhocVariable {
				_, _, err := runner.VariableValues(t.Context(), d, tt.variable.Name, testRange(), nil, datasource.QueryUser{})
				if !errors.Is(err, ErrInvalidRequest) || !strings.Contains(err.Error(), "has no value list") {
					t.Fatalf("VariableValues err = %v, want invalid request: no value list", err)
				}
			} else if tt.wantValues != nil {
				got, _, err := runner.VariableValues(t.Context(), d, tt.variable.Name, testRange(), nil, datasource.QueryUser{})
				if err != nil || !reflect.DeepEqual(got, tt.wantValues) {
					t.Fatalf("VariableValues = %q, %v, want %q", got, err, tt.wantValues)
				}
			}
			defaults, err := runner.Defaults(t.Context(), d, testRange(), nil, datasource.QueryUser{})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Defaults err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Defaults: %v", err)
			}
			if !reflect.DeepEqual(defaults[tt.variable.Name], tt.wantDefault) {
				t.Fatalf("default = %+v, want %+v", defaults[tt.variable.Name], tt.wantDefault)
			}
		})
	}
}

func TestNewVariableTypeSelections(t *testing.T) {
	auto := intervalVariable("iv", "1m", "5m")
	auto.Auto = true
	allConstant := constantVariable("c", "shop")
	allConstant.IncludeAll = true
	one := func(values ...string) Selection { return Selection{Values: values} }
	repeat := func(n int) Selection { return one(slices.Repeat([]string{"job|=|a"}, n)...) }

	tests := []struct {
		name     string
		variable spec.Variable
		sel      Selection
		ok       bool
	}{
		{"constant its value", constantVariable("c", "shop"), one("shop"), true},
		{"constant longer than a selection value may be", constantVariable("c", strings.Repeat("a", 1500)), one(strings.Repeat("a", 1500)), true},
		{"constant another value", constantVariable("c", "shop"), one("other"), false},
		{"constant All", allConstant, Selection{All: true}, false},
		{"constant two values", constantVariable("c", "shop"), one("shop", "shop"), false},
		{"interval an option", auto, one("5m"), true},
		{"interval auto", auto, one("$__auto"), true},
		{"interval auto when not enabled", intervalVariable("iv", "1m"), one("$__auto"), false},
		{"interval not an option", auto, one("10m"), false},
		{"interval two values", auto, one("1m", "5m"), false},
		{"textbox a value", textboxVariable("tb"), one("hello world"), true},
		{"textbox an empty value", textboxVariable("tb"), one(""), true},
		{"textbox two values", textboxVariable("tb"), one("a", "b"), false},
		{"textbox no value", textboxVariable("tb"), Selection{}, false},
		{"textbox a control character", textboxVariable("tb"), one("a\nb"), false},
		{"textbox a long value", textboxVariable("tb"), one(strings.Repeat("a", 1025)), false},
		{"datasource a listed uid", datasourceVariable("ds", spec.Prometheus), one("prom2"), true},
		{"datasource a uid of another type", datasourceVariable("ds", spec.Prometheus), one("ch"), false},
		{"datasource an unknown uid", datasourceVariable("ds", spec.Prometheus), one("nope"), false},
		{"adhoc no filters", adhocVariable("f", "prom"), Selection{}, true},
		{"adhoc equal", adhocVariable("f", "prom"), one("job|=|api"), true},
		{"adhoc all operators", adhocVariable("f", "prom"), one("job|=|a", "job|!=|b", "job|=~|c.*", "job|!~|d"), true},
		{"adhoc value with a pipe", adhocVariable("f", "prom"), one("job|=|a|b"), true},
		{"adhoc empty value", adhocVariable("f", "prom"), one("job|=|"), true},
		{"adhoc clickhouse column", adhocVariable("f", "ch"), one("LogAttributes.http|=|x"), true},
		{"adhoc prometheus key with a dot", adhocVariable("f", "prom"), one("a.b|=|x"), false},
		{"adhoc key with a space", adhocVariable("f", "prom"), one("Bad Key|=|x"), false},
		{"adhoc clickhouse key with a space", adhocVariable("f", "ch"), one("Bad Key|=|x"), false},
		{"adhoc operator", adhocVariable("f", "prom"), one("job|<|x"), false},
		{"adhoc missing parts", adhocVariable("f", "prom"), one("job|=x"), false},
		{"adhoc bad regex", adhocVariable("f", "prom"), one("job|=~|a("), false},
		{"adhoc control character", adhocVariable("f", "prom"), one("job|=|a\x01"), false},
		{"adhoc long value", adhocVariable("f", "prom"), one("job|=|" + strings.Repeat("a", 1025)), false},
		{"adhoc All", adhocVariable("f", "prom"), Selection{All: true}, false},
		{"adhoc twenty filters", adhocVariable("f", "prom"), repeat(20), true},
		{"adhoc twenty one filters", adhocVariable("f", "prom"), repeat(21), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newTypesRunner(t)
			_, err := runner.Defaults(t.Context(), dashboardOf(nil, tt.variable), testRange(), map[string]Selection{tt.variable.Name: tt.sel}, datasource.QueryUser{})
			if tt.ok && err != nil {
				t.Fatalf("Defaults: %v", err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("err = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestAdhocErrorsNeverEchoTheFilter(t *testing.T) {
	runner := newTypesRunner(t)
	secret := "super-secret-value"
	_, err := runner.Defaults(t.Context(), dashboardOf(nil, adhocVariable("f", "prom")), testRange(), map[string]Selection{"f": {Values: []string{"job|=~|" + secret + "("}}}, datasource.QueryUser{})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("err = %v, want an error without the value", err)
	}
}

func TestQueryVariableRegexAndSort(t *testing.T) {
	tests := []struct {
		name    string
		regex   string
		sort    string
		answers []string
		want    []string
	}{
		{"neither", "", "", []string{"b", "a"}, []string{"b", "a"}},
		{"regex keeps matches", "/^web-/", "", []string{"web-1", "api-1", "web-2"}, []string{"web-1", "web-2"}},
		{"regex first capture group", "/^web-(.*)$/", "", []string{"web-1", "api-1", "web-2"}, []string{"1", "2"}},
		{"regex named value group", "/^(?<other>\\w+)-(?<value>\\d+)$/", "", []string{"web-1", "api-2"}, []string{"1", "2"}},
		{"regex drops duplicates", "/^(\\w+)-\\d+$/", "", []string{"web-1", "web-2", "api-1"}, []string{"web", "api"}},
		{"alpha", "", "alpha", []string{"b", "c", "a"}, []string{"a", "b", "c"}},
		{"alphaDesc", "", "alphaDesc", []string{"b", "c", "a"}, []string{"c", "b", "a"}},
		{"num", "", "num", []string{"10", "9", "x"}, []string{"x", "9", "10"}},
		{"numDesc", "", "numDesc", []string{"10", "9", "x"}, []string{"10", "9", "x"}},
		{"regex then sort", "/^web-(.*)$/", "numDesc", []string{"web-1", "web-10", "api-3", "web-2"}, []string{"10", "2", "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onVariable = answer(tt.answers...)
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			v := queryVariable("v", "prom", "label_values(job)")
			v.Regex, v.Sort = tt.regex, tt.sort
			d := dashboardOf(nil, v)
			got, _, err := runner.VariableValues(t.Context(), d, "v", testRange(), nil, datasource.QueryUser{})
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("VariableValues = %q, %v, want %q", got, err, tt.want)
			}
			defaults, err := runner.Defaults(t.Context(), d, testRange(), nil, datasource.QueryUser{})
			if err != nil || !reflect.DeepEqual(defaults["v"], Selection{Values: tt.want[:1]}) {
				t.Fatalf("Defaults = %+v, %v, want first of %q", defaults["v"], err, tt.want)
			}
		})
	}

	t.Run("a regex that matches nothing leaves the variable empty", func(t *testing.T) {
		fake := newFake("prometheus")
		fake.onVariable = answer("a", "b")
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
		v := queryVariable("v", "prom", "label_values(job)")
		v.Regex = "/zzz/"
		if _, err := runner.Defaults(t.Context(), dashboardOf(nil, v), testRange(), nil, datasource.QueryUser{}); !errors.Is(err, ErrVariableEmpty) {
			t.Fatalf("err = %v, want ErrVariableEmpty", err)
		}
	})

	t.Run("the shared value cache keeps the raw list", func(t *testing.T) {
		fake := newFake("prometheus")
		fake.onVariable = answer("web-1", "api-1")
		reg, err := datasource.NewRegistry(discardLogger(), datasource.Instance{UID: "prom", Datasource: fake})
		if err != nil {
			t.Fatal(err)
		}
		runner := NewRunner(reg, discardLogger(), WithValueCache())
		filtered := queryVariable("a", "prom", "label_values(job)")
		filtered.Regex = "/^web-/"
		plain := queryVariable("b", "prom", "label_values(job)")
		d := dashboardOf(nil, filtered, plain)
		for name, want := range map[string][]string{"a": {"web-1"}, "b": {"web-1", "api-1"}} {
			got, _, err := runner.VariableValues(t.Context(), d, name, testRange(), nil, datasource.QueryUser{})
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("VariableValues(%s) = %q, %v, want %q", name, got, err, want)
			}
		}
		if n := fake.callCount(); n != 1 {
			t.Fatalf("upstream calls = %d, want 1", n)
		}
	})
}

func TestAutoInterval(t *testing.T) {
	at := func(d time.Duration) datasource.TimeRange {
		return datasource.TimeRange{From: testNow.Add(-d), To: testNow}
	}
	with := func(steps int, floor string) spec.Variable {
		return spec.Variable{AutoSteps: steps, AutoMin: floor}
	}
	const day = 24 * time.Hour
	tests := []struct {
		name string
		rng  datasource.TimeRange
		v    spec.Variable
		want time.Duration
	}{
		{"six hours rounds 12m up to 15m", at(6 * time.Hour), with(0, ""), 15 * time.Minute},
		{"autoMin raises it", at(6 * time.Hour), with(0, "30m"), 30 * time.Minute},
		{"short range is held to the default minimum", at(time.Minute), with(0, ""), 10 * time.Second},
		{"autoMin below the ladder step changes nothing", at(6 * time.Hour), with(0, "1s"), 15 * time.Minute},
		{"autoSteps", at(6 * time.Hour), with(60, ""), 10 * time.Minute},
		{"thirty days", at(30 * day), with(0, ""), day},
		{"a year", at(365 * day), with(0, ""), 30 * day},
		{"twenty years rounds the ladder's last step", at(20 * 365 * day), with(0, ""), 365 * day},
		{"beyond the ladder rounds to whole days", at(730 * day), with(1, ""), 730 * day},
		{"beyond the ladder keeps whole days only", at(730*day + time.Hour), with(1, ""), 731 * day},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := autoInterval(tt.rng, tt.v); got != tt.want {
				t.Fatalf("autoInterval = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunPanelNewVariableTypesReachTheQuery(t *testing.T) {
	auto := intervalVariable("iv", "1m", "5m")
	auto.Auto = true
	allValued := includeAllQuery("pod", "ch", "SELECT DISTINCT Pod FROM t")
	allValued.AllValue = "total"
	customAllValued := customVariable("env", "prod", "dev")
	customAllValued.IncludeAll, customAllValued.AllValue = true, "env-all"

	run := func(t *testing.T, query string, selections map[string]Selection, variables ...spec.Variable) (string, error) {
		t.Helper()
		fake := newFake("clickhouse")
		fake.onVariable = answer("a", "b")
		var expanded string
		fake.onInstant = func(_ context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
			exp, err := macros.Expand(macros.ClickHouse, q.Text, macros.Env{Range: r, Interval: time.Minute, Variables: q.Variables})
			if err != nil {
				return nil, err
			}
			expanded = exp.Text
			return dataFrames(), nil
		}
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": fake})
		panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "ch", Queries: []spec.Query{{Query: query}}}
		d := dashboardOf([]spec.Panel{panel}, variables...)
		_, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: selections})
		return expanded, err
	}

	t.Run("constant, textbox, interval and datasource values", func(t *testing.T) {
		query := "SELECT ${c:sqlstring}, ${tb:sqlstring}, ${iv:sqlstring}, ${ds:sqlstring}"
		got, err := run(t, query, map[string]Selection{
			"c":  {Values: []string{"shop"}},
			"tb": {Values: []string{"it's"}},
			"iv": {Values: []string{"5m"}},
			"ds": {Values: []string{"ch"}},
		}, constantVariable("c", "shop"), textboxVariable("tb"), auto, datasourceVariable("ds", spec.ClickHouse))
		if err != nil {
			t.Fatalf("RunPanel: %v", err)
		}
		if want := "SELECT 'shop', 'it''s', '5m', 'ch'"; got != want {
			t.Fatalf("expanded = %q, want %q", got, want)
		}
	})

	t.Run("interval auto is the formatted step of the range", func(t *testing.T) {
		got, err := run(t, "SELECT ${iv:sqlstring}", map[string]Selection{"iv": {Values: []string{"$__auto"}}}, auto)
		if err != nil {
			t.Fatalf("RunPanel: %v", err)
		}
		if want := "SELECT '15m'"; got != want {
			t.Fatalf("expanded = %q, want %q", got, want)
		}
	})

	t.Run("interval auto honors autoMin", func(t *testing.T) {
		v := auto
		v.AutoMin = "30m"
		got, err := run(t, "SELECT ${iv:sqlstring}", map[string]Selection{"iv": {Values: []string{"$__auto"}}}, v)
		if err != nil || got != "SELECT '30m'" {
			t.Fatalf("expanded = %q, %v", got, err)
		}
	})

	t.Run("All with allValue expands to it without a format", func(t *testing.T) {
		got, err := run(t, "SELECT 1 WHERE Pod = $pod", map[string]Selection{"pod": {All: true}}, allValued)
		if err != nil || got != "SELECT 1 WHERE Pod = total" {
			t.Fatalf("expanded = %q, %v", got, err)
		}
		got, err = run(t, "SELECT 1 WHERE Env = '$env'", map[string]Selection{"env": {All: true}}, customAllValued)
		if err != nil || got != "SELECT 1 WHERE Env = 'env-all'" {
			t.Fatalf("expanded = %q, %v", got, err)
		}
	})

	t.Run("All with allValue still has to be a safe token in code", func(t *testing.T) {
		v := allValued
		v.AllValue = "1 OR 1=1"
		if _, err := run(t, "SELECT 1 WHERE Pod = $pod", map[string]Selection{"pod": {All: true}}, v); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
	})

	t.Run("a ClickHouse adhoc reference expands", func(t *testing.T) {
		f := adhocVariable("f", "ch")
		got, err := run(t, "SELECT 1 WHERE $f", map[string]Selection{"f": {}}, f)
		if err != nil || got != "SELECT 1 WHERE 1 = 1" {
			t.Fatalf("expanded = %q, %v", got, err)
		}
		got, err = run(t, "SELECT 1 WHERE $f", map[string]Selection{"f": {Values: []string{"ServiceName|=|cart", "Body|!~|x|y"}}}, f)
		if err != nil || got != "SELECT 1 WHERE (ServiceName = 'cart' AND NOT match(Body, '^(?s:x|y)$'))" {
			t.Fatalf("expanded = %q, %v", got, err)
		}
	})

	t.Run("a variable query cannot reference an adhoc variable", func(t *testing.T) {
		f := adhocVariable("f", "ch")
		bad := queryVariable("q", "ch", "SELECT DISTINCT Pod FROM t WHERE $f")
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": newFake("clickhouse")})
		_, _, err := runner.VariableValues(t.Context(), dashboardOf(nil, f, bad), "q", testRange(), map[string]Selection{"f": {}}, datasource.QueryUser{})
		if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
	})
}

func TestAdhocFiltersApplyToPrometheusQueriesOfTheirDatasource(t *testing.T) {
	prom, other := newFake("prometheus"), newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom, "prom2": other})
	panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "prom", Queries: []spec.Query{{Query: "up"}, {Query: "up", Datasource: "prom2"}}}
	d := dashboardOf([]spec.Panel{panel}, adhocVariable("f", "prom"), adhocVariable("g", "prom"), adhocVariable("h", "prom2"))
	selections := map[string]Selection{
		"f": {Values: []string{"job|=|api", "pod|!~|web-.*"}},
		"g": {Values: []string{"env|!=|dev"}},
		"h": {},
	}
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: selections}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	want := []datasource.AdhocFilter{{Key: "job", Op: "=", Value: "api"}, {Key: "pod", Op: "!~", Value: "web-.*"}, {Key: "env", Op: "!=", Value: "dev"}}
	if calls := prom.callsTo("instant"); len(calls) != 1 || !reflect.DeepEqual(calls[0].query.Filters, want) {
		t.Fatalf("prom calls = %+v, want one with filters %+v", calls, want)
	}
	if calls := other.callsTo("instant"); len(calls) != 1 || len(calls[0].query.Filters) != 0 {
		t.Fatalf("prom2 calls = %+v, want one without filters", calls)
	}
}

func TestAdhocFiltersDoNotApplyToVariableQueries(t *testing.T) {
	prom := newFake("prometheus")
	prom.onVariable = answer("a")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom})
	d := dashboardOf(nil, adhocVariable("f", "prom"), queryVariable("q", "prom", "label_values(job)"))
	if _, _, err := runner.VariableValues(t.Context(), d, "q", testRange(), map[string]Selection{"f": {Values: []string{"job|=|api"}}}, datasource.QueryUser{}); err != nil {
		t.Fatalf("VariableValues: %v", err)
	}
	if calls := prom.callsTo("variable"); len(calls) != 1 || len(calls[0].query.Filters) != 0 {
		t.Fatalf("variable calls = %+v, want one without filters", calls)
	}
}

func TestAdhocFiltersAndDatasourceRegexesAreParsedOncePerRequest(t *testing.T) {
	runner := newTypesRunner(t)
	st := newVarState()
	adhoc := adhocVariable("f", "prom")
	sel := Selection{Values: []string{"job|=|api", "job|!=|web"}}
	first, err := runner.adhocFilters(st, adhoc, sel)
	if err != nil || len(first) != 2 {
		t.Fatalf("adhocFilters = %+v, %v", first, err)
	}
	second, err := runner.adhocFilters(st, adhoc, sel)
	if err != nil || &first[0] != &second[0] {
		t.Fatalf("a second lookup parsed the filters again: %v", err)
	}
	other, err := runner.adhocFilters(st, adhoc, Selection{Values: []string{"job|=|db"}})
	if err != nil || len(other) != 1 || other[0].Value != "db" {
		t.Fatalf("a different selection must not reuse the cached filters: %+v, %v", other, err)
	}

	a, err := st.parsed.regex("ds", "/^prom/")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := st.parsed.regex("ds", "/^prom/"); a != b {
		t.Fatal("a second lookup compiled the datasource regex again")
	}
}

func TestVariableRegexInterpolatesEarlierVariables(t *testing.T) {
	envOptions := customVariable("env", "a.b", "axb", "x)|(y")
	envOptions.IncludeAll, envOptions.Multi = true, true
	answers := []string{"a.b", "axb", "a.b-1", "x)|(y", "other"}
	tests := []struct {
		name    string
		regex   string
		earlier Selection
		want    []string
		wantErr error
	}{
		{"a value is matched literally", "/^$env$/", Selection{Values: []string{"a.b"}}, []string{"a.b"}, nil},
		{"braced reference with a suffix", "/^${env}-1$/", Selection{Values: []string{"a.b"}}, []string{"a.b-1"}, nil},
		{"regex metacharacters cannot inject a pattern", "/^$env$/", Selection{Values: []string{"x)|(y"}}, []string{"x)|(y"}, nil},
		{"several values become an alternation", "/^$env$/", Selection{Values: []string{"a.b", "axb"}}, []string{"a.b", "axb"}, nil},
		{"All matches anything", "/^$env$/", Selection{All: true}, answers, nil},
		{"a plain pattern is interpolated too", "$env-1", Selection{Values: []string{"a.b"}}, []string{"a.b-1"}, nil},
		{"a name that is not an earlier variable stays in the pattern", "/^other$later/", Selection{Values: []string{"a.b"}}, []string{}, nil},
		{"another format is refused", "/^${env:csv}$/", Selection{Values: []string{"a.b"}}, nil, datasource.ErrInvalidQuery},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onVariable = answer(answers...)
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			v := includeAllQuery("svc", "prom", "label_values(job)")
			v.Regex = tt.regex
			d := dashboardOf(nil, envOptions, v)
			got, _, err := runner.VariableValues(t.Context(), d, "svc", testRange(), map[string]Selection{"env": tt.earlier}, datasource.QueryUser{})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("VariableValues = %q, %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestVariableRegexUsesTheDefaultOfAnEarlierVariable(t *testing.T) {
	fake := newFake("prometheus")
	fake.onVariable = answer("web-1", "api-1", "web-2")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	v := queryVariable("svc", "prom", "label_values(job)")
	v.Regex = "/^$tier-/"
	d := dashboardOf(nil, customVariable("tier", "api", "web"), v)
	defaults, err := runner.Defaults(t.Context(), d, testRange(), nil, datasource.QueryUser{})
	if err != nil || !reflect.DeepEqual(defaults["svc"], Selection{Values: []string{"api-1"}}) {
		t.Fatalf("Defaults = %+v, %v, want api-1", defaults, err)
	}
}

func TestVariableRegexRefusesAnAdhocReference(t *testing.T) {
	fake := newFake("prometheus")
	fake.onVariable = answer("a")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	v := queryVariable("svc", "prom", "label_values(job)")
	v.Regex = "/$f/"
	d := dashboardOf(nil, adhocVariable("f", "prom"), v)
	if _, _, err := runner.VariableValues(t.Context(), d, "svc", testRange(), nil, datasource.QueryUser{}); err == nil {
		t.Fatal("VariableValues accepted an adhoc variable in a regex")
	}
}

func TestDatasourceVariableRegexInterpolatesEarlierVariables(t *testing.T) {
	pick := customVariable("pick", "prom", "prom2", "pr.m")
	ds := datasourceVariable("ds", spec.Prometheus)
	ds.Regex = "/^$pick$/"
	runner := newTypesRunner(t)
	d := dashboardOf(nil, pick, ds)
	values, _, err := runner.VariableValues(t.Context(), d, "ds", testRange(), map[string]Selection{"pick": {Values: []string{"prom2"}}}, datasource.QueryUser{})
	if err != nil || !reflect.DeepEqual(values, []string{"prom2"}) {
		t.Fatalf("VariableValues = %q, %v, want prom2", values, err)
	}
	values, _, err = runner.VariableValues(t.Context(), d, "ds", testRange(), map[string]Selection{"pick": {Values: []string{"pr.m"}}}, datasource.QueryUser{})
	if err != nil || len(values) != 0 {
		t.Fatalf("VariableValues = %q, %v, want nothing (pr.m is literal)", values, err)
	}
	if _, err := runner.Defaults(t.Context(), d, testRange(), map[string]Selection{"pick": {Values: []string{"prom"}}, "ds": {Values: []string{"prom2"}}}, datasource.QueryUser{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("a datasource its regex filters out: err = %v, want ErrInvalidRequest", err)
	}
	got, err := runner.Defaults(t.Context(), d, testRange(), map[string]Selection{"pick": {Values: []string{"prom"}}, "ds": {Values: []string{"prom"}}}, datasource.QueryUser{})
	if err != nil || !reflect.DeepEqual(got["ds"], Selection{Values: []string{"prom"}}) {
		t.Fatalf("Defaults = %+v, %v", got, err)
	}
}

type typeCountingDatasource struct {
	*fakeDatasource
	types atomic.Int64
}

func (d *typeCountingDatasource) Type() string {
	d.types.Add(1)
	return d.fakeDatasource.Type()
}

func TestChainedDatasourceVariableRegexesAreCheckedOncePerSelection(t *testing.T) {
	const chain = 20
	counting := &typeCountingDatasource{fakeDatasource: newFake("prometheus")}
	reg, err := datasource.NewRegistry(slog.New(slog.DiscardHandler), datasource.Instance{UID: "prom", Datasource: counting})
	if err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(reg, discardLogger())
	variables := make([]spec.Variable, chain)
	for i := range variables {
		variables[i] = datasourceVariable("d"+strconv.Itoa(i), spec.Prometheus)
		if i > 0 {
			variables[i].Regex = "/^$d" + strconv.Itoa(i-1) + "|.*$/"
		}
	}
	d := dashboardOf([]spec.Panel{panelOf("p0", spec.Timeseries, "prom", `up{job="$d19"}`)}, variables...)

	counting.types.Store(0)
	start := time.Now()
	defaults, err := runner.Defaults(t.Context(), d, testRange(), nil, datasource.QueryUser{})
	if err != nil || len(defaults) != chain {
		t.Fatalf("Defaults = %v, %v", defaults, err)
	}
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p0", Range: testRange(), Selections: defaults}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if n := counting.types.Load(); n > 50*chain {
		t.Fatalf("the datasource list was walked %d times for %d chained variables, want a count that grows with the chain and not exponentially", n, chain)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Defaults and RunPanel took %v", elapsed)
	}
}

func TestVariableRegexMatchesAnEarlierValueWithSlashesLiterally(t *testing.T) {
	fake := newFake("prometheus")
	fake.onVariable = answer("/prom/", "x", "/x/z", "prom")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	v := queryVariable("svc", "prom", "label_values(job)")
	v.Regex = "$env"
	d := dashboardOf(nil, customVariable("env", "/prom/", "/x/z"), v)
	for pick, want := range map[string][]string{"/prom/": {"/prom/"}, "/x/z": {"/x/z"}} {
		got, _, err := runner.VariableValues(t.Context(), d, "svc", testRange(), map[string]Selection{"env": {Values: []string{pick}}}, datasource.QueryUser{})
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("env=%q: VariableValues = %q, %v, want %q", pick, got, err, want)
		}
	}
}

func TestVariableRegexWithAnUnparsableReferenceFailsWithTheScanError(t *testing.T) {
	ds := datasourceVariable("ds", spec.Prometheus)
	ds.Regex = "/${a b}/"
	query := queryVariable("svc", "prom", "label_values(job)")
	query.Regex = "/^${a b}$/"
	for _, v := range []spec.Variable{ds, query} {
		t.Run(string(v.Type), func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onVariable = answer("a")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
			d := dashboardOf(nil, v)
			_, _, err := runner.VariableValues(t.Context(), d, v.Name, testRange(), nil, datasource.QueryUser{})
			if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "invalid reference") {
				t.Fatalf("VariableValues err = %v, want the invalid reference scan error", err)
			}
			if _, err := runner.Defaults(t.Context(), d, testRange(), nil, datasource.QueryUser{}); !errors.Is(err, datasource.ErrInvalidQuery) {
				t.Fatalf("Defaults err = %v, want the scan error", err)
			}
		})
	}
}
