package grafana

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxThresholds  = 10
	maxMappings    = 50
	maxDecimals    = 10
	maxDisplayName = 200
	maxNoValue     = 100
	maxCustomUnit  = 20
	maxFormatUnit  = 40
	maxMappedValue = 200

	colorSchemeReason = "This color scheme is not supported yet; series use the default palette."
)

type propSite struct {
	value  string
	report string
	whole  bool
}

func (s propSite) at(suffix string) string {
	if s.whole {
		return s.report
	}
	return s.value + suffix
}

type propHandler func(b *panelBuild, p *spec.FieldProps, v any, s propSite)

var propOrder = []string{
	"unit", "decimals", "min", "max", "displayName", "noValue", "thresholds", "mappings", "links",
	"color", "custom.drawStyle", "custom.fillOpacity", "custom.lineStyle",
	"custom.lineInterpolation", "custom.lineWidth", "custom.showPoints", "custom.pointSize", "custom.spanNulls",
	"custom.stacking", "custom.axisPlacement", "custom.hideFrom",
}

var propHandlers = map[string]propHandler{
	"unit":                     propUnit,
	"decimals":                 propDecimals,
	"min":                      propMin,
	"max":                      propMax,
	"displayName":              propDisplayName,
	"noValue":                  propNoValue,
	"thresholds":               propThresholds,
	"mappings":                 propMappings,
	"links":                    propLinks,
	"color":                    propColor,
	"custom.drawStyle":         propDrawStyle,
	"custom.fillOpacity":       propFillOpacity,
	"custom.lineStyle":         propLineStyle,
	"custom.lineInterpolation": propLineInterpolation,
	"custom.lineWidth":         propLineWidth,
	"custom.showPoints":        propShowPoints,
	"custom.pointSize":         propPointSize,
	"custom.spanNulls":         propSpanNulls,
	"custom.stacking":          propStacking,
	"custom.axisPlacement":     propAxisPlacement,
	"custom.hideFrom":          propHideFrom,
}

var unitSet = sync.OnceValue(func() map[string]bool {
	set := map[string]bool{}
	for _, unit := range spec.Units() {
		set[unit] = true
	}
	return set
})

var coloringCellTypes = map[string]bool{"color-text": true, "color-background": true, "color-background-solid": true}

var customUnitForms = []struct {
	prefix string
	limit  int
}{
	{"suffix:", maxCustomUnit},
	{"prefix:", maxCustomUnit},
	{"time:", maxFormatUnit},
	{"si:", maxFormatUnit},
	{"count:", maxFormatUnit},
	{"currency:", maxFormatUnit},
}

func customUnitForm(unit string) (rest string, limit int, ok bool) {
	for _, form := range customUnitForms {
		if rest, ok := strings.CutPrefix(unit, form.prefix); ok {
			return rest, form.limit, true
		}
	}
	return "", 0, false
}

func validUnit(unit string) bool {
	if unitSet()[unit] {
		return true
	}
	rest, limit, ok := customUnitForm(unit)
	if !ok {
		return false
	}
	n := utf8.RuneCountInString(rest)
	return n >= 1 && n <= limit && !strings.ContainsAny(rest, "\n\r")
}

func importedUnit(unit string) (string, bool) {
	if validUnit(unit) {
		return unit, true
	}
	if _, _, custom := customUnitForm(unit); custom {
		return "", false
	}
	text := "suffix:" + unit
	return text, validUnit(text)
}

func numberValue(v any) (float64, bool) {
	if s, ok := v.(string); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	}
	return asFloat(v)
}

func (b *panelBuild) convertFieldConfig() {
	b.thresholdsMatter = b.thresholdsApply()
	b.consumeCellDisplay()
	for _, id := range propOrder {
		path := "fieldConfig.defaults." + id
		if v, ok := b.value(path); ok && v != nil {
			s := propSite{value: path, report: path}
			propHandlers[id](b, &b.panel.FieldProps, v, s)
		}
	}
	b.fixMinMax(&b.panel.FieldProps, "fieldConfig.defaults.max")
	b.convertOverrides()
	b.convertLegacy()
	b.convertDiscrete()
}

func (b *panelBuild) fixMinMax(p *spec.FieldProps, path string) {
	if p.Min != nil && p.Max != nil && *p.Min >= *p.Max {
		p.Min, p.Max = nil, nil
		b.partial(path, "The minimum is not below the maximum, so neither is kept.")
	}
}

