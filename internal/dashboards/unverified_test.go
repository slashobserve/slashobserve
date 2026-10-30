package dashboards

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type testPanel struct {
	id, title, datasource, query string
	extraQuery                   string
	panelInterval                string
	queryInterval                string
	x                            int
}

func testDashboard(t *testing.T, variables string, panels ...testPanel) *spec.Dashboard {
	t.Helper()
	var items []string
	for i, p := range panels {
		queries := fmt.Sprintf(`{"query":%q}`, p.query)
		if p.queryInterval != "" {
			queries = fmt.Sprintf(`{"query":%q,"interval":%q}`, p.query, p.queryInterval)
		}
		panelInterval := ""
		if p.panelInterval != "" {
			panelInterval = fmt.Sprintf(`,"interval":%q`, p.panelInterval)
		}
		if p.extraQuery != "" {
			queries += fmt.Sprintf(`,{"query":%q}`, p.extraQuery)
		}
		items = append(items, fmt.Sprintf(`{"id":%q,"type":"stat","title":%q,"gridPos":{"x":%d,"y":0,"w":6,"h":4},"datasource":%q%s,"queries":[%s]}`, p.id, p.title, i*6+p.x, p.datasource, panelInterval, queries))
	}
	raw := fmt.Sprintf(`{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"variables":[%s],"panels":[%s]}`, variables, strings.Join(items, ","))
	d, err := spec.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse test dashboard: %v", err)
	}
	return d
}

func issueAt(path string) spec.Issue {
	return spec.Issue{Path: path, Code: queryproxy.CodeQueryEmpty, Message: "empty"}
}

func panelIDs(u Unverified) []string {
	var ids []string
	for _, p := range u.Panels {
		ids = append(ids, p.PanelID)
	}
	return ids
}

func TestUnverifiedOfAttributesIssuesToTheLongestPointer(t *testing.T) {
	d := testDashboard(t, `{"name":"a","type":"custom","options":["x"]},{"name":"b","type":"custom","options":["y"]}`,
		testPanel{id: "p1", title: "One", datasource: "prom", query: "up"},
		testPanel{id: "p2", title: "Two", datasource: "prom", query: "up"},
	)
	row := spec.Dashboard{Panels: []spec.Panel{{ID: "r", Panels: []spec.Panel{{ID: "c"}}}, {ID: "z"}}}
	got, rest := UnverifiedOf(&row, []spec.Issue{issueAt("/panels/0/panels/0/queries/0/query"), issueAt("/panels/0/title"), issueAt("/time")})
	if ids := panelIDs(got); len(ids) != 2 || ids[0] != "r" || ids[1] != "c" {
		t.Fatalf("row child and row issues: panels %v", ids)
	}
	if len(rest) != 1 || rest[0].Path != "/time" {
		t.Fatalf("unattributed issues %v, want only /time", rest)
	}

	got, rest = UnverifiedOf(d, []spec.Issue{issueAt("/variables/1"), issueAt("/variables/1/query"), issueAt("/variables/12"), issueAt("/panels/1/queries"), issueAt("/panels/10/queries")})
	if len(got.Variables) != 1 || got.Variables[0].Name != "b" || len(got.Variables[0].Issues) != 2 {
		t.Fatalf("variables %+v, want variable b with 2 issues", got.Variables)
	}
	if ids := panelIDs(got); len(ids) != 1 || ids[0] != "p2" {
		t.Fatalf("panels %v, want [p2]", ids)
	}
	if len(rest) != 2 || rest[0].Path != "/variables/12" || rest[1].Path != "/panels/10/queries" {
		t.Fatalf("unattributed %v, want /variables/12 and /panels/10/queries in input order", rest)
	}
}

