package grafana

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func tooltipPanel(t *testing.T, typ, tooltip string) (spec.Display, PanelLine) {
	t.Helper()
	panel, line := convertedPanel(t, panelDoc(typ, upTarget+`,"options":{"tooltip":`+tooltip+`}`))
	return displayIfAny(panel), line
}

func displayIfAny(panel spec.Panel) spec.Display {
	if panel.Display == nil {
		return spec.Display{}
	}
	return *panel.Display
}

func TestTooltipSortFollowsGrafanaWhereSeveralSeriesAreListed(t *testing.T) {
	cases := []struct {
		name, tooltip, want string
	}{
		{"multi with no sort", `{"mode":"multi"}`, "none"},
		{"multi with sort none", `{"mode":"multi","sort":"none"}`, "none"},
		{"multi with desc equals the Spec default", `{"mode":"multi","sort":"desc"}`, ""},
		{"multi with asc", `{"mode":"multi","sort":"asc"}`, "asc"},
		{"single with no sort", `{"mode":"single"}`, ""},
		{"single with asc lists one series", `{"mode":"single","sort":"asc"}`, ""},
		{"hidden with asc", `{"mode":"none","sort":"asc"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := tooltipPanel(t, "timeseries", tc.tooltip)
			requireImported(t, line)
			if d.TooltipSort != tc.want {
				t.Fatalf("tooltipSort = %q, want %q", d.TooltipSort, tc.want)
			}
		})
	}
}

func TestTooltipSortOfATrendAndACandlestick(t *testing.T) {
	d, line := tooltipPanel(t, "trend", `{"mode":"multi"}`)
	requireImported(t, line)
	if d.TooltipSort != "none" {
		t.Fatalf("trend tooltipSort = %q", d.TooltipSort)
	}
	for _, tooltip := range []string{`{"mode":"single"}`, `{"mode":"multi"}`} {
		d, line = tooltipPanel(t, "candlestick", tooltip)
		requireImported(t, line)
		if d.TooltipSort != "none" {
			t.Fatalf("candlestick %s tooltipSort = %q", tooltip, d.TooltipSort)
		}
	}
	d, _ = tooltipPanel(t, "candlestick", `{"mode":"none"}`)
	if d.TooltipSort != "" {
		t.Fatalf("hidden candlestick tooltipSort = %q", d.TooltipSort)
	}
}

func TestTooltipSortOfASharedCursorDashboard(t *testing.T) {
	doc := strings.Replace(panelDoc("timeseries", upTarget+`,"options":{"tooltip":{"mode":"single"}}`), `"schemaVersion":39,`, `"graphTooltip":2,"schemaVersion":39,`, 1)
	panel, line := convertedPanel(t, doc)
	requireImported(t, line)
	if got := displayIfAny(panel).TooltipSort; got != "none" {
		t.Fatalf("tooltipSort = %q", got)
	}
	crosshair := strings.Replace(doc, `"graphTooltip":2`, `"graphTooltip":1`, 1)
	panel, _ = convertedPanel(t, crosshair)
	if got := displayIfAny(panel).TooltipSort; got != "" {
		t.Fatalf("crosshair tooltipSort = %q", got)
	}
}

func TestTooltipUnknownSortIsReported(t *testing.T) {
	d, line := tooltipPanel(t, "timeseries", `{"mode":"multi","sort":"random"}`)
	if d.TooltipSort != "" {
		t.Fatalf("tooltipSort = %q", d.TooltipSort)
	}
	if line.Status != statusPartial || len(line.Paths) != 1 || line.Paths[0] != "options.tooltip.sort" {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTooltipHideZeros(t *testing.T) {
	d, line := tooltipPanel(t, "timeseries", `{"mode":"single","hideZeros":true}`)
	requireImported(t, line)
	if d.TooltipHideZeros == nil || !*d.TooltipHideZeros {
		t.Fatalf("display = %+v", d)
	}
	d, line = tooltipPanel(t, "timeseries", `{"mode":"single","hideZeros":false}`)
	requireImported(t, line)
	if d.TooltipHideZeros != nil {
		t.Fatalf("hideZeros false is the Spec default, got %v", *d.TooltipHideZeros)
	}
}

func TestTooltipSizesAreRoundedAndClamped(t *testing.T) {
	cases := []struct {
		name, tooltip     string
		wantWidth, wantHt int
	}{
		{"inside the range", `{"maxWidth":300,"maxHeight":200}`, 300, 200},
		{"rounded", `{"maxWidth":300.6,"maxHeight":199.4}`, 301, 199},
		{"too small", `{"maxWidth":20,"maxHeight":0}`, 100, 100},
		{"too large", `{"maxWidth":5000,"maxHeight":2001}`, 2000, 2000},
		{"absent", `{}`, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := tooltipPanel(t, "timeseries", tc.tooltip)
			requireImported(t, line)
			if d.TooltipMaxWidth != tc.wantWidth || d.TooltipMaxHeight != tc.wantHt {
				t.Fatalf("sizes = %d x %d", d.TooltipMaxWidth, d.TooltipMaxHeight)
			}
		})
	}
}

func TestTooltipSizeThatIsNotANumberIsReported(t *testing.T) {
	d, line := tooltipPanel(t, "timeseries", `{"maxWidth":"wide"}`)
	if d.TooltipMaxWidth != 0 {
		t.Fatalf("tooltipMaxWidth = %d", d.TooltipMaxWidth)
	}
	if line.Status != statusPartial || len(line.Paths) != 1 || line.Paths[0] != "options.tooltip.maxWidth" {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestLegacyGraphTooltipSort(t *testing.T) {
	cases := []struct {
		name, tooltip, want string
	}{
		{"shared with sort 0", `{"shared":true,"sort":0}`, "none"},
		{"shared with no sort", `{"shared":true}`, "none"},
		{"shared with ascending", `{"shared":true,"sort":1}`, "asc"},
		{"shared with sort 2 equals the Spec default", `{"shared":true,"sort":2}`, ""},
		{"not shared lists one series", `{"shared":false,"sort":1}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := graphPanel(t, `"tooltip":`+tc.tooltip)
			requireImported(t, line)
			if got := displayIfAny(panel).TooltipSort; got != tc.want {
				t.Fatalf("tooltipSort = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLegacyGraphTooltipUnknownSortIsReported(t *testing.T) {
	_, line := graphPanel(t, `"tooltip":{"shared":true,"sort":7}`)
	if line.Status != statusPartial || len(line.Paths) != 1 || line.Paths[0] != "tooltip.sort" {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestLegacyGraphTooltipCumulativeValues(t *testing.T) {
	panel, line := graphPanel(t, `"stack":true,"tooltip":{"shared":true,"value_type":"cumulative","sort":2}`)
	requireImported(t, line)
	d := displayOf(t, panel)
	if d.TooltipCumulative == nil || !*d.TooltipCumulative || d.TooltipSort != "" {
		t.Fatalf("display = %+v", d)
	}
	panel, line = graphPanel(t, `"tooltip":{"shared":true,"value_type":"individual"}`)
	requireImported(t, line)
	if displayOf(t, panel).TooltipCumulative != nil {
		t.Fatalf("individual values are the Spec default")
	}
}

func TestPanelWithOnlyTooltipOptionsImportsFully(t *testing.T) {
	d, line := tooltipPanel(t, "timeseries", `{"mode":"multi","sort":"asc","hideZeros":true,"maxWidth":400,"maxHeight":300}`)
	requireImported(t, line)
	if d.Tooltip != "all" || d.TooltipSort != "asc" || d.TooltipHideZeros == nil || d.TooltipMaxWidth != 400 || d.TooltipMaxHeight != 300 {
		t.Fatalf("display = %+v", d)
	}
}
