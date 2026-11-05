package tempo

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const (
	traceIDHexLen    = 32
	shortTraceHexLen = 16
	emptyQuery       = `a Tempo query needs a trace id or a TraceQL query such as {resource.service.name="checkout"}`
)

type traceResponse struct {
	Batches []resourceSpans `json:"batches"`
}

type resourceSpans struct {
	Resource struct {
		Attributes []keyValue `json:"attributes"`
	} `json:"resource"`
	ScopeSpans []struct {
		Spans []span `json:"spans"`
	} `json:"scopeSpans"`
}

type span struct {
	TraceID      string     `json:"traceId"`
	SpanID       string     `json:"spanId"`
	ParentSpanID string     `json:"parentSpanId"`
	Name         string     `json:"name"`
	Start        unixNanos  `json:"startTimeUnixNano"`
	End          unixNanos  `json:"endTimeUnixNano"`
	Attributes   []keyValue `json:"attributes"`
	Status       struct {
		Code json.RawMessage `json:"code"`
	} `json:"status"`
}

type keyValue struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

type anyValue struct {
	StringValue *string         `json:"stringValue"`
	BoolValue   *bool           `json:"boolValue"`
	IntValue    json.RawMessage `json:"intValue"`
	DoubleValue json.RawMessage `json:"doubleValue"`
	BytesValue  *string         `json:"bytesValue"`
	ArrayValue  *struct {
		Values []anyValue `json:"values"`
	} `json:"arrayValue"`
	KvlistValue *struct {
		Values []keyValue `json:"values"`
	} `json:"kvlistValue"`
}

type unixNanos int64

func (n *unixNanos) UnmarshalJSON(b []byte) error {
	v, err := strconv.ParseInt(strings.Trim(string(b), `"`), 10, 64)
	if err != nil {
		return err
	}
	*n = unixNanos(v)
	return nil
}

type searchResponse struct {
	Traces []struct {
		TraceID         string    `json:"traceID"`
		RootServiceName string    `json:"rootServiceName"`
		RootTraceName   string    `json:"rootTraceName"`
		Start           unixNanos `json:"startTimeUnixNano"`
		DurationMs      int64     `json:"durationMs"`
	} `json:"traces"`
}

type spanRow struct {
	traceID, spanID, parentID, name, service string
	start                                    time.Time
	duration                                 int64
	status, attributes, resource             string
}

func (d *Datasource) QueryRange(ctx context.Context, q datasource.Query, r datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
	return d.QueryInstant(ctx, q, r)
}

func (d *Datasource) QueryInstant(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(q.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	exp, err := macros.Expand(macros.TraceQL, q.Text, macros.Env{
		Range:     r,
		Interval:  max(time.Second, (r.To.Sub(r.From) / 1000).Truncate(time.Second), q.MinInterval),
		Variables: q.Variables,
		Dashboard: q.Dashboard,
		User:      q.User,
	})
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(exp.Text)
	if text == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", emptyQuery)
	}
	if id, ok := traceIDOf(text); ok {
		return d.trace(ctx, id, lim)
	}
	return d.search(ctx, text, r, lim)
}

func traceIDOf(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) >= 2 && (text[0] == '"' || text[0] == '`') && text[len(text)-1] == text[0] {
		text = text[1 : len(text)-1]
	}
	if len(text) != traceIDHexLen && len(text) != shortTraceHexLen {
		return "", false
	}
	if _, err := hex.DecodeString(text); err != nil {
		return "", false
	}
	return strings.ToLower(text), true
}

func (d *Datasource) trace(ctx context.Context, id string, lim datasource.Limits) ([]datasource.Frame, error) {
	status, raw, err := d.fetch(ctx, "/api/traces/"+id, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return []datasource.Frame{}, nil
	}
	if err := statusError(status, raw); err != nil {
		return nil, err
	}
	var resp traceResponse
	if json.Unmarshal(raw, &resp) != nil {
		return nil, unreadable()
	}
	rows, err := spanRows(resp, lim.MaxRows)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []datasource.Frame{}, nil
	}
	return []datasource.Frame{spanFrame(rows)}, nil
}

func spanRows(resp traceResponse, maxRows int) ([]spanRow, error) {
	var rows []spanRow
	for _, batch := range resp.Batches {
		resource := attributeMap(batch.Resource.Attributes)
		service, _ := resource["service.name"].(string)
		resourceJSON, err := json.Marshal(resource)
		if err != nil {
			return nil, unreadable()
		}
		for _, scope := range batch.ScopeSpans {
			for _, s := range scope.Spans {
				if len(rows) == maxRows {
					return nil, datasource.Errorf(datasource.ErrLimitExceeded, "row limit exceeded: more than %d spans", maxRows)
				}
				row, err := toRow(s, service, string(resourceJSON))
				if err != nil {
					return nil, err
				}
				rows = append(rows, row)
			}
		}
	}
	slices.SortStableFunc(rows, func(a, b spanRow) int {
		return cmp.Or(a.start.Compare(b.start), strings.Compare(a.spanID, b.spanID))
	})
	return rows, nil
}

func toRow(s span, service, resource string) (spanRow, error) {
	traceID, err1 := hexID(s.TraceID)
	spanID, err2 := hexID(s.SpanID)
	parentID, err3 := hexID(s.ParentSpanID)
	attributes, err4 := json.Marshal(attributeMap(s.Attributes))
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || spanID == "" {
		return spanRow{}, unreadable()
	}
	return spanRow{
		traceID:    traceID,
		spanID:     spanID,
		parentID:   parentID,
		name:       s.Name,
		service:    service,
		start:      time.Unix(0, int64(s.Start)).UTC(),
		duration:   max(0, int64(s.End-s.Start)),
		status:     statusName(s.Status.Code),
		attributes: string(attributes),
		resource:   resource,
	}, nil
}