func TestCarryUnverifiedKeepsUnchangedFailingPanels(t *testing.T) {
	base := testDashboard(t, ``, testPanel{id: "up", title: "Up", datasource: "prom", query: "sum(up)"}, testPanel{id: "missing", title: "Missing", datasource: "prom", query: "sum(nope)", x: 6})
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "missing", Issues: []spec.Issue{issueAt("/panels/1/queries")}}}}
	fails := []spec.Issue{issueAt("/panels/1/queries")}

	moved := testDashboard(t, ``, testPanel{id: "up", title: "Renamed", datasource: "prom", query: "sum(up)"}, testPanel{id: "missing", title: "Missing again", datasource: "prom", query: "sum(nope)", x: 9})
	carried, blocking := CarryUnverified(base, prev, moved, fails)
	if len(blocking) != 0 || len(carried.Panels) != 1 || carried.Panels[0].PanelID != "missing" {
		t.Fatalf("moved and retitled: carried %+v blocking %v, want missing carried", carried, blocking)
	}

	reordered := testDashboard(t, ``, testPanel{id: "missing", title: "Missing", datasource: "prom", query: "sum(nope)"}, testPanel{id: "up", title: "Up", datasource: "prom", query: "sum(up)", x: 6})
	carried, blocking = CarryUnverified(base, prev, reordered, []spec.Issue{issueAt("/panels/0/queries")})
	if len(blocking) != 0 || len(carried.Panels) != 1 || carried.Panels[0].Issues[0].Path != "/panels/0/queries" {
		t.Fatalf("reordered panels: carried %+v blocking %v, want the new path carried", carried, blocking)
	}

	_, blocking = CarryUnverified(base, Unverified{}, base, fails)
	if len(blocking) != 1 {
		t.Fatalf("empty previous list: blocking %v, want the strict rule", blocking)
	}
	_, blocking = CarryUnverified(nil, prev, base, fails)
	if len(blocking) != 1 {
		t.Fatalf("no previous spec: blocking %v, want the strict rule", blocking)
	}
}

func TestCarryUnverifiedBlocksChangedQueries(t *testing.T) {
	base := testDashboard(t, ``, testPanel{id: "m", title: "M", datasource: "prom", query: "sum(nope)", extraQuery: "sum(other)"})
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "m", Issues: []spec.Issue{issueAt("/panels/0/queries")}}}}
	fails := []spec.Issue{issueAt("/panels/0/queries/0/query")}

	changes := map[string]*spec.Dashboard{
		"query text":   testDashboard(t, ``, testPanel{id: "m", title: "M", datasource: "prom", query: "sum(nope2)", extraQuery: "sum(other)"}),
		"datasource":   testDashboard(t, ``, testPanel{id: "m", title: "M", datasource: "ch", query: "sum(nope)", extraQuery: "sum(other)"}),
		"query count":  testDashboard(t, ``, testPanel{id: "m", title: "M", datasource: "prom", query: "sum(nope)"}),
		"query order":  testDashboard(t, ``, testPanel{id: "m", title: "M", datasource: "prom", query: "sum(other)", extraQuery: "sum(nope)"}),
		"new panel id": testDashboard(t, ``, testPanel{id: "n", title: "M", datasource: "prom", query: "sum(nope)", extraQuery: "sum(other)"}),
	}
	for name, next := range changes {
		carried, blocking := CarryUnverified(base, prev, next, fails)
		if len(blocking) != 1 || !carried.Empty() {
			t.Errorf("%s: carried %+v blocking %v, want the issue blocking", name, carried, blocking)
		}
	}

	moved := base.Panels[0]
	moved.Datasource = ""
	moved.Queries = append([]spec.Query(nil), moved.Queries...)
	moved.Queries[0].Datasource = "prom"
	moved.Queries[1].Datasource = "prom"
	next := *base
	next.Panels = []spec.Panel{moved}
	if carried, blocking := CarryUnverified(base, prev, &next, fails); len(blocking) != 0 || len(carried.Panels) != 1 {
		t.Errorf("datasource moved onto the queries: carried %+v blocking %v, want carried", carried, blocking)
	}
}

