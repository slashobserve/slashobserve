package grafana

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const graphModeReason = "Graph panels in series or histogram mode are not supported yet."

var (
	regexAlias  = regexp.MustCompile(`^/.*/[gimsuy]*$`)
	legendCalcs = []struct{ key, reducer string }{
		{"avg", "mean"}, {"current", "lastNotNull"}, {"max", "max"}, {"min", "min"}, {"total", "sum"},
	}
	singlestatReducers = map[string]string{
		"avg": "mean", "current": "lastNotNull", "max": "max", "min": "min", "total": "sum",
		"first": "first", "delta": "delta", "diff": "diff", "range": "range",
	}
	thresholdColorModes = map[string]string{"critical": "red", "warning": "orange", "ok": "green"}
)

func legacyPlaceholderReason(n *panelNode) string {
	if n.sourceType != "graph" {
		return ""
	}
	if xaxis, ok := asObject(n.raw["xaxis"]); ok {
		if mode := asString(xaxis["mode"]); mode != "" && mode != "time" {
			return graphModeReason
		}
	}
	return ""
}

func (b *panelBuild) convertLegacy() {
	switch b.n.sourceType {
	case "graph":
		b.legacyGraph()
	case "singlestat", "grafana-singlestat-panel":
		b.legacySinglestat()
	case "table-old":
		b.legacyTable()
	}
}

func flagOr(m map[string]any, key string, fallback bool) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return fallback
	}
	return truthy(v)
}

func (b *panelBuild) legacyUnit(p *spec.FieldProps, v any, path string) {
	unit, _ := v.(string)
	if unit == "" {
		return
	}
	imported, ok := importedUnit(unit)
	if !ok {
		b.partial(path, fmt.Sprintf("Unit %s is not supported yet.", unit))
		return
	}
	p.Unit = imported
}

func (b *panelBuild) legacyDecimals(p *spec.FieldProps, v any) {
	if f, ok := numberValue(v); ok && f == math.Trunc(f) && f >= 0 && f <= maxDecimals {
		n := int(f)
		p.Decimals = &n
	}
}

func (b *panelBuild) legacyGraph() {
	p := &b.panel.FieldProps
	axes := b.graphAxes(p)
	b.graphDrawing(p)
	b.graphLegendAndTooltip()
	b.graphThresholds(p)
	b.graphOverrides(axes)
	b.graphExtras()
}

type graphAxis struct {
	props  spec.FieldProps
	hidden bool
}

func (b *panelBuild) graphAxis(v any, path string) graphAxis {
	y, _ := asObject(v)
	var axis graphAxis
	b.legacyUnit(&axis.props, y["format"], path+".format")
	b.legacyDecimals(&axis.props, y["decimals"])
	if f, ok := numberValue(y["min"]); ok {
		axis.props.Min = &f
	}
	if f, ok := numberValue(y["max"]); ok {
		axis.props.Max = &f
	}
	b.fixMinMax(&axis.props, path+".max")
	if f, ok := numberValue(y["logBase"]); ok && f != 1 {
		b.partial(path+".logBase", "Logarithmic axes are not supported yet.")
	}
	if label := asString(y["label"]); label != "" {
		b.partial(path+".label", "Axis labels are not supported yet.")
	}
	axis.hidden = y["show"] == false
	return axis
}

func (b *panelBuild) graphAxes(p *spec.FieldProps) [2]graphAxis {
	items, _ := asArray(b.raw["yaxes"])
	b.consume("yaxes")
	var axes [2]graphAxis
	if len(items) > 0 {
		axes[0] = b.graphAxis(items[0], "yaxes[0]")
	}
	if len(items) > 1 && b.usesSecondAxis() {
		axes[1] = b.graphAxis(items[1], "yaxes[1]")
	}
	first := axes[0].props
	p.Unit = cmp.Or(first.Unit, p.Unit)
	if first.Decimals != nil {
		p.Decimals = first.Decimals
	} else if p.Decimals == nil {
		b.legacyDecimals(p, b.raw["decimals"])
	}
	b.consume("decimals")
	if first.Min != nil {
		p.Min = first.Min
	}
	if first.Max != nil {
		p.Max = first.Max
	}
	b.fixMinMax(p, "yaxes[0].max")
	if axes[0].hidden {
		p.Axis = "hidden"
	}
	return axes
}

