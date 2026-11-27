package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nameAggregate = "measure_aggregate"

	opTotal    = "total"
	opRatio    = "ratio"
	opQuantile = "quantile"

	maxAggregateBy = 5
	bucketSuffix   = "_bucket"

	descAggregate = "Measure one figure over a whole window, pooled from every sample or line in it and evaluated once at the window's end, never pieced together from range points or spot values. Use it for every total, share and quantile you report. op total: the increase of a Prometheus counter. op ratio: the total of query divided by the total of denominator, such as errors over requests; 0 when query has none while denominator has some; multiply by 100 for a percent. op quantile: a Prometheus histogram's quantile from the increase of its buckets over the window. On Prometheus use measure_window for max, min and avg. by groups the result, such as by service_name. Cite the query in the result."

	propAggregateOp    = `"op":{"type":"string","enum":["total","ratio","quantile"],"description":"What to measure over the window."}`
	propAggregateQuery = `"query":{"type":"string","description":"A series selector of a counter, or of a histogram for quantile, with concrete label values, such as http_requests_total{service_name=\"cart\",code=~\"5..\"}."}`
	propAggregateDen   = `"denominator":{"type":"string","description":"For ratio: the selector to divide by, such as http_requests_total{service_name=\"cart\"}."}`
	propAggregateQ     = `"quantile":{"type":"number","exclusiveMinimum":0,"maximum":1,"description":"For quantile, such as 0.99."}`
	propAggregateBy    = `"by":{"type":"array","maxItems":5,"items":{"type":"string","maxLength":64},"description":"Labels to group by; leave out for one figure over everything matched."}`
	schemaAggregate    = `{"type":"object","properties":{` + propDatasource + `,` + propAggregateOp + `,` + propAggregateQuery + `,` + propAggregateDen + `,` + propAggregateQ + `,` + propAggregateBy + `,` + propFrom + `,` + propTo + `},"required":["datasource","query","op"],"additionalProperties":false}`
)

var aggregateOps = []string{opTotal, opRatio, opQuantile}

func init() { registerEntries((*Registry).aggregateEntries) }

func (r *Registry) aggregateEntries() []entry {
	return []entry{
		{Tool{Name: nameAggregate, Description: descAggregate, InputSchema: json.RawMessage(schemaAggregate), ReadOnly: true, MinRole: auth.RoleViewer}, r.aggregate},
	}
}

type aggregateInput struct {
	Datasource  string   `json:"datasource"`
	Op          string   `json:"op"`
	Query       string   `json:"query"`
	Denominator string   `json:"denominator"`
	Quantile    *float64 `json:"quantile"`
	By          []string `json:"by"`
	From        string   `json:"from"`
	To          string   `json:"to"`
}

type aggregatePlan struct {
	query       string
	numerator   string
	denominator string
}

type aggregatedValue struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Value       *float64          `json:"value,omitempty"`
	Numerator   *float64          `json:"numerator,omitempty"`
	Denominator *float64          `json:"denominator,omitempty"`
	Undefined   bool              `json:"undefined,omitempty"`
}

type aggregateOut struct {
	Op               string            `json:"op"`
	From             string            `json:"from"`
	To               string            `json:"to"`
	Query            string            `json:"query"`
	NumeratorQuery   string            `json:"numerator_query,omitempty"`
	DenominatorQuery string            `json:"denominator_query,omitempty"`
	Empty            bool              `json:"empty"`
	Truncated        bool              `json:"truncated,omitempty"`
	Values           []aggregatedValue `json:"values,omitempty"`
	Note             string            `json:"note,omitempty"`
}

