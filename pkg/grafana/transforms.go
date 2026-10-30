package grafana

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxTransforms     = 20
	maxTransformNames = 100
	maxExprLength     = 400
	maxLimit          = 100000
	maxReducers       = 10
	maxWindowSize     = 10000
	maxSortThen       = 10

	defaultWindowShare = 0.1
)

var reducerNames = map[string]string{
	"last": "Last", "lastNotNull": "Last *", "first": "First", "firstNotNull": "First *",
	"min": "Min", "max": "Max", "mean": "Mean", "median": "Median", "sum": "Total",
	"count": "Count", "range": "Range", "delta": "Delta", "diff": "Difference",
	"stdDev": "StdDev", "distinctCount": "Distinct Count", "changeCount": "Change Count",
}

func isReducer(id string) bool {
	_, ok := reducerNames[id]
	return ok
}

func isAggregateReducer(id string) bool {
	return id == uniqueValuesReducer || isReducer(id)
}

var exprPattern = regexp.MustCompile(`^\s*\S+\s*[-+*/%^]\s*\S+\s*$`)

var (
	unaryFunctions   = []string{"abs", "exp", "ln", "floor", "ceil", "round"}
	windowFunctions  = []string{"mean", "stdDev", "variance"}
	windowAlignments = []string{"trailing", "centered"}
	binaryOperators  = []string{"+", "-", "*", "/", "%", "^"}
	varReference     = regexp.MustCompile(`^\$(?:([A-Za-z][A-Za-z0-9_]{0,39})|\{([A-Za-z][A-Za-z0-9_]{0,39})\})$`)
)

type transformBuilder func(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool)

type transformSpec struct {
	handled    []string
	allOptions bool
	build      transformBuilder
}

var transformSpecs = map[string]transformSpec{
	"merge":              {build: simpleTransform("merge")},
	"seriesToRows":       {build: simpleTransform("seriesToRows")},
	"organize":           {handled: []string{"excludeByName", "includeByName", "renameByName", "indexByName"}, build: organizeTransform},
	"rename":             {handled: []string{"renameByName"}, build: renameTransform},
	"order":              {handled: []string{"indexByName"}, build: orderTransform},
	"filterFields":       {handled: []string{"include", "exclude"}, build: filterFieldsMatcherTransform},
	"reduce":             {handled: []string{"reducers", "mode", "includeTimeField", "labelsToFields", "fields"}, build: reduceTransform},
	"joinByField":        {handled: []string{"byField", "mode"}, build: joinTransform},
	"seriesToColumns":    {handled: []string{"byField", "mode"}, build: joinTransform},
	"filterFieldsByName": {handled: []string{"include", "exclude", "byVariable"}, build: filterFieldsTransform},
	"calculateField":     {handled: []string{"mode", "alias", "binary", "reduce", "unary", "index", "cumulative", "window", "replaceFields"}, build: calculateTransform},
	"sortBy":             {handled: []string{"sort"}, build: sortTransform},
	"limit":              {handled: []string{"limitField"}, build: limitTransform},
	"convertFieldType":   {handled: []string{"conversions", "fields"}, build: convertFieldTypeTransform},
	"formatString":       {handled: []string{"stringField", "outputFormat", "substringStart", "substringEnd"}, build: formatStringTransform},
	"formatTime":         {handled: []string{"timeField", "outputFormat", "useTimezone"}, build: formatTimeTransform},
	"renameByRegex":      {handled: []string{"regex", "renamePattern"}, build: renameByRegexTransform},
	"filterByValue":      {handled: []string{"type", "match", "filters"}, build: filterByValueTransform},
	"filterByRefId":      {handled: []string{"include"}, build: filterByRefIDTransform},
	"extractFields":      {handled: []string{"source", "format", "replace", "keepTime", "jsonPaths", "delimiter", "regExp"}, build: extractFieldsTransform},
	"labelsToFields":     {handled: []string{"mode", "keepLabels", "valueLabel"}, build: labelsToFieldsTransform},
	"joinByLabels":       {handled: []string{"value", "join"}, build: joinByLabelsTransform},
	"partitionByValues":  {handled: []string{"fields", "keepFields", "naming"}, build: partitionByValuesTransform},
	"prepareTimeSeries":  {handled: []string{"format"}, build: prepareTimeSeriesTransform},
	"timeSeriesTable":    {allOptions: true, build: timeSeriesTableTransform},
	"rowsToFields":       {handled: []string{"mappings"}, build: rowsToFieldsTransform},
	"configFromData":     {handled: []string{"configRefId", "mappings", "applyTo"}, build: configFromDataTransform},
	"concatenate":        {handled: []string{"frameNameMode", "frameNameLabel"}, build: concatenateTransform},
	"groupBy":            {handled: []string{"fields"}, build: groupByTransform},
	"groupingToMatrix":   {handled: []string{"columnField", "rowField", "valueField", "emptyValue"}, build: groupingToMatrixTransform},
	"groupToNestedTable": {handled: []string{"fields", "showSubframeHeaders"}, build: groupToNestedTableTransform},
	"histogram":          {handled: []string{"bucketCount", "bucketSize", "bucketOffset", "combine"}, build: histogramTransform},
	"regression":         {handled: []string{"modelType", "degree", "xFieldName", "yFieldName", "predictionCount"}, build: regressionTransform},
	"heatmap":            {handled: []string{"xBuckets", "yBuckets", "keepOriginalData"}, build: heatmapTransform},
	"transpose":          {handled: []string{"firstFieldName", "restFieldsName", "emptyValue"}, build: transposeTransform},
	"spatial":            {handled: []string{"action", "source", "calculate", "modify"}, build: spatialTransform},
	"fieldLookup":        {handled: []string{"lookupField", "gazetteer"}, build: fieldLookupTransform},
}

