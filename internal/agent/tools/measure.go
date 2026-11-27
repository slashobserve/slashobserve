package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nameMeasure = "measure_window"

	statMax       = "max"
	statMin       = "min"
	statAvg       = "avg"
	statLast      = "last"
	statFirstTime = "first_time"
	statLastTime  = "last_time"

	measureDefaultStep = time.Minute
	measureTimeStep    = 15 * time.Second
	measureMaxSteps    = 11000
	everySample        = "every sample"

	descMeasure = "Measure one figure of a PromQL expression over a whole window on a Prometheus datasource, computed from every sample or step, never read off the sampled points of a range query. Use it for every peak, minimum, average and duration you report. stat is max, min, avg or last; first_time or last_time give when a condition first or last held, such as where \"== 0\" for when a queue emptied. A plain selector is measured over every stored sample; any other expression is evaluated every step, 1m unless you pass one, so pass the step the question names. max and min of one series also say when the value was reached. For a total, ratio or quantile use measure_aggregate. Cite the query in the result. To time a drain, set from to the peak's time and ask first_time where \"== 0\": seconds_after_from is the duration."

	propMeasureStat  = `"stat":{"type":"string","enum":["max","min","avg","last","first_time","last_time"],"description":"What to measure over the window."}`
	propMeasureStep  = `"step":{"type":"string","pattern":"^[1-9][0-9]{0,4}(s|m|h)$","description":"How often the expression is evaluated, such as 15s or 5m."}`
	propMeasureWhere = `"where":{"type":"string","maxLength":40,"description":"For first_time and last_time: a comparison with a number, such as == 0, > 100 or <= 0.5."}`
	propMeasureQuery = `"query":{"type":"string","description":"A PromQL expression that returns series, with concrete label values and no dashboard variables, such as sum(queue_depth{service=\"orders\"})."}`
	schemaMeasure    = `{"type":"object","properties":{` + propDatasource + `,` + propMeasureQuery + `,` + propMeasureStat + `,` + propFrom + `,` + propTo + `,` + propMeasureStep + `,` + propMeasureWhere + `},"required":["datasource","query","stat"],"additionalProperties":false}`
)

