package grafana

import (
	"encoding/json"
	"fmt"
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
