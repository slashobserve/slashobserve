package otlphttp

import (
	"cmp"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type UnixNanos int64

func (n *UnixNanos) UnmarshalJSON(b []byte) error {
	v, err := strconv.ParseInt(strings.Trim(string(b), `"`), 10, 64)
	if err != nil {
		return err
	}
	*n = UnixNanos(v)
	return nil
}

type ResourceSpans struct {
	Resource struct {
		Attributes []KeyValue `json:"attributes"`
	} `json:"resource"`
	ScopeSpans []struct {
		Spans []Span `json:"spans"`
	} `json:"scopeSpans"`
}

type Span struct {
	TraceID      string          `json:"traceId"`
	SpanID       string          `json:"spanId"`
	ParentSpanID string          `json:"parentSpanId"`
	Name         string          `json:"name"`
	Kind         json.RawMessage `json:"kind"`
	Start        UnixNanos       `json:"startTimeUnixNano"`
	End          UnixNanos       `json:"endTimeUnixNano"`
	Attributes   []KeyValue      `json:"attributes"`
	Events       []SpanEvent     `json:"events"`
	Links        []SpanLink      `json:"links"`
	Status       struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	} `json:"status"`
}

type SpanEvent struct {
	Time       UnixNanos  `json:"timeUnixNano"`
	Name       string     `json:"name"`
	Attributes []KeyValue `json:"attributes"`
}

type SpanLink struct {
	TraceID    string     `json:"traceId"`
	SpanID     string     `json:"spanId"`
	Attributes []KeyValue `json:"attributes"`
}

type KeyValue struct {
	Key   string   `json:"key"`
	Value AnyValue `json:"value"`
}

type AnyValue struct {
	StringValue *string         `json:"stringValue"`
	BoolValue   *bool           `json:"boolValue"`
	IntValue    json.RawMessage `json:"intValue"`
	DoubleValue json.RawMessage `json:"doubleValue"`
	BytesValue  *string         `json:"bytesValue"`
	ArrayValue  *struct {
		Values []AnyValue `json:"values"`
	} `json:"arrayValue"`
	KvlistValue *struct {
		Values []KeyValue `json:"values"`
	} `json:"kvlistValue"`
}

type Row struct {
	TraceID, SpanID, ParentID, Name, Service string
	Start                                    time.Time
	Duration                                 int64
	Status, StatusMessage                    string
	Attributes, Resource                     string
	Kind, Events, Links                      string
	Attrs, Res                               map[string]any
	Spans, Errors                            *int64
}

func Rows(batches []ResourceSpans) ([]Row, error) {
	var rows []Row
	for _, batch := range batches {
		resource := AttributeMap(batch.Resource.Attributes)
		service, _ := resource["service.name"].(string)
		resourceJSON, err := json.Marshal(resource)
		if err != nil {
			return nil, datasource.Unreadable()
		}
		for _, scope := range batch.ScopeSpans {
			for _, s := range scope.Spans {
				row, err := toRow(s, service, resource, string(resourceJSON))
				if err != nil {
					return nil, err
				}
				rows = append(rows, row)
			}
		}
	}
	slices.SortStableFunc(rows, func(a, b Row) int {
		return cmp.Or(a.Start.Compare(b.Start), strings.Compare(a.SpanID, b.SpanID))
	})
	return rows, nil
}

func toRow(s Span, service string, resource map[string]any, resourceJSON string) (Row, error) {
	traceID, err1 := HexID(s.TraceID)
	spanID, err2 := HexID(s.SpanID)
	parentID, err3 := HexID(s.ParentSpanID)
	attrs := AttributeMap(s.Attributes)
	attributes, err4 := json.Marshal(attrs)
	events, err5 := eventsJSON(s.Events)
	links, err6 := linksJSON(s.Links)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil || spanID == "" {
		return Row{}, datasource.Unreadable()
	}
	return Row{
		TraceID:       traceID,
		SpanID:        spanID,
		ParentID:      parentID,
		Name:          s.Name,
		Service:       service,
		Start:         time.Unix(0, int64(s.Start)).UTC(),
		Duration:      max(0, int64(s.End-s.Start)),
		Status:        datasource.SpanStatusName(strings.Trim(string(s.Status.Code), `"`)),
		StatusMessage: s.Status.Message,
		Attributes:    string(attributes),
		Resource:      resourceJSON,
		Kind:          datasource.SpanKindName(strings.Trim(string(s.Kind), `"`)),
		Events:        events,
		Links:         links,
		Attrs:         attrs,
		Res:           resource,
	}, nil
}

func eventsJSON(events []SpanEvent) (string, error) {
	out := make([]datasource.SpanEvent, len(events))
	for i, e := range events {
		out[i] = datasource.SpanEvent{TimeMS: time.Unix(0, int64(e.Time)).UnixMilli(), Name: e.Name, Attributes: AttributeMap(e.Attributes)}
	}
	return datasource.SpanEventsJSON(out)
}

func linksJSON(links []SpanLink) (string, error) {
	out := make([]datasource.SpanLink, len(links))
	for i, l := range links {
		traceID, err := HexID(l.TraceID)
		if err != nil {
			return "", err
		}
		spanID, err := HexID(l.SpanID)
		if err != nil {
			return "", err
		}
		out[i] = datasource.SpanLink{TraceID: traceID, SpanID: spanID, Attributes: AttributeMap(l.Attributes)}
	}
	return datasource.SpanLinksJSON(out)
}

func HexID(id string) (string, error) {
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

func TraceIDOf(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) >= 2 && (text[0] == '"' || text[0] == '`') && text[len(text)-1] == text[0] {
		text = text[1 : len(text)-1]
	}
	if !datasource.ValidTraceID(text) {
		return "", false
	}
	return strings.ToLower(text), true
}

func AttributeMap(kvs []KeyValue) map[string]any {
	out := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		out[kv.Key] = kv.Value.Plain()
	}
	return out
}

func (v AnyValue) Plain() any {
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
			values[i] = item.Plain()
		}
		return values
	case v.KvlistValue != nil:
		return AttributeMap(v.KvlistValue.Values)
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

func Frame(rows []Row) datasource.Frame {
	columns := []struct {
		name  string
		typ   datasource.FieldType
		value func(Row) any
	}{
		{"TraceId", datasource.FieldString, func(r Row) any { return r.TraceID }},
		{"SpanId", datasource.FieldString, func(r Row) any { return r.SpanID }},
		{"ParentSpanId", datasource.FieldString, func(r Row) any { return r.ParentID }},
		{"SpanName", datasource.FieldString, func(r Row) any { return r.Name }},
		{"ServiceName", datasource.FieldString, func(r Row) any { return r.Service }},
		{"StartTime", datasource.FieldTime, func(r Row) any { return r.Start }},
		{"Duration", datasource.FieldNumber, func(r Row) any { return r.Duration }},
		{"StatusCode", datasource.FieldString, func(r Row) any { return r.Status }},
		{"StatusMessage", datasource.FieldString, func(r Row) any { return r.StatusMessage }},
		{"SpanAttributes", datasource.FieldString, func(r Row) any { return r.Attributes }},
		{"ResourceAttributes", datasource.FieldString, func(r Row) any { return r.Resource }},
		{"SpanKind", datasource.FieldString, func(r Row) any { return r.Kind }},
		{"SpanEvents", datasource.FieldString, func(r Row) any { return r.Events }},
		{"SpanLinks", datasource.FieldString, func(r Row) any { return r.Links }},
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