func (b *panelBuild) usesSecondAxis() bool {
	items, _ := asArray(b.raw["seriesOverrides"])
	for _, item := range items {
		if o, ok := asObject(item); ok {
			if n, isInt := asInt(o["yaxis"]); isInt && n == 2 {
				return true
			}
		}
	}
	return false
}

func (b *panelBuild) graphDrawing(p *spec.FieldProps) {
	bars := flagOr(b.raw, "bars", false)
	lines := flagOr(b.raw, "lines", true)
	switch {
	case bars:
		p.Style = "bars"
	case !lines:
		p.Style = "points"
		b.consume("points")
	}
	b.consume("bars", "lines")
	b.graphFill(p, bars)
	if flagOr(b.raw, "dashes", false) {
		p.LineStyle = "dashed"
	}
	b.consume("dashes")
	b.graphStack(p)
	if !flagOr(b.raw, "points", false) {
		b.consume("pointradius")
	}
}

func (b *panelBuild) graphFill(p *spec.FieldProps, bars bool) {
	fill := func(value float64) {
		n := int(math.Round(min(max(value*10, 0), 100)))
		p.Fill = &n
	}
	if f, ok := numberValue(b.raw["fill"]); ok {
		fill(f)
	}
	if bars {
		full := 100
		p.Fill = &full
	}
	b.consume("fill")
}

func (b *panelBuild) graphStack(p *spec.FieldProps) {
	if !flagOr(b.raw, "stack", false) {
		return
	}
	p.Stack = "normal"
	if flagOr(b.raw, "percentage", false) {
		p.Stack = "percent"
		b.consume("percentage")
	}
	b.consume("stack")
}

func (b *panelBuild) graphLegendAndTooltip() {
	legend, _ := asObject(b.raw["legend"])
	d := b.disp()
	switch {
	case !flagOr(legend, "show", true):
		d.Legend = "hidden"
	case truthy(legend["rightSide"]):
		d.Legend = "right"
	}
	if truthy(legend["values"]) {
		for _, calc := range legendCalcs {
			if truthy(legend[calc.key]) {
				d.LegendValues = append(d.LegendValues, calc.reducer)
			}
		}
	}
	b.consume("legend.show", "legend.rightSide", "legend.values")
	for _, calc := range legendCalcs {
		b.consume("legend." + calc.key)
	}
	b.graphLegendTable(d, legend)
	tooltip, _ := asObject(b.raw["tooltip"])
	if flagOr(tooltip, "shared", true) {
		d.Tooltip = "all"
	}
	b.consume("tooltip.shared")
}

func (b *panelBuild) graphThresholds(p *spec.FieldProps) {
	items, _ := asArray(b.raw["thresholds"])
	b.consume("thresholds")
	steps := []thresholdStep{{index: -1, color: "transparent"}}
	for i, item := range items {
		t, _ := asObject(item)
		path := fmt.Sprintf("thresholds[%d]", i)
		value, ok := numberValue(t["value"])
		if asString(t["op"]) != "gt" || !ok {
			b.partial(path, "Thresholds below a value are not supported yet.")
			continue
		}
		steps = append(steps, thresholdStep{index: i, value: &value, color: legacyThresholdColor(t)})
	}
	if len(steps) == 1 {
		return
	}
	p.Thresholds = b.finishThresholds(steps, func(i int) string { return fmt.Sprintf("thresholds[%d]", i) })
}

