package elasticsearch

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/kql"
)

const (
	hintFields  = 3
	hintValues  = 10
	shownValues = 5
)

type valueClause struct {
	field string
	value string
}

func valueClauses(body string, m *Mapping) []valueClause {
	root, diagnostics := kql.Parse(body)
	if root == nil || len(diagnostics) > 0 {
		return nil
	}
	var clauses []valueClause
	var walk func(n *kql.Node)
	walk = func(n *kql.Node) {
		switch n.Kind {
		case kql.And, kql.Group:
			for _, child := range n.Children {
				walk(child)
			}
		case kql.Match:
			f, ok := m.Fields[n.Field]
			if ok && !n.Wildcard && f.Kind == KindKeyword && f.Aggregatable && !f.Conflict {
				clauses = append(clauses, valueClause{field: n.Field, value: n.Value})
			}
		}
	}
	walk(root)
	return clauses
}

func (d *Datasource) withValueHint(ctx context.Context, prep prepared, r datasource.TimeRange, frames []datasource.Frame) []datasource.Frame {
	if prep.form != FormKQL || len(frames) != 1 || frames[0].Rows() != 0 || frames[0].Meta == nil || frames[0].Meta.Partial {
		return frames
	}
	frames[0].Meta.Hint = cmp.Or(unknownFieldHint(prep), d.valueHint(ctx, prep, r))
	return frames
}

func unknownFieldHint(prep prepared) string {
	compiled, err := kql.CompileKnowing(prep.body, kqlFields(prep.mapping), prep.mapping.KnownField)
	if err != nil {
		return ""
	}
	var texts []string
	for _, diag := range compiled.Diagnostics {
		rest, ok := strings.CutPrefix(diag.Message, "unknown field ")
		if !ok {
			continue
		}
		field, suggestion, found := strings.Cut(rest, "; ")
		text := fmt.Sprintf("Field %s is not in the mapping", field)
		if found {
			text += "; " + suggestion
		} else {
			text += "."
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, " ")
}

func (d *Datasource) valueHint(ctx context.Context, prep prepared, r datasource.TimeRange) string {
	clauses := valueClauses(prep.body, prep.mapping)
	var fields []string
	for _, c := range clauses {
		if !slices.Contains(fields, c.field) {
			fields = append(fields, c.field)
		}
	}
	if len(fields) == 0 || len(fields) > hintFields {
		return ""
	}
	everything := userQuery{clause: matchAll()}
	body := buildBody(everything, d.opts.TimeField, r, nil, plan{kind: searchCount, step: r.To.Sub(r.From), timeout: prep.lim.Timeout})
	aggs := make(map[string]any, 2*len(fields))
	for _, name := range fields {
		aggs[name] = map[string]any{"terms": map[string]any{"field": name, "size": hintValues}}
		aggs[name+cardinalitySuffix] = map[string]any{"cardinality": map[string]any{"field": name}}
	}
	body["aggs"] = aggs
	raw, err := d.search(ctx, body)
	if err != nil {
		return ""
	}
	resp, err := parseSearch(raw)
	if err != nil || resp.partial() {
		return ""
	}
	for _, c := range clauses {
		var field datasource.FacetField
		field.Name = c.field
		if fillFacetField(&field, resp) != nil {
			continue
		}
		seen := make([]string, len(field.Values))
		for i, v := range field.Values {
			seen[i] = v.Value
		}
		if text := valueMissText(c, seen, field.Cardinality <= len(seen)); text != "" {
			return text
		}
	}
	return ""
}

func valueMissText(c valueClause, seen []string, complete bool) string {
	if len(seen) == 0 || slices.Contains(seen, c.value) {
		return ""
	}
	sameLetters := slices.IndexFunc(seen, func(v string) bool { return strings.EqualFold(v, c.value) })
	if sameLetters < 0 && !complete {
		return ""
	}
	listed := strings.Join(seen[:min(len(seen), shownValues)], ", ")
	head := fmt.Sprintf("No document has %s = %s in this range", c.field, c.value)
	if sameLetters < 0 {
		return fmt.Sprintf("%s; values seen: %s.", head, listed)
	}
	return fmt.Sprintf("%s, but it has %s; values are case sensitive, so seen: %s.", head, seen[sameLetters], listed)
}
