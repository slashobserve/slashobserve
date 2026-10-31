package grafana

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxOverrides  = 50
	maxNameMatch  = 200
	maxRegexMatch = 500

	neverMatchID = "neverMatch"
)

var overrideFieldTypes = []string{"number", "string", "time", "boolean"}

func (b *panelBuild) convertOverrides() {
	items, _ := asArray(lookupValue(b.raw, "fieldConfig.overrides"))
	for i, item := range items {
		path := fmt.Sprintf("fieldConfig.overrides[%d]", i)
		if b.takeTimeAxisOverride(item, path) {
			continue
		}
		for _, override := range b.convertOverride(item, path) {
			b.addOverride(override, path)
			if instant, hasInstant := b.instantRefs[override.Match.Query]; hasInstant {
				override.Match = spec.Match{Query: instant}
				b.addOverride(override, path)
			}
		}
	}
}

func (b *panelBuild) takeTimeAxisOverride(item any, path string) bool {
	if b.typ != spec.Timeseries && b.typ != spec.Trend && b.typ != spec.Candlestick {
		return false
	}
	raw, _ := asObject(item)
	matcher, _ := asObject(raw["matcher"])
	properties, _ := asArray(raw["properties"])
	if asString(matcher["id"]) != "byType" || asString(matcher["options"]) != "time" || len(properties) != 1 {
		return false
	}
	only, _ := asObject(properties[0])
	if asString(only["id"]) != "custom.axisPlacement" || asString(only["value"]) != "hidden" {
		return false
	}
	b.consume(path)
	b.disp().XAxis = "hidden"
	return true
}

func (b *panelBuild) addOverride(override spec.Override, path string) {
	if len(b.panel.Overrides) >= maxOverrides {
		b.partial(path, "A panel holds at most 50 overrides.")
		return
	}
	b.panel.Overrides = append(b.panel.Overrides, override)
}

func (b *panelBuild) convertOverride(item any, path string) []spec.Override {
	raw, ok := asObject(item)
	if !ok {
		return nil
	}
	matcher, _ := asObject(raw["matcher"])
	if asString(matcher["id"]) == neverMatchID {
		b.consume(path)
		return nil
	}
	matches, ok := b.matchesFor(matcher)
	if !ok {
		b.partial(path, "")
		return nil
	}
	b.consume(path+".matcher.id", path+".matcher.options")
	var props spec.FieldProps
	properties, _ := asArray(raw["properties"])
	for j, item := range properties {
		b.overrideProperty(&props, item, path, j)
	}
	b.fixMinMax(&props, path)
	if fieldPropsEmpty(props) {
		return nil
	}
	overrides := make([]spec.Override, len(matches))
	for i, match := range matches {
		overrides[i] = spec.Override{Match: match, FieldProps: props}
	}
	return overrides
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
	handler, ok := b.propHandler(id)
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

func (b *panelBuild) matchesFor(matcher map[string]any) ([]spec.Match, bool) {
	id := asString(matcher["id"])
	if fieldType, ok := namedFieldTypes[id]; ok {
		return []spec.Match{{FieldType: fieldType}}, true
	}
	if fields, ok := positionalFields[id]; ok {
		return []spec.Match{{Fields: fields}}, true
	}
	switch id {
	case "byTypes":
		return typesMatches(matcher["options"])
	case "byRegexpOrNames":
		match, ok := regexpOrNamesMatch(matcher["options"])
		return []spec.Match{match}, ok
	}
	match, ok := b.matchFor(matcher)
	return []spec.Match{match}, ok
}

var namedFieldTypes = map[string]string{"numeric": "number", "time": "time"}

var positionalFields = map[string]string{"first": "first", "firstTimeField": "firstTime", "alwaysMatch": "all"}

func typesMatches(options any) ([]spec.Match, bool) {
	items, _ := asArray(options)
	var matches []spec.Match
	var seen []string
	for _, item := range items {
		name, isText := item.(string)
		if !isText || !slices.Contains(overrideFieldTypes, name) {
			return nil, false
		}
		if !slices.Contains(seen, name) {
			seen = append(seen, name)
			matches = append(matches, spec.Match{FieldType: name})
		}
	}
	return matches, len(matches) > 0
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
	case "byNames":
		return namesMatch(options)
	case "byValue":
		return valueMatch(options)
	}
	return spec.Match{}, false
}

