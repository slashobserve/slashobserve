package grafana

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	datagridReason      = "Editing cells is not supported; the grid is shown as a table."
	shareReason         = "Only row heights and column widths from 0.1 to 1 are supported."
	xyMatcherReason     = "Only fields chosen by name, and Y fields chosen by name, names or a regular expression, are supported."
	xyExcludeReason     = "The excluded fields must be a list of names."
	xySeriesCountReason = "At most 20 series are supported."
	pointRangeReason    = "The smallest point size must not exceed the largest."
	widthReason         = "Only widths above 0 and up to 1 are supported."
	pointSizeReason     = "Only point sizes from 1 to 40 are supported."
	fieldNameLenReason  = "A field name must be between 1 and 200 characters."
	nodeUnitsDiffReason = "The node and edge units differ, so neither is applied."
	nodeArcReason       = "This arc needs a field and a color SlashObserve supports."
	spanSearchReason    = "The span search must be between 1 and 200 characters."
	spanNamesReason     = "Span filters take up to 50 distinct names of 1 to 200 characters."
	spanDurationReason  = "A span duration filter is a number with the unit ns, us, ms, s, m or h, up to 24 hours."
	minShare            = 0.1
	maxFieldName        = 200
	maxSpanFilterNames  = 50
	maxSpanSearch       = 200
	maxSpanFilterMillis = 86400000
	maxPointSize        = 40
	maxXYSeries         = 20
	maxXYMatcher        = 500
	maxXYPointSize      = 100
	minTickRotation     = -90
	maxTickRotation     = 90
	maxTickSpacing      = 500
	maxTickLength       = 200
	minBarValueSize     = 6
	maxBarValueSize     = 100
	maxBarRadius        = 0.5
)

func (b *panelBuild) boundedIntOption(path string, lo, hi int) int {
	v, ok := b.value(path)
	if !ok {
		return 0
	}
	n, isInt := asInt(v)
	if !isInt || n < lo || n > hi {
		b.partial(path, fmt.Sprintf("The value must be a whole number from %d to %d.", lo, hi))
		return 0
	}
	b.consume(path)
	return n
}

func (b *panelBuild) radiusOption(path string) float64 {
	v, ok := b.value(path)
	if !ok {
		return 0
	}
	f, isNumber := numberValue(v)
	if !isNumber || f < 0 || f > maxBarRadius {
		b.partial(path, "Only bar radii from 0 to 0.5 are supported.")
		return 0
	}
	b.consume(path)
	return f
}

func (b *panelBuild) fieldNameOption(path string) string {
	v, ok := b.take(path)
	name, isText := v.(string)
	switch {
	case !ok || !isText || name == "":
		return ""
	case !validLength(name, maxFieldName):
		b.partial(path, fieldNameLenReason)
		return ""
	}
	return name
}

func (b *panelBuild) fractionOption(path string) *float64 {
	v, ok := b.take(path)
	if !ok {
		return nil
	}
	f, isNumber := numberValue(v)
	if !isNumber || f <= 0 || f > 1 {
		b.partial(path, widthReason)
		return nil
	}
	return &f
}

func (b *panelBuild) shareOption(path string) *float64 {
	v, ok := b.take(path)
	if !ok {
		return nil
	}
	f, isNumber := numberValue(v)
	if !isNumber || f < minShare || f > 1 {
		b.partial(path, shareReason)
		return nil
	}
	return &f
}

func (b *panelBuild) datagridOptions() {
	options, _ := asObject(lookupValue(b.raw, "options"))
	for _, key := range sortedKeys(options) {
		path := "options." + key
		if key == "selectedSeries" || (present(options[key]) && !isGrafanaDefault(b.typ, path, options[key])) {
			b.partial(path, datagridReason)
		}
	}
}

