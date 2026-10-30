package grafana

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxLegendValues = 8
	maxFooterValues = 5
	minDataPoints   = 10
	maxDataPoints   = 11000
	maxBucketCount  = 1000
	legacyHeatmapV  = 37
)

var (
	timeFromPattern  = regexp.MustCompile(`^(?:now-)?([1-9][0-9]*)([smhdwMy])$`)
	timeShiftPattern = regexp.MustCompile(`^[1-9][0-9]*[smhdwMy]$`)
)

var displayBuilders = map[spec.PanelType]func(*panelBuild){
	spec.Timeseries:    displayTimeseries,
	spec.Stat:          displayStat,
	spec.Gauge:         displayGauge,
	spec.BarGauge:      displayBarGauge,
	spec.Table:         displayTable,
	spec.PieChart:      displayPie,
	spec.Heatmap:       displayHeatmap,
	spec.Histogram:     displayHistogram,
	spec.StateTimeline: displayStateTimeline,
	spec.Logs:          displayLogs,
	spec.BarChart:      displayBarChart,
	spec.StatusHistory: displayStatusHistory,
	spec.Trend:         displayTrend,
	spec.XYChart:       displayXYChart,
	spec.Candlestick:   displayCandlestick,
	spec.NodeGraph:     displayNodeGraph,
	spec.Geomap:        displayGeomap,
	spec.Canvas:        displayCanvas,
	spec.AlertList:     displayAlertList,
}

func (b *panelBuild) convertDisplay() {
	if build := displayBuilders[b.typ]; build != nil {
		build(b)
	}
}

func (b *panelBuild) disp() *spec.Display {
	if b.display == nil {
		b.display = &spec.Display{}
	}
	return b.display
}

func (b *panelBuild) enumOption(path string, accepted ...string) string {
	v, _ := b.value(path)
	s := asString(v)
	if !slices.Contains(accepted, s) {
		return ""
	}
	b.consume(path)
	return s
}

func (b *panelBuild) boolOption(path string) (*bool, bool) {
	v, _ := b.value(path)
	flag, isBool := v.(bool)
	if !isBool {
		return nil, false
	}
	b.consume(path)
	return &flag, true
}

func (b *panelBuild) reduceOption(d *spec.Display) {
	if v, ok := b.value("options.reduceOptions.calcs"); ok {
		calcs := stringList(v)
		b.consume("options.reduceOptions.calcs")
		b.applyCalcs(d, calcs)
	}
	if truthy(lookupValue(b.raw, "options.reduceOptions.values")) {
		b.partial("options.reduceOptions.values", "Showing every value instead of one reduced value is not supported yet.")
	}
}

func (b *panelBuild) applyCalcs(d *spec.Display, calcs []string) {
	if len(calcs) == 0 {
		return
	}
	if !isReducer(calcs[0]) {
		b.partial("options.reduceOptions.calcs", fmt.Sprintf("The reducer %s is not supported yet.", calcs[0]))
		return
	}
	d.Reduce = calcs[0]
	if len(calcs) > 1 {
		b.partial("options.reduceOptions.calcs", "Only the first reducer is used.")
	}
}

func (b *panelBuild) legendPlacement(fallback string) string {
	show, hasShow := b.boolOption("options.legend.showLegend")
	mode := b.enumOption("options.legend.displayMode", "list", "hidden")
	placement := b.enumOption("options.legend.placement", "right", "bottom")
	if v, ok := b.value("options.legend.asTable"); ok && !truthy(v) {
		b.consume("options.legend.asTable")
	}
	switch {
	case hasShow && !*show, mode == "hidden":
		return "hidden"
	case placement == "right" || placement == "bottom":
		return placement
	}
	return fallback
}

func displayTimeseries(b *panelBuild) {
	d := b.disp()
	b.legendAndTooltip(d)
	if v, ok := b.value("options.legend.calcs"); ok {
		d.LegendValues = b.legendValues(v)
	}
}

