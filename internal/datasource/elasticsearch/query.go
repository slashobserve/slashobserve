package elasticsearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/kql"
)

const (
	maxLogLines    = 5000
	maxStepBuckets = 11000
	countAgg       = "count"
	nanosFormat    = "strict_date_optional_time_nanos"
	trackedHits    = 10000
)

var dslKeys = []string{"query", "aggs", "aggregations", "size", "sort", "fields", "_source", "runtime_mappings"}

type searchKind int

const (
	searchLines searchKind = iota
	searchCount
)

type userQuery struct {
	clause map[string]any
	dsl    map[string]any
}

func (u userQuery) aggs() map[string]any {
	for _, key := range []string{"aggs", "aggregations"} {
		if tree, ok := u.dsl[key].(map[string]any); ok {
			return tree
		}
	}
	return nil
}

type plan struct {
	kind      searchKind
	size      int
	from      int
	paged     bool
	step      time.Duration
	timeout   time.Duration
	totalHits any
}

type prepared struct {
	user    userQuery
	mapping *Mapping
	adhoc   []any
	lim     datasource.Limits
	form    Form
}

func (d *Datasource) QueryRange(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if step < time.Millisecond {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "step must be at least 1ms")
	}
	return d.query(ctx, q, r, step, false)
}

func (d *Datasource) QueryInstant(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return d.query(ctx, q, r, instantInterval(r, q.MinInterval), true)
}

