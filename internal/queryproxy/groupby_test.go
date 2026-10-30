package queryproxy

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func groupByVariable(name, uid string, defaults ...string) spec.Variable {
	v := spec.Variable{Name: name, Type: spec.GroupByVariable, Datasource: uid}
	if len(defaults) > 0 {
		v.Default = &spec.Selection{Values: defaults, List: true}
	}
	return v
}

func groupByOfQueries(fake *fakeDatasource) map[string][]string {
	out := map[string][]string{}
	for _, c := range fake.callsTo("instant") {
		out[c.query.Text] = c.query.GroupBy
	}
	return out
}

func runGroupBy(t *testing.T, runner *Runner, panel spec.Panel, selections map[string]Selection, variables ...spec.Variable) error {
	t.Helper()
	d := dashboardOf([]spec.Panel{panel}, variables...)
	selections, err := runner.Defaults(t.Context(), d, testRange(), selections, datasource.QueryUser{})
	if err != nil {
		return err
	}
	_, err = runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: panel.ID, Range: testRange(), Selections: selections})
	return err
}

func TestGroupByKeysReachQueriesOnTheVariableDatasource(t *testing.T) {
	prom, other := newFake("prometheus"), newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom, "other": other})
	panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "prom", Queries: []spec.Query{{Query: "sum(a)"}, {Query: "sum(b)", Datasource: "other"}}}
	selections := map[string]Selection{"g": {Values: []string{"pod", "job"}}}
	if err := runGroupBy(t, runner, panel, selections, groupByVariable("g", "prom")); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if got, want := groupByOfQueries(prom), map[string][]string{"sum(a)": {"pod", "job"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("prom group by = %+v, want %+v", got, want)
	}
	if got, want := groupByOfQueries(other), map[string][]string{"sum(b)": nil}; !reflect.DeepEqual(got, want) {
		t.Fatalf("other group by = %+v, want %+v", got, want)
	}
}

func TestGroupByFollowsADatasourceVariable(t *testing.T) {
	prom, other := newFake("prometheus"), newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom, "other": other})
	panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "${ds}", Queries: []spec.Query{{Query: "sum(a)"}}}
	variables := []spec.Variable{withDefault(datasourceVariable("ds", spec.Prometheus), "prom"), groupByVariable("g", "${ds}")}
	selections := map[string]Selection{"ds": {Values: []string{"other"}}, "g": {Values: []string{"pod"}}}
	if err := runGroupBy(t, runner, panel, selections, variables...); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if got, want := groupByOfQueries(other), map[string][]string{"sum(a)": {"pod"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("other group by = %+v, want %+v", got, want)
	}
	if got := prom.callCount(); got != 0 {
		t.Fatalf("prom calls = %d, want none", got)
	}
}

func TestGroupByIsNotInjectedIntoAQueryThatNamesTheVariable(t *testing.T) {
	prom := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom})
	panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "prom", Queries: []spec.Query{{Query: "sum by ($g) (a)"}, {Query: "sum(b)"}}}
	selections := map[string]Selection{"g": {Values: []string{"pod", "job"}}}
	if err := runGroupBy(t, runner, panel, selections, groupByVariable("g", "prom")); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	want := map[string][]string{"sum by ($g) (a)": nil, "sum(b)": {"pod", "job"}}
	if got := groupByOfQueries(prom); !reflect.DeepEqual(got, want) {
		t.Fatalf("group by = %+v, want %+v", got, want)
	}
	for _, c := range prom.callsTo("instant") {
		if c.query.Text != "sum by ($g) (a)" {
			continue
		}
		if len(c.query.Variables) != 1 || !reflect.DeepEqual(c.query.Variables[0].Values, []string{"pod", "job"}) {
			t.Fatalf("variables = %+v, want g with both keys", c.query.Variables)
		}
	}
}

func TestGroupByIsNotSentToClickHouse(t *testing.T) {
	ch := newFake("clickhouse")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": ch})
	panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "ch", Queries: []spec.Query{{Query: "SELECT 1"}}}
	selections := map[string]Selection{"g": {Values: []string{"pod"}}}
	if err := runGroupBy(t, runner, panel, selections, groupByVariable("g", "ch")); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if got := groupByOfQueries(ch); !reflect.DeepEqual(got, map[string][]string{"SELECT 1": nil}) {
		t.Fatalf("group by = %+v, want none", got)
	}
}

