package clickhouse

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var spanAt = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func spanResultSet(kinds []string, events bool, links ...spanLinkRow) *resultSet {
	names := []string{"TraceId", "SpanId", "ParentSpanId", "SpanName", "ServiceName", "StartTime", "Duration", "StatusCode", "SpanAttributes", "ResourceAttributes", "SpanKind", "EventTimes", "EventNames", "EventAttributes", "LinkTraceIds", "LinkSpanIds", "LinkAttributes"}
	types := []datasource.FieldType{
		datasource.FieldString, datasource.FieldString, datasource.FieldString, datasource.FieldString, datasource.FieldString,
		datasource.FieldTime, datasource.FieldNumber, datasource.FieldString, datasource.FieldOther, datasource.FieldOther,
		datasource.FieldString, datasource.FieldOther, datasource.FieldOther, datasource.FieldOther,
		datasource.FieldOther, datasource.FieldOther, datasource.FieldOther,
	}
	rs := &resultSet{}
	for i := range names {
		rs.columns = append(rs.columns, column{name: names[i], typ: types[i]})
	}
	for i, kind := range kinds {
		var times []time.Time
		var eventNames []string
		var attrs []map[string]string
		if events && i == 0 {
			times = []time.Time{spanAt.Add(500 * time.Millisecond), spanAt.Add(time.Second)}
			eventNames = []string{"exception", "retry"}
			attrs = []map[string]string{{"exception.type": "TimeoutError"}, {}}
		}
		var linkTraceIDs, linkSpanIDs []string
		var linkAttrs []map[string]string
		for _, l := range links {
			if l.row == i {
				linkTraceIDs = append(linkTraceIDs, l.traceID)
				linkSpanIDs = append(linkSpanIDs, l.spanID)
				linkAttrs = append(linkAttrs, l.attrs)
			}
		}
		rs.rows = append(rs.rows, []any{
			"0af7651916cd43dd8448eb211c80319c", "b7ad6b716920333" + string(rune('0'+i)), "", "POST /checkout", "checkout",
			spanAt.Add(time.Duration(i) * time.Millisecond), uint64(50_000_000), "STATUS_CODE_ERROR",
			map[string]string{"http.method": "POST"}, map[string]string{"service.name": "checkout"},
			kind, times, eventNames, attrs, linkTraceIDs, linkSpanIDs, linkAttrs,
		})
	}
	return rs
}

func TestSpanFrameFoldsKindAndEvents(t *testing.T) {
	f, err := spanFrame(spanResultSet([]string{"SPAN_KIND_SERVER", "Client", "2", "", "SPAN_KIND_CONSUMER"}, true))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(f.Fields))
	for i, field := range f.Fields {
		names[i] = string(field.Type) + ":" + field.Name
	}
	want := []string{
		"string:TraceId", "string:SpanId", "string:ParentSpanId", "string:SpanName", "string:ServiceName",
		"time:StartTime", "number:Duration", "string:StatusCode", "string:SpanAttributes", "string:ResourceAttributes",
		"string:SpanKind", "string:SpanEvents", "string:SpanLinks",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
	checks := map[string][]any{
		"SpanKind":   {"Server", "Client", "Server", "Unspecified", "Consumer"},
		"StartTime":  {spanAt, spanAt.Add(time.Millisecond), spanAt.Add(2 * time.Millisecond), spanAt.Add(3 * time.Millisecond), spanAt.Add(4 * time.Millisecond)},
		"Duration":   {float64(50_000_000), float64(50_000_000), float64(50_000_000), float64(50_000_000), float64(50_000_000)},
		"StatusCode": {"STATUS_CODE_ERROR", "STATUS_CODE_ERROR", "STATUS_CODE_ERROR", "STATUS_CODE_ERROR", "STATUS_CODE_ERROR"},
		"SpanEvents": {
			`[{"time_ms":1793966400500,"name":"exception","attributes":{"exception.type":"TimeoutError"}},{"time_ms":1793966401000,"name":"retry","attributes":{}}]`,
			"[]", "[]", "[]", "[]",
		},
		"SpanAttributes":     {`{"http.method":"POST"}`, `{"http.method":"POST"}`, `{"http.method":"POST"}`, `{"http.method":"POST"}`, `{"http.method":"POST"}`},
		"ResourceAttributes": {`{"service.name":"checkout"}`, `{"service.name":"checkout"}`, `{"service.name":"checkout"}`, `{"service.name":"checkout"}`, `{"service.name":"checkout"}`},
	}
	for name, values := range checks {
		i := slices.IndexFunc(f.Fields, func(field datasource.Field) bool { return field.Name == name })
		if got := f.Fields[i].Values; !slices.Equal(got, values) {
			t.Errorf("%s = %v, want %v", name, got, values)
		}
	}
}

func TestSpanFrameWithNoRowsIsNotFound(t *testing.T) {
	if _, err := spanFrame(spanResultSet(nil, false)); !errors.Is(err, datasource.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestTraceQueriesPassTheGuard(t *testing.T) {
	for _, q := range []string{traceQuery, traceCountQuery} {
		if _, err := guard(q); err != nil {
			t.Errorf("guard refused %q: %v", q, err)
		}
	}
	want := "SELECT TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Timestamp AS StartTime, Duration, StatusCode, SpanAttributes, ResourceAttributes, SpanKind, Events.Timestamp AS EventTimes, Events.Name AS EventNames, Events.Attributes AS EventAttributes, Links.TraceId AS LinkTraceIds, Links.SpanId AS LinkSpanIds, Links.Attributes AS LinkAttributes\n" +
		"FROM otel_traces WHERE TraceId = {id:String} AND Timestamp >= {from:DateTime64(9, 'UTC')} AND Timestamp <= {to:DateTime64(9, 'UTC')}\n" +
		"ORDER BY Timestamp, SpanId LIMIT {n:UInt64}"
	if traceQuery != want {
		t.Fatalf("trace query =\n%s\nwant\n%s", traceQuery, want)
	}
}

func TestCountOfReadsTheFirstCell(t *testing.T) {
	rs := &resultSet{columns: []column{{name: "count()", typ: datasource.FieldNumber}}, rows: [][]any{{uint64(25000)}}}
	if n, ok := countOf(rs); !ok || n != 25000 {
		t.Fatalf("count = %d, %v", n, ok)
	}
	if _, ok := countOf(&resultSet{}); ok {
		t.Fatal("an empty result counted")
	}
}

type spanLinkRow struct {
	row             int
	traceID, spanID string
	attrs           map[string]string
}

func TestSpanFrameFoldsLinks(t *testing.T) {
	f, err := spanFrame(spanResultSet([]string{"Server", "Client"}, false,
		spanLinkRow{0, "00F067AA0BA902B700F067AA0BA902B7", "00F067AA0BA902B7", map[string]string{"kind": "follows_from"}},
		spanLinkRow{0, "1b2c3d4e5f60718293a4b5c6d7e8f901", "b7ad6b7169203331", nil},
	))
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(f.Fields, func(field datasource.Field) bool { return field.Name == "SpanLinks" })
	want := []any{
		`[{"trace_id":"00f067aa0ba902b700f067aa0ba902b7","span_id":"00f067aa0ba902b7","attributes":{"kind":"follows_from"}},` +
			`{"trace_id":"1b2c3d4e5f60718293a4b5c6d7e8f901","span_id":"b7ad6b7169203331","attributes":{}}]`,
		"[]",
	}
	if got := f.Fields[i].Values; !slices.Equal(got, want) {
		t.Fatalf("links = %v, want %v", got, want)
	}
}
