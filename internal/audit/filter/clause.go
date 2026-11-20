package filter

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

const (
	maxClauses      = 1000
	maxClauseDepth  = 20
	maxTermsValues  = 1000
	queryStringTerm = `+-=&|><!(){}[]^"~*?:\/ `
	likeEscape      = " ESCAPE '\\'"
)

const supportedClauses = "bool, term, terms, match, match_phrase, range, wildcard, exists, multi_match, query_string, match_all and match_none"

type clauseError struct {
	msg   string
	field string
	free  bool
}

func (e *clauseError) Error() string { return e.msg }

func failOn(field, format string, args ...any) *clauseError {
	return &clauseError{msg: fmt.Sprintf(format, args...), field: field}
}

type compiler struct {
	b     *builder
	count int
}

func compileClause(b *builder, clause map[string]any) (string, error) {
	c := &compiler{b: b}
	return c.clause(clause, 0)
}

func (c *compiler) clause(raw any, depth int) (string, error) {
	c.count++
	if depth > maxClauseDepth || c.count > maxClauses {
		return "", errors.New("query too complex")
	}
	obj, ok := raw.(map[string]any)
	if !ok || len(obj) != 1 {
		return "", errors.New("a clause is an object with exactly one query type, such as bool or term")
	}
	for kind, body := range obj {
		return c.typed(kind, body, depth)
	}
	return "", nil
}

func (c *compiler) typed(kind string, body any, depth int) (string, error) {
	switch kind {
	case "bool":
		return c.boolClause(body, depth)
	case "term":
		return c.term(body)
	case "terms":
		return c.terms(body)
	case "match", "match_phrase":
		return c.match(kind, body)
	case "range":
		return c.rangeClause(body)
	case "wildcard":
		return c.wildcard(body)
	case "exists":
		return c.exists(body)
	case "multi_match":
		return c.multiMatch(body)
	case "query_string":
		return c.queryString(body)
	case "match_all", "match_none":
		return c.matchAll(kind, body)
	}
	return "", fmt.Errorf("unsupported query clause %q; supported: %s", truncate(kind), supportedClauses)
}

func truncate(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

func asObject(body any, what string) (map[string]any, error) {
	obj, ok := body.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s takes an object", what)
	}
	return obj, nil
}

func onlyKeys(obj map[string]any, what string, allowed ...string) error {
	for _, key := range slices.Sorted(maps.Keys(obj)) {
		if !slices.Contains(allowed, key) {
			return failOn(key, "unsupported key %q in %s; supported: %s", truncate(key), what, strings.Join(allowed, ", "))
		}
	}
	return nil
}

func singleField(body any, what string) (string, any, error) {
	obj, err := asObject(body, what)
	if err != nil {
		return "", nil, err
	}
	if len(obj) != 1 {
		return "", nil, fmt.Errorf("%s takes exactly one field", what)
	}
	for name, v := range obj {
		return name, v, nil
	}
	return "", nil, nil
}

func scalarText(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		return x.String(), true
	case bool:
		return strconv.FormatBool(x), true
	}
	return "", false
}

func fieldParam(name string, v any, key, what string, allowed ...string) (any, error) {
	obj, ok := v.(map[string]any)
	if !ok {
		return v, nil
	}
	if err := onlyKeys(obj, what+" on "+name, allowed...); err != nil {
		return nil, err
	}
	inner, ok := obj[key]
	if !ok {
		return nil, failOn(name, "%s on %s needs %q", what, name, key)
	}
	return inner, nil
}

func (c *compiler) field(name string) (*Field, error) {
	f, ok := lookupField(name)
	if !ok || strings.Contains(name, "*") {
		return nil, failOn(name, "%s", unknownFieldMessage(name))
	}
	return f, nil
}

func (c *compiler) fields(name string) ([]*Field, error) {
	if !strings.Contains(name, "*") {
		f, err := c.field(name)
		return []*Field{f}, err
	}
	found := matchingFields(name)
	if len(found) == 0 {
		return nil, failOn(name, "no field matches %q", truncate(name))
	}
	return found, nil
}