func TestCarryUnverifiedDropsPanelsThatPassOrVanish(t *testing.T) {
	base := testDashboard(t, ``, testPanel{id: "m", title: "M", datasource: "prom", query: "sum(nope)"})
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "m", Issues: []spec.Issue{issueAt("/panels/0/queries")}}}}
	if carried, blocking := CarryUnverified(base, prev, base, nil); !carried.Empty() || blocking != nil {
		t.Fatalf("panel that now passes: carried %+v blocking %v, want nothing", carried, blocking)
	}
	if carried, blocking := CarryUnverified(base, prev, testDashboard(t, ``, testPanel{id: "other", title: "O", datasource: "prom", query: "up"}), nil); !carried.Empty() || blocking != nil {
		t.Fatalf("removed panel: carried %+v blocking %v, want nothing", carried, blocking)
	}
}

func TestCarryUnverifiedTreatsUnattributedIssuesAsBlocking(t *testing.T) {
	base := testDashboard(t, ``, testPanel{id: "m", title: "M", datasource: "prom", query: "sum(nope)"})
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "m", Issues: []spec.Issue{issueAt("/panels/0/queries")}}}}
	carried, blocking := CarryUnverified(base, prev, base, []spec.Issue{issueAt("/time"), issueAt("/panels/0/queries")})
	if len(blocking) != 1 || blocking[0].Path != "/time" || len(carried.Panels) != 1 {
		t.Fatalf("carried %+v blocking %v, want /time blocking and the panel carried", carried, blocking)
	}
}

func TestCarryUnverifiedFollowsVariablesAPanelReaches(t *testing.T) {
	const inst = `{"name":"inst","type":"query","datasource":"prom","query":"label_values(nope, instance)"}`
	panel := testPanel{id: "by", title: "By", datasource: "prom", query: `sum(up{instance=~"$inst"})`}
	base := testDashboard(t, inst, panel)
	prev := Unverified{
		Panels:    []UnverifiedPanel{{PanelID: "by", Issues: []spec.Issue{issueAt("/panels/0/queries/0/query")}}},
		Variables: []UnverifiedVariable{{Name: "inst", Issues: []spec.Issue{issueAt("/variables/0")}}},
	}
	fails := []spec.Issue{issueAt("/panels/0/queries/0/query"), issueAt("/variables/0")}

	if carried, blocking := CarryUnverified(base, prev, base, fails); len(blocking) != 0 || len(carried.Panels) != 1 || len(carried.Variables) != 1 {
		t.Fatalf("unchanged: carried %+v blocking %v, want both carried", carried, blocking)
	}

	edited := testDashboard(t, `{"name":"inst","type":"query","datasource":"prom","query":"label_values(other, instance)"}`, panel)
	carried, blocking := CarryUnverified(base, prev, edited, fails)
	if len(blocking) != 2 || !carried.Empty() {
		t.Fatalf("edited variable: carried %+v blocking %v, want the variable and the panel that reads it blocking", carried, blocking)
	}

	dropped := *base
	dropped.Variables = nil
	if _, blocking := CarryUnverified(base, prev, &dropped, []spec.Issue{issueAt("/panels/0/queries/0/query")}); len(blocking) != 1 {
		t.Fatalf("variable no longer declared: blocking %v, want the panel blocking", blocking)
	}
}

func TestCarryUnverifiedFollowsTheIntervalVariableAMinStepNames(t *testing.T) {
	const resolution = `{"name":"iv","type":"interval","options":["30s","1m"]}`
	const changed = `{"name":"iv","type":"interval","options":["30s","5m"]}`
	cases := []struct {
		name  string
		panel testPanel
	}{
		{"panel interval", testPanel{id: "p", title: "P", datasource: "prom", query: "up", panelInterval: "${iv}"}},
		{"query interval", testPanel{id: "p", title: "P", datasource: "prom", query: "up", queryInterval: "${iv}"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := testDashboard(t, resolution, tc.panel)
			prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "p", Issues: []spec.Issue{issueAt("/panels/0/queries/0/query")}}}}
			fails := []spec.Issue{issueAt("/panels/0/queries/0/query")}

			if carried, blocking := CarryUnverified(base, prev, base, fails); len(blocking) != 0 || len(carried.Panels) != 1 {
				t.Fatalf("unchanged variable: carried %+v blocking %v, want the panel carried", carried, blocking)
			}
			edited := testDashboard(t, changed, tc.panel)
			if carried, blocking := CarryUnverified(base, prev, edited, fails); len(blocking) != 1 || !carried.Empty() {
				t.Fatalf("edited variable: carried %+v blocking %v, want the panel blocking", carried, blocking)
			}
		})
	}
}

