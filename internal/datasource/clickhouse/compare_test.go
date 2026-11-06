package clickhouse

import (
	"fmt"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestCompareQueryPassesTheGuard(t *testing.T) {
	if _, err := guard(compareQuery); err != nil {
		t.Fatalf("guard refused %q: %v", compareQuery, err)
	}
	want := "SELECT TraceId, groupUniqArray(10000)(Pair) AS Pairs\nFROM (\n" +
		"SELECT TraceId, arrayJoin(arrayConcat(\n" +
		"  [('name', SpanName), ('kind', toString(SpanKind)), ('status', toString(StatusCode))],\n" +
		"  arrayMap((k, v) -> (concat('span.', k), v), mapKeys(SpanAttributes), mapValues(SpanAttributes)),\n" +
		"  arrayMap((k, v) -> (concat('resource.', k), v), mapKeys(ResourceAttributes), mapValues(ResourceAttributes)))) AS Pair\n" +
		"FROM (\n" +
		"SELECT TraceId, SpanName, SpanKind, StatusCode, SpanAttributes, ResourceAttributes\n" +
		"FROM otel_traces WHERE TraceId IN {ids:Array(String)} AND Timestamp >= {from:DateTime64(9, 'UTC')} AND Timestamp <= {to:DateTime64(9, 'UTC')}\n" +
		"ORDER BY Timestamp LIMIT {spans:UInt64} BY TraceId\n" +
		")\n)\nGROUP BY TraceId"
	if compareQuery != want {
		t.Fatalf("compare query =\n%s\nwant\n%s", compareQuery, want)
	}
}

func TestArrayLiteral(t *testing.T) {
	if got := arrayLiteral([]string{"0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331"}); got != "['0af7651916cd43dd8448eb211c80319c','b7ad6b7169203331']" {
		t.Fatalf("literal = %s", got)
	}
	if got := arrayLiteral(nil); got != "[]" {
		t.Fatalf("empty literal = %s", got)
	}
}

func TestTraceAttributeSetsFromResultSet(t *testing.T) {
	rs := &resultSet{
		columns: []column{{name: "TraceId", typ: datasource.FieldString}, {name: "Pairs", typ: datasource.FieldOther}},
		rows: [][]any{
			{"B7AD6B7169203331", [][]any{{"name", "charge"}, {"kind", "SPAN_KIND_CLIENT"}, {"status", "STATUS_CODE_ERROR"}, {"span.db.system", "postgres"}}},
			{"0af7651916cd43dd8448eb211c80319c", [][]any{
				{"status", "STATUS_CODE_OK"}, {"status", "Ok"}, {"kind", "2"}, {"name", "POST /checkout"},
				{"resource.service.name", "checkout"}, {"span.http.method", "POST"}, {"broken"},
			}},
		},
	}
	sets := traceAttributeSets(rs, []string{"0af7651916cd43dd8448eb211c80319c", "00f067aa0ba902b700f067aa0ba902b7", "b7ad6b7169203331"})
	want := "[{0af7651916cd43dd8448eb211c80319c [{kind Server} {name POST /checkout} {resource.service.name checkout} {span.http.method POST} {status Ok}]} " +
		"{b7ad6b7169203331 [{kind Client} {name charge} {span.db.system postgres} {status Error}]}]"
	if got := fmt.Sprint(sets); got != want {
		t.Fatalf("sets =\n%s\nwant\n%s", got, want)
	}
	if got := traceAttributeSets(&resultSet{columns: rs.columns}, []string{"b7ad6b7169203331"}); len(got) != 0 || got == nil {
		t.Fatalf("no rows = %#v, want an empty list", got)
	}
}
