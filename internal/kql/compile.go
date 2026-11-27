package kql

import (
	"errors"
	"strconv"
	"strings"
)

var rangeKeys = map[string]string{">": "gt", ">=": "gte", "<": "lt", "<=": "lte"}

const queryStringSpecials = `+-=&|><!(){}[]^"~*?:\/ `

type compiler struct {
	fields     Fields
	knownField func(string) bool
	seen       map[string]int
	warnings   []Diagnostic
	unknown    []UnknownField
	optional   int
}

func Compile(text string, fields Fields) (Compiled, error) {
	return CompileKnowing(text, fields, nil)
}

func CompileKnowing(text string, fields Fields, knownField func(string) bool) (Compiled, error) {
	root, diags := Parse(text)
	if len(diags) > 0 {
		return Compiled{Diagnostics: diags}, errors.New(diags[0].Message)
	}
	if root == nil {
		return Compiled{}, nil
	}
	c := &compiler{fields: fields, knownField: knownField, seen: map[string]int{}}
	query := c.clause(root)
	return Compiled{Query: query, Diagnostics: c.warnings, UnknownFields: c.unknown}, nil
}

func (c *compiler) clause(n *Node) map[string]any {
	switch n.Kind {
	case And:
		return map[string]any{"bool": map[string]any{"filter": c.clauses(n.Children)}}
	case Or:
		return map[string]any{"bool": map[string]any{"should": c.optionalClauses(n.Children), "minimum_should_match": 1}}
	case Not:
		return map[string]any{"bool": map[string]any{"must_not": c.optionalClauses(n.Children)}}
	case Group:
		return c.clause(n.Children[0])
	case Match:
		c.noteField(n)
		return c.match(n)
	case Range:
		c.noteField(n)
		return map[string]any{"range": map[string]any{n.Field: map[string]any{rangeKeys[n.Op]: n.Value}}}
	case Exists:
		c.noteField(n)
		return map[string]any{"exists": map[string]any{"field": n.Field}}
	}
	return free(n)
}

func (c *compiler) clauses(nodes []*Node) []any {
	out := make([]any, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, c.clause(n))
	}
	return out
}

func (c *compiler) optionalClauses(nodes []*Node) []any {
	c.optional++
	defer func() { c.optional-- }()
	return c.clauses(nodes)
}

func (c *compiler) match(n *Node) map[string]any {
	if strings.Contains(n.Field, "*") {
		return acrossFields(n)
	}
	switch {
	case n.Wildcard:
		return map[string]any{"wildcard": map[string]any{n.Field: map[string]any{"value": wildcardPattern(n.segments), "case_insensitive": true}}}
	case n.Quoted && c.fields[n.Field] == Text:
		return map[string]any{"match_phrase": map[string]any{n.Field: n.Value}}
	}
	return map[string]any{"match": map[string]any{n.Field: n.Value}}
}

func acrossFields(n *Node) map[string]any {
	if n.Wildcard {
		return map[string]any{"query_string": map[string]any{"query": queryStringPattern(n.segments), "fields": []any{n.Field}, "lenient": true, "analyze_wildcard": true}}
	}
	return map[string]any{"multi_match": map[string]any{"query": n.Value, "fields": []any{n.Field}, "type": matchType(n), "lenient": true}}
}

func free(n *Node) map[string]any {
	if n.Wildcard {
		return map[string]any{"query_string": map[string]any{"query": queryStringPattern(n.segments), "lenient": true, "analyze_wildcard": true}}
	}
	return map[string]any{"multi_match": map[string]any{"query": n.Value, "type": matchType(n), "lenient": true}}
}

func matchType(n *Node) string {
	if n.Quoted {
		return "phrase"
	}
	return "best_fields"
}

func (c *compiler) noteField(n *Node) {
	if c.fields == nil {
		return
	}
	required := c.optional == 0
	if at, seen := c.seen[n.Field]; seen {
		if at >= 0 && required {
			c.unknown[at].Required = true
		}
		return
	}
	c.seen[n.Field] = -1
	if c.known(n.Field) {
		return
	}
	near := c.sameEnding(n.Field)
	message := "unknown field " + strconv.Quote(n.Field)
	if near != "" {
		message += "; did you mean " + strconv.Quote(near) + "?"
	}
	c.warnings = append(c.warnings, Diagnostic{Severity: SeverityWarning, Message: message, Start: n.fieldStart, End: n.fieldEnd})
	c.seen[n.Field] = len(c.unknown)
	c.unknown = append(c.unknown, UnknownField{Name: n.Field, Suggestion: near, Required: required})
}

func (c *compiler) sameEnding(field string) string {
	if strings.Contains(field, "*") {
		return ""
	}
	lower := strings.ToLower(field)
	var found string
	for name := range c.fields {
		other := strings.ToLower(name)
		if name == field || strings.HasSuffix(name, ".keyword") {
			continue
		}
		if other != lower && !strings.HasSuffix(other, "."+lower) && !strings.HasSuffix(lower, "."+other) {
			continue
		}
		if found != "" {
			return ""
		}
		found = name
	}
	return found
}

func (c *compiler) known(field string) bool {
	if c.knownField != nil {
		return c.knownField(field)
	}
	if !strings.Contains(field, "*") {
		_, ok := c.fields[field]
		return ok
	}
	for name := range c.fields {
		if globMatch(field, name) {
			return true
		}
	}
	return false
}

func globMatch(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	name = name[len(parts[0]):]
	for i, part := range parts[1:] {
		at := strings.Index(name, part)
		if at < 0 {
			return false
		}
		if i == len(parts)-2 {
			return strings.HasSuffix(name, part)
		}
		name = name[at+len(part):]
	}
	return true
}

func wildcardPattern(segs []segment) string {
	return pattern(segs, `\*?`)
}

func queryStringPattern(segs []segment) string {
	return pattern(segs, queryStringSpecials)
}

func pattern(segs []segment, specials string) string {
	var b strings.Builder
	for _, s := range segs {
		if s.star {
			b.WriteByte('*')
			continue
		}
		for i := 0; i < len(s.text); i++ {
			if strings.IndexByte(specials, s.text[i]) >= 0 {
				b.WriteByte('\\')
			}
			b.WriteByte(s.text[i])
		}
	}
	return b.String()
}