func (b *panelBuild) convertTransforms() {
	items, _ := asArray(b.raw["transformations"])
	for i, item := range items {
		path := fmt.Sprintf("transformations[%d]", i)
		b.consume(path)
		t, ok := asObject(item)
		if !ok || truthy(t["disabled"]) {
			continue
		}
		tr, ok := b.convertTransform(t, path)
		if !ok {
			continue
		}
		if len(b.panel.Transform) >= maxTransforms {
			b.partial(path, "A panel holds at most 20 transformations.")
			continue
		}
		b.panel.Transform = append(b.panel.Transform, tr)
	}
}

func (b *panelBuild) convertTransform(t map[string]any, path string) (spec.Transform, bool) {
	entry, known := transformSpecs[asString(t["id"])]
	if !known {
		b.partial(path, "")
		return spec.Transform{}, false
	}
	opts, _ := asObject(t["options"])
	tr, ok := entry.build(b, opts, path)
	if ok {
		tr.Queries = b.transformQueries(t, path)
	}
	handled := entry.handled
	if entry.allOptions {
		handled = sortedKeys(opts)
	}
	b.transformExtras(t, opts, handled, path)
	return tr, ok
}

func (b *panelBuild) transformExtras(t, opts map[string]any, handled []string, path string) {
	if topic := asString(t["topic"]); topic != "" && topic != "series" {
		b.partial(path+".topic", "Transformations of annotations are not supported yet.")
	}
	for _, key := range sortedKeys(opts) {
		if !slices.Contains(handled, key) && present(opts[key]) {
			b.partial(path+".options."+key, "")
		}
	}
}

func simpleTransform(kind string) transformBuilder {
	return func(*panelBuild, map[string]any, string) (spec.Transform, bool) {
		return spec.Transform{Type: kind}, true
	}
}

func clippedNames(b *panelBuild, names []string, path string) []string {
	var out []string
	for _, name := range names {
		if !validLength(name, maxNameMatch) || slices.Contains(out, name) {
			continue
		}
		if len(out) >= maxTransformNames {
			b.partial(path, fmt.Sprintf("A transformation keeps at most %d names.", maxTransformNames))
			break
		}
		out = append(out, name)
	}
	return out
}