func (b *panelBuild) colorMode() string {
	v, _ := b.value("fieldConfig.defaults.color.mode")
	return asString(v)
}

func (b *panelBuild) thresholdsApply() bool {
	switch b.typ {
	case spec.Stat, spec.Gauge, spec.BarGauge, spec.StateTimeline, spec.StatusHistory, spec.Canvas:
		return true
	case spec.Timeseries, spec.Trend, spec.Candlestick:
		v, _ := b.value("fieldConfig.defaults.custom.thresholdsStyle.mode")
		mode := asString(v)
		b.consume("fieldConfig.defaults.custom.thresholdsStyle.mode")
		return (mode != "" && mode != "off") || b.colorMode() == "thresholds" || b.legacyThresholds()
	case spec.Table:
		return b.cellColoring()
	}
	return b.colorMode() == "thresholds"
}

func (b *panelBuild) legacyThresholds() bool {
	items, _ := asArray(b.raw["thresholds"])
	return b.n.sourceType == "graph" && len(items) > 0
}

func (b *panelBuild) cellColoring() bool {
	for _, path := range []string{"fieldConfig.defaults.custom.cellOptions.type", "fieldConfig.defaults.custom.displayMode"} {
		if v, ok := b.value(path); ok && coloringCellTypes[asString(v)] {
			return true
		}
	}
	overrides, _ := asArray(lookupValue(b.raw, "fieldConfig.overrides"))
	for _, item := range overrides {
		if overrideColorsCells(item) {
			return true
		}
	}
	return b.n.sourceType == "table-old"
}

func lookupValue(m map[string]any, path string) any {
	v, _ := lookup(m, path)
	return v
}

func overrideColorsCells(item any) bool {
	ov, _ := asObject(item)
	props, _ := asArray(ov["properties"])
	for _, raw := range props {
		prop, _ := asObject(raw)
		value, _ := asObject(prop["value"])
		switch asString(prop["id"]) {
		case "custom.cellOptions":
			if coloringCellTypes[asString(value["type"])] {
				return true
			}
		case "custom.displayMode":
			if coloringCellTypes[asString(prop["value"])] {
				return true
			}
		}
	}
	return false
}

func (b *panelBuild) consumeCellDisplay() {
	if b.typ != spec.Table {
		return
	}
	if v, ok := b.value("fieldConfig.defaults.custom.displayMode"); ok && asString(v) == "auto" {
		b.consume("fieldConfig.defaults.custom.displayMode")
	}
	v, ok := b.value("fieldConfig.defaults.custom.cellOptions.type")
	if !ok {
		return
	}
	switch kind := asString(v); {
	case kind == "color-text":
		b.consume("fieldConfig.defaults.custom.cellOptions")
	case coloringCellTypes[kind]:
		b.partial("fieldConfig.defaults.custom.cellOptions", "Cell colors are drawn in the SlashObserve style.")
	}
}

func propUnit(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	unit, ok := v.(string)
	if !ok || unit == "" {
		return
	}
	imported, ok := importedUnit(unit)
	if !ok {
		b.partial(s.at(""), fmt.Sprintf("Unit %s is not supported yet.", unit))
		return
	}
	p.Unit = imported
}

func propDecimals(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	f, ok := numberValue(v)
	if !ok {
		return
	}
	n := int(math.Round(f))
	if float64(n) != f || n < 0 || n > maxDecimals {
		n = min(max(n, 0), maxDecimals)
		b.partial(s.at(""), "Decimals are kept as a whole number between 0 and 10.")
	}
	p.Decimals = &n
}

func propMin(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	if f, ok := numberValue(v); ok {
		p.Min = &f
	}
}

func propMax(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	if f, ok := numberValue(v); ok {
		p.Max = &f
	}
}

func propDisplayName(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	name, _ := v.(string)
	if strings.TrimSpace(name) == "" {
		return
	}
	cut, wasCut := cutRunes(name, maxDisplayName)
	if wasCut {
		b.partial(s.at(""), "The display name was cut to 200 characters.")
	}
	p.DisplayName = cut
}

