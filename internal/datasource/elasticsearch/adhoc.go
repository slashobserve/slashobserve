package elasticsearch

import (
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func adhocClauses(m *Mapping, filters []datasource.AdhocFilter) ([]any, error) {
	var clauses []any
	for _, groups := range datasource.AdhocScopes(filters) {
		alternatives := make([][]any, len(groups))
		for i, group := range groups {
			for _, f := range group {
				clause, err := adhocClause(m, f)
				if err != nil {
					return nil, err
				}
				alternatives[i] = append(alternatives[i], clause)
			}
		}
		if len(alternatives) == 1 {
			clauses = append(clauses, alternatives[0]...)
			continue
		}
		should := make([]any, len(alternatives))
		for i, terms := range alternatives {
			should[i] = map[string]any{"bool": map[string]any{"filter": terms}}
		}
		clauses = append(clauses, map[string]any{"bool": map[string]any{"should": should, "minimum_should_match": 1}})
	}
	return clauses, nil
}

func adhocClause(m *Mapping, f datasource.AdhocFilter) (map[string]any, error) {
	if f.Key == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "ad hoc filter has no field")
	}
	field, mapped := m.Fields[f.Key]
	exact := f.Key
	if mapped && field.Keyword != "" {
		exact = field.Keyword
	}
	switch f.Op {
	case "=", "!=":
		clause := map[string]any{"term": map[string]any{exact: f.Value}}
		if mapped && field.Kind == KindText && field.Keyword == "" {
			clause = map[string]any{"match_phrase": map[string]any{f.Key: f.Value}}
		}
		return negateIf(f.Op == "!=", clause), nil
	case "=~", "!~":
		clause := map[string]any{"regexp": map[string]any{exact: map[string]any{"value": f.Value, "case_insensitive": true}}}
		return negateIf(f.Op == "!~", clause), nil
	case "<", ">":
		bound := "lt"
		if f.Op == ">" {
			bound = "gt"
		}
		return map[string]any{"range": map[string]any{f.Key: map[string]any{bound: f.Value}}}, nil
	}
	return nil, datasource.Errorf(datasource.ErrInvalidQuery, "ad hoc filter operator %q is not supported", truncateText(f.Op))
}

func negateIf(negate bool, clause map[string]any) map[string]any {
	if !negate {
		return clause
	}
	return map[string]any{"bool": map[string]any{"must_not": []any{clause}}}
}
