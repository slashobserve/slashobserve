package grafana

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	classicConditionsReason = "Classic conditions belong to alert rules, which SlashObserve does not import."
	sqlExpressionReason     = "SQL expressions are not supported."
	noExpressionInputReason = "This expression uses no query."
	expressionCycleReason   = "This expression depends on itself through its inputs."
	unreadableMathReason    = "The math expression holds a reference SlashObserve cannot read."
	noInputReason           = "The expression has no input query."
	hysteresisReason        = "A threshold with a separate unload condition is not supported."
)

var (
	expressionReducers = map[string]bool{"mean": true, "min": true, "max": true, "sum": true, "count": true, "last": true, "median": true}
	downsamplers       = map[string]bool{"mean": true, "min": true, "max": true, "sum": true, "last": true}
	upsamplers         = map[string]string{"pad": "pad", "backfilling": "backfill", "fillna": "fillNA"}
	resampleWindow     = regexp.MustCompile(`^[1-9][0-9]{0,4}(ms|[smhdwy])$`)

	thresholdEvaluators = map[string]string{
		"gt": "gt", "lt": "lt",
		"within_range": "withinRange", "outside_range": "outsideRange",
		"within_range_included": "withinRangeIncluded", "outside_range_included": "outsideRangeIncluded",
	}
	thresholdComparisons = map[string]string{"gte": ">=", "lte": "<=", "eq": "==", "ne": "!="}

	mathFunctions = []string{"abs", "ceil", "floor", "round", "log", "exp", "sqrt", "is_null", "is_nan", "is_number", "is_inf"}
)

type pendingExpression struct {
	inputs []string
	build  func(rename func(string) string) spec.Expression
}

func unsupportedFunctionReason(name string) string {
	return fmt.Sprintf("The function %s is not supported yet.", name)
}

func missingInputReason(ref string) string {
	return fmt.Sprintf("This expression uses query %s, which could not be imported.", ref)
}

func unsupportedValueReason(what, value string) string {
	if value == "" {
		return fmt.Sprintf("The expression has no %s.", what)
	}
	return fmt.Sprintf("The %s %s is not supported yet.", what, value)
}

func (tb *targetBuild) expression() (spec.Query, string) {
	tb.use("type", "expression", "reducer", "settings", "window", "downsampler", "upsampler", "conditions")
	var (
		pending *pendingExpression
		reason  string
	)
	switch kind := asString(tb.t["type"]); kind {
	case spec.ExpressionMath:
		pending, reason = mathExpression(asString(tb.t["expression"]))
	case spec.ExpressionReduce:
		pending, reason = tb.reduceExpression()
	case spec.ExpressionResample:
		pending, reason = tb.resampleExpression()
	case spec.ExpressionThreshold:
		pending, reason = tb.thresholdExpression()
	case "classic_conditions":
		reason = classicConditionsReason
	case "sql":
		reason = sqlExpressionReason
	default:
		reason = unsupportedValueReason("expression type", kind)
	}
	if reason != "" {
		return spec.Query{}, reason
	}
	tb.expr = pending
	return spec.Query{Hide: truthy(tb.t["hide"])}, ""
}

func mathExpression(text string) (*pendingExpression, string) {
	if strings.TrimSpace(text) == "" {
		return nil, "The math expression is empty."
	}
	_, refs, reason := rewriteMath(text, func(ref string) string { return ref })
	if reason != "" {
		return nil, reason
	}
	return &pendingExpression{inputs: refs, build: func(rename func(string) string) spec.Expression {
		out, _, _ := rewriteMath(text, rename)
		return spec.Expression{Type: spec.ExpressionMath, Math: out}
	}}, ""
}

func (tb *targetBuild) expressionInput() string {
	input := strings.TrimSpace(asString(tb.t["expression"]))
	input = strings.TrimPrefix(input, "$")
	return strings.TrimSuffix(strings.TrimPrefix(input, "{"), "}")
}