func propNoValue(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	text, ok := v.(string)
	if !ok {
		if f, isNumber := asFloat(v); isNumber {
			text = strconv.FormatFloat(f, 'f', -1, 64)
		}
	}
	if text == "" {
		return
	}
	cut, wasCut := cutRunes(text, maxNoValue)
	if wasCut {
		b.partial(s.at(""), "The no-value text was cut to 100 characters.")
	}
	p.NoValue = cut
}

type thresholdStep struct {
	index int
	value *float64
	color string
}

func propThresholds(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	cfg, ok := asObject(v)
	if !ok || !b.thresholdsMatter {
		return
	}
	steps := b.thresholdSteps(cfg["steps"], s)
	if len(steps) > 0 {
		p.Thresholds = steps
	}
	if asString(cfg["mode"]) == "percentage" {
		p.ThresholdMode = "percentage"
	}
}

func (b *panelBuild) thresholdSteps(raw any, s propSite) []spec.Threshold {
	items, _ := asArray(raw)
	var steps []thresholdStep
	for i, item := range items {
		step, ok := asObject(item)
		if !ok {
			continue
		}
		var value *float64
		if f, isNumber := numberValue(step["value"]); isNumber {
			value = &f
		}
		steps = append(steps, thresholdStep{index: i, value: value, color: asString(step["color"])})
	}
	return b.finishThresholds(steps, func(i int) string { return s.at(fmt.Sprintf(".steps[%d]", i)) })
}

func (b *panelBuild) finishThresholds(steps []thresholdStep, path func(int) string) []spec.Threshold {
	slices.SortStableFunc(steps, compareSteps)
	if len(steps) > 0 {
		steps[0].value = nil
	}
	var out []spec.Threshold
	var last *float64
	for k, step := range steps {
		report := path(step.index)
		color, ok := thresholdColor(step.color)
		switch {
		case strings.EqualFold(strings.TrimSpace(step.color), "transparent") && step.value == nil:
			continue
		case !ok:
			b.partial(report, fmt.Sprintf("The threshold color %q is not supported yet.", step.color))
		case k > 0 && sameThreshold(last, step.value, len(out) > 0):
			b.partial(report, "Thresholds that repeat a value are not kept.")
		case len(out) >= maxThresholds:
			b.partial(report, "A panel holds at most 10 thresholds.")
		default:
			out = append(out, spec.Threshold{Value: step.value, Color: color})
			last = step.value
		}
	}
	return out
}

func compareSteps(a, b thresholdStep) int {
	switch {
	case a.value == nil && b.value == nil:
		return 0
	case a.value == nil:
		return -1
	case b.value == nil:
		return 1
	}
	return cmp.Compare(*a.value, *b.value)
}

func sameThreshold(last, value *float64, haveLast bool) bool {
	if !haveLast {
		return false
	}
	if last == nil || value == nil {
		return last == nil && value == nil
	}
	return *last == *value
}

func propColor(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	cfg, ok := asObject(v)
	if !ok {
		return
	}
	mode := asString(cfg["mode"])
	switch {
	case mode == "" || mode == "palette-classic":
		return
	case mode == "thresholds" && thresholdColoredTypes[b.typ]:
		return
	case mode == "fixed" || mode == "shades":
		if color, ok := seriesColor(asString(cfg["fixedColor"])); ok {
			p.Color = color
			return
		}
	}
	b.partial(s.at(""), colorSchemeReason)
}

var thresholdColoredTypes = map[spec.PanelType]bool{
	spec.Stat: true, spec.Gauge: true, spec.BarGauge: true, spec.Table: true, spec.StateTimeline: true,
	spec.StatusHistory: true, spec.Geomap: true, spec.Canvas: true,
}

func propDrawStyle(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	switch style := asString(v); style {
	case "line", "bars", "points":
		p.Style = style
	default:
		b.partial(s.at(""), fmt.Sprintf("The draw style %s is not supported yet.", style))
	}
}

func propFillOpacity(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	if f, ok := numberValue(v); ok {
		fill := int(math.Round(min(max(f, 0), 100)))
		p.Fill = &fill
	}
}

func propLineStyle(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value + ".fill")
	b.consume(s.value + ".dash")
	cfg, ok := asObject(v)
	if !ok {
		return
	}
	switch asString(cfg["fill"]) {
	case "dash":
		p.LineStyle = "dashed"
	case "dot":
		p.LineStyle = "dotted"
	case "solid":
		p.LineStyle = "solid"
	}
}