func (c *compiler) equalSQL(name string, f *Field, text string) (string, error) {
	conds, err := f.equal(text)
	if err != nil {
		return "", failOn(name, "%s", err.Error())
	}
	parts := make([]string, len(conds))
	for i, cd := range conds {
		parts[i] = cd.sql(c.b)
	}
	return leaf("(" + strings.Join(parts, " AND ") + ")"), nil
}

func leaf(expr string) string {
	return "COALESCE(" + expr + ", false)"
}

func (c *compiler) term(body any) (string, error) {
	name, v, err := singleField(body, "term")
	if err != nil {
		return "", err
	}
	if v, err = fieldParam(name, v, "value", "term", "value"); err != nil {
		return "", err
	}
	text, ok := scalarText(v)
	if !ok {
		return "", failOn(name, "term on %s needs a text or number value", name)
	}
	f, err := c.field(name)
	if err != nil {
		return "", err
	}
	return c.equalSQL(name, f, text)
}

func (c *compiler) match(kind string, body any) (string, error) {
	name, v, err := singleField(body, kind)
	if err != nil {
		return "", err
	}
	if v, err = fieldParam(name, v, "query", kind, "query"); err != nil {
		return "", err
	}
	text, ok := scalarText(v)
	if !ok {
		return "", failOn(name, "%s on %s needs a text or number value", kind, name)
	}
	f, err := c.field(name)
	if err != nil {
		return "", err
	}
	return c.equalSQL(name, f, text)
}

func (c *compiler) terms(body any) (string, error) {
	name, v, err := singleField(body, "terms")
	if err != nil {
		return "", err
	}
	list, ok := v.([]any)
	if !ok || len(list) == 0 || len(list) > maxTermsValues {
		return "", failOn(name, "terms on %s needs a list of 1 to %d values", name, maxTermsValues)
	}
	f, err := c.field(name)
	if err != nil {
		return "", err
	}
	alternatives := make([]string, len(list))
	for i, item := range list {
		text, ok := scalarText(item)
		if !ok {
			return "", failOn(name, "terms on %s needs text or number values", name)
		}
		if alternatives[i], err = c.equalSQL(name, f, text); err != nil {
			return "", err
		}
	}
	return "(" + strings.Join(alternatives, " OR ") + ")", nil
}

func (c *compiler) rangeClause(body any) (string, error) {
	name, v, err := singleField(body, "range")
	if err != nil {
		return "", err
	}
	bounds, err := asObject(v, "range on "+name)
	if err != nil {
		return "", err
	}
	if err := onlyKeys(bounds, "range on "+name, "gt", "gte", "lt", "lte"); err != nil {
		return "", err
	}
	if len(bounds) == 0 {
		return "", failOn(name, "range on %s needs gt, gte, lt or lte", name)
	}
	f, err := c.field(name)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(bounds))
	for _, key := range slices.Sorted(maps.Keys(bounds)) {
		text, ok := scalarText(bounds[key])
		if !ok {
			return "", failOn(name, "range on %s needs text or number bounds", name)
		}
		cd, err := f.ordered(key, text)
		if err != nil {
			return "", failOn(name, "%s", err.Error())
		}
		parts = append(parts, cd.sql(c.b))
	}
	return leaf("(" + strings.Join(parts, " AND ") + ")"), nil
}

func (c *compiler) wildcard(body any) (string, error) {
	name, v, err := singleField(body, "wildcard")
	if err != nil {
		return "", err
	}
	insensitive := false
	if obj, ok := v.(map[string]any); ok {
		if err := onlyKeys(obj, "wildcard on "+name, "value", "case_insensitive"); err != nil {
			return "", err
		}
		insensitive, _ = obj["case_insensitive"].(bool)
		v = obj["value"]
	}
	pattern, ok := v.(string)
	if !ok {
		return "", failOn(name, "wildcard on %s needs a text pattern", name)
	}
	f, err := c.field(name)
	if err != nil {
		return "", err
	}
	return c.likeSQL(f, likePattern(elasticsearchWildcard(pattern)), insensitive), nil
}

func (c *compiler) likeSQL(f *Field, pattern string, insensitive bool) string {
	op := " LIKE "
	if insensitive {
		op = " ILIKE "
	}
	return leaf(f.textExpr() + op + c.b.add(pattern) + likeEscape)
}

