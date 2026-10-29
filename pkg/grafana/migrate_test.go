package grafana

import (
	"fmt"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestLegacyPanelWithoutUsableHeightTakesTheRowHeight(t *testing.T) {
	res := convertDoc(t, `{"title":"T","schemaVersion":12,"rows":[{"height":"250px","panels":[
		{"id":1,"type":"text","span":6,"height":0},
		{"id":2,"type":"text","span":6,"height":"380px"},
		{"id":3,"type":"text","span":6},
		{"id":4,"type":"text","span":6,"height":"tall"},
		{"id":5,"type":"text","span":6,"height":"114.9px"}
	]}]}`)
	want := map[string]spec.GridPos{
		"p1": {X: 0, Y: 0, W: 12, H: 7},
		"p2": {X: 12, Y: 0, W: 12, H: 10},
		"p3": {X: 0, Y: 7, W: 12, H: 7},
		"p4": {X: 12, Y: 10, W: 12, H: 7},
		"p5": {X: 0, Y: 14, W: 12, H: 3},
	}
	for id, grid := range want {
		if got := gridOf(t, res, id); got != grid {
			t.Errorf("%s = %+v, want %+v", id, got, grid)
		}
	}
}

func TestRepeatClonesNeedARepeatMarker(t *testing.T) {
	res := convertDoc(t, dashboardJSON("",
		panelJSON(1, 0, 0, 12, 4, `"repeatIteration":1700000000000`),
		panelJSON(2, 12, 0, 12, 4, `"repeat":"host","repeatIteration":1700000000000`),
		panelJSON(3, 0, 4, 12, 4, `"repeatPanelId":2`),
		panelJSON(4, 12, 4, 12, 4, `"repeatedByRow":true`),
	))
	for _, id := range []string{"p1", "p2"} {
		if !hasLine(res, id) {
			t.Errorf("%s was dropped though nothing marks it as a clone", id)
		}
	}
	for _, id := range []string{"p3", "p4"} {
		if hasLine(res, id) {
			t.Errorf("%s is a repeat clone and should be skipped", id)
		}
	}
	notes := notesFor(res, "repeated panels")
	if len(notes) != 1 || !hasPath(notes, "panels[2]") || !hasPath(notes, "panels[3]") || hasPath(notes, "panels[0]") {
		t.Fatalf("repeated panels notes = %+v", notes)
	}
}

func TestLegacyRowsUseRepeatIterationToFindClones(t *testing.T) {
	res := convertDoc(t, `{"title":"T","schemaVersion":12,"rows":[
		{"title":"Host","repeat":"host","repeatIteration":5,"panels":[{"id":1,"type":"text","span":12}]},
		{"title":"Host","repeatIteration":5,"panels":[{"id":2,"type":"text","span":12}]}
	]}`)
	if !hasLine(res, "p1") || hasLine(res, "p2") {
		t.Fatalf("panel lines = %+v, want only p1; notes %+v", res.Report.Panels, res.Report.Notes)
	}
	if notes := notesFor(res, "repeated panels"); len(notes) != 1 || !hasPath(notes, "rows[1]") || hasPath(notes, "rows[0]") {
		t.Fatalf("repeated panels notes = %+v", notes)
	}
}

func TestOpenRowIgnoresLeftoverPanels(t *testing.T) {
	stale := panelJSON(7, 0, 1, 12, 4, "")
	doc := dashboardJSON("", rowJSON(5, 0, false, `"panels":[`+stale+`]`), panelJSON(8, 0, 1, 12, 4, ""))
	res := convertDoc(t, doc)
	if hasLine(res, "p7") || !hasLine(res, "p8") {
		t.Fatalf("panel lines = %+v, want only p8", res.Report.Panels)
	}
	if notes := notesFor(res, "rows"); len(notes) != 1 || !hasPath(notes, "panels[0].panels") {
		t.Fatalf("rows notes = %+v", notes)
	}
	if summary, err := Detect([]byte(doc)); err != nil || summary.Panels != 1 {
		t.Fatalf("summary = %+v, err = %v, want 1 panel", summary, err)
	}
}

func TestRenamedAndRetiredPanelTypesImportAsTheirModernType(t *testing.T) {
	cases := []struct{ name, typ, members, want string }{
		{"datagrid", "datagrid", ``, "table"},
		{"new logs", "logs-new", ``, "logs"},
		{"logs table", "logstable", ``, "table"},
		{"singlestat plugin as stat", "grafana-singlestat-panel", ``, "stat"},
		{"singlestat plugin with a gauge", "grafana-singlestat-panel", `"gauge":{"show":true,"minValue":0,"maxValue":100}`, "gauge"},
		{"singlestat as stat", "singlestat", ``, "stat"},
		{"singlestat with a gauge", "singlestat", `"gauge":{"show":true,"minValue":0,"maxValue":100}`, "gauge"},
		{"old table", "table-old", ``, "table"},
		{"old graph", "graph", ``, "timeseries"},
		{"old pie chart", "grafana-piechart-panel", ``, "piechart"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			members := upTarget
			if tc.members != "" {
				members += "," + tc.members
			}
			panel, line := convertedPanel(t, panelDoc(tc.typ, members))
			if string(panel.Type) != tc.want || line.Type != tc.want || line.SourceType != tc.typ {
				t.Fatalf("type = %s, line type = %s, source type = %s, want %s from %s", panel.Type, line.Type, line.SourceType, tc.want, tc.typ)
			}
		})
	}
}