func (tb *targetBuild) reduceExpression() (*pendingExpression, string) {
	reducer := asString(tb.t["reducer"])
	if !expressionReducers[reducer] {
		return nil, unsupportedValueReason("reducer", reducer)
	}
	settings, _ := asObject(tb.t["settings"])
	x := spec.Expression{Type: spec.ExpressionReduce, Reducer: reducer}
	switch mode := asString(settings["mode"]); mode {
	case "":
	case "dropNN":
		x.Mode = "dropNonNumbers"
	case "replaceNN":
		value, ok := asFloat(settings["replaceWithValue"])
		if !ok {
			return nil, "The reduce mode replaceNN has no value to replace with."
		}
		x.Mode, x.ReplaceWith = spec.ModeReplaceNonNumbers, &value
	default:
		return nil, unsupportedValueReason("reduce mode", mode)
	}
	return tb.inputExpression(x)
}

func (tb *targetBuild) resampleExpression() (*pendingExpression, string) {
	window := asString(tb.t["window"])
	if !resampleWindow.MatchString(window) {
		return nil, unsupportedValueReason("resample window", window)
	}
	downsampler := asString(tb.t["downsampler"])
	if !downsamplers[downsampler] {
		return nil, unsupportedValueReason("downsampler", downsampler)
	}
	upsampler, ok := upsamplers[asString(tb.t["upsampler"])]
	if !ok {
		return nil, unsupportedValueReason("upsampler", asString(tb.t["upsampler"]))
	}
	x := spec.Expression{Type: spec.ExpressionResample, Window: window, Downsampler: downsampler, Upsampler: upsampler}
	return tb.inputExpression(x)
}

func (tb *targetBuild) thresholdExpression() (*pendingExpression, string) {
	conditions, _ := asArray(tb.t["conditions"])
	if len(conditions) == 0 {
		return nil, unsupportedValueReason("threshold evaluator", "")
	}
	condition, _ := asObject(conditions[0])
	if present(condition["unloadEvaluator"]) {
		return nil, hysteresisReason
	}
	evaluator, _ := asObject(condition["evaluator"])
	kind := asString(evaluator["type"])
	params, ok := numberList(evaluator["params"])
	if !ok {
		return nil, "The threshold values must be numbers."
	}
	op, comparison := thresholdComparisons[kind]
	name, known := thresholdEvaluators[kind]
	if !known && !comparison {
		return nil, unsupportedValueReason("threshold evaluator", kind)
	}
	want := 1
	if known {
		want = thresholdParamCount(name)
	}
	if len(params) != want {
		return nil, fmt.Sprintf("The threshold evaluator %s needs %d number(s).", kind, want)
	}
	if comparison {
		return tb.comparisonExpression(op, params[0])
	}
	return tb.inputExpression(spec.Expression{Type: spec.ExpressionThreshold, Evaluator: name, Params: params})
}

func thresholdParamCount(evaluator string) int {
	if evaluator == "gt" || evaluator == "lt" {
		return 1
	}
	return 2
}

func (tb *targetBuild) comparisonExpression(op string, value float64) (*pendingExpression, string) {
	input := tb.expressionInput()
	if input == "" {
		return nil, noInputReason
	}
	text := strconv.FormatFloat(value, 'g', -1, 64)
	return &pendingExpression{inputs: []string{input}, build: func(rename func(string) string) spec.Expression {
		return spec.Expression{Type: spec.ExpressionMath, Math: "$" + rename(input) + " " + op + " " + text}
	}}, ""
}

func (tb *targetBuild) inputExpression(x spec.Expression) (*pendingExpression, string) {
	input := tb.expressionInput()
	if input == "" {
		return nil, noInputReason
	}
	return &pendingExpression{inputs: []string{input}, build: func(rename func(string) string) spec.Expression {
		x.Input = rename(input)
		return x
	}}, ""
}

