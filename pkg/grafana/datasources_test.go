package grafana

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func sourceNamed(t *testing.T, res *Result, key string) SourceDatasource {
	t.Helper()
	for _, s := range res.Datasources {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("no source datasource %q in %+v", key, res.Datasources)
	return SourceDatasource{}
}

func TestBuiltInDatasourcesAreListedWithoutAKind(t *testing.T) {
	res := convertDoc(t, dashboardJSON("",
		`{"id":1,"type":"vendor-news-panel","title":"News","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":{"type":"datasource","uid":"grafana"}}`,
		`{"id":2,"type":"timeseries","title":"Copy","gridPos":{"x":6,"y":0,"w":6,"h":4},"datasource":"-- Dashboard --","targets":[{"panelId":3,"refId":"A"}]}`,
		`{"id":3,"type":"timeseries","title":"Mixed","gridPos":{"x":12,"y":0,"w":6,"h":4},"datasource":`+promRef+`,"targets":[{"expr":"up","refId":"A"},{"datasource":{"type":"__expr__","uid":"__expr__"},"refId":"B"}]}`,
	))
	if !res.Ready() {
		t.Fatal("built-in datasources should not block the import")
	}
	for _, tc := range []struct {
		key, typ string
		panels   int
	}{{"-- Grafana --", "grafana", 1}, {"-- Dashboard --", "dashboard", 1}, {"__expr__", "__expr__", 1}} {
		s := sourceNamed(t, res, tc.key)
		if s.Kind != nil || s.Match != nil || s.Type == nil || *s.Type != tc.typ || s.Panels != tc.panels || len(s.Candidates) != 0 {
			t.Errorf("%s = %+v", tc.key, s)
		}
	}
	if got := sourceNamed(t, res, "prom"); got.Kind == nil || *got.Kind != "prometheus" || got.Panels != 1 {
		t.Errorf("prom = %+v", got)
	}
	if got := panelLine(t, res, "p1").Reasons[0]; got != "Grafana panel type vendor-news-panel is not supported yet." {
		t.Errorf("p1 reason = %q", got)
	}
}

func TestBuiltInDatasourceCannotBeMapped(t *testing.T) {
	doc := dashboardJSON("", `{"id":1,"type":"news","title":"News","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"-- Grafana --"}`)
	_, err := Convert([]byte(doc), Options{Datasources: testDatasources, Mapping: map[string]string{"-- Grafana --": "prom"}})
	var input *InputError
	if !errors.As(err, &input) || input.Code != codeInvalidMapping {
		t.Fatalf("err = %v, want invalid_mapping", err)
	}
}

func TestDirectUIDMatchOnlyAppliesToUIDKeys(t *testing.T) {
	two := Options{Datasources: []Datasource{{UID: "prom", Type: "prometheus"}, {UID: "prom2", Type: "prometheus"}}}
	query := `"targets":[{"expr":"up","refId":"A"}]`

	byUID := convertWith(t, dashboardJSON("", panelJSON(1, 0, 0, 12, 4, `"datasource":{"type":"prometheus","uid":"prom"},`+query)), two)
	if s := sourceNamed(t, byUID, "prom"); s.Match == nil || *s.Match != "prom" || *s.MatchedBy != "uid" {
		t.Fatalf("uid source = %+v, want a uid match", s)
	}

	byName := convertWith(t, dashboardJSON("", panelJSON(1, 0, 0, 12, 4, `"datasource":"prom",`+query)), two)
	if s := sourceNamed(t, byName, "prom"); s.Match != nil || byName.Ready() {
		t.Fatalf("legacy name source = %+v, want no match: it only names the datasource", s)
	}

	unnamed := convertWith(t, dashboardJSON("", panelJSON(1, 0, 0, 12, 4, query)), Options{Datasources: []Datasource{{UID: "default", Type: "prometheus"}, {UID: "other", Type: "prometheus"}}})
	if s := sourceNamed(t, unnamed, "default"); s.Match != nil {
		t.Fatalf("default source = %+v, want no match", s)
	}
}

func TestVariableOnlyDefaultDatasourceTakesTheDashboardsOnlyKind(t *testing.T) {
	variable := `{"name":"job","type":"query","datasource":null,"query":"label_values(up, job)","options":[{"value":"a"}]}`
	prom := panelJSON(1, 0, 0, 12, 4, `"datasource":`+promRef+`,"targets":[{"expr":"up","refId":"A"}]`)
	ch := panelJSON(2, 12, 0, 12, 4, `"datasource":{"type":"grafana-clickhouse-datasource","uid":"ch"},"targets":[{"rawSql":"SELECT 1","refId":"A"}]`)

	one := convertDoc(t, dashboardJSON(templating(variable), prom))
	if v := variableNamed(t, one, "job"); v.Type != spec.QueryVariable || v.Datasource != "prom" {
		t.Fatalf("variable = %+v, want a query variable on prom", v)
	}
	if s := sourceNamed(t, one, "default"); s.Kind == nil || *s.Kind != "prometheus" || s.Panels != 0 {
		t.Fatalf("default source = %+v", s)
	}

	both := convertDoc(t, dashboardJSON(templating(variable), prom, ch))
	if s := sourceNamed(t, both, "default"); s.Kind != nil {
		t.Fatalf("default source = %+v, want no kind with two kinds in use", s)
	}
	if v := variableNamed(t, both, "job"); v.Type != spec.CustomVariable || !slices.Equal(v.Options, []string{"a"}) {
		t.Fatalf("variable = %+v, want the saved values as a custom variable", v)
	}
}

func TestPanelOnAnUnsupportedDatasourceWithoutTargetsNamesIt(t *testing.T) {
	res := convertDoc(t, dashboardJSON("", `{"id":1,"type":"logs","title":"Logs","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":{"type":"loki","uid":"loki1"}}`))
	if got := panelLine(t, res, "p1").Reasons[0]; got != "Loki is not supported yet." {
		t.Fatalf("reason = %q", got)
	}
}

const promDatasourceVariable = `{"name":"ds","type":"datasource","query":"prometheus","current":{"text":"prom","value":"prom"}}`

func jobVariableOn(datasource string) string {
	return `{"name":"job","type":"query","datasource":` + datasource + `,"query":"label_values(up, job)"}`
}

func TestQueryVariableKeepsTheDatasourceVariableItUses(t *testing.T) {
	forms := map[string]string{
		"braces":  `{"type":"prometheus","uid":"${ds}"}`,
		"dollar":  `{"type":"prometheus","uid":"$ds"}`,
		"bracket": `{"type":"prometheus","uid":"[[ds]]"}`,
		"string":  `"${ds}"`,
	}
	for name, datasource := range forms {
		t.Run(name, func(t *testing.T) {
			res := convertVariables(t, promDatasourceVariable, jobVariableOn(datasource))
			if got := variableNamed(t, res, "job").Datasource; got != "${ds}" {
				t.Fatalf("job datasource = %q, want ${ds}", got)
			}
			if notes := notesFor(res, "variable job"); len(notes) != 0 {
				t.Fatalf("job notes = %+v, want none", notes)
			}
			if names := variableNames(res); !slices.Equal(names, []string{"ds", "job"}) {
				t.Fatalf("variable order = %v, want ds then job", names)
			}
		})
	}
}

func TestQueryVariableDeclaredBeforeItsDatasourceVariableIsMovedAfterIt(t *testing.T) {
	res := convertVariables(t, jobVariableOn(`{"type":"prometheus","uid":"${ds}"}`), promDatasourceVariable)
	if names := variableNames(res); !slices.Equal(names, []string{"ds", "job"}) {
		t.Fatalf("variable order = %v, want ds then job", names)
	}
	if notes := notesFor(res, "variables"); len(notes) != 1 || !hasPath(notes, "templating.list[1]") {
		t.Fatalf("variables notes = %+v, want one naming the moved datasource variable", notes)
	}
}

func TestInputDatasourceStaysPinnedEvenWhenAVariableHasTheSameName(t *testing.T) {
	doc := `{"title":"T","__inputs":[{"name":"DS_PROMETHEUS","label":"Prometheus","type":"datasource","pluginId":"prometheus"}],` +
		`"templating":{"list":[{"name":"DS_PROMETHEUS","type":"datasource","query":"prometheus"},` + jobVariableOn(`{"type":"prometheus","uid":"${DS_PROMETHEUS}"}`) + `]},` +
		`"panels":[` + panelJSON(1, 0, 0, 12, 4, `"type":"stat","datasource":{"type":"prometheus","uid":"${DS_PROMETHEUS}"},"targets":[{"refId":"A","expr":"up"}]`) + `]}`
	res := convertDoc(t, doc)
	if got := variableNamed(t, res, "job").Datasource; got != "prom" {
		t.Fatalf("job datasource = %q, want the matched uid prom", got)
	}
	if entry, ok := res.Spec.FindPanel("p1"); !ok || entry.Panel.Datasource != "prom" {
		t.Fatalf("p1 = %+v, want datasource prom", entry.Panel)
	}
}

func TestReferencesFallBackToTheMatchWhenTheDatasourceVariableIsNotImported(t *testing.T) {
	variables := make([]string, 0, 21)
	for i := range 20 {
		variables = append(variables, fmt.Sprintf(`{"name":"c%d","type":"custom","query":"a,b"}`, i))
	}
	variables = append(variables, promDatasourceVariable)
	onDS := `"datasource":{"type":"prometheus","uid":"${ds}"},"targets":[{"refId":"A","expr":"up"}]`
	res := convertDoc(t, dashboardJSON(templating(variables...),
		panelJSON(1, 0, 0, 12, 4, `"type":"stat",`+onDS),
		panelJSON(2, 12, 0, 12, 4, `"type":"stat",`+onDS),
	))
	for _, id := range []string{"p1", "p2"} {
		if entry, ok := res.Spec.FindPanel(id); !ok || entry.Panel.Datasource != "prom" {
			t.Fatalf("%s = %+v, want the matched uid prom", id, entry.Panel)
		}
	}
	want := "Datasource variable $ds could not be imported, so what used it runs on prom."
	var notes []Note
	for _, note := range notesFor(res, "variable ds") {
		if note.Reason == want {
			notes = append(notes, note)
		}
	}
	if len(notes) != 1 || len(notes[0].Paths) != 0 {
		t.Fatalf("notes = %+v, want one note %q without paths", res.Report.Notes, want)
	}
}

func TestQueryOverrideKeepsTheDatasourceVariableOnAPanelWithALiteralDatasource(t *testing.T) {
	panel := panelJSON(1, 0, 0, 12, 4, `"type":"stat","datasource":{"type":"prometheus","uid":"prom"},"targets":[`+
		`{"refId":"A","expr":"up"},`+
		`{"refId":"B","expr":"up","datasource":{"type":"prometheus","uid":"${ds}"}}]`)
	res := convertDoc(t, dashboardJSON(templating(promDatasourceVariable), panel))
	entry, ok := res.Spec.FindPanel("p1")
	if !ok || entry.Panel.Datasource != "prom" || len(entry.Panel.Queries) != 2 {
		t.Fatalf("p1 = %+v, want datasource prom and two queries", entry.Panel)
	}
	if got := entry.Panel.Queries[0].Datasource; got != "" {
		t.Fatalf("query A datasource = %q, want none", got)
	}
	if got := entry.Panel.Queries[1].Datasource; got != "${ds}" {
		t.Fatalf("query B datasource = %q, want ${ds}", got)
	}
}
