package auditsource

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
)

const defaultHits = 10

var searchKeys = []string{"query", "size", "from", "sort", "track_total_hits", "timeout", "aggs", "aggregations"}

type searchRequest struct {
	window    filter.Window
	clause    map[string]any
	size      int
	from      int
	ascending bool
	trackUpTo int
	aggs      map[string]any
}

func decodeBody(body []byte) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, &refusal{status: http.StatusBadRequest, kind: "parsing_exception", reason: "the request body is not a JSON object"}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, &refusal{status: http.StatusBadRequest, kind: "parsing_exception", reason: "unexpected data after the request body"}
	}
	return obj, nil
}

func parseSearch(body []byte) (searchRequest, error) {
	obj, err := decodeBody(body)
	if err != nil {
		return searchRequest{}, err
	}
	for _, key := range slices.Sorted(maps.Keys(obj)) {
		if !slices.Contains(searchKeys, key) {
			return searchRequest{}, refuse("the audit datasource does not support %q in a search body; it reads %s", key, strings.Join(searchKeys, ", "))
		}
	}
	query, ok := obj["query"].(map[string]any)
	if !ok {
		return searchRequest{}, refuse("a search on the audit datasource needs a query with a range on %s", TimeField)
	}
	req := searchRequest{size: defaultHits, trackUpTo: resultWindow}
	if req.window, req.clause, err = splitWindow(query); err != nil {
		return searchRequest{}, err
	}
	if req.clause, err = rewriteLucene(req.clause); err != nil {
		return searchRequest{}, err
	}
	if req.size, req.from, err = paging(obj); err != nil {
		return searchRequest{}, err
	}
	if sort, ok := obj["sort"]; ok {
		if req.ascending, err = parseSort(sort); err != nil {
			return searchRequest{}, err
		}
	}
	if v, ok := obj["track_total_hits"]; ok {
		if req.trackUpTo, err = trackedTotal(v); err != nil {
			return searchRequest{}, err
		}
	}
	if req.aggs, err = aggTree(obj); err != nil {
		return searchRequest{}, err
	}
	return req, nil
}

func paging(obj map[string]any) (int, int, error) {
	size, err := count(obj, "size", defaultHits)
	if err != nil {
		return 0, 0, err
	}
	from, err := count(obj, "from", 0)
	if err != nil {
		return 0, 0, err
	}
	if from+size > resultWindow {
		return 0, 0, refuse("Result window is too large, from + size must be less than or equal to: [%d] but was [%d]", resultWindow, from+size)
	}
	return min(size, filter.MaxRows), from, nil
}

func count(obj map[string]any, key string, fallback int) (int, error) {
	v, ok := obj[key]
	if !ok {
		return fallback, nil
	}
	n, ok := wholeNumber(v)
	if !ok || n < 0 {
		return 0, refuse("%s must be a whole number of 0 or more", key)
	}
	return n, nil
}

func wholeNumber(v any) (int, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(n.String())
	return i, err == nil
}

func trackedTotal(v any) (int, error) {
	switch x := v.(type) {
	case bool:
		if x {
			return resultWindow, nil
		}
		return 0, nil
	case json.Number:
		if n, ok := wholeNumber(x); ok && n >= 0 {
			return min(n, resultWindow), nil
		}
	}
	return 0, refuse("track_total_hits must be true, false or a whole number")
}

func aggTree(obj map[string]any) (map[string]any, error) {
	short, hasShort := obj["aggs"]
	long, hasLong := obj["aggregations"]
	switch {
	case hasShort && hasLong:
		return nil, refuse("use aggs or aggregations, not both")
	case hasLong:
		short = long
	case !hasShort:
		return nil, nil
	}
	tree, ok := short.(map[string]any)
	if !ok {
		return nil, refuse("aggs must be an object")
	}
	return tree, nil
}

