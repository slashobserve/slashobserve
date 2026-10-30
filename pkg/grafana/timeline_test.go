package grafana

import (
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func timelinePanel(t *testing.T, typ, options, custom string) (spec.Panel, PanelLine) {
	t.Helper()
	members := upTarget + `,"options":` + options
	if custom != "" {
		members += `,"fieldConfig":{"defaults":{"custom":` + custom + `},"overrides":[]}`
	}
	return convertedPanel(t, panelDoc(typ, members))
}

func TestTimelineRowHeightAndColumnWidthAreWrittenWheneverGrafanaHasThem(t *testing.T) {
	cases := []struct {
		name, typ, options, want string
	}{
		{"state timeline row height", "state-timeline", `{"rowHeight":0.5}`, `"rowHeight":0.5`},
		{"state timeline default height", "state-timeline", `{"rowHeight":0.9}`, `"rowHeight":0.9`},
		{"status history row height", "status-history", `{"rowHeight":0.4}`, `"rowHeight":0.4`},
		{"status history column width", "status-history", `{"colWidth":0.6}`, `"columnWidth":0.6`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := timelinePanel(t, tc.typ, tc.options, "")
			requireImported(t, line)
			if got := displayJSON(t, panel); !strings.Contains(got, tc.want) {
				t.Fatalf("display = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestTimelineSizesOutsideTheRangeAreReported(t *testing.T) {
	cases := []struct {
		typ, options, path string
	}{
		{"state-timeline", `{"rowHeight":0.05}`, "options.rowHeight"},
		{"state-timeline", `{"rowHeight":1.5}`, "options.rowHeight"},
		{"status-history", `{"colWidth":0}`, "options.colWidth"},
	}
	for _, tc := range cases {
		t.Run(tc.typ+" "+tc.path, func(t *testing.T) {
			_, line := timelinePanel(t, tc.typ, tc.options, "")
			if line.Status != statusPartial || strings.Join(line.Paths, " ") != tc.path {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestStateTimelineAlignValue(t *testing.T) {
	for _, align := range []string{"center", "right"} {
		panel, line := timelinePanel(t, "state-timeline", `{"alignValue":"`+align+`"}`, "")
		requireImported(t, line)
		if got := displayOf(t, panel).AlignValue; got != align {
			t.Fatalf("alignValue = %q, want %q", got, align)
		}
	}
	panel, line := timelinePanel(t, "state-timeline", `{"alignValue":"left"}`, "")
	requireImported(t, line)
	if got := displayIfAny(panel).AlignValue; got != "" {
		t.Fatalf("alignValue = %q, want the default dropped", got)
	}
}

func TestTimelineLegendAndTooltipImport(t *testing.T) {
	for _, typ := range []string{"state-timeline", "status-history"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := timelinePanel(t, typ, `{"legend":{"showLegend":true,"displayMode":"table","placement":"right","width":240},"tooltip":{"mode":"multi","sort":"asc","hideZeros":true,"maxWidth":300,"maxHeight":200}}`, "")
			requireImported(t, line)
			d := displayOf(t, panel)
			if d.Legend != "right" || d.LegendMode != "table" || d.LegendWidth != 240 {
				t.Fatalf("legend = %q %q %d", d.Legend, d.LegendMode, d.LegendWidth)
			}
			if d.Tooltip != "all" || d.TooltipSort != "asc" || d.TooltipHideZeros == nil || !*d.TooltipHideZeros || d.TooltipMaxWidth != 300 || d.TooltipMaxHeight != 200 {
				t.Fatalf("tooltip = %+v", d)
			}
		})
	}
}

func TestTimelineHiddenLegendAndTooltip(t *testing.T) {
	for _, legend := range []string{`{"showLegend":false}`, `{"displayMode":"hidden"}`} {
		panel, line := timelinePanel(t, "state-timeline", `{"legend":`+legend+`,"tooltip":{"mode":"none"}}`, "")
		requireImported(t, line)
		d := displayOf(t, panel)
		if d.Legend != "hidden" || d.Tooltip != "hidden" {
			t.Fatalf("legend %s: legend = %q tooltip = %q", legend, d.Legend, d.Tooltip)
		}
	}
}

func TestTimelineLegendSortAndCalculationsStayReported(t *testing.T) {
	_, line := timelinePanel(t, "state-timeline", `{"legend":{"sortBy":"Mean","sortDesc":true,"calcs":["mean"]}}`, "")
	if line.Status != statusPartial {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
	want := "options.legend.calcs options.legend.sortBy options.legend.sortDesc"
	got := append([]string(nil), line.Paths...)
	if strings.Join(got, " ") != want {
		t.Fatalf("paths = %v, want %s", got, want)
	}
}

func TestTimelineLineWidthIsWrittenAsGrafanaHasIt(t *testing.T) {
	for _, width := range []int{0, 1, 3} {
		for _, typ := range []string{"state-timeline", "status-history"} {
			panel, line := timelinePanel(t, typ, `{}`, `{"lineWidth":`+strconv.Itoa(width)+`}`)
			requireImported(t, line)
			if panel.LineWidth == nil || *panel.LineWidth != width {
				t.Fatalf("%s lineWidth %d imported as %v", typ, width, panel.LineWidth)
			}
		}
	}
}

func TestTimelineSpanNullsImport(t *testing.T) {
	panel, line := timelinePanel(t, "state-timeline", `{}`, `{"spanNulls":true}`)
	requireImported(t, line)
	if panel.SpanNulls != true {
		t.Fatalf("spanNulls = %v", panel.SpanNulls)
	}
	panel, line = timelinePanel(t, "state-timeline", `{}`, `{"spanNulls":60000}`)
	requireImported(t, line)
	if panel.SpanNulls == nil || panel.SpanNulls == false {
		t.Fatalf("spanNulls = %v", panel.SpanNulls)
	}
}

func TestTimelineFillOpacityIsWrittenAndATransparentFillIsReported(t *testing.T) {
	panel, line := timelinePanel(t, "state-timeline", `{}`, `{"fillOpacity":40}`)
	requireImported(t, line)
	if panel.Fill == nil || *panel.Fill != 40 {
		t.Fatalf("fill = %v", panel.Fill)
	}
	_, line = timelinePanel(t, "status-history", `{}`, `{"fillOpacity":0}`)
	if line.Status != statusPartial || strings.Join(line.Paths, " ") != "fieldConfig.defaults.custom.fillOpacity" {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTimelineLineStyleAndAxisPropsAreStillReported(t *testing.T) {
	for _, typ := range []string{"state-timeline", "status-history"} {
		for _, custom := range []string{`{"showPoints":"always"}`, `{"axisLabel":"x"}`, `{"lineInterpolation":"smooth"}`} {
			_, line := timelinePanel(t, typ, `{}`, custom)
			if line.Status != statusPartial {
				t.Fatalf("%s %s: status = %s", typ, custom, line.Status)
			}
		}
	}
}

func TestDiscreteLegendCanBeHidden(t *testing.T) {
	panel, _ := convertedPanel(t, panelDoc("natel-discrete-panel", upTarget+`,"showLegend":false`))
	if got := displayOf(t, panel).Legend; got != "hidden" {
		t.Fatalf("legend = %q", got)
	}
}