func (b *panelBuild) transformQueries(t map[string]any, path string) []string {
	if !present(t["filter"]) {
		return nil
	}
	filter, _ := asObject(t["filter"])
	if asString(filter["id"]) != "byRefId" {
		b.partial(path+".filter", "")
		return nil
	}
	ids, complete := b.filterRefIDs(filter["options"])
	var queries []string
	for _, id := range ids {
		queries = b.includeRef(queries, b.refs[id])
	}
	if !complete || len(queries) == 0 {
		b.partial(path+".filter", "")
	}
	return queries
}

func (b *panelBuild) filterRefIDs(options any) ([]string, bool) {
	if text, ok := options.(string); ok {
		if last := strings.LastIndex(text, "/"); strings.HasPrefix(text, "/") && last > 0 {
			return b.refIDsMatching(text[1:last], text[last+1:])
		}
		return b.mappedRefIDs(strings.Split(text, "|"))
	}
	return b.mappedRefIDs(stringList(options))
}

func (b *panelBuild) mappedRefIDs(candidates []string) ([]string, bool) {
	var ids []string
	complete := len(candidates) > 0
	for _, id := range candidates {
		id = strings.TrimSpace(id)
		if _, mapped := b.refs[id]; !mapped || !refPattern.MatchString(id) {
			complete = false
			continue
		}
		ids = append(ids, id)
	}
	return ids, complete
}

func (b *panelBuild) refIDsMatching(pattern, flags string) ([]string, bool) {
	if strings.Contains(flags, "i") {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, false
	}
	var ids []string
	for _, id := range slices.Sorted(maps.Keys(b.refs)) {
		if re.MatchString(id) {
			ids = append(ids, id)
		}
	}
	return ids, len(ids) > 0
}

func truthyNames(b *panelBuild, v any, path string) []string {
	flags, _ := asObject(v)
	var names []string
	for _, name := range sortedKeys(flags) {
		if truthy(flags[name]) {
			names = append(names, name)
		}
	}
	return clippedNames(b, names, path)
}

func organizeTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "organize"}
	t.Include = truthyNames(b, opts["includeByName"], path+".options.includeByName")
	t.Exclude = truthyNames(b, opts["excludeByName"], path+".options.excludeByName")
	t.Rename = renames(b, opts["renameByName"], path+".options.renameByName")
	t.Order = fieldOrder(b, opts["indexByName"], path+".options.indexByName")
	return t, len(t.Include) > 0 || len(t.Exclude) > 0 || len(t.Rename) > 0 || len(t.Order) > 0
}

func renameTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "organize", Rename: renames(b, opts["renameByName"], path+".options.renameByName")}
	return t, len(t.Rename) > 0
}

func orderTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "organize", Order: fieldOrder(b, opts["indexByName"], path+".options.indexByName")}
	return t, len(t.Order) > 0
}

func matcherNames(b *panelBuild, v any, path string) (names []string, pattern string, ok bool) {
	matcher, isObject := asObject(v)
	if !isObject {
		return nil, "", false
	}
	switch asString(matcher["id"]) {
	case "byName":
		names = []string{asString(matcher["options"])}
	case "byNames":
		if options, isObject := asObject(matcher["options"]); isObject {
			names = stringList(options["names"])
		} else {
			names = stringList(matcher["options"])
		}
	case "byRegexp":
		pattern = asString(matcher["options"])
		if !validLength(pattern, maxRegexMatch) {
			return nil, "", false
		}
	default:
		return nil, "", false
	}
	names = clippedNames(b, names, path)
	return names, pattern, len(names) > 0 || pattern != ""
}

func filterFieldsMatcherTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "filterFieldsByName"}
	for _, side := range []struct {
		key     string
		names   *[]string
		pattern *string
	}{
		{"include", &t.Include, &t.IncludeRegex},
		{"exclude", &t.Exclude, &t.ExcludeRegex},
	} {
		if !present(opts[side.key]) {
			continue
		}
		names, pattern, ok := matcherNames(b, opts[side.key], path+".options."+side.key)
		if !ok {
			b.partial(path+".options."+side.key, "")
			continue
		}
		*side.names, *side.pattern = names, pattern
	}
	return t, len(t.Include) > 0 || t.IncludeRegex != "" || len(t.Exclude) > 0 || t.ExcludeRegex != ""
}