func legacyThresholdColor(t map[string]any) string {
	mode := asString(t["colorMode"])
	if color, ok := thresholdColorModes[mode]; ok {
		return color
	}
	if mode == "custom" {
		return cmp.Or(asString(t["fillColor"]), asString(t["lineColor"]))
	}
	return "red"
}

func (b *panelBuild) graphExtras() {
	if present(b.raw["alert"]) {
		b.partial("alert", "Alerts are not supported yet.")
	}
	if present(b.raw["timeRegions"]) {
		b.partial("timeRegions", "Time regions are not supported yet.")
	}
}

func (b *panelBuild) graphOverrides(axes [2]graphAxis) {
	colors, _ := asObject(b.raw["aliasColors"])
	b.consume("aliasColors")
	for _, alias := range sortedKeys(colors) {
		raw := asString(colors[alias])
		if raw == "" {
			continue
		}
		color, ok := seriesColor(raw)
		if !ok {
			b.partial("aliasColors."+alias, fmt.Sprintf("The color %q is not supported yet.", raw))
			continue
		}
		b.legacyOverride(alias, spec.FieldProps{Color: color}, "aliasColors."+alias)
	}
	items, _ := asArray(b.raw["seriesOverrides"])
	b.consume("seriesOverrides")
	for i, item := range items {
		o, ok := asObject(item)
		if !ok || asString(o["alias"]) == "" {
			continue
		}
		path := fmt.Sprintf("seriesOverrides[%d]", i)
		b.legacyOverride(asString(o["alias"]), b.seriesOverrideProps(o, axes, path), path)
	}
}

func (b *panelBuild) legacyOverride(alias string, props spec.FieldProps, path string) {
	var match spec.Match
	switch {
	case regexAlias.MatchString(alias) && validLength(alias, maxRegexMatch):
		match.SeriesRegex = alias
	case !regexAlias.MatchString(alias) && validLength(alias, maxNameMatch):
		match.Series = alias
	default:
		b.partial(path, "The series name is too long to be matched.")
		return
	}
	if fieldPropsEmpty(props) {
		return
	}
	b.addOverride(spec.Override{Match: match, FieldProps: props}, path)
}

func (b *panelBuild) seriesOverrideProps(o map[string]any, axes [2]graphAxis, path string) spec.FieldProps {
	var props spec.FieldProps
	for _, key := range sortedKeys(o) {
		if key == "alias" || key == "$$hashKey" || o[key] == nil {
			continue
		}
		if !b.seriesOverrideKey(&props, key, o, axes) {
			b.partial(path+"."+key, fmt.Sprintf("The series override option %s is not supported yet.", key))
		}
	}
	b.overrideStyle(&props, o)
	return props
}

func (b *panelBuild) overrideStyle(props *spec.FieldProps, o map[string]any) {
	switch {
	case truthy(o["bars"]):
		props.Style = "bars"
		full := 100
		props.Fill = &full
	case o["lines"] == false:
		props.Style = "points"
	case o["bars"] == false || o["lines"] == true:
		props.Style = "line"
	}
}

func (b *panelBuild) seriesOverrideKey(props *spec.FieldProps, key string, o map[string]any, axes [2]graphAxis) bool {
	v := o[key]
	switch key {
	case "color":
		color, ok := seriesColor(asString(v))
		props.Color = color
		return ok
	case "fill":
		f, ok := numberValue(v)
		if ok {
			fill := int(math.Round(min(max(f*10, 0), 100)))
			props.Fill = &fill
		}
		return ok
	case "bars", "lines", "dashLength", "spaceLength":
		return true
	case "points":
		return o["lines"] == false
	case "dashes":
		props.LineStyle = lineStyleFor(truthy(v))
		return true
	case "stack":
		flag, isBool := v.(bool)
		if isBool {
			props.Stack = stackFor(flag)
		}
		return isBool
	case "yaxis":
		return secondAxis(props, v, axes)
	case "hiddenSeries":
		hidden := truthy(v)
		props.Hidden = &hidden
		return true
	}
	return false
}