const (
	maxLineWidth     = 10
	canvasLineWidth  = 2
	grafanaPointSize = 5
	canvasPointSize  = 4
	maxSpanNullsMs   = 31536000000
)

func lineWidthFor(width float64) int {
	n := int(math.Round(min(max(width, 0), maxLineWidth)))
	if n == 1 {
		return canvasLineWidth
	}
	return n
}

func pointSizeFor(size float64) int {
	n := int(math.Round(min(max(size, 1), maxPointSize)))
	if n == grafanaPointSize {
		return canvasPointSize
	}
	return n
}

func propLineInterpolation(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	switch mode := asString(v); mode {
	case "linear":
		if s.whole {
			p.LineInterpolation = mode
		}
	case "smooth", "stepBefore", "stepAfter":
		p.LineInterpolation = mode
	default:
		b.partial(s.at(""), fmt.Sprintf("The line interpolation %s is not supported yet.", mode))
	}
}

func propLineWidth(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	if f, ok := numberValue(v); ok {
		setLineWidth(p, lineWidthFor(f), s.whole)
	}
}

func setLineWidth(p *spec.FieldProps, width int, explicit bool) {
	if width != canvasLineWidth || explicit {
		p.LineWidth = &width
	}
}

func propShowPoints(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	switch mode := asString(v); mode {
	case "always", "never":
		p.Points = mode
	case "auto":
		if s.whole {
			p.Points = mode
		}
	default:
		b.partial(s.at(""), fmt.Sprintf("The point display %s is not supported yet.", mode))
	}
}

func propPointSize(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	f, ok := numberValue(v)
	if !ok {
		return
	}
	shown := p.Points == "always" || p.Style == "points" || b.panel.Points == "always" || b.panel.Style == "points"
	if s.whole || shown {
		setPointSize(p, pointSizeFor(f), s.whole)
	}
}

func setPointSize(p *spec.FieldProps, size int, explicit bool) {
	if size != canvasPointSize || explicit {
		p.PointSize = &size
	}
}

func propSpanNulls(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	if on, isBool := v.(bool); isBool {
		setSpanNulls(p, on, s.whole)
		return
	}
	f, ok := numberValue(v)
	switch {
	case !ok:
	case f >= 1:
		p.SpanNulls = int64(math.Round(min(f, maxSpanNullsMs)))
	default:
		setSpanNulls(p, false, s.whole)
	}
}

func setSpanNulls(p *spec.FieldProps, on, explicit bool) {
	if on || explicit {
		p.SpanNulls = on
	}
}

func propStacking(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value + ".mode")
	cfg, ok := asObject(v)
	if !ok {
		return
	}
	switch mode := asString(cfg["mode"]); mode {
	case "normal", "percent", "none":
		p.Stack = mode
	}
}

func propAxisPlacement(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	switch placement := asString(v); placement {
	case "left", "right", "hidden":
		p.Axis = placement
	}
}

func propHideFrom(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	cfg, ok := asObject(v)
	if !ok {
		return
	}
	b.consume(s.value + ".viz")
	if truthy(cfg["viz"]) {
		hidden := true
		p.Hidden = &hidden
	}
	for _, key := range []string{"legend", "tooltip"} {
		if truthy(cfg[key]) {
			b.partial(s.at("."+key), fmt.Sprintf("Hiding a series from the %s is not supported yet.", key))
		}
		b.consume(s.value + "." + key)
	}
}

func propLinks(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	items, _ := asArray(v)
	links := b.linkList(items, func(i int) string { return s.at(fmt.Sprintf("[%d]", i)) })
	if len(links) > 0 {
		p.Links = links
	}
}

var staticPanelNames = map[spec.PanelType]string{spec.Text: "text panels", spec.DashList: "dashboard lists", spec.News: "news panels"}

func (b *panelBuild) convertLinks() {
	items, _ := asArray(b.raw["links"])
	b.consume("links")
	if len(items) == 0 {
		return
	}
	if name, static := staticPanelNames[b.typ]; static {
		b.partial("links", fmt.Sprintf("Links on %s are not supported yet.", name))
		return
	}
	links := b.linkList(items, func(i int) string { return fmt.Sprintf("links[%d]", i) })
	for _, link := range links {
		if !slices.Contains(b.panel.Links, link) && len(b.panel.Links) < maxLinks {
			b.panel.Links = append(b.panel.Links, link)
		}
	}
}