func TestCarryUnverifiedFollowsVariableQueriesAndSurvivesCycles(t *testing.T) {
	chain := func(leaf string) string {
		return `{"name":"b","type":"query","datasource":"prom","query":"label_values(up, ` + leaf + `)"},` +
			`{"name":"a","type":"query","datasource":"prom","query":"label_values(up{x=\"$b\"}, i)"}`
	}
	panel := testPanel{id: "p", title: "P", datasource: "prom", query: `sum(up{i="$a"})`}
	base := testDashboard(t, chain("j"), panel)
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "p", Issues: []spec.Issue{issueAt("/panels/0/queries")}}}}
	fails := []spec.Issue{issueAt("/panels/0/queries")}

	if carried, blocking := CarryUnverified(base, prev, base, fails); len(blocking) != 0 || len(carried.Panels) != 1 {
		t.Fatalf("unchanged chain: carried %+v blocking %v", carried, blocking)
	}
	if _, blocking := CarryUnverified(base, prev, testDashboard(t, chain("k"), panel), fails); len(blocking) != 1 {
		t.Fatalf("variable two hops away changed: blocking %v, want the panel blocking", blocking)
	}

	cyclic := *base
	cyclic.Variables = []spec.Variable{
		{Name: "a", Type: spec.QueryVariable, Datasource: "prom", Query: "label_values(up{x=\"$b\"}, i)"},
		{Name: "b", Type: spec.QueryVariable, Datasource: "prom", Query: "label_values(up{y=\"$a\"}, j)"},
	}
	if carried, blocking := CarryUnverified(&cyclic, prev, &cyclic, fails); len(blocking) != 0 || len(carried.Panels) != 1 {
		t.Fatalf("cyclic variables: carried %+v blocking %v, want carried without looping", carried, blocking)
	}
}

func TestCarryUnverifiedCountsAdhocVariablesOnTheQueryDatasource(t *testing.T) {
	adhoc := func(datasource string) string {
		return `{"name":"filters","type":"adhoc","datasource":"` + datasource + `"}`
	}
	panel := testPanel{id: "p", title: "P", datasource: "prom", query: "sum(nope)"}
	base := testDashboard(t, adhoc("prom"), panel)
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "p", Issues: []spec.Issue{issueAt("/panels/0/queries")}}}}
	fails := []spec.Issue{issueAt("/panels/0/queries")}

	if _, blocking := CarryUnverified(base, prev, testDashboard(t, ``, panel), fails); len(blocking) != 1 {
		t.Fatalf("adhoc variable on the datasource removed: blocking %v, want the panel blocking", blocking)
	}
	if carried, blocking := CarryUnverified(base, prev, testDashboard(t, adhoc("prom")+`,{"name":"other","type":"adhoc","datasource":"ch"}`, panel), fails); len(blocking) != 0 || len(carried.Panels) != 1 {
		t.Fatalf("adhoc variable on another datasource added: carried %+v blocking %v, want carried", carried, blocking)
	}
}

