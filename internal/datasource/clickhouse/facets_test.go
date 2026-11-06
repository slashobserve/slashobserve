package clickhouse

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/ttlcache"
)

type fakeColumn struct {
	name, typ string
}

func (c fakeColumn) Name() string             { return c.name }
func (c fakeColumn) Nullable() bool           { return false }
func (c fakeColumn) DatabaseTypeName() string { return c.typ }
func (c fakeColumn) ScanType() reflect.Type {
	if c.typ == "UInt64" {
		return reflect.TypeFor[uint64]()
	}
	return reflect.TypeFor[string]()
}

type fakeRows struct {
	driver.Rows
	columns []fakeColumn
	rows    [][]any
	at      int
}

func (r *fakeRows) Next() bool { r.at++; return r.at <= len(r.rows) }
func (r *fakeRows) Scan(dest ...any) error {
	for i, v := range r.rows[r.at-1] {
		switch d := dest[i].(type) {
		case *string:
			*d = v.(string)
		case *uint64:
			*d = v.(uint64)
		}
	}
	return nil
}
func (r *fakeRows) ColumnTypes() []driver.ColumnType {
	out := make([]driver.ColumnType, len(r.columns))
	for i, c := range r.columns {
		out[i] = c
	}
	return out
}
func (r *fakeRows) Close() error { return nil }
func (r *fakeRows) Err() error   { return nil }

type fakeConn struct {
	driver.Conn
	mu         sync.Mutex
	statements []string
	columns    []string
	keys       []string
	values     map[string][][]any
}

func (c *fakeConn) Query(_ context.Context, query string, _ ...any) (driver.Rows, error) {
	c.mu.Lock()
	c.statements = append(c.statements, query)
	c.mu.Unlock()
	switch {
	case strings.HasPrefix(query, "SELECT * FROM ("):
		columns := make([]fakeColumn, len(c.columns))
		for i, name := range c.columns {
			columns[i] = fakeColumn{name: name, typ: "String"}
		}
		return &fakeRows{columns: columns}, nil
	case strings.HasPrefix(query, "SELECT arrayJoin(mapKeys("):
		rows := make([][]any, len(c.keys))
		var remaining uint64 = 1 << 10
		for i, key := range c.keys {
			rows[i] = []any{key, remaining}
			remaining--
		}
		return &fakeRows{columns: []fakeColumn{{"K", "String"}, {"count()", "UInt64"}}, rows: rows}, nil
	}
	_, after, _ := strings.Cut(query, "SELECT toString(")
	expr, _, _ := strings.Cut(after, ") AS Value")
	return &fakeRows{columns: []fakeColumn{{"Value", "String"}, {"Count", "UInt64"}}, rows: c.values[expr]}, nil
}

func (c *fakeConn) sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.statements...)
}

func facetsDS(conn *fakeConn) *Datasource {
	d := &Datasource{conn: conn, limits: datasource.Limits{}.WithDefaults(datasource.DefaultLimits), database: "otel", columns: ttlcache.New[tableName, []string](columnCacheTTL, columnCacheCost)}
	d.readOnlyUntil.Store(time.Now().Add(time.Hour).UnixNano())
	return d
}

func facetReq(query string, fields []string, limit int) datasource.FacetRequest {
	end := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	return datasource.FacetRequest{Query: datasource.Query{Text: query}, Range: datasource.TimeRange{From: end.Add(-time.Hour), To: end}, Fields: fields, Limit: limit, Sample: 1000}
}

const facetInner = "SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes FROM otel_logs WHERE (Timestamp >= fromUnixTimestamp64Milli(toInt64(1793962800000), 'UTC') AND Timestamp <= fromUnixTimestamp64Milli(toInt64(1793966400000), 'UTC')) "

