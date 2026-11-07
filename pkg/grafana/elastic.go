package grafana

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	luceneQueryPrefix    = "lucene:"
	luceneMatchAll       = "*"
	elasticUnlimitedSize = 500
	elasticDefaultWindow = 5
	movingAverageScript  = "MovingFunctions.unweightedAvg(values)"

	rawDocumentReason = "raw_document shows _source; it is converted as lines."
	noTimeFieldReason = "The date histogram names no field and the query has no time field."
)

var elasticRules = targetRules{silent: map[string]bool{"timeField": true, "isLogsQuery": true}}

var (
	elasticLineTypes     = map[string]bool{"raw_data": true, "raw_document": true, "logs": true}
	openSearchLineTypes  = map[string]bool{"Logs": true, "Raw Data": true, "Raw Document": true}
	elasticFieldMetrics  = map[string]bool{"avg": true, "sum": true, "min": true, "max": true, "cardinality": true, "percentiles": true, "extended_stats": true, "rate": true}
	elasticPipelines     = map[string]bool{"derivative": true, "cumulative_sum": true, "serial_diff": true}
	calendarInterval     = regexp.MustCompile(`^1(w|M|q|y)$`)
	aliasTerm            = regexp.MustCompile(`\{\{\s*term\s+([^}\s]+)\s*\}\}`)
	elasticMetricKeys    = []string{"id", "type", "field", "settings", "hide", "meta", "pipelineAgg", "pipelineVariables"}
	elasticBucketKeys    = []string{"id", "type", "field", "settings"}
	elasticDefaultPcts   = []float64{25, 50, 75, 95, 99}
	elasticMetricSetting = map[string][]string{
		"percentiles":    {"percents", "script"},
		"extended_stats": {"sigma", "script"},
		"cardinality":    {"precision_threshold", "script"},
		"rate":           {"unit", "mode", "script"},
		"moving_avg":     {"window", "model", "predict", "minimize", "settings"},
		"moving_fn":      {"window", "script", "shift"},
		"derivative":     {"unit"},
		"serial_diff":    {"lag"},
		"bucket_script":  {"script"},
	}
)

type elasticAgg = map[string]any

func (tb *targetBuild) elastic() (spec.Query, string) {
	tb.rules = elasticRules
	tb.use("query", "queryType", "luceneQueryType", "metrics")
	if asString(tb.t["queryType"]) == "PPL" {
		return spec.Query{}, decisionElasticPPL.reason
	}
	lucene := strings.TrimSpace(asString(tb.t["query"]))
	if lucene == "" {
		lucene = luceneMatchAll
	}
	metrics := objectList(tb.t["metrics"])
	if lines, rawDocument := tb.elasticLines(metrics); lines {
		if rawDocument != "" {
			tb.partial(rawDocument, rawDocumentReason)
		}
		tb.noEffect("alias", "bucketAggs")
		return spec.Query{Query: luceneQueryPrefix + lucene, Instant: true}, ""
	}
	if len(metrics) == 0 {
		metrics = []map[string]any{{"type": "count"}}
	}
	tb.use("bucketAggs")
	buckets := objectList(tb.t["bucketAggs"])
	if _, listed := tb.t["bucketAggs"]; !listed {
		buckets = []map[string]any{{"type": "date_histogram"}}
	}
	legend := tb.elasticLegend(metrics)
	if tb.elasticRangeCount(metrics, buckets) {
		return spec.Query{Query: luceneQueryPrefix + lucene, Legend: legend}, ""
	}
	body, reason := tb.elasticDSL(lucene, metrics, buckets)
	if reason != "" {
		return spec.Query{}, reason
	}
	return spec.Query{Query: body, Legend: legend}, ""
}

func objectList(v any) []map[string]any {
	items, _ := asArray(v)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if object, ok := asObject(item); ok {
			out = append(out, object)
		}
	}
	return out
}

func (tb *targetBuild) elasticLines(metrics []map[string]any) (lines bool, rawDocumentAt string) {
	queryType := asString(tb.t["queryType"])
	if elasticLineTypes[queryType] {
		return true, pathIf(queryType == "raw_document", "queryType")
	}
	if len(metrics) > 0 {
		if typ := asString(metrics[0]["type"]); elasticLineTypes[typ] {
			return true, pathIf(typ == "raw_document", "metrics[0].type")
		}
	}
	luceneType := asString(tb.t["luceneQueryType"])
	if openSearchLineTypes[luceneType] {
		return true, pathIf(luceneType == "Raw Document", "luceneQueryType")
	}
	return false, ""
}

