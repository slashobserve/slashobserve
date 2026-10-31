package queryproxy

import (
	"errors"
	"reflect"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func withBase(v spec.Variable, base ...spec.AdhocFilter) spec.Variable {
	v.BaseFilters = base
	return v
}

func f(key, op, value string) datasource.AdhocFilter {
	return datasource.AdhocFilter{Key: key, Op: op, Value: value}
}

func scopedOr(key, op, value string, scope int) datasource.AdhocFilter {
	return datasource.AdhocFilter{Key: key, Op: op, Value: value, Condition: "or", Scope: scope}
}

func inScope(x datasource.AdhocFilter, scope int) datasource.AdhocFilter {
	x.Scope = scope
	return x
}

func TestBaseFiltersAreAppliedAlongsideTheSelectedFilters(t *testing.T) {
	base := []spec.AdhocFilter{{Key: "env", Operator: "=", Value: "prod"}}
	tests := []struct {
		name       string
		variable   spec.Variable
		selection  map[string]Selection
		wantFilter []datasource.AdhocFilter
	}{
		{
			"base filters apply with nothing selected",
			withBase(adhocVariable("f", "prom"), base...),
			map[string]Selection{"f": {}},
			[]datasource.AdhocFilter{f("env", "=", "prod")},
		},
		{
			"base filters come before the selected ones",
			withBase(adhocVariable("f", "prom"), base...),
			map[string]Selection{"f": {Values: []string{"job|=|api"}}},
			[]datasource.AdhocFilter{f("env", "=", "prod"), f("job", "=", "api")},
		},
		{
			"a variable without base filters is unchanged",
			adhocVariable("f", "prom"),
			map[string]Selection{"f": {Values: []string{"job|=|api"}}},
			[]datasource.AdhocFilter{f("job", "=", "api")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prom := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom})
			panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "prom", Queries: []spec.Query{{Query: "up"}}}
			d := dashboardOf([]spec.Panel{panel}, tt.variable)
			if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: tt.selection}); err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			if calls := prom.callsTo("instant"); len(calls) != 1 || !reflect.DeepEqual(calls[0].query.Filters, tt.wantFilter) {
				t.Fatalf("calls = %+v, want one with filters %+v", calls, tt.wantFilter)
			}
		})
	}
}

func TestOrInSelectedFiltersStaysInsideItsOwnScope(t *testing.T) {
	base := []spec.AdhocFilter{{Key: "env", Operator: "=", Value: "prod"}}
	tests := []struct {
		name      string
		variables []spec.Variable
		selection map[string]Selection
		want      []datasource.AdhocFilter
	}{
		{
			"base filters and an or group are separate scopes",
			[]spec.Variable{applyToAll(withBase(adhocVariable("f", "ch"), base...))},
			map[string]Selection{"f": {Values: []string{"a|=|1", "|b|=|2"}}},
			[]datasource.AdhocFilter{f("env", "=", "prod"), inScope(f("a", "=", "1"), 1), scopedOr("b", "=", "2", 1)},
		},
		{
			"an or group of one variable does not swallow the next variable",
			[]spec.Variable{applyToAll(adhocVariable("f", "ch")), applyToAll(adhocVariable("g", "ch"))},
			map[string]Selection{"f": {Values: []string{"a|=|1", "|b|=|2"}}, "g": {Values: []string{"c|=|3"}}},
			[]datasource.AdhocFilter{f("a", "=", "1"), scopedOr("b", "=", "2", 0), inScope(f("c", "=", "3"), 1)},
		},
		{
			"plain variables stay one flat list",
			[]spec.Variable{applyToAll(adhocVariable("f", "ch")), applyToAll(adhocVariable("g", "ch"))},
			map[string]Selection{"f": {Values: []string{"a|=|1"}}, "g": {Values: []string{"c|=|3"}}},
			[]datasource.AdhocFilter{f("a", "=", "1"), f("c", "=", "3")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := newFake("clickhouse")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": ch})
			panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "ch", Queries: []spec.Query{{Query: "SELECT 1"}}}
			d := dashboardOf([]spec.Panel{panel}, tt.variables...)
			if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: tt.selection}); err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			if got := filtersByQuery(t, ch)["SELECT 1"]; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filters = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestADefaultFilterWithOrIsSelectedAsAPrefixedText(t *testing.T) {
	saved := []spec.AdhocFilter{{Key: "a", Operator: "=", Value: "1"}, {Key: "b", Operator: "<", Value: "5", Condition: "or"}}
	runner := newTypesRunner(t)
	defaults, err := runner.Defaults(t.Context(), dashboardOf(nil, adhocWithDefault("f", "ch", saved...)), testRange(), nil, datasource.QueryUser{})
	want := Selection{Values: []string{"a|=|1", "|b|<|5"}}
	if err != nil || !reflect.DeepEqual(defaults["f"], want) {
		t.Fatalf("Defaults = %+v, %v, want %+v", defaults["f"], err, want)
	}
}

func TestAnExplicitReferenceReceivesBaseAndSelectedFilters(t *testing.T) {
	ch := newFake("clickhouse")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": ch})
	base := []spec.AdhocFilter{{Key: "env", Operator: "=", Value: "prod"}}
	panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "ch", Queries: []spec.Query{{Query: "SELECT 1 WHERE $f"}}}
	d := dashboardOf([]spec.Panel{panel}, withBase(adhocVariable("f", "ch"), base...))
	selections := map[string]Selection{"f": {Values: []string{"a|=|1", "|b|>|2"}}}
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: selections}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	calls := ch.callsTo("instant")
	if len(calls) != 1 || len(calls[0].query.Variables) != 1 {
		t.Fatalf("calls = %+v, want one call with one variable", calls)
	}
	want := []datasource.AdhocFilter{f("env", "=", "prod"), inScope(f("a", "=", "1"), 1), scopedOr("b", ">", "2", 1)}
	if got := calls[0].query.Variables[0].Filters; !reflect.DeepEqual(got, want) {
		t.Fatalf("variable filters = %+v, want %+v", got, want)
	}
	if len(calls[0].query.Filters) != 0 {
		t.Fatalf("query filters = %+v, want none because the query names the variable", calls[0].query.Filters)
	}
}

func TestBaseFiltersAreCheckedLikeSelectedOnes(t *testing.T) {
	tests := []struct {
		name string
		base spec.AdhocFilter
		ds   string
	}{
		{"a key Prometheus refuses", spec.AdhocFilter{Key: "Bad Key", Operator: "=", Value: "x"}, "prom"},
		{"a bad regular expression", spec.AdhocFilter{Key: "job", Operator: "=~", Value: "a("}, "prom"},
		{"an unknown operator", spec.AdhocFilter{Key: "job", Operator: "~", Value: "x"}, "prom"},
		{"a control character in the value", spec.AdhocFilter{Key: "job", Operator: "=", Value: "a\x01"}, "prom"},
		{"a ClickHouse key with a space", spec.AdhocFilter{Key: "Bad Key", Operator: "=", Value: "x"}, "ch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newTypesRunner(t)
			d := dashboardOf(nil, withBase(adhocVariable("f", tt.ds), tt.base))
			if _, err := runner.Defaults(t.Context(), d, testRange(), nil, datasource.QueryUser{}); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("err = %v, want ErrInvalidRequest", err)
			}
		})
	}
}
