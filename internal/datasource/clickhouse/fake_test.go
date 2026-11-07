package clickhouse

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type fakeColumn struct {
	name, typ string
}

func (c fakeColumn) Name() string             { return c.name }
func (c fakeColumn) Nullable() bool           { return false }
func (c fakeColumn) DatabaseTypeName() string { return c.typ }
func (c fakeColumn) ScanType() reflect.Type {
	switch c.typ {
	case "UInt64":
		return reflect.TypeFor[uint64]()
	case "DateTime64(9)":
		return reflect.TypeFor[time.Time]()
	}
	return reflect.TypeFor[string]()
}

func stringColumns(names ...string) []fakeColumn {
	out := make([]fakeColumn, len(names))
	for i, name := range names {
		out[i] = fakeColumn{name, "String"}
	}
	return out
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
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(v))
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
	columns []fakeColumn
	rows    [][]any
	keys    []string
	values  map[string][][]any

	mu         sync.Mutex
	statements []string
}

func (c *fakeConn) Query(_ context.Context, query string, _ ...any) (driver.Rows, error) {
	c.mu.Lock()
	c.statements = append(c.statements, query)
	c.mu.Unlock()
	switch {
	case strings.HasSuffix(query, "LIMIT 0"):
		return &fakeRows{columns: c.columns}, nil
	case strings.HasPrefix(query, "SELECT * FROM ("):
		return &fakeRows{columns: c.columns, rows: c.rows}, nil
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