func renames(b *panelBuild, v any, path string) map[string]string {
	renameByName, _ := asObject(v)
	out := map[string]string{}
	for _, name := range sortedKeys(renameByName) {
		to := asString(renameByName[name])
		if to == "" || !validLength(to, maxNameMatch) {
			continue
		}
		if len(out) >= maxTransformNames {
			b.partial(path, fmt.Sprintf("A transformation keeps at most %d names.", maxTransformNames))
			break
		}
		out[name] = to
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func fieldOrder(b *panelBuild, v any, path string) []string {
	indexByName, _ := asObject(v)
	names := sortedKeys(indexByName)
	index := func(name string) float64 {
		f, ok := asFloat(indexByName[name])
		if !ok {
			return math.MaxFloat64
		}
		return f
	}
	slices.SortStableFunc(names, func(a, c string) int { return cmp.Compare(index(a), index(c)) })
	return clippedNames(b, names, path)
}

func reducerList(b *panelBuild, v any, path string) []string {
	return acceptedReducers(b, v, path, isReducer)
}

func acceptedReducers(b *panelBuild, v any, path string, accepted func(string) bool) []string {
	var out []string
	unsupported := false
	for _, item := range stringList(v) {
		switch {
		case !accepted(item):
			unsupported = true
		case !slices.Contains(out, item) && len(out) < maxReducers:
			out = append(out, item)
		}
	}
	if unsupported {
		b.partial(path, "Some reducers are not supported yet.")
	}
	return out
}

func reduceTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	reducers := reducerList(b, opts["reducers"], path+".options.reducers")
	if len(reducers) == 0 {
		b.partial(path, "")
		return spec.Transform{}, false
	}
	t := spec.Transform{Type: "reduce", Reducers: reducers}
	switch mode := asString(opts["mode"]); mode {
	case "seriesToRows", "reduceFields":
		t.Mode = mode
	case "":
	default:
		b.partial(path+".options.mode", "")
	}
	t.IncludeTime = truthy(opts["includeTimeField"])
	t.LabelsToFields = truthy(opts["labelsToFields"])
	if present(opts["fields"]) {
		names, pattern, ok := matcherNames(b, opts["fields"], path+".options.fields")
		if ok {
			t.Fields, t.FieldsRegex = names, pattern
		} else {
			b.partial(path+".options.fields", "")
		}
	}
	return t, true
}

func joinTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "joinByField"}
	if field := asString(opts["byField"]); validLength(field, maxNameMatch) {
		t.Field = field
	}
	switch mode := asString(opts["mode"]); mode {
	case "outer", "inner", "outerTabular":
		t.Mode = mode
	case "":
	default:
		b.partial(path+".options.mode", "")
	}
	return t, true
}

func variableReferenceName(text string) string {
	m := varReference.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return ""
	}
	return cmp.Or(m[1], m[2])
}

func (b *panelBuild) usableVariable(name string) bool {
	return name != "" && !b.c.droppedVars[name]
}

func filterFieldsTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	include, _ := asObject(opts["include"])
	exclude, _ := asObject(opts["exclude"])
	t := spec.Transform{Type: "filterFieldsByName"}
	includePath := path + ".options.include"
	name := asString(include["variable"])
	switch {
	case truthy(opts["byVariable"]):
		reference := cmp.Or(variableReferenceName(name), name)
		if variableNamePattern.MatchString(reference) && b.usableVariable(reference) {
			t.Variable = reference
		} else {
			b.partial(path+".options.byVariable", "")
		}
	default:
		t.Include = clippedNames(b, stringList(include["names"]), includePath+".names")
		if pattern := asString(include["pattern"]); validLength(pattern, maxRegexMatch) {
			t.IncludeRegex = pattern
		}
		if name != "" {
			b.partial(includePath+".variable", "")
		}
	}
	t.Exclude = clippedNames(b, stringList(exclude["names"]), path+".options.exclude.names")
	if pattern := asString(exclude["pattern"]); validLength(pattern, maxRegexMatch) {
		t.ExcludeRegex = pattern
	}
	b.reportOtherKeys(include, []string{"names", "pattern", "variable"}, includePath)
	b.reportOtherKeys(exclude, []string{"names", "pattern"}, path+".options.exclude")
	return t, len(t.Include) > 0 || t.IncludeRegex != "" || len(t.Exclude) > 0 || t.ExcludeRegex != "" || t.Variable != ""
}