func hexID(id string) (string, error) {
	if id == "" {
		return "", nil
	}
	if len(id) == traceIDHexLen || len(id) == shortTraceHexLen {
		if _, err := hex.DecodeString(id); err == nil {
			return strings.ToLower(id), nil
		}
	}
	b, err := base64.StdEncoding.DecodeString(id)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func statusName(code json.RawMessage) string {
	switch strings.Trim(string(code), `"`) {
	case "1", "STATUS_CODE_OK":
		return "Ok"
	case "2", "STATUS_CODE_ERROR":
		return "Error"
	}
	return "Unset"
}

func attributeMap(kvs []keyValue) map[string]any {
	out := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		out[kv.Key] = kv.Value.plain()
	}
	return out
}

func (v anyValue) plain() any {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.BoolValue != nil:
		return *v.BoolValue
	case v.IntValue != nil:
		return number(v.IntValue)
	case v.DoubleValue != nil:
		return number(v.DoubleValue)
	case v.BytesValue != nil:
		return *v.BytesValue
	case v.ArrayValue != nil:
		values := make([]any, len(v.ArrayValue.Values))
		for i, item := range v.ArrayValue.Values {
			values[i] = item.plain()
		}
		return values
	case v.KvlistValue != nil:
		return attributeMap(v.KvlistValue.Values)
	}
	return nil
}

func number(raw json.RawMessage) any {
	text := strings.Trim(string(raw), `"`)
	var n json.Number
	if json.Unmarshal([]byte(text), &n) == nil {
		return n
	}
	return text
}

func spanFrame(rows []spanRow) datasource.Frame {
	columns := []struct {
		name  string
		typ   datasource.FieldType
		value func(spanRow) any
	}{
		{"TraceId", datasource.FieldString, func(r spanRow) any { return r.traceID }},
		{"SpanId", datasource.FieldString, func(r spanRow) any { return r.spanID }},
		{"ParentSpanId", datasource.FieldString, func(r spanRow) any { return r.parentID }},
		{"SpanName", datasource.FieldString, func(r spanRow) any { return r.name }},
		{"ServiceName", datasource.FieldString, func(r spanRow) any { return r.service }},
		{"StartTime", datasource.FieldTime, func(r spanRow) any { return r.start }},
		{"Duration", datasource.FieldNumber, func(r spanRow) any { return r.duration }},
		{"StatusCode", datasource.FieldString, func(r spanRow) any { return r.status }},
		{"SpanAttributes", datasource.FieldString, func(r spanRow) any { return r.attributes }},
		{"ResourceAttributes", datasource.FieldString, func(r spanRow) any { return r.resource }},
	}
	f := datasource.Frame{Fields: make([]datasource.Field, len(columns))}
	for i, c := range columns {
		values := make([]any, len(rows))
		for j, r := range rows {
			values[j] = c.value(r)
		}
		f.Fields[i] = datasource.Field{Name: c.name, Type: c.typ, Values: values}
	}
	return f
}

func (d *Datasource) search(ctx context.Context, text string, r datasource.TimeRange, lim datasource.Limits) ([]datasource.Frame, error) {
	params := url.Values{
		"q":     {text},
		"start": {strconv.FormatInt(r.From.Unix(), 10)},
		"end":   {strconv.FormatInt(ceilSeconds(r.To), 10)},
		"limit": {strconv.Itoa(lim.MaxRows)},
	}
	var resp searchResponse
	if err := d.getJSON(ctx, "/api/search", params, &resp); err != nil {
		return nil, err
	}
	if len(resp.Traces) > lim.MaxRows {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "row limit exceeded: more than %d traces", lim.MaxRows)
	}
	rows := make([]spanRow, 0, len(resp.Traces))
	for _, t := range resp.Traces {
		id := strings.ToLower(t.TraceID)
		if len(id) < traceIDHexLen {
			id = strings.Repeat("0", traceIDHexLen-len(id)) + id
		}
		rows = append(rows, spanRow{
			traceID:  id,
			spanID:   id,
			name:     t.RootTraceName,
			service:  t.RootServiceName,
			start:    time.Unix(0, int64(t.Start)).UTC(),
			duration: t.DurationMs * int64(time.Millisecond),
		})
	}
	if len(rows) == 0 {
		return []datasource.Frame{}, nil
	}
	slices.SortStableFunc(rows, func(a, b spanRow) int {
		return cmp.Or(b.start.Compare(a.start), strings.Compare(a.traceID, b.traceID))
	})
	return []datasource.Frame{traceFrame(rows)}, nil
}

func ceilSeconds(t time.Time) int64 {
	if t.Equal(t.Truncate(time.Second)) {
		return t.Unix()
	}
	return t.Unix() + 1
}

func traceFrame(rows []spanRow) datasource.Frame {
	full := spanFrame(rows)
	keep := []string{"TraceId", "SpanId", "SpanName", "ServiceName", "StartTime", "Duration"}
	f := datasource.Frame{}
	for _, field := range full.Fields {
		if slices.Contains(keep, field.Name) {
			f.Fields = append(f.Fields, field)
		}
	}
	return f
}
