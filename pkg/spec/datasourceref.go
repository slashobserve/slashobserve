package spec

import (
	"fmt"
	"regexp"
	"strconv"
)

var bracedVariableRef = regexp.MustCompile(`^\$\{([A-Za-z][A-Za-z0-9_]{0,39})\}$`)

func DatasourceVariableName(ref string) (string, bool) {
	m := bracedVariableRef.FindStringSubmatch(ref)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func checkDatasourceRefs(d *Dashboard) []Issue {
	index := map[string]int{}
	types := map[string]VariableType{}
	for i, v := range d.Variables {
		if _, dup := index[v.Name]; dup {
			continue
		}
		index[v.Name] = i
		types[v.Name] = v.Type
	}

	var issues []Issue
	check := func(ref, path string, variable int) {
		name, ok := DatasourceVariableName(ref)
		if !ok {
			return
		}
		at, declared := index[name]
		switch {
		case !declared:
			issues = append(issues, Issue{
				Path:    path,
				Code:    CodeUnknownVariable,
				Message: fmt.Sprintf("datasource references undeclared variable %q; declare a datasource variable with that name or use a datasource uid", name),
			})
		case types[name] != DatasourceVariable:
			issues = append(issues, Issue{
				Path:    path,
				Code:    CodeInvalidReference,
				Message: fmt.Sprintf("datasource ${%s} must name a datasource variable, but %q is a %s variable", name, name, types[name]),
			})
		case variable >= 0 && at >= variable:
			issues = append(issues, Issue{
				Path:    path,
				Code:    CodeVariableOrder,
				Message: fmt.Sprintf("datasource ${%s} is declared at %s; a variable can only use datasource variables declared before it", name, pointer("variables", strconv.Itoa(at))),
			})
		}
	}

	for i, v := range d.Variables {
		if v.Type == QueryVariable || v.Type == AdhocVariable {
			check(v.Datasource, pointer("variables", strconv.Itoa(i), "datasource"), i)
		}
	}
	for _, e := range d.AllPanels() {
		if !e.Panel.Type.IsData() {
			continue
		}
		check(e.Panel.Datasource, e.Pointer+"/datasource", -1)
		for j, q := range e.Panel.Queries {
			if q.Datasource != "" {
				check(q.Datasource, e.Pointer+pointer("queries", strconv.Itoa(j), "datasource"), -1)
			}
		}
	}
	return issues
}