func displayBarChart(b *panelBuild) {
	d := b.disp()
	d.Orientation = b.orientation()
	d.XField = b.fieldNameOption("options.xField")
	d.ShowValues = b.enumOption("options.showValue", "auto", "always", "never")
	d.BarWidth = b.fractionOption("options.barWidth")
	d.GroupWidth = b.fractionOption("options.groupWidth")
	d.XTickRotation = b.boundedIntOption("options.xTickLabelRotation", minTickRotation, maxTickRotation)
	d.XTickSpacing = b.boundedIntOption("options.xTickLabelSpacing", 0, maxTickSpacing)
	d.XTickMaxLength = b.boundedIntOption("options.xTickLabelMaxLength", 0, maxTickLength)
	d.BarRadius = b.radiusOption("options.barRadius")
	if flag, ok := b.boolOption("options.fullHighlight"); ok && *flag {
		d.FullHighlight = flag
	}
	d.ColorByField = b.fieldNameOption("options.colorByField")
	d.ValueSize = b.boundedIntOption("options.text.valueSize", minBarValueSize, maxBarValueSize)
	b.seriesLegendAndTooltip(d)
	if mode := b.enumOption("options.stacking", "none", "normal", "percent"); mode != "" {
		b.panel.Stack = mode
	}
}

func displayStatusHistory(b *panelBuild) {
	d := b.disp()
	d.ShowValues = b.enumOption("options.showValue", "auto", "always", "never")
	d.RowHeight = b.shareOption("options.rowHeight")
	d.ColumnWidth = b.shareOption("options.colWidth")
	b.timelineLegendAndTooltip(d)
}

func displayTrend(b *panelBuild) {
	displayTimeseries(b)
	b.disp().XField = b.fieldNameOption("options.xField")
}

func displayXYChart(b *panelBuild) {
	d := b.disp()
	d.XField = b.fieldNameOption("options.dims.x")
	b.xySeriesMapping(d)
	d.Show = b.enumOption("fieldConfig.defaults.custom.show", "points", "lines", "points+lines")
	b.pointSizeOption(d)
	b.pointSizeRange(d)
	b.seriesLegendAndTooltip(d)
}

func (b *panelBuild) xySeriesMapping(d *spec.Display) {
	mode := b.enumOption("options.mapping", "auto", "manual")
	if mode == "" {
		mode = b.enumOption("options.seriesMapping", "auto", "manual")
	}
	if mode == "manual" {
		d.Series = b.xyManualSeries()
		return
	}
	if exclude := b.xyExcludedFields(); exclude != "" {
		d.Series = []spec.XYSeries{{X: d.XField, Y: exclude}}
	}
}

func (b *panelBuild) xyExcludedFields() string {
	const path = "options.dims.exclude"
	v, ok := b.value(path)
	if !ok {
		return ""
	}
	pattern, valid := namesMatch(map[string]any{"names": v, "mode": "exclude"})
	if !valid || len(pattern.SeriesRegex) > maxXYMatcher {
		b.partial(path, xyExcludeReason)
		return ""
	}
	b.consume(path)
	return pattern.SeriesRegex
}

func (b *panelBuild) xyManualSeries() []spec.XYSeries {
	items, _ := asArray(lookupValue(b.raw, "options.series"))
	if len(items) == 0 {
		return []spec.XYSeries{{}}
	}
	if len(items) > maxXYSeries {
		b.partial("options.series", xySeriesCountReason)
		items = items[:maxXYSeries]
	}
	series := make([]spec.XYSeries, len(items))
	for i, item := range items {
		entry, _ := asObject(item)
		series[i] = b.xySeriesEntry(fmt.Sprintf("options.series[%d]", i), entry)
	}
	return series
}

func (b *panelBuild) xySeriesEntry(path string, entry map[string]any) spec.XYSeries {
	var series spec.XYSeries
	series.X = b.xyFieldMatcher(path, "x", entry)
	series.Size = b.xyFieldMatcher(path, "size", entry)
	series.Color = b.xyFieldMatcher(path, "color", entry)
	series.Y = b.xyYMatcher(path+".y", entry["y"])
	series.Query, series.Frame = b.xyFrameMatcher(path+".frame", entry["frame"])
	series.Name = b.xyName(path+".name", entry["name"])
	b.xyLegacyPointFields(path, entry, &series)
	return series
}

