package grafana

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const panelFrame = `"id":1,"title":"T","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"}`

const upTarget = `"targets":[{"refId":"A","expr":"up"}]`

func panelDoc(typ, members string) string {
	return fmt.Sprintf(`{"title":"T","schemaVersion":39,"panels":[{%s,"type":%q,%s}]}`, panelFrame, typ, members)
}

func convertedPanel(t *testing.T, doc string) (spec.Panel, PanelLine) {
	t.Helper()
	res := convertDoc(t, doc)
	if !res.Ready() {
		t.Fatalf("result is not ready: %+v", res.Datasources)
	}
	entry, ok := res.Spec.FindPanel("p1")
	if !ok {
		t.Fatalf("no panel p1 in the Spec\n%s", doc)
	}
	return *entry.Panel, panelLine(t, res, "p1")
}

func graphPanel(t *testing.T, members string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("graph", upTarget+","+members))
}

func singlestatPanel(t *testing.T, members string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("singlestat", upTarget+","+members))
}

func displayOf(t *testing.T, panel spec.Panel) spec.Display {
	t.Helper()
	if panel.Display == nil {
		t.Fatalf("panel has no display: %+v", panel)
	}
	return *panel.Display
}

func TestLegacyGraphLegendSurvivesTheDisplayStep(t *testing.T) {
	cases := []struct{ name, legend, want string }{
		{"hidden", `{"show":false}`, "hidden"},
		{"right side", `{"show":true,"rightSide":true}`, "right"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := graphPanel(t, `"legend":`+tc.legend)
			if got := displayOf(t, panel).Legend; got != tc.want {
				t.Fatalf("legend = %q, want %q", got, tc.want)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestSinglestatColorModeSurvivesTheDisplayStep(t *testing.T) {
	cases := []struct{ name, members, want string }{
		{"background", `"colorBackground":true`, "background"},
		{"neither", `"colorBackground":false,"colorValue":false`, "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, _ := singlestatPanel(t, tc.members)
			if got := displayOf(t, panel).ColorMode; got != tc.want {
				t.Fatalf("colorMode = %q, want %q", got, tc.want)
			}
		})
	}
}

func thresholdText(steps []spec.Threshold) []string {
	out := make([]string, len(steps))
	for i, step := range steps {
		out[i] = step.Color
		if step.Value != nil {
			out[i] += fmt.Sprintf("@%g", *step.Value)
		}
	}
	return out
}

func TestLegacyGraphThresholdsKeepTheirColorMode(t *testing.T) {
	cases := []struct{ mode, want string }{
		{"critical", "critical@80"},
		{"warning", "warning@80"},
		{"ok", "ok@80"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			panel, line := graphPanel(t, `"thresholds":[{"colorMode":"`+tc.mode+`","op":"gt","value":80,"fill":true,"line":true}]`)
			if got := thresholdText(panel.Thresholds); !slices.Equal(got, []string{tc.want}) {
				t.Fatalf("thresholds = %v, want [%s]", got, tc.want)
			}
			if len(line.Paths) != 0 {
				t.Fatalf("paths = %v, reasons = %v", line.Paths, line.Reasons)
			}
		})
	}
}

func TestSeriesOverridesWithUnparseableValuesAreReportedNotWritten(t *testing.T) {
	cases := []struct{ name, member, path string }{
		{"stack group", `"stack":"A"`, "seriesOverrides[0].stack"},
		{"fill text", `"fill":"abc"`, "seriesOverrides[0].fill"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := graphPanel(t, `"seriesOverrides":[{"alias":"errors","color":"#ff0000",`+tc.member+`}]`)
			if len(panel.Overrides) != 1 {
				t.Fatalf("overrides = %+v, want only the color", panel.Overrides)
			}
			got := panel.Overrides[0]
			if got.Match.Series != "errors" || got.Color != "#ff0000" {
				t.Fatalf("override = %+v", got)
			}
			if got.Stack != "" || got.Fill != nil {
				t.Fatalf("override wrote stack %q fill %v for an unparseable value", got.Stack, got.Fill)
			}
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v, want %s reported", line.Status, line.Paths, tc.path)
			}
		})
	}
}