func lineStyleFor(dashed bool) string {
	if dashed {
		return "dashed"
	}
	return "solid"
}

func stackFor(stacked bool) string {
	if stacked {
		return "normal"
	}
	return "none"
}

func secondAxis(props *spec.FieldProps, v any, axes [2]graphAxis) bool {
	n, ok := asInt(v)
	if !ok || (n != 1 && n != 2) {
		return false
	}
	if n == 1 {
		return true
	}
	props.Axis = "right"
	if axes[1].hidden {
		props.Axis = "hidden"
	}
	second := axes[1].props
	props.Unit, props.Decimals, props.Min, props.Max = second.Unit, second.Decimals, second.Min, second.Max
	return true
}

func (b *panelBuild) legacySinglestat() {
	p := &b.panel.FieldProps
	d := b.disp()
	b.singlestatUnit(p)
	b.legacyDecimals(p, b.raw["decimals"])
	b.consume("decimals", "format")
	if text := asString(b.raw["nullText"]); text != "" {
		p.NoValue, _ = cutRunes(text, maxNoValue)
	}
	b.consume("nullText")
	b.singlestatReducer(d)
	b.singlestatThresholds(p)
	b.singlestatColors(d)
	b.singlestatMappings(p)
	b.singlestatGauge(p, d)
	if present(b.raw["tableColumn"]) {
		b.partial("tableColumn", "Picking one column of a table result is not supported yet.")
	}
	b.consume("tableColumn", "prefixFontSize", "postfixFontSize", "valueFontSize", "mappingTypes", "mappingType")
}

func (b *panelBuild) singlestatUnit(p *spec.FieldProps) {
	format := asString(b.raw["format"])
	prefix, postfix := asString(b.raw["prefix"]), asString(b.raw["postfix"])
	b.consume("prefix", "postfix")
	if (prefix != "" || postfix != "") && (format == "none" || format == "short" || format == "") {
		switch {
		case prefix != "" && postfix != "":
			b.partial("prefix", "A prefix and a postfix together are not supported yet.")
			fallthrough
		case postfix != "":
			p.Unit = "suffix:" + postfix
		default:
			p.Unit = "prefix:" + prefix
		}
		if !validUnit(p.Unit) {
			p.Unit = ""
			b.partial("postfix", "The prefix or postfix is too long to be kept.")
		}
		return
	}
	b.legacyUnit(p, format, "format")
	if prefix != "" || postfix != "" {
		b.partial("postfix", "A prefix or postfix cannot be combined with this unit.")
	}
}

func (b *panelBuild) singlestatReducer(d *spec.Display) {
	name := asString(b.raw["valueName"])
	b.consume("valueName")
	if name == "" {
		return
	}
	reducer, ok := singlestatReducers[name]
	if !ok {
		b.partial("valueName", fmt.Sprintf("The value %s is not supported yet.", name))
		return
	}
	d.Reduce = reducer
}

func grafanaStatThresholds() []spec.Threshold {
	return []spec.Threshold{{Color: "ok"}, {Value: ptr(80.0), Color: "critical"}}
}

func (b *panelBuild) singlestatThresholds(p *spec.FieldProps) {
	text := strings.TrimSpace(asString(b.raw["thresholds"]))
	colors := stringList(b.raw["colors"])
	b.consume("thresholds", "colors")
	if text == "" || len(colors) == 0 {
		if b.typ == spec.Gauge || truthy(b.raw["colorValue"]) || truthy(b.raw["colorBackground"]) {
			p.Thresholds = grafanaStatThresholds()
		}
		return
	}
	levels := strings.Split(text, ",")
	var steps []thresholdStep
	for i, color := range colors {
		step := thresholdStep{index: i, color: color}
		if i > 0 {
			if i-1 >= len(levels) {
				break
			}
			value, ok := numberValue(strings.TrimSpace(levels[i-1]))
			if !ok {
				b.partial("thresholds", "The thresholds are not numbers.")
				return
			}
			step.value = &value
		}
		steps = append(steps, step)
	}
	p.Thresholds = b.finishThresholds(steps, func(i int) string { return fmt.Sprintf("colors[%d]", i) })
}

