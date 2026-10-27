package grafana

import (
	"encoding/json"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxOverrides  = 50
	maxNameMatch  = 200
	maxRegexMatch = 500
)

var overrideFieldTypes = []string{"number", "string", "time", "boolean"}

func (b *panelBuild) convertOverrides() {
	items, _ := asArray(lookupValue(b.raw, "fieldConfig.overrides"))
	for i, item := range items {
		path := fmt.Sprintf("fieldConfig.overrides[%d]", i)
		override, ok := b.convertOverride(item, path)
		if ok {
			b.addOverride(override, path)
		}
	}
}

func (b *panelBuild) addOverride(override spec.Override, path string) {
	if len(b.panel.Overrides) >= maxOverrides {
		b.partial(path, "A panel holds at most 50 overrides.")
		return
	}
	b.panel.Overrides = append(b.panel.Overrides, override)
}

func (b *panelBuild) convertOverride(item any, path string) (spec.Override, bool) {
	raw, ok := asObject(item)
	if !ok {
		return spec.Override{}, false
	}
	matcher, _ := asObject(raw["matcher"])
	match, ok := b.matchFor(matcher)
	if !ok {
		b.partial(path, "")
		return spec.Override{}, false
	}
	b.consume(path+".matcher.id", path+".matcher.options")
	var props spec.FieldProps
	properties, _ := asArray(raw["properties"])
	for j, item := range properties {
		b.overrideProperty(&props, item, path, j)
	}
	b.fixMinMax(&props, path)
	if fieldPropsEmpty(props) {
		return spec.Override{}, false
	}
	return spec.Override{Match: match, FieldProps: props}, true
}

func (b *panelBuild) overrideProperty(props *spec.FieldProps, item any, path string, j int) {
	prop, _ := asObject(item)
	b.consume(fmt.Sprintf("%s.properties[%d].id", path, j))
	site := propSite{
		value:  fmt.Sprintf("%s.properties[%d].value", path, j),
		report: fmt.Sprintf("%s.properties[%d]", path, j),
		whole:  true,
	}
	id := asString(prop["id"])
	handler, ok := propHandlers[id]
	if !ok {
		if isCosmetic("fieldConfig.defaults."+id, prop["value"]) {
			b.consume(site.report)
			return
		}
		b.partial(site.report, "")
		return
	}
	if value := prop["value"]; value != nil {
		handler(b, props, value, site)
	}
}

func fieldPropsEmpty(props spec.FieldProps) bool {
	data, err := json.Marshal(props)
	return err == nil && string(data) == "{}"
}

func (b *panelBuild) matchFor(matcher map[string]any) (spec.Match, bool) {
	options := matcher["options"]
	text, isText := options.(string)
	switch asString(matcher["id"]) {
	case "byName":
		if !isText || !validLength(text, maxNameMatch) {
			return spec.Match{}, false
		}
		if b.typ == spec.Table || b.typ == spec.Logs || b.typ == spec.NodeGraph {
			return spec.Match{Field: text}, true
		}
		return spec.Match{Series: text}, true
	case "byRegexp":
		return spec.Match{SeriesRegex: text}, isText && validLength(text, maxRegexMatch)
	case "byFrameRefID":
		ref, ok := b.refs[text]
		return spec.Match{Query: ref}, isText && ok
	case "byType":
		return spec.Match{FieldType: text}, isText && slices.Contains(overrideFieldTypes, text)
	}
	return spec.Match{}, false
}

func validLength(s string, limit int) bool {
	n := utf8.RuneCountInString(s)
	return n >= 1 && n <= limit
}