func clickHousePanel(t *testing.T, sql string) (spec.Panel, PanelLine) {
	t.Helper()
	target, err := json.Marshal(map[string]any{"refId": "A", "rawSql": sql})
	if err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf(`{"title":"T","schemaVersion":39,"panels":[{"id":1,"type":"timeseries","title":"T","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"grafana-clickhouse-datasource","uid":"ch"},"targets":[%s]}]}`, target)
	res := convertDoc(t, doc)
	line := panelLine(t, res, "p1")
	if line.Status == statusPlaceholder {
		return spec.Panel{}, line
	}
	entry, ok := res.Spec.FindPanel("p1")
	if !ok {
		t.Fatalf("no panel p1 in the Spec")
	}
	return *entry.Panel, line
}

func TestTimeGroupIntervalUnitsAreNotGuessedFromTheirPrefix(t *testing.T) {
	cases := []struct{ interval, want string }{
		{"5m", "toStartOfInterval(toDateTime(ts), INTERVAL 5 MINUTE)"},
		{"3 minutes", "toStartOfInterval(toDateTime(ts), INTERVAL 3 MINUTE)"},
		{"10s", "toStartOfInterval(toDateTime(ts), INTERVAL 10 SECOND)"},
		{"1 day", "toStartOfInterval(toDateTime(ts), INTERVAL 1 DAY)"},
		{"500ms", ""},
		{"2 ms", ""},
	}
	for _, tc := range cases {
		t.Run(tc.interval, func(t *testing.T) {
			panel, line := clickHousePanel(t, "SELECT $__timeGroup(ts, '"+tc.interval+"') AS t FROM logs")
			if tc.want != "" {
				if got := panel.Queries[0].Query; got != "SELECT "+tc.want+" AS t FROM logs" {
					t.Fatalf("query = %q", got)
				}
				return
			}
			dropped := line.Queries[0].Dropped
			if line.Status != statusPlaceholder || dropped == nil || !strings.Contains(*dropped, "$__timeGroup(ts, '"+tc.interval+"')") {
				t.Fatalf("status %s, dropped %v, want the macro named as unsupported", line.Status, dropped)
			}
		})
	}
}

func TestOverrideByRefIDFollowsTheFirstQueryOfADuplicatedRefID(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("timeseries", `"targets":[{"refId":"A","expr":"up"},{"refId":"A","expr":"down"}],
		"fieldConfig":{"defaults":{},"overrides":[{"matcher":{"id":"byFrameRefID","options":"A"},"properties":[{"id":"unit","value":"bytes"}]}]}`))
	if len(panel.Overrides) != 1 {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
	ref := panel.Overrides[0].Match.Query
	for i := range panel.Queries {
		if panel.QueryRef(i) == ref && panel.Queries[i].Query != "up" {
			t.Fatalf("the override targets %s, which runs %q, want the first query", ref, panel.Queries[i].Query)
		}
	}
	if !slices.Contains(line.Queries[1].Rewrites, "refId A -> B") {
		t.Fatalf("rewrites of the second query = %v", line.Queries[1].Rewrites)
	}
}

func TestHugeNumbersOfUnknownOptionsAreCappedWithoutSlowdown(t *testing.T) {
	const leaves = 200000
	var options strings.Builder
	options.WriteString(`"options":{`)
	for i := range leaves {
		if i > 0 {
			options.WriteByte(',')
		}
		fmt.Fprintf(&options, `"unknown%d":1`, i)
	}
	options.WriteString("}")
	doc := panelDoc("timeseries", upTarget+","+options.String())

	start := time.Now()
	_, line := convertedPanel(t, doc)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("converting %d unknown options took %s", leaves, elapsed)
	}
	if line.Status != statusPartial || len(line.Paths) != 100 {
		t.Fatalf("status %s with %d paths, want partial with the cap of 100", line.Status, len(line.Paths))
	}
	if want := fmt.Sprintf("+%d more options were not imported", leaves-100); !slices.Contains(line.Reasons, want) {
		t.Fatalf("reasons = %v, want %q", line.Reasons, want)
	}
	if len(line.Reasons) != 2 {
		t.Fatalf("reasons = %v, want the shared reason and the overflow count only", line.Reasons)
	}
}

func TestDroppedTargetTakesItsPartialPathsAndReasonsWithIt(t *testing.T) {
	longLegend := strings.Repeat("l", 250)
	panel, line := convertedPanel(t, panelDoc("timeseries", `"targets":[
		{"refId":"A","expr":"up{pod=\"$undeclared\"}","legendFormat":"`+longLegend+`"},
		{"refId":"B","expr":"up"}]`))
	if len(panel.Queries) != 1 {
		t.Fatalf("queries = %+v, want only the second target", panel.Queries)
	}
	if !slices.Equal(line.Paths, []string{"targets[0]"}) {
		t.Fatalf("paths = %v, want only the dropped target", line.Paths)
	}
	for _, reason := range line.Reasons {
		if strings.Contains(reason, "legend was cut") {
			t.Fatalf("reasons = %v keep a note about a query that was dropped", line.Reasons)
		}
	}
}

