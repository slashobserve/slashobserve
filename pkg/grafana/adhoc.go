package grafana

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxAdhocDefaults        = 20
	maxAdhocValueRunes      = 500
	maxAdhocValueBytes      = 1024
	maxAdhocKeyLabelRunes   = 200
	maxAdhocValueLabelRunes = 500
	datasourceOr            = "or"
)

var (
	adhocOperators           = []string{"=", "!=", "=~", "!~"}
	adhocClickHouseOperators = append(slices.Clone(adhocOperators), "<", ">")
)

type adhocProblem struct{ reason, path string }

func (c *converter) adhocDefault(raw map[string]any, path, subject, kind string) []spec.AdhocFilter {
	return c.adhocFilterList(raw, "filters", path, subject, kind)
}

func (c *converter) adhocFilterList(raw map[string]any, key, path, subject, kind string) []spec.AdhocFilter {
	list, _ := asArray(raw[key])
	var filters []spec.AdhocFilter
	var problems []adhocProblem
	for i, item := range list {
		at := fmt.Sprintf("%s.%s[%d]", path, key, i)
		f, found := adhocFilterOf(item, at, kind)
		problems = append(problems, found...)
		switch {
		case len(found) > 0 && f.Key == "":
		case len(filters) >= maxAdhocDefaults:
			problems = append(problems, adhocProblem{fmt.Sprintf("SlashObserve keeps at most %d saved filters per variable.", maxAdhocDefaults), at})
		default:
			filters = append(filters, f)
		}
	}
	c.noteAdhocProblems(subject, problems)
	return filters
}

func (c *converter) noteAdhocProblems(subject string, problems []adhocProblem) {
	var reasons []string
	for _, p := range problems {
		if !slices.Contains(reasons, p.reason) {
			reasons = append(reasons, p.reason)
		}
	}
	for _, reason := range reasons {
		var paths []string
		for _, p := range problems {
			if p.reason == reason {
				paths = append(paths, p.path)
			}
		}
		c.note(subject, reason, paths...)
	}
}

func adhocFilterOf(item any, at, kind string) (spec.AdhocFilter, []adhocProblem) {
	obj, ok := asObject(item)
	if !ok {
		return spec.AdhocFilter{}, []adhocProblem{{"A saved filter is not an object and is not kept.", at}}
	}
	f := spec.AdhocFilter{Key: asString(obj["key"]), Operator: asString(obj["operator"])}
	value, hasValue := obj["value"].(string)
	f.Value = value
	problems := adhocFilterExtras(&f, obj, at, kind)
	if reason := adhocFilterRefusal(f, hasValue, kind); reason != "" {
		return spec.AdhocFilter{}, append(problems, adhocProblem{reason, at})
	}
	return f, problems
}

func adhocFilterExtras(f *spec.AdhocFilter, obj map[string]any, at, kind string) []adhocProblem {
	var problems []adhocProblem
	add := func(reason, key string) { problems = append(problems, adhocProblem{reason, at + "." + key}) }
	for _, key := range sortedKeys(obj) {
		switch key {
		case "key", "operator", "value":
		case "condition":
			if reason := adhocCondition(f, asString(obj[key]), kind); reason != "" {
				add(reason, key)
			}
		case "keyLabel":
			if label, ok := adhocLabel(obj[key], maxAdhocKeyLabelRunes); ok {
				f.KeyLabel = label
			} else if present(obj[key]) {
				add("A saved filter's key label that is not text of 1 to 200 characters is not kept.", key)
			}
		case "valueLabels":
			if reason := adhocValueLabels(f, obj[key]); reason != "" {
				add(reason, key)
			}
		case "values":
			if present(obj[key]) && !adhocValuesMatch(f, obj[key]) {
				add("A saved filter's list of values that is not its regular expression's alternatives joined with | is not kept.", key)
			}
		default:
			if present(obj[key]) {
				add("Some options of the saved filters are not supported yet.", key)
			}
		}
	}
	return problems
}

func adhocCondition(f *spec.AdhocFilter, condition, kind string) string {
	switch {
	case condition == "" || strings.EqualFold(condition, "AND"):
		return ""
	case strings.EqualFold(condition, "OR") && kind == kindClickHouse:
		f.Condition = datasourceOr
		return ""
	}
	return fmt.Sprintf("SlashObserve combines saved filters with AND; %q is not kept.", condition)
}