func (b *panelBuild) xyFieldMatcher(path, key string, entry map[string]any) string {
	v, ok := entry[key]
	if !ok {
		return ""
	}
	site := path + "." + key
	if name, isText := v.(string); isText {
		return b.xyFieldName(site, site, name)
	}
	id, options := matcherOf(v)
	name, isText := options.(string)
	if id != "byName" || !isText {
		b.partial(site, xyMatcherReason)
		return ""
	}
	return b.xyFieldName(site, site+".matcher", name)
}

func (b *panelBuild) xyFieldName(site, consumed, name string) string {
	if !validLength(name, maxFieldName) {
		b.partial(site, fieldNameLenReason)
		return ""
	}
	b.consume(consumed)
	return name
}

func matcherOf(v any) (string, any) {
	side, _ := asObject(v)
	matcher, _ := asObject(side["matcher"])
	return asString(matcher["id"]), matcher["options"]
}

func (b *panelBuild) xyYMatcher(path string, v any) string {
	if v == nil {
		return ""
	}
	if name, isText := v.(string); isText {
		return b.xyPattern(path, path, name)
	}
	id, options := matcherOf(v)
	site := path + ".matcher"
	switch id {
	case "byName":
		if name, isText := options.(string); isText && !strings.HasPrefix(name, "/") {
			return b.xyPattern(path, site, name)
		}
	case "byRegexp":
		if pattern, isText := options.(string); isText {
			return b.xyPattern(path, site, "/"+pattern+"/")
		}
	case "byNames":
		if names, isList := asArray(options); isList {
			options = map[string]any{"names": names}
		}
		if pattern, valid := namesMatch(options); valid {
			return b.xyPattern(path, site, slashed(pattern.SeriesRegex))
		}
	}
	b.partial(path, xyMatcherReason)
	return ""
}

func slashed(pattern string) string {
	if strings.HasPrefix(pattern, "/") {
		return pattern
	}
	return "/" + pattern + "/"
}

func (b *panelBuild) xyPattern(path, consumed, pattern string) string {
	if !validLength(pattern, maxXYMatcher) {
		b.partial(path, xyMatcherReason)
		return ""
	}
	b.consume(consumed)
	return pattern
}

func (b *panelBuild) xyFrameMatcher(path string, v any) (query, frame string) {
	if v == nil {
		return "", ""
	}
	id, options := matcherOf(v)
	text, isText := options.(string)
	switch {
	case id == "byRefId" && isText:
		if ref, known := b.refs[text]; known {
			b.consume(path + ".matcher")
			return ref, ""
		}
	case id == "byName" && isText && !strings.HasPrefix(text, "/"):
		return "", b.xyPattern(path, path+".matcher", text)
	case id == "byRegexp" && isText:
		return "", b.xyPattern(path, path+".matcher", "/"+text+"/")
	}
	b.partial(path, xyMatcherReason)
	return "", ""
}

func (b *panelBuild) xyName(path string, v any) string {
	if v == nil {
		return ""
	}
	if name, isText := v.(string); isText {
		return b.xyDisplayName(path, name)
	}
	cfg, _ := asObject(v)
	name, isText := cfg["fixed"].(string)
	if !isText {
		return ""
	}
	return b.xyDisplayName(path+".fixed", name)
}

func (b *panelBuild) xyDisplayName(path, name string) string {
	if !validLength(name, maxFieldName) {
		b.partial(path, fieldNameLenReason)
		return ""
	}
	b.consume(path)
	return name
}