func TestRegexPatternsArriveVerbatim(t *testing.T) {
	override := func(typ, members string) func(*testing.T) string {
		return func(t *testing.T) string {
			panel, _ := convertedPanel(t, panelDoc(typ, upTarget+","+members))
			if len(panel.Overrides) != 1 {
				t.Fatalf("overrides = %+v", panel.Overrides)
			}
			return panel.Overrides[0].Match.SeriesRegex
		}
	}
	cases := []struct {
		name string
		run  func(*testing.T) string
		want string
	}{
		{"graph alias with slashes", override("graph", `"seriesOverrides":[{"alias":"/^api-.*/","color":"#00ff00"}]`), "/^api-.*/"},
		{"graph alias with flags", override("graph", `"seriesOverrides":[{"alias":"/err.*/i","color":"#00ff00"}]`), "/err.*/i"},
		{"graph alias color with slashes", override("graph", `"aliasColors":{"/^api-.*/":"#00ff00"}`), "/^api-.*/"},
		{"table-old style pattern", override("table-old", `"styles":[{"pattern":"/^col.*/","unit":"bytes","type":"number"}]`), "/^col.*/"},
		{"byRegexp without slashes", override("timeseries", `"fieldConfig":{"defaults":{},"overrides":[{"matcher":{"id":"byRegexp","options":"^db-.*"},"properties":[{"id":"unit","value":"bytes"}]}]}`), "^db-.*"},
		{"byRegexp with slashes", override("timeseries", `"fieldConfig":{"defaults":{},"overrides":[{"matcher":{"id":"byRegexp","options":"/db-.*/i"},"properties":[{"id":"unit","value":"bytes"}]}]}`), "/db-.*/i"},
		{"regex mapping", func(t *testing.T) string {
			panel, _ := convertedPanel(t, panelDoc("stat", upTarget+`,"fieldConfig":{"defaults":{"mappings":[{"type":"regex","options":{"pattern":"/^a.*/i","result":{"text":"A"}}}]},"overrides":[]}`))
			return panel.Mappings[0].Regex
		}, "/^a.*/i"},
		{"filter fields pattern", func(t *testing.T) string {
			panel, _ := convertedPanel(t, panelDoc("table", upTarget+`,"transformations":[{"id":"filterFieldsByName","options":{"include":{"pattern":"/^cpu/"}}}]`))
			return panel.Transform[0].IncludeRegex
		}, "/^cpu/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.run(t); got != tc.want {
				t.Fatalf("pattern = %q, want %q", got, tc.want)
			}
		})
	}
}

func statWithSteps(t *testing.T, steps string) spec.Panel {
	t.Helper()
	panel, _ := convertedPanel(t, panelDoc("stat", upTarget+`,"fieldConfig":{"defaults":{"thresholds":{"mode":"absolute","steps":[`+steps+`]}},"overrides":[]}`))
	return panel
}

func TestFirstThresholdStepIsTheBaseWhateverItsValue(t *testing.T) {
	cases := []struct{ name, steps string }{
		{"null base", `{"color":"green","value":null},{"color":"red","value":80}`},
		{"base with a value", `{"color":"green","value":10},{"color":"red","value":80}`},
		{"unsorted", `{"color":"red","value":80},{"color":"green","value":10}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := thresholdText(statWithSteps(t, tc.steps).Thresholds)
			if want := []string{"ok", "critical@80"}; !slices.Equal(got, want) {
				t.Fatalf("thresholds = %v, want %v", got, want)
			}
		})
	}
}

func TestLegacyGraphThresholdsKeepTheirOrderBelowTheFirstValue(t *testing.T) {
	panel, _ := graphPanel(t, `"thresholds":[
		{"colorMode":"warning","op":"gt","value":50},
		{"colorMode":"critical","op":"gt","value":90}]`)
	if got, want := thresholdText(panel.Thresholds), []string{"warning@50", "critical@90"}; !slices.Equal(got, want) {
		t.Fatalf("thresholds = %v, want %v", got, want)
	}
}

func TestSinglestatThresholdsFollowTheirColors(t *testing.T) {
	const colors = `"colors":["#299c46","rgba(237, 129, 40, 0.89)","#d44a3a"]`
	cases := []struct {
		name, members string
		want          []string
	}{
		{"two levels", colors + `,"thresholds":"50,80"`, []string{"ok", "warning@50", "critical@80"}},
		{"empty levels keep the first color as the base", colors + `,"thresholds":""`, []string{"ok"}},
		{"missing levels keep the first color as the base", colors, []string{"ok"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, _ := singlestatPanel(t, tc.members)
			if got := thresholdText(panel.Thresholds); !slices.Equal(got, tc.want) {
				t.Fatalf("thresholds = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSinglestatWithoutASparklineShowsNoGraph(t *testing.T) {
	cases := []struct {
		name, members string
		wantGraph     bool
	}{
		{"no sparkline key", `"valueName":"avg"`, false},
		{"sparkline off", `"sparkline":{"show":false}`, false},
		{"sparkline on", `"sparkline":{"show":true}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, _ := singlestatPanel(t, tc.members)
			graph := panel.Display == nil || panel.Display.Graph == nil || *panel.Display.Graph
			if graph != tc.wantGraph {
				t.Fatalf("graph = %v, want %v (display %+v)", graph, tc.wantGraph, panel.Display)
			}
		})
	}
}