func pathIf(when bool, path string) string {
	if when {
		return path
	}
	return ""
}

func (tb *targetBuild) elasticRangeCount(metrics, buckets []map[string]any) bool {
	if len(metrics) != 1 || asString(metrics[0]["type"]) != "count" || len(buckets) != 1 || asString(buckets[0]["type"]) != "date_histogram" {
		return false
	}
	field := asString(buckets[0]["field"])
	if field != "" && field != asString(tb.t["timeField"]) {
		return false
	}
	settings, _ := asObject(buckets[0]["settings"])
	for key, value := range settings {
		switch key {
		case "interval":
			if s := asString(value); s != "" && s != "auto" {
				return false
			}
		case "min_doc_count", "trimEdges":
			if n, ok := intSetting(value); ok && n != 0 {
				return false
			}
		default:
			if present(value) {
				return false
			}
		}
	}
	return true
}

func (tb *targetBuild) elasticLegend(metrics []map[string]any) string {
	tb.use("alias")
	alias := asString(tb.t["alias"])
	if alias == "" {
		return ""
	}
	legend := aliasTerm.ReplaceAllString(alias, "{{$1}}")
	legend = strings.ReplaceAll(legend, "{{field}}", asString(metrics[0]["field"]))
	cut, wasCut := cutRunes(legend, maxLegend)
	if wasCut {
		tb.partial("alias", "The legend was cut to 200 characters.")
	}
	return cut
}

func (tb *targetBuild) elasticDSL(lucene string, metrics, buckets []map[string]any) (string, string) {
	aggs, reason := tb.elasticMetricAggs(metrics)
	if reason != "" {
		return "", reason
	}
	for i := len(buckets) - 1; i >= 0; i-- {
		agg, reason := tb.elasticBucket(i, buckets[i])
		if reason != "" {
			return "", reason
		}
		if len(aggs) > 0 {
			agg["aggs"] = aggs
		}
		aggs = elasticAgg{aggName(i, buckets[i]): agg}
	}
	body := elasticAgg{"query": queryString(lucene)}
	if len(aggs) > 0 {
		body["aggs"] = aggs
	}
	return encodeDSL(body)
}

func queryString(lucene string) elasticAgg {
	return elasticAgg{"query_string": elasticAgg{"query": lucene, "analyze_wildcard": true}}
}

func encodeDSL(body elasticAgg) (string, string) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return "", "The query could not be written as Query DSL."
	}
	return strings.TrimSuffix(buf.String(), "\n"), ""
}

func aggName(i int, agg map[string]any) string {
	if id := asString(agg["id"]); id != "" {
		return id
	}
	return strconv.Itoa(i + 1)
}

func (tb *targetBuild) elasticBucket(i int, bucket map[string]any) (elasticAgg, string) {
	path := fmt.Sprintf("bucketAggs[%d]", i)
	tb.unknownKeys(path, bucket, elasticBucketKeys)
	field := asString(bucket["field"])
	settings, _ := asObject(bucket["settings"])
	switch typ := asString(bucket["type"]); typ {
	case "date_histogram":
		return tb.dateHistogram(path, field, settings)
	case "terms":
		tb.unknownSettings(path, settings, "size", "order", "orderBy", "min_doc_count", "missing")
		return elasticAgg{"terms": termsAgg(field, settings)}, ""
	case "filters":
		tb.unknownSettings(path, settings, "filters")
		return elasticAgg{"filters": elasticAgg{"filters": filtersAgg(settings)}}, ""
	case "histogram":
		tb.unknownSettings(path, settings, "interval", "min_doc_count")
		return elasticAgg{"histogram": histogramAgg(field, settings)}, ""
	case "geohash_grid", "nested":
		return nil, decisionElasticGeoNested.reason
	default:
		return nil, unsupportedValueReason("bucket aggregation", typ)
	}
}

