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
	maxAdhocDefaults   = 20
	maxAdhocValueRunes = 500
	maxAdhocValueBytes = 1024
)

var adhocOperators = []string{"=", "!=", "=~", "!~"}

type adhocProblem struct{ reason, path string }

func (c *converter) adhocDefault(raw map[string]any, path, subject, kind string) []spec.AdhocFilter {
	list, _ := asArray(raw["filters"])
	var filters []spec.AdhocFilter
	var problems []adhocProblem
	for i, item := range list {
		at := fmt.Sprintf("%s.filters[%d]", path, i)
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
	problems := adhocFilterExtras(obj, at)
	f := spec.AdhocFilter{Key: asString(obj["key"]), Operator: asString(obj["operator"])}
	value, hasValue := obj["value"].(string)
	f.Value = value
	if reason := adhocFilterRefusal(f, hasValue, kind); reason != "" {
		return spec.AdhocFilter{}, append(problems, adhocProblem{reason, at})
	}
	return f, problems
}

func adhocFilterExtras(obj map[string]any, at string) []adhocProblem {
	var problems []adhocProblem
	for _, key := range sortedKeys(obj) {
		switch key {
		case "key", "operator", "value":
		case "condition":
			if condition := asString(obj[key]); condition != "" && !strings.EqualFold(condition, "AND") {
				problems = append(problems, adhocProblem{fmt.Sprintf("SlashObserve combines saved filters with AND; %q is not kept.", condition), at + ".condition"})
			}
		default:
			if present(obj[key]) {
				problems = append(problems, adhocProblem{"Some options of the saved filters are not supported yet.", at + "." + key})
			}
		}
	}
	return problems
}

func adhocFilterRefusal(f spec.AdhocFilter, hasValue bool, kind string) string {
	switch {
	case f.Key == "" || f.Operator == "" || !hasValue:
		return "A saved filter without a key, an operator and a value is not kept."
	case !slices.Contains(adhocOperators, f.Operator):
		return fmt.Sprintf("SlashObserve supports the filter operators =, !=, =~ and !~; saved filters that use %s are not kept.", f.Operator)
	case !validAdhocKey(f.Key, kind):
		return "A saved filter whose key is not a label or column name of its datasource is not kept."
	case utf8.RuneCountInString(f.Value) > maxAdhocValueRunes || len(f.Value) > maxAdhocValueBytes || hasControlRune(f.Value):
		return "A saved filter whose value is longer than 500 characters or 1024 bytes, or holds a control character, is not kept."
	case (f.Operator == "=~" || f.Operator == "!~") && !compilesAsRegexp(f.Value):
		return "A saved filter whose regular expression is not valid is not kept."
	}
	return ""
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
		if v.v.Type != spec.AdhocVariable || src == nil || src.kind != kindClickHouse {
			continue
		}
		unfiltered, count := c.queriesWithout(v.v.Name, src)
		if count == 0 {
			continue
		}
		c.note("variable "+v.v.Name, fmt.Sprintf("Grafana applies these filters to every query on this datasource; SlashObserve applies them only in queries that use $%s, so %s not filtered.", v.v.Name, unfilteredQueries(count)), unfiltered...)
	}
}

func unfilteredQueries(count int) string {
	if count == 1 {
		return "1 query is"
	}
	return fmt.Sprintf("%d queries are", count)
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
