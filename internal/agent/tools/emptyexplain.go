package tools

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxDiagnosedSelectors = 3
	maxCloseNames         = 3
	maxShownNames         = 8
)

var serviceKeys = []string{"service_name", "service", "app", "application", "job", "svc", "service_namespace", "k8s_deployment_name", "container"}

type emptySelector struct {
	metric   string
	matchers []*labels.Matcher
	text     string
}

func emptySelectors(dsType spec.DatasourceType, query string) []emptySelector {
	switch dsType {
	case spec.Prometheus:
		return promSelectors(query)
	case spec.Loki:
		return streamSelectors(query)
	}
	return nil
}

func promSelectors(query string) []emptySelector {
	expr, err := selectorParser.ParseExpr(query)
	if err != nil {
		return nil
	}
	var out []emptySelector
	seen := map[string]bool{}
	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		vs, ok := node.(*parser.VectorSelector)
		if !ok || len(out) == maxDiagnosedSelectors {
			return nil
		}
		sel := emptySelector{metric: vs.Name}
		for _, m := range vs.LabelMatchers {
			if m.Name == metricNameLabel {
				if m.Type == labels.MatchEqual {
					sel.metric = m.Value
				}
				continue
			}
			sel.matchers = append(sel.matchers, m)
		}
		sel.text = vs.String()
		if !seen[sel.text] {
			seen[sel.text] = true
			out = append(out, sel)
		}
		return nil
	})
	return out
}

func streamSelectors(query string) []emptySelector {
	var out []emptySelector
	for _, text := range braceGroups(query) {
		matchers, err := selectorParser.ParseMetricSelector(text)
		if err != nil {
			continue
		}
		out = append(out, emptySelector{matchers: matchers, text: text})
		if len(out) == maxDiagnosedSelectors {
			break
		}
	}
	return out
}

func braceGroups(text string) []string {
	var out []string
	open := -1
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '"', '`':
			end := stringEnd(text, i)
			if end < 0 {
				return out
			}
			i = end - 1
		case '{':
			open = i
		case '}':
			if open >= 0 {
				out = append(out, text[open:i+1])
				open = -1
			}
		}
	}
	return out
}

func stringEnd(text string, open int) int {
	quote := text[open]
	for i := open + 1; i < len(text); i++ {
		switch {
		case quote == '"' && text[i] == '\\':
			i++
		case text[i] == quote:
			return i + 1
		}
	}
	return -1
}

type nameRead struct {
	done      bool
	failed    bool
	truncated bool
	names     []string
}

type emptyFacts struct {
	keys     nameRead
	keysEver nameRead
	metrics  nameRead
	matches  nameRead
	values   map[string]nameRead
	ever     map[string]nameRead
}

type factKind int

const (
	factNone factKind = iota
	factKeys
	factKeysEver
	factMetrics
	factValues
	factValuesEver
	factMatches
)

type factNeed struct {
	kind factKind
	key  string
}

type diagnosis struct {
	note      string
	need      factNeed
	allExists bool
}

func positive(m *labels.Matcher) bool {
	return (m.Type == labels.MatchEqual || m.Type == labels.MatchRegexp) && !m.Matches("")
}

func diagnoseSelector(store spec.DatasourceType, sel emptySelector, f emptyFacts) diagnosis {
	switch {
	case !f.keys.done:
		return diagnosis{need: factNeed{kind: factKeys}}
	case f.keys.failed:
		return diagnosis{}
	case len(f.keys.names) == 0:
		return noSeries(store, sel, f)
	}
	for _, m := range sel.matchers {
		if !positive(m) {
			continue
		}
		if !slices.Contains(f.keys.names, m.Name) {
			return missingKey(store, sel, m, f)
		}
		got := f.values[m.Name]
		switch {
		case !got.done:
			return diagnosis{need: factNeed{kind: factValues, key: m.Name}}
		case got.failed:
			return diagnosis{}
		case !slices.ContainsFunc(got.names, m.Matches):
			if got.truncated {
				return diagnosis{}
			}
			return missingValue(store, sel, m, f)
		}
	}
	return everyLabelExists(store, sel, f)
}

func scopeOf(store spec.DatasourceType, sel emptySelector) string {
	switch {
	case sel.metric != "":
		return "metric " + sel.metric
	case store == spec.Loki:
		return "any stream"
	}
	return "any series"
}

func seriesOf(store spec.DatasourceType, sel emptySelector) string {
	switch {
	case sel.metric != "":
		return "series of metric " + sel.metric
	case store == spec.Loki:
		return "streams"
	}
	return "series"
}

func noSeries(store spec.DatasourceType, sel emptySelector, f emptyFacts) diagnosis {
	switch {
	case !f.keysEver.done:
		return diagnosis{need: factNeed{kind: factKeysEver}}
	case f.keysEver.failed:
		return diagnosis{}
	case len(f.keysEver.names) > 0 && sel.metric != "":
		return diagnosis{note: fmt.Sprintf("Metric %s has series in this datasource, but none with samples in this window: widen or move the window", sel.metric)}
	case len(f.keysEver.names) > 0:
		return diagnosis{note: "This datasource has data in the 7 days around this window, but none in it: widen or move the window"}
	case sel.metric == "" || store != spec.Prometheus:
		return diagnosis{note: "This datasource holds no data in this window or the 7 days around it"}
	case !f.metrics.done:
		return diagnosis{need: factNeed{kind: factMetrics}}
	}
	note := fmt.Sprintf("No metric named %s exists in this datasource", sel.metric)
	if near := closest(sel.metric, f.metrics.names, maxCloseNames); len(near) > 0 {
		note += "; the closest names are " + quotedList(near)
	} else if !f.metrics.failed {
		note += "; list_metrics shows the names it has"
	}
	return diagnosis{note: note}
}

