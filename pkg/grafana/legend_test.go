package grafana

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func timeseriesLegend(t *testing.T, legend string) (spec.Display, PanelLine) {
	t.Helper()
	panel, line := convertedPanel(t, panelDoc("timeseries", upTarget+`,"options":{"legend":`+legend+`}`))
	if panel.Display == nil {
		return spec.Display{}, line
	}
	return *panel.Display, line
}

func TestTimeseriesLegendModeFollowsGrafanaOnlyWhereTheImpliedModeDiffers(t *testing.T) {
	cases := []struct {
		name, legend, wantMode string
		wantValues             []string
	}{
		{"list with calcs", `{"displayMode":"list","calcs":["mean"]}`, "list", []string{"mean"}},
		{"table without calcs", `{"displayMode":"table","calcs":[]}`, "table", nil},
		{"table with calcs", `{"displayMode":"table","calcs":["mean"]}`, "", []string{"mean"}},
		{"list without calcs", `{"displayMode":"list","calcs":[]}`, "", nil},
		{"asTable without calcs", `{"asTable":true,"calcs":[]}`, "table", nil},
		{"asTable with calcs", `{"asTable":true,"calcs":["max"]}`, "", []string{"max"}},
		{"asTable false with calcs", `{"asTable":false,"calcs":["max"]}`, "list", []string{"max"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := timeseriesLegend(t, tc.legend)
			if d.LegendMode != tc.wantMode {
				t.Fatalf("legendMode = %q, want %q", d.LegendMode, tc.wantMode)
			}
			if !slices.Equal(d.LegendValues, tc.wantValues) {
				t.Fatalf("legendValues = %v, want %v", d.LegendValues, tc.wantValues)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestTimeseriesLegendSortWidthAndVisibility(t *testing.T) {
	d, line := timeseriesLegend(t, `{"isVisible":false,"sortBy":"Mean","sortDesc":true,"width":250.4}`)
	if d.Legend != "hidden" || d.LegendSortBy != "mean" || d.LegendSortDesc == nil || !*d.LegendSortDesc || d.LegendWidth != 250 {
		t.Fatalf("display = %+v", d)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}

	d, _ = timeseriesLegend(t, `{"sortBy":"Name","sortDesc":false}`)
	if d.LegendSortBy != "name" || d.LegendSortDesc != nil {
		t.Fatalf("display = %+v", d)
	}

	d, _ = timeseriesLegend(t, `{"sortBy":"Last *","isVisible":true}`)
	if d.LegendSortBy != "lastNotNull" || d.Legend != "" {
		t.Fatalf("display = %+v", d)
	}
}

func TestTimeseriesLegendWidthIsClamped(t *testing.T) {
	cases := []struct {
		width string
		want  int
	}{{"10", 40}, {"5000", 2000}, {"0", 0}}
	for _, tc := range cases {
		d, line := timeseriesLegend(t, `{"width":`+tc.width+`}`)
		if d.LegendWidth != tc.want {
			t.Fatalf("width %s: legendWidth = %d, want %d", tc.width, d.LegendWidth, tc.want)
		}
		if line.Status != statusImported {
			t.Fatalf("width %s: status = %s, paths %v", tc.width, line.Status, line.Paths)
		}
	}
}

func TestTimeseriesLegendSortByUnknownValueIsReported(t *testing.T) {
	d, line := timeseriesLegend(t, `{"sortBy":"Bogus calc"}`)
	if d.LegendSortBy != "" {
		t.Fatalf("legendSortBy = %q", d.LegendSortBy)
	}
	if line.Status != statusPartial || !slices.Contains(line.Paths, "options.legend.sortBy") {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTrendLegendImportsLikeTimeseries(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("trend", upTarget+`,"options":{"legend":{"displayMode":"table","calcs":[]}}`))
	if got := displayOf(t, panel).LegendMode; got != "table" {
		t.Fatalf("legendMode = %q", got)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestLegacyGraphLegendTable(t *testing.T) {
	cases := []struct {
		name, legend, wantMode string
	}{
		{"values without alignAsTable", `{"show":true,"values":true,"avg":true}`, "list"},
		{"values aligned as a table", `{"show":true,"values":true,"avg":true,"alignAsTable":true}`, ""},
		{"table without values", `{"show":true,"alignAsTable":true}`, "table"},
		{"no values and no table", `{"show":true,"alignAsTable":false}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := graphPanel(t, `"legend":`+tc.legend)
			if got := displayOf(t, panel).LegendMode; got != tc.wantMode {
				t.Fatalf("legendMode = %q, want %q", got, tc.wantMode)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestLegacyGraphLegendSortWidthAndHiding(t *testing.T) {
	panel, line := graphPanel(t, `"legend":{"show":true,"sort":"current","sortDesc":true,"sideWidth":300,"hideZero":true,"hideEmpty":true}`)
	d := displayOf(t, panel)
	if d.LegendSortBy != "lastNotNull" || d.LegendSortDesc == nil || !*d.LegendSortDesc || d.LegendWidth != 300 {
		t.Fatalf("display = %+v", d)
	}
	if d.LegendHideZero == nil || !*d.LegendHideZero || d.LegendHideEmpty == nil || !*d.LegendHideEmpty {
		t.Fatalf("hide flags = %+v", d)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestLegacyGraphLegendFalseHidingIsSilent(t *testing.T) {
	panel, line := graphPanel(t, `"legend":{"show":true,"hideZero":false,"hideEmpty":false,"sort":null}`)
	d := displayOf(t, panel)
	if d.LegendHideZero != nil || d.LegendHideEmpty != nil || d.LegendSortBy != "" {
		t.Fatalf("display = %+v", d)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}
