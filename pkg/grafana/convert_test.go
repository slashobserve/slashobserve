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
	const withUnknownProperties = `{"matcher":{"id":"byName","options":"s"},"properties":[{"id":"unit","value":"bytes"},{"id":"noSuchA","value":1},{"id":"noSuchB","value":1}]}`
	const pastTheOverrideCap = `{"matcher":{"id":"byName","options":"s"},"properties":[{"id":"unit","value":"bytes"},{"id":"min","value":10},{"id":"max","value":5}]}`
	overrides := `"fieldConfig":{"defaults":{},"overrides":[` + strings.Repeat(withUnknownProperties+",", 50) + pastTheOverrideCap + `]}`
	doc := panelDoc("timeseries", upTarget+","+overrides+","+options.String())

	start := time.Now()
	_, line := convertedPanel(t, doc)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("converting %d unknown options took %s", leaves, elapsed)
	}
	if line.Status != statusPartial || len(line.Paths) != 100 {
		t.Fatalf("status %s with %d paths, want partial with the cap of 100", line.Status, len(line.Paths))
	}
	if want := fmt.Sprintf("%d more options were not imported.", leaves+1); !slices.Contains(line.Reasons, want) {
		t.Fatalf("reasons = %v, want %q: the override past the cap is one path reported twice", line.Reasons, want)
	}
	if len(line.Reasons) != 2 {
		t.Fatalf("reasons = %v, want the shared reason and the overflow count only", line.Reasons)
	}
}

func TestHugeNumbersOfTargetsKeepTheReportBounded(t *testing.T) {
	empty := func(n int) string { return strings.Repeat("{},", n) }
	hidden := func(n int) string { return strings.Repeat(`{"hide":true},`, n) }
	const million = 1_000_000
	const hundredThousand = 100_000
	cases := []struct {
		name, typ, list string
		targets         int
		wantUp          bool
		wantReason      string
	}{
		{"a kept query after the listed targets", "timeseries", "[" + empty(200) + `{"refId":"Z","expr":"up"},` + empty(million-202) + "{}]", million, true, fmt.Sprintf("%d more queries were not imported.", million-101)},
		{"no query can be imported", "timeseries", "[" + empty(hundredThousand-1) + "{}]", hundredThousand, false, fmt.Sprintf("%d more queries were not imported.", hundredThousand-100)},
		{"placeholder panel", "barchart", "[" + empty(hundredThousand-1) + "{}]", hundredThousand, false, fmt.Sprintf("%d more queries were not imported.", hundredThousand-100)},
		{"one visible target past the cap", "timeseries", "[" + empty(100) + "{}]", 101, false, "1 more query was not imported."},
		{"one visible target past the cap of a placeholder panel", "barchart", "[" + empty(100) + "{}]", 101, false, "1 more query was not imported."},
		{"hidden targets past the cap are not counted", "timeseries", `[{"refId":"Z","expr":"up","noSuchKey":1},` + empty(99) + hidden(4) + `{"hide":true}]`, 105, true, ""},
		{"hidden targets past the cap of a placeholder panel are not counted", "barchart", "[" + empty(100) + hidden(4) + `{"hide":true}]`, 105, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			res := convertDoc(t, panelDoc(tc.typ, `"targets":`+tc.list))
			if elapsed := time.Since(start); elapsed > 20*time.Second {
				t.Fatalf("converting %d targets took %s", tc.targets, elapsed)
			}
			line := panelLine(t, res, "p1")
			if len(line.Queries) > 100 {
				t.Fatalf("the panel lists %d queries, want at most 100", len(line.Queries))
			}
			if len(line.Paths) > 100 {
				t.Fatalf("the panel lists %d paths, want at most 100", len(line.Paths))
			}
			report, err := json.Marshal(res.Report)
			if err != nil || len(report) > 100_000 {
				t.Fatalf("the report takes %d bytes (err %v), want at most 100000", len(report), err)
			}
			if tc.wantUp {
				entry, _ := res.Spec.FindPanel("p1")
				queries := entry.Panel.Queries
				if len(queries) != 1 || queries[0].Query != "up" || queries[0].Ref != "Z" {
					t.Fatalf("queries = %+v, want the one valid target kept with its ref", queries)
				}
			}
			if tc.wantReason == "" {
				for _, reason := range line.Reasons {
					if strings.Contains(reason, "more ") {
						t.Fatalf("reasons = %v, want no count of hidden targets", line.Reasons)
					}
				}
				if slices.Contains(line.Paths, "targets") {
					t.Fatalf("paths = %v, want no path for hidden targets", line.Paths)
				}
				return
			}
			if !slices.Contains(line.Paths, "targets") {
				t.Fatalf("paths = %v, want the rest reported once on targets", line.Paths)
			}
			if !slices.Contains(line.Reasons, tc.wantReason) {
				t.Fatalf("reasons = %v, want %q", line.Reasons, tc.wantReason)
			}
		})
	}
}