func missingKey(store spec.DatasourceType, sel emptySelector, m *labels.Matcher, f emptyFacts) diagnosis {
	switch {
	case !f.keysEver.done:
		return diagnosis{need: factNeed{kind: factKeysEver}}
	case slices.Contains(f.keysEver.names, m.Name):
		return diagnosis{note: fmt.Sprintf("Label %s is on %s in the 7 days around this window, but on none with samples in it: widen or move the window", m.Name, seriesOf(store, sel))}
	}
	note := fmt.Sprintf("Label key %s does not exist on %s in this window", m.Name, scopeOf(store, sel))
	if near := closest(m.Name, f.keys.names, maxCloseNames); len(near) > 0 {
		note += "; the closest keys it has are " + quotedList(near)
	} else {
		note += "; its keys are " + quotedList(firstNames(f.keys.names, maxShownNames))
	}
	if store == spec.Loki {
		note += "; a stream selector takes stream labels only, and a field such as level is filtered after a pipe, as | level=\"error\""
	}
	return diagnosis{note: note}
}

func missingValue(store spec.DatasourceType, sel emptySelector, m *labels.Matcher, f emptyFacts) diagnosis {
	ever, ok := f.ever[m.Name]
	switch {
	case !ok:
		return diagnosis{need: factNeed{kind: factValuesEver, key: m.Name}}
	case slices.ContainsFunc(ever.names, m.Matches):
		return diagnosis{note: fmt.Sprintf("%s matches %s in the 7 days around this window, but none with samples in it: widen or move the window", m, seriesOf(store, sel))}
	}
	values := f.values[m.Name].names
	note := fmt.Sprintf("%s matches no %s in this window", m, seriesOf(store, sel))
	if near := closest(literalOf(m), values, maxCloseNames); len(near) > 0 {
		note += "; the closest values of " + m.Name + " are " + quotedList(near)
	} else {
		note += "; " + m.Name + " has " + plural(len(values), "value", "values") + " such as " + quotedList(firstNames(values, maxShownNames))
	}
	return diagnosis{note: note}
}

func everyLabelExists(store spec.DatasourceType, sel emptySelector, f emptyFacts) diagnosis {
	if store != spec.Loki {
		return diagnosis{allExists: true}
	}
	switch {
	case !f.matches.done:
		return diagnosis{need: factNeed{kind: factMatches}}
	case f.matches.failed:
		return diagnosis{}
	case len(f.matches.names) > 0:
		return diagnosis{note: fmt.Sprintf("%s lines of stream selector %s fall in this window, so the line filters, parsers or label filters after it dropped every one: check them against sample_rows", f.matches.names[0], sel.text)}
	}
	return diagnosis{note: fmt.Sprintf("Stream selector %s matches no line in this window, though each of its label values is in the datasource: widen or move the window, or read list_label_values with the selector in metric to see which values occur together", sel.text)}
}

func allExistNote(sels []emptySelector) string {
	if len(sels) == 1 && sels[0].metric != "" {
		return fmt.Sprintf("Each label value of the selector exists on metric %s in this window, so either no series has them all together or the query's functions or label matching removed every series", sels[0].metric)
	}
	return "Every selector's metric and label values exist in this window, so either no series has a selector's values all together or the query's functions or label matching removed every series"
}

func literalOf(m *labels.Matcher) string {
	if m.Type == labels.MatchEqual {
		return m.Value
	}
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, m.Value)
}

func firstNames(names []string, n int) []string {
	return names[:min(len(names), n)]
}

func quotedList(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = strconv.Quote(clip(n, maxStepNameBytes))
	}
	return strings.Join(quoted, ", ")
}

func tokens(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '_' || r == '-' || r == '.' || r == ':' || r == '/' })
}

func closeness(target, candidate string) (int, bool) {
	t, c := strings.ToLower(target), strings.ToLower(candidate)
	switch {
	case t == c:
		return 0, false
	case strings.Join(tokens(t), "") == strings.Join(tokens(c), ""):
		return 0, true
	case strings.Contains(c, t) || len(c) > 2 && strings.Contains(t, c):
		return 1, true
	case slices.Contains(serviceKeys, t) && slices.Contains(serviceKeys, c):
		return 2, true
	}
	if own, shared := sharedTokens(t, c); shared > 0 && 2*shared >= own {
		return 2 + own - shared, true
	}
	if d := editDistance(t, c); d <= max(1, len(t)/3) {
		return 3 + d, true
	}
	return 0, false
}

func sharedTokens(target, candidate string) (own, shared int) {
	theirs := tokens(candidate)
	for _, tok := range tokens(target) {
		if len(tok) <= 2 {
			continue
		}
		own++
		if slices.Contains(theirs, tok) {
			shared++
		}
	}
	return own, shared
}

func closest(target string, candidates []string, n int) []string {
	type scored struct {
		name  string
		score int
	}
	var kept []scored
	for _, c := range candidates {
		if score, ok := closeness(target, c); ok {
			kept = append(kept, scored{c, score})
		}
	}
	slices.SortStableFunc(kept, func(a, b scored) int {
		return cmp.Or(cmp.Compare(a.score, b.score), cmp.Compare(len(a.name), len(b.name)), strings.Compare(a.name, b.name))
	})
	out := make([]string, 0, n)
	for _, k := range kept[:min(len(kept), n)] {
		out = append(out, k.name)
	}
	return out
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