func operandText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		if matcher, ok := asObject(x["matcher"]); ok {
			return operandText(matcher["options"])
		}
		return operandText(x["fixed"])
	}
	if f, ok := asFloat(v); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return ""
}

func calculateTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "calculateField"}
	mode := cmp.Or(asString(opts["mode"]), "reduceRow")
	var fallback string
	var ok bool
	switch mode {
	case "binary":
		fallback, ok = binaryMode(b, &t, opts, path)
	case "reduceRow":
		fallback, ok = reduceRowMode(b, &t, opts, path)
	case "unary":
		fallback, ok = unaryMode(b, &t, opts, path)
	case "index":
		fallback, ok = indexMode(b, &t, opts, path)
	case "cumulativeFunctions":
		fallback, ok = cumulativeMode(b, &t, opts, path)
	case "windowFunctions":
		fallback, ok = windowMode(b, &t, opts, path)
	default:
		b.partial(path+".options.mode", "")
	}
	if !ok {
		return t, false
	}
	t.Alias, _ = cutRunes(cmp.Or(asString(opts["alias"]), fallback), maxNameMatch)
	t.Replace = truthy(opts["replaceFields"])
	return t, true
}

func binaryMode(b *panelBuild, t *spec.Transform, opts map[string]any, path string) (string, bool) {
	expr, ok := binaryExpression(opts)
	if !ok {
		b.partial(path+".options.binary", "")
		return "", false
	}
	t.Expr = expr
	return expr, true
}

func reduceRowMode(b *panelBuild, t *spec.Transform, opts map[string]any, path string) (string, bool) {
	reduce, _ := asObject(opts["reduce"])
	reducer := cmp.Or(asString(reduce["reducer"]), "sum")
	if !isReducer(reducer) {
		b.partial(path+".options.reduce", "")
		return "", false
	}
	t.ReduceRow = reducer
	if present(reduce["include"]) {
		b.partial(path+".options.reduce.include", "")
	}
	return reducerNames[reducer], true
}

func (b *panelBuild) optionalFieldName(options map[string]any, key, path string) (string, bool) {
	name := asString(options[key])
	if name != "" && !validLength(name, maxNameMatch) {
		b.partial(path+"."+key, "")
		return "", false
	}
	return name, true
}

func unaryMode(b *panelBuild, t *spec.Transform, opts map[string]any, path string) (string, bool) {
	unary, _ := asObject(opts["unary"])
	unaryPath := path + ".options.unary"
	fn := cmp.Or(asString(unary["operator"]), "abs")
	if !slices.Contains(unaryFunctions, fn) {
		b.partial(unaryPath+".operator", "")
		return "", false
	}
	field, ok := b.optionalFieldName(unary, "fieldName", unaryPath)
	if !ok {
		return "", false
	}
	b.reportOtherKeys(unary, []string{"operator", "fieldName"}, unaryPath)
	t.Unary = &spec.Unary{Field: field}
	if fn != "abs" {
		t.Unary.Fn = fn
	}
	if field == "" {
		return fn, true
	}
	return fn + "(" + field + ")", true
}

func indexMode(b *panelBuild, t *spec.Transform, opts map[string]any, path string) (string, bool) {
	index, _ := asObject(opts["index"])
	b.reportOtherKeys(index, []string{"asPercentile"}, path+".options.index")
	t.Index = &spec.RowIndex{Percentile: truthy(index["asPercentile"])}
	return "Row", true
}

func cumulativeMode(b *panelBuild, t *spec.Transform, opts map[string]any, path string) (string, bool) {
	cumulative, _ := asObject(opts["cumulative"])
	cumulativePath := path + ".options.cumulative"
	reducer := cmp.Or(asString(cumulative["reducer"]), "sum")
	if reducer != "sum" && reducer != "mean" {
		b.partial(cumulativePath+".reducer", "")
		return "", false
	}
	field, ok := b.optionalFieldName(cumulative, "field", cumulativePath)
	if !ok {
		return "", false
	}
	b.reportOtherKeys(cumulative, []string{"reducer", "field"}, cumulativePath)
	t.Cumulative = &spec.Cumulative{Field: field}
	if reducer == "mean" {
		t.Cumulative.Fn = reducer
		return "Cumulative mean", true
	}
	return "Cumulative total", true
}