func TestGroupBySelectionDefaults(t *testing.T) {
	tests := []struct {
		name      string
		selection map[string]Selection
		want      []string
	}{
		{"absent uses the saved default", nil, []string{"pod"}},
		{"an explicit empty selection means none", map[string]Selection{"g": {Values: []string{""}}}, nil},
		{"a given selection wins over the default", map[string]Selection{"g": {Values: []string{"job"}}}, []string{"job"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prom := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom})
			panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "prom", Queries: []spec.Query{{Query: "sum(a)"}}}
			if err := runGroupBy(t, runner, panel, tt.selection, groupByVariable("g", "prom", "pod")); err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			if got := groupByOfQueries(prom)["sum(a)"]; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("group by = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGroupBySelectionValidation(t *testing.T) {
	tooMany := make([]string, spec.MaxGroupByKeys+1)
	for i := range tooMany {
		tooMany[i] = "k" + strings.Repeat("a", i)
	}
	tests := []struct {
		name      string
		selection Selection
		message   string
	}{
		{"a key with a dash", Selection{Values: []string{"a-b"}}, `variable "g" has a label that is not a valid label name`},
		{"a key that starts with a digit", Selection{Values: []string{"1a"}}, `variable "g" has a label that is not a valid label name`},
		{"a key that is too long", Selection{Values: []string{"a" + strings.Repeat("b", spec.MaxGroupByKeyLength)}}, `variable "g" has a label that is not a valid label name`},
		{"too many keys", Selection{Values: tooMany}, `variable "g" has more than 20 labels`},
		{"All", Selection{All: true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prom := newFake("prometheus")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom})
			panel := spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "prom", Queries: []spec.Query{{Query: "sum(a)"}}}
			err := runGroupBy(t, runner, panel, map[string]Selection{"g": tt.selection}, groupByVariable("g", "prom"))
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("err = %v, want ErrInvalidRequest", err)
			}
			if tt.message != "" && !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("err = %q, want it to contain %q", err, tt.message)
			}
			if prom.callCount() != 0 {
				t.Fatalf("upstream called %d times", prom.callCount())
			}
		})
	}
}

func TestGroupByVariableHasNoValueList(t *testing.T) {
	prom := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom})
	_, _, err := runner.VariableValues(t.Context(), dashboardOf(nil, groupByVariable("g", "prom")), "g", testRange(), nil, datasource.QueryUser{})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
	if prom.callCount() != 0 {
		t.Fatalf("upstream called %d times", prom.callCount())
	}
}

func TestCheckSpecGroupByUsesTheSavedDefaultAndChecksTheDatasource(t *testing.T) {
	panels := []spec.Panel{panelOf("p0", spec.Timeseries, "prom", "sum(up)")}
	tests := []struct {
		name      string
		variables []spec.Variable
		want      []wantIssue
	}{
		{"a prometheus datasource", []spec.Variable{groupByVariable("g", "prom", "pod")}, nil},
		{"an unknown datasource", []spec.Variable{groupByVariable("g", "missing")}, []wantIssue{{"/variables/0/datasource", CodeUnknownDatasource, `datasource "missing" is not configured`}}},
		{"a clickhouse datasource", []spec.Variable{groupByVariable("g", "ch")}, []wantIssue{{"/variables/0/datasource", spec.CodeInvalidReference, ""}}},
		{"a datasource variable", []spec.Variable{withDefault(datasourceVariable("dsv", spec.Prometheus), "prom"), groupByVariable("g", "${dsv}")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prom, ch := newFake("prometheus"), newFake("clickhouse")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom, "ch": ch})
			issues, err := runner.CheckSpec(t.Context(), dashboardOf(panels, tt.variables...), testNow, datasource.QueryUser{})
			if err != nil {
				t.Fatalf("CheckSpec error: %v", err)
			}
			assertIssues(t, issues, tt.want)
		})
	}
}

func TestCheckSpecAppliesTheSavedGroupByDefaultToQueries(t *testing.T) {
	prom := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom})
	panels := []spec.Panel{panelOf("p0", spec.Timeseries, "prom", "sum(up)")}
	if _, err := runner.CheckSpec(t.Context(), dashboardOf(panels, groupByVariable("g", "prom", "pod", "job")), testNow, datasource.QueryUser{}); err != nil {
		t.Fatalf("CheckSpec error: %v", err)
	}
	calls := prom.recorded()
	if len(calls) == 0 {
		t.Fatal("no upstream calls")
	}
	for _, c := range calls {
		if !reflect.DeepEqual(c.query.GroupBy, []string{"pod", "job"}) {
			t.Fatalf("group by = %+v, want the saved default", c.query.GroupBy)
		}
	}
}