func TestLogFacetsGroupsEachDefaultColumnAndAttributeKey(t *testing.T) {
	conn := &fakeConn{
		columns: []string{"Timestamp", "SeverityText", "Body", "ServiceName", "TraceId", "SpanId", "LogAttributes"},
		keys:    []string{"http.method", "http.route"},
		values: map[string][][]any{
			"`ServiceName`":                 {{"frontend", uint64(48210)}, {"checkout", uint64(9931)}},
			"`SeverityText`":                {{"INFO", uint64(50000)}, {"ERROR", uint64(8000)}, {"WARN", uint64(141)}},
			"`LogAttributes`[{key:String}]": {{"POST", uint64(612)}, {"GET", uint64(388)}},
		},
	}
	got, err := facetsDS(conn).LogFacets(context.Background(), facetReq(webLogsTemplate, nil, 2))
	if err != nil {
		t.Fatal(err)
	}
	if got.Sampled || got.LinesSampled != 0 || got.Truncated {
		t.Fatalf("facets = %+v, want exact unsampled counts", got)
	}
	var names []string
	for _, f := range got.Fields {
		names = append(names, f.Name)
	}
	if want := []string{"SeverityText", "ServiceName", "LogAttributes.http.method", "LogAttributes.http.route"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
	service := got.Fields[1]
	want := datasource.FacetField{Name: "ServiceName", Kind: "column", Exact: true, Cardinality: 2, Values: []datasource.FacetValue{{Value: "frontend", Count: 48210}, {Value: "checkout", Count: 9931}}}
	if !reflect.DeepEqual(service, want) {
		t.Fatalf("ServiceName = %+v, want %+v", service, want)
	}
	if level := got.Fields[0]; !level.Truncated || level.Cardinality != 3 || len(level.Values) != 2 {
		t.Fatalf("SeverityText = %+v, want the top 2 of 3 values and truncated", level)
	}
	if method := got.Fields[2]; method.Truncated || !reflect.DeepEqual(method.Values, []datasource.FacetValue{{Value: "POST", Count: 612}, {Value: "GET", Count: 388}}) {
		t.Fatalf("http.method = %+v", method)
	}
	sent := conn.sent()
	wantStatements := []string{
		"SELECT * FROM (\n" + facetInner + "\n) LIMIT 0",
		"SELECT arrayJoin(mapKeys(`LogAttributes`)) AS K, count() FROM (\n" + facetInner + "\n) GROUP BY K ORDER BY count() DESC LIMIT 20",
		"SELECT toString(`SeverityText`) AS Value, count() AS Count FROM (\n" + facetInner + "\n) GROUP BY Value ORDER BY Count DESC, Value LIMIT 3",
		"SELECT toString(`ServiceName`) AS Value, count() AS Count FROM (\n" + facetInner + "\n) GROUP BY Value ORDER BY Count DESC, Value LIMIT 3",
		"SELECT toString(`LogAttributes`[{key:String}]) AS Value, count() AS Count FROM (\n" + facetInner + "\n) GROUP BY Value ORDER BY Count DESC, Value LIMIT 3",
		"SELECT toString(`LogAttributes`[{key:String}]) AS Value, count() AS Count FROM (\n" + facetInner + "\n) GROUP BY Value ORDER BY Count DESC, Value LIMIT 3",
	}
	if len(sent) != len(wantStatements) || !reflect.DeepEqual(sent[:2], wantStatements[:2]) {
		t.Fatalf("statements =\n%s\nwant\n%s", strings.Join(sent, "\n"), strings.Join(wantStatements, "\n"))
	}
	groups := slices.Sorted(slices.Values(sent[2:]))
	if want := slices.Sorted(slices.Values(wantStatements[2:])); !reflect.DeepEqual(groups, want) {
		t.Fatalf("group statements =\n%s\nwant\n%s", strings.Join(groups, "\n"), strings.Join(want, "\n"))
	}
}

func TestLogFacetsCountsOnlyTheRequestedFields(t *testing.T) {
	conn := &fakeConn{columns: []string{"Timestamp", "Body", "ServiceName", "LogAttributes"}, values: map[string][][]any{
		"`Body`":                        {{"hello", uint64(1)}},
		"`LogAttributes`[{key:String}]": {{"/cart", uint64(7)}},
	}}
	got, err := facetsDS(conn).LogFacets(context.Background(), facetReq(webLogsTemplate, []string{"LogAttributes.http.route", "Body"}, 10))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Fields) != 2 || got.Fields[0].Name != "Body" || got.Fields[1].Name != "LogAttributes.http.route" {
		t.Fatalf("fields = %+v", got.Fields)
	}
	for _, statement := range conn.sent() {
		if strings.HasPrefix(statement, "SELECT arrayJoin(") {
			t.Fatalf("the attribute key query ran for an explicit field list")
		}
	}
	_, err = facetsDS(conn).LogFacets(context.Background(), facetReq(webLogsTemplate, []string{"Missing"}, 10))
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), `field "Missing" is not a column of the query`) {
		t.Fatalf("err = %v", err)
	}
	_, err = facetsDS(&fakeConn{columns: []string{"Body"}}).LogFacets(context.Background(), facetReq("SELECT Body FROM otel_logs", []string{"LogAttributes.http.route"}, 10))
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("an attribute key of a query without LogAttributes: err = %v", err)
	}
}

func TestLogFacetsRefusesWhatVolumeInnerRefuses(t *testing.T) {
	conn := &fakeConn{}
	_, err := facetsDS(conn).LogFacets(context.Background(), facetReq("SELECT * FROM a UNION ALL SELECT * FROM b", nil, 10))
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.HasSuffix(err.Error(), volumeNeedsSelect) {
		t.Fatalf("err = %v", err)
	}
	if len(conn.sent()) != 0 {
		t.Fatalf("statements ran for a refused query: %v", conn.sent())
	}
}
