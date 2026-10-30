package grafana

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxLegendValues       = 8
	maxFooterValues       = 5
	minDataPoints         = 10
	maxDataPoints         = 11000
	maxBucketCount        = 1000
	maxReduceRows         = 5000
	maxReduceFieldsLength = 200
	minStatTextSize       = 6
	maxVizLimit           = 2000
	maxTextSize           = 200
	minTooltipSize        = 100
	maxTooltipSize        = 2000
	legacyHeatmapV        = 37

	grafanaHeatmapSteps     = 64
	grafanaHeatmapScheme    = "Oranges"
	grafanaHeatmapFill      = "dark-orange"
	grafanaHeatmapExponent  = 0.5
	grafanaHeatmapGap       = 1
	grafanaHeatmapFilterLow = 1e-9
	grafanaExemplarColor    = "rgba(255,0,255,0.7)"
	minColorSteps           = 2
	maxColorSteps           = 128
	minColorExponent        = 0.1
	maxColorExponent        = 2
	maxCellGap              = 25
	maxCellRadius           = 10
)

var bucketDurationSeconds = map[string]float64{"": 1, "ms": 0.001, "s": 1, "sec": 1, "m": 60, "min": 60, "h": 3600, "hr": 3600, "d": 86400, "w": 604800}

var (
	timeFromPattern       = regexp.MustCompile(`^(?:now-)?([1-9][0-9]*)([smhdwMy])$`)
	timeShiftPattern      = regexp.MustCompile(`^[1-9][0-9]*[smhdwMy]$`)
	bucketDurationPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)(ms|s|sec|m|min|h|hr|d|w)?$`)
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
	spec.Traces:        displayTraces,
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
	if every, ok := b.boolOption("options.reduceOptions.values"); ok && *every {
		d.AllValues = every
	}
	b.reduceRowLimit(d)
	b.reduceFields(d)
}

func (b *panelBuild) reduceRowLimit(d *spec.Display) {
	v, ok := b.value("options.reduceOptions.limit")
	if !ok {
		return
	}
	limit, isInt := asInt(v)
	if !isInt || limit < 1 || limit > maxReduceRows {
		b.partial("options.reduceOptions.limit", fmt.Sprintf("The row limit must be a whole number from 1 to %d.", maxReduceRows))
		return
	}
	b.consume("options.reduceOptions.limit")
	if limit != maxReduceRows {
		d.Limit = limit
	}
}

func (b *panelBuild) reduceFields(d *spec.Display) {
	v, ok := b.value("options.reduceOptions.fields")
	if !ok {
		return
	}
	fields, isString := v.(string)
	if !isString || utf8.RuneCountInString(fields) > maxReduceFieldsLength {
		return
	}
	b.consume("options.reduceOptions.fields")
	d.Fields = fields
}

func (b *panelBuild) applyCalcs(d *spec.Display, calcs []string) {
	if len(calcs) == 0 {
		return
	}
	if !isReducer(calcs[0]) {
		b.partial("options.reduceOptions.calcs", fmt.Sprintf("The reducer %s is not supported yet.", calcs[0]))
		return
	}
	// Grafana itself uses only the first reducer here, so the rest change nothing.
	d.Reduce = calcs[0]
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
	b.seriesLegendAndTooltip(b.disp())
}

func (b *panelBuild) seriesLegendAndTooltip(d *spec.Display) {
	b.legendAndTooltip(d)
	b.tooltipOptions(d)
	if v, ok := b.value("options.legend.calcs"); ok {
		d.LegendValues = b.legendValues(v)
	}
	b.legendTable(d)
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

func (b *panelBuild) tooltipOptions(d *spec.Display) {
	if hide, ok := b.boolOption("options.tooltip.hideZeros"); ok && *hide {
		d.TooltipHideZeros = hide
	}
	d.TooltipMaxWidth = b.tooltipSizeOption("options.tooltip.maxWidth")
	d.TooltipMaxHeight = b.tooltipSizeOption("options.tooltip.maxHeight")
	if b.n.sourceType == "graph" {
		return
	}
	sort := "none"
	if v, has := b.value("options.tooltip.sort"); has && v != nil {
		if sort = b.enumOption("options.tooltip.sort", "none", "asc", "desc"); sort == "" {
			return
		}
	}
	d.TooltipSort = b.writtenTooltipSort(d, sort)
}

func (b *panelBuild) tooltipSizeOption(path string) int {
	v, ok := b.value(path)
	if !ok {
		return 0
	}
	size, isNumber := numberValue(v)
	if !isNumber {
		b.partial(path, fmt.Sprintf("The tooltip size must be a number of pixels from %d to %d.", minTooltipSize, maxTooltipSize))
		return 0
	}
	b.consume(path)
	return min(max(int(math.Round(size)), minTooltipSize), maxTooltipSize)
}

func (b *panelBuild) writtenTooltipSort(d *spec.Display, sort string) string {
	if sortsInSeriesOrder[b.typ] {
		return sort
	}
	if sort == "desc" || !b.listsSeveralSeries(d) {
		return ""
	}
	return sort
}

func (b *panelBuild) listsSeveralSeries(d *spec.Display) bool {
	if d.Tooltip == "hidden" {
		return false
	}
	return d.Tooltip == "all" || b.typ == spec.Candlestick || b.c.head.CursorSync == spec.CursorSyncTooltip
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
	b.statLayout(d)
	b.statPercentChange(d)
}

func (b *panelBuild) statLayout(d *spec.Display) {
	if b.enumOption("options.justifyMode", "auto", "center") == "center" {
		d.Justify = "center"
	}
	d.WideLayout, _ = b.boolOption("options.wideLayout")
	d.TitleSize = b.statSizeOption("options.text.titleSize")
	d.ValueSize = b.statSizeOption("options.text.valueSize")
}

func (b *panelBuild) statSizeOption(path string) int {
	v, ok := b.value(path)
	if !ok {
		return 0
	}
	size, isInt := asInt(v)
	if !isInt || size < minStatTextSize || size > maxTextSize {
		b.partial(path, fmt.Sprintf("The text size must be a whole number from %d to %d.", minStatTextSize, maxTextSize))
		return 0
	}
	b.consume(path)
	return size
}

func (b *panelBuild) statPercentChange(d *spec.Display) {
	if shown, ok := b.boolOption("options.showPercentChange"); ok && *shown {
		d.PercentChange = shown
	}
	switch b.enumOption("options.percentChangeColorMode", "standard", "inverted", "same_as_value") {
	case "inverted":
		d.PercentChangeColor = "inverted"
	case "same_as_value":
		d.PercentChangeColor = "value"
	}
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

type vizLimits struct{ minWidth, minHeight, maxHeight int }

var (
	gaugeSizingDefaults    = vizLimits{minWidth: 75, minHeight: 75}
	barGaugeSizingDefaults = vizLimits{minWidth: 8, minHeight: 16, maxHeight: 300}
)

func displayGauge(b *panelBuild) {
	d := b.disp()
	b.reduceOption(d)
	if flag, ok := b.boolOption("options.showThresholdMarkers"); ok {
		d.ThresholdMarkers = flag
	}
	if flag, ok := b.boolOption("options.showThresholdLabels"); ok {
		d.ThresholdLabels = flag
	}
	b.vizSizing(d, gaugeSizingDefaults)
	b.vizTextSizes(d)
}

func displayBarGauge(b *panelBuild) {
	d := b.disp()
	b.reduceOption(d)
	d.Orientation = b.orientation()
	d.BarStyle = b.enumOption("options.displayMode", "lcd")
	d.ValueMode = b.enumOption("options.valueMode", "color", "text", "hidden")
	d.NamePlacement = b.enumOption("options.namePlacement", "auto", "top", "left", "hidden")
	if flag, ok := b.boolOption("options.showUnfilled"); ok {
		d.ShowUnfilled = flag
	}
	b.vizSizing(d, barGaugeSizingDefaults)
	b.vizTextSizes(d)
}

func (b *panelBuild) vizSizing(d *spec.Display, grafanaDefaults vizLimits) {
	manual := b.enumOption("options.sizing", "auto", "manual") == "manual"
	d.MinWidth = b.vizLimit("options.minVizWidth", manual, grafanaDefaults.minWidth)
	d.MinHeight = b.vizLimit("options.minVizHeight", manual, grafanaDefaults.minHeight)
	if grafanaDefaults.maxHeight > 0 {
		d.MaxHeight = b.vizLimit("options.maxVizHeight", manual, grafanaDefaults.maxHeight)
	}
}

func (b *panelBuild) vizLimit(path string, manual bool, grafanaDefault int) int {
	v, present := b.value(path)
	if !manual {
		b.consume(path)
		return 0
	}
	if !present {
		return grafanaDefault
	}
	n, isInt := asInt(v)
	if !isInt || n < 0 || n > maxVizLimit {
		b.partial(path, fmt.Sprintf("A size limit must be a whole number from 1 to %d.", maxVizLimit))
		return 0
	}
	b.consume(path)
	return n
}

func (b *panelBuild) vizTextSizes(d *spec.Display) {
	d.TitleSize = b.textSize("options.text.titleSize")
	d.ValueSize = b.textSize("options.text.valueSize")
}

func (b *panelBuild) textSize(path string) int {
	v, ok := b.value(path)
	if !ok {
		return 0
	}
	n, isInt := asInt(v)
	if !isInt || n < 1 || n > maxTextSize {
		b.partial(path, fmt.Sprintf("A text size must be a whole number from 1 to %d.", maxTextSize))
		return 0
	}
	b.consume(path)
	return n
}

func displayTable(b *panelBuild) {
	d := b.disp()
	if b.n.sourceType == "datagrid" {
		b.datagridOptions()
	}
	if flag, ok := b.boolOption("options.showHeader"); ok {
		d.Header = flag
	}
	b.tableOptions(d)
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

var (
	pieSliceLabelOrder = []string{"name", "value", "percent"}
	pieLegendOrder     = []string{"value", "percent"}
)

func displayPie(b *panelBuild) {
	d := b.disp()
	b.reduceOption(d)
	d.PieType = b.enumOption("options.pieType", "pie", "donut")
	d.Legend = b.legendPlacement("bottom")
	d.SliceLabels = b.pieChoices("options.displayLabels", pieSliceLabelOrder, "Some slice labels are not supported yet.")
	b.pieLegend(d)
	b.pieTooltip(d)
	b.vizTextSizes(d)
}

func (b *panelBuild) pieChoices(path string, order []string, reason string) []string {
	v, present := b.value(path)
	if !present {
		return nil
	}
	items, isList := asArray(v)
	if !isList {
		return nil
	}
	b.consume(path)
	chosen := stringList(items)
	if len(chosen) != len(items) || slices.ContainsFunc(chosen, func(id string) bool { return !slices.Contains(order, id) }) {
		b.partial(path, reason)
	}
	var out []string
	for _, id := range order {
		if slices.Contains(chosen, id) {
			out = append(out, id)
		}
	}
	return out
}

func (b *panelBuild) pieLegend(d *spec.Display) {
	if b.n.sourceType == "piechart" {
		shown := b.pieChoices("options.legend.values", pieLegendOrder, "Some legend values are not supported yet.")
		if !slices.Equal(shown, pieLegendOrder) {
			d.LegendShow = ptr(append([]string{}, shown...))
		}
	}
	table := b.enumOption("options.legend.displayMode", "table", "list") == "table"
	if v, ok := b.boolOption("options.legend.asTable"); ok && *v {
		table = true
	}
	if table {
		d.LegendMode = "table"
	}
	if v, ok := b.take("options.legend.sortBy"); ok {
		b.legendSortBy(d, "options.legend.sortBy", v, pieSortKey)
	}
	b.legendSortDesc(d, "options.legend.sortDesc")
	b.legendWidth(d, "options.legend.width")
}

func pieSortKey(name string) (string, bool) {
	switch name {
	case "Name":
		return "name", true
	case "Value", "Percent":
		return "value", true
	}
	return "", false
}

func (b *panelBuild) pieTooltip(d *spec.Display) {
	switch b.enumOption("options.tooltip.mode", "single", "multi", "none") {
	case "multi":
		d.Tooltip = "all"
	case "none":
		d.Tooltip = "hidden"
	}
	if hide, ok := b.boolOption("options.tooltip.hideZeros"); ok && *hide {
		d.TooltipHideZeros = hide
	}
	d.TooltipMaxWidth = b.tooltipSizeOption("options.tooltip.maxWidth")
	d.TooltipMaxHeight = b.tooltipSizeOption("options.tooltip.maxHeight")
	if sort := b.enumOption("options.tooltip.sort", "none", "asc", "desc"); sort != "none" {
		d.TooltipSort = sort
	}
}

func displayHeatmap(b *panelBuild) {
	d := b.disp()
	if flag, ok := b.boolOption("options.calculate"); ok {
		d.Calculate = flag
	}
	b.heatmapScale(d)
	if b.isLegacyHeatmap() {
		b.legacyHeatmap(d)
		return
	}
	b.heatmapColors(d)
	b.heatmapCells(d)
	b.heatmapValueFilters(d)
	if shown, ok := b.boolOption("options.legend.show"); ok && !*shown {
		d.Legend = "hidden"
	}
	b.heatmapShowValues(d)
	b.heatmapCalculation(d)
	b.heatmapYAxis(d)
	b.heatmapRowsLayout(d)
	b.heatmapTooltip(d)
	b.heatmapExemplarColor(d)
}

func (b *panelBuild) heatmapExemplarColor(d *spec.Display) {
	const path = "options.exemplars.color"
	if v, ok := b.value(path); ok && v == grafanaExemplarColor {
		b.consume(path)
		return
	}
	d.ExemplarColor = b.heatmapColorValue(path)
}

func (b *panelBuild) isLegacyHeatmap() bool {
	return asString(b.raw["dataFormat"]) != "" && b.c.schemaVersion() < legacyHeatmapV
}

func (b *panelBuild) heatmapCalculation(d *spec.Display) {
	d.XBuckets = b.heatmapBuckets("options.calculation.xBuckets", xBucketSize)
	d.YBuckets = b.heatmapBuckets("options.calculation.yBuckets", yBucketSize)
	b.heatmapXBucketScale()
	b.heatmapYBucketScale(d)
}

func (b *panelBuild) heatmapBuckets(path string, size func(any) (float64, bool)) *spec.Buckets {
	mode := b.enumOption(path+".mode", "size", "count")
	buckets := &spec.Buckets{Mode: mode}
	if v, ok := b.value(path + ".value"); ok && v != nil && v != "" {
		parse := numberValue
		if mode != "count" {
			parse = size
		}
		if value, valid := parse(v); valid && value > 0 {
			buckets.Value = &value
			b.consume(path + ".value")
		} else {
			b.partial(path+".value", "The bucket size or count must be a positive number.")
		}
	} else if ok {
		b.consume(path + ".value")
	}
	switch {
	case buckets.Mode != "":
	case buckets.Value != nil:
		buckets.Mode = "size"
	default:
		return nil
	}
	return buckets
}

func yBucketSize(v any) (float64, bool) {
	return numberValue(v)
}

func xBucketSize(v any) (float64, bool) {
	if f, ok := asFloat(v); ok {
		return f, true
	}
	m := bucketDurationPattern.FindStringSubmatch(strings.TrimSpace(asString(v)))
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return n * bucketDurationSeconds[m[2]], true
}

func (b *panelBuild) heatmapXBucketScale() {
	const path = "options.calculation.xBuckets.scale"
	scale := b.enumOption(path+".type", "linear", "log", "symlog")
	if scale == "log" || scale == "symlog" {
		b.partial(path+".type", "Logarithmic time buckets are not supported yet.")
		return
	}
	b.consume(path + ".log")
}

func (b *panelBuild) heatmapYBucketScale(d *spec.Display) {
	const path = "options.calculation.yBuckets.scale"
	scale := b.enumOption(path+".type", "linear", "log", "symlog")
	if scale != "linear" || d.YScale != "" {
		d.YBucketScale = scale
	}
	if scale != "log" && scale != "symlog" {
		b.consume(path + ".log")
		return
	}
	if base, written := b.heatmapLogBase(path + ".log"); written {
		d.YBucketLogBase = base
	}
}

func (b *panelBuild) heatmapLogBase(path string) (int, bool) {
	v, ok := b.value(path)
	if !ok || v == nil {
		return 0, false
	}
	base, isInt := asInt(v)
	if !isInt || (base != 2 && base != defaultLogBase) {
		b.partial(path, logBaseReason)
		return 0, false
	}
	b.consume(path)
	return base, true
}

func (b *panelBuild) heatmapYAxis(d *spec.Display) {
	const prefix = "options.yAxis."
	switch v, _ := b.value(prefix + "axisPlacement"); asString(v) {
	case "right", "hidden":
		d.YAxis = b.enumOption(prefix+"axisPlacement", "right", "hidden")
	case "left", "auto":
		b.consume(prefix + "axisPlacement")
	}
	if v, ok := b.take(prefix + "unit"); ok {
		b.heatmapUnit(&d.YUnit, v, prefix+"unit")
	}
	d.YDecimals = b.heatmapDecimals(prefix + "decimals")
	d.YMin = b.heatmapNumberOption(prefix + "min")
	d.YMax = b.heatmapNumberOption(prefix + "max")
	if n, ok := b.heatmapWholeNumber(prefix+"axisWidth", 0, maxAxisWidth); ok {
		d.YAxisWidth = &n
	}
	b.heatmapAxisLabel(d, prefix+"axisLabel")
	if reverse, ok := b.boolOption(prefix + "reverse"); ok && *reverse {
		d.YReverse = reverse
	}
}

func (b *panelBuild) heatmapAxisLabel(d *spec.Display, path string) {
	v, _ := b.take(path)
	label := strings.TrimSpace(asString(v))
	if label == "" {
		return
	}
	cut, wasCut := cutRunes(label, maxAxisLabel)
	if wasCut {
		b.partial(path, "The axis label was cut to 100 characters.")
	}
	d.YAxisLabel = cut
}

func (b *panelBuild) heatmapRowsLayout(d *spec.Display) {
	if layout := b.enumOption("options.rowsFrame.layout", "auto", "le", "ge", "unknown"); layout != "auto" {
		d.RowsLayout = layout
	}
}

func (b *panelBuild) heatmapTooltip(d *spec.Display) {
	switch b.enumOption("options.tooltip.mode", "single", "multi", "none") {
	case "multi":
		d.Tooltip = "all"
	case "none":
		d.Tooltip = "hidden"
	}
	if flag, ok := b.boolOption("options.tooltip.yHistogram"); ok && *flag {
		d.TooltipHistogram = flag
	}
	if flag, ok := b.boolOption("options.tooltip.showColorScale"); ok && *flag {
		d.TooltipColorScale = flag
	}
	d.TooltipMaxWidth = b.tooltipSizeOption("options.tooltip.maxWidth")
	d.TooltipMaxHeight = b.tooltipSizeOption("options.tooltip.maxHeight")
}

func (b *panelBuild) heatmapColorValue(path string) string {
	v, _ := b.value(path)
	name := strings.TrimSpace(asString(v))
	if name == "" {
		return ""
	}
	color, ok := thresholdColor(name)
	if _, literal := literalColor(name); !ok || (!literal && strings.HasPrefix(color, "#")) {
		b.partial(path, fmt.Sprintf("The heatmap color %q is not supported yet.", name))
		return ""
	}
	b.consume(path)
	return color
}

func (b *panelBuild) heatmapUnit(target *string, v any, path string) {
	unit, isText := v.(string)
	if !isText || unit == "" {
		return
	}
	if imported, valid := importedUnit(unit); valid {
		*target = imported
	} else {
		b.partial(path, fmt.Sprintf("Unit %s is not supported yet.", unit))
	}
}

func (b *panelBuild) heatmapDecimals(path string) *int {
	v, ok := b.take(path)
	if !ok || v == nil {
		return nil
	}
	decimals, isNumber := numberValue(v)
	n := int(decimals)
	if !isNumber || float64(n) != decimals || n < 0 || n > maxDecimals {
		b.partial(path, "Decimals are kept as a whole number between 0 and 10.")
		return nil
	}
	return &n
}

func (b *panelBuild) heatmapWholeNumber(path string, low, high int) (int, bool) {
	v, ok := b.take(path)
	if !ok || v == nil {
		return 0, false
	}
	f, isNumber := numberValue(v)
	n := int(f)
	if !isNumber || float64(n) != f || n < low || n > high {
		b.partial(path, fmt.Sprintf("This value is kept as a whole number between %d and %d.", low, high))
		return 0, false
	}
	return n, true
}

func (b *panelBuild) heatmapColors(d *spec.Display) {
	mode := b.enumOption("options.color.mode", "scheme", "opacity")
	if mode == "" {
		mode = "scheme"
	}
	if mode == "scheme" {
		d.ColorMode = "scheme"
		b.heatmapScheme(d, "options.color.scheme", "", grafanaHeatmapScheme)
		b.consume("options.color.fill", "options.color.scale", "options.color.exponent")
	} else {
		b.heatmapFill(d, "options.color.fill")
		b.heatmapScaleAndExponent(d)
		b.consume("options.color.scheme")
	}
	d.ColorSteps = b.heatmapIntOption("options.color.steps", grafanaHeatmapSteps, minColorSteps, maxColorSteps)
	d.ColorMin = b.heatmapNumberOption("options.color.min")
	d.ColorMax = b.heatmapNumberOption("options.color.max")
	if reverse, ok := b.boolOption("options.color.reverse"); ok && *reverse {
		d.ColorReverse = reverse
	}
}

func (b *panelBuild) heatmapScheme(d *spec.Display, path, prefix, fallback string) {
	v, _ := b.take(path)
	name := strings.TrimPrefix(asString(v), prefix)
	if name == "" {
		name = fallback
	}
	scheme, ok := heatmapSchemes[name]
	switch {
	case !ok:
		b.partial(path, fmt.Sprintf("The heatmap color scheme %s is not supported yet.", name))
	case scheme != spec.Defaults().Display[spec.Heatmap].ColorScheme:
		d.ColorScheme = scheme
	}
}

func (b *panelBuild) heatmapFill(d *spec.Display, path string) {
	if v, _ := b.value(path); strings.TrimSpace(asString(v)) == "" {
		b.consume(path)
		d.ColorFill, _ = thresholdColor(grafanaHeatmapFill)
		return
	}
	d.ColorFill = b.heatmapColorValue(path)
}

func (b *panelBuild) heatmapScaleAndExponent(d *spec.Display) {
	scale := b.enumOption("options.color.scale", "linear", "exponential")
	if scale == "" {
		scale = "exponential"
	}
	if scale == "linear" {
		b.consume("options.color.exponent")
		return
	}
	d.ColorScale = scale
	b.heatmapExponent(d, "options.color.exponent")
}

func (b *panelBuild) heatmapExponent(d *spec.Display, path string) {
	v, ok := b.take(path)
	if !ok || v == nil {
		return
	}
	exponent, isNumber := numberValue(v)
	switch {
	case !isNumber || exponent < minColorExponent || exponent > maxColorExponent:
		b.partial(path, "The color exponent is kept between 0.1 and 2.")
	case exponent != grafanaHeatmapExponent:
		d.ColorExponent = &exponent
	}
}

func (b *panelBuild) heatmapCells(d *spec.Display) {
	d.CellGap = b.heatmapIntOption("options.cellGap", grafanaHeatmapGap, 0, maxCellGap)
	d.CellRadius = b.heatmapIntOption("options.cellRadius", 0, 0, maxCellRadius)
	if v, ok := b.take("options.cellValues.unit"); ok {
		b.heatmapUnit(&d.CellUnit, v, "options.cellValues.unit")
	}
	d.CellDecimals = b.heatmapDecimals("options.cellValues.decimals")
}

func (b *panelBuild) heatmapValueFilters(d *spec.Display) {
	if low := b.heatmapNumberOption("options.filterValues.le"); low != nil && *low != grafanaHeatmapFilterLow && *low != 0 {
		d.HideBelow = low
	}
	d.HideAbove = b.heatmapNumberOption("options.filterValues.ge")
}

func (b *panelBuild) heatmapShowValues(d *spec.Display) {
	shown := b.enumOption("options.showValue", "auto", "always", "never")
	if _, written := b.value("options.showValue"); shown == "" && !written {
		shown = "auto"
	}
	if shown != "never" {
		d.ShowValues = shown
	}
}

func (b *panelBuild) heatmapNumberOption(path string) *float64 {
	v, ok := b.take(path)
	if !ok || v == nil {
		return nil
	}
	f, isNumber := numberValue(v)
	if !isNumber {
		b.partial(path, "This value is not a number.")
		return nil
	}
	return &f
}

func (b *panelBuild) heatmapIntOption(path string, fallback, low, high int) int {
	v, ok := b.take(path)
	if !ok || v == nil {
		return fallback
	}
	f, isNumber := numberValue(v)
	n := int(f)
	if !isNumber || float64(n) != f || n < low || n > high {
		b.partial(path, fmt.Sprintf("This value is kept as a whole number between %d and %d.", low, high))
		return 0
	}
	return n
}

func (b *panelBuild) heatmapScale(d *spec.Display) {
	const typePath = "fieldConfig.defaults.custom.scaleDistribution.type"
	v, _ := b.value(typePath)
	scale := asString(v)
	if scale == "" {
		scale = asString(lookupValue(b.raw, "options.yAxis.scaleType"))
		b.consume("options.yAxis.scaleType")
	}
	if scale != "log" && scale != "symlog" {
		return
	}
	d.YScale = scale
	b.consume(typePath)
	if base, ok := b.heatmapLogBase("fieldConfig.defaults.custom.scaleDistribution.log"); ok {
		d.YBucketLogBase = base
	}
}

func displayHistogram(b *panelBuild) {
	d := b.disp()
	if d.Stack == "" {
		d.Stack = "none"
	}
	if flag, ok := b.boolOption("options.combine"); ok && *flag {
		d.Combine = flag
	}
	if v, ok := b.take("options.bucketOffset"); ok {
		if offset, isNumber := numberValue(v); isNumber && offset != 0 {
			d.BucketOffset = &offset
		}
	}
	b.seriesLegendAndTooltip(d)
	if d.Tooltip == "" {
		d.Tooltip = "single"
	}
	if d.TooltipHideZeros == nil {
		d.TooltipHideZeros = ptr(false)
	}
	b.histogramBuckets(d)
}

func (b *panelBuild) histogramBuckets(d *spec.Display) {
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
	d.RowHeight = b.shareOption("options.rowHeight")
	d.AlignValue = b.enumOption("options.alignValue", "left", "center", "right")
	b.timelineLegendAndTooltip(d)
}

func (b *panelBuild) timelineLegendAndTooltip(d *spec.Display) {
	b.legendAndTooltip(d)
	b.tooltipOptions(d)
	if _, ok := b.value("options.legend"); !ok {
		return
	}
	table := b.enumOption("options.legend.displayMode", "table", "list") == "table"
	if v, ok := b.boolOption("options.legend.asTable"); ok && *v {
		table = true
	}
	if table {
		d.LegendMode = "table"
	}
	b.legendWidth(d, "options.legend.width")
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
	b.logsLines(d)
}

func (b *panelBuild) logsLines(d *spec.Display) {
	d.ShowLabels, _ = b.boolOption("options.showLabels")
	d.ShowCommonLabels, _ = b.boolOption("options.showCommonLabels")
	d.Details, _ = b.boolOption("options.enableLogDetails")
	d.Prettify, _ = b.boolOption("options.prettifyLogMessage")
	d.Controls, _ = b.boolOption("options.showControls")
	d.Highlight, _ = b.boolOption("options.syntaxHighlighting")
	d.Dedup = b.enumOption("options.dedupStrategy", "none", "exact", "numbers", "signature")
	d.FontSize = b.enumOption("options.fontSize", "default", "small")
	d.DetailsMode = b.enumOption("options.detailsMode", "inline", "sidebar")
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

func (b *panelBuild) convertPanelChrome() {
	b.convertTransparent()
	b.convertHideTimeOverride()
}

func (b *panelBuild) convertTransparent() {
	if !truthy(b.raw["transparent"]) {
		b.consume("transparent")
		return
	}
	if b.typ != spec.Text && !b.typ.IsData() {
		return
	}
	b.panel.Transparent = true
	b.consume("transparent")
}

func (b *panelBuild) convertHideTimeOverride() {
	if truthy(b.raw["hideTimeOverride"]) && (b.panel.TimeFrom != "" || b.panel.TimeShift != "") {
		b.partial("hideTimeOverride", hideTimeOverrideReason)
		return
	}
	b.consume("hideTimeOverride")
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