func (b *panelBuild) legendAndTooltip(d *spec.Display) {
	if placement := b.legendPlacement(""); placement != "" {
		d.Legend = placement
	}
	switch b.enumOption("options.tooltip.mode", "single", "multi", "none") {
	case "multi":
		d.Tooltip = "all"
	case "none":
		d.Tooltip = "hidden"
	}
}

func (b *panelBuild) legendValues(v any) []string {
	b.consume("options.legend.calcs")
	var out []string
	unsupported := false
	for _, calc := range stringList(v) {
		switch {
		case !isReducer(calc):
			unsupported = true
		case slices.Contains(out, calc):
		case len(out) >= maxLegendValues:
			unsupported = true
		default:
			out = append(out, calc)
		}
	}
	if unsupported {
		b.partial("options.legend.calcs", "Some legend values are not supported yet.")
	}
	return out
}

func displayStat(b *panelBuild) {
	d := b.disp()
	b.reduceOption(d)
	switch mode := b.enumOption("options.colorMode", "value", "background", "background_solid", "none"); mode {
	case "":
	case "background_solid":
		d.ColorMode = "background"
	default:
		d.ColorMode = mode
	}
	b.statGraphAndText(d)
}

func (b *panelBuild) statGraphAndText(d *spec.Display) {
	switch v, _ := b.value("options.graphMode"); asString(v) {
	case "none":
		b.consume("options.graphMode")
		graph := false
		d.Graph = &graph
	case "area":
		b.consume("options.graphMode")
	}
	switch mode := b.enumOption("options.textMode", "auto", "value", "name", "none", "value_and_name"); mode {
	case "value_and_name":
		d.TextMode = "valueAndName"
	case "auto":
	default:
		d.TextMode = mode
	}
	d.Orientation = b.orientation()
}

func (b *panelBuild) orientation() string {
	return b.enumOption("options.orientation", "auto", "horizontal", "vertical")
}

func displayGauge(b *panelBuild) {
	d := b.disp()
	b.reduceOption(d)
	if flag, ok := b.boolOption("options.showThresholdMarkers"); ok {
		d.ThresholdMarkers = flag
	}
}

func displayBarGauge(b *panelBuild) {
	d := b.disp()
	b.reduceOption(d)
	d.Orientation = b.orientation()
}

func displayTable(b *panelBuild) {
	d := b.disp()
	if b.n.sourceType == "datagrid" {
		b.datagridOptions()
	}
	if flag, ok := b.boolOption("options.showHeader"); ok {
		d.Header = flag
	}
	show, hasShow := b.boolOption("options.footer.show")
	reducer, _ := b.take("options.footer.reducer")
	if hasShow && *show {
		d.Footer = b.footerReducers(reducer)
	}
}

func (b *panelBuild) footerReducers(v any) []string {
	var out []string
	unsupported := false
	for _, calc := range stringList(v) {
		switch {
		case !isReducer(calc):
			unsupported = true
		case slices.Contains(out, calc):
		case len(out) >= maxFooterValues:
			unsupported = true
		default:
			out = append(out, calc)
		}
	}
	if unsupported {
		b.partial("options.footer.reducer", "Some footer values are not supported yet.")
	}
	return out
}

func displayPie(b *panelBuild) {
	d := b.disp()
	b.reduceOption(d)
	d.PieType = b.enumOption("options.pieType", "pie", "donut")
	d.Legend = b.legendPlacement("bottom")
}

func displayHeatmap(b *panelBuild) {
	d := b.disp()
	if flag, ok := b.boolOption("options.calculate"); ok {
		d.Calculate = flag
	}
	b.heatmapScale(d)
	if format := asString(b.raw["dataFormat"]); format != "" && b.c.schemaVersion() < legacyHeatmapV {
		calculate := format != "tsbuckets"
		d.Calculate = &calculate
		b.partial("dataFormat", "This heatmap uses the old Grafana heatmap; its options are imported as far as they map.")
	}
}

func (b *panelBuild) heatmapScale(d *spec.Display) {
	v, _ := b.value("fieldConfig.defaults.custom.scaleDistribution.type")
	scale := asString(v)
	if scale == "" {
		scale = asString(lookupValue(b.raw, "options.yAxis.scaleType"))
		b.consume("options.yAxis.scaleType")
	}
	if scale != "log" {
		return
	}
	d.YScale = "log"
	b.consume("fieldConfig.defaults.custom.scaleDistribution")
}