func (b *panelBuild) singlestatColors(d *spec.Display) {
	b.consume("colorBackground", "colorValue", "sparkline")
	if b.typ != spec.Stat {
		return
	}
	switch {
	case truthy(b.raw["colorBackground"]):
		d.ColorMode = "background"
	case truthy(b.raw["colorValue"]):
		d.ColorMode = "value"
	default:
		d.ColorMode = "none"
	}
	sparkline, _ := asObject(b.raw["sparkline"])
	graph := flagOr(sparkline, "show", false)
	d.Graph = &graph
}

func (b *panelBuild) singlestatMappings(p *spec.FieldProps) {
	kind, _ := asInt(b.raw["mappingType"])
	valueMaps, _ := asArray(b.raw["valueMaps"])
	rangeMaps, _ := asArray(b.raw["rangeMaps"])
	b.consume("valueMaps", "rangeMaps")
	if kind != 2 {
		p.Mappings = append(p.Mappings, b.legacyMappings(valueMaps, "valueMaps", legacyValueMapping)...)
	}
	if kind != 1 {
		p.Mappings = append(p.Mappings, b.legacyMappings(rangeMaps, "rangeMaps", legacyRangeMapping)...)
	}
	if len(p.Mappings) > maxMappings {
		p.Mappings = p.Mappings[:maxMappings]
		b.partial("valueMaps", "A panel holds at most 50 value mappings.")
	}
}

func (b *panelBuild) legacyMappings(items []any, base string, convert func(map[string]any) ([]spec.Mapping, string)) []spec.Mapping {
	var out []spec.Mapping
	for i, item := range items {
		m, ok := asObject(item)
		if !ok {
			continue
		}
		mappings, reason := convert(m)
		if reason != "" {
			b.partial(fmt.Sprintf("%s[%d]", base, i), reason)
		}
		out = append(out, mappings...)
	}
	return out
}

func legacyRangeMapping(m map[string]any) ([]spec.Mapping, string) {
	from, to := asString(m["from"]), asString(m["to"])
	if from == "null" && to == "null" {
		text := strings.TrimSpace(asString(m["text"]))
		mapping, ok := mappingResult{text: text}.apply(spec.Mapping{Special: "null"})
		return keepMapping(mapping, ok)
	}
	return rangeMapping(nil, m)
}

func (b *panelBuild) singlestatGauge(p *spec.FieldProps, d *spec.Display) {
	gauge, _ := asObject(b.raw["gauge"])
	b.consume("gauge")
	if !truthy(gauge["show"]) {
		return
	}
	if f, ok := numberValue(gauge["minValue"]); ok {
		p.Min = &f
	}
	if f, ok := numberValue(gauge["maxValue"]); ok {
		p.Max = &f
	}
	b.fixMinMax(p, "gauge.maxValue")
	if flag, ok := gauge["thresholdMarkers"].(bool); ok && b.typ == spec.Gauge {
		d.ThresholdMarkers = &flag
	}
	if truthy(gauge["thresholdLabels"]) {
		b.partial("gauge.thresholdLabels", "Threshold labels are not shown.")
	}
}

