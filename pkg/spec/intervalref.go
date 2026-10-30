package spec

import (
	"fmt"
	"strconv"
)

func IntervalVariableName(ref string) (string, bool) {
	m := bracedVariableRef.FindStringSubmatch(ref)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func checkIntervalRefs(d *Dashboard) []Issue {
	types := map[string]VariableType{}
	for _, v := range d.Variables {
		if _, dup := types[v.Name]; !dup {
			types[v.Name] = v.Type
		}
	}

	var issues []Issue
	check := func(ref, path string) {
		name, ok := IntervalVariableName(ref)
		if !ok {
			return
		}
		kind, declared := types[name]
		switch {
		case !declared:
			issues = append(issues, Issue{
				Path:    path,
				Code:    CodeUnknownVariable,
				Message: fmt.Sprintf("interval references undeclared variable %q; declare an interval variable with that name or write a duration", name),
			})
		case kind != IntervalVariable:
			issues = append(issues, Issue{
				Path:    path,
				Code:    CodeInvalidReference,
				Message: fmt.Sprintf("interval ${%s} must name an interval variable, but %q is a %s variable", name, name, kind),
			})
		}
	}

	for _, e := range d.DataPanels() {
		check(e.Panel.Interval, e.Pointer+"/interval")
		for j, q := range e.Panel.Queries {
			check(q.Interval, e.Pointer+pointer("queries", strconv.Itoa(j), "interval"))
		}
	}
	return issues
}