var valueMatchOps = []string{"eq", "neq", "lt", "lte", "gt", "gte"}

func isValueMatchReducer(reducer string) bool {
	return isReducer(reducer) || reducer == "allIsNull" || reducer == "allIsZero"
}

func valueMatch(options any) (spec.Match, bool) {
	cfg, ok := asObject(options)
	if !ok {
		return spec.Match{}, false
	}
	reducer := asString(cfg["reducer"])
	op := asString(cfg["op"])
	value, isNumber := numberValue(cfg["value"])
	if !isValueMatchReducer(reducer) || !slices.Contains(valueMatchOps, op) || !isNumber {
		return spec.Match{}, false
	}
	return spec.Match{Value: &spec.ValueMatch{Reducer: reducer, Op: op, Value: value}}, true
}

func regexpOrNamesMatch(options any) (spec.Match, bool) {
	cfg, ok := asObject(options)
	if !ok {
		return spec.Match{}, false
	}
	pattern, patternOK := cfg["pattern"].(string)
	if cfg["pattern"] != nil && !patternOK {
		return spec.Match{}, false
	}
	var names string
	items, namesOK := asArray(cfg["names"])
	if cfg["names"] != nil && !namesOK {
		return spec.Match{}, false
	}
	if len(items) > 0 {
		match, valid := namesMatch(map[string]any{"names": items})
		if !valid {
			return spec.Match{}, false
		}
		names = match.SeriesRegex
	}
	var combined string
	valid := true
	switch {
	case pattern == "" && names == "":
		valid = false
	case names == "":
		combined = pattern
	case pattern == "":
		combined = names
	default:
		combined, valid = joinAlternatives(pattern, names)
	}
	return spec.Match{SeriesRegex: combined}, valid && validLength(combined, maxRegexMatch)
}

func joinAlternatives(pattern, anchoredNames string) (string, bool) {
	if !strings.HasPrefix(pattern, "/") {
		return "(?:" + pattern + ")|" + anchoredNames, true
	}
	end := strings.LastIndex(pattern, "/")
	if end == 0 {
		return "", false
	}
	return "/(?:" + pattern[1:end] + ")|" + anchoredNames + "/" + pattern[end+1:], true
}

func namesMatch(options any) (spec.Match, bool) {
	cfg, ok := asObject(options)
	if !ok {
		return spec.Match{}, false
	}
	items, _ := asArray(cfg["names"])
	escaped := make([]string, 0, len(items))
	for _, item := range items {
		name, isText := item.(string)
		if !isText {
			return spec.Match{}, false
		}
		escaped = append(escaped, escapeRegexLiteral(name))
	}
	if len(escaped) == 0 {
		return spec.Match{}, false
	}
	alternatives := strings.Join(escaped, "|")
	var pattern string
	switch asString(cfg["mode"]) {
	case "", "include":
		pattern = "^(?:" + alternatives + ")$"
	case "exclude":
		pattern = "/^(?!(?:" + alternatives + ")$).*$/"
	default:
		return spec.Match{}, false
	}
	return spec.Match{SeriesRegex: pattern}, validLength(pattern, maxRegexMatch)
}

const regexMetacharacters = `\^$.*+?()[]{}|/`

func escapeRegexLiteral(name string) string {
	var out strings.Builder
	for _, r := range name {
		switch {
		case strings.ContainsRune(regexMetacharacters, r):
			out.WriteByte('\\')
			out.WriteRune(r)
		case r == '\n':
			out.WriteString(`\n`)
		case r == '\r':
			out.WriteString(`\r`)
		case r == '\t':
			out.WriteString(`\t`)
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func validLength(s string, limit int) bool {
	n := utf8.RuneCountInString(s)
	return n >= 1 && n <= limit
}
