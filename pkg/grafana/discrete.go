package grafana

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	discreteSource       = "natel-discrete-panel"
	discreteOpReason     = "Only value maps that match an equal value are supported."
	discreteColorReason  = "The mapping color %q is not supported yet."
	discreteMappingLimit = "A panel holds at most 50 value mappings."
)

func (b *panelBuild) convertDiscrete() {
	if b.n.sourceType != discreteSource {
		return
	}
	p := &b.panel.FieldProps
	if _, ok := b.take("units"); ok {
		b.legacyUnit(p, b.raw["units"], "units")
	}
	b.legacyDecimals(p, b.raw["decimals"])
	b.consume("decimals", "timeOptions")
	mappings := append(b.discreteValueMaps(), b.discreteRangeMaps()...)
	mappings = b.discreteColorMaps(mappings)
	if len(mappings) > maxMappings {
		mappings = mappings[:maxMappings]
		b.partial("colorMaps", discreteMappingLimit)
	}
	p.Mappings = append(p.Mappings, mappings...)
}

func (b *panelBuild) discreteValueMaps() []spec.Mapping {
	items, _ := asArray(b.raw["valueMaps"])
	b.consume("valueMaps")
	var out []spec.Mapping
	for i, item := range items {
		path := fmt.Sprintf("valueMaps[%d]", i)
		m, _ := asObject(item)
		if op := asString(m["op"]); op != "" && op != "=" {
			b.partial(path, discreteOpReason)
			continue
		}
		mapped, reason := legacyValueMapping(m)
		if reason != "" {
			b.partial(path, reason)
		}
		out = append(out, mapped...)
	}
	return out
}

func (b *panelBuild) discreteRangeMaps() []spec.Mapping {
	items, _ := asArray(b.raw["rangeMaps"])
	b.consume("rangeMaps")
	var out []spec.Mapping
	for i, item := range items {
		m, _ := asObject(item)
		if emptyBound(m["from"]) && emptyBound(m["to"]) {
			continue
		}
		mapped, reason := rangeMapping(nil, m)
		if reason != "" {
			b.partial(fmt.Sprintf("rangeMaps[%d]", i), reason)
		}
		out = append(out, mapped...)
	}
	return out
}

func emptyBound(v any) bool {
	bound, ok := rangeBound(v)
	return ok && bound == nil
}

func (b *panelBuild) discreteColorMaps(mappings []spec.Mapping) []spec.Mapping {
	items, _ := asArray(b.raw["colorMaps"])
	b.consume("colorMaps")
	for i, item := range items {
		path := fmt.Sprintf("colorMaps[%d]", i)
		m, _ := asObject(item)
		text, raw := strings.TrimSpace(asString(m["text"])), asString(m["color"])
		if text == "" || raw == "" {
			continue
		}
		color, ok := thresholdColor(raw)
		if !ok {
			b.partial(path, fmt.Sprintf(discreteColorReason, raw))
			continue
		}
		matched := false
		for j := range mappings {
			if mappings[j].Color == "" && (mappings[j].Value == text || mappings[j].Text == text) {
				mappings[j].Color = color
				matched = true
			}
		}
		if !matched && utf8.RuneCountInString(text) <= maxMappedValue && !slices.ContainsFunc(mappings, func(m spec.Mapping) bool { return m.Value == text }) {
			mappings = append(mappings, spec.Mapping{Value: text, Color: color})
		}
	}
	return mappings
}