func TestPluginSinglestatTakesTheLegacyOptions(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("grafana-singlestat-panel", upTarget+`,"format":"percentunit","valueName":"avg","thresholds":"0.01,0.05","colors":["#299c46","rgba(237, 129, 40, 0.89)","#d44a3a"],"colorValue":true`))
	if panel.Unit != "percentunit" || panel.Display == nil || panel.Display.Reduce != "mean" || len(panel.Thresholds) != 3 {
		t.Fatalf("panel = %+v display = %+v", panel, panel.Display)
	}
	if len(line.Paths) != 0 {
		t.Fatalf("paths = %v", line.Paths)
	}
}

func TestDatagridOptionsAreReportedBecauseCellsCannotBeEdited(t *testing.T) {
	const reason = "Editing cells is not supported; the grid is shown as a table."
	cases := []struct {
		name, options string
		paths         []string
	}{
		{"no options", ``, nil},
		{"selected series", `"options":{"selectedSeries":0}`, []string{"options.selectedSeries"}},
		{"other options", `"options":{"selectedSeries":1,"cellHeight":"lg"}`, []string{"options.cellHeight", "options.selectedSeries"}},
		{"defaults only", `"options":{"cellHeight":"sm","showHeader":true}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			members := upTarget
			if tc.options != "" {
				members += "," + tc.options
			}
			_, line := convertedPanel(t, panelDoc("datagrid", members))
			if !slices.Equal(line.Paths, tc.paths) {
				t.Fatalf("paths = %v, want %v", line.Paths, tc.paths)
			}
			if len(tc.paths) > 0 && !slices.Contains(line.Reasons, reason) {
				t.Fatalf("reasons = %v, want %q", line.Reasons, reason)
			}
		})
	}
}

func TestPanelTypesWithoutAnEquivalentSayWhy(t *testing.T) {
	cases := []struct{ typ, reason string }{
		{"annolist", "SlashObserve has no annotations yet, so an annotation list has nothing to show."},
		{"welcome", "Grafana's home page panels have no SlashObserve equivalent."},
		{"gettingstarted", "Grafana's home page panels have no SlashObserve equivalent."},
		{"debug", "Grafana's debug and live panels have no SlashObserve equivalent."},
		{"live", "Grafana's debug and live panels have no SlashObserve equivalent."},
		{"alertGroups", "Alertmanager is not supported yet."},
		{"vendor-chart-panel", "Grafana panel type vendor-chart-panel is not supported yet."},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			res := convertDoc(t, dashboardJSON("", fmt.Sprintf(`{"id":1,"type":%q,"title":"T","gridPos":{"x":0,"y":0,"w":6,"h":4}}`, tc.typ)))
			line := panelLine(t, res, "p1")
			if line.Status != statusPlaceholder || len(line.Reasons) == 0 || line.Reasons[0] != tc.reason {
				t.Fatalf("status = %s, reasons = %v, want %q", line.Status, line.Reasons, tc.reason)
			}
		})
	}
}

func TestDatasourceReasonBeatsThePanelTypeReason(t *testing.T) {
	doc := dashboardJSON("", `{"id":1,"type":"annolist","title":"N","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":{"type":"influxdb","uid":"flux"}}`)
	res := convertDoc(t, doc)
	line := panelLine(t, res, "p1")
	if line.Status != statusPlaceholder || line.Reasons[0] == "SlashObserve has no annotations yet, so an annotation list has nothing to show." {
		t.Fatalf("reasons = %v, want the datasource reason first", line.Reasons)
	}
}