func (b *panelBuild) xyLegacyPointFields(path string, entry map[string]any, series *spec.XYSeries) {
	for _, legacy := range []struct {
		key    string
		target *string
	}{{"pointSize", &series.Size}, {"pointColor", &series.Color}} {
		cfg, ok := asObject(entry[legacy.key])
		if !ok {
			continue
		}
		field, isText := cfg["field"].(string)
		if !isText || *legacy.target != "" {
			continue
		}
		site := path + "." + legacy.key + ".field"
		*legacy.target = b.xyFieldName(site, site, field)
	}
}

func (b *panelBuild) pointSizeRange(d *spec.Display) {
	d.PointSizeMin = b.boundedIntOption("fieldConfig.defaults.custom.pointSize.min", 1, maxXYPointSize)
	d.PointSizeMax = b.boundedIntOption("fieldConfig.defaults.custom.pointSize.max", 1, maxXYPointSize)
	if d.PointSizeMin != 0 && d.PointSizeMax != 0 && d.PointSizeMin > d.PointSizeMax {
		b.partial("fieldConfig.defaults.custom.pointSize.min", pointRangeReason)
		b.partial("fieldConfig.defaults.custom.pointSize.max", pointRangeReason)
		d.PointSizeMin, d.PointSizeMax = 0, 0
	}
}

func (b *panelBuild) pointSizeOption(d *spec.Display) {
	path := "fieldConfig.defaults.custom.pointSize.fixed"
	v, ok := b.take(path)
	if !ok {
		return
	}
	f, isNumber := numberValue(v)
	if !isNumber || f != math.Trunc(f) || f < 1 || f > maxPointSize {
		b.partial(path, pointSizeReason)
		return
	}
	d.PointSize = int(f)
}

func displayCandlestick(b *panelBuild) {
	d := b.disp()
	d.Mode = b.enumOption("options.mode", "candles+volume", "candles", "volume")
	d.CandleStyle = b.enumOption("options.candleStyle", "candles", "ohlcbars")
	d.ColorStrategy = b.enumOption("options.colorStrategy", "open-close", "close-close")
	d.UpColor = b.candleColor("options.colors.up")
	d.DownColor = b.candleColor("options.colors.down")
	d.Open = b.fieldNameOption("options.fields.open")
	d.High = b.fieldNameOption("options.fields.high")
	d.Low = b.fieldNameOption("options.fields.low")
	d.Close = b.fieldNameOption("options.fields.close")
	d.Volume = b.fieldNameOption("options.fields.volume")
	if v, ok := b.boolOption("options.includeAllFields"); ok && *v {
		d.IncludeAllFields = v
	}
	b.legendAndTooltip(d)
	b.tooltipOptions(d)
}

func (b *panelBuild) candleColor(path string) string {
	v, ok := b.take(path)
	name := strings.TrimSpace(asString(v))
	if !ok || name == "" {
		return ""
	}
	color, supported := thresholdColor(name)
	if !supported {
		b.partial(path, fmt.Sprintf("The candle color %q is not supported yet.", name))
		return ""
	}
	return color
}

func displayNodeGraph(b *panelBuild) {
	d := b.disp()
	d.Layout = b.enumOption("options.layoutAlgorithm", "layered", "force", "grid")
	if b.enumOption("options.zoomMode", "cooperative", "greedy") == "greedy" {
		d.WheelZoom = ptr(true)
	}
	b.statUnitOverride("mainStatUnit", "mainstat")
	b.statUnitOverride("secondaryStatUnit", "secondarystat")
	b.arcOverrides()
}

func (b *panelBuild) statUnitOverride(key, field string) {
	nodePath, edgePath := "options.nodes."+key, "options.edges."+key
	node := strings.TrimSpace(asString(lookupValue(b.raw, nodePath)))
	edge := strings.TrimSpace(asString(lookupValue(b.raw, edgePath)))
	path, unit := nodePath, node
	if node == "" {
		path, unit = edgePath, edge
	}
	switch {
	case unit == "":
		return
	case edge != "" && node != "" && edge != node:
		b.partial(nodePath, nodeUnitsDiffReason)
		b.partial(edgePath, nodeUnitsDiffReason)
		return
	case !validUnit(unit):
		b.partial(path, fmt.Sprintf("Unit %s is not supported yet.", unit))
		return
	}
	b.consume(nodePath, edgePath)
	b.addOverride(spec.Override{Match: spec.Match{Field: field}, FieldProps: spec.FieldProps{Unit: unit}}, path)
}

