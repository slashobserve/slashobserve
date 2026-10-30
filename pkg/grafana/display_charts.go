package grafana

import (
	"fmt"
	"math"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	datagridReason      = "Editing cells is not supported; the grid is shown as a table."
	shareReason         = "Only row heights and column widths from 0.1 to 1 are supported."
	forceLayoutReason   = "The force layout is drawn layered."
	xyMappingReason     = "Only a manual mapping with one series and an X field chosen by name is supported; the fields are picked automatically."
	xyMatcherReason     = "Choosing the Y, color and size fields is not supported; they are picked automatically."
	widthReason         = "Only widths above 0 and up to 1 are supported."
	pointSizeReason     = "Only point sizes from 1 to 40 are supported."
	fieldNameLenReason  = "A field name must be between 1 and 200 characters."
	nodeUnitsDiffReason = "The node and edge units differ; one unit is kept for each statistic."
	nodeArcReason       = "This arc needs a field and a color SlashObserve supports."
	minShare            = 0.1
	maxFieldName        = 200
	maxPointSize        = 40
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
	if d.XField == "" {
		d.XField = b.manualXField()
	}
	d.Show = b.enumOption("fieldConfig.defaults.custom.show", "points", "lines", "points+lines")
	b.pointSizeOption(d)
	b.legendAndTooltip(d)
}

func (b *panelBuild) manualXField() string {
	if asString(lookupValue(b.raw, "options.mapping")) != "manual" {
		return ""
	}
	series, _ := asArray(lookupValue(b.raw, "options.series"))
	if len(series) != 1 {
		b.partial("options.mapping", xyMappingReason)
		b.partial("options.series", xyMappingReason)
		return ""
	}
	only, _ := asObject(series[0])
	field := matcherByName(only["x"])
	if field == "" || !validLength(field, maxFieldName) {
		b.partial("options.mapping", xyMappingReason)
		b.partial("options.series", xyMappingReason)
		return ""
	}
	b.consume("options.mapping", "options.series[0].x")
	for _, key := range []string{"y", "color", "size"} {
		if present(only[key]) {
			b.partial("options.series[0]."+key, xyMatcherReason)
		}
	}
	return field
}

func matcherByName(v any) string {
	side, _ := asObject(v)
	matcher, _ := asObject(side["matcher"])
	if asString(matcher["id"]) != "byName" {
		return ""
	}
	return asString(matcher["options"])
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
	switch layout := b.enumOption("options.layoutAlgorithm", "layered", "force", "grid"); layout {
	case "force":
		d.Layout = "layered"
		b.partial("options.layoutAlgorithm", forceLayoutReason)
	default:
		d.Layout = layout
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