var (
	measureStats    = []string{statMax, statMin, statAvg, statLast, statFirstTime, statLastTime}
	measureStepExpr = regexp.MustCompile(`^([1-9][0-9]{0,4})(s|m|h)$`)
	measureWhere    = regexp.MustCompile(`^(==|!=|>=|<=|>|<)\s*(-?[0-9]+(?:\.[0-9]+)?)$`)
	plainSelector   = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[^{}]*\})?$`)
)

func init() { registerEntries((*Registry).measureEntries) }

func (r *Registry) measureEntries() []entry {
	return []entry{
		{Tool{Name: nameMeasure, Description: descMeasure, InputSchema: json.RawMessage(schemaMeasure), ReadOnly: true, MinRole: auth.RoleViewer}, r.measure},
	}
}

type measureInput struct {
	Datasource string `json:"datasource"`
	Query      string `json:"query"`
	Stat       string `json:"stat"`
	From       string `json:"from"`
	To         string `json:"to"`
	Step       string `json:"step"`
	Where      string `json:"where"`
}

type measuredValue struct {
	Labels  map[string]string `json:"labels,omitempty"`
	Value   *float64          `json:"value,omitempty"`
	At      string            `json:"at,omitempty"`
	Seconds *float64          `json:"seconds_after_from,omitempty"`
}

type measureOut struct {
	Stat      string          `json:"stat"`
	From      string          `json:"from"`
	To        string          `json:"to"`
	Sampled   string          `json:"sampled"`
	Query     string          `json:"query"`
	Empty     bool            `json:"empty"`
	Truncated bool            `json:"truncated,omitempty"`
	Values    []measuredValue `json:"values,omitempty"`
	Note      string          `json:"note,omitempty"`
}

type measurePlan struct {
	query   string
	sampled string
	times   bool
}

func parseStep(text string) (time.Duration, bool) {
	m := measureStepExpr.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(m[1])
	unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour}[m[2]]
	return time.Duration(n) * unit, true
}

func promSeconds(d time.Duration) string {
	return strconv.FormatInt(int64(d/time.Second), 10) + "s"
}

func overTime(fn, expr string, window, step time.Duration) string {
	if step == 0 {
		return fmt.Sprintf("%s_over_time(%s[%s])", fn, expr, promSeconds(window))
	}
	return fmt.Sprintf("%s_over_time((%s)[%s:%s])", fn, expr, promSeconds(window), promSeconds(step))
}

func timeOf(fn, expr, comparison string, window, step time.Duration) string {
	return fmt.Sprintf("%s_over_time((timestamp(%s) and ((%s) %s))[%s:%s])", fn, expr, expr, comparison, promSeconds(window), promSeconds(step))
}

func measureStep(in measureInput, expr string, times bool, window time.Duration) (time.Duration, string) {
	var step time.Duration
	if in.Step != "" {
		var ok bool
		if step, ok = parseStep(in.Step); !ok {
			return 0, "step must be a number and a unit, such as 15s, 1m or 5m"
		}
	}
	switch {
	case step > 0:
	case times:
		step = measureTimeStep
	case !plainSelector.MatchString(expr):
		step = measureDefaultStep
	}
	if step > 0 && window/step > measureMaxSteps {
		return 0, fmt.Sprintf("the window holds more than %d steps of %s; pass a longer step or a shorter window", measureMaxSteps, promSeconds(step))
	}
	return step, ""
}

func planMeasure(in measureInput, window time.Duration) (measurePlan, string) {
	expr := strings.TrimSpace(in.Query)
	times := in.Stat == statFirstTime || in.Stat == statLastTime
	switch {
	case times && in.Where == "":
		return measurePlan{}, "first_time and last_time need where, such as == 0"
	case !times && in.Where != "":
		return measurePlan{}, "where goes only with first_time and last_time"
	}
	step, problem := measureStep(in, expr, times, window)
	if problem != "" {
		return measurePlan{}, problem
	}
	sampled := everySample
	if step > 0 {
		sampled = "every " + promSeconds(step)
	}
	if !times {
		return measurePlan{query: overTime(in.Stat, expr, window, step), sampled: sampled}, ""
	}
	m := measureWhere.FindStringSubmatch(strings.TrimSpace(in.Where))
	if m == nil {
		return measurePlan{}, "where must be a comparison with a number, such as == 0, > 100 or <= 0.5"
	}
	fn := statMin
	if in.Stat == statLastTime {
		fn = statMax
	}
	return measurePlan{query: timeOf(fn, expr, m[1]+" "+m[2], window, step), sampled: sampled, times: true}, ""
}

type measureTarget struct {
	inst   datasource.Instance
	rng    datasource.TimeRange
	window time.Duration
	plan   measurePlan
}

func (r *Registry) measureTarget(ctx context.Context, in measureInput) (measureTarget, Result, bool) {
	inst, res, ok := r.instance(ctx, in.Datasource)
	if !ok {
		return measureTarget{}, res, false
	}
	if inst.Datasource.Type() != "prometheus" {
		return measureTarget{}, invalid("measure_window reads Prometheus datasources only; on %s use query_instant with the store's own aggregate", inst.Datasource.Type()), false
	}
	if res, ok := checkQueryText(in.Query); !ok {
		return measureTarget{}, res, false
	}
	if !slices.Contains(measureStats, in.Stat) {
		return measureTarget{}, invalid("stat must be one of %s", strings.Join(measureStats, ", ")), false
	}
	rng, res, ok := resolveRunRange(ctx, in.From, in.To)
	if !ok {
		return measureTarget{}, res, false
	}
	window := rng.To.Sub(rng.From).Truncate(time.Second)
	if window < time.Second {
		return measureTarget{}, invalid("the window must be at least one second"), false
	}
	plan, problem := planMeasure(in, window)
	if problem != "" {
		return measureTarget{}, invalid("%s", problem), false
	}
	return measureTarget{inst: inst, rng: rng, window: window, plan: plan}, Result{}, true
}

func (r *Registry) measure(ctx context.Context, input json.RawMessage) (Result, error) {
	var in measureInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	target, res, ok := r.measureTarget(ctx, in)
	if !ok {
		return res, nil
	}
	inst, rng, plan := target.inst, target.rng, target.plan
	frames, failed, err := r.measureRun(ctx, inst, plan.query, rng)
	if failed != nil {
		return *failed, err
	}
	out := measureOut{
		Stat:    in.Stat,
		From:    rng.From.UTC().Format(time.RFC3339),
		To:      rng.To.UTC().Format(time.RFC3339),
		Sampled: plan.sampled,
		Query:   plan.query,
	}
	out.Values, out.Truncated = measuredValues(frames, plan.times, rng.From)
	out.Empty = len(out.Values) == 0
	switch {
	case out.Empty && plan.times:
		out.Note = "The condition never held at a step of this window. That is a finding about these steps only: check the value with stat last, or pass a shorter step."
	case out.Empty:
		out.Note = explainedNote(r.explainEmpty(ctx, inst, plan.query, rng), "No series matched in this window. An empty result does not show the value was zero: check the metric and label names with list_metrics and list_label_values.")
	case !plan.times && (in.Stat == statMax || in.Stat == statMin) && len(out.Values) == 1:
		r.measureWhen(ctx, inst, in, &out, rng, target.window)
	}
	return success("Measured "+in.Stat+" on "+inst.UID+": "+describeMeasured(out), out), nil
}

func (r *Registry) measureRun(ctx context.Context, inst datasource.Instance, query string, rng datasource.TimeRange) ([]datasource.Frame, *Result, error) {
	dash, selections, res, ok := adhocDashboard(inst, spec.Table, instantInput{Datasource: inst.UID, Query: query}, "")
	if !ok {
		return nil, &res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	got, err := r.deps.Runner.RunPanel(ctx, queryproxy.Request{
		Dashboard:  dash,
		PanelID:    adhocPanelID,
		Range:      rng,
		Selections: selections,
		Limits:     queryproxy.DraftLimits,
		Explore:    true,
		User:       queryproxy.UserFor(principal),
	})
	if err != nil {
		failed, callErr := datasourceFailure("Measured on "+inst.UID, err)
		return nil, &failed, callErr
	}
	return got.Frames(), nil, nil
}

func (r *Registry) measureWhen(ctx context.Context, inst datasource.Instance, in measureInput, out *measureOut, rng datasource.TimeRange, window time.Duration) {
	value := *out.Values[0].Value
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	step := measureTimeStep
	if given, ok := parseStep(in.Step); ok {
		step = given
	}
	if window/step > measureMaxSteps {
		return
	}
	literal := strconv.FormatFloat(value, 'g', -1, 64)
	frames, failed, err := r.measureRun(ctx, inst, timeOf(statMin, strings.TrimSpace(in.Query), "== "+literal, window, step), rng)
	if failed != nil || err != nil {
		return
	}
	if when, _ := measuredValues(frames, true, rng.From); len(when) == 1 {
		out.Values[0].At, out.Values[0].Seconds = when[0].At, when[0].Seconds
	}
}

func lastNumber(f datasource.Frame) (float64, bool) {
	for _, field := range f.Fields {
		if field.Type != datasource.FieldNumber || len(field.Values) == 0 {
			continue
		}
		v, ok := field.Values[len(field.Values)-1].(float64)
		return v, ok
	}
	return 0, false
}

func measuredValues(frames []datasource.Frame, times bool, from time.Time) ([]measuredValue, bool) {
	var out []measuredValue
	for _, f := range frames {
		v, ok := lastNumber(f)
		if !ok {
			continue
		}
		if len(out) == maxSeriesOut {
			return out, true
		}
		item := measuredValue{Labels: clippedLabels(f.Labels)}
		if times {
			at := time.UnixMilli(int64(math.Round(v * 1000))).UTC()
			seconds := at.Sub(from).Seconds()
			item.At, item.Seconds = at.Format(time.RFC3339), &seconds
		} else {
			item.Value = &v
		}
		out = append(out, item)
	}
	return out, false
}

func describeMeasured(out measureOut) string {
	switch {
	case out.Empty:
		return "no value"
	case len(out.Values) > 1:
		return strconv.Itoa(len(out.Values)) + " series"
	case out.Values[0].Value != nil:
		return strconv.FormatFloat(*out.Values[0].Value, 'g', -1, 64)
	}
	return out.Values[0].At
}
