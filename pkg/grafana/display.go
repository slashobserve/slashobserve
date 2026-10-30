package grafana

import (
	"fmt"
	"math"
	"regexp"
	"slices"
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