func checkAggregateFields(in aggregateInput) string {
	switch {
	case !slices.Contains(aggregateOps, in.Op):
		return "op must be one of " + strings.Join(aggregateOps, ", ")
	case (in.Op == opRatio) != (in.Denominator != ""):
		return "denominator goes with op ratio, and ratio needs one"
	case (in.Op == opQuantile) != (in.Quantile != nil):
		return "quantile goes with op quantile, and op quantile needs one, such as 0.99"
	case in.Quantile != nil && (*in.Quantile <= 0 || *in.Quantile > 1):
		return "quantile must be above 0 and at most 1, such as 0.99"
	case len(in.By) > maxAggregateBy:
		return fmt.Sprintf("by takes at most %d labels", maxAggregateBy)
	}
	for _, label := range in.By {
		if !spec.IsLabelName(label) {
			return fmt.Sprintf("by: %q is not a label name", clip(label, maxStepNameBytes))
		}
	}
	return ""
}

func grouping(by []string, extra ...string) string {
	return strings.Join(append(extra, by...), ", ")
}

func promCounterSelector(field, text string) (string, string) {
	matchers, err := selectorParser.ParseMetricSelector(strings.TrimSpace(text))
	if err != nil {
		return "", field + " must be a series selector such as http_requests_total{service_name=\"cart\"}, not an expression: the server adds increase and sum itself"
	}
	return renderSelector(matchers, ""), ""
}

func renderSelector(matchers []*labels.Matcher, suffix string) string {
	name := ""
	var rest []string
	for _, m := range matchers {
		if m.Name == metricNameLabel && m.Type == labels.MatchEqual && name == "" {
			name = m.Value
			continue
		}
		rest = append(rest, m.String())
	}
	if name != "" {
		name = strings.TrimSuffix(name, suffix) + suffix
	}
	if len(rest) == 0 && name != "" {
		return name
	}
	return name + "{" + strings.Join(rest, ", ") + "}"
}

func promTotal(selector string, by []string, window time.Duration) string {
	return fmt.Sprintf("sum by (%s) (increase(%s[%s]))", grouping(by), selector, promSeconds(window))
}

func ratioOf(numerator, denominator string) string {
	return fmt.Sprintf("(%s or 0 * %s) / %s", numerator, denominator, denominator)
}

func planPromAggregate(in aggregateInput, window time.Duration) (aggregatePlan, string) {
	if in.Op == opQuantile {
		matchers, err := selectorParser.ParseMetricSelector(strings.TrimSpace(in.Query))
		if err != nil {
			return aggregatePlan{}, "query must be the series selector of a histogram, such as http_server_duration_seconds{service_name=\"cart\"}"
		}
		if slices.Contains(in.By, "le") {
			return aggregatePlan{}, "by must not name le: the server groups by le itself"
		}
		buckets := renderSelector(matchers, bucketSuffix)
		q := strconv.FormatFloat(*in.Quantile, 'g', -1, 64)
		return aggregatePlan{query: fmt.Sprintf("histogram_quantile(%s, sum by (%s) (increase(%s[%s])))", q, grouping(in.By, "le"), buckets, promSeconds(window))}, ""
	}
	numerator, problem := promCounterSelector("query", in.Query)
	if problem != "" {
		return aggregatePlan{}, problem
	}
	total := promTotal(numerator, in.By, window)
	if in.Op == opTotal {
		return aggregatePlan{query: total}, ""
	}
	denominator, problem := promCounterSelector("denominator", in.Denominator)
	if problem != "" {
		return aggregatePlan{}, problem
	}
	den := promTotal(denominator, in.By, window)
	return aggregatePlan{query: ratioOf(total, den), numerator: total, denominator: den}, ""
}

func planAggregate(in aggregateInput, dsType spec.DatasourceType, window time.Duration) (aggregatePlan, string) {
	if problem := checkAggregateFields(in); problem != "" {
		return aggregatePlan{}, problem
	}
	if dsType != spec.Prometheus {
		return aggregatePlan{}, fmt.Sprintf("measure_aggregate reads Prometheus datasources only; on %s use query_instant with the store's own aggregate", dsType)
	}
	return planPromAggregate(in, window)
}