func (tb *targetBuild) dateHistogram(path, field string, settings map[string]any) (elasticAgg, string) {
	if field == "" {
		field = asString(tb.t["timeField"])
	}
	if field == "" {
		return nil, noTimeFieldReason
	}
	tb.unknownSettings(path, settings, "interval", "min_doc_count", "trimEdges", "offset", "timeZone")
	agg := elasticAgg{
		"field":           field,
		"min_doc_count":   intSettingOr(settings["min_doc_count"], 0),
		"extended_bounds": elasticAgg{"min": "$__from", "max": "$__to"},
	}
	switch interval := asString(settings["interval"]); {
	case interval == "" || interval == "auto":
		agg["fixed_interval"] = "$__interval"
	case calendarInterval.MatchString(interval):
		agg["calendar_interval"] = interval
	default:
		agg["fixed_interval"] = interval
	}
	if offset := asString(settings["offset"]); offset != "" {
		agg["offset"] = offset
	}
	if zone := asString(settings["timeZone"]); zone != "" {
		agg["time_zone"] = zone
	}
	if n, ok := intSetting(settings["trimEdges"]); ok && n != 0 {
		tb.partial(path+".settings.trimEdges", decisionElasticTrimEdges.reason)
	}
	return elasticAgg{"date_histogram": agg}, ""
}

func termsAgg(field string, settings map[string]any) elasticAgg {
	orderBy := asString(settings["orderBy"])
	switch orderBy {
	case "", "_count":
		orderBy = "_count"
	case "_term", "_key":
		orderBy = "_key"
	}
	order := asString(settings["order"])
	if order == "" {
		order = "desc"
	}
	agg := elasticAgg{"field": field, "order": elasticAgg{orderBy: order}}
	if size, ok := intSetting(settings["size"]); ok {
		if size <= 0 {
			size = elasticUnlimitedSize
		}
		agg["size"] = size
	}
	if n, ok := intSetting(settings["min_doc_count"]); ok {
		agg["min_doc_count"] = n
	}
	if missing := asString(settings["missing"]); missing != "" {
		agg["missing"] = missing
	}
	return agg
}

func filtersAgg(settings map[string]any) elasticAgg {
	out := elasticAgg{}
	for _, filter := range objectList(settings["filters"]) {
		query := strings.TrimSpace(asString(filter["query"]))
		if query == "" {
			query = luceneMatchAll
		}
		name := asString(filter["label"])
		if name == "" {
			name = query
		}
		out[name] = queryString(query)
	}
	return out
}

func histogramAgg(field string, settings map[string]any) elasticAgg {
	agg := elasticAgg{"field": field, "interval": intSettingOr(settings["interval"], 1000)}
	if n, ok := intSetting(settings["min_doc_count"]); ok {
		agg["min_doc_count"] = n
	}
	return agg
}

func (tb *targetBuild) elasticMetricAggs(metrics []map[string]any) (elasticAgg, string) {
	aggs := elasticAgg{}
	kept, dropped := 0, ""
	for i, metric := range metrics {
		path := fmt.Sprintf("metrics[%d]", i)
		tb.unknownKeys(path, metric, elasticMetricKeys)
		if truthy(metric["hide"]) {
			tb.partial(path+".hide", decisionElasticHiddenMetrics.reason)
		}
		agg, at, reason := tb.elasticMetric(path, metric, metrics)
		switch {
		case reason != "":
			tb.partial(path+"."+at, reason)
			dropped = reason
		case agg == nil:
			kept++
		default:
			aggs[aggName(i, metric)] = agg
			kept++
		}
	}
	if kept == 0 && dropped != "" {
		return nil, dropped
	}
	return aggs, ""
}