func (b *panelBuild) arcOverrides() {
	items, _ := asArray(lookupValue(b.raw, "options.nodes.arcs"))
	for i, item := range items {
		path := fmt.Sprintf("options.nodes.arcs[%d]", i)
		arc, _ := asObject(item)
		field := asString(arc["field"])
		color, ok := seriesColor(asString(arc["color"]))
		if !validLength(field, maxFieldName) || !ok {
			b.partial(path, nodeArcReason)
			continue
		}
		b.consume(path)
		b.addOverride(spec.Override{Match: spec.Match{Field: field}, FieldProps: spec.FieldProps{Color: color}}, path)
	}
}

var spanDurationPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(ns|us|µs|μs|ms|s|m|h)$`)

var spanDurationMillis = map[string]float64{"ns": 1e-6, "us": 1e-3, "µs": 1e-3, "μs": 1e-3, "ms": 1, "s": 1000, "m": 60000, "h": 3600000}

func displayTraces(b *panelBuild) {
	d := b.disp()
	const base = "options.spanFilters"
	d.SpanSearch = b.spanSearch(base + ".query")
	d.ServiceNames, d.ExcludeServices = b.spanNameFilter(base+".serviceName", base+".serviceNameOperator")
	d.SpanNames, d.ExcludeSpanNames = b.spanNameFilter(base+".spanName", base+".spanNameOperator")
	d.MinSpanDuration = b.spanDuration(base + ".from")
	d.MaxSpanDuration = b.spanDuration(base + ".to")
	d.MatchesOnly = b.setFlag(base + ".matchesOnly")
	d.CriticalPathOnly = b.setFlag(base + ".criticalPathOnly")
}

func (b *panelBuild) setFlag(path string) *bool {
	if flag, ok := b.boolOption(path); ok && *flag {
		return flag
	}
	return nil
}

func (b *panelBuild) spanSearch(path string) string {
	v, ok := b.value(path)
	text, isText := v.(string)
	if !ok || !isText {
		return ""
	}
	text = strings.TrimSpace(text)
	if text != "" && !validLength(text, maxSpanSearch) {
		b.partial(path, spanSearchReason)
		return ""
	}
	b.consume(path)
	return text
}

func (b *panelBuild) spanNameFilter(path, operatorPath string) ([]string, *bool) {
	names := b.spanNames(path)
	if b.enumOption(operatorPath, "=", "!=") == "!=" {
		return names, ptr(true)
	}
	return names, nil
}

func (b *panelBuild) spanNames(path string) []string {
	v, ok := b.value(path)
	if !ok {
		return nil
	}
	items, isList := asArray(singleToList(v))
	if !isList {
		return nil
	}
	var names []string
	for _, item := range items {
		name, isText := item.(string)
		if !isText || !validLength(name, maxFieldName) {
			b.partial(path, spanNamesReason)
			return nil
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	if len(names) > maxSpanFilterNames {
		b.partial(path, spanNamesReason)
		return nil
	}
	b.consume(path)
	return names
}

func (b *panelBuild) spanDuration(path string) *float64 {
	v, ok := b.value(path)
	text, isText := v.(string)
	if !ok || !isText {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		b.consume(path)
		return nil
	}
	m := spanDurationPattern.FindStringSubmatch(text)
	if m == nil {
		b.partial(path, spanDurationReason)
		return nil
	}
	n, err := strconv.ParseFloat(m[1], 64)
	millis := math.Round(n*spanDurationMillis[m[2]]*1e6) / 1e6
	if err != nil || millis > maxSpanFilterMillis {
		b.partial(path, spanDurationReason)
		return nil
	}
	b.consume(path)
	return &millis
}
