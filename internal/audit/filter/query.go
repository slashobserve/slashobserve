package filter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/slashobserve/slashobserve/internal/kql"
)

type Form = kql.Form

const (
	FormKQL    = kql.FormKQL
	FormLucene = kql.FormLucene
	FormDSL    = kql.FormDSL
)

const (
	maxQueryBytes = 64 * 1024
	maxSize       = 1000
)

type Error struct {
	Pos int
	End int
	Msg string
}

func (e *Error) Error() string {
	return fmt.Sprintf("at byte %d: %s", e.Pos, e.Msg)
}

type Query struct {
	Form   Form
	Clause map[string]any
	Aggs   []Agg
	Size   *int

	Warnings []string
}

func Parse(text string) (Query, error) {
	if len(text) > maxQueryBytes {
		return Query{}, &Error{End: 0, Msg: fmt.Sprintf("query is longer than %d bytes", maxQueryBytes)}
	}
	form, body, offset := kql.Classify(text, kql.FormKQL)
	switch form {
	case FormLucene:
		return parseLucene(body, offset)
	case FormDSL:
		return parseDSL(body, offset)
	}
	return parseKQL(body, offset)
}

func parseKQL(body string, offset int) (Query, error) {
	compiled, err := kql.Compile(body, kqlFields())
	if err != nil {
		for _, d := range compiled.Diagnostics {
			if d.Severity == kql.SeverityError {
				return Query{}, &Error{Pos: offset + d.Start, End: offset + d.End, Msg: d.Message}
			}
		}
		return Query{}, &Error{Pos: offset, End: offset + len(body), Msg: err.Error()}
	}
	q := Query{Form: FormKQL}
	if compiled.Query != nil {
		var err error
		if q.Clause, err = normalizeClause(compiled.Query); err != nil {
			return Query{}, &Error{Pos: offset, End: offset + len(body), Msg: err.Error()}
		}
	}
	if err := q.dryRun(); err != nil {
		start, end := kqlSpan(body, err)
		return Query{}, &Error{Pos: offset + start, End: offset + end, Msg: err.Error()}
	}
	q.Warnings = kqlWarnings(body)
	return q, nil
}

func normalizeClause(clause map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(clause)
	if err != nil {
		return nil, err
	}
	return decodeObject(string(raw))
}

func (q Query) dryRun() error {
	if q.Clause == nil {
		return nil
	}
	_, err := compileClause(&builder{}, q.Clause)
	return err
}

func kqlSpan(body string, err error) (int, int) {
	var ce *clauseError
	root, _ := kql.Parse(body)
	if errors.As(err, &ce) && root != nil {
		if n := findNode(root, ce); n != nil {
			return n.Start, n.End
		}
	}
	return 0, len(body)
}

func findNode(n *kql.Node, ce *clauseError) *kql.Node {
	switch {
	case ce.free && n.Kind == kql.Free && n.Wildcard:
		return n
	case !ce.free && ce.field != "" && n.Field == ce.field && n.Kind != kql.Free:
		return n
	}
	for _, child := range n.Children {
		if found := findNode(child, ce); found != nil {
			return found
		}
	}
	return nil
}

func parseDSL(body string, offset int) (Query, error) {
	top, err := decodeObject(body)
	if err != nil {
		var syn *json.SyntaxError
		var typ *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syn):
			return Query{}, &Error{Pos: offset + int(syn.Offset), End: offset + int(syn.Offset), Msg: "invalid JSON: " + syn.Error()}
		case errors.As(err, &typ):
			return Query{}, &Error{Pos: offset + int(typ.Offset), End: offset + int(typ.Offset), Msg: "invalid JSON: " + typ.Error()}
		}
		return Query{}, &Error{Pos: offset, End: offset + len(body), Msg: "invalid JSON: " + err.Error()}
	}
	q, err := dslQuery(top)
	if err != nil {
		start, end := dslSpan(body, err)
		return Query{}, &Error{Pos: offset + start, End: offset + end, Msg: err.Error()}
	}
	return q, nil
}

func decodeObject(body string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var top map[string]any
	if err := dec.Decode(&top); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected text after the JSON body")
	}
	return top, nil
}

func dslQuery(top map[string]any) (Query, error) {
	if err := onlyKeys(top, "the query body", "query", "aggs", "aggregations", "size"); err != nil {
		return Query{}, err
	}
	q := Query{Form: FormDSL}
	if raw, ok := top["query"]; ok {
		obj, isObject := raw.(map[string]any)
		if !isObject {
			return Query{}, failOn("query", "query must be an object")
		}
		if len(obj) > 0 {
			q.Clause = obj
		}
	}
	if err := q.dryRun(); err != nil {
		return Query{}, err
	}
	if raw, ok := top["size"]; ok {
		n, isWhole := wholeNumber(raw)
		if !isWhole || n < 0 || n > maxSize {
			return Query{}, failOn("size", "size must be a whole number from 0 to %d", maxSize)
		}
		q.Size = &n
	}
	tree, err := aggTree(top)
	if err != nil || tree == nil {
		return q, err
	}
	q.Aggs, err = parseAggs(tree)
	return q, err
}

func aggTree(top map[string]any) (map[string]any, error) {
	aggs, hasAggs := top["aggs"]
	long, hasLong := top["aggregations"]
	if hasAggs && hasLong {
		return nil, failOn("aggregations", "use aggs or aggregations, not both")
	}
	if hasLong {
		aggs = long
	}
	if !hasAggs && !hasLong {
		return nil, nil
	}
	tree, ok := aggs.(map[string]any)
	if !ok {
		return nil, failOn("aggs", "aggs must be an object")
	}
	return tree, nil
}

func dslSpan(body string, err error) (int, int) {
	var ce *clauseError
	if errors.As(err, &ce) && ce.field != "" {
		key, _ := json.Marshal(ce.field)
		if at := bytes.Index([]byte(body), key); at >= 0 {
			return at, at + len(key)
		}
	}
	return 0, len(body)
}
