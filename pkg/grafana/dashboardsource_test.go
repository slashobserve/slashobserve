package grafana

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const dashboardDatasource = `{"type":"datasource","uid":"-- Dashboard --"}`

func sourcePanelDoc(id, x int, extra string) string {
	return fmt.Sprintf(`{"id":%d,"type":"timeseries","title":"Source %d","gridPos":{"x":%d,"y":0,"w":6,"h":4},"datasource":%s,"targets":[{"refId":"A","expr":"up","datasource":%s}]%s}`, id, id, x, promRef, promRef, extra)
}

func reusingPanelDoc(id, x int, target, extra string) string {
	return fmt.Sprintf(`{"id":%d,"type":"table","title":"Reuse %d","gridPos":{"x":%d,"y":0,"w":6,"h":4},"datasource":%s,"targets":[%s]%s}`, id, id, x, dashboardDatasource, target, extra)
}

func reuseTarget(members string) string {
	return fmt.Sprintf(`{"refId":"A","datasource":%s,%s}`, dashboardDatasource, members)
}

func convertedSpec(t *testing.T, doc string) *Result {
	t.Helper()
	res := convertDoc(t, doc)
	if !res.Ready() {
		t.Fatalf("result is not ready: %+v", res.Datasources)
	}
	data, err := json.Marshal(res.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.Parse(data); err != nil {
		t.Fatalf("the converted Spec does not parse: %v\n%s", err, data)
	}
	return res
}

func specPanelOf(t *testing.T, res *Result, id string) spec.Panel {
	t.Helper()
	entry, ok := res.Spec.FindPanel(id)
	if !ok {
		t.Fatalf("no panel %q in the Spec", id)
	}
	return *entry.Panel
}

func TestDashboardSourceImportsAsAReusingQuery(t *testing.T) {
	cases := []struct {
		name    string
		members string
		want    spec.Query
	}{
		{"panel only", `"panelId":1`, spec.Query{Source: &spec.DashboardSource{Panel: "p1"}}},
		{"with transforms", `"panelId":1,"withTransforms":true`, spec.Query{Source: &spec.DashboardSource{Panel: "p1", WithTransforms: true}}},
		{"hidden", `"panelId":1,"hide":true`, spec.Query{Hide: true, Source: &spec.DashboardSource{Panel: "p1"}}},
		{"ad hoc filters", `"panelId":1,"adHocFiltersEnabled":true`, spec.Query{Source: &spec.DashboardSource{Panel: "p1", AdHocFilters: true}}},
		{"ad hoc filters off", `"panelId":1,"adHocFiltersEnabled":false`, spec.Query{Source: &spec.DashboardSource{Panel: "p1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertedSpec(t, dashboardJSON("", sourcePanelDoc(1, 0, ""), reusingPanelDoc(2, 6, reuseTarget(tc.members), "")))

			panel := specPanelOf(t, res, "p2")
			if panel.Datasource != "" || len(panel.Queries) != 1 {
				t.Fatalf("panel = %+v, want no datasource and one query", panel)
			}
			got := panel.Queries[0]
			if got.Hide != tc.want.Hide || got.Source == nil || *got.Source != *tc.want.Source || got.Query != "" {
				t.Fatalf("query = %+v, want %+v", got, tc.want)
			}
			if line := panelLine(t, res, "p2"); line.Status != statusImported {
				t.Fatalf("line = %+v, want imported", line)
			}
		})
	}
}

func TestDashboardSourceFindsItsSourceWhereverItSits(t *testing.T) {
	target := reuseTarget(`"panelId":2`)

	later := convertedSpec(t, dashboardJSON("", reusingPanelDoc(1, 0, target, ""), sourcePanelDoc(2, 6, "")))
	if got := specPanelOf(t, later, "p1").Queries[0].Source; got == nil || got.Panel != "p2" {
		t.Fatalf("source = %+v, want p2 though it comes later", got)
	}

	inRow := convertedSpec(t, dashboardJSON("",
		rowJSON(10, 0, true, `"panels":[`+sourcePanelDoc(2, 0, "")+`]`),
		reusingPanelDoc(1, 0, target, ""),
	))
	if got := specPanelOf(t, inRow, "p1").Queries[0].Source; got == nil || got.Panel != "p2" {
		t.Fatalf("source = %+v, want p2 inside a collapsed row", got)
	}

	reusingInRow := convertedSpec(t, dashboardJSON("",
		sourcePanelDoc(2, 0, ""),
		rowJSON(10, 4, true, `"panels":[`+reusingPanelDoc(1, 0, target, "")+`]`),
	))
	if got := specPanelOf(t, reusingInRow, "p1").Queries[0].Source; got == nil || got.Panel != "p2" {
		t.Fatalf("source = %+v, want p2 for a panel inside a collapsed row", got)
	}
}

func TestDashboardSourceThatCannotBeReusedMakesAPlaceholder(t *testing.T) {
	target := reuseTarget(`"panelId":1`)
	cases := []struct {
		name string
		doc  string
		id   string
	}{
		{"no such panel", dashboardJSON("", sourcePanelDoc(3, 0, ""), reusingPanelDoc(2, 6, target, "")), "p2"},
		{"a source that is a text panel", dashboardJSON("", panelJSON(1, 0, 0, 6, 4, `"content":"hi"`), reusingPanelDoc(2, 6, target, "")), "p2"},
		{"a source that is a placeholder", dashboardJSON("", `{"id":1,"type":"vendor-panel","title":"Vendor","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":`+promRef+`,"targets":[{"refId":"A","expr":"up"}]}`, reusingPanelDoc(2, 6, target, "")), "p2"},
		{"a panel reusing itself", dashboardJSON("", reusingPanelDoc(1, 0, target, "")), "p1"},
		{"a chain", dashboardJSON("", sourcePanelDoc(3, 0, ""), reusingPanelDoc(1, 6, reuseTarget(`"panelId":3`), ""), reusingPanelDoc(2, 12, target, "")), "p2"},
		{"a panel id that is not a number", dashboardJSON("", sourcePanelDoc(1, 0, ""), reusingPanelDoc(2, 6, reuseTarget(`"panelId":"one"`), "")), "p2"},
		{"no panel id", dashboardJSON("", sourcePanelDoc(1, 0, ""), reusingPanelDoc(2, 6, reuseTarget(`"withTransforms":true`), "")), "p2"},
		{
			"a source that repeats",
			dashboardJSON(templating(`{"name":"env","type":"custom","query":"a,b","options":[{"value":"a"},{"value":"b"}],"current":{"value":"a"}}`),
				sourcePanelDoc(1, 0, `,"repeat":"env"`), reusingPanelDoc(2, 6, target, "")),
			"p2",
		},
		{
			"a source in a repeating row",
			dashboardJSON(templating(`{"name":"env","type":"custom","query":"a,b","options":[{"value":"a"},{"value":"b"}],"current":{"value":"a"}}`),
				rowJSON(10, 0, true, `"repeat":"env","panels":[`+sourcePanelDoc(1, 0, "")+`]`), reusingPanelDoc(2, 0, target, "")),
			"p2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertedSpec(t, tc.doc)
			panel := specPanelOf(t, res, tc.id)
			if panel.Type != spec.Placeholder {
				t.Fatalf("panel = %+v, want a placeholder", panel)
			}
			line := panelLine(t, res, tc.id)
			if !strings.Contains(strings.Join(line.Reasons, " "), reusedPanelMissingReason) {
				t.Fatalf("reasons = %q, want %q", line.Reasons, reusedPanelMissingReason)
			}
		})
	}
}

func TestDashboardSourceDropsTheQuerySettingsOfTheReusingPanel(t *testing.T) {
	extra := `,"interval":"30s","maxDataPoints":500,"timeFrom":"1h","timeShift":"1h"`
	res := convertedSpec(t, dashboardJSON("", sourcePanelDoc(1, 0, ""), reusingPanelDoc(2, 6, reuseTarget(`"panelId":1`), extra)))

	panel := specPanelOf(t, res, "p2")
	if panel.Interval != "" || panel.MaxDataPoints != 0 || panel.TimeFrom != "" || panel.TimeShift != "" {
		t.Fatalf("panel = %+v, want no query settings", panel)
	}
	line := panelLine(t, res, "p2")
	if line.Status != statusPartial {
		t.Fatalf("status = %q, want partial", line.Status)
	}
	for _, path := range []string{"interval", "maxDataPoints", "timeFrom", "timeShift"} {
		if !contains(line.Paths, path) {
			t.Errorf("paths = %q, want %q reported", line.Paths, path)
		}
	}
	if !contains(line.Reasons, reusedSettingReason) {
		t.Errorf("reasons = %q, want %q", line.Reasons, reusedSettingReason)
	}
}

func TestDashboardSourceKeepsTheSourceSettingsOfThePanelItReuses(t *testing.T) {
	res := convertedSpec(t, dashboardJSON("", sourcePanelDoc(1, 0, `,"interval":"30s","maxDataPoints":500,"timeFrom":"1h"`), reusingPanelDoc(2, 6, reuseTarget(`"panelId":1`), "")))

	source := specPanelOf(t, res, "p1")
	if source.Interval != "30s" || source.MaxDataPoints != 500 || source.TimeFrom != "now-1h" {
		t.Fatalf("source = %+v, want its own settings", source)
	}
	if line := panelLine(t, res, "p2"); line.Status != statusImported {
		t.Fatalf("line = %+v, want imported", line)
	}
}

func TestDashboardSourceNextToOtherQueriesIsDropped(t *testing.T) {
	targets := `{"refId":"A","expr":"up","datasource":` + promRef + `},` + reuseTarget(`"panelId":1`)
	panel := `{"id":2,"type":"timeseries","title":"Mixed","gridPos":{"x":6,"y":0,"w":6,"h":4},"datasource":` + promRef + `,"targets":[` + targets + `]}`
	res := convertedSpec(t, dashboardJSON("", sourcePanelDoc(1, 0, ""), panel))

	got := specPanelOf(t, res, "p2")
	if got.Datasource != "prom" || len(got.Queries) != 1 || got.Queries[0].Query != "up" || got.Queries[0].Source != nil {
		t.Fatalf("panel = %+v, want only the prometheus query", got)
	}
	if line := panelLine(t, res, "p2"); line.Status != statusPartial || !contains(line.Reasons, mixedSourceReason) {
		t.Fatalf("line = %+v, want partial with %q", line, mixedSourceReason)
	}
}

func TestDashboardSourceOverridesMatchTheSourcesQueryRefs(t *testing.T) {
	source := `{"id":1,"type":"timeseries","title":"Source","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":` + promRef + `,"targets":[{"refId":"Z","expr":"up","datasource":` + promRef + `}]}`
	overrides := `,"fieldConfig":{"defaults":{},"overrides":[{"matcher":{"id":"byFrameRefID","options":"Z"},"properties":[{"id":"unit","value":"s"}]}]}`
	res := convertedSpec(t, dashboardJSON("", source, reusingPanelDoc(2, 6, reuseTarget(`"panelId":1`), overrides)))

	got := specPanelOf(t, res, "p2")
	if len(got.Overrides) != 1 || got.Overrides[0].Match.Query != "Z" {
		t.Fatalf("overrides = %+v, want one matching query Z", got.Overrides)
	}
	if line := panelLine(t, res, "p2"); line.Status != statusImported {
		t.Fatalf("line = %+v, want imported", line)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