func (r *Registry) aggregate(ctx context.Context, input json.RawMessage) (Result, error) {
	var in aggregateInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	inst, res, ok := r.instance(ctx, in.Datasource)
	if !ok {
		return res, nil
	}
	if res, ok := checkQueryText(in.Query); !ok {
		return res, nil
	}
	if len(in.Denominator) > maxQueryBytes {
		return invalid("denominator must be at most %d bytes", maxQueryBytes), nil
	}
	rng, res, ok := resolveRunRange(ctx, in.From, in.To)
	if !ok {
		return res, nil
	}
	window := rng.To.Sub(rng.From).Truncate(time.Second)
	if window < time.Second {
		return invalid("the window must be at least one second"), nil
	}
	plan, problem := planAggregate(in, typeOf(inst), window)
	if problem != "" {
		return invalid("%s", problem), nil
	}
	out := aggregateOut{
		Op:               in.Op,
		From:             rng.From.UTC().Format(time.RFC3339),
		To:               rng.To.UTC().Format(time.RFC3339),
		Query:            plan.query,
		NumeratorQuery:   plan.numerator,
		DenominatorQuery: plan.denominator,
	}
	if failed, err := r.runAggregate(ctx, inst, plan, rng, &out); failed != nil || err != nil {
		return *failed, err
	}
	out.Empty = len(out.Values) == 0
	if out.Empty {
		out.Note = "No series matched in this window. An empty result does not show the value was zero: check the metric and label names with list_metrics and list_label_values."
	}
	return success("Measured "+in.Op+" on "+inst.UID+": "+describeAggregated(out), out), nil
}

func (r *Registry) runAggregate(ctx context.Context, inst datasource.Instance, plan aggregatePlan, rng datasource.TimeRange, out *aggregateOut) (*Result, error) {
	frames, failed, err := r.measureRun(ctx, inst, plan.query, rng)
	if failed != nil {
		return failed, err
	}
	out.Values, out.Truncated = aggregatedValues(frames)
	if plan.numerator == "" || len(out.Values) == 0 {
		return nil, nil
	}
	parts := map[string][]datasource.Frame{}
	for _, q := range []string{plan.numerator, plan.denominator} {
		got, failed, err := r.measureRun(ctx, inst, q, rng)
		if failed != nil {
			return failed, err
		}
		parts[q] = got
	}
	attachParts(out.Values, parts[plan.numerator], parts[plan.denominator])
	return nil, nil
}

func aggregatedValues(frames []datasource.Frame) ([]aggregatedValue, bool) {
	var out []aggregatedValue
	for _, f := range frames {
		v, ok := lastNumber(f)
		if !ok {
			continue
		}
		if len(out) == maxSeriesOut {
			return out, true
		}
		item := aggregatedValue{Labels: clippedLabels(f.Labels)}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			item.Undefined = true
		} else {
			item.Value = &v
		}
		out = append(out, item)
	}
	return out, false
}

func attachParts(values []aggregatedValue, numerator, denominator []datasource.Frame) {
	totals := func(frames []datasource.Frame) map[string]float64 {
		got := map[string]float64{}
		for _, f := range frames {
			if v, ok := lastNumber(f); ok && !math.IsNaN(v) && !math.IsInf(v, 0) {
				got[labelsText(clippedLabels(f.Labels))] = v
			}
		}
		return got
	}
	nums, dens := totals(numerator), totals(denominator)
	for i := range values {
		key := labelsText(values[i].Labels)
		den, ok := dens[key]
		if !ok {
			continue
		}
		num := nums[key]
		values[i].Numerator, values[i].Denominator = &num, &den
	}
}

func describeAggregated(out aggregateOut) string {
	switch {
	case out.Empty:
		return "no value"
	case len(out.Values) > 1:
		return plural(len(out.Values), "group", "groups")
	case out.Values[0].Value != nil:
		return strconv.FormatFloat(*out.Values[0].Value, 'g', -1, 64)
	}
	return "undefined, nothing was counted in the window"
}