func calculateFields(t *testing.T, transformation string) []spec.Transform {
	t.Helper()
	panel, _ := convertedPanel(t, panelDoc("table", upTarget+`,"transformations":[`+transformation+`]`))
	return panel.Transform
}

func TestCalculateFieldDefaultsToTotalOfTheRow(t *testing.T) {
	cases := []struct {
		name, transformation string
		want                 spec.Transform
	}{
		{"no options", `{"id":"calculateField"}`, spec.Transform{Type: "calculateField", ReduceRow: "sum", Alias: "Total"}},
		{"empty options", `{"id":"calculateField","options":{}}`, spec.Transform{Type: "calculateField", ReduceRow: "sum", Alias: "Total"}},
		{"alias only", `{"id":"calculateField","options":{"alias":"Sum of all"}}`, spec.Transform{Type: "calculateField", ReduceRow: "sum", Alias: "Sum of all"}},
		{"reducer without mode", `{"id":"calculateField","options":{"reduce":{"reducer":"max"},"alias":"Peak"}}`, spec.Transform{Type: "calculateField", ReduceRow: "max", Alias: "Peak"}},
		{"binary mode stays binary", `{"id":"calculateField","options":{"mode":"binary","binary":{"left":"a","operator":"+","right":"b"},"alias":"ab"}}`, spec.Transform{Type: "calculateField", Expr: "a + b", Alias: "ab"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateFields(t, tc.transformation)
			if len(got) != 1 || !reflect.DeepEqual(got[0], tc.want) {
				t.Fatalf("transform = %+v, want [%+v]", got, tc.want)
			}
		})
	}
}

func TestOverrideKeysWithoutAConversionAreReported(t *testing.T) {
	override := func(members string) string {
		return `"fieldConfig":{"defaults":{},"overrides":[{` + members + `}]}`
	}
	const matcher = `"matcher":{"id":"byName","options":"errors"}`
	const unit = `"properties":[{"id":"unit","value":"bytes"}]`
	cases := []struct {
		name, members, path string
	}{
		{"override key", matcher + `,` + unit + `,"description":"kept by hand"`, "fieldConfig.overrides[0].description"},
		{"matcher key", `"matcher":{"id":"byName","options":"errors","scope":"series"},` + unit, "fieldConfig.overrides[0].matcher.scope"},
		{"property key", matcher + `,"properties":[{"id":"unit","value":"bytes","description":"x"}]`, "fieldConfig.overrides[0].properties[0].description"},
		{"value key", matcher + `,"properties":[{"id":"custom.lineStyle","value":{"fill":"dash","dash":[10,10],"extra":true}}]`, "fieldConfig.overrides[0].properties[0].value.extra"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc("timeseries", upTarget+","+override(tc.members)))
			if len(panel.Overrides) != 1 {
				t.Fatalf("overrides = %+v, want the convertible part kept", panel.Overrides)
			}
			if line.Status != statusPartial || !slices.Equal(line.Paths, []string{tc.path}) {
				t.Fatalf("status %s, paths %v, want exactly %s", line.Status, line.Paths, tc.path)
			}
		})
	}
}