func parseSort(v any) (bool, error) {
	items, ok := v.([]any)
	if !ok {
		items = []any{v}
	}
	ascending, sorted := false, false
	for _, item := range items {
		field, order, err := sortItem(item)
		if err != nil {
			return false, err
		}
		switch field {
		case "_doc", "_score", "id":
		case TimeField:
			if !sorted {
				ascending, sorted = order == "asc", true
			}
		default:
			return false, refuse("the audit datasource sorts only by %s, not %q", TimeField, field)
		}
	}
	return ascending, nil
}

func sortItem(item any) (string, string, error) {
	switch x := item.(type) {
	case string:
		return x, "asc", nil
	case map[string]any:
		if len(x) != 1 {
			break
		}
		for field, spec := range x {
			switch s := spec.(type) {
			case string:
				return field, strings.ToLower(s), nil
			case map[string]any:
				order, _ := s["order"].(string)
				if order == "" {
					order = "asc"
				}
				return field, strings.ToLower(order), nil
			}
		}
	}
	return "", "", refuse("a sort entry is a field name or an object with one field")
}

func splitWindow(query map[string]any) (filter.Window, map[string]any, error) {
	w, found, err := windowOf(query)
	if err != nil || found {
		return w, nil, err
	}
	b, isBool := query["bool"].(map[string]any)
	list, hasFilter := b["filter"].([]any)
	if isBool && len(query) == 1 && hasFilter {
		for i, item := range list {
			clause, _ := item.(map[string]any)
			w, found, err := windowOf(clause)
			if err != nil {
				return filter.Window{}, nil, err
			}
			if !found {
				continue
			}
			rest := slices.Delete(slices.Clone(list), i, i+1)
			if len(rest) == 0 && len(b) == 1 {
				return w, nil, nil
			}
			kept := maps.Clone(b)
			kept["filter"] = rest
			return w, map[string]any{"bool": kept}, nil
		}
	}
	return filter.Window{}, nil, refuse("a search on the audit datasource needs a range on %s with a format, at the top of the query or in its bool filter", TimeField)
}

func windowOf(clause map[string]any) (filter.Window, bool, error) {
	r, ok := clause["range"].(map[string]any)
	if !ok || len(clause) != 1 || len(r) != 1 {
		return filter.Window{}, false, nil
	}
	bounds, ok := r[TimeField].(map[string]any)
	if !ok {
		return filter.Window{}, false, nil
	}
	format, ok := bounds["format"].(string)
	if !ok {
		return filter.Window{}, false, nil
	}
	read, unit, err := boundReader(format)
	if err != nil {
		return filter.Window{}, false, err
	}
	var w filter.Window
	for _, key := range slices.Sorted(maps.Keys(bounds)) {
		if key == "format" {
			continue
		}
		if err := setBound(&w, key, bounds[key], read, unit); err != nil {
			return filter.Window{}, false, err
		}
	}
	if w.From.IsZero() || w.To.IsZero() {
		return filter.Window{}, false, refuse("the range on %s needs a lower and an upper bound", TimeField)
	}
	return w, true, nil
}

func setBound(w *filter.Window, key string, v any, read func(any) (time.Time, error), unit time.Duration) error {
	at, err := read(v)
	if err != nil {
		return err
	}
	switch key {
	case "gte":
		w.From = at
	case "gt":
		w.From = at.Add(unit)
	case "lt":
		w.To = at
	case "lte":
		w.To = at.Add(unit)
	default:
		return refuse("unsupported key %q in the range on %s; supported: gte, gt, lt, lte and format", key, TimeField)
	}
	return nil
}

func boundReader(format string) (func(any) (time.Time, error), time.Duration, error) {
	switch format {
	case "epoch_millis":
		return func(v any) (time.Time, error) {
			text, _ := v.(string)
			if n, ok := v.(json.Number); ok {
				text = n.String()
			}
			ms, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return time.Time{}, refuse("an epoch_millis bound must be a whole number of milliseconds")
			}
			return time.UnixMilli(ms).UTC(), nil
		}, time.Millisecond, nil
	case "strict_date_optional_time_nanos", "strict_date_optional_time", "date_optional_time":
		return func(v any) (time.Time, error) {
			text, _ := v.(string)
			at, err := time.Parse(time.RFC3339Nano, text)
			if err != nil {
				return time.Time{}, refuse("a %s bound must be an RFC 3339 time, not %q", format, text)
			}
			return at.UTC(), nil
		}, time.Nanosecond, nil
	}
	return nil, 0, refuse("the audit datasource reads %s ranges in epoch_millis or strict_date_optional_time_nanos, not %q", TimeField, format)
}