func TestCarryUnverifiedFollowsTheDatasourceVariableAPanelReferences(t *testing.T) {
	ds := func(extra string) string {
		return `{"name":"ds","type":"datasource","datasourceType":"prometheus","default":"prom"` + extra + `}`
	}
	panel := testPanel{id: "p", title: "P", datasource: "${ds}", query: "sum(nope)"}
	base := testDashboard(t, ds(``), panel)
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "p", Issues: []spec.Issue{issueAt("/panels/0/queries")}}}}
	fails := []spec.Issue{issueAt("/panels/0/queries")}

	if carried, blocking := CarryUnverified(base, prev, base, fails); len(blocking) != 0 || len(carried.Panels) != 1 {
		t.Fatalf("unchanged datasource variable: carried %+v blocking %v, want carried", carried, blocking)
	}
	for name, changed := range map[string]string{
		"default": `{"name":"ds","type":"datasource","datasourceType":"prometheus","default":"prom2"}`,
		"regex":   ds(`,"regex":"/^prom/"`),
	} {
		if carried, blocking := CarryUnverified(base, prev, testDashboard(t, changed, panel), fails); len(blocking) != 1 || !carried.Empty() {
			t.Fatalf("changed %s: carried %+v blocking %v, want the panel blocking", name, carried, blocking)
		}
	}
	if _, blocking := CarryUnverified(base, prev, testDashboard(t, ds(``), testPanel{id: "p", title: "P", datasource: "prom", query: "sum(nope)"}), fails); len(blocking) != 1 {
		t.Fatalf("panel moved from the variable to a uid: blocking %v, want the panel blocking", blocking)
	}
}

func TestCarryUnverifiedCountsAdhocVariablesAgainstAReferencedDatasource(t *testing.T) {
	const ds = `{"name":"ds","type":"datasource","datasourceType":"prometheus","default":"prom"}`
	adhoc := func(datasource string) string {
		return `{"name":"filters","type":"adhoc","datasource":"` + datasource + `"}`
	}
	panel := testPanel{id: "p", title: "P", datasource: "${ds}", query: "sum(nope)"}
	base := testDashboard(t, ds+`,`+adhoc("prom"), panel)
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "p", Issues: []spec.Issue{issueAt("/panels/0/queries")}}}}
	fails := []spec.Issue{issueAt("/panels/0/queries")}

	if carried, blocking := CarryUnverified(base, prev, base, fails); len(blocking) != 0 || len(carried.Panels) != 1 {
		t.Fatalf("unchanged adhoc variable: carried %+v blocking %v, want carried", carried, blocking)
	}
	if _, blocking := CarryUnverified(base, prev, testDashboard(t, ds+`,`+adhoc("prom2"), panel), fails); len(blocking) != 1 {
		t.Fatalf("adhoc variable changed: blocking %v, want the panel blocking", blocking)
	}
}

func TestCarryUnverifiedKeepsOnlyUnchangedFailingVariables(t *testing.T) {
	const inst = `{"name":"inst","type":"query","datasource":"prom","query":"label_values(nope, instance)"}`
	base := testDashboard(t, inst, testPanel{id: "p", title: "P", datasource: "prom", query: "up"})
	prev := Unverified{Variables: []UnverifiedVariable{{Name: "inst", Issues: []spec.Issue{issueAt("/variables/0")}}}}
	fails := []spec.Issue{issueAt("/variables/0")}

	if carried, blocking := CarryUnverified(base, prev, base, fails); len(blocking) != 0 || len(carried.Variables) != 1 {
		t.Fatalf("unchanged variable: carried %+v blocking %v, want carried", carried, blocking)
	}
	changed := testDashboard(t, `{"name":"inst","type":"query","datasource":"prom","query":"label_values(nope2, instance)"}`, testPanel{id: "p", title: "P", datasource: "prom", query: "up"})
	if carried, blocking := CarryUnverified(base, prev, changed, fails); len(blocking) != 1 || !carried.Empty() {
		t.Fatalf("changed variable: carried %+v blocking %v, want blocking", carried, blocking)
	}
}

func TestUnverifiedEmptyAndLookups(t *testing.T) {
	if !(Unverified{}).Empty() {
		t.Fatal("zero value must be empty")
	}
	u := Unverified{Panels: []UnverifiedPanel{{PanelID: "a"}}, Variables: []UnverifiedVariable{{Name: "v"}}}
	if u.Empty() {
		t.Fatal("non-empty list reported empty")
	}
	if _, ok := u.Panel("a"); !ok {
		t.Fatal("panel a not found")
	}
	if _, ok := u.Panel("b"); ok {
		t.Fatal("panel b found")
	}
	if _, ok := u.Variable("v"); !ok {
		t.Fatal("variable v not found")
	}
}

