package tempo_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const eventTraceBody = `{"batches":[{
	"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}}]},
	"scopeSpans":[{"spans":[
		{"traceId":"CvdlGRbNQ92ESOshHIAxnA==","spanId":"t61rcWkgMzE=","name":"POST /checkout","kind":"SPAN_KIND_SERVER",
		 "startTimeUnixNano":"1793983968658000000","endTimeUnixNano":"1793983968700000000",
		 "events":[{"timeUnixNano":"1793983968658500000","name":"exception","attributes":[{"key":"exception.type","value":{"stringValue":"TimeoutError"}}]}],
		 "links":[{"traceId":"APBnqgupArcA8GeqC6kCtw==","spanId":"APBnqgupArc=","traceState":"","attributes":[]},
		          {"traceId":"00F067AA0BA902B700F067AA0BA902B7","spanId":"B7AD6B7169203331","attributes":[{"key":"kind","value":{"stringValue":"follows_from"}}]}],
		 "status":{"code":"STATUS_CODE_ERROR"}},
		{"traceId":"CvdlGRbNQ92ESOshHIAxnA==","spanId":"APBnqgupArc=","parentSpanId":"t61rcWkgMzE=","name":"charge","kind":3,
		 "startTimeUnixNano":"1793983968660000000","endTimeUnixNano":"1793983968690000000","status":{}},
		{"traceId":"CvdlGRbNQ92ESOshHIAxnA==","spanId":"AQIDBAUGBwg=","parentSpanId":"t61rcWkgMzE=","name":"queue",
		 "startTimeUnixNano":"1793983968661000000","endTimeUnixNano":"1793983968662000000","status":{}}
	]}]
}]}`

var traceStart = time.Unix(0, 1793966399950000000).UTC()

func manySpans(n int) string {
	spans := make([]string, n)
	for i := range n {
		start := traceStart.Add(time.Duration(n-1-i) * time.Millisecond)
		spans[i] = fmt.Sprintf(`{"traceId":"%s","spanId":"%016x","name":"s%d","startTimeUnixNano":"%d","endTimeUnixNano":"%d"}`,
			traceID, i+1, i, start.UnixNano(), start.Add(time.Millisecond).UnixNano())
	}
	return `{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}}]},"scopeSpans":[{"spans":[` + strings.Join(spans, ",") + `]}]}]}`
}

func TestTraceSendsRangeHints(t *testing.T) {
	ds, fake := serve(t, map[string]reply{tracePath: {0, traceBody}})
	if _, err := ds.Trace(context.Background(), traceID, lastHour, 100); err != nil {
		t.Fatal(err)
	}
	req := fake.last(t)
	if req.path != tracePath || req.params.Get("start") != "1793962800" || req.params.Get("end") != "1793966401" {
		t.Fatalf("request = %s %v, want start and end hints in seconds", req.path, req.params)
	}
}

func TestTraceKeepsTheEarliestSpansUnderTheBudget(t *testing.T) {
	ds, _ := serve(t, map[string]reply{tracePath: {0, manySpans(25)}})
	f, err := ds.Trace(context.Background(), traceID, lastHour, 10)
	if err != nil {
		t.Fatal(err)
	}
	if f.Rows() != 10 || f.Meta == nil || !f.Meta.Truncated || f.Meta.RowsTotal != 25 {
		t.Fatalf("rows = %d, meta = %+v; want 10 rows, truncated, 25 in total", f.Rows(), f.Meta)
	}
	if got := fieldValues(t, f, "SpanId")[0]; got != "0000000000000019" {
		t.Fatalf("first span = %v, want the earliest span", got)
	}
	if got := fieldValues(t, f, "StartTime")[0]; got != traceStart {
		t.Fatalf("first start = %v, want %v", got, traceStart)
	}
	whole, err := ds.Trace(context.Background(), traceID, lastHour, 25)
	if err != nil || whole.Rows() != 25 || whole.Meta == nil || whole.Meta.Truncated || whole.Meta.RowsTotal != 0 {
		t.Fatalf("rows = %d, meta = %+v, %v; want every span and no cut at the budget", whole.Rows(), whole.Meta, err)
	}
}

func TestTraceCarriesLinksAndBytes(t *testing.T) {
	ds, _ := serve(t, map[string]reply{tracePath: {0, eventTraceBody}})
	f, err := ds.Trace(context.Background(), traceID, lastHour, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{
		`[{"trace_id":"00f067aa0ba902b700f067aa0ba902b7","span_id":"00f067aa0ba902b7","attributes":{}},` +
			`{"trace_id":"00f067aa0ba902b700f067aa0ba902b7","span_id":"b7ad6b7169203331","attributes":{"kind":"follows_from"}}]`,
		`[]`,
		`[]`,
	}
	if got := fieldValues(t, f, "SpanLinks"); !slices.Equal(got, want) {
		t.Fatalf("links = %v, want %v", got, want)
	}
	if f.Meta == nil || f.Meta.Bytes != int64(len(eventTraceBody)) || f.Meta.Truncated {
		t.Fatalf("meta = %+v, want bytes %d", f.Meta, len(eventTraceBody))
	}
	badLink := strings.Replace(eventTraceBody, `"spanId":"APBnqgupArc=","traceState"`, `"spanId":"not-an-id","traceState"`, 1)
	ds, _ = serve(t, map[string]reply{tracePath: {0, badLink}})
	if _, err := ds.Trace(context.Background(), traceID, lastHour, 100); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v, want an unreadable upstream error for a bad link id", err)
	}
}