func (tb *targetBuild) elasticMetric(path string, metric map[string]any, all []map[string]any) (agg elasticAgg, at, reason string) {
	typ := asString(metric["type"])
	field := asString(metric["field"])
	settings, _ := asObject(metric["settings"])
	switch {
	case typ == "top_metrics":
		return nil, "type", decisionElasticTopMetrics.reason
	case elasticFieldMetrics[typ] && scriptText(settings["script"]) != "":
		return nil, "settings.script", decisionElasticScripts.reason
	}
	tb.unknownSettings(path, settings, elasticMetricSetting[typ]...)
	switch {
	case typ == "count":
		return nil, "", ""
	case elasticFieldMetrics[typ]:
		return elasticAgg{typ: fieldMetric(typ, field, settings)}, "", ""
	case typ == "moving_avg":
		return elasticAgg{"moving_fn": elasticAgg{
			"buckets_path": bucketsPath(all, asString(metric["pipelineAgg"])),
			"window":       intSettingOr(settings["window"], elasticDefaultWindow),
			"script":       movingAverageScript,
		}}, "", ""
	case typ == "moving_fn":
		return elasticAgg{"moving_fn": elasticAgg{
			"buckets_path": bucketsPath(all, asString(metric["pipelineAgg"])),
			"window":       intSettingOr(settings["window"], elasticDefaultWindow),
			"script":       scriptText(settings["script"]),
		}}, "", ""
	case elasticPipelines[typ]:
		agg := elasticAgg{"buckets_path": bucketsPath(all, asString(metric["pipelineAgg"]))}
		if unit := asString(settings["unit"]); unit != "" {
			agg["unit"] = unit
		}
		if lag, ok := intSetting(settings["lag"]); ok {
			agg["lag"] = lag
		}
		return elasticAgg{typ: agg}, "", ""
	case typ == "bucket_script":
		paths := elasticAgg{}
		for _, variable := range objectList(metric["pipelineVariables"]) {
			paths[asString(variable["name"])] = bucketsPath(all, asString(variable["pipelineAgg"]))
		}
		return elasticAgg{"bucket_script": elasticAgg{"buckets_path": paths, "script": scriptText(settings["script"])}}, "", ""
	}
	return nil, "type", unsupportedValueReason("metric", typ)
}

func fieldMetric(typ, field string, settings map[string]any) elasticAgg {
	agg := elasticAgg{"field": field}
	switch typ {
	case "percentiles":
		agg["percents"] = floatSettings(settings["percents"], elasticDefaultPcts)
	case "extended_stats":
		if sigma, ok := asFloat(settingNumber(settings["sigma"])); ok {
			agg["sigma"] = sigma
		}
	case "cardinality":
		if n, ok := intSetting(settings["precision_threshold"]); ok {
			agg["precision_threshold"] = n
		}
	case "rate":
		if unit := asString(settings["unit"]); unit != "" {
			agg["unit"] = unit
		}
		if mode := asString(settings["mode"]); mode != "" {
			agg["mode"] = mode
		}
	}
	return agg
}

func bucketsPath(all []map[string]any, id string) string {
	for _, metric := range all {
		if asString(metric["id"]) == id && asString(metric["type"]) == "count" {
			return "_count"
		}
	}
	return id
}

func scriptText(v any) string {
	if object, ok := asObject(v); ok {
		if inline := asString(object["inline"]); inline != "" {
			return inline
		}
		return asString(object["source"])
	}
	return asString(v)
}

func settingNumber(v any) any {
	if s, ok := v.(string); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return f
		}
		return nil
	}
	return v
}

func intSetting(v any) (int, bool) {
	f, ok := asFloat(settingNumber(v))
	if !ok {
		return 0, false
	}
	return int(f), true
}

func intSettingOr(v any, fallback int) int {
	if n, ok := intSetting(v); ok {
		return n
	}
	return fallback
}

func floatSettings(v any, fallback []float64) []float64 {
	items, _ := asArray(v)
	out := make([]float64, 0, len(items))
	for _, item := range items {
		if f, ok := asFloat(settingNumber(item)); ok {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func (tb *targetBuild) unknownKeys(path string, object map[string]any, known []string) {
	for _, key := range sortedKeys(object) {
		if !slices.Contains(known, key) && present(object[key]) {
			tb.partial(path+"."+key, genericQueryNote)
		}
	}
}

func (tb *targetBuild) unknownSettings(path string, settings map[string]any, known ...string) {
	tb.unknownKeys(path+".settings", settings, known)
}

func elasticVariableQuery(v any) (map[string]any, bool) {
	text := strings.TrimSpace(variableQueryText(v))
	if !strings.HasPrefix(text, "{") {
		return nil, false
	}
	var query map[string]any
	if err := json.Unmarshal([]byte(text), &query); err != nil || asString(query["find"]) == "" {
		return nil, false
	}
	return query, true
}

func elasticLabelValues(query map[string]any) string {
	field := strings.TrimSpace(asString(query["field"]))
	if asString(query["find"]) != "terms" || field == "" {
		return ""
	}
	if lucene := strings.TrimSpace(asString(query["query"])); lucene != "" {
		return "label_values(" + luceneQueryPrefix + lucene + ", " + field + ")"
	}
	return "label_values(" + field + ")"
}

func filtersAppliedByRunner(kind string) bool {
	return kind == kindClickHouse || kind == kindElastic || kind == kindOpenSearch
}
