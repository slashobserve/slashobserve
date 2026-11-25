package elasticsearch

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/kql"
)

const (
	hintFields     = 3
	hintValues     = 10
	hintTextFields = 5
	shownValues    = 5
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

func (d *Datasource) withHint(ctx context.Context, prep prepared, r datasource.TimeRange, frames []datasource.Frame, counted bool) []datasource.Frame {
	if prep.form != FormKQL || len(frames) != 1 || !hasNoHits(frames[0]) || (frames[0].Meta != nil && frames[0].Meta.Partial) {
		return frames
	}
	hint := d.unknownFieldHint(prep)
	if hint == "" && !counted {
		hint = d.valueHint(ctx, prep, r)
	}
	if hint == "" {
		return frames
	}
	if frames[0].Meta == nil {
		frames[0].Meta = &datasource.FrameMeta{}
	}
	frames[0].Meta.Hint = hint
	return frames
}

func hasNoHits(f datasource.Frame) bool {
	for _, field := range f.Fields {
		if field.Type != datasource.FieldTime && slices.ContainsFunc(field.Values, func(v any) bool { return v != nil && v != float64(0) }) {
			return false
		}
	}
	return true
}

func (d *Datasource) unknownFieldHint(prep prepared) string {
	compiled, err := kql.CompileKnowing(prep.body, kqlFields(prep.mapping), prep.mapping.KnownField)
	if err != nil {
		return ""
	}
	var texts []string
	for _, f := range compiled.UnknownFields {
		if !f.Required {
			continue
		}
		text := fmt.Sprintf("Field %s is not in the mapping", strconv.Quote(f.Name))
		switch tryFields := d.textFieldsToTry(prep.mapping); {
		case f.Suggestion != "":
			text += fmt.Sprintf("; did you mean %s?", strconv.Quote(f.Suggestion))
		case len(tryFields) > 0:
			text += "; text fields to try: " + strings.Join(tryFields, ", ") + "."
		default:
			text += "."
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, " ")
}

func (d *Datasource) textFieldsToTry(m *Mapping) []string {
	var names []string
	if roles, err := m.roles(d.opts.Fields); err == nil && m.Fields[roles.Message].Kind == KindText {
		names = append(names, roles.Message)
	}
	for _, name := range m.Names() {
		if m.Fields[name].Kind == KindText && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names[:min(len(names), hintTextFields)]
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