func TestTraceCarriesKindAndEvents(t *testing.T) {
	ds, _ := serve(t, map[string]reply{tracePath: {0, eventTraceBody}})
	f, err := ds.Trace(context.Background(), traceID, lastHour, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := fieldValues(t, f, "SpanKind"); !slices.Equal(got, []any{"Server", "Client", "Unspecified"}) {
		t.Fatalf("kinds = %v", got)
	}
	want := []any{
		`[{"time_ms":1793983968658,"name":"exception","attributes":{"exception.type":"TimeoutError"}}]`,
		`[]`,
		`[]`,
	}
	if got := fieldValues(t, f, "SpanEvents"); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestTraceNotFound(t *testing.T) {
	cases := map[string]map[string]reply{
		"404":      nil,
		"no spans": {tracePath: {0, `{"batches":[]}`}},
	}
	for name, replies := range cases {
		t.Run(name, func(t *testing.T) {
			ds, _ := serve(t, replies)
			if _, err := ds.Trace(context.Background(), traceID, lastHour, 100); !errors.Is(err, datasource.ErrNotFound) {
				t.Fatalf("err = %v, want not found", err)
			}
		})
	}
}

func TestTraceIDReturnsOneRowPerSpan(t *testing.T) {
	ds, fake := serve(t, map[string]reply{tracePath: {0, traceBody}})
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: " " + strings.ToUpper(traceID) + "\n"}, lastHour)
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.last(t).path; got != tracePath {
		t.Fatalf("path = %s, want the lowercased trace id", got)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %+v, want one", frames)
	}
	f := frames[0]
	want := []string{
		"string:TraceId", "string:SpanId", "string:ParentSpanId", "string:SpanName", "string:ServiceName",
		"time:StartTime", "number:Duration", "string:StatusCode", "string:SpanAttributes", "string:ResourceAttributes",
		"string:SpanKind", "string:SpanEvents", "string:SpanLinks",
	}
	if got := fieldNames(f); !slices.Equal(got, want) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
	checks := map[string][]any{
		"TraceId":      {traceID, traceID},
		"SpanId":       {"b7ad6b7169203331", "00f067aa0ba902b7"},
		"ParentSpanId": {"", "b7ad6b7169203331"},
		"SpanName":     {"POST /checkout", "charge"},
		"ServiceName":  {"checkout", "checkout"},
		"StartTime":    {time.Unix(0, 1793966399950000000).UTC(), time.Unix(0, 1793966399960000000).UTC()},
		"Duration":     {int64(50 * time.Millisecond), int64(30 * time.Millisecond)},
		"StatusCode":   {"Error", "Unset"},
		"SpanAttributes": {
			`{"http.method":"POST","http.status_code":500,"nested":{"k":"v"},"ratio":0.5,"retry":true,"tags":["a",2],"weird":"NaN"}`,
			`{}`,
		},
		"ResourceAttributes": {`{"k8s.pod.name":"checkout-0","service.name":"checkout"}`, `{"k8s.pod.name":"checkout-0","service.name":"checkout"}`},
		"SpanKind":           {"Server", "Client"},
		"SpanEvents":         {"[]", "[]"},
		"SpanLinks":          {"[]", "[]"},
	}
	for name, values := range checks {
		if got := fieldValues(t, f, name); !slices.Equal(got, values) {
			t.Errorf("%s = %v, want %v", name, got, values)
		}
	}
}

func TestTraceIDRowsAreLimited(t *testing.T) {
	ds, _ := serve(t, map[string]reply{tracePath: {0, traceBody}})
	frames, err := ds.QueryRange(context.Background(), datasource.Query{Text: traceID, Limits: datasource.Limits{MaxRows: 1}}, lastHour, time.Minute)
	if err != nil || len(frames) != 1 {
		t.Fatalf("frames = %+v, %v; want one frame", frames, err)
	}
	f := frames[0]
	if f.Rows() != 1 || f.Meta == nil || !f.Meta.Truncated || f.Meta.RowsTotal != 2 {
		t.Fatalf("rows = %d, meta = %+v; want the first span, truncated, 2 in total", f.Rows(), f.Meta)
	}
	if got := fieldValues(t, f, "SpanName"); !slices.Equal(got, []any{"POST /checkout"}) {
		t.Fatalf("names = %v, want the earliest span", got)
	}
}

func TestUnknownTraceIsEmpty(t *testing.T) {
	ds, _ := serve(t, nil)
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "b7ad6b7169203331"}, lastHour)
	if err != nil || len(frames) != 0 || frames == nil {
		t.Fatalf("frames = %#v, %v, want an empty list", frames, err)
	}
}