func windowMode(b *panelBuild, t *spec.Transform, opts map[string]any, path string) (string, bool) {
	window, _ := asObject(opts["window"])
	windowPath := path + ".options.window"
	fn := cmp.Or(asString(window["reducer"]), "mean")
	if !slices.Contains(windowFunctions, fn) {
		b.partial(windowPath+".reducer", "")
		return "", false
	}
	align := cmp.Or(asString(window["windowAlignment"]), "trailing")
	if !slices.Contains(windowAlignments, align) {
		b.partial(windowPath+".windowAlignment", "")
		return "", false
	}
	field, ok := b.optionalFieldName(window, "field", windowPath)
	if !ok {
		return "", false
	}
	w := &spec.Window{Field: field}
	if !windowSize(b, w, window, windowPath) {
		return "", false
	}
	b.reportOtherKeys(window, []string{"reducer", "field", "windowSize", "windowSizeMode", "windowAlignment"}, windowPath)
	if fn != "mean" {
		w.Fn = fn
	}
	if align != "trailing" {
		w.Align = align
	}
	t.Window = w
	return strings.ToUpper(align[:1]) + align[1:] + " " + fn, true
}

func windowSize(b *panelBuild, w *spec.Window, window map[string]any, path string) bool {
	mode := asString(window["windowSizeMode"])
	size, given := numberValue(window["windowSize"])
	fixed := mode == "fixed" || (mode == "" && size > 1)
	switch {
	case !slices.Contains([]string{"", "fixed", "percentage"}, mode):
		b.partial(path+".windowSizeMode", "")
		return false
	case !given && mode == "fixed":
		b.partial(path+".windowSizeMode", "")
		return false
	case !given:
		return true
	case fixed && size == math.Trunc(size) && size >= 1 && size <= maxWindowSize:
		w.Size = int(size)
	case !fixed && size > 0 && size <= 1:
		if size != defaultWindowShare {
			w.Share = size
		}
	default:
		b.partial(path+".windowSize", "")
		return false
	}
	return true
}

func binaryExpression(opts map[string]any) (string, bool) {
	binary, _ := asObject(opts["binary"])
	operator := asString(binary["operator"])
	if !slices.Contains(binaryOperators, operator) {
		return "", false
	}
	expr := operandText(binary["left"]) + " " + operator + " " + operandText(binary["right"])
	return expr, len(expr) <= maxExprLength && exprPattern.MatchString(expr) &&
		len(strings.Fields(expr)) == 3
}

func sortTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	entries, _ := asArray(opts["sort"])
	if len(entries) == 0 {
		return spec.Transform{}, false
	}
	var keys []spec.SortKey
	for i, entry := range entries {
		item, _ := asObject(entry)
		field := asString(item["field"])
		if !validLength(field, maxNameMatch) || len(keys) > maxSortThen {
			b.partial(fmt.Sprintf("%s.options.sort[%d]", path, i), "")
			break
		}
		keys = append(keys, spec.SortKey{Field: field, Desc: truthy(item["desc"])})
	}
	if len(keys) == 0 {
		return spec.Transform{}, false
	}
	t := spec.Transform{Type: "sortBy", Field: keys[0].Field, Desc: keys[0].Desc}
	if len(keys) > 1 {
		t.Then = keys[1:]
	}
	return t, true
}

func limitTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	if name := variableReferenceName(asString(opts["limitField"])); b.usableVariable(name) {
		return spec.Transform{Type: "limit", Count: "${" + name + "}"}, true
	}
	count, ok := numberValue(opts["limitField"])
	if !ok || count != math.Trunc(count) || count < 1 || count > maxLimit {
		b.partial(path+".options.limitField", "")
		return spec.Transform{}, false
	}
	return spec.Transform{Type: "limit", Count: int(count)}, true
}