func TestFullyConvertedOverrideReportsNothing(t *testing.T) {
	_, line := convertedPanel(t, panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{},"overrides":[
		{"matcher":{"id":"byName","options":"errors"},"properties":[
			{"id":"unit","value":"bytes"},{"id":"color","value":{"mode":"fixed","fixedColor":"red"}},
			{"id":"custom.lineStyle","value":{"fill":"dash","dash":[10,10]}},
			{"id":"custom.hideFrom","value":{"legend":false,"tooltip":false,"viz":true}},
			{"id":"thresholds","value":{"mode":"absolute","steps":[{"color":"green","value":null}]}}]}]}`))
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status %s, paths %v, reasons %v", line.Status, line.Paths, line.Reasons)
	}
}

func TestDatasourceVariableIsNotedOnceForTheDashboardNotPerPanel(t *testing.T) {
	panels := make([]string, 3)
	for i := range panels {
		panels[i] = fmt.Sprintf(`{"id":%d,"type":"stat","title":"P%d","gridPos":{"x":%d,"y":0,"w":8,"h":4},"datasource":{"type":"prometheus","uid":"${ds}"},"targets":[{"refId":"A","expr":"up"}]}`, i+1, i+1, i*8)
	}
	doc := `{"title":"T","schemaVersion":39,"templating":{"list":[{"name":"ds","type":"datasource","query":"prometheus","current":{"text":"prom","value":"prom"}}]},"panels":[` + strings.Join(panels, ",") + `]}`
	res := convertDoc(t, doc)

	want := "Datasource variable $ds now uses prom."
	var notes []Note
	for _, note := range res.Report.Notes {
		if note.Reason == want {
			notes = append(notes, note)
		}
	}
	if len(notes) != 1 || notes[0].Subject != "variable ds" || notes[0].Status != statusPartial {
		t.Fatalf("notes = %+v, want one note %q", res.Report.Notes, want)
	}
	for _, line := range res.Report.Panels {
		if line.Status != statusImported || len(line.Paths) != 0 || len(line.Reasons) != 0 {
			t.Fatalf("panel %s = %+v, want imported with nothing reported", line.PanelID, line)
		}
	}
	if entry, ok := res.Spec.FindPanel("p1"); !ok || entry.Panel.Datasource != "prom" {
		t.Fatalf("p1 = %+v, want datasource prom", entry.Panel)
	}
}

const wantLegendTableReason = "Legend tables are not supported; the legend is a plain list."

func TestLegendTablesAreReportedBecauseTheSpecHasNoTableLegend(t *testing.T) {
	cases := []struct {
		name  string
		panel func(t *testing.T) (spec.Panel, PanelLine)
		paths []string
	}{
		{"table display mode", func(t *testing.T) (spec.Panel, PanelLine) {
			return convertedPanel(t, panelDoc("timeseries", upTarget+`,"options":{"legend":{"displayMode":"table","placement":"bottom","showLegend":true,"calcs":["mean"]}}`))
		}, []string{"options.legend.displayMode"}},
		{"as table flag", func(t *testing.T) (spec.Panel, PanelLine) {
			return convertedPanel(t, panelDoc("timeseries", upTarget+`,"options":{"legend":{"displayMode":"list","asTable":true,"showLegend":true}}`))
		}, []string{"options.legend.asTable"}},
		{"legacy aligned as table", func(t *testing.T) (spec.Panel, PanelLine) {
			return graphPanel(t, `"legend":{"alignAsTable":true,"show":true}`)
		}, []string{"legend.alignAsTable"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := tc.panel(t)
			if line.Status != statusPartial || !slices.Equal(line.Paths, tc.paths) || !slices.Contains(line.Reasons, wantLegendTableReason) {
				t.Fatalf("status %s, paths %v, reasons %v, want %v and %q", line.Status, line.Paths, line.Reasons, tc.paths, wantLegendTableReason)
			}
		})
	}
}

func TestPlainLegendOptionsReportNothing(t *testing.T) {
	cases := map[string]func(t *testing.T) PanelLine{
		"list": func(t *testing.T) PanelLine {
			_, line := convertedPanel(t, panelDoc("timeseries", upTarget+`,"options":{"legend":{"displayMode":"list","asTable":false,"placement":"right","showLegend":true,"calcs":["max"]}}`))
			return line
		},
		"legacy not a table": func(t *testing.T) PanelLine {
			_, line := graphPanel(t, `"legend":{"alignAsTable":false,"show":true}`)
			return line
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			if line := run(t); line.Status != statusImported || len(line.Paths) != 0 {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}