func (d *Datasource) query(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration, instant bool) ([]datasource.Frame, error) {
	lim := d.opts.Limits.Tighten(q.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	prep, err := d.prepare(ctx, q, r, step, lim)
	if err != nil {
		return nil, err
	}
	p := plan{kind: searchLines, size: min(lim.MaxRows, maxLogLines), timeout: lim.Timeout, totalHits: trackedHits}
	if !instant && prep.form != FormDSL {
		p.kind, p.totalHits = searchCount, false
		if p.step, err = histogramStep(r, step, lim); err != nil {
			return nil, err
		}
	}
	body := buildBody(prep.user, d.opts.TimeField, r, prep.adhoc, p)
	raw, err := d.search(ctx, body)
	if err != nil {
		return nil, err
	}
	return d.resultFrames(raw, body, prep, p)
}

func (d *Datasource) prepare(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration, lim datasource.Limits) (prepared, error) {
	text, err := d.expand(q, r, step)
	if err != nil {
		return prepared{}, err
	}
	form, body := Classify(text, d.opts.QueryLanguage)
	m, err := d.Mapping(ctx)
	if err != nil {
		return prepared{}, err
	}
	user, err := compileUser(form, body, m)
	if err != nil {
		return prepared{}, err
	}
	adhoc, err := adhocClauses(m, q.Filters)
	if err != nil {
		return prepared{}, err
	}
	return prepared{user: user, mapping: m, adhoc: adhoc, lim: lim, form: form}, nil
}

func histogramStep(r datasource.TimeRange, step time.Duration, lim datasource.Limits) (time.Duration, error) {
	step = max((step + time.Second - 1).Truncate(time.Second), time.Second)
	buckets := int64(r.To.Sub(r.From)/step) + 1
	switch {
	case buckets > maxStepBuckets:
		return 0, datasource.Errorf(datasource.ErrLimitExceeded, "step too small: %d buckets, max %d", buckets, maxStepBuckets)
	case buckets > int64(lim.MaxPoints):
		return 0, datasource.Errorf(datasource.ErrLimitExceeded, "step too small: %d points per series, max points is %d", buckets, lim.MaxPoints)
	}
	return step, nil
}

func fixedInterval(step time.Duration) string {
	secs := int64(step / time.Second)
	switch {
	case secs%86400 == 0:
		return strconv.FormatInt(secs/86400, 10) + "d"
	case secs%3600 == 0:
		return strconv.FormatInt(secs/3600, 10) + "h"
	case secs%60 == 0:
		return strconv.FormatInt(secs/60, 10) + "m"
	}
	return strconv.FormatInt(secs, 10) + "s"
}

func compileUser(form Form, body string, m *Mapping) (userQuery, error) {
	switch form {
	case FormDSL:
		obj, problems := parseDSL(body)
		if len(problems) > 0 {
			return userQuery{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", problems[0].message)
		}
		clause, _ := obj["query"].(map[string]any)
		if clause == nil {
			clause = matchAll()
		}
		delete(obj, "query")
		return userQuery{clause: clause, dsl: obj}, nil
	case FormLucene:
		if body == "" {
			return userQuery{clause: matchAll()}, nil
		}
		return userQuery{clause: luceneQuery(body)}, nil
	}
	compiled, err := kql.Compile(body, kqlFields(m))
	if err != nil {
		offset := 0
		if len(compiled.Diagnostics) > 0 {
			offset = compiled.Diagnostics[0].Start
		}
		return userQuery{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s at offset %d", truncateText(err.Error()), offset)
	}
	if compiled.Query == nil {
		return userQuery{clause: matchAll()}, nil
	}
	return userQuery{clause: compiled.Query}, nil
}

func matchAll() map[string]any { return map[string]any{"match_all": map[string]any{}} }

func luceneQuery(text string) map[string]any {
	return map[string]any{"query_string": map[string]any{"query": text, "analyze_wildcard": true}}
}

var kqlKinds = map[FieldKind]kql.FieldKind{
	KindKeyword:   kql.Keyword,
	KindText:      kql.Text,
	KindNumber:    kql.Number,
	KindDate:      kql.Date,
	KindDateNanos: kql.Date,
	KindBoolean:   kql.Boolean,
	KindIP:        kql.IP,
	KindNested:    kql.Nested,
	KindObject:    kql.Object,
	KindFlattened: kql.Object,
}

func kqlFields(m *Mapping) kql.Fields {
	fields := make(kql.Fields, len(m.Fields))
	for name, f := range m.Fields {
		kind, ok := kqlKinds[f.Kind]
		if !ok {
			kind = kql.Other
		}
		fields[name] = kind
	}
	return fields
}

type dslProblem struct {
	message    string
	start, end int
}

func parseDSL(body string) (map[string]any, []dslProblem) {
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, []dslProblem{syntaxProblem(err, body)}
	}
	if _, err := dec.Token(); err != io.EOF {
		at := int(dec.InputOffset())
		return nil, []dslProblem{{message: "unexpected data after the query body", start: min(at, len(body)), end: len(body)}}
	}
	var problems []dslProblem
	for _, key := range topLevelKeys(body) {
		if !slices.Contains(dslKeys, key.name) {
			problems = append(problems, dslProblem{message: fmt.Sprintf("unsupported key %q in the query body", truncateText(key.name)), start: key.start, end: key.end})
		}
	}
	return obj, problems
}

func syntaxProblem(err error, body string) dslProblem {
	at := 0
	var syntax *json.SyntaxError
	switch {
	case errors.As(err, &syntax):
		at = int(syntax.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF):
		at = len(body)
	}
	at = min(max(at, 0), len(body))
	return dslProblem{message: fmt.Sprintf("query body is not valid JSON: %s", truncateText(err.Error())), start: max(at-1, 0), end: at}
}

type dslKey struct {
	name       string
	start, end int
}

func topLevelKeys(body string) []dslKey {
	dec := json.NewDecoder(strings.NewReader(body))
	var keys []dslKey
	depth, isKey := 0, true
	for {
		tok, err := dec.Token()
		if err != nil {
			return keys
		}
		if delim, ok := tok.(json.Delim); ok {
			if delim == '{' || delim == '[' {
				depth++
				continue
			}
			if depth--; depth == 1 {
				isKey = true
			}
			continue
		}
		if depth != 1 {
			continue
		}
		if name, ok := tok.(string); ok && isKey {
			end := int(dec.InputOffset())
			encoded, _ := json.Marshal(name)
			keys = append(keys, dslKey{name: name, start: max(end-len(encoded), 0), end: end})
		}
		isKey = !isKey
	}
}

func timeRangeClause(timeField string, r datasource.TimeRange) map[string]any {
	return map[string]any{"range": map[string]any{timeField: map[string]any{
		"gte":    r.From.UTC().Format(time.RFC3339Nano),
		"lt":     r.To.UTC().Format(time.RFC3339Nano),
		"format": nanosFormat,
	}}}
}

func defaultSort(timeField string) []any {
	return []any{
		map[string]any{timeField: map[string]any{"order": "desc", "numeric_type": "date_nanos", "unmapped_type": "date"}},
		map[string]any{"_doc": "desc"},
	}
}

func buildBody(u userQuery, timeField string, r datasource.TimeRange, adhoc []any, p plan) map[string]any {
	filter := append([]any{timeRangeClause(timeField, r), u.clause}, adhoc...)
	body := map[string]any{
		"timeout": strconv.FormatInt(p.timeout.Milliseconds(), 10) + "ms",
		"query":   map[string]any{"bool": map[string]any{"filter": filter}},
	}
	for _, key := range []string{"fields", "_source", "runtime_mappings"} {
		if v, ok := u.dsl[key]; ok {
			body[key] = v
		}
	}
	switch tree := u.aggs(); {
	case p.kind == searchCount && u.dsl == nil:
		body["size"] = 0
		body["track_total_hits"] = false
		body["aggs"] = map[string]any{countAgg: map[string]any{"date_histogram": map[string]any{
			"field":          timeField,
			"fixed_interval": fixedInterval(p.step),
			"min_doc_count":  0,
			"extended_bounds": map[string]any{
				"min": r.From.UnixMilli(),
				"max": r.To.UnixMilli(),
			},
		}}}
	case tree != nil:
		body["size"] = 0
		body["track_total_hits"] = true
		body["aggs"] = tree
	default:
		body["size"] = linesSize(u, p)
		body["track_total_hits"] = p.totalHits
		body["sort"] = defaultSort(timeField)
		if sort, ok := u.dsl["sort"]; ok {
			body["sort"] = sort
		}
		if p.from > 0 {
			body["from"] = p.from
		}
	}
	return body
}

func linesSize(u userQuery, p plan) int {
	if number, ok := u.dsl["size"].(json.Number); ok && !p.paged {
		if size, err := number.Int64(); err == nil && size >= 0 {
			return int(min(size, int64(p.size)))
		}
	}
	return p.size
}

type searchResponse struct {
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Failed int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total struct {
			Value    int64  `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		Hits []hit `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]json.RawMessage `json:"aggregations"`
}

func (r searchResponse) partial() bool { return r.TimedOut || r.Shards.Failed > 0 }

func parseSearch(raw []byte) (searchResponse, error) {
	var resp searchResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return searchResponse{}, unreadable()
	}
	return resp, nil
}

func (d *Datasource) resultFrames(raw []byte, body map[string]any, prep prepared, p plan) ([]datasource.Frame, error) {
	resp, err := parseSearch(raw)
	if err != nil {
		return nil, err
	}
	if tree, ok := body["aggs"].(map[string]any); ok {
		frames, err := foldAggregations(tree, resp.Aggregations, int(resp.Hits.Total.Value), d.opts.TimeField, prep.lim)
		if err != nil {
			return nil, err
		}
		if resp.partial() && len(frames) > 0 {
			meta := datasource.FrameMeta{}
			if frames[0].Meta != nil {
				meta = *frames[0].Meta
			}
			meta.Partial = true
			frames[0].Meta = &meta
		}
		return frames, nil
	}
	frame, err := d.hitsResult(resp, prep.mapping)
	if err != nil {
		return nil, err
	}
	frame.Meta = &datasource.FrameMeta{
		Truncated: len(resp.Hits.Hits) == body["size"].(int),
		Partial:   resp.partial(),
	}
	if resp.Hits.Total.Relation == "eq" {
		frame.Meta.RowsTotal = int(resp.Hits.Total.Value)
	}
	return []datasource.Frame{frame}, nil
}

func (d *Datasource) hitsResult(resp searchResponse, m *Mapping) (datasource.Frame, error) {
	roles, err := m.roles(d.opts.Fields)
	if err != nil {
		return datasource.Frame{}, err
	}
	frame := hitsFrame(resp.Hits.Hits, roles)
	if m.Fields[d.opts.TimeField].Kind == KindDate {
		roundToMillis(&frame)
	}
	return frame, nil
}

func roundToMillis(f *datasource.Frame) {
	times := f.Fields[0].Values
	for i, v := range times {
		if at, ok := v.(time.Time); ok {
			at = at.Truncate(time.Millisecond)
			times[i] = at
			f.Fields[1].Values[i] = strconv.FormatInt(at.UnixNano(), 10)
		}
	}
}
