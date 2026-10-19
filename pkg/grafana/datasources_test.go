package grafana

import (
	"errors"
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
		`{"id":1,"type":"news","title":"News","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":{"type":"datasource","uid":"grafana"}}`,
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
	if got := panelLine(t, res, "p1").Reasons[0]; got != "Grafana panel type news is not supported yet." {
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