func (b *panelBuild) linkList(items []any, path func(int) string) []spec.Link {
	var links []spec.Link
	for i, item := range items {
		link, reason := linkOf(item)
		switch {
		case reason != "":
			b.partial(path(i), reason)
		case slices.Contains(links, link):
		case len(links) >= maxLinks:
			b.partial(path(i), "A panel holds at most 20 links.")
		default:
			links = append(links, link)
			b.noteLinkOptions(item, path(i))
		}
	}
	return links
}

func (b *panelBuild) noteLinkOptions(item any, path string) {
	l, _ := asObject(item)
	for _, key := range []string{"keepTime", "includeVars"} {
		if truthy(l[key]) {
			b.partial(path+"."+key, "Links do not carry the time range or variable selections along.")
		}
	}
}

var linkScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

func hasForeignScheme(url string) bool {
	url = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, url)
	scheme := strings.ToLower(linkScheme.FindString(strings.TrimLeftFunc(url, unicode.IsControl)))
	return scheme != "" && scheme != "http:" && scheme != "https:" && scheme != "mailto:"
}

func linkOf(item any) (spec.Link, string) {
	l, ok := asObject(item)
	if !ok {
		return spec.Link{}, "This link is not supported yet."
	}
	if kind := asString(l["type"]); kind != "" && kind != "link" {
		return spec.Link{}, "Only plain links are supported; links to other dashboards are not kept."
	}
	url := strings.TrimSpace(asString(l["url"]))
	if url == "" {
		return spec.Link{}, "A link without an address is not kept."
	}
	if _, tooLong := cutRunes(url, 2000); tooLong {
		return spec.Link{}, "A link address longer than 2000 characters is not kept."
	}
	if hasForeignScheme(url) {
		return spec.Link{}, "A link whose address does not start with http, https or mailto is not kept."
	}
	title := strings.TrimSpace(asString(l["title"]))
	if title == "" {
		title = url
	}
	title, _ = cutRunes(title, 200)
	return spec.Link{Title: title, URL: url, NewTab: truthy(l["targetBlank"])}, ""
}

func propMappings(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	items, _ := asArray(v)
	p.Mappings = b.convertMappings(items, s)
}

func (b *panelBuild) convertMappings(items []any, s propSite) []spec.Mapping {
	var out []spec.Mapping
	for i, item := range items {
		m, ok := asObject(item)
		if !ok {
			continue
		}
		report := s.at(fmt.Sprintf("[%d]", i))
		entries, reason := b.mappingEntries(m)
		if reason != "" {
			b.partial(report, reason)
		}
		for _, entry := range entries {
			if len(out) >= maxMappings {
				b.partial(report, "A panel holds at most 50 value mappings.")
				return out
			}
			out = append(out, entry)
		}
	}
	return out
}

func (b *panelBuild) mappingEntries(m map[string]any) ([]spec.Mapping, string) {
	options, _ := asObject(m["options"])
	switch kind := mappingKind(m); kind {
	case "value":
		return valueMappings(options, m)
	case "range":
		return rangeMapping(options, m)
	case "regex":
		return regexMapping(options)
	case "special":
		return specialMapping(options)
	}
	return nil, "This value mapping is not supported yet."
}

func mappingKind(m map[string]any) string {
	if kind := asString(m["type"]); kind != "" {
		return kind
	}
	switch n, _ := asInt(m["type"]); n {
	case 1:
		return "value"
	case 2:
		return "range"
	}
	return ""
}

type mappingResult struct{ text, color string }

func resultOf(v any) (mappingResult, string) {
	r, _ := asObject(v)
	out := mappingResult{text: strings.TrimSpace(asString(r["text"]))}
	out.text, _ = cutRunes(out.text, 200)
	if raw := asString(r["color"]); raw != "" {
		if color, ok := thresholdColor(raw); ok {
			out.color = color
		} else {
			return out, fmt.Sprintf("The mapping color %q is not supported yet.", raw)
		}
	}
	return out, ""
}

func (r mappingResult) apply(m spec.Mapping) (spec.Mapping, bool) {
	m.Text, m.Color = r.text, r.color
	return m, r.text != "" || r.color != ""
}

