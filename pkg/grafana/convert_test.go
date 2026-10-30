package grafana

import (
	"encoding/json"
	"fmt"
	"math"
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

func convertUnknownOptions(t *testing.T, leaves int) (PanelLine, time.Duration) {
	t.Helper()
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
	elapsed := time.Since(start)
	if line.Status != statusPartial || len(line.Paths) != 100 {
		t.Fatalf("status %s with %d paths, want partial with the cap of 100", line.Status, len(line.Paths))
	}
	if want := fmt.Sprintf("%d more options were not imported.", leaves+1); !slices.Contains(line.Reasons, want) {
		t.Fatalf("reasons = %v, want %q: the override past the cap is one path reported twice", line.Reasons, want)
	}
	if len(line.Reasons) != 2 {
		t.Fatalf("reasons = %v, want the shared reason and the overflow count only", line.Reasons)
	}
	return line, elapsed
}

func TestHugeNumbersOfUnknownOptionsAreCappedWithoutSlowdown(t *testing.T) {
	const small, large, attempts = 10_000, 100_000, 3
	fastest := func(leaves int) time.Duration {
		best := time.Duration(math.MaxInt64)
		for range attempts {
			_, elapsed := convertUnknownOptions(t, leaves)
			best = min(best, elapsed)
		}
		return best
	}
	smallTime, largeTime := fastest(small), fastest(large)
	growth := float64(largeTime) / float64(smallTime)
	linearGrowth := float64(large) / float64(small)
	if growth > 3*linearGrowth {
		t.Fatalf("%d unknown options took %s and %d took %s: %.0fx for %.0fx the options, want linear growth",
			small, smallTime, large, largeTime, growth, linearGrowth)
	}
}

func TestIndexNumbersInPathsAreNormalized(t *testing.T) {
	cases := map[string]string{
		"":                          "",
		"options.text":              "options.text",
		"options.tags[0]":           "options.tags[]",
		"a[12].b[345].c":            "a[].b[].c",
		"a[x].b[]":                  "a[x].b[]",
		"a[[1]":                     "a[[]",
		"a[1":                       "a[1",
		"targets[0].meta[1][2]":     "targets[].meta[][]",
		"fieldConfig.overrides[10]": "fieldConfig.overrides[]",
	}
	for path, want := range cases {
		if got := normalizePath(path); got != want {
			t.Errorf("normalizePath(%q) = %q, want %q", path, got, want)
		}
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

func TestDroppedTargetTakesItsOmittedPathsWithIt(t *testing.T) {
	manyIssues := `"legendFormat":"` + strings.Repeat("l", 250) + `","interval":"7x","exemplar":true,"intervalFactor":2,"step":"7x"`
	targets := `{"refId":"B","expr":"up"},` + strings.Repeat("{},", 98) + `{"refId":"Z","expr":"up{pod=\"$undeclared\"}",` + manyIssues + `}`
	_, line := convertedPanel(t, panelDoc("timeseries", `"targets":[`+targets+`]`))
	if len(line.Paths) != 99 {
		t.Fatalf("paths = %d, want the 98 empty targets and the dropped one", len(line.Paths))
	}
	for _, reason := range line.Reasons {
		if strings.Contains(reason, "more options") {
			t.Fatalf("reasons = %v, want no count: the dropped target's omitted paths left with it", line.Reasons)
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
		{"colored values without colors get Grafana's defaults", `"thresholds":"","colorValue":true`, defaults},
		{"a colored background without colors gets Grafana's defaults", `"colorBackground":true`, defaults},
		{"levels with colored values but no colors get Grafana's defaults", `"thresholds":"50,80","colorValue":true`, defaults},
		{"a gauge with levels but no colors gets Grafana's defaults", `"gauge":{"show":true},"thresholds":"50,80"`, defaults},
		{"an uncolored stat without colors writes none", `"thresholds":""`, []string{}},
		{"a gauge with empty levels gets Grafana's defaults whatever the color flags", `"gauge":{"show":true},"thresholds":"","colorValue":false,"colorBackground":false`, defaults},
		{"a gauge without colors or levels gets Grafana's defaults", `"gauge":{"show":true}`, defaults},
		{"a gauge with levels and colors keeps them", `"gauge":{"show":true},` + colors + `,"thresholds":"50,80"`, []string{"ok", "warning@50", "critical@80"}},
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

func TestUnknownReducerLeavesTheStatPartial(t *testing.T) {
	_, line := convertedPanel(t, panelDoc("stat", upTarget+`,"options":{"reduceOptions":{"calcs":["bogus"]}}`))
	const reason = "The reducer bogus is not supported yet."
	if line.Status != statusPartial || !slices.Equal(line.Paths, []string{"options.reduceOptions.calcs"}) || !slices.Equal(line.Reasons, []string{reason}) {
		t.Fatalf("status %s, paths %v, reasons %v, want partial with %q", line.Status, line.Paths, line.Reasons, reason)
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

func TestDatasourceVariableReferencesAreKept(t *testing.T) {
	panels := make([]string, 3)
	for i := range panels {
		panels[i] = fmt.Sprintf(`{"id":%d,"type":"stat","title":"P%d","gridPos":{"x":%d,"y":0,"w":8,"h":4},"datasource":{"type":"prometheus","uid":"${ds}"},"targets":[{"refId":"A","expr":"up"}]}`, i+1, i+1, i*8)
	}
	doc := `{"title":"T","schemaVersion":39,"templating":{"list":[{"name":"ds","type":"datasource","query":"prometheus","current":{"text":"prom","value":"prom"}}]},"panels":[` + strings.Join(panels, ",") + `]}`
	res := convertDoc(t, doc)

	for _, note := range res.Report.Notes {
		if strings.Contains(note.Reason, "$ds") {
			t.Fatalf("note %+v mentions the datasource variable, want none", note)
		}
	}
	for _, line := range res.Report.Panels {
		if line.Status != statusImported || len(line.Paths) != 0 || len(line.Reasons) != 0 {
			t.Fatalf("panel %s = %+v, want imported with nothing reported", line.PanelID, line)
		}
		if len(line.Queries) != 1 || line.Queries[0].Datasource == nil || *line.Queries[0].Datasource != "${ds}" {
			t.Fatalf("panel %s query lines = %+v, want datasource ${ds}", line.PanelID, line.Queries)
		}
	}
	for _, id := range []string{"p1", "p2", "p3"} {
		if entry, ok := res.Spec.FindPanel(id); !ok || entry.Panel.Datasource != "${ds}" {
			t.Fatalf("%s = %+v, want datasource ${ds}", id, entry.Panel)
		}
	}
	ds := variableNamed(t, res, "ds")
	if ds.Type != spec.DatasourceVariable || ds.Default == nil || !slices.Equal(ds.Default.Values, []string{"prom"}) {
		t.Fatalf("ds = %+v, want a datasource variable defaulting to prom", ds)
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

func displayJSON(t *testing.T, panel spec.Panel) string {
	t.Helper()
	if panel.Display == nil {
		return "null"
	}
	data, err := json.Marshal(panel.Display)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestChartPanelTypesImportTheirOptions(t *testing.T) {
	cases := []struct {
		name, typ, members string
		display            string
		stack              string
		paths              []string
	}{
		{
			name: "bar chart options", typ: "barchart",
			members: `"options":{"orientation":"horizontal","xField":"pod","showValue":"always","barWidth":0.5,"groupWidth":0.8,"stacking":"percent","legend":{"displayMode":"list","placement":"right","showLegend":true},"tooltip":{"mode":"multi","sort":"none"}}`,
			display: `{"legend":"right","tooltip":"all","orientation":"horizontal","showValues":"always","xField":"pod","barWidth":0.5,"groupWidth":0.8}`,
			stack:   "percent",
		},
		{
			name: "bar chart defaults", typ: "barchart",
			members: `"options":{"orientation":"auto","showValue":"auto","barWidth":0.97,"groupWidth":0.7,"stacking":"none","xTickLabelRotation":0,"xTickLabelSpacing":0,"barRadius":0,"fullHighlight":false,"legend":{"displayMode":"list","placement":"bottom","showLegend":true,"calcs":[]},"tooltip":{"mode":"single","sort":"none"}}`,
			display: "null",
		},
		{
			name: "bar chart options with no Spec field", typ: "barchart",
			members: `"options":{"colorByField":"pod","xTickLabelRotation":45,"xTickLabelSpacing":10,"barRadius":0.2,"fullHighlight":true}`,
			display: "null",
			paths:   []string{"options.barRadius", "options.colorByField", "options.fullHighlight", "options.xTickLabelRotation", "options.xTickLabelSpacing"},
		},
		{
			name: "bar chart width outside the range", typ: "barchart",
			members: `"options":{"barWidth":0,"groupWidth":1}`,
			display: `{"groupWidth":1}`,
			paths:   []string{"options.barWidth"},
		},
		{
			name: "status history", typ: "status-history",
			members: `"options":{"showValue":"never","rowHeight":0.9,"colWidth":0.9}`,
			display: `{"showValues":"never"}`,
		},
		{
			name: "status history sizes", typ: "status-history",
			members: `"options":{"rowHeight":0.5,"colWidth":0.7}`,
			display: "null",
			paths:   []string{"options.rowHeight", "options.colWidth"},
		},
		{
			name: "trend", typ: "trend",
			members: `"options":{"xField":"size","legend":{"displayMode":"list","placement":"right","showLegend":true,"calcs":["max","mean"]},"tooltip":{"mode":"none","sort":"none"}}`,
			display: `{"legend":"right","legendValues":["max","mean"],"tooltip":"hidden","xField":"size"}`,
		},
		{
			name: "xy chart version 1", typ: "xychart",
			members: `"options":{"dims":{"x":"severity"}},"fieldConfig":{"defaults":{"custom":{"show":"lines","pointSize":{"fixed":8}}},"overrides":[]}`,
			display: `{"xField":"severity","show":"lines","pointSize":8}`,
		},
		{
			name: "xy chart manual mapping", typ: "xychart",
			members: `"options":{"mapping":"manual","series":[{"x":{"matcher":{"id":"byName","options":"severity"}},"y":{"matcher":{"id":"byNames","options":["len"]}},"size":{"matcher":{"id":"byName","options":"n"}}}]}`,
			display: `{"xField":"severity"}`,
			paths:   []string{"options.series[0].y", "options.series[0].size"},
		},
		{
			name: "xy chart with two series", typ: "xychart",
			members: `"options":{"mapping":"manual","series":[{"x":{"matcher":{"id":"byName","options":"a"}}},{"x":{"matcher":{"id":"byName","options":"b"}}}]}`,
			display: "null",
			paths:   []string{"options.mapping", "options.series"},
		},
		{
			name: "xy chart point size outside the range", typ: "xychart",
			members: `"options":{"mapping":"auto"},"fieldConfig":{"defaults":{"custom":{"pointSize":{"fixed":60},"show":"points","pointShape":"circle","pointStrokeWidth":1,"fillOpacity":50}},"overrides":[]}`,
			display: "null",
			paths:   []string{"fieldConfig.defaults.custom.pointSize.fixed"},
		},
		{
			name: "candlestick", typ: "candlestick",
			members: `"options":{"mode":"candles","candleStyle":"ohlcbars","colorStrategy":"close-close","colors":{"up":"blue","down":"orange","flat":"gray"},"fields":{"open":"o","high":"h","low":"l","close":"c","volume":"v"},"includeAllFields":false}`,
			display: `{"mode":"candles","candleStyle":"ohlcbars","colorStrategy":"close-close","upColor":"info","downColor":"warning","open":"o","high":"h","low":"l","close":"c","volume":"v"}`,
		},
		{
			name: "candlestick options with no Spec field", typ: "candlestick",
			members: `"options":{"colors":{"up":"nonsense","down":"orange","flat":"purple"},"includeAllFields":true}`,
			display: `{"downColor":"warning"}`,
			paths:   []string{"options.colors.up", "options.colors.flat", "options.includeAllFields"},
		},
		{
			name: "node graph grid layout", typ: "nodeGraph",
			members: `"options":{"layoutAlgorithm":"grid","zoomMode":"greedy"}`,
			display: `{"layout":"grid"}`,
		},
		{
			name: "node graph force layout", typ: "nodeGraph",
			members: `"options":{"layoutAlgorithm":"force"}`,
			display: "null",
			paths:   []string{"options.layoutAlgorithm"},
		},
		{
			name: "traces", typ: "traces",
			members: `"options":{"limit":10}`,
			display: "null",
			paths:   []string{"options.limit"},
		},
		{
			name: "flame graph", typ: "flamegraph",
			members: `"options":{}`,
			display: "null",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc(tc.typ, upTarget+","+tc.members))
			if string(panel.Type) != tc.typ || line.Type != tc.typ {
				t.Fatalf("type = %s, line type = %s, want %s", panel.Type, line.Type, tc.typ)
			}
			if got := displayJSON(t, panel); got != tc.display {
				t.Errorf("display = %s, want %s", got, tc.display)
			}
			if panel.Stack != tc.stack {
				t.Errorf("stack = %q, want %q", panel.Stack, tc.stack)
			}
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v (reasons %v)", line.Paths, tc.paths, line.Reasons)
			}
		})
	}
}

func TestStatusHistorySizeIsReportedWithItsOwnReason(t *testing.T) {
	_, line := convertedPanel(t, panelDoc("status-history", upTarget+`,"options":{"rowHeight":0.5}`))
	if !slices.Contains(line.Reasons, rowHeightReason) {
		t.Fatalf("reasons = %v, want %q", line.Reasons, rowHeightReason)
	}
}

func TestNodeGraphUnitsAndArcsBecomeFieldOverrides(t *testing.T) {
	cases := []struct {
		name, options string
		want          []spec.Override
		paths         []string
	}{
		{
			name:    "equal node and edge unit",
			options: `"nodes":{"mainStatUnit":"ms"},"edges":{"mainStatUnit":"ms"}`,
			want:    []spec.Override{{Match: spec.Match{Field: "mainstat"}, FieldProps: spec.FieldProps{Unit: "ms"}}},
		},
		{
			name:    "one side only",
			options: `"nodes":{"secondaryStatUnit":"percentunit"}`,
			want:    []spec.Override{{Match: spec.Match{Field: "secondarystat"}, FieldProps: spec.FieldProps{Unit: "percentunit"}}},
		},
		{
			name:    "edge side only",
			options: `"edges":{"secondaryStatUnit":"bytes"}`,
			want:    []spec.Override{{Match: spec.Match{Field: "secondarystat"}, FieldProps: spec.FieldProps{Unit: "bytes"}}},
		},
		{
			name:    "different node and edge units",
			options: `"nodes":{"mainStatUnit":"ms"},"edges":{"mainStatUnit":"s"}`,
			paths:   []string{"options.nodes.mainStatUnit", "options.edges.mainStatUnit"},
		},
		{
			name:    "unit the Spec lacks",
			options: `"nodes":{"mainStatUnit":"nonsense"}`,
			paths:   []string{"options.nodes.mainStatUnit"},
		},
		{
			name:    "arcs",
			options: `"nodes":{"arcs":[{"field":"arc__ok","color":"green"},{"field":"arc__bad","color":"#ff0000"},{"field":"","color":"red"},{"field":"arc__nocolor"}]}`,
			want: []spec.Override{
				{Match: spec.Match{Field: "arc__ok"}, FieldProps: spec.FieldProps{Color: "#73BF69"}},
				{Match: spec.Match{Field: "arc__bad"}, FieldProps: spec.FieldProps{Color: "#ff0000"}},
			},
			paths: []string{"options.nodes.arcs[2]", "options.nodes.arcs[3]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc("nodeGraph", upTarget+`,"options":{`+tc.options+`}`))
			if !reflect.DeepEqual(panel.Overrides, tc.want) {
				t.Errorf("overrides = %+v, want %+v", panel.Overrides, tc.want)
			}
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v", line.Paths, tc.paths)
			}
		})
	}
}

func TestNodeGraphFieldOverridesMatchFieldsNotSeries(t *testing.T) {
	panel, _ := convertedPanel(t, panelDoc("nodeGraph", upTarget+`,"fieldConfig":{"defaults":{},"overrides":[{"matcher":{"id":"byName","options":"arc__errors"},"properties":[{"id":"color","value":{"mode":"fixed","fixedColor":"red"}}]}]}`))
	want := []spec.Override{{Match: spec.Match{Field: "arc__errors"}, FieldProps: spec.FieldProps{Color: "#F2495C"}}}
	if !reflect.DeepEqual(panel.Overrides, want) {
		t.Fatalf("overrides = %+v, want %+v", panel.Overrides, want)
	}
}

func TestTrendAndCandlestickKeepThresholdsStyle(t *testing.T) {
	for _, typ := range []string{"trend", "candlestick", "timeseries"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc(typ, upTarget+`,"fieldConfig":{"defaults":{"custom":{"thresholdsStyle":{"mode":"line"}},"thresholds":{"mode":"absolute","steps":[{"color":"green","value":null},{"color":"red","value":80}]}},"overrides":[]}`))
			if got := thresholdText(panel.Thresholds); !slices.Equal(got, []string{"ok", "critical@80"}) {
				t.Fatalf("thresholds = %v", got)
			}
			if len(line.Paths) != 0 {
				t.Fatalf("paths = %v", line.Paths)
			}
		})
	}
}

func TestTraceQueriesBecomeTableQueries(t *testing.T) {
	const frame = `"id":1,"title":"T","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"grafana-clickhouse-datasource","uid":"ch"}`
	cases := []struct{ name, typ, target string }{
		{"query type traces", "traces", `{"refId":"A","rawSql":"SELECT 1","queryType":"traces"}`},
		{"format 3", "traces", `{"refId":"A","rawSql":"SELECT 1","format":3}`},
		{"node graph", "nodeGraph", `{"refId":"A","rawSql":"SELECT 1","queryType":"traces"}`},
		{"altinity traces", "traces", `{"refId":"A","query":"SELECT 1","format":"traces","datasource":{"type":"vertamedia-clickhouse-datasource","uid":"ch"}}`},
		{"altinity flame graph", "flamegraph", `{"refId":"A","query":"SELECT 1","format":"flamegraph","datasource":{"type":"vertamedia-clickhouse-datasource","uid":"ch"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, fmt.Sprintf(`{"title":"T","schemaVersion":39,"panels":[{%s,"type":%q,"targets":[%s]}]}`, frame, tc.typ, tc.target))
			if len(panel.Queries) != 1 || panel.Queries[0].Format != "table" {
				t.Fatalf("queries = %+v, want one table query", panel.Queries)
			}
			if line.Status == statusPlaceholder || line.Queries[0].Dropped != nil {
				t.Fatalf("line = %+v, want the trace query kept", line)
			}
		})
	}
}

func TestAutoLegendIsKeptVerbatim(t *testing.T) {
	panel, _ := convertedPanel(t, panelDoc("timeseries", `"targets":[{"refId":"A","expr":"up","legendFormat":"__auto"}]`))
	if len(panel.Queries) != 1 || panel.Queries[0].Legend != "__auto" {
		t.Fatalf("queries = %+v, want legend __auto", panel.Queries)
	}
}

func TestSourcePanelTypesMapToTheirModernType(t *testing.T) {
	cases := []struct{ source, want string }{
		{"grafana-worldmap-panel", "geomap"},
		{"natel-discrete-panel", "state-timeline"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			typ, sourceType := panelTypeOf(map[string]any{"type": tc.source})
			if typ != tc.want || sourceType != tc.source {
				t.Fatalf("panelTypeOf = %s, %s, want %s, %s", typ, sourceType, tc.want, tc.source)
			}
		})
	}
}

func geomapLayer(location, style string) string {
	return fmt.Sprintf(`{"type":"markers","name":"Markers","location":%s,"tooltip":true,"config":{"showLegend":true,"style":%s}}`, location, style)
}

func geomapOptions(view, layers string) string {
	return fmt.Sprintf(`"options":{"view":%s,"layers":[%s],"controls":{"mouseWheelZoom":true,"showZoom":true,"showAttribution":true},"tooltip":{"mode":"details"}}`, view, layers)
}

func TestGeomapImportsItsFirstMarkersLayer(t *testing.T) {
	const plainStyle = `{"color":{"fixed":"dark-green"},"opacity":0.4,"size":{"fixed":5,"min":2,"max":15}}`
	cases := []struct {
		name, options string
		display       string
		color         string
		paths         []string
	}{
		{
			name:    "coordinates with a sized and colored field",
			options: geomapOptions(`{"id":"coords","lat":52,"lon":5,"zoom":4}`, geomapLayer(`{"mode":"coords","latitude":"lat","longitude":"lon"}`, `{"color":{"field":"Value","fixed":"red"},"size":{"field":"Value","min":3,"max":20}}`)),
			display: `{"view":"coords","lat":52,"lon":5,"zoom":4,"location":"coords","latField":"lat","lonField":"lon","sizeField":"Value","colorField":"Value","markerSize":3,"markerSizeMax":20}`,
		},
		{
			name:    "geohash with a fixed size and color",
			options: geomapOptions(`{"id":"fit"}`, geomapLayer(`{"mode":"geohash","geohash":"hash"}`, `{"color":{"fixed":"blue"},"size":{"fixed":9,"min":2,"max":15}}`)),
			display: `{"location":"geohash","geohashField":"hash","markerSize":9}`,
			color:   "#5794F2",
		},
		{
			name:    "world view and automatic location",
			options: geomapOptions(`{"id":"zero","lat":0,"lon":0,"zoom":1}`, geomapLayer(`{"mode":"auto"}`, plainStyle)),
			display: `{"view":"world"}`,
			color:   "#37872D",
		},
		{
			name:    "a lookup layer is reported",
			options: geomapOptions(`{"id":"fit"}`, geomapLayer(`{"mode":"lookup","lookup":"country","gazetteer":"public/gazetteer/countries.json"}`, plainStyle)),
			display: "null",
			color:   "#37872D",
			paths:   []string{"options.layers[0].location"},
		},
		{
			name:    "a named region is reported",
			options: geomapOptions(`{"id":"europe","lat":46,"lon":14,"zoom":4}`, geomapLayer(`{"mode":"auto"}`, plainStyle)),
			display: "null",
			color:   "#37872D",
			paths:   []string{"options.view.id"},
		},
		{
			name:    "coordinates outside the range are reported",
			options: geomapOptions(`{"id":"coords","lat":120,"lon":5,"zoom":30}`, geomapLayer(`{"mode":"auto"}`, plainStyle)),
			display: `{"view":"coords","lon":5}`,
			color:   "#37872D",
			paths:   []string{"options.view.lat", "options.view.zoom"},
		},
		{
			name:    "a second markers layer and a heatmap layer are reported",
			options: geomapOptions(`{"id":"fit"}`, geomapLayer(`{"mode":"auto"}`, plainStyle)+","+geomapLayer(`{"mode":"auto"}`, plainStyle)+`,{"type":"heatmap","config":{}}`),
			display: "null",
			color:   "#37872D",
			paths:   []string{"options.layers[1]", "options.layers[2]"},
		},
		{
			name:    "the base map and non-default controls are reported",
			options: `"options":{"basemap":{"type":"default"},"controls":{"mouseWheelZoom":false,"showScale":true},"layers":[],"view":{"id":"fit"}}`,
			display: "null",
			paths:   []string{"options.basemap", "options.controls.mouseWheelZoom", "options.controls.showScale"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc("geomap", upTarget+","+tc.options))
			if got := displayJSON(t, panel); got != tc.display {
				t.Errorf("display = %s, want %s", got, tc.display)
			}
			if panel.Color != tc.color {
				t.Errorf("color = %q, want %q", panel.Color, tc.color)
			}
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v (reasons %v)", line.Paths, tc.paths, line.Reasons)
			}
		})
	}
}

func TestGeomapThresholdColorModeIsNotReported(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("geomap", upTarget+`,"fieldConfig":{"defaults":{"color":{"mode":"thresholds"},"thresholds":{"mode":"absolute","steps":[{"color":"green","value":null},{"color":"red","value":80}]}},"overrides":[]}`))
	if got := thresholdText(panel.Thresholds); !slices.Equal(got, []string{"ok", "critical@80"}) {
		t.Fatalf("thresholds = %v", got)
	}
	if len(line.Paths) != 0 {
		t.Fatalf("paths = %v", line.Paths)
	}
}

func TestWorldmapImportsAsAGeomap(t *testing.T) {
	const centered = `"mapCenter":"custom","mapCenterLatitude":20,"mapCenterLongitude":10,"initialZoom":"3"`
	cases := []struct {
		name, members, display string
		thresholds             []string
		paths                  []string
	}{
		{
			name: "table with coordinates",
			members: centered + `,"locationData":"table","tableQueryOptions":{"queryType":"coordinates","latitudeField":"lat","longitudeField":"lon","metricField":"hits","geohashField":"geohash","labelField":""},` +
				`"circleMinSize":2,"circleMaxSize":30,"thresholds":"10,100","colors":["rgba(245, 54, 54, 0.9)","#fade2a","#32ac2d"],"valueName":"current","maxDataPoints":1`,
			display:    `{"view":"coords","lat":20,"lon":10,"zoom":3,"location":"coords","latField":"lat","lonField":"lon","sizeField":"hits","colorField":"hits","markerSize":2,"markerSizeMax":30}`,
			thresholds: []string{"#f53636", "warning@10", "#32ac2d@100"},
		},
		{
			name:    "table with geohash",
			members: `"mapCenter":"(0°, 0°)","locationData":"table","tableQueryOptions":{"queryType":"geohash","geohashField":"hash","metricField":"hits"}`,
			display: `{"view":"world","location":"geohash","geohashField":"hash","sizeField":"hits","colorField":"hits"}`,
		},
		{
			name:    "countries are looked up by name",
			members: `"mapCenter":"(0°, 0°)","locationData":"countries","tableQueryOptions":{"queryType":"geohash","geohashField":"geohash"}`,
			display: `{"view":"world"}`,
			paths:   []string{"locationData"},
		},
		{
			name: "a total value is reported for table coordinates",
			members: centered + `,"locationData":"table","tableQueryOptions":{"queryType":"coordinates","latitudeField":"lat","longitudeField":"lon","metricField":"hits","geohashField":"geohash","labelField":""},` +
				`"valueName":"total"`,
			display: `{"view":"coords","lat":20,"lon":10,"zoom":3,"location":"coords","latField":"lat","lonField":"lon","sizeField":"hits","colorField":"hits"}`,
			paths:   []string{"valueName"},
		},
		{
			name:    "a total value is reported for table geohash",
			members: `"mapCenter":"(0°, 0°)","locationData":"table","tableQueryOptions":{"queryType":"geohash","geohashField":"hash","metricField":"hits"},"valueName":"total"`,
			display: `{"view":"world","location":"geohash","geohashField":"hash","sizeField":"hits","colorField":"hits"}`,
			paths:   []string{"valueName"},
		},
		{
			name:    "a total value is reported for countries",
			members: `"mapCenter":"(0°, 0°)","locationData":"countries","tableQueryOptions":{"queryType":"geohash","geohashField":"geohash"},"valueName":"total"`,
			display: `{"view":"world"}`,
			paths:   []string{"locationData", "valueName"},
		},
		{
			name:    "other locations and centers are reported",
			members: `"mapCenter":"Europe","locationData":"json endpoint","valueName":"max"`,
			display: "null",
			paths:   []string{"locationData", "mapCenter", "valueName"},
		},
		{
			name:    "thresholds without matching colors are reported",
			members: `"thresholds":"10,100","colors":["red","green"]`,
			display: "null",
			paths:   []string{"colors"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc("grafana-worldmap-panel", upTarget+","+tc.members))
			if panel.Type != spec.Geomap || line.SourceType != "grafana-worldmap-panel" {
				t.Fatalf("type = %s, source type %s", panel.Type, line.SourceType)
			}
			if got := displayJSON(t, panel); got != tc.display {
				t.Errorf("display = %s, want %s", got, tc.display)
			}
			if got := thresholdText(panel.Thresholds); !slices.Equal(got, tc.thresholds) && len(tc.thresholds) > 0 {
				t.Errorf("thresholds = %v, want %v", got, tc.thresholds)
			}
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v (reasons %v)", line.Paths, tc.paths, line.Reasons)
			}
		})
	}
}

func canvasPanel(t *testing.T, w, h int, elements string) (spec.Panel, PanelLine) {
	t.Helper()
	doc := fmt.Sprintf(`{"title":"T","schemaVersion":39,"panels":[{"id":1,"title":"T","gridPos":{"x":0,"y":0,"w":%d,"h":%d},"datasource":{"type":"prometheus","uid":"prom"},"type":"canvas",%s,"options":{"root":{"type":"frame","name":"root","elements":[%s]}}}]}`, w, h, upTarget, elements)
	return convertedPanel(t, doc)
}

func canvasElements(t *testing.T, panel spec.Panel) []spec.CanvasElement {
	t.Helper()
	return displayOf(t, panel).Elements
}

func TestCanvasPlacementInPercentIsKept(t *testing.T) {
	panel, line := canvasPanel(t, 12, 10, `{"type":"rectangle","name":"Box","constraint":{"horizontal":"scale","vertical":"scale"},"placement":{"left":10,"top":20,"width":30,"height":40},"config":{"text":{"mode":"fixed","fixed":"Hi"}}}`)
	want := []spec.CanvasElement{{ID: "Box", Kind: "rectangle", Left: 10, Top: 20, Width: 30, Height: 40, Text: "Hi"}}
	if got := canvasElements(t, panel); !reflect.DeepEqual(got, want) {
		t.Fatalf("elements = %+v, want %+v", got, want)
	}
	if len(line.Paths) != 0 {
		t.Fatalf("paths = %v", line.Paths)
	}
}

func TestCanvasPixelPlacementBecomesPercentAndIsReported(t *testing.T) {
	cases := []struct {
		name, placement, constraint string
		want                        [4]float64
	}{
		{"left and top", `{"left":72,"top":34,"width":144,"height":68}`, `{"horizontal":"left","vertical":"top"}`, [4]float64{10, 10, 20, 20}},
		{"right and bottom", `{"right":72,"bottom":34,"width":144,"height":68}`, `{"horizontal":"right","vertical":"bottom"}`, [4]float64{70, 70, 20, 20}},
		{"left and right", `{"left":72,"right":72,"top":0,"height":340}`, `{"horizontal":"leftright","vertical":"top"}`, [4]float64{10, 0, 80, 100}},
		{"past the panel", `{"left":1000,"top":-50,"width":2000,"height":68}`, `{"horizontal":"left","vertical":"top"}`, [4]float64{100, 0, 100, 20}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := canvasPanel(t, 12, 10, `{"type":"rectangle","name":"Box","constraint":`+tc.constraint+`,"placement":`+tc.placement+`}`)
			els := canvasElements(t, panel)
			if len(els) != 1 {
				t.Fatalf("elements = %+v", els)
			}
			if got := [4]float64{els[0].Left, els[0].Top, els[0].Width, els[0].Height}; got != tc.want {
				t.Errorf("box = %v, want %v", got, tc.want)
			}
			if !slices.Contains(line.Paths, "options.root.elements[0].placement") || !slices.Contains(line.Reasons, canvasPixelReason) {
				t.Errorf("paths = %v, reasons = %v, want the pixel conversion reported", line.Paths, line.Reasons)
			}
		})
	}
}

func TestCanvasElementKindsTextsAndFields(t *testing.T) {
	const scaled = `"constraint":{"horizontal":"scale","vertical":"scale"},"placement":{"left":0,"top":0,"width":10,"height":10}`
	cases := []struct {
		name, element string
		want          spec.CanvasElement
	}{
		{"metric value", `{"type":"metric-value","name":"m",` + scaled + `,"config":{"text":{"mode":"field","field":"cpu"}}}`, spec.CanvasElement{Kind: "metric", Field: "cpu"}},
		{"text with fixed text", `{"type":"text","name":"t",` + scaled + `,"config":{"text":{"mode":"fixed","fixed":"Label"}}}`, spec.CanvasElement{Kind: "text", Text: "Label"}},
		{"text with a field", `{"type":"text","name":"t",` + scaled + `,"config":{"text":{"mode":"field","field":"cpu"}}}`, spec.CanvasElement{Kind: "metric", Field: "cpu"}},
		{"ellipse with a field", `{"type":"ellipse","name":"t",` + scaled + `,"config":{"text":{"mode":"field","field":"cpu"}}}`, spec.CanvasElement{Kind: "ellipse", Field: "cpu"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, _ := canvasPanel(t, 12, 10, tc.element)
			els := canvasElements(t, panel)
			if len(els) != 1 || els[0].Kind != tc.want.Kind || els[0].Text != tc.want.Text || els[0].Field != tc.want.Field {
				t.Fatalf("elements = %+v, want %+v", els, tc.want)
			}
		})
	}
}

func TestCanvasColorsStyleAndFieldColors(t *testing.T) {
	const scaled = `"constraint":{"horizontal":"scale","vertical":"scale"},"placement":{"left":0,"top":0,"width":10,"height":10}`
	element := `{"type":"metric-value","name":"m",` + scaled + `,"config":{"text":{"mode":"field","field":"cpu"},"color":{"fixed":"#ffffff"},"size":30,"align":"left","valign":"middle"},` +
		`"background":{"color":{"field":"cpu"}},"border":{"width":1,"color":{"fixed":"red"}}}`
	panel, line := canvasPanel(t, 12, 10, element)
	want := spec.CanvasElement{ID: "m", Kind: "metric", Width: 10, Height: 10, Field: "cpu", Fill: "field", Stroke: "#F2495C", TextColor: "#ffffff", FontSize: 30, Align: "left"}
	if got := canvasElements(t, panel); !reflect.DeepEqual(got, []spec.CanvasElement{want}) {
		t.Fatalf("elements = %+v, want %+v", got, want)
	}
	if len(line.Paths) != 0 {
		t.Fatalf("paths = %v, reasons = %v", line.Paths, line.Reasons)
	}
}

func TestCanvasOptionsWithoutACounterpartAreReported(t *testing.T) {
	const scaled = `"constraint":{"horizontal":"scale","vertical":"scale"},"placement":{"left":0,"top":0,"width":10,"height":10,"rotation":45}`
	element := `{"type":"rectangle","name":"m",` + scaled + `,"config":{"size":100,"valign":"top"},` +
		`"background":{"color":{"field":"other"},"image":{"fixed":"img/a.png"}},"border":{"width":3,"radius":4,"color":{"fixed":"red"}}}`
	_, line := canvasPanel(t, 12, 10, element)
	want := []string{
		"options.root.elements[0].placement.rotation",
		"options.root.elements[0].background.color.field",
		"options.root.elements[0].border.width",
		"options.root.elements[0].border.radius",
		"options.root.elements[0].background.image",
		"options.root.elements[0].config.size",
		"options.root.elements[0].config.valign",
	}
	for _, path := range want {
		if !slices.Contains(line.Paths, path) {
			t.Errorf("paths = %v, want %s reported", line.Paths, path)
		}
	}
}

func TestCanvasLeavesOutUnsupportedElementsAndNeverImportsLinksOrActions(t *testing.T) {
	const scaled = `"constraint":{"horizontal":"scale","vertical":"scale"},"placement":{"left":0,"top":0,"width":10,"height":10}`
	elements := `{"type":"server","name":"s",` + scaled + `},` +
		`{"type":"rectangle","name":"r",` + scaled + `,"links":[{"title":"Docs","url":"https://example.com"}],"actions":[{"type":"fetch","fetch":{"url":"https://example.com/restart","method":"POST"}}]}`
	panel, line := canvasPanel(t, 12, 10, elements)
	if els := canvasElements(t, panel); len(els) != 1 || els[0].ID != "r" {
		t.Fatalf("elements = %+v, want only the rectangle", els)
	}
	for _, path := range []string{"options.root.elements[0]", "options.root.elements[1].links", "options.root.elements[1].actions"} {
		if !slices.Contains(line.Paths, path) {
			t.Errorf("paths = %v, want %s reported", line.Paths, path)
		}
	}
	if !slices.Contains(line.Reasons, fmt.Sprintf(canvasTypeReason, "server")) {
		t.Errorf("reasons = %v", line.Reasons)
	}
	data, err := json.Marshal(panel)
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"actions", "links", "fetch", "example.com"} {
		if strings.Contains(string(data), word) {
			t.Errorf("the imported panel holds %q: %s", word, data)
		}
	}
}

func TestCanvasIdsAreCleanedAndUnique(t *testing.T) {
	const scaled = `"constraint":{"horizontal":"scale","vertical":"scale"},"placement":{"left":0,"top":0,"width":10,"height":10}`
	names := []string{"Web server", "Web server", "1st", "", strings.Repeat("a", 80)}
	var elements []string
	for _, name := range names {
		elements = append(elements, fmt.Sprintf(`{"type":"rectangle","name":%q,%s}`, name, scaled))
	}
	panel, _ := canvasPanel(t, 12, 10, strings.Join(elements, ","))
	var ids []string
	for _, el := range canvasElements(t, panel) {
		ids = append(ids, el.ID)
	}
	want := []string{"Web_server", "Web_server_2", "e_1st", "e_", strings.Repeat("a", 64)}
	if !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
}

func TestCanvasConnectionsFollowElementNames(t *testing.T) {
	const scaled = `"constraint":{"horizontal":"scale","vertical":"scale"},"placement":{"left":0,"top":0,"width":10,"height":10}`
	elements := `{"type":"rectangle","name":"A",` + scaled + `,"connections":[` +
		`{"targetName":"B","color":{"fixed":"blue"},"size":{"fixed":4},"path":"straight","lineStyle":{"style":"solid"},"source":{"x":1,"y":0},"target":{"x":0,"y":0}},` +
		`{"targetName":"B","color":{"field":"cpu"},"size":{"fixed":2},"path":"curved","lineStyle":{"style":"dashed"}},` +
		`{"targetName":"Gone"},` +
		`{"targetName":"Icon"},` +
		`{"targetName":"B","size":{"fixed":5,"field":"cpu"}}]},` +
		`{"type":"rectangle","name":"B",` + scaled + `},` +
		`{"type":"icon","name":"Icon",` + scaled + `}`
	panel, line := canvasPanel(t, 12, 10, elements)
	want := []spec.CanvasConnection{
		{From: "A", To: "B", Color: "#5794F2", Width: 4},
		{From: "A", To: "B", Color: "field", Field: "cpu"},
		{From: "A", To: "B", Width: 5},
	}
	if got := displayOf(t, panel).Connections; !reflect.DeepEqual(got, want) {
		t.Fatalf("connections = %+v, want %+v", got, want)
	}
	for _, path := range []string{
		"options.root.elements[0].connections[1].path", "options.root.elements[0].connections[1].lineStyle.style",
		"options.root.elements[0].connections[2]", "options.root.elements[0].connections[3]", "options.root.elements[0].connections[4].size.field",
	} {
		if !slices.Contains(line.Paths, path) {
			t.Errorf("paths = %v, want %s reported", line.Paths, path)
		}
	}
}

func TestCanvasGroupsAreFlattenedIntoTheRoot(t *testing.T) {
	const scaled = `"constraint":{"horizontal":"scale","vertical":"scale"}`
	group := `{"type":"frame","name":"Group",` + scaled + `,"placement":{"left":50,"top":50,"width":50,"height":50},"elements":[` +
		`{"type":"rectangle","name":"Inner",` + scaled + `,"placement":{"left":50,"top":0,"width":50,"height":100}}]}`
	panel, line := canvasPanel(t, 12, 10, group)
	want := []spec.CanvasElement{{ID: "Inner", Kind: "rectangle", Left: 75, Top: 50, Width: 25, Height: 50}}
	if got := canvasElements(t, panel); !reflect.DeepEqual(got, want) {
		t.Fatalf("elements = %+v, want %+v", got, want)
	}
	if !slices.Contains(line.Reasons, canvasFrameReason) {
		t.Fatalf("reasons = %v", line.Reasons)
	}
}

func TestCanvasKeepsTheFirstFiftyElementsAndHundredConnections(t *testing.T) {
	const scaled = `"constraint":{"horizontal":"scale","vertical":"scale"},"placement":{"left":0,"top":0,"width":10,"height":10}`
	var elements []string
	for i := range 52 {
		connections := ""
		if i == 0 {
			var items []string
			for range 101 {
				items = append(items, `{"targetName":"e1"}`)
			}
			connections = `,"connections":[` + strings.Join(items, ",") + `]`
		}
		elements = append(elements, fmt.Sprintf(`{"type":"rectangle","name":"e%d",%s%s}`, i, scaled, connections))
	}
	panel, line := canvasPanel(t, 12, 10, strings.Join(elements, ","))
	if n := len(canvasElements(t, panel)); n != 50 {
		t.Errorf("elements = %d, want 50", n)
	}
	if n := len(displayOf(t, panel).Connections); n != 100 {
		t.Errorf("connections = %d, want 100", n)
	}
	for _, reason := range []string{canvasElementsReason, canvasConnLimitReason} {
		if !slices.Contains(line.Reasons, reason) {
			t.Errorf("reasons = %v, want %q", line.Reasons, reason)
		}
	}
}

func alertListPanel(t *testing.T, options string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("alertlist", `"options":{`+options+`}`))
}

func TestAlertListBecomesAPrometheusAlertsQuery(t *testing.T) {
	cases := []struct {
		name, options, query, display string
		paths                         []string
	}{
		{
			name:    "defaults",
			options: `"datasource":"prom","maxItems":20,"sortOrder":1,"viewMode":"list","dashboardAlerts":false,"groupMode":"default","groupBy":[],"alertName":"","alertInstanceLabelFilter":"","showInstances":false,"stateFilter":{"firing":true,"pending":true,"recovering":true,"error":true,"noData":false,"normal":false},"statColorMode":"none","statValueMappings":[]`,
			query:   `ALERTS{alertstate=~"firing|pending"}`,
			display: "null",
		},
		{
			name:    "only firing alerts",
			options: `"datasource":"prom","stateFilter":{"firing":true,"pending":false}`,
			query:   `ALERTS{alertstate="firing"}`,
			display: "null",
		},
		{
			name:    "only pending alerts",
			options: `"datasource":"prom","stateFilter":{"firing":false,"pending":true}`,
			query:   `ALERTS{alertstate="pending"}`,
			display: "null",
		},
		{
			name:    "states Prometheus does not have",
			options: `"datasource":"prom","stateFilter":{"firing":true,"pending":true,"normal":true,"noData":true}`,
			query:   `ALERTS{alertstate=~"firing|pending"}`,
			display: "null",
			paths:   []string{"options.stateFilter.noData", "options.stateFilter.normal"},
		},
		{
			name:    "label filter in braces",
			options: `"datasource":"prom","alertInstanceLabelFilter":"{team=\"web\",severity=~\"warn.*\"}"`,
			query:   `ALERTS{alertstate=~"firing|pending",team="web",severity=~"warn.*"}`,
			display: "null",
		},
		{
			name:    "bare label matchers",
			options: `"datasource":"prom","alertInstanceLabelFilter":"team=\"web\", env!=\"dev\""`,
			query:   `ALERTS{alertstate=~"firing|pending",team="web", env!="dev"}`,
			display: "null",
		},
		{
			name:    "label filter that is not a matcher list",
			options: `"datasource":"prom","alertInstanceLabelFilter":"team=web"`,
			query:   `ALERTS{alertstate=~"firing|pending"}`,
			display: "null",
			paths:   []string{"options.alertInstanceLabelFilter"},
		},
		{
			name:    "alert name",
			options: `"datasource":"prom","alertName":"high.latency \"p99\""`,
			query:   `ALERTS{alertstate=~"firing|pending",alertname=~"(?i).*high\\.latency \"p99\".*"}`,
			display: "null",
		},
		{
			name:    "limit, sort and stat view",
			options: `"datasource":"prom","maxItems":5,"sortOrder":5,"viewMode":"stat"`,
			query:   `ALERTS{alertstate=~"firing|pending"}`,
			display: `{"limit":5,"sort":"timeDesc","viewMode":"stat"}`,
		},
		{
			name:    "limit and sort out of range",
			options: `"datasource":"prom","maxItems":500,"sortOrder":9`,
			query:   `ALERTS{alertstate=~"firing|pending"}`,
			display: "null",
			paths:   []string{"options.maxItems", "options.sortOrder"},
		},
		{
			name:    "options with no counterpart",
			options: `"datasource":"prom","dashboardAlerts":true,"groupMode":"custom","groupBy":["alertname"],"folder":{"uid":"f","title":"F"},"showInstances":true,"statColorMode":"background"`,
			query:   `ALERTS{alertstate=~"firing|pending"}`,
			display: "null",
			paths:   []string{"options.dashboardAlerts", "options.folder.title", "options.folder.uid", "options.groupBy", "options.groupMode", "options.showInstances", "options.statColorMode"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := alertListPanel(t, tc.options)
			if panel.Type != spec.AlertList || len(panel.Queries) != 1 || panel.Queries[0].Query != tc.query {
				t.Fatalf("panel = %+v, want one query %s", panel, tc.query)
			}
			if panel.Datasource != "prom" {
				t.Errorf("datasource = %q", panel.Datasource)
			}
			if got := displayJSON(t, panel); got != tc.display {
				t.Errorf("display = %s, want %s", got, tc.display)
			}
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v (reasons %v)", line.Paths, tc.paths, line.Reasons)
			}
		})
	}
}

func TestAlertListsThatPrometheusCannotListArePlaceholders(t *testing.T) {
	cases := []struct{ name, options, reason string }{
		{"legacy show option", `"show":"current","maxItems":10`, legacyAlertListReason},
		{"legacy state array", `"stateFilter":["alerting","pending"],"datasource":"prom"`, legacyAlertListReason},
		{"grafana-managed alerts", `"datasource":""`, managedAlertListReason},
		{"no datasource", `"maxItems":10`, managedAlertListReason},
		{"the grafana rule source", `"datasource":"grafana"`, managedAlertListReason},
		{"a source that is not Prometheus", `"datasource":{"type":"grafana-clickhouse-datasource","uid":"ch"}`, alertSourceReason},
		{"a source that is not supported", `"datasource":{"type":"loki","uid":"loki1"}`, "Loki is not supported yet."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := alertListPanel(t, tc.options)
			if panel.Type != spec.Placeholder || panel.Reason != tc.reason || line.Status != statusPlaceholder {
				t.Fatalf("panel = %+v, line status %s, want a placeholder with %q", panel, line.Status, tc.reason)
			}
		})
	}
}

func TestAlertListNeverGuessesBetweenTwoPrometheusDatasources(t *testing.T) {
	doc := panelDoc("alertlist", `"options":{"datasource":"Rules"}`)
	res := convertWith(t, doc, Options{Datasources: []Datasource{{UID: "a", Type: "prometheus"}, {UID: "b", Type: "prometheus"}}})
	if res.Ready() {
		t.Fatalf("result is ready without a choice between %v", res.Datasources)
	}
	var found bool
	for _, source := range res.Datasources {
		if source.Key == "Rules" {
			found = true
			if source.Match != nil || len(source.Candidates) != 2 {
				t.Errorf("source = %+v, want no match and two candidates", source)
			}
		}
	}
	if !found {
		t.Fatalf("datasources = %+v, want the alert list's source", res.Datasources)
	}
	mapped := convertWith(t, doc, Options{Datasources: []Datasource{{UID: "a", Type: "prometheus"}, {UID: "b", Type: "prometheus"}}, Mapping: map[string]string{"Rules": "b"}})
	entry, ok := mapped.Spec.FindPanel("p1")
	if !ok || entry.Panel.Datasource != "b" {
		t.Fatalf("panel = %+v, want datasource b", entry)
	}
}

func dashListPanel(t *testing.T, options string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("dashlist", `"options":{`+options+`}`))
}

func TestDashListImportsAsAStaticPanel(t *testing.T) {
	panel, line := dashListPanel(t, `"query":"checkout","tags":["Prod","slo.v2","","a b"],"maxItems":15,"keepTime":true,"includeVars":true,"showHeadings":true,"showFolderNames":false,"showSearch":true,"showStarred":false,"showRecentlyViewed":false,"folderUID":"","folderId":0`)
	if panel.Type != spec.DashList || panel.Datasource != "" || len(panel.Queries) != 0 {
		t.Fatalf("panel = %+v, want a static panel", panel)
	}
	want := `{"limit":15,"query":"checkout","tags":["prod","slo.v2","a-b"],"keepTime":true,"includeVars":true}`
	if got := displayJSON(t, panel); got != want {
		t.Errorf("display = %s, want %s", got, want)
	}
	if !slices.Equal(line.Paths, []string{"options.tags[2]"}) {
		t.Errorf("paths = %v, want only the empty tag reported", line.Paths)
	}
}

func TestDashListNeverNeedsADatasourceMatch(t *testing.T) {
	doc := `{"title":"T","schemaVersion":39,"panels":[{"id":1,"title":"T","type":"dashlist","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"loki","uid":"l"},"options":{"maxItems":10}}]}`
	_, line := convertedPanel(t, doc)
	if line.Status != statusImported {
		t.Fatalf("line = %+v", line)
	}
}

func TestDashListOptionsWithoutACounterpartAreReported(t *testing.T) {
	_, line := dashListPanel(t, `"showStarred":true,"showRecentlyViewed":true,"folderUID":"abc","maxItems":500,"tags":["a","b","c","d","e","f","g","h","i","j","k"]`)
	for _, path := range []string{"options.showStarred", "options.showRecentlyViewed", "options.folderUID", "options.maxItems", "options.tags[10]"} {
		if !slices.Contains(line.Paths, path) {
			t.Errorf("paths = %v, want %s reported", line.Paths, path)
		}
	}
	if !slices.Contains(line.Reasons, dashListSources) || !slices.Contains(line.Reasons, dashListFolders) {
		t.Errorf("reasons = %v", line.Reasons)
	}
}

func TestStaticPanelsIgnoreLeftoverTargets(t *testing.T) {
	targets := map[string]string{
		"datasource only": `[{"datasource":{"uid":"$datasource"},"refId":"A"}]`,
		"with a query":    `[{"refId":"A","expr":"up"},{"refId":"B","expr":"down","hide":true}]`,
	}
	for _, typ := range []string{"text", "dashlist", "news"} {
		for name, list := range targets {
			t.Run(typ+" "+name, func(t *testing.T) {
				panel, line := convertedPanel(t, panelDoc(typ, `"targets":`+list))
				if len(panel.Queries) != 0 || len(line.Queries) != 0 {
					t.Errorf("queries = %v, lines = %v, want none", panel.Queries, line.Queries)
				}
				if line.Status != statusImported || len(line.Paths) != 0 || len(line.Reasons) != 0 {
					t.Errorf("line = %+v, want imported with nothing reported", line)
				}
			})
		}
	}
}

func newsPanel(t *testing.T, members string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("news", members))
}

func TestNewsImportsAsANewsPanel(t *testing.T) {
	longFeed := "https://example.com/" + strings.Repeat("a", 2001-len("https://example.com/"))
	cases := []struct {
		name    string
		members string
		feed    string
		paths   []string
		reason  string
	}{
		{"explicit https url", `"options":{"feedUrl":"https://example.com/feed.xml"}`, "https://example.com/feed.xml", nil, ""},
		{"surrounding spaces are trimmed", `"options":{"feedUrl":"  https://example.com/feed.xml\n"}`, "https://example.com/feed.xml", nil, ""},
		{"empty url is Grafana's blog", `"options":{"feedUrl":""}`, "https://grafana.com/blog/news.xml", nil, ""},
		{"blank url is Grafana's blog", `"options":{"feedUrl":"   "}`, "https://grafana.com/blog/news.xml", nil, ""},
		{"absent url is Grafana's blog", `"options":{"showImage":true}`, "https://grafana.com/blog/news.xml", nil, ""},
		{"no options is Grafana's blog", `"description":"d"`, "https://grafana.com/blog/news.xml", nil, ""},
		{"http is upgraded and reported", `"options":{"feedUrl":"http://example.com/feed.xml"}`, "https://example.com/feed.xml", []string{"options.feedUrl"}, newsHTTPOnly},
		{"uppercase http is upgraded and reported", `"options":{"feedUrl":"HTTP://example.com/feed.xml"}`, "https://example.com/feed.xml", []string{"options.feedUrl"}, newsHTTPOnly},
		{"ftp is dropped", `"options":{"feedUrl":"ftp://x"}`, "", []string{"options.feedUrl"}, newsFeedBad},
		{"scheme only is dropped", `"options":{"feedUrl":"https://"}`, "", []string{"options.feedUrl"}, newsFeedBad},
		{"url with a space is dropped", `"options":{"feedUrl":"https://example.com/a b"}`, "", []string{"options.feedUrl"}, newsFeedBad},
		{"relative url is dropped", `"options":{"feedUrl":"/blog/news.xml"}`, "", []string{"options.feedUrl"}, newsFeedBad},
		{"non-string url is dropped", `"options":{"feedUrl":5}`, "", []string{"options.feedUrl"}, newsFeedBad},
		{"url of 2001 bytes is dropped", `"options":{"feedUrl":"` + longFeed + `"}`, "", []string{"options.feedUrl"}, newsFeedBad},
		{"url of 2000 bytes is kept", `"options":{"feedUrl":"` + longFeed[:2000] + `"}`, longFeed[:2000], nil, ""},
		{"showImage false and useProxy true are silent", `"options":{"feedUrl":"https://example.com/feed.xml","showImage":false,"useProxy":true}`, "https://example.com/feed.xml", nil, ""},
		{"repeat is reported", `"repeat":"host","options":{"feedUrl":"https://example.com/feed.xml"}`, "https://example.com/feed.xml", []string{"repeat"}, newsRepeat},
		{"links are reported", `"links":[{"title":"x","url":"https://example.com"}]`, "https://grafana.com/blog/news.xml", []string{"links"}, "Links on news panels are not supported yet."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := newsPanel(t, tc.members)
			if panel.Type != spec.News || panel.Datasource != "" || len(panel.Queries) != 0 {
				t.Fatalf("panel = %+v, want a static news panel", panel)
			}
			got := ""
			if panel.Display != nil {
				got = panel.Display.FeedURL
			}
			if got != tc.feed {
				t.Errorf("feedUrl = %q, want %q", got, tc.feed)
			}
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v", line.Paths, tc.paths)
			}
			if tc.reason != "" && !slices.Contains(line.Reasons, tc.reason) {
				t.Errorf("reasons = %v, want %q", line.Reasons, tc.reason)
			}
			wantStatus := statusImported
			if len(tc.paths) > 0 {
				wantStatus = statusPartial
			}
			if line.Status != wantStatus {
				t.Errorf("status = %s, want %s", line.Status, wantStatus)
			}
		})
	}
}

func TestNewsNeverNeedsADatasourceMatch(t *testing.T) {
	doc := `{"title":"T","schemaVersion":39,"panels":[{"id":1,"title":"T","type":"news","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"grafana","uid":"-- Grafana --"},"options":{"feedUrl":"https://example.com/feed.xml"}}]}`
	_, line := convertedPanel(t, doc)
	if line.Status != statusImported {
		t.Fatalf("line = %+v", line)
	}
}

func TestDiscretePanelImportsAsAStateTimeline(t *testing.T) {
	members := `"units":"percent","decimals":1,"valueMaps":[{"value":"null","op":"=","text":"N/A"},{"value":"1","op":"=","text":"UP"},{"value":"0","op":"!=","text":"nonzero"}],` +
		`"rangeMaps":[{"from":"null","to":"null","text":"N/A"},{"from":"2","to":"5","text":"SLOW"}],` +
		`"colorMaps":[{"text":"N/A","color":"#CCC"},{"text":"UP","color":"green"},{"text":"SLOW","color":"orange"},{"text":"unmapped","color":"#ff0000"},{"text":"bad","color":"nonsense"}],` +
		`"rowHeight":50,"textSize":24,"showLegend":true,"display":"timeline","timeOptions":[{"name":"Years","value":"years"}],"timePrecision":{"name":"Minutes","value":"minutes"}`
	panel, line := convertedPanel(t, panelDoc("natel-discrete-panel", upTarget+","+members))
	if panel.Type != spec.StateTimeline || line.SourceType != "natel-discrete-panel" {
		t.Fatalf("type = %s, source type %s", panel.Type, line.SourceType)
	}
	if panel.Unit != "percent" || panel.Decimals == nil || *panel.Decimals != 1 {
		t.Errorf("unit = %q, decimals = %v", panel.Unit, panel.Decimals)
	}
	two, five := 2.0, 5.0
	want := []spec.Mapping{
		{Special: "null", Text: "N/A", Color: "#CCCCCC"},
		{Value: "1", Text: "UP", Color: "ok"},
		{Range: &[2]*float64{&two, &five}, Text: "SLOW", Color: "warning"},
		{Value: "unmapped", Color: "#ff0000"},
	}
	if !reflect.DeepEqual(panel.Mappings, want) {
		t.Errorf("mappings = %+v, want %+v", panel.Mappings, want)
	}
	wantPaths := []string{"valueMaps[2]", "colorMaps[4]"}
	if !slices.Equal(line.Paths, wantPaths) {
		t.Errorf("paths = %v, want %v (reasons %v)", line.Paths, wantPaths, line.Reasons)
	}
}

func TestDiscreteLayoutOptionsAreReportedUnlessTheyAreDefaults(t *testing.T) {
	_, line := convertedPanel(t, panelDoc("natel-discrete-panel", upTarget+`,"rowHeight":80,"writeAllValues":true,"extendLastValue":true,"showTimeAxis":false`))
	want := []string{"rowHeight", "showTimeAxis", "writeAllValues"}
	if !slices.Equal(line.Paths, want) {
		t.Fatalf("paths = %v, want %v", line.Paths, want)
	}
}

func TestStatusHistoryColoredByThresholdsIsNotReported(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("status-history", upTarget+`,"fieldConfig":{"defaults":{"color":{"mode":"thresholds"},"thresholds":{"mode":"absolute","steps":[{"color":"green","value":null},{"color":"red","value":80}]}},"overrides":[]}`))
	if got := thresholdText(panel.Thresholds); !slices.Equal(got, []string{"ok", "critical@80"}) {
		t.Fatalf("thresholds = %v", got)
	}
	if len(line.Paths) != 0 {
		t.Fatalf("paths = %v, reasons = %v", line.Paths, line.Reasons)
	}
}
