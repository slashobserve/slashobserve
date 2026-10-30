package grafana

import (
	"fmt"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxCellText      = 200
	cellActionReason = "Cell actions are not supported yet."
	cellsPath        = "fieldConfig.defaults.custom.cellOptions."
)

var cellTypes = map[string]string{
	"color-text":       "text",
	"color-background": "background",
	"gauge":            "gauge",
	"sparkline":        "sparkline",
	"json-view":        "json",
	"image":            "image",
	"data-links":       "links",
	"pill":             "pill",
}

var legacyCellModes = map[string]spec.Cell{
	"color-text":             {Type: "text"},
	"color-background":       {Type: "background"},
	"color-background-solid": {Type: "background"},
	"gradient-gauge":         {Type: "gauge"},
	"lcd-gauge":              {Type: "gauge", GaugeStyle: "lcd"},
	"basic":                  {Type: "gauge"},
	"gauge":                  {Type: "gauge"},
	"sparkline":              {Type: "sparkline"},
	"json-view":              {Type: "json"},
	"image":                  {Type: "image"},
	"data-links":             {Type: "links"},
}

var sparklineFieldOptions = []struct {
	key     string
	handler propHandler
}{
	{"drawStyle", propDrawStyle},
	{"lineInterpolation", propLineInterpolation},
	{"lineWidth", propLineWidth},
	{"fillOpacity", propFillOpacity},
	{"showPoints", propShowPoints},
	{"spanNulls", propSpanNulls},
	{"axisSoftMin", propAxisSoftMin},
	{"axisSoftMax", propAxisSoftMax},
}

var cellOptionKeys = map[string][]string{
	"background": {"mode", "applyToRow"},
	"gauge":      {"mode", "valueDisplayMode"},
	"image":      {"alt", "title"},
	"sparkline": {
		"hideValue", "drawStyle", "lineInterpolation", "lineWidth", "fillOpacity", "gradientMode",
		"showPoints", "pointSize", "spanNulls", "axisSoftMin", "axisSoftMax",
	},
}

func (b *panelBuild) cellSite(s propSite, key string) propSite {
	return propSite{value: s.value + "." + key, report: s.report, whole: s.whole}
}

func propCellOptions(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	opts, ok := asObject(v)
	if !ok {
		return
	}
	b.consume(s.value + ".type")
	kind := asString(opts["type"])
	cell := &spec.Cell{Type: "auto"}
	if kind != "" && kind != "auto" {
		mapped, known := cellTypes[kind]
		if !known {
			reason := fmt.Sprintf("The cell type %s is not supported yet.", kind)
			if kind == "actions" {
				reason = cellActionReason
			}
			b.partial(s.at(""), reason)
			return
		}
		cell.Type = mapped
	}
	b.cellOptions(p, cell, opts, s)
	b.consumeInertCellOptions(cell.Type, opts, s)
	if wrap, isBool := opts["wrapText"].(bool); isBool {
		b.consume(s.value + ".wrapText")
		setCellWrap(cell, wrap, s.whole)
	}
	setCell(p, cell, s.whole)
}

func setCell(p *spec.FieldProps, cell *spec.Cell, explicit bool) {
	if p.Cell != nil && cell.Wrap == nil {
		cell.Wrap = p.Cell.Wrap
	}
	if *cell == (spec.Cell{Type: "auto"}) && !explicit {
		return
	}
	p.Cell = cell
}

func setCellWrap(cell *spec.Cell, wrap, explicit bool) {
	if wrap || explicit {
		cell.Wrap = &wrap
	}
}

func (b *panelBuild) cellOptions(p *spec.FieldProps, cell *spec.Cell, opts map[string]any, s propSite) {
	switch cell.Type {
	case "background":
		b.backgroundCell(cell, opts, s)
	case "gauge":
		b.gaugeCell(cell, opts, s)
	case "image":
		cell.ImageAlt = b.cellText(opts, "alt", s)
		cell.ImageTitle = b.cellText(opts, "title", s)
	case "sparkline":
		b.sparklineCell(p, cell, opts, s)
	}
}

func (b *panelBuild) backgroundCell(cell *spec.Cell, opts map[string]any, s propSite) {
	b.cosmeticCellOption(opts, "mode", s)
	if on, isBool := opts["applyToRow"].(bool); isBool {
		b.consume(s.value + ".applyToRow")
		if on || s.whole {
			cell.WholeRow = &on
		}
	}
}

func (b *panelBuild) gaugeCell(cell *spec.Cell, opts map[string]any, s propSite) {
	if mode, ok := opts["mode"]; ok {
		if asString(mode) == "lcd" {
			b.consume(s.value + ".mode")
			cell.GaugeStyle = "lcd"
		} else {
			b.cosmeticCellOption(opts, "mode", s)
		}
	}
	switch mode := asString(opts["valueDisplayMode"]); mode {
	case "color", "text", "hidden":
		b.consume(s.value + ".valueDisplayMode")
		cell.ValueMode = mode
	}
}

func (b *panelBuild) sparklineCell(p *spec.FieldProps, cell *spec.Cell, opts map[string]any, s propSite) {
	if hide, isBool := opts["hideValue"].(bool); isBool {
		b.consume(s.value + ".hideValue")
		if hide {
			cell.ValueMode = "hidden"
		}
	}
	b.cosmeticCellOption(opts, "gradientMode", s)
	for _, option := range sparklineFieldOptions {
		if v, ok := opts[option.key]; ok && v != nil {
			option.handler(b, p, v, b.cellSite(s, option.key))
		}
	}
	if v, ok := opts["pointSize"]; ok {
		b.consume(s.value + ".pointSize")
		if f, isNumber := numberValue(v); isNumber {
			setPointSize(p, pointSizeFor(f), s.whole)
		}
	}
}

func (b *panelBuild) cosmeticCellOption(opts map[string]any, key string, s propSite) {
	v, ok := opts[key]
	if ok && isCosmetic(cellsPath+key, v) {
		b.consume(s.value + "." + key)
	}
}

func (b *panelBuild) cellText(opts map[string]any, key string, s propSite) string {
	text, _ := opts[key].(string)
	b.consume(s.value + "." + key)
	if text == "" {
		return ""
	}
	cut, wasCut := cutRunes(text, maxCellText)
	if wasCut {
		b.partial(s.at(""), fmt.Sprintf("The image %s was cut to 200 characters.", key))
	}
	return cut
}

func (b *panelBuild) consumeInertCellOptions(kind string, opts map[string]any, s propSite) {
	applicable := map[string]bool{}
	for _, key := range cellOptionKeys[kind] {
		applicable[key] = true
	}
	for _, keys := range cellOptionKeys {
		for _, key := range keys {
			if _, present := opts[key]; present && !applicable[key] {
				b.consume(s.value + "." + key)
			}
		}
	}
}

func propDisplayMode(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	mode := asString(v)
	if mode == "" || mode == "auto" {
		return
	}
	if p.Cell != nil && p.Cell.Type != "auto" {
		return
	}
	legacy, known := legacyCellModes[mode]
	if !known {
		b.partial(s.at(""), fmt.Sprintf("The cell type %s is not supported yet.", mode))
		return
	}
	cell := legacy
	setCell(p, &cell, s.whole)
}

func propWrapText(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	wrap, isBool := v.(bool)
	if !isBool || (!wrap && !s.whole) {
		return
	}
	cell := &spec.Cell{Type: "auto"}
	if p.Cell != nil {
		copied := *p.Cell
		cell = &copied
	}
	cell.Wrap = &wrap
	p.Cell = cell
}
