package grafana

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func vizPanel(t *testing.T, typ, options string) (spec.Display, PanelLine) {
	t.Helper()
	panel, line := convertedPanel(t, panelDoc(typ, upTarget+`,"options":`+options))
	if panel.Display == nil {
		return spec.Display{}, line
	}
	return *panel.Display, line
}

func TestGaugeThresholdLabelsAndTextSizesImport(t *testing.T) {
	d, line := vizPanel(t, "gauge", `{"showThresholdLabels":true,"showThresholdMarkers":false,"text":{"titleSize":18,"valueSize":44}}`)
	if d.ThresholdLabels == nil || !*d.ThresholdLabels || d.ThresholdMarkers == nil || *d.ThresholdMarkers {
		t.Fatalf("display = %+v", d)
	}
	if d.TitleSize != 18 || d.ValueSize != 44 {
		t.Fatalf("sizes = %d, %d, want 18, 44", d.TitleSize, d.ValueSize)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestVizLimitsApplyOnlyWhenSizingIsManual(t *testing.T) {
	cases := []struct {
		name, typ, options string
		want               vizLimits
	}{
		{"gauge auto ignores the limits", "gauge", `{"sizing":"auto","minVizWidth":120,"minVizHeight":130}`, vizLimits{}},
		{"gauge without sizing ignores the limits", "gauge", `{"minVizWidth":120}`, vizLimits{}},
		{"gauge manual keeps its limits", "gauge", `{"sizing":"manual","minVizWidth":120,"minVizHeight":130}`, vizLimits{minWidth: 120, minHeight: 130}},
		{"gauge manual falls back to Grafana's limits", "gauge", `{"sizing":"manual"}`, vizLimits{minWidth: 75, minHeight: 75}},
		{"gauge manual with a zero limit has no minimum", "gauge", `{"sizing":"manual","minVizWidth":0}`, vizLimits{minHeight: 75}},
		{"bar gauge auto ignores the limits", "bargauge", `{"sizing":"auto","minVizHeight":10,"minVizWidth":0,"maxVizHeight":300}`, vizLimits{}},
		{"bar gauge manual keeps its limits", "bargauge", `{"sizing":"manual","minVizWidth":40,"minVizHeight":30,"maxVizHeight":100}`, vizLimits{minWidth: 40, minHeight: 30, maxHeight: 100}},
		{"bar gauge manual falls back to Grafana's limits", "bargauge", `{"sizing":"manual"}`, vizLimits{minWidth: 8, minHeight: 16, maxHeight: 300}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := vizPanel(t, tc.typ, tc.options)
			got := vizLimits{minWidth: d.MinWidth, minHeight: d.MinHeight, maxHeight: d.MaxHeight}
			if got != tc.want {
				t.Fatalf("limits = %+v, want %+v", got, tc.want)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestBarGaugeOptionsImport(t *testing.T) {
	d, line := vizPanel(t, "bargauge", `{"displayMode":"lcd","valueMode":"hidden","namePlacement":"left","showUnfilled":false,"text":{"titleSize":12,"valueSize":30}}`)
	if d.BarStyle != "lcd" || d.ValueMode != "hidden" || d.NamePlacement != "left" {
		t.Fatalf("display = %+v", d)
	}
	if d.ShowUnfilled == nil || *d.ShowUnfilled {
		t.Fatalf("showUnfilled = %v, want false", d.ShowUnfilled)
	}
	if d.TitleSize != 12 || d.ValueSize != 30 {
		t.Fatalf("sizes = %d, %d, want 12, 30", d.TitleSize, d.ValueSize)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestBarGaugeFlatStylesAndGrafanaDefaultsWriteNothing(t *testing.T) {
	cases := []struct{ name, options string }{
		{"gradient", `{"displayMode":"gradient"}`},
		{"basic", `{"displayMode":"basic"}`},
		{"color values", `{"valueMode":"color"}`},
		{"automatic names", `{"namePlacement":"auto"}`},
		{"unfilled area", `{"showUnfilled":true}`},
		{"empty text sizes", `{"text":{}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := vizPanel(t, "bargauge", tc.options)
			if d.BarStyle != "" || d.ValueMode != "" || d.NamePlacement != "" || d.ShowUnfilled != nil || d.TitleSize != 0 {
				t.Fatalf("display = %+v", d)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestVizValuesOutOfRangeAreReported(t *testing.T) {
	cases := []struct{ name, typ, options, path string }{
		{"limit above 2000", "bargauge", `{"sizing":"manual","maxVizHeight":2001}`, "options.maxVizHeight"},
		{"negative limit", "gauge", `{"sizing":"manual","minVizWidth":-5}`, "options.minVizWidth"},
		{"fractional limit", "gauge", `{"sizing":"manual","minVizHeight":80.5}`, "options.minVizHeight"},
		{"zero title size", "gauge", `{"text":{"titleSize":0}}`, "options.text.titleSize"},
		{"value size above 200", "bargauge", `{"text":{"valueSize":201}}`, "options.text.valueSize"},
		{"unknown name placement", "bargauge", `{"namePlacement":"bottom"}`, "options.namePlacement"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := vizPanel(t, tc.typ, tc.options)
			if line.Status != statusPartial || !slices.Equal(line.Paths, []string{tc.path}) {
				t.Fatalf("status = %s, paths %v, want partial at %s", line.Status, line.Paths, tc.path)
			}
			if d.MaxHeight != 0 || d.TitleSize != 0 || d.ValueSize != 0 || d.NamePlacement != "" {
				t.Fatalf("display = %+v, want the value left out", d)
			}
		})
	}
}

func TestLegacySinglestatGaugeImportsThresholdLabels(t *testing.T) {
	panel, line := singlestatPanel(t, `"gauge":{"show":true,"thresholdMarkers":true,"thresholdLabels":true}`)
	d := displayOf(t, panel)
	if d.ThresholdLabels == nil || !*d.ThresholdLabels {
		t.Fatalf("display = %+v", d)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v, reasons %v", line.Status, line.Paths, line.Reasons)
	}
}