func TestUnverifiedOfReplacesNULInMessages(t *testing.T) {
	d := testDashboard(t, ``, testPanel{id: "p", title: "P", datasource: "prom", query: "up"})
	got, _ := UnverifiedOf(d, []spec.Issue{{Path: "/panels/0/queries", Code: queryproxy.CodeQueryEmpty, Message: "bad a\x00b"}})
	if msg := got.Panels[0].Issues[0].Message; strings.ContainsRune(msg, 0) || !strings.Contains(msg, "a\uFFFDb") {
		t.Fatalf("message %q, want the NUL replaced", msg)
	}
}

func expressionDashboard(t *testing.T, variables, expression string) *spec.Dashboard {
	t.Helper()
	raw := fmt.Sprintf(`{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"variables":[%s],"panels":[{"id":"p","type":"timeseries","title":"P","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"ref":"A","query":"up","datasource":"ch","hide":true},{"ref":"B","expression":%s}]}]}`, variables, expression)
	d, err := spec.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse test dashboard: %v", err)
	}
	return d
}

func TestReachedByPanelLetsAnExpressionReachOnlyWhatItsInputsReach(t *testing.T) {
	const double = `{"type":"math","math":"$A * 2"}`
	adhoc := func(datasource string) string {
		return `{"name":"filters","type":"adhoc","datasource":"` + datasource + `"}`
	}
	onPanelDatasource := expressionDashboard(t, adhoc("prom"), double)
	if got, ok := reachedByPanel(onPanelDatasource, &onPanelDatasource.Panels[0]); !ok || len(got) != 0 {
		t.Fatalf("reached = %v, %v; want nothing, because the expression adds no datasource of its own", got, ok)
	}
	onInputDatasource := expressionDashboard(t, adhoc("ch"), double)
	if got, ok := reachedByPanel(onInputDatasource, &onInputDatasource.Panels[0]); !ok || len(got) != 1 {
		t.Fatalf("reached = %v, %v; want the adhoc variable on the input's datasource", got, ok)
	}
}

func TestCarryUnverifiedBlocksAChangedExpression(t *testing.T) {
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "p", Issues: []spec.Issue{issueAt("/panels/0/queries/1/expression")}}}}
	fails := []spec.Issue{issueAt("/panels/0/queries/1/expression")}
	base := expressionDashboard(t, ``, `{"type":"math","math":"$A * 2"}`)

	if carried, blocking := CarryUnverified(base, prev, expressionDashboard(t, ``, `{"type":"math","math":"$A * 2"}`), fails); len(blocking) != 0 || len(carried.Panels) != 1 {
		t.Fatalf("unchanged expression: carried %+v blocking %v, want carried", carried, blocking)
	}
	if _, blocking := CarryUnverified(base, prev, expressionDashboard(t, ``, `{"type":"math","math":"$A * 3"}`), fails); len(blocking) != 1 {
		t.Fatalf("changed expression: blocking %v, want the panel blocking", blocking)
	}
}

func TestCarryUnverifiedCountsGroupByVariablesOnTheQueryDatasource(t *testing.T) {
	groupBy := func(datasource string) string {
		return `{"name":"by","type":"groupby","datasource":"` + datasource + `"}`
	}
	panel := testPanel{id: "p", title: "P", datasource: "prom", query: "sum(nope)"}
	base := testDashboard(t, groupBy("prom"), panel)
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "p", Issues: []spec.Issue{issueAt("/panels/0/queries")}}}}
	fails := []spec.Issue{issueAt("/panels/0/queries")}

	if _, blocking := CarryUnverified(base, prev, testDashboard(t, ``, panel), fails); len(blocking) != 1 {
		t.Fatalf("group by variable on the datasource removed: blocking %v, want the panel blocking", blocking)
	}
	if carried, blocking := CarryUnverified(base, prev, testDashboard(t, groupBy("prom")+`,{"name":"other","type":"groupby","datasource":"prom2"}`, panel), fails); len(blocking) != 0 || len(carried.Panels) != 1 {
		t.Fatalf("group by variable on another datasource added: carried %+v blocking %v, want carried", carried, blocking)
	}
}