func rewriteLucene(clause map[string]any) (map[string]any, error) {
	if clause == nil {
		return nil, nil
	}
	out, err := rewriteNode(clause)
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

func rewriteNode(node any) (any, error) {
	switch x := node.(type) {
	case map[string]any:
		if text, ok := luceneText(x); ok {
			q, err := filter.Parse("lucene:" + text)
			if err != nil {
				return nil, err
			}
			if q.Clause == nil {
				return map[string]any{"match_all": map[string]any{}}, nil
			}
			return q.Clause, nil
		}
		out := make(map[string]any, len(x))
		for key, child := range x {
			rewritten, err := rewriteNode(child)
			if err != nil {
				return nil, err
			}
			out[key] = rewritten
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, child := range x {
			rewritten, err := rewriteNode(child)
			if err != nil {
				return nil, err
			}
			out[i] = rewritten
		}
		return out, nil
	}
	return node, nil
}

func luceneText(clause map[string]any) (string, bool) {
	qs, ok := clause["query_string"].(map[string]any)
	if !ok || len(clause) != 1 {
		return "", false
	}
	_, lenient := qs["lenient"]
	_, fields := qs["fields"]
	text, isText := qs["query"].(string)
	return text, isText && !lenient && !fields
}

func (e *emulator) search(ctx context.Context, body []byte) (map[string]any, error) {
	req, err := parseSearch(body)
	if err != nil {
		return nil, err
	}
	q := filter.Query{Clause: req.clause}
	hits := map[string]any{"max_score": nil, "hits": []any{}}
	if req.size > 0 {
		events, err := e.store.Rows(ctx, q, req.window, filter.Page{Limit: req.size, Offset: req.from, Ascending: req.ascending})
		if err != nil {
			return nil, err
		}
		docs := make([]any, len(events))
		for i, ev := range events {
			docs[i] = map[string]any{"_index": Index, "_id": ev.ID, "_score": nil, "_source": documentOf(ev)}
		}
		hits["hits"] = docs
	}
	if req.trackUpTo > 0 {
		n, err := e.store.Count(ctx, q, req.window, req.trackUpTo+1)
		if err != nil {
			return nil, err
		}
		hits["total"] = totalOf(n, req.trackUpTo)
	}
	out := map[string]any{
		"took":      0,
		"timed_out": false,
		"_shards":   map[string]any{"total": 1, "successful": 1, "skipped": 0, "failed": 0},
		"hits":      hits,
	}
	if req.aggs != nil {
		aggs, err := e.aggregate(ctx, req.clause, req.window, req.aggs)
		if err != nil {
			return nil, err
		}
		out["aggregations"] = aggs
	}
	return out, nil
}

func totalOf(n int64, upTo int) map[string]any {
	if n > int64(upTo) {
		return map[string]any{"value": upTo, "relation": "gte"}
	}
	return map[string]any{"value": n, "relation": "eq"}
}

var anyWindow = filter.Window{From: time.Unix(0, 0).UTC(), To: time.Unix(1, 0).UTC()}

func validate(body []byte) (map[string]any, error) {
	obj, err := decodeBody(body)
	if err != nil {
		return nil, err
	}
	clause, _ := obj["query"].(map[string]any)
	clause, err = rewriteLucene(clause)
	if err == nil {
		_, err = filter.Query{Clause: clause}.Count(anyWindow, 1)
	}
	if err == nil {
		return map[string]any{"valid": true}, nil
	}
	message := err.Error()
	if problem, ok := trail.ProblemOf(err); ok {
		message = problem.Message
	}
	return map[string]any{"valid": false, "explanations": []any{map[string]any{"index": Index, "valid": false, "error": message}}}, nil
}
