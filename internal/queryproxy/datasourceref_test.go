package queryproxy

import (
	"errors"
	"reflect"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func refDashboard(panels []spec.Panel, variables ...spec.Variable) *spec.Dashboard {
	return dashboardOf(panels, append([]spec.Variable{datasourceVariable("ds", spec.Prometheus)}, variables...)...)
}

func refPanel(uid string) spec.Panel {
	return panelOf("p1", spec.Table, uid, "up")
}

func TestResolveDatasourceUID(t *testing.T) {
	d := refDashboard(nil, customVariable("c", "x"))
	one := map[string]Selection{"ds": {Values: []string{"prom2"}}}
	tests := []struct {
		name       string
		ref        string
		selections map[string]Selection
		want       string
		wantErr    error
	}{
		{"a uid passes through", "prom", nil, "prom", nil},
		{"a reference resolves to its selection", "${ds}", one, "prom2", nil},
		{"a missing selection", "${ds}", nil, "", ErrInvalidRequest},
		{"All selected", "${ds}", map[string]Selection{"ds": {All: true}}, "", ErrInvalidRequest},
		{"two values selected", "${ds}", map[string]Selection{"ds": {Values: []string{"prom", "prom2"}}}, "", ErrInvalidRequest},
		{"an undeclared name", "${nope}", one, "", ErrUnknownVariable},
		{"a variable that is not a datasource variable", "${c}", one, "", ErrUnknownVariable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveDatasourceUID(d, tt.ref, tt.selections)
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Fatalf("ResolveDatasourceUID = %q, %v, want %q, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestPanelRunsOnTheSelectedDatasource(t *testing.T) {
	regexed := datasourceVariable("ds", spec.Prometheus)
	regexed.Regex = "/^prom2$/"
	tests := []struct {
		name      string
		variable  spec.Variable
		panel     spec.Panel
		selection map[string]Selection
		wantProm2 int
		wantProm  int
		wantErr   error
	}{
		{"the selected datasource runs the panel", datasourceVariable("ds", spec.Prometheus), refPanel("${ds}"), map[string]Selection{"ds": {Values: []string{"prom2"}}}, 1, 0, nil},
		{"the other datasource runs it when selected", datasourceVariable("ds", spec.Prometheus), refPanel("${ds}"), map[string]Selection{"ds": {Values: []string{"prom"}}}, 0, 1, nil},
		{"a datasource of another type is refused", datasourceVariable("ds", spec.Prometheus), refPanel("${ds}"), map[string]Selection{"ds": {Values: []string{"ch"}}}, 0, 0, ErrInvalidRequest},
		{"a value outside the regex is refused", regexed, refPanel("${ds}"), map[string]Selection{"ds": {Values: []string{"prom"}}}, 0, 0, ErrInvalidRequest},
		{"no selection is refused", datasourceVariable("ds", spec.Prometheus), refPanel("${ds}"), nil, 0, 0, ErrInvalidRequest},
		{
			"a query override runs on the selected datasource",
			datasourceVariable("ds", spec.Prometheus),
			spec.Panel{ID: "p1", Type: spec.Table, Title: "p1", Datasource: "prom", Queries: []spec.Query{{Query: "up", Datasource: "${ds}"}}},
			map[string]Selection{"ds": {Values: []string{"prom2"}}}, 1, 0, nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prom, prom2, ch := newFake("prometheus"), newFake("prometheus"), newFake("clickhouse")
			runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom, "prom2": prom2, "ch": ch})
			d := dashboardOf([]spec.Panel{tt.panel}, tt.variable)
			_, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: tt.selection})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("RunPanel error = %v, want %v", err, tt.wantErr)
			}
			if got := len(prom2.callsTo("instant")); got != tt.wantProm2 {
				t.Fatalf("prom2 instant calls = %d, want %d", got, tt.wantProm2)
			}
			if got := len(prom.callsTo("instant")); got != tt.wantProm {
				t.Fatalf("prom instant calls = %d, want %d", got, tt.wantProm)
			}
			if ch.callCount() != 0 {
				t.Fatalf("clickhouse was called: %+v", ch.recorded())
			}
		})
	}
}

func TestQueryVariableOnADatasourceVariableCachesPerSelectedDatasource(t *testing.T) {
	prom, prom2 := newFake("prometheus"), newFake("prometheus")
	prom.onVariable = answer("from-prom")
	prom2.onVariable = answer("from-prom2")
	reg, err := datasource.NewRegistry(discardLogger(),
		datasource.Instance{UID: "prom", Datasource: prom},
		datasource.Instance{UID: "prom2", Datasource: prom2},
	)
	if err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(reg, discardLogger(), WithValueCache())
	d := refDashboard(nil, queryVariable("q", "${ds}", "label_values(job)"))
	for range 2 {
		for uid, want := range map[string][]string{"prom": {"from-prom"}, "prom2": {"from-prom2"}} {
			got, _, err := valueStrings(runner.VariableValues(t.Context(), d, "q", testRange(), map[string]Selection{"ds": {Values: []string{uid}}}, datasource.QueryUser{}))
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("VariableValues with ds=%s = %q, %v, want %q", uid, got, err, want)
			}
		}
	}
	if got := len(prom.callsTo("variable")); got != 1 {
		t.Fatalf("prom variable calls = %d, want 1", got)
	}
	if got := len(prom2.callsTo("variable")); got != 1 {
		t.Fatalf("prom2 variable calls = %d, want 1", got)
	}
}