func adhocValueLabels(f *spec.AdhocFilter, v any) string {
	labels, _ := asArray(v)
	label, ok := "", false
	if len(labels) > 0 {
		label, ok = adhocLabel(labels[0], maxAdhocValueLabelRunes)
	}
	if ok {
		f.ValueLabel = label
	}
	switch {
	case present(v) && !ok:
		return "A saved filter's value label that is not text of 1 to 500 characters is not kept."
	case len(labels) > 1:
		return "Only the first value label of a saved filter is kept."
	}
	return ""
}

func adhocLabel(v any, maxRunes int) (string, bool) {
	label, ok := v.(string)
	return label, ok && label != "" && utf8.RuneCountInString(label) <= maxRunes
}

func adhocValuesMatch(f *spec.AdhocFilter, v any) bool {
	list, ok := asArray(v)
	if !ok || (f.Operator != "=~" && f.Operator != "!~") {
		return false
	}
	values := make([]string, len(list))
	for i, item := range list {
		if values[i], ok = item.(string); !ok {
			return false
		}
	}
	return strings.Join(values, "|") == f.Value
}

func adhocFilterRefusal(f spec.AdhocFilter, hasValue bool, kind string) string {
	switch {
	case f.Key == "" || f.Operator == "" || !hasValue:
		return "A saved filter without a key, an operator and a value is not kept."
	case !slices.Contains(adhocOperatorsFor(kind), f.Operator):
		return adhocOperatorRefusal(f.Operator, kind)
	case !validAdhocKey(f.Key, kind):
		return "A saved filter whose key is not a label or column name of its datasource is not kept."
	case utf8.RuneCountInString(f.Value) > maxAdhocValueRunes || len(f.Value) > maxAdhocValueBytes || hasControlRune(f.Value):
		return "A saved filter whose value is longer than 500 characters or 1024 bytes, or holds a control character, is not kept."
	case (f.Operator == "=~" || f.Operator == "!~") && !compilesAsRegexp(f.Value):
		return "A saved filter whose regular expression is not valid is not kept."
	}
	return ""
}

func adhocOperatorsFor(kind string) []string {
	if kind == kindClickHouse {
		return adhocClickHouseOperators
	}
	return adhocOperators
}

func adhocOperatorRefusal(op, kind string) string {
	if kind == kindClickHouse {
		return fmt.Sprintf("SlashObserve supports the filter operators =, !=, =~, !~, < and > on ClickHouse; saved filters that use %s are not kept.", op)
	}
	return fmt.Sprintf("SlashObserve supports the filter operators =, !=, =~ and !~; saved filters that use %s are not kept.", op)
}

func validAdhocKey(key, kind string) bool {
	if kind == kindClickHouse {
		return spec.IsColumnArg(key)
	}
	return spec.IsLabelName(key)
}

func hasControlRune(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

func compilesAsRegexp(pattern string) bool {
	_, err := regexp.Compile(pattern)
	return err == nil
}

func (c *converter) noteClickHouseAdhoc() {
	for _, v := range c.vars {
		src := c.adhocSources[v.v.Name]
		if v.v.Type != spec.AdhocVariable || src == nil || src.typ != altinityPlugin {
			continue
		}
		unfiltered, count := c.queriesWithout(v.v.Name, src)
		if count == 0 {
			continue
		}
		c.note("variable "+v.v.Name, "Grafana's Altinity plugin applies these filters to the outermost query only; SlashObserve applies them on every table a query reads, subqueries included.", unfiltered...)
	}
}

func (c *converter) queriesWithout(variable string, src *sourceDS) ([]string, int) {
	var paths []string
	count := 0
	visit := func(bp *builtPanel) {
		for _, bq := range bp.queries {
			if bq.src != src || usesVariable(bq.q.Query, variable) {
				continue
			}
			count++
			if len(paths) < maxPanelPaths {
				paths = append(paths, bp.node.path+fmt.Sprintf(".targets[%d]", bq.index))
			}
		}
	}
	for _, bp := range c.built {
		visit(bp)
		for _, child := range bp.children {
			visit(child)
		}
	}
	return paths, count
}

func usesVariable(query, name string) bool {
	refs, err := spec.ScanQuery(query)
	if err != nil {
		return false
	}
	for _, ref := range spec.FlattenReferences(refs) {
		if ref.Kind == spec.VariableRef && ref.Name == name {
			return true
		}
	}
	return false
}
