package grafana

import (
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestTargetLogBaseImportsWithoutANote(t *testing.T) {
	doc := panelDoc("graph", `"targets":[{"refId":"A","expr":"up","logBase":1}]`)
	panel, line := convertedPanel(t, doc)
	if len(panel.Queries) != 1 || line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("queries = %d, status %s, paths %v, want one query imported with no report", len(panel.Queries), line.Status, line.Paths)
	}
	requireLine(t, blocksOf(t, doc), "panels[0].targets[0]", "logBase", outcomeNoEffect)
}

func TestTargetKeysOutsideTheIgnoredTableStillReport(t *testing.T) {
	_, line := convertedPanel(t, panelDoc("graph", `"targets":[{"refId":"A","expr":"up","mystery":1}]`))
	if line.Status != statusPartial {
		t.Fatalf("status = %s, paths %v, want partial", line.Status, line.Paths)
	}
}

func TestExemplarOnAStatPanelRecordsNoEffect(t *testing.T) {
	doc := panelDoc("stat", `"targets":[{"refId":"A","expr":"up","exemplar":true}]`)
	panel, line := convertedPanel(t, doc)
	if panel.Queries[0].Exemplar || line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("query = %+v, status %s, paths %v, want imported without exemplar", panel.Queries[0], line.Status, line.Paths)
	}
	requireLine(t, blocksOf(t, doc), "panels[0].targets[0]", "exemplar", outcomeNoEffect)
}

func TestEmptyQueryIsDroppedWithoutANoteBesideARealOne(t *testing.T) {
	cases := []struct{ name, target string }{
		{"prometheus", `{"refId":"B","expr":"   ","legendFormat":"x"}`},
		{"official clickhouse", `{"refId":"B","rawSql":"","queryType":"timeseries"}`},
		{"altinity", `{"refId":"B","query":"","dateTimeType":"DATETIME"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := panelDoc("timeseries", `"targets":[{"refId":"A","expr":"up"},`+tc.target+`]`)
			panel, line := convertedPanel(t, doc)
			if len(panel.Queries) != 1 || panel.Queries[0].Query != "up" {
				t.Fatalf("queries = %+v, want only the real one", panel.Queries)
			}
			if line.Status != statusImported || len(line.Paths) != 0 {
				t.Fatalf("status = %s, paths %v, want imported with no report", line.Status, line.Paths)
			}
			if line.Queries[1].Dropped == nil || *line.Queries[1].Dropped != emptyQueryReason {
				t.Fatalf("query lines = %+v, want the empty query listed as dropped", line.Queries)
			}
		})
	}
}

func TestEmptyQueryOptionsAreRecordedAsNoEffect(t *testing.T) {
	doc := panelDoc("timeseries", `"targets":[{"refId":"A","expr":"up"},{"refId":"B","expr":"","legendFormat":"x","exemplar":true}]`)
	blocks := blocksOf(t, doc)
	requireLine(t, blocks, "panels[0].targets[1]", "legendFormat", outcomeNoEffect)
	requireLine(t, blocks, "panels[0].targets[1]", "exemplar", outcomeNoEffect)
}

func TestEmptyQueryAloneKeepsTheNoQueryPlaceholder(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("timeseries", `"targets":[{"refId":"A","expr":""}]`))
	if panel.Type != spec.Placeholder || line.Status != statusPlaceholder {
		t.Fatalf("type = %s, status %s, want the placeholder", panel.Type, line.Status)
	}
}

func TestIgnoredUnitScaleAppliesToEveryPanelType(t *testing.T) {
	for _, typ := range []spec.PanelType{spec.Timeseries, spec.Stat, spec.Gauge, spec.Heatmap, spec.Table, spec.PanelType("graph")} {
		if !isIgnoredOption(panelScope(typ), "fieldConfig.defaults.unitScale") {
			t.Errorf("unitScale is not ignored on %s", typ)
		}
	}
	if isIgnoredOption(variableScope("query"), "fieldConfig.defaults.unitScale") {
		t.Error("unitScale is ignored outside panels")
	}
}

func TestIgnoredUnitScaleImportsWithoutANote(t *testing.T) {
	_, line := convertedPanel(t, panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{"unitScale":true},"overrides":[]}`))
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths %v, want imported with no report", line.Status, line.Paths)
	}
}

func TestSilentScopedVarsImportWithoutANote(t *testing.T) {
	doc := panelDoc("graph", upTarget+`,"scopedVars":{"host":{"selected":false,"text":"a","value":"a"}}`)
	_, line := convertedPanel(t, doc)
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths %v, want imported with no report", line.Status, line.Paths)
	}
	requireLine(t, blocksOf(t, doc), "panels[0]", "scopedVars", outcomeBookkeeping)
}

func TestStatLegendOptionsAreIgnoredAndOtherPanelsKeepReportingThem(t *testing.T) {
	stat := `{"legend":{"calcs":["mean"],"displayMode":"table","placement":"right","showLegend":false}}`
	_, line := convertedPanel(t, panelDoc("stat", upTarget+`,"options":`+stat))
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("stat: status = %s, paths %v, want imported with no report", line.Status, line.Paths)
	}
	_, line = convertedPanel(t, panelDoc("gauge", upTarget+`,"options":`+stat))
	if line.Status != statusPartial {
		t.Fatalf("gauge: status = %s, paths %v, want the legend options reported", line.Status, line.Paths)
	}
}
