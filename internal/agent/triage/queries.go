package triage

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	basisSpanMetrics = "span metrics"
	basisHTTPMetrics = "HTTP server metrics"
	basisTraceSpans  = "trace spans"
	basisLogLevels   = "log levels"

	spanKinds       = `span_kind=~"SPAN_KIND_SERVER|SPAN_KIND_CONSUMER|"`
	spanError       = `status_code="STATUS_CODE_ERROR"`
	levelLabel      = "detected_level"
	defaultLabel    = "service_name"
	traceServiceKey = "resource.service.name"
	deployPattern   = `(?i)(deploy|rollout|rolled out|restart|version)`
	flagEventName   = "feature_flag.evaluation"
	flagKeyAttr     = "event.feature_flag.key"
	minRateWindow   = 2 * time.Minute
)

var (
	labelName    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	errorLevels  = []string{"error", "err", "fatal", "critical", "crit", "panic", "emerg", "alert"}
	flagVariants = []string{"event.feature_flag.result.variant", "event.feature_flag.variant"}
	logFlagKeys  = []string{"feature_flag_key", "feature_flag_result_variant", "feature_flag_variant"}
)

type metricPlan struct {
	label    string
	requests string
	errors   []string
	bucket   string
	scale    float64
	basis    string
}

func pickMetrics(calls, durations []string, label string) (metricPlan, bool) {
	if !labelName.MatchString(label) {
		return metricPlan{}, false
	}
	slices.Sort(calls)
	slices.Sort(durations)
	if span, ok := spanCallsName(calls); ok {
		prefix := strings.TrimSuffix(span, "calls_total")
		bucket, _ := first(durations, func(n string) bool {
			return strings.HasPrefix(n, prefix) && strings.Contains(n, "duration") && strings.HasSuffix(n, "_bucket")
		})
		selector := span + "{" + spanKinds
		return metricPlan{label: label, requests: selector + "}", errors: []string{selector + "," + spanError + "}"}, bucket: bucket, scale: scaleOf(bucket), basis: basisSpanMetrics}, true
	}
	bucket, ok := first(durations, func(n string) bool {
		return strings.HasPrefix(n, "http_server") && strings.Contains(n, "duration") && strings.HasSuffix(n, "_bucket")
	})
	if !ok {
		return metricPlan{}, false
	}
	count := strings.TrimSuffix(bucket, "_bucket") + "_count"
	return metricPlan{
		label: label, requests: count, bucket: bucket, scale: scaleOf(bucket), basis: basisHTTPMetrics,
		errors: []string{count + `{http_response_status_code=~"5.."}`, count + `{http_status_code=~"5.."}`},
	}, true
}

func spanCallsName(calls []string) (string, bool) {
	if name, ok := first(calls, func(n string) bool { return strings.HasSuffix(n, "calls_total") && strings.Contains(n, "span") }); ok {
		return name, true
	}
	return first(calls, func(n string) bool { return strings.HasSuffix(n, "calls_total") })
}

func first(names []string, keep func(string) bool) (string, bool) {
	i := slices.IndexFunc(names, keep)
	if i < 0 {
		return "", false
	}
	return names[i], true
}

func scaleOf(bucket string) float64 {
	if strings.Contains(bucket, "milliseconds") || bucket == "" {
		return 1
	}
	if strings.Contains(bucket, "seconds") {
		return 1000
	}
	return 1
}

func (p metricPlan) names() string {
	names := []string{strings.SplitN(p.requests, "{", 2)[0]}
	if p.bucket != "" {
		names = append(names, p.bucket)
	}
	return strings.Join(names, " and ")
}

func (p metricPlan) requestQuery(rate time.Duration) string {
	return fmt.Sprintf("sum by (%s) (rate(%s[%s]))", p.label, p.requests, promDuration(rate))
}

func (p metricPlan) errorQuery(rate time.Duration) string {
	parts := make([]string, len(p.errors))
	for i, sel := range p.errors {
		parts[i] = fmt.Sprintf("sum by (%s) (rate(%s[%s]))", p.label, sel, promDuration(rate))
	}
	return strings.Join(parts, " or ")
}

func (p metricPlan) latencyQuery(rate time.Duration) string {
	sel := p.bucket
	if p.basis == basisSpanMetrics {
		sel += "{" + spanKinds + "}"
	}
	return fmt.Sprintf("histogram_quantile(0.95, sum by (%s, le) (rate(%s[%s])))", p.label, sel, promDuration(rate))
}

func promDuration(d time.Duration) string {
	return strconv.FormatInt(int64(d/time.Second), 10) + "s"
}

func rateWindow(step time.Duration) time.Duration {
	return max(2*step, minRateWindow)
}

func logStreams(label string) string {
	return fmt.Sprintf("{%s=~\".+\"}", label)
}

func logLevelQuery(label string, step time.Duration) string {
	return fmt.Sprintf("sum by (%s, %s) (count_over_time(%s[%s]))", label, levelLabel, logStreams(label), promDuration(step))
}

func deployLineQuery(label string, step time.Duration) string {
	return fmt.Sprintf("sum by (%s) (count_over_time(%s |~ %s [%s]))", label, logStreams(label), strconv.Quote(deployPattern), promDuration(step))
}

func logFlagQuery(label string, step time.Duration) string {
	return fmt.Sprintf("sum by (%s) (count_over_time(%s | %s != \"\" [%s]))", strings.Join(logFlagKeys, ", "), logStreams(label), logFlagKeys[0], promDuration(step))
}

func lokiSelector(label, service string) string {
	return fmt.Sprintf("{%s=%s}", label, strconv.Quote(service))
}

func traceAll(dsType string) string {
	if dsType == "tempo" {
		return "{}"
	}
	return ""
}

func traceService(dsType, service string) string {
	if dsType == "tempo" {
		return fmt.Sprintf("{ %s = %s }", traceServiceKey, strconv.Quote(service))
	}
	return "ServiceName = '" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(service) + "'"
}

func flagQuery(conds ...string) string {
	all := append([]string{"event:name = " + strconv.Quote(flagEventName)}, conds...)
	return "{ " + strings.Join(all, " && ") + " }"
}

func attrEquals(attr, value string) string {
	return attr + " = " + strconv.Quote(value)
}
