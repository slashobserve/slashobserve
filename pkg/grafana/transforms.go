package grafana

import (
	"cmp"
	"fmt"
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

var exprPattern = regexp.MustCompile(`^\s*\S+\s*[-+*/]\s*\S+\s*$`)

type transformBuilder func(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool)

type transformSpec struct {
	handled    []string
	allOptions bool
	build      transformBuilder
}

var transformSpecs = map[string]transformSpec{
	"merge":              {build: simpleTransform("merge")},
	"seriesToRows":       {build: simpleTransform("seriesToRows")},
	"organize":           {handled: []string{"excludeByName", "renameByName", "indexByName"}, build: organizeTransform},
	"reduce":             {handled: []string{"reducers", "mode", "includeTimeField", "labelsToFields", "fields"}, build: reduceTransform},
	"joinByField":        {handled: []string{"byField", "mode"}, build: joinTransform},
	"seriesToColumns":    {handled: []string{"byField", "mode"}, build: joinTransform},
	"filterFieldsByName": {handled: []string{"include"}, build: filterFieldsTransform},
	"calculateField":     {handled: []string{"mode", "alias", "binary", "reduce", "replaceFields"}, build: calculateTransform},
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
	"transpose":          {handled: []string{"firstFieldName", "restFieldsName", "emptyValue"}, build: transposeTransform},
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
	handled := entry.handled
	if entry.allOptions {
		handled = sortedKeys(opts)
	}
	b.transformExtras(t, opts, handled, path)
	return tr, ok
}

func (b *panelBuild) transformExtras(t, opts map[string]any, handled []string, path string) {
	if present(t["filter"]) {
		b.partial(path+".filter", "Transformations that apply to some queries only are not supported yet.")
	}
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

func organizeTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "organize"}
	exclude, _ := asObject(opts["excludeByName"])
	var excluded []string
	for _, name := range sortedKeys(exclude) {
		if truthy(exclude[name]) {
			excluded = append(excluded, name)
		}
	}
	t.Exclude = clippedNames(b, excluded, path+".options.excludeByName")
	t.Rename = renames(b, opts["renameByName"], path+".options.renameByName")
	t.Order = fieldOrder(b, opts["indexByName"], path+".options.indexByName")
	return t, len(t.Exclude) > 0 || len(t.Rename) > 0 || len(t.Order) > 0
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
	var out []string
	unsupported := false
	for _, item := range stringList(v) {
		switch {
		case !isReducer(item):
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
	for _, key := range []string{"includeTimeField", "labelsToFields", "fields"} {
		if present(opts[key]) {
			b.partial(path+".options."+key, "")
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
	case "outer", "inner":
		t.Mode = mode
	case "":
	default:
		b.partial(path+".options.mode", "")
	}
	return t, true
}

func filterFieldsTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	include, _ := asObject(opts["include"])
	t := spec.Transform{Type: "filterFieldsByName"}
	t.Include = clippedNames(b, stringList(include["names"]), path+".options.include.names")
	if pattern := asString(include["pattern"]); validLength(pattern, maxRegexMatch) {
		t.IncludeRegex = pattern
	}
	for _, key := range sortedKeys(include) {
		if key != "names" && key != "pattern" && present(include[key]) {
			b.partial(path+".options.include."+key, "")
		}
	}
	return t, len(t.Include) > 0 || t.IncludeRegex != ""
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
	switch mode {
	case "binary":
		expr, ok := binaryExpression(opts)
		if !ok {
			b.partial(path+".options.binary", "")
			return t, false
		}
		t.Expr, fallback = expr, expr
	case "reduceRow":
		reduce, _ := asObject(opts["reduce"])
		reducer := cmp.Or(asString(reduce["reducer"]), "sum")
		if !isReducer(reducer) {
			b.partial(path+".options.reduce", "")
			return t, false
		}
		t.ReduceRow, fallback = reducer, reducerNames[reducer]
		if present(reduce["include"]) {
			b.partial(path+".options.reduce.include", "")
		}
	default:
		b.partial(path+".options.mode", "")
		return t, false
	}
	t.Alias, _ = cutRunes(cmp.Or(asString(opts["alias"]), fallback), maxNameMatch)
	if truthy(opts["replaceFields"]) {
		b.partial(path+".options.replaceFields", "")
	}
	return t, true
}

func binaryExpression(opts map[string]any) (string, bool) {
	binary, _ := asObject(opts["binary"])
	operator := asString(binary["operator"])
	if !slices.Contains([]string{"+", "-", "*", "/"}, operator) {
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
	first, _ := asObject(entries[0])
	field := asString(first["field"])
	if !validLength(field, maxNameMatch) {
		b.partial(path+".options.sort", "")
		return spec.Transform{}, false
	}
	if len(entries) > 1 {
		b.partial(path+".options.sort[1]", "")
	}
	return spec.Transform{Type: "sortBy", Field: field, Desc: truthy(first["desc"])}, true
}

func limitTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	count, ok := numberValue(opts["limitField"])
	if !ok || count != math.Trunc(count) || count < 1 || count > maxLimit {
		b.partial(path+".options.limitField", "")
		return spec.Transform{}, false
	}
	return spec.Transform{Type: "limit", Count: int(count)}, true
}
