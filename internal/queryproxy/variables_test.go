package queryproxy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
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
			got, err := runner.Defaults(t.Context(), d, testRange(), tt.given)
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
	got, err := runner.Defaults(t.Context(), d, testRange(), given)
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
	got, err := runner.Defaults(t.Context(), d, testRange(), given)
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
		{
			"query variable on mismatched datasource",
			[]spec.Variable{{Name: "q", Type: spec.QueryVariable, Query: "x", Datasource: &spec.DatasourceRef{Type: spec.ClickHouse, UID: "prom"}}},
			[]string{"a"}, nil, ErrDatasourceTypeMismatch,
		},
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
			_, err := runner.Defaults(t.Context(), dashboardOf(nil, tt.variables...), testRange(), tt.given)
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
		got, err := runner.Defaults(t.Context(), dashboardOf(nil, v), testRange(), map[string]Selection{"v": sel})
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
			got, truncated, err := runner.VariableValues(t.Context(), d, "v", testRange(), nil)
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

	got, truncated, err := runner.VariableValues(t.Context(), d, "env", testRange(), nil)
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

	if _, _, err := runner.VariableValues(t.Context(), d, "nope", testRange(), nil); !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("unknown variable err = %v", err)
	}
	if _, _, err := runner.VariableValues(t.Context(), nil, "nope", testRange(), nil); !errors.Is(err, ErrUnknownVariable) {
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
	if _, _, err := runner.VariableValues(t.Context(), d, "b", testRange(), earlier); err != nil {
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
			_, _, err := runner.VariableValues(t.Context(), d, "b", testRange(), map[string]Selection{"a": tt.sel})
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
			_, _, err := runner.VariableValues(t.Context(), dashboardOf(nil, tt.v), "v", tt.rng, nil)
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
	got, truncated, err := runner.VariableValues(t.Context(), d, "v", testRange(), nil)
	if !errors.Is(err, datasource.ErrUnavailable) || got != nil || truncated {
		t.Fatalf("got %v truncated=%v err=%v", got, truncated, err)
	}

	fake.onVariable = func(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
		panic("kaboom-secret")
	}
	_, _, err = runner.VariableValues(t.Context(), d, "v", testRange(), nil)
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
		d := dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "prom", `up{env=~"$env"}`)}, v)
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
		d := dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "prom", "up")}, includeAllQuery("job", "prom", "label_values(job)"))
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
		d := dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "prom", `up{a=~"$job", b=~"${job:pipe}"}`)}, includeAllQuery("job", "prom", "label_values(job)"))
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
		if _, _, err := runner.VariableValues(t.Context(), d, "pod", testRange(), map[string]Selection{"env": {All: true}}); err != nil {
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
		d := dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "prom", `up{job=~"$job"}`)}, includeAllQuery("job", "prom", "label_values(job)"))
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
		ref := spec.DatasourceRef{Type: spec.ClickHouse, UID: "ch"}
		v := includeAllQuery("svc", "ch", "SELECT DISTINCT ServiceName FROM otel_logs")
		v.Datasource = &ref
		return spec.Panel{ID: "p1", Type: spec.Stat, Title: "p1", Datasource: ref, Query: query}, v
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
				d := dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "prom", `up{job=~"$job"}`)}, includeAllQuery("job", "prom", "label_values(job)"))
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
			d := dashboardOf([]spec.Panel{panelOf("p1", spec.Stat, "prom", "up")}, tt.variables...)
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
		_, _, err := runner.VariableValues(t.Context(), d, "b", testRange(), map[string]Selection{name: sel})
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("selection for %q: err = %v, want ErrInvalidRequest", name, err)
		}
	}
	if fake.callCount() != 0 {
		t.Fatalf("upstream called %d times", fake.callCount())
	}

	if _, _, err := runner.VariableValues(t.Context(), d, "b", testRange(), nil); err != nil {
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
			_, err := runner.Defaults(t.Context(), dashboardOf(nil, tt.v), testRange(), nil)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("err = %v, want ErrInvalidRequest", err)
			}
		})
	}
}