func displayHistogram(b *panelBuild) {
	d := b.disp()
	if v, ok := b.take("options.bucketSize"); ok {
		if size, isNumber := numberValue(v); isNumber && size > 0 {
			d.BucketSize = &size
		}
	}
	if v, ok := b.take("options.bucketCount"); ok {
		if count, isNumber := numberValue(v); isNumber && count == math.Trunc(count) && count >= 1 && count <= maxBucketCount {
			d.BucketCount = int(count)
		}
	}
}

func displayStateTimeline(b *panelBuild) {
	d := b.disp()
	if flag, ok := b.boolOption("options.mergeValues"); ok {
		d.MergeValues = flag
	}
	d.ShowValues = b.enumOption("options.showValue", "auto", "always", "never")
}

func displayLogs(b *panelBuild) {
	d := b.disp()
	if flag, ok := b.boolOption("options.wrapLogMessage"); ok {
		d.Wrap = flag
	}
	if flag, ok := b.boolOption("options.showTime"); ok {
		d.ShowTime = flag
	}
	switch b.enumOption("options.sortOrder", "Ascending", "Descending") {
	case "Ascending":
		d.Order = "oldest"
	case "Descending":
		d.Order = "newest"
	}
}

func (c *converter) schemaVersion() int {
	n, _ := asInt(c.src.dash["schemaVersion"])
	return n
}

func (b *panelBuild) convertPanelOptions() {
	b.convertRepeat()
	b.convertPanelTimes()
	if v, ok := b.value("maxDataPoints"); ok {
		b.convertMaxDataPoints(v)
	}
	if interval := strings.TrimSpace(asString(b.raw["interval"])); interval != "" {
		if value, ok := b.c.minStep(interval); ok {
			b.panel.Interval = value
		} else {
			b.partial("interval", fmt.Sprintf("The min interval %s cannot be kept; SlashObserve picks the step.", interval))
		}
	}
}

func (b *panelBuild) convertRepeat() {
	name := asString(b.raw["repeat"])
	if name == "" {
		b.consume("minSpan")
		return
	}
	if !b.c.declaredVariables()[name] {
		b.partial("repeat", fmt.Sprintf("Its repeat variable $%s could not be imported, so the panel is not repeated.", name))
		return
	}
	b.panel.Repeat = name
	switch dir := asString(b.raw["repeatDirection"]); dir {
	case "h", "v":
		b.panel.RepeatDirection = dir
	}
	if n, ok := asInt(b.raw["maxPerRow"]); ok && n >= 1 && n <= spec.GridColumns {
		b.panel.MaxPerRow = n
	} else if present(b.raw["maxPerRow"]) {
		b.partial("maxPerRow", "The maximum number of panels per row is not supported.")
	}
}

func (b *panelBuild) convertPanelTimes() {
	if from := strings.TrimSpace(asString(b.raw["timeFrom"])); from != "" {
		if m := timeFromPattern.FindStringSubmatch(from); m != nil {
			b.panel.TimeFrom = "now-" + m[1] + m[2]
		} else {
			b.partial("timeFrom", fmt.Sprintf("The relative time %s is not supported yet.", from))
		}
	}
	if shift := strings.TrimSpace(asString(b.raw["timeShift"])); shift != "" {
		if timeShiftPattern.MatchString(shift) {
			b.panel.TimeShift = shift
		} else {
			b.partial("timeShift", fmt.Sprintf("The time shift %s is not supported yet.", shift))
		}
	}
}

func (b *panelBuild) convertMaxDataPoints(v any) {
	f, ok := numberValue(v)
	switch {
	case !ok || f == 0, b.typ == spec.Geomap && f == 1:
	case f != math.Trunc(f) || f < minDataPoints || f > maxDataPoints:
		b.partial("maxDataPoints", "Only 10 to 11000 data points are supported.")
	default:
		b.panel.MaxDataPoints = int(f)
	}
}