func valueMappings(options, legacy map[string]any) ([]spec.Mapping, string) {
	if len(options) == 0 {
		return legacyValueMapping(legacy)
	}
	keys := sortedKeys(options)
	index := func(key string) float64 {
		entry, _ := asObject(options[key])
		f, ok := asFloat(entry["index"])
		if !ok {
			return math.MaxFloat64
		}
		return f
	}
	slices.SortStableFunc(keys, func(a, b string) int { return cmp.Compare(index(a), index(b)) })
	var out []spec.Mapping
	reason := ""
	for _, key := range keys {
		entry, _ := asObject(options[key])
		result, colorReason := resultOf(entry)
		if colorReason != "" {
			reason = colorReason
		}
		n := utf8.RuneCountInString(key)
		mapping, ok := result.apply(spec.Mapping{Value: key})
		switch {
		case n < 1 || n > 200:
			reason = "Mapped values must be between 1 and 200 characters."
		case !ok:
			reason = "A value mapping without text or color is not kept."
		default:
			out = append(out, mapping)
		}
	}
	return out, reason
}

func legacyValueMapping(m map[string]any) ([]spec.Mapping, string) {
	value := strings.TrimSpace(asString(m["value"]))
	if f, ok := asFloat(m["value"]); ok {
		value = strconv.FormatFloat(f, 'f', -1, 64)
	}
	result := mappingResult{text: strings.TrimSpace(asString(m["text"]))}
	if value == "null" {
		mapping, ok := result.apply(spec.Mapping{Special: "null"})
		return keepMapping(mapping, ok)
	}
	n := utf8.RuneCountInString(value)
	if n < 1 || n > 200 {
		return nil, "Mapped values must be between 1 and 200 characters."
	}
	mapping, ok := result.apply(spec.Mapping{Value: value})
	return keepMapping(mapping, ok)
}

func keepMapping(m spec.Mapping, ok bool) ([]spec.Mapping, string) {
	if !ok {
		return nil, "A value mapping without text or color is not kept."
	}
	return []spec.Mapping{m}, ""
}

func rangeBound(v any) (*float64, bool) {
	if v == nil {
		return nil, true
	}
	if s, ok := v.(string); ok && (s == "" || s == "null") {
		return nil, true
	}
	f, ok := numberValue(v)
	if !ok {
		return nil, false
	}
	return &f, true
}

func rangeMapping(options, legacy map[string]any) ([]spec.Mapping, string) {
	source := options
	if len(source) == 0 {
		source = legacy
	}
	from, okFrom := rangeBound(source["from"])
	to, okTo := rangeBound(source["to"])
	if !okFrom || !okTo || (from == nil && to == nil) {
		return nil, "This range mapping is not supported yet."
	}
	result, reason := resultOf(options["result"])
	if len(options) == 0 {
		result = mappingResult{text: strings.TrimSpace(asString(legacy["text"]))}
	}
	mapping, ok := result.apply(spec.Mapping{Range: &[2]*float64{from, to}})
	if !ok {
		return nil, "A range mapping without text or color is not kept."
	}
	return []spec.Mapping{mapping}, reason
}

func regexMapping(options map[string]any) ([]spec.Mapping, string) {
	pattern := asString(options["pattern"])
	if n := utf8.RuneCountInString(pattern); n < 1 || n > 500 {
		return nil, "This regular expression mapping is not supported yet."
	}
	result, reason := resultOf(options["result"])
	mapping, ok := result.apply(spec.Mapping{Regex: pattern})
	if !ok {
		return nil, "A mapping without text or color is not kept."
	}
	return []spec.Mapping{mapping}, reason
}

func specialMapping(options map[string]any) ([]spec.Mapping, string) {
	match := asString(options["match"])
	var kinds []string
	switch match {
	case "null", "nan", "true", "false", "empty":
		kinds = []string{match}
	case "null+nan":
		kinds = []string{"null", "nan"}
	default:
		return nil, "This special value mapping is not supported yet."
	}
	result, reason := resultOf(options["result"])
	var out []spec.Mapping
	for _, kind := range kinds {
		mapping, ok := result.apply(spec.Mapping{Special: kind})
		if !ok {
			return nil, "A mapping without text or color is not kept."
		}
		out = append(out, mapping)
	}
	return out, reason
}