func TestDroppedTargetTakesItsPartialPathsAndReasonsWithIt(t *testing.T) {
	const cutReason = "The legend was cut to 200 characters."
	longLegend := strings.Repeat("l", 250)
	panel, line := convertedPanel(t, panelDoc("timeseries", `"targets":[
		{"refId":"A","expr":"up{pod=\"$undeclared\"}","legendFormat":"`+longLegend+`"},
		{"refId":"B","expr":"up","legendFormat":"`+longLegend+`"}]`))
	if len(panel.Queries) != 1 {
		t.Fatalf("queries = %+v, want only the second target", panel.Queries)
	}
	if !slices.Equal(line.Paths, []string{"targets[0]", "targets[1].legendFormat"}) {
		t.Fatalf("paths = %v, want the dropped target and the kept target's cut legend", line.Paths)
	}
	cuts := 0
	for _, reason := range line.Reasons {
		if reason == cutReason {
			cuts++
		}
	}
	if cuts != 1 {
		t.Fatalf("reasons = %v, want the kept target's %q once", line.Reasons, cutReason)
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
	defaults := []string{"ok", "critical@80"}
	cases := []struct {
		name, members string
		want          []string
	}{
		{"two levels", colors + `,"thresholds":"50,80"`, []string{"ok", "warning@50", "critical@80"}},
		{"one level", colors + `,"thresholds":"50"`, []string{"ok", "warning@50"}},
		{"empty levels without a color mode write none", colors + `,"thresholds":""`, []string{}},
		{"blank levels without a color mode write none", colors + `,"thresholds":"  "`, []string{}},
		{"missing levels without a color mode write none", colors, []string{}},
		{"empty levels with colored values get Grafana's defaults", colors + `,"thresholds":"","colorValue":true`, defaults},
		{"blank levels with a colored background get Grafana's defaults", colors + `,"thresholds":"  ","colorBackground":true`, defaults},
		{"missing levels with colored values get Grafana's defaults", colors + `,"colorValue":true`, defaults},
		{"levels win over the defaults", colors + `,"thresholds":"50,80","colorValue":true`, []string{"ok", "warning@50", "critical@80"}},
		{"colored values without colors write none", `"thresholds":"","colorValue":true`, []string{}},
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
		{"explicit reduceRow without reducer", `{"id":"calculateField","options":{"mode":"reduceRow"}}`, spec.Transform{Type: "calculateField", ReduceRow: "sum", Alias: "Total"}},
		{"explicit reduceRow mean", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"mean"}}}`, spec.Transform{Type: "calculateField", ReduceRow: "mean", Alias: "Mean"}},
		{"explicit reduceRow max", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"max"}}}`, spec.Transform{Type: "calculateField", ReduceRow: "max", Alias: "Max"}},
		{"explicit reduceRow min", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"min"}}}`, spec.Transform{Type: "calculateField", ReduceRow: "min", Alias: "Min"}},
		{"explicit reduceRow lastNotNull", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"lastNotNull"}}}`, spec.Transform{Type: "calculateField", ReduceRow: "lastNotNull", Alias: "Last *"}},
		{"explicit reduceRow last", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"last"}}}`, spec.Transform{Type: "calculateField", ReduceRow: "last", Alias: "Last"}},
		{"explicit reduceRow firstNotNull", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"firstNotNull"}}}`, spec.Transform{Type: "calculateField", ReduceRow: "firstNotNull", Alias: "First *"}},
		{"explicit reduceRow first", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"first"}}}`, spec.Transform{Type: "calculateField", ReduceRow: "first", Alias: "First"}},
		{"explicit reduceRow count", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"count"}}}`, spec.Transform{Type: "calculateField", ReduceRow: "count", Alias: "Count"}},
		{"explicit reduceRow diff", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"diff"}}}`, spec.Transform{Type: "calculateField", ReduceRow: "diff", Alias: "Difference"}},
		{"explicit reduceRow stdDev", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"stdDev"}}}`, spec.Transform{Type: "calculateField", ReduceRow: "stdDev", Alias: "StdDev"}},
		{"explicit reduceRow keeps the alias", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"mean"},"alias":"Average"}}`, spec.Transform{Type: "calculateField", ReduceRow: "mean", Alias: "Average"}},
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

func TestOptionsTheRenderersAlreadyDrawTheSameWayAreNotReported(t *testing.T) {
	series := func(custom, options string) string {
		return panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{"custom":{`+custom+`}},"overrides":[]},"options":{`+options+`}`)
	}
	override := func(typ, property string) string {
		return panelDoc(typ, upTarget+`,"fieldConfig":{"defaults":{},"overrides":[{"matcher":{"id":"byName","options":"a"},"properties":[{"id":"unit","value":"bytes"},`+property+`]}]}`)
	}
	cases := []struct {
		name, doc string
		silent    bool
	}{
		{"points never", series(`"showPoints":"never"`, ""), true},
		{"points always", series(`"showPoints":"always"`, ""), false},
		{"line width 2", series(`"lineWidth":2`, ""), true},
		{"line width 3", series(`"lineWidth":3`, ""), false},
		{"tooltip sorted descending", series("", `"tooltip":{"mode":"single","sort":"desc"}`), true},
		{"tooltip sorted ascending", series("", `"tooltip":{"mode":"single","sort":"asc"}`), false},
		{"legend width", series("", `"legend":{"displayMode":"list","placement":"right","width":300}`), true},
		{"override line width 2", override("timeseries", `{"id":"custom.lineWidth","value":2}`), true},
		{"override line width 4", override("timeseries", `{"id":"custom.lineWidth","value":4}`), false},
		{"stat justified automatically", panelDoc("stat", upTarget+`,"options":{"justifyMode":"auto"}`), true},
		{"stat centered", panelDoc("stat", upTarget+`,"options":{"justifyMode":"center"}`), false},
		{"table column width", panelDoc("table", upTarget+`,"fieldConfig":{"defaults":{"custom":{"width":120}},"overrides":[]}`), true},
		{"table override column width", override("table", `{"id":"custom.width","value":90}`), true},
		{"legend click marker", panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{},"overrides":[{"__systemRef":"hideSeriesFrom","matcher":{"id":"byName","options":"a"},"properties":[{"id":"custom.hideFrom","value":{"viz":true}}]}]}`), true},
		{"legacy line width 2", panelDoc("graph", upTarget+`,"linewidth":2`), true},
		{"legacy line width 3", panelDoc("graph", upTarget+`,"linewidth":3`), false},
		{"table column min width", panelDoc("table", upTarget+`,"fieldConfig":{"defaults":{"custom":{"minWidth":50}},"overrides":[]}`), true},
		{"table override column min width", override("table", `{"id":"custom.minWidth","value":200}`), true},
		{"legacy legend width", panelDoc("graph", upTarget+`,"legend":{"show":true,"rightSide":true,"sideWidth":350}`), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := convertedPanel(t, tc.doc)
			if quiet := line.Status == statusImported && len(line.Paths) == 0; quiet != tc.silent {
				t.Fatalf("status %s, paths %v, want silent = %v", line.Status, line.Paths, tc.silent)
			}
		})
	}
}

func TestOptionsThatChangeMeaningAreAlwaysReported(t *testing.T) {
	cases := []struct{ name, doc string }{
		{"span nulls", panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{"custom":{"spanNulls":true}},"overrides":[]}`)},
		{"cumulative tooltip values", panelDoc("graph", upTarget+`,"tooltip":{"shared":true,"value_type":"cumulative"}`)},
		{"negative-Y transform", panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{},"overrides":[{"matcher":{"id":"byName","options":"a"},"properties":[{"id":"custom.transform","value":"negative-Y"}]}]}`)},
		{"query interval", panelDoc("timeseries", `"targets":[{"refId":"A","expr":"up","interval":"$interval"}]`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := convertedPanel(t, tc.doc)
			if line.Status != statusPartial || len(line.Paths) == 0 {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}