func (b *panelBuild) legacyTable() {
	d := b.disp()
	styles, _ := asArray(b.raw["styles"])
	b.consume("styles")
	for i, item := range styles {
		style, ok := asObject(item)
		if ok {
			b.tableStyle(style, fmt.Sprintf("styles[%d]", i))
		}
	}
	if flag, ok := b.raw["showHeader"].(bool); ok {
		d.Header = &flag
		b.consume("showHeader")
	}
	switch asString(b.raw["transform"]) {
	case "", "table":
		b.consume("transform")
	default:
		b.partial("transform", "This table transform is not supported yet.")
	}
	if asString(b.raw["fontSize"]) == "100%" {
		b.consume("fontSize")
	}
	if present(b.raw["columns"]) {
		b.partial("columns", "Column selections are not supported yet.")
	}
}

var tableStyleHandled = []string{"pattern", "type", "unit", "decimals", "alias", "thresholds", "colors", "colorMode", "align"}

func (b *panelBuild) tableStyle(style map[string]any, path string) {
	pattern := asString(style["pattern"])
	var props spec.FieldProps
	if asString(style["type"]) == "hidden" {
		hidden := true
		props.Hidden = &hidden
	}
	b.legacyUnit(&props, style["unit"], path+".unit")
	b.legacyDecimals(&props, style["decimals"])
	if alias := strings.TrimSpace(asString(style["alias"])); alias != "" && pattern != "/.*/" {
		props.DisplayName, _ = cutRunes(alias, maxDisplayName)
	}
	props.Thresholds = b.styleThresholds(style, path)
	b.styleExtras(style, path)
	switch {
	case pattern == "/.*/":
		b.mergeTableDefaults(props)
	case regexAlias.MatchString(pattern) && validLength(pattern, maxRegexMatch):
		b.legacyTableOverride(spec.Match{SeriesRegex: pattern}, props, path)
	case validLength(pattern, maxNameMatch):
		b.legacyTableOverride(spec.Match{Field: pattern}, props, path)
	default:
		b.partial(path+".pattern", "The column pattern is too long to be matched.")
	}
}

func (b *panelBuild) styleThresholds(style map[string]any, path string) []spec.Threshold {
	colors := stringList(style["colors"])
	var levels []float64
	for _, item := range anySlice(style["thresholds"]) {
		f, ok := numberValue(item)
		if !ok {
			b.partial(path+".thresholds", "The thresholds are not numbers.")
			return nil
		}
		levels = append(levels, f)
	}
	var steps []thresholdStep
	for i, color := range colors {
		step := thresholdStep{index: i, color: color}
		if i > 0 {
			if i-1 >= len(levels) {
				break
			}
			value := levels[i-1]
			step.value = &value
		}
		steps = append(steps, step)
	}
	return b.finishThresholds(steps, func(i int) string { return fmt.Sprintf("%s.colors[%d]", path, i) })
}

func anySlice(v any) []any {
	items, _ := asArray(v)
	return items
}

func (b *panelBuild) styleExtras(style map[string]any, path string) {
	for _, key := range sortedKeys(style) {
		if slices.Contains(tableStyleHandled, key) || !present(style[key]) {
			continue
		}
		b.partial(path+"."+key, "")
	}
	if asString(style["type"]) == "date" {
		b.partial(path+".type", "Date columns are shown as numbers.")
	}
	switch mode := asString(style["colorMode"]); mode {
	case "row", "cell":
		b.partial(path+".colorMode", "Cell colors are drawn in the SlashObserve style.")
	}
	if align := asString(style["align"]); align != "" && align != "auto" {
		b.partial(path+".align", "")
	}
}

func (b *panelBuild) mergeTableDefaults(props spec.FieldProps) {
	p := &b.panel.FieldProps
	p.Unit = cmp.Or(props.Unit, p.Unit)
	if props.Decimals != nil {
		p.Decimals = props.Decimals
	}
	if len(props.Thresholds) > 0 {
		p.Thresholds = props.Thresholds
	}
}

func (b *panelBuild) legacyTableOverride(match spec.Match, props spec.FieldProps, path string) {
	if fieldPropsEmpty(props) {
		return
	}
	b.addOverride(spec.Override{Match: match, FieldProps: props}, path)
}