func (c *compiler) exists(body any) (string, error) {
	obj, err := asObject(body, "exists")
	if err != nil {
		return "", err
	}
	if err := onlyKeys(obj, "exists", "field"); err != nil {
		return "", err
	}
	name, ok := obj["field"].(string)
	if !ok {
		return "", errors.New("exists needs a field name")
	}
	found, err := c.fields(name)
	if err != nil {
		return "", err
	}
	parts := make([]string, len(found))
	for i, f := range found {
		parts[i] = f.nullColumn() + " IS NOT NULL"
	}
	return "(" + strings.Join(parts, " OR ") + ")", nil
}

func (c *compiler) matchAll(kind string, body any) (string, error) {
	obj, err := asObject(body, kind)
	if err != nil || len(obj) != 0 {
		return "", fmt.Errorf("%s takes an empty object", kind)
	}
	if kind == "match_none" {
		return "false", nil
	}
	return "true", nil
}

func (c *compiler) boolClause(body any, depth int) (string, error) {
	obj, err := asObject(body, "bool")
	if err != nil {
		return "", err
	}
	if err := onlyKeys(obj, "bool", "must", "filter", "should", "must_not", "minimum_should_match"); err != nil {
		return "", err
	}
	var parts []string
	for _, key := range []string{"must", "filter"} {
		sqls, err := c.clauseList(obj[key], depth)
		if err != nil {
			return "", err
		}
		parts = append(parts, sqls...)
	}
	required := len(parts) > 0
	negated, err := c.clauseList(obj["must_not"], depth)
	if err != nil {
		return "", err
	}
	for _, s := range negated {
		parts = append(parts, "NOT "+s)
	}
	mark := len(c.b.args)
	should, err := c.clauseList(obj["should"], depth)
	if err != nil {
		return "", err
	}
	if len(should) > 0 {
		atLeast, err := minimumShould(obj["minimum_should_match"], required)
		if err != nil {
			return "", err
		}
		switch s := shouldSQL(should, atLeast); s {
		case "", "false":
			c.b.args = c.b.args[:mark]
			if s != "" {
				parts = append(parts, s)
			}
		default:
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return "true", nil
	}
	return "(" + strings.Join(parts, " AND ") + ")", nil
}

func (c *compiler) clauseList(v any, depth int) ([]string, error) {
	var items []any
	switch x := v.(type) {
	case nil:
		return nil, nil
	case []any:
		items = x
	default:
		items = []any{x}
	}
	out := make([]string, len(items))
	for i, item := range items {
		s, err := c.clause(item, depth+1)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

func minimumShould(v any, hasRequired bool) (int, error) {
	if v == nil {
		if hasRequired {
			return 0, nil
		}
		return 1, nil
	}
	text, ok := scalarText(v)
	if !ok {
		return 0, errors.New("minimum_should_match must be a whole number")
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 0 {
		return 0, errors.New("minimum_should_match must be a whole number; percentages are not supported")
	}
	return n, nil
}

func shouldSQL(should []string, atLeast int) string {
	switch {
	case atLeast <= 0:
		return ""
	case atLeast > len(should):
		return "false"
	case atLeast == 1:
		return "(" + strings.Join(should, " OR ") + ")"
	}
	counted := make([]string, len(should))
	for i, s := range should {
		counted[i] = "(" + s + ")::int"
	}
	return "(" + strings.Join(counted, " + ") + " >= " + strconv.Itoa(atLeast) + ")"
}

func (c *compiler) multiMatch(body any) (string, error) {
	obj, err := asObject(body, "multi_match")
	if err != nil {
		return "", err
	}
	if err := onlyKeys(obj, "multi_match", "query", "fields", "type", "lenient"); err != nil {
		return "", err
	}
	text, ok := scalarText(obj["query"])
	if !ok {
		return "", errors.New("multi_match needs a query")
	}
	phrase := false
	if t, set := obj["type"]; set {
		switch t {
		case "phrase":
			phrase = true
		case "best_fields":
		default:
			return "", errors.New("multi_match supports the types best_fields and phrase")
		}
	}
	names, err := parseFieldList(obj["fields"])
	if err != nil {
		return "", err
	}
	if names == nil {
		return c.freeText(text, phrase), nil
	}
	lenient, _ := obj["lenient"].(bool)
	return c.acrossFields(names, text, lenient)
}

func parseFieldList(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return nil, errors.New("fields must be a list of field names")
	}
	names := make([]string, len(list))
	for i, item := range list {
		s, ok := item.(string)
		if !ok || strings.Contains(s, "^") {
			return nil, errors.New("fields must be a list of plain field names; boosts are not supported")
		}
		names[i] = s
	}
	return names, nil
}

func (c *compiler) acrossFields(names []string, text string, lenient bool) (string, error) {
	seen := map[*Field]bool{}
	var alternatives []string
	for _, name := range names {
		found, err := c.fields(name)
		if err != nil {
			return "", err
		}
		for _, f := range found {
			if seen[f] {
				continue
			}
			seen[f] = true
			s, err := c.equalSQL(name, f, text)
			switch {
			case err == nil:
				alternatives = append(alternatives, s)
			case !lenient:
				return "", err
			}
		}
	}
	if len(alternatives) == 0 {
		return "false", nil
	}
	return "(" + strings.Join(alternatives, " OR ") + ")", nil
}

func (c *compiler) freeText(text string, phrase bool) string {
	fn := "plainto_tsquery"
	if phrase {
		fn = "phraseto_tsquery"
	}
	return "(search @@ " + fn + "('simple', " + c.b.add(text) + "))"
}

var tsqueryQuoter = strings.NewReplacer(`\`, `\\`, `'`, `''`)

func (c *compiler) queryString(body any) (string, error) {
	obj, err := asObject(body, "query_string")
	if err != nil {
		return "", err
	}
	if err := onlyKeys(obj, "query_string", "query", "fields", "lenient", "analyze_wildcard"); err != nil {
		return "", err
	}
	q, ok := obj["query"].(string)
	if !ok {
		return "", errors.New("query_string needs a query")
	}
	pieces, err := queryStringPieces(q)
	if err != nil {
		return "", err
	}
	names, err := parseFieldList(obj["fields"])
	if err != nil {
		return "", err
	}
	if names == nil {
		return c.freeWildcard(pieces)
	}
	seen := map[*Field]bool{}
	var alternatives []string
	for _, name := range names {
		found, err := c.fields(name)
		if err != nil {
			return "", err
		}
		for _, f := range found {
			if !seen[f] {
				seen[f] = true
				alternatives = append(alternatives, c.likeSQL(f, likePattern(pieces), true))
			}
		}
	}
	return "(" + strings.Join(alternatives, " OR ") + ")", nil
}

func queryStringPieces(q string) ([]wildcardPiece, error) {
	var pieces []wildcardPiece
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			pieces = append(pieces, wildcardPiece{text: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(q); i++ {
		switch c := q[i]; {
		case c == '\\' && i+1 < len(q):
			i++
			lit.WriteByte(q[i])
		case c == '*':
			flush()
			pieces = append(pieces, wildcardPiece{kind: pieceMany})
		case strings.IndexByte(queryStringTerm, c) >= 0:
			return nil, &clauseError{msg: "query_string accepts only words and * wildcards; Lucene syntax is not supported, use KQL", free: true}
		default:
			lit.WriteByte(c)
		}
	}
	flush()
	return pieces, nil
}

func (c *compiler) freeWildcard(pieces []wildcardPiece) (string, error) {
	switch {
	case len(pieces) == 1 && pieces[0].kind == pieceMany:
		return "true", nil
	case len(pieces) == 1 && pieces[0].kind == pieceText:
		return c.freeText(pieces[0].text, false), nil
	case len(pieces) == 2 && pieces[0].kind == pieceText && pieces[1].kind == pieceMany:
		arg := "'" + tsqueryQuoter.Replace(pieces[0].text) + "':*"
		return "(search @@ to_tsquery('simple', " + c.b.add(arg) + "))", nil
	}
	return "", &clauseError{msg: "a wildcard in free text must be at the end of a word, as in check*; use field:*value to match inside a field", free: true}
}