func numberList(v any) ([]float64, bool) {
	items, _ := asArray(v)
	out := make([]float64, 0, len(items))
	for _, item := range items {
		n, ok := asFloat(item)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func rewriteMath(text string, rename func(string) string) (string, []string, string) {
	var out strings.Builder
	var refs []string
	for i := 0; i < len(text); {
		c := text[i]
		switch {
		case c == '$':
			name, end := readMathRef(text, i)
			if name == "" {
				return "", nil, unreadableMathReason
			}
			if !slices.Contains(refs, name) {
				refs = append(refs, name)
			}
			out.WriteString("$" + rename(name))
			i = end
		case isDigit(c) || c == '.':
			end := mathNumberEnd(text, i)
			out.WriteString(text[i:end])
			i = end
		case isMathRefByte(c, true) || c == '_':
			end := i
			for end < len(text) && (isMathRefByte(text[end], false)) {
				end++
			}
			name := text[i:end]
			if !slices.Contains(mathFunctions, name) || end >= len(text) || text[end] != '(' {
				return "", nil, unsupportedFunctionReason(name)
			}
			out.WriteString(name)
			i = end
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.String(), refs, ""
}

func readMathRef(text string, at int) (string, int) {
	start, braced := at+1, false
	if start < len(text) && text[start] == '{' {
		start, braced = start+1, true
	}
	end := start
	for end < len(text) && isMathRefByte(text[end], end == start) {
		end++
	}
	switch {
	case end == start:
		return "", 0
	case !braced:
		return text[start:end], end
	case end < len(text) && text[end] == '}':
		return text[start:end], end + 1
	}
	return "", 0
}

func isMathRefByte(c byte, first bool) bool {
	letter := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
	return letter || (!first && (isDigit(c) || c == '_'))
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func mathNumberEnd(text string, start int) int {
	end := start
	for end < len(text) && (isDigit(text[end]) || text[end] == '.') {
		end++
	}
	if end >= len(text) || (text[end] != 'e' && text[end] != 'E') {
		return end
	}
	next := end + 1
	if next < len(text) && (text[next] == '+' || text[next] == '-') {
		next++
	}
	if next >= len(text) || !isDigit(text[next]) {
		return end
	}
	for next < len(text) && isDigit(text[next]) {
		next++
	}
	return next
}

func (b *panelBuild) firstDataQuery() int {
	return slices.IndexFunc(b.built, func(bq *builtQuery) bool { return bq.src != nil })
}

func (b *panelBuild) grafanaRefIDs() []string {
	explicit := explicitRefIDs(b.built)
	ids := make([]string, len(b.built))
	for k, bq := range b.built {
		if bq.refID != "" {
			ids[k] = bq.refID
			continue
		}
		if implied, ok := impliedRefID(bq.index, explicit); ok {
			ids[k] = implied
		}
	}
	return ids
}

func (b *panelBuild) brokenExpression() (int, string) {
	ids := b.grafanaRefIDs()
	graph := map[string][]string{}
	for k, bq := range b.built {
		if _, seen := graph[ids[k]]; bq.expr != nil && !seen {
			graph[ids[k]] = bq.expr.inputs
		}
	}
	for k, bq := range b.built {
		if bq.expr == nil {
			continue
		}
		for _, input := range bq.expr.inputs {
			if !slices.Contains(ids, input) {
				return k, missingInputReason(input)
			}
		}
		if reachesRef(graph, ids[k], ids[k], map[string]bool{}) {
			return k, expressionCycleReason
		}
	}
	return -1, ""
}

func reachesRef(graph map[string][]string, from, target string, seen map[string]bool) bool {
	for _, next := range graph[from] {
		if next == target {
			return true
		}
		if !seen[next] {
			seen[next] = true
			if reachesRef(graph, next, target, seen) {
				return true
			}
		}
	}
	return false
}

func (b *panelBuild) dropBuilt(k int, reason string) {
	bq := b.built[k]
	b.built = slices.Delete(b.built, k, k+1)
	items, _ := asArray(b.raw["targets"])
	target, _ := asObject(items[bq.index])
	b.dropTarget(bq.index, target, reason)
}

func (b *panelBuild) dropBrokenExpressions() {
	for {
		k, reason := b.brokenExpression()
		if k < 0 {
			break
		}
		b.dropBuilt(k, reason)
	}
	if len(b.built) > 0 && b.firstDataQuery() < 0 && !b.reusesPanel() {
		for len(b.built) > 0 {
			b.dropBuilt(0, noExpressionInputReason)
		}
	}
	for _, bq := range b.built {
		if bq.late != nil {
			bq.late.reportUnconsumed()
		}
	}
}

func (b *panelBuild) finishExpressions() {
	rename := func(id string) string { return b.refs[id] }
	for k, bq := range b.built {
		if bq.expr != nil {
			x := bq.expr.build(rename)
			b.panel.Queries[k].Expression = &x
		}
	}
}
