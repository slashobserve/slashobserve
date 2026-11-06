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
	TraceID      string          `json:"traceId"`
	SpanID       string          `json:"spanId"`
	ParentSpanID string          `json:"parentSpanId"`
	Name         string          `json:"name"`
	Kind         json.RawMessage `json:"kind"`
	Start        unixNanos       `json:"startTimeUnixNano"`
	End          unixNanos       `json:"endTimeUnixNano"`
	Attributes   []keyValue      `json:"attributes"`
	Events       []spanEvent     `json:"events"`
	Links        []spanLink      `json:"links"`
	Status       struct {
		Code json.RawMessage `json:"code"`
	} `json:"status"`
}

type spanEvent struct {
	Time       unixNanos  `json:"timeUnixNano"`
	Name       string     `json:"name"`
	Attributes []keyValue `json:"attributes"`
}

type spanLink struct {
	TraceID    string     `json:"traceId"`
	SpanID     string     `json:"spanId"`
	Attributes []keyValue `json:"attributes"`
}

type spanRow struct {
	traceID, spanID, parentID, name, service string
	start                                    time.Time
	duration                                 int64
	status, attributes, resource             string
	kind, events, links                      string
	spans, errors                            *int64
}

func (d *Datasource) Trace(ctx context.Context, id string, r datasource.TimeRange, maxSpans int) (datasource.Frame, error) {
	ctx, cancel := bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	return d.trace(ctx, id, r, maxSpans)
}

func (d *Datasource) trace(ctx context.Context, id string, r datasource.TimeRange, maxSpans int) (datasource.Frame, error) {
	params := url.Values{
		"start": {strconv.FormatInt(r.From.Unix(), 10)},
		"end":   {strconv.FormatInt(ceilSeconds(r.To), 10)},
	}
	status, raw, err := d.fetch(ctx, "/api/traces/"+id, params)
	if err != nil {
		return datasource.Frame{}, err
	}
	if status == http.StatusNotFound {
		return datasource.Frame{}, traceNotFound()
	}
	if err := statusError(status, raw); err != nil {
		return datasource.Frame{}, err
	}
	var resp traceResponse
	if json.Unmarshal(raw, &resp) != nil {
		return datasource.Frame{}, unreadable()
	}
	rows, err := spanRows(resp)
	if err != nil {
		return datasource.Frame{}, err
	}
	if len(rows) == 0 {
		return datasource.Frame{}, traceNotFound()
	}
	f := spanFrame(rows[:min(len(rows), maxSpans)])
	f.Meta = &datasource.FrameMeta{Bytes: int64(len(raw))}
	if len(rows) > maxSpans {
		f.Meta.Truncated, f.Meta.RowsTotal = true, len(rows)
	}
	return f, nil
}

func traceNotFound() error {
	return datasource.Errorf(datasource.ErrNotFound, "trace not found")
}

func spanRows(resp traceResponse) ([]spanRow, error) {
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
	events, err5 := eventsJSON(s.Events)
	links, err6 := linksJSON(s.Links)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil || spanID == "" {
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
		kind:       kindName(s.Kind),
		events:     events,
		links:      links,
	}, nil
}

func eventsJSON(events []spanEvent) (string, error) {
	out := make([]datasource.SpanEvent, len(events))
	for i, e := range events {
		out[i] = datasource.SpanEvent{TimeMS: time.Unix(0, int64(e.Time)).UnixMilli(), Name: e.Name, Attributes: attributeMap(e.Attributes)}
	}
	return datasource.SpanEventsJSON(out)
}

func linksJSON(links []spanLink) (string, error) {
	out := make([]datasource.SpanLink, len(links))
	for i, l := range links {
		traceID, err := hexID(l.TraceID)
		if err != nil {
			return "", err
		}
		spanID, err := hexID(l.SpanID)
		if err != nil {
			return "", err
		}
		out[i] = datasource.SpanLink{TraceID: traceID, SpanID: spanID, Attributes: attributeMap(l.Attributes)}
	}
	return datasource.SpanLinksJSON(out)
}

func kindName(raw json.RawMessage) string {
	return datasource.SpanKindName(strings.Trim(string(raw), `"`))
}

func hexID(id string) (string, error) {
	if id == "" {
		return "", nil
	}
	if datasource.ValidTraceID(id) {
		return strings.ToLower(id), nil
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
		{"SpanKind", datasource.FieldString, func(r spanRow) any { return r.kind }},
		{"SpanEvents", datasource.FieldString, func(r spanRow) any { return r.events }},
		{"SpanLinks", datasource.FieldString, func(r spanRow) any { return r.links }},
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
