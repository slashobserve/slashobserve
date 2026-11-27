package elasticsearch

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	refChildOf     = "CHILD_OF"
	tagDot         = "@"
	tagError       = "error"
	tagSpanKind    = "span.kind"
	tagStatusCode  = "otel.status_code"
	tagStatusText  = "otel.status_description"
	tagEventName   = "event"
	statusCodeErr  = "ERROR"
	statusCodeOK   = "OK"
	traceIDLength  = 32
	spanIDLength   = 16
	microsPerMilli = 1000
)

var consumedTags = []string{tagError, tagSpanKind, tagStatusCode, tagStatusText}

type jaegerKeyValue struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type jaegerReference struct {
	RefType string `json:"refType"`
	TraceID string `json:"traceID"`
	SpanID  string `json:"spanID"`
}

type jaegerLog struct {
	Timestamp int64            `json:"timestamp"`
	Fields    []jaegerKeyValue `json:"fields"`
}

type jaegerSpan struct {
	TraceID       string            `json:"traceID"`
	SpanID        string            `json:"spanID"`
	ParentSpanID  string            `json:"parentSpanID"`
	OperationName string            `json:"operationName"`
	References    []jaegerReference `json:"references"`
	StartTime     int64             `json:"startTime"`
	Duration      int64             `json:"duration"`
	Tags          []jaegerKeyValue  `json:"tags"`
	Tag           map[string]any    `json:"tag"`
	Logs          []jaegerLog       `json:"logs"`
	Process       struct {
		ServiceName string           `json:"serviceName"`
		Tags        []jaegerKeyValue `json:"tags"`
		Tag         map[string]any   `json:"tag"`
	} `json:"process"`
}

type spanDoc struct {
	traceID, spanID, parentID, name, service string
	start                                    time.Time
	durationNanos                            int64
	status, statusMessage, kind              string
	attrs, resource                          map[string]any
	events                                   []datasource.SpanEvent
	links                                    []datasource.SpanLink
}

func decodeSpan(raw json.RawMessage) (spanDoc, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var s jaegerSpan
	if err := dec.Decode(&s); err != nil || s.SpanID == "" {
		return spanDoc{}, datasource.Unreadable()
	}
	tags := tagMap(s.Tags, s.Tag)
	doc := spanDoc{
		traceID:       normalizeID(s.TraceID, traceIDLength),
		spanID:        normalizeID(s.SpanID, spanIDLength),
		name:          s.OperationName,
		service:       s.Process.ServiceName,
		start:         time.UnixMicro(s.StartTime).UTC(),
		durationNanos: max(s.Duration, 0) * int64(time.Microsecond),
		status:        statusOf(tags),
		statusMessage: stringValue(tags[tagStatusText]),
		kind:          datasource.SpanKindName(stringValue(tags[tagSpanKind])),
		resource:      tagMap(s.Process.Tags, s.Process.Tag),
	}
	doc.resource["service.name"] = s.Process.ServiceName
	for _, key := range consumedTags {
		delete(tags, key)
	}
	doc.attrs = tags
	doc.parentID, doc.links = parentAndLinks(s, doc.traceID)
	doc.events = eventsOf(s.Logs)
	return doc, nil
}

// tagMap reads both forms Jaeger writes: the nested tags array and, with tags-as-fields, the tag object whose keys carry @ for each dot.
func tagMap(nested []jaegerKeyValue, flat map[string]any) map[string]any {
	out := make(map[string]any, len(nested)+len(flat))
	for key, value := range flat {
		out[strings.ReplaceAll(key, tagDot, ".")] = value
	}
	for _, kv := range nested {
		out[kv.Key] = kv.Value
	}
	return out
}

func statusOf(tags map[string]any) string {
	code := strings.ToUpper(stringValue(tags[tagStatusCode]))
	switch {
	case isTrue(tags[tagError]), code == statusCodeErr:
		return datasource.SpanStatusName(statusCodeErr)
	case code == statusCodeOK:
		return datasource.SpanStatusName(statusCodeOK)
	}
	return datasource.SpanStatusName("")
}

func isTrue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	}
	return false
}

func parentAndLinks(s jaegerSpan, traceID string) (string, []datasource.SpanLink) {
	parent := ""
	if s.ParentSpanID != "" {
		parent = normalizeID(s.ParentSpanID, spanIDLength)
	}
	var links []datasource.SpanLink
	for _, ref := range s.References {
		refTrace, refSpan := normalizeID(ref.TraceID, traceIDLength), normalizeID(ref.SpanID, spanIDLength)
		sameTrace := ref.TraceID == "" || refTrace == traceID
		switch {
		case ref.RefType == refChildOf && sameTrace && (parent == "" || parent == refSpan):
			parent = refSpan
		case refSpan != "":
			if !sameTrace || refSpan != parent {
				links = append(links, datasource.SpanLink{TraceID: refTrace, SpanID: refSpan})
			}
		}
	}
	return parent, links
}

func eventsOf(logs []jaegerLog) []datasource.SpanEvent {
	events := make([]datasource.SpanEvent, 0, len(logs))
	for _, l := range logs {
		attrs := tagMap(l.Fields, nil)
		name := stringValue(attrs[tagEventName])
		delete(attrs, tagEventName)
		events = append(events, datasource.SpanEvent{TimeMS: l.Timestamp / microsPerMilli, Name: name, Attributes: attrs})
	}
	return events
}

// normalizeID lower-cases an id and pads it with the leading zeros older Jaeger versions dropped, to 16 or 32 characters.
func normalizeID(id string, full int) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return ""
	}
	switch {
	case len(id) < spanIDLength:
		return strings.Repeat("0", spanIDLength-len(id)) + id
	case len(id) > spanIDLength && len(id) < full:
		return strings.Repeat("0", full-len(id)) + id
	}
	return id
}

// storedTraceIDs lists every form Jaeger may have stored a trace id in: as given, without leading zeros, and as 16 characters when the high half is zero.
func storedTraceIDs(id string) []string {
	id = strings.ToLower(id)
	forms := []string{id}
	add := func(form string) {
		if form != "" && !slices.Contains(forms, form) {
			forms = append(forms, form)
		}
	}
	add(strings.TrimLeft(id, "0"))
	if len(id) == traceIDLength && strings.Trim(id[:spanIDLength], "0") == "" {
		add(id[spanIDLength:])
	}
	return forms
}
