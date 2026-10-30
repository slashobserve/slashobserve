package grafana

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func pieOptions(t *testing.T, options string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("piechart", upTarget+`,"options":`+options))
}

func TestPieSliceLabels(t *testing.T) {
	cases := []struct {
		name, labels string
		want         []string
	}{
		{"none is the default", `[]`, nil},
		{"name", `["name"]`, []string{"name"}},
		{"percent", `["percent"]`, []string{"percent"}},
		{"written in Grafana's order, stored in drawing order", `["percent","name"]`, []string{"name", "percent"}},
		{"duplicates collapse", `["value","value"]`, []string{"value"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := pieOptions(t, `{"displayLabels":`+tc.labels+`}`)
			requireImported(t, line)
			if got := displayOf(t, panel).SliceLabels; !slices.Equal(got, tc.want) {
				t.Fatalf("sliceLabels = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPieSliceLabelOfAnUnknownKindIsReported(t *testing.T) {
	panel, line := pieOptions(t, `{"displayLabels":["name","bogus"]}`)
	if line.Status != statusPartial || !slices.Contains(line.Paths, "options.displayLabels") {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	if got := displayOf(t, panel).SliceLabels; !slices.Equal(got, []string{"name"}) {
		t.Fatalf("sliceLabels = %v", got)
	}
}

func TestPieLegendValuesAreWrittenWhereGrafanaDiffersFromTheSpecDefault(t *testing.T) {
	cases := []struct {
		name, options string
		want          *[]string
	}{
		{"no legend options show no values in Grafana", `{}`, &[]string{}},
		{"an empty list shows none", `{"legend":{"values":[]}}`, &[]string{}},
		{"value only", `{"legend":{"values":["value"]}}`, &[]string{"value"}},
		{"percent only", `{"legend":{"values":["percent"]}}`, &[]string{"percent"}},
		{"both equal the Spec default", `{"legend":{"values":["value","percent"]}}`, nil},
		{"both in the other order equal the Spec default", `{"legend":{"values":["percent","value"]}}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := pieOptions(t, tc.options)
			requireImported(t, line)
			got := displayOf(t, panel).LegendShow
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("legendShow = %v, want absent", *got)
			case tc.want != nil && (got == nil || !slices.Equal(*got, *tc.want)):
				t.Fatalf("legendShow = %v, want %v", got, *tc.want)
			}
		})
	}
}

func TestPieLegendValuesOfAnEmptyListSurviveSerialisation(t *testing.T) {
	panel, _ := pieOptions(t, `{}`)
	raw, err := json.Marshal(displayOf(t, panel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"legendShow":[]`) {
		t.Fatalf("display = %s", raw)
	}
}

func TestPieLegendTableSortAndWidth(t *testing.T) {
	panel, line := pieOptions(t, `{"legend":{"displayMode":"table","sortBy":"Value","sortDesc":true,"width":240.4,"values":["value","percent"]}}`)
	requireImported(t, line)
	d := displayOf(t, panel)
	if d.LegendMode != "table" || d.LegendSortBy != "value" || d.LegendSortDesc == nil || !*d.LegendSortDesc || d.LegendWidth != 240 {
		t.Fatalf("display = %+v", d)
	}
}

func TestPieLegendAsTableAndSortByName(t *testing.T) {
	panel, line := pieOptions(t, `{"legend":{"asTable":true,"sortBy":"Name"}}`)
	requireImported(t, line)
	d := displayOf(t, panel)
	if d.LegendMode != "table" || d.LegendSortBy != "name" || d.LegendSortDesc != nil {
		t.Fatalf("display = %+v", d)
	}
}

func TestPieLegendListStaysTheDefault(t *testing.T) {
	panel, line := pieOptions(t, `{"legend":{"displayMode":"list","asTable":false}}`)
	requireImported(t, line)
	if mode := displayOf(t, panel).LegendMode; mode != "" {
		t.Fatalf("legendMode = %q", mode)
	}
}

func TestPieLegendSortByACalculationIsReported(t *testing.T) {
	panel, line := pieOptions(t, `{"legend":{"sortBy":"Mean"}}`)
	if line.Status != statusPartial || !slices.Contains(line.Paths, "options.legend.sortBy") {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	if got := displayOf(t, panel).LegendSortBy; got != "" {
		t.Fatalf("legendSortBy = %q", got)
	}
}

func TestPieTooltip(t *testing.T) {
	cases := []struct {
		name, tooltip string
		check         func(spec.Display) bool
	}{
		{"single is the default", `{"mode":"single","sort":"none"}`, func(d spec.Display) bool {
			return d.Tooltip == "" && d.TooltipSort == ""
		}},
		{"multi lists every slice", `{"mode":"multi"}`, func(d spec.Display) bool { return d.Tooltip == "all" }},
		{"none hides the tooltip", `{"mode":"none"}`, func(d spec.Display) bool { return d.Tooltip == "hidden" }},
		{"sort ascending", `{"mode":"multi","sort":"asc"}`, func(d spec.Display) bool { return d.TooltipSort == "asc" }},
		{"sort descending", `{"mode":"multi","sort":"desc"}`, func(d spec.Display) bool { return d.TooltipSort == "desc" }},
		{"sort is kept with a single tooltip", `{"mode":"single","sort":"asc"}`, func(d spec.Display) bool {
			return d.Tooltip == "" && d.TooltipSort == "asc"
		}},
		{"hide zeros", `{"hideZeros":true}`, func(d spec.Display) bool { return d.TooltipHideZeros != nil && *d.TooltipHideZeros }},
		{"hide zeros false is the default", `{"hideZeros":false}`, func(d spec.Display) bool { return d.TooltipHideZeros == nil }},
		{"size limits", `{"maxWidth":300.4,"maxHeight":200}`, func(d spec.Display) bool {
			return d.TooltipMaxWidth == 300 && d.TooltipMaxHeight == 200
		}},
		{"size limits are clamped", `{"maxWidth":5000,"maxHeight":10}`, func(d spec.Display) bool {
			return d.TooltipMaxWidth == 2000 && d.TooltipMaxHeight == 100
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := pieOptions(t, `{"tooltip":`+tc.tooltip+`}`)
			requireImported(t, line)
			if d := displayOf(t, panel); !tc.check(d) {
				t.Fatalf("display = %+v", d)
			}
		})
	}
}

func TestPieTextSizes(t *testing.T) {
	panel, line := pieOptions(t, `{"text":{"titleSize":20,"valueSize":40}}`)
	requireImported(t, line)
	if d := displayOf(t, panel); d.TitleSize != 20 || d.ValueSize != 40 {
		t.Fatalf("display = %+v", d)
	}
	_, line = pieOptions(t, `{"text":{"titleSize":0}}`)
	if line.Status != statusPartial || !slices.Contains(line.Paths, "options.text.titleSize") {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
}

func TestPieHidesSeriesFromTheLegendAndTheTooltip(t *testing.T) {
	custom := `{"hideFrom":{"legend":true,"tooltip":true,"viz":false}}`
	panel, line := convertedPanel(t, panelDoc("piechart", upTarget+`,"fieldConfig":{"defaults":{"custom":`+custom+`},"overrides":[]}`))
	if !clean(line) {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	if panel.HideInLegend == nil || !*panel.HideInLegend || panel.HideInTooltip == nil || !*panel.HideInTooltip {
		t.Fatalf("panel props = %+v", panel.FieldProps)
	}
	panel, line = convertedPanel(t, overridePanel("piechart", byUp, property("custom.hideFrom", `{"legend":true,"tooltip":false,"viz":false}`)))
	if !clean(line) {
		t.Fatalf("override: status %s, paths %v", line.Status, line.Paths)
	}
	if got := onlyOverride(t, panel); got.HideInLegend == nil || !*got.HideInLegend || got.HideInTooltip != nil {
		t.Fatalf("override props = %+v", got.FieldProps)
	}
}

func TestPieDoesNotDrawTheOtherSeriesProps(t *testing.T) {
	_, line := convertedPanel(t, panelDoc("piechart", upTarget+`,"fieldConfig":{"defaults":{"custom":{"transform":"negative-Y"}},"overrides":[]}`))
	if line.Status != statusPartial || !slices.Contains(line.Paths, "fieldConfig.defaults.custom.transform") {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
}

func TestOtherTypesStillReportHidingFromTheLegend(t *testing.T) {
	_, line := convertedPanel(t, panelDoc("state-timeline", upTarget+`,"fieldConfig":{"defaults":{"custom":{"hideFrom":{"legend":true}}},"overrides":[]}`))
	if line.Status != statusPartial || !slices.Contains(line.Paths, "fieldConfig.defaults.custom.hideFrom.legend") {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
}

func TestLegacyPieChartKeepsItsLegendValuesAndReportsThem(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("grafana-piechart-panel", upTarget+`,"legend":{"show":true,"values":true,"percentage":true}`))
	if panel.Type != spec.PieChart {
		t.Fatalf("type = %s", panel.Type)
	}
	if got := displayOf(t, panel).LegendShow; got != nil {
		t.Fatalf("legendShow = %v, want absent for a legacy pie", *got)
	}
	if line.Status != statusPartial {
		t.Fatalf("status = %s", line.Status)
	}
}
