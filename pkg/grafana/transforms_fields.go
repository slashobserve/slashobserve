package grafana

import (
	"cmp"
	"fmt"
	"iter"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxConversions   = 50
	maxValueFilters  = 20
	maxFieldPaths    = 50
	maxRefFilters    = 26
	maxFormatLength  = 60
	maxJoinWith      = 10
	maxDelimiter     = 10
	maxPattern       = 200
	maxSubstringEdge = 10000
)

var dateFormatTokens = []string{"YYYY", "YY", "MM", "M", "DD", "D", "HH", "H", "hh", "h", "mm", "ss", "SSS", "A", "a"}

var stringFormats = map[string]string{
	"Upper Case": "upper", "Lower Case": "lower", "Sentence Case": "sentence", "Title Case": "title",
	"Pascal Case": "pascal", "Camel Case": "camel", "Snake Case": "snake", "Kebab Case": "kebab",
	"Trim": "trim", "Substring": "substring",
}

var conversionTypes = []string{"number", "string", "time", "boolean", "enum", "other"}

var valueFilterOps = map[string]string{
	"greater": "gt", "greaterOrEqual": "gte", "lower": "lt", "lowerOrEqual": "lte",
	"equal": "eq", "notEqual": "neq", "isNull": "null", "isNotNull": "notNull",
	"regex": "regex", "range": "range", "between": "range", "substring": "regex", "notSubstring": "regex",
}

var extractFormats = []string{"auto", "json", "kvp", "delimiter", "regexp"}

func supportedDateFormat(format string) bool {
	if !validLength(format, maxFormatLength) {
		return false
	}
	for run := range letterRuns(format) {
		if !slices.Contains(dateFormatTokens, run) {
			return false
		}
	}
	return true
}

func letterRuns(text string) iter.Seq[string] {
	return func(yield func(string) bool) {
		start := -1
		var current rune
		for i, r := range text {
			isLetter := r < utf8.RuneSelf && unicode.IsLetter(r)
			if start >= 0 && (!isLetter || r != current) {
				if !yield(text[start:i]) {
					return
				}
				start = -1
			}
			if isLetter && start < 0 {
				start, current = i, r
			}
		}
		if start >= 0 {
			yield(text[start:])
		}
	}
}

func nameOption(opts map[string]any, key string) (string, bool) {
	name := asString(opts[key])
	return name, validLength(name, maxNameMatch)
}

func (b *panelBuild) dropTransform(path string) (spec.Transform, bool) {
	b.partial(path, "")
	return spec.Transform{}, false
}

func (b *panelBuild) reportOtherKeys(item map[string]any, handled []string, path string) {
	for _, key := range sortedKeys(item) {
		if !slices.Contains(handled, key) && present(item[key]) {
			b.partial(path+"."+key, "")
		}
	}
}

func convertFieldTypeTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	if present(opts["fields"]) {
		b.partial(path+".options.fields", "")
	}
	items, _ := asArray(opts["conversions"])
	t := spec.Transform{Type: "convertFieldType"}
	for i, item := range items {
		itemPath := fmt.Sprintf("%s.options.conversions[%d]", path, i)
		if len(t.Conversions) >= maxConversions {
			b.partial(itemPath, fmt.Sprintf("A transformation keeps at most %d conversions.", maxConversions))
			continue
		}
		conversion, ok := b.conversion(item, itemPath)
		if ok {
			t.Conversions = append(t.Conversions, conversion)
		}
	}
	if len(t.Conversions) == 0 {
		return b.dropTransform(path)
	}
	return t, true
}

func (b *panelBuild) conversion(item any, path string) (spec.Conversion, bool) {
	obj, _ := asObject(item)
	field, fieldOK := nameOption(obj, "targetField")
	to := asString(obj["destinationType"])
	if !fieldOK || !slices.Contains(conversionTypes, to) {
		b.partial(path, "")
		return spec.Conversion{}, false
	}
	conversion := spec.Conversion{Field: field, To: to}
	handled := []string{"targetField", "destinationType", "dateFormat", "joinWith"}
	if format := asString(obj["dateFormat"]); format != "" {
		if !supportedDateFormat(format) {
			b.partial(path+".dateFormat", "")
			return spec.Conversion{}, false
		}
		conversion.DateFormat = format
	}
	if joinWith := asString(obj["joinWith"]); joinWith != "" {
		if utf8.RuneCountInString(joinWith) > maxJoinWith {
			b.partial(path+".joinWith", "")
			return spec.Conversion{}, false
		}
		conversion.JoinWith = joinWith
	}
	b.reportOtherKeys(obj, handled, path)
	return conversion, true
}

func formatStringTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	field, ok := nameOption(opts, "stringField")
	format, known := stringFormats[asString(opts["outputFormat"])]
	if !ok {
		return b.dropTransform(path)
	}
	if !known {
		b.partial(path+".options.outputFormat", "")
		return spec.Transform{}, false
	}
	t := spec.Transform{Type: "formatString", Field: field, Format: format}
	if format == "substring" {
		t.Start = b.substringEdge(opts["substringStart"], path+".options.substringStart")
		t.End = b.substringEdge(opts["substringEnd"], path+".options.substringEnd")
	}
	return t, true
}

func (b *panelBuild) substringEdge(v any, path string) *int {
	if !present(v) {
		return nil
	}
	n, ok := asInt(v)
	if !ok || n < 0 || n > maxSubstringEdge {
		b.partial(path, "")
		return nil
	}
	if n == 0 {
		return nil
	}
	return &n
}

func formatTimeTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	field, ok := nameOption(opts, "timeField")
	if !ok {
		return b.dropTransform(path)
	}
	format := asString(opts["outputFormat"])
	if !supportedDateFormat(format) {
		b.partial(path+".options.outputFormat", "")
		return spec.Transform{}, false
	}
	useZone, set := opts["useTimezone"].(bool)
	utc := (set && !useZone) || b.c.head.Timezone == "utc"
	switch zone := asString(opts["timezone"]); zone {
	case "utc":
		utc = true
	case "", "browser":
	default:
		b.partial(path+".options.timezone", "")
	}
	return spec.Transform{Type: "formatTime", Field: field, Format: format, UTC: utc}, true
}

func renameByRegexTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	regex := asString(opts["regex"])
	if !validLength(regex, maxPattern) {
		return b.dropTransform(path)
	}
	renameTo := asString(opts["renamePattern"])
	if !validLength(renameTo, maxPattern) {
		b.partial(path+".options.renamePattern", "")
		return spec.Transform{}, false
	}
	return spec.Transform{Type: "renameByRegex", Regex: regex, RenameTo: renameTo}, true
}

func filterByValueTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "filterByValue"}
	switch keep := cmp.Or(asString(opts["type"]), "include"); keep {
	case "include":
	case "exclude":
		t.Keep = keep
	default:
		b.partial(path+".options.type", "")
	}
	switch match := cmp.Or(asString(opts["match"]), "any"); match {
	case "any":
	case "all":
		t.Match = match
	default:
		b.partial(path+".options.match", "")
	}
	items, _ := asArray(opts["filters"])
	for i, item := range items {
		itemPath := fmt.Sprintf("%s.options.filters[%d]", path, i)
		if len(t.Filters) >= maxValueFilters {
			b.partial(itemPath, fmt.Sprintf("A transformation keeps at most %d filters.", maxValueFilters))
			continue
		}
		filter, negated, ok := b.valueFilter(item, itemPath, len(items) == 1)
		if !ok {
			continue
		}
		t.Filters = append(t.Filters, filter)
		if negated {
			t.Keep = flippedKeep(t.Keep)
		}
	}
	if len(t.Filters) == 0 {
		return b.dropTransform(path)
	}
	return t, true
}

func flippedKeep(keep string) string {
	if keep == "exclude" {
		return ""
	}
	return "exclude"
}

func (b *panelBuild) valueFilter(item any, path string, alone bool) (filter spec.ValueFilter, negated, ok bool) {
	obj, _ := asObject(item)
	field, fieldOK := nameOption(obj, "fieldName")
	config, _ := asObject(obj["config"])
	if !fieldOK {
		b.partial(path, "")
		return spec.ValueFilter{}, false, false
	}
	id := asString(config["id"])
	op, known := valueFilterOps[id]
	switch {
	case !known:
		b.partial(path+".config.id", "")
		return spec.ValueFilter{}, false, false
	case id == "notSubstring" && !alone:
		b.partial(path+".config.id", "A filter for text a value lacks is supported only as the single filter of a transformation.")
		return spec.ValueFilter{}, false, false
	}
	b.reportOtherKeys(obj, []string{"fieldName", "config"}, path)
	b.reportOtherKeys(config, []string{"id", "options"}, path+".config")
	options, _ := asObject(config["options"])
	filter = spec.ValueFilter{Field: field, Op: op}
	optionPath := path + ".config.options"
	switch op {
	case "gt", "gte", "lt", "lte":
		var number float64
		number, ok = numberValue(options["value"])
		filter.Value = number
	case "eq", "neq":
		filter.Value, ok = comparedFilterValue(options["value"])
	case "regex":
		pattern := asString(options["value"])
		if id == "substring" || id == "notSubstring" {
			pattern = regexp.QuoteMeta(pattern)
		}
		filter.Value, ok = pattern, validLength(pattern, maxPattern)
	case "range":
		filter.From, filter.To, ok = filterRange(options)
	default:
		ok = true
	}
	if !ok {
		b.partial(optionPath, "")
		return spec.ValueFilter{}, false, false
	}
	return filter, id == "notSubstring", true
}

