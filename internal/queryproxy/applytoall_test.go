package queryproxy

import (
	"reflect"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func applyToAll(v spec.Variable) spec.Variable {
	v.ApplyToAll = true
	return v
}

func filtersByQuery(t *testing.T, fake *fakeDatasource) map[string][]datasource.AdhocFilter {
	t.Helper()
	out := map[string][]datasource.AdhocFilter{}
	for _, c := range fake.callsTo("instant") {
		out[c.query.Text] = c.query.Filters
	}
	return out
}

func TestApplyToAllFiltersEveryClickHouseQueryThatDoesNotUseTheVariable(t *testing.T) {
	selF := map[string]Selection{"f": {Values: []string{"ServiceName|=|cart"}}}
	selFG := map[string]Selection{
		"f": {Values: []string{"ServiceName|=|cart"}},
		"g": {Values: []string{"Body|!~|noise"}},
	}
	filtersF := []datasource.AdhocFilter{{Key: "ServiceName", Op: "=", Value: "cart"}}
	tests := []struct {
		name      string
		variables []spec.Variable
		selection map[string]Selection
		want      map[string][]datasource.AdhocFilter
	}{
		{
			"a query that does not name the variable is filtered, one that names it is not, another datasource is not",
			[]spec.Variable{applyToAll(adhocVariable("f", "ch"))},
			selF,
			map[string][]datasource.AdhocFilter{"SELECT 1": filtersF, "SELECT 2 WHERE $f": nil},
		},
		{
			"a variable without applyToAll filters nothing by itself",
			[]spec.Variable{adhocVariable("f", "ch")},
			selF,
			map[string][]datasource.AdhocFilter{"SELECT 1": nil, "SELECT 2 WHERE $f": nil},
		},
		{
			"only the variables that opt in contribute",
			[]spec.Variable{applyToAll(adhocVariable("f", "ch")), adhocVariable("g", "ch")},
			selFG,
			map[string][]datasource.AdhocFilter{"SELECT 1": filtersF, "SELECT 2 WHERE $f": nil},
		},
		{
			"a variable with no filters selected adds nothing",
			[]spec.Variable{applyToAll(adhocVariable("f", "ch"))},
			map[string]Selection{"f": {}},
			map[string][]datasource.AdhocFilter{"SELECT 1": nil, "SELECT 2 WHERE $f": nil},
		},
		{
			"a variable on another datasource adds nothing",
			[]spec.Variable{applyToAll(adhocVariable("f", "ch2"))},
			selF,
			map[string][]datasource.AdhocFilter{"SELECT 1": nil, "SELECT 2 WHERE $f": nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch, ch2 := newFake("clickhouse"), newFake("clickhouse")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": ch, "ch2": ch2})
			panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "ch", Queries: []spec.Query{{Query: "SELECT 1"}, {Query: "SELECT 2 WHERE $f"}}}
			d := dashboardOf([]spec.Panel{panel}, tt.variables...)
			if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: tt.selection}); err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			if got := filtersByQuery(t, ch); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filters by query = %+v, want %+v", got, tt.want)
			}
			if calls := ch2.callsTo("instant"); len(calls) != 0 {
				t.Fatalf("ch2 calls = %+v, want none", calls)
			}
		})
	}
}

func TestApplyToAllDoesNotReachQueriesOnAnotherDatasource(t *testing.T) {
	ch, other := newFake("clickhouse"), newFake("clickhouse")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": ch, "other": other})
	panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "ch", Queries: []spec.Query{{Query: "SELECT 1"}, {Query: "SELECT 3", Datasource: "other"}}}
	d := dashboardOf([]spec.Panel{panel}, applyToAll(adhocVariable("f", "ch")))
	selections := map[string]Selection{"f": {Values: []string{"ServiceName|=|cart"}}}
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: selections}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	want := []datasource.AdhocFilter{{Key: "ServiceName", Op: "=", Value: "cart"}}
	if got := filtersByQuery(t, ch); !reflect.DeepEqual(got, map[string][]datasource.AdhocFilter{"SELECT 1": want}) {
		t.Fatalf("ch filters = %+v, want the filter on SELECT 1", got)
	}
	if got := filtersByQuery(t, other); !reflect.DeepEqual(got, map[string][]datasource.AdhocFilter{"SELECT 3": nil}) {
		t.Fatalf("other filters = %+v, want none", got)
	}
}

func TestApplyToAllIsInertOnPrometheus(t *testing.T) {
	prom := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom})
	panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "prom", Queries: []spec.Query{{Query: "up"}}}
	d := dashboardOf([]spec.Panel{panel}, applyToAll(adhocVariable("f", "prom")))
	selections := map[string]Selection{"f": {Values: []string{"job|=|api"}}}
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: selections}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	want := []datasource.AdhocFilter{{Key: "job", Op: "=", Value: "api"}}
	if calls := prom.callsTo("instant"); len(calls) != 1 || !reflect.DeepEqual(calls[0].query.Filters, want) {
		t.Fatalf("calls = %+v, want one with filters %+v", calls, want)
	}
}
