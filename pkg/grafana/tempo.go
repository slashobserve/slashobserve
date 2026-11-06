package grafana

import (
	"regexp"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

var tempoRules = targetRules{
	silent: map[string]bool{
		"filters": true, "groupBy": true, "serviceMapQuery": true, "serviceMapIncludeNamespace": true,
		"search": true, "serviceName": true, "spanName": true, "minDuration": true, "maxDuration": true,
		"step": true, "exemplars": true, "metricsQueryType": true,
	},
	defaults: map[string]any{"limit": 20, "spss": 3, "tableType": "traces"},
}

var traceQLMetricsPipeline = regexp.MustCompile(`\|\s*(rate|count_over_time|min_over_time|max_over_time|avg_over_time|sum_over_time|quantile_over_time|histogram_over_time|compare)\s*\(`)

var tempoDroppedQueryTypes = map[string]string{
	"traceqlSearch": decisionTempoSearchBuilder.reason,
	"nativeSearch":  decisionTempoSearchBuilder.reason,
	"serviceMap":    decisionTempoServiceGraph.reason,
	"upload":        decisionTraceUpload.reason,
	"clear":         emptyQueryReason,
}

func (tb *targetBuild) tempo() (spec.Query, string) {
	tb.rules = tempoRules
	tb.use("query", "queryType")
	switch queryType := asString(tb.t["queryType"]); queryType {
	case "", "traceql", "traceId":
	default:
		if reason, ok := tempoDroppedQueryTypes[queryType]; ok {
			return spec.Query{}, reason
		}
		return spec.Query{}, unsupportedValueReason("query type", queryType)
	}
	query := asString(tb.t["query"])
	switch {
	case strings.TrimSpace(query) == "":
		return spec.Query{}, emptyQueryReason
	case traceQLMetricsPipeline.MatchString(query):
		return spec.Query{}, decisionTraceQLMetrics.reason
	}
	return spec.Query{Query: query}, ""
}