func TestCheckSpecWithADatasourceVariable(t *testing.T) {
	t.Run("a reference panel validates against the default, or the first datasource when there is none", func(t *testing.T) {
		tests := []struct {
			name      string
			variable  spec.Variable
			wantProm  int
			wantProm2 int
		}{
			{"default", withDefault(datasourceVariable("ds", spec.Prometheus), "prom2"), 0, 1},
			{"no default", datasourceVariable("ds", spec.Prometheus), 1, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				prom, prom2 := newFake("prometheus"), newFake("prometheus")
				runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom, "prom2": prom2})
				d := dashboardOf([]spec.Panel{refPanel("${ds}")}, tt.variable)
				issues, err := runner.CheckSpec(t.Context(), d, testNow, datasource.QueryUser{})
				if err != nil || issues != nil {
					t.Fatalf("CheckSpec = %+v, %v, want no issues", issues, err)
				}
				if got := len(prom.callsTo("instant")); got != tt.wantProm {
					t.Fatalf("prom instant calls = %d, want %d", got, tt.wantProm)
				}
				if got := len(prom2.callsTo("instant")); got != tt.wantProm2 {
					t.Fatalf("prom2 instant calls = %d, want %d", got, tt.wantProm2)
				}
			})
		}
	})

	t.Run("no datasource of the variable's type fails it and everything that depends on it without a call", func(t *testing.T) {
		ch := newFake("clickhouse")
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ch": ch})
		d := refDashboard([]spec.Panel{refPanel("${ds}")}, queryVariable("q", "${ds}", "label_values(job)"))
		issues, err := runner.CheckSpec(t.Context(), d, testNow, datasource.QueryUser{})
		if err != nil {
			t.Fatal(err)
		}
		assertIssues(t, issues, []wantIssue{
			{"/panels/0/queries/0/query", CodeVariableUnresolved, ""},
			{"/variables/0", CodeVariableEmpty, ""},
			{"/variables/1", CodeVariableUnresolved, ""},
		})
		if ch.callCount() != 0 {
			t.Fatalf("upstream calls = %+v, want none", ch.recorded())
		}
	})

	t.Run("a macro the variable's datasource type lacks is reported without a call", func(t *testing.T) {
		prom := newFake("prometheus")
		runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": prom})
		panel := panelOf("p1", spec.Table, "${ds}", "up[$__timeFilter(ts)]")
		issues, err := runner.CheckSpec(t.Context(), refDashboard([]spec.Panel{panel}), testNow, datasource.QueryUser{})
		if err != nil {
			t.Fatal(err)
		}
		assertIssues(t, issues, []wantIssue{{"/panels/0/queries/0/query", spec.CodeUnknownMacro, "macro $__timeFilter is not available for prometheus queries"}})
		if prom.callsTo("instant") != nil {
			t.Fatalf("the panel ran: %+v", prom.recorded())
		}
	})
}

func TestAdhocFiltersFollowTheResolvedDatasource(t *testing.T) {
	tests := []struct {
		name       string
		adhocOn    string
		panelOn    string
		selected   string
		ranOn      string
		wantFilter bool
	}{
		{"an adhoc on the variable filters a query on the datasource it selects", "${ds}", "prom2", "prom2", "prom2", true},
		{"an adhoc on the variable does not filter a query on another datasource", "${ds}", "prom2", "prom", "prom2", false},
		{"an adhoc on a uid does not filter a query that resolves elsewhere", "prom", "${ds}", "prom2", "prom2", false},
		{"an adhoc on a uid filters a query that resolves to it", "prom", "${ds}", "prom", "prom", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakes := map[string]*fakeDatasource{"prom": newFake("prometheus"), "prom2": newFake("prometheus")}
			runner := newTestRunner(t, discardLogger(), fakes)
			d := refDashboard([]spec.Panel{refPanel(tt.panelOn)}, adhocVariable("f", tt.adhocOn))
			selections := map[string]Selection{"ds": {Values: []string{tt.selected}}, "f": {Values: []string{"job|=|api"}}}
			if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), Selections: selections}); err != nil {
				t.Fatalf("RunPanel: %v", err)
			}
			calls := fakes[tt.ranOn].callsTo("instant")
			if len(calls) != 1 {
				t.Fatalf("%s instant calls = %d, want 1", tt.ranOn, len(calls))
			}
			var want []datasource.AdhocFilter
			if tt.wantFilter {
				want = []datasource.AdhocFilter{{Key: "job", Op: "=", Value: "api"}}
			}
			if !reflect.DeepEqual(calls[0].query.Filters, want) {
				t.Fatalf("filters = %+v, want %+v", calls[0].query.Filters, want)
			}
		})
	}
}