func comparedFilterValue(v any) (any, bool) {
	if s, isString := v.(string); isString {
		return s, utf8.RuneCountInString(s) <= maxPattern
	}
	f, ok := asFloat(v)
	return f, ok
}

func filterRange(options map[string]any) (from, to *float64, ok bool) {
	bounds := map[string]**float64{"from": &from, "to": &to}
	for key, dst := range bounds {
		v := options[key]
		if v == nil || v == "" {
			continue
		}
		f, valid := numberValue(v)
		if !valid {
			return nil, nil, false
		}
		*dst = &f
	}
	return from, to, from != nil || to != nil
}

func filterByRefIDTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "filterByRefId"}
	ids := b.includedRefIDs(asString(opts["include"]), path+".options.include")
	if present(opts["exclude"]) {
		if !present(opts["include"]) {
			ids = slices.Sorted(maps.Keys(b.refs))
		}
		excluded, ok := b.excludedRefIDs(asString(opts["exclude"]))
		if !ok {
			b.partial(path+".options.exclude", "")
			return b.dropTransform(path)
		}
		ids = slices.DeleteFunc(ids, func(id string) bool { return excluded[id] })
	}
	for _, id := range ids {
		t.Include = b.includeRef(t.Include, b.refs[id])
	}
	if len(t.Include) == 0 {
		return b.dropTransform(path)
	}
	return t, true
}

func (b *panelBuild) includedRefIDs(text, path string) []string {
	var ids []string
	for _, id := range strings.Split(text, "|") {
		id = strings.TrimSpace(id)
		_, mapped := b.refs[id]
		switch {
		case id == "":
		case !refPattern.MatchString(id) || !mapped:
			b.partial(path, "")
		case !slices.Contains(ids, id):
			ids = append(ids, id)
		}
	}
	return ids
}

func (b *panelBuild) excludedRefIDs(text string) (map[string]bool, bool) {
	excluded := map[string]bool{}
	if last := strings.LastIndex(text, "/"); strings.HasPrefix(text, "/") && last > 0 {
		re, err := refIDRegexp(text[1:last], text[last+1:])
		if err != nil {
			return nil, false
		}
		for id := range b.refs {
			excluded[id] = re.MatchString(id)
		}
		return excluded, true
	}
	for id := range strings.SplitSeq(text, "|") {
		excluded[strings.TrimSpace(id)] = true
	}
	return excluded, true
}

func (b *panelBuild) includeRef(include []string, ref string) []string {
	for _, r := range []string{ref, b.instantRefs[ref]} {
		if r != "" && !slices.Contains(include, r) && len(include) < maxRefFilters {
			include = append(include, r)
		}
	}
	return include
}

func extractFieldsTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	field, ok := nameOption(opts, "source")
	if !ok {
		return b.dropTransform(path)
	}
	t := spec.Transform{Type: "extractFields", Field: field}
	switch format := cmp.Or(asString(opts["format"]), "auto"); {
	case format == "auto":
	case slices.Contains(extractFormats, format):
		t.Format = format
	default:
		b.partial(path+".options.format", "")
		return spec.Transform{}, false
	}
	t.Replace = truthy(opts["replace"])
	t.KeepTime = truthy(opts["keepTime"])
	t.Paths = b.fieldPaths(opts["jsonPaths"], path+".options.jsonPaths")
	if delimiter := asString(opts["delimiter"]); delimiter != "" {
		if utf8.RuneCountInString(delimiter) > maxDelimiter {
			b.partial(path+".options.delimiter", "")
		} else {
			t.Delimiter = delimiter
		}
	}
	if regex := asString(opts["regExp"]); regex != "" {
		if !validLength(regex, maxPattern) {
			b.partial(path+".options.regExp", "")
		} else {
			t.Regex = regex
		}
	}
	return t, true
}

func (b *panelBuild) fieldPaths(v any, path string) []spec.FieldPath {
	items, _ := asArray(v)
	var out []spec.FieldPath
	for i, item := range items {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		obj, _ := asObject(item)
		jsonPath, alias := asString(obj["path"]), asString(obj["alias"])
		switch {
		case len(out) >= maxFieldPaths:
			b.partial(itemPath, fmt.Sprintf("A transformation keeps at most %d paths.", maxFieldPaths))
		case !validLength(jsonPath, maxPattern) || (alias != "" && !validLength(alias, maxPattern)):
			b.partial(itemPath, "")
		default:
			b.reportOtherKeys(obj, []string{"path", "alias"}, itemPath)
			out = append(out, spec.FieldPath{Path: jsonPath, Alias: alias})
		}
	}
	return out
}
