package grafana

import (
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	lokiStreamReason   = "Grafana tails this query live."
	lokiLogRangeReason = "A Loki log query reads the whole time range."
	lokiStepReason     = "Grafana uses this step as is; SlashObserve uses it as the smallest step."
)

var lokiRules = targetRules{
	silent:   map[string]bool{"supportingQueryType": true},
	defaults: map[string]any{"direction": "backward"},
}

var lokiDefaultSteps = []string{"$__interval", "${__interval}", "$__auto", "${__auto}"}

func (tb *targetBuild) loki() (spec.Query, string) {
	tb.rules = lokiRules
	tb.use("expr")
	expr := asString(tb.t["expr"])
	if strings.TrimSpace(expr) == "" {
		return spec.Query{}, emptyQueryReason
	}
	instant, instantKey := tb.lokiInstant()
	if spec.LogQLReturnsLines(expr) {
		if instant {
			tb.partial(instantKey, lokiLogRangeReason)
		}
		tb.noEffect("legendFormat", "resolution", "step")
		return spec.Query{Query: tb.replaceAuto(expr, false)}, ""
	}
	q := spec.Query{Query: tb.replaceAuto(expr, instant), Legend: tb.legend(), Instant: instant}
	if instant {
		tb.noEffect("resolution", "step")
	} else {
		q.IntervalFactor = tb.resolutionFactor("resolution")
		q.Interval = tb.lokiStep()
	}
	tb.noEffect("maxLines", "direction")
	return q, ""
}

func (tb *targetBuild) noEffect(keys ...string) {
	for _, key := range keys {
		if present(tb.t[key]) {
			tb.use(key)
			tb.b.recordImplicit(tb.path+"."+key, outcomeNoEffect)
		}
	}
}

func (tb *targetBuild) lokiInstant() (bool, string) {
	tb.use("queryType", "instant", "range")
	queryType := asString(tb.t["queryType"])
	if queryType == "" {
		return truthy(tb.t["instant"]), "instant"
	}
	for _, key := range []string{"instant", "range"} {
		if present(tb.t[key]) {
			tb.b.recordImplicit(tb.path+"."+key, outcomeBookkeeping)
		}
	}
	switch queryType {
	case "range":
	case "instant":
		return true, "queryType"
	case "stream":
		tb.partial("queryType", lokiStreamReason)
	default:
		tb.partial("queryType", unsupportedValueReason("query type", queryType))
	}
	return false, "queryType"
}

func (tb *targetBuild) replaceAuto(expr string, instant bool) string {
	refs, err := spec.ScanQuery(expr)
	if err != nil {
		return expr
	}
	replacement := "$__interval"
	if instant {
		replacement = "$__range"
	}
	autos := slices.DeleteFunc(spec.FlattenReferences(refs), func(ref spec.Reference) bool {
		return ref.Kind != spec.MacroRef || ref.Name != "__auto"
	})
	slices.SortFunc(autos, func(a, b spec.Reference) int { return b.Start - a.Start })
	for _, ref := range autos {
		expr = expr[:ref.Start] + replacement + expr[ref.End:]
	}
	if len(autos) > 0 {
		tb.addRewrite("$__auto", replacement)
	}
	return expr
}

func (tb *targetBuild) lokiStep() string {
	tb.use("step")
	text := strings.TrimSpace(asString(tb.t["step"]))
	switch {
	case text == "":
		return ""
	case slices.Contains(lokiDefaultSteps, text):
		tb.b.recordImplicit(tb.path+".step", outcomeDefault)
		return ""
	}
	value, ok := tb.b.c.minStep(text)
	if !ok || value == "" {
		tb.partial("step", minStepLostReason(text))
		return ""
	}
	tb.partial("step", lokiStepReason)
	return value
}
