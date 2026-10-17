package clickhouse

import (
	"encoding"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type column struct {
	name string
	typ  datasource.FieldType
}

type resultSet struct {
	columns []column
	rows    [][]any
}

var numericTypes = map[string]bool{
	"Int8": true, "Int16": true, "Int32": true, "Int64": true, "Int128": true, "Int256": true,
	"UInt8": true, "UInt16": true, "UInt32": true, "UInt64": true, "UInt128": true, "UInt256": true,
	"Float32": true, "Float64": true, "BFloat16": true,
}

func unwrapType(name string) string {
	for {
		switch {
		case strings.HasPrefix(name, "Nullable(") && strings.HasSuffix(name, ")"):
			name = name[len("Nullable(") : len(name)-1]
		case strings.HasPrefix(name, "LowCardinality(") && strings.HasSuffix(name, ")"):
			name = name[len("LowCardinality(") : len(name)-1]
		default:
			return name
		}
	}
}

func fieldType(databaseType string) datasource.FieldType {
	name := unwrapType(databaseType)
	base, _, _ := strings.Cut(name, "(")
	switch {
	case base == "Date", base == "Date32", base == "DateTime", base == "DateTime64":
		return datasource.FieldTime
	case numericTypes[name], base == "Decimal", base == "Decimal32", base == "Decimal64", base == "Decimal128", base == "Decimal256":
		return datasource.FieldNumber
	case base == "String", base == "FixedString", base == "Enum8", base == "Enum16", base == "UUID", base == "IPv4", base == "IPv6":
		return datasource.FieldString
	case base == "Bool":
		return datasource.FieldBool
	}
	return datasource.FieldOther
}

func scanRows(rows driver.Rows, maxRows int) (*resultSet, error) {
	defer func() { _ = rows.Close() }()
	types := rows.ColumnTypes()
	rs := &resultSet{columns: make([]column, len(types))}
	for i, ct := range types {
		rs.columns[i] = column{name: ct.Name(), typ: fieldType(ct.DatabaseTypeName())}
	}
	for rows.Next() {
		if maxRows > 0 && len(rs.rows) >= maxRows {
			return nil, datasource.Errorf(datasource.ErrLimitExceeded, "result exceeds %d rows", maxRows)
		}
		dest := make([]any, len(types))
		for i, ct := range types {
			dest[i] = reflect.New(ct.ScanType()).Interface()
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := make([]any, len(dest))
		for i, p := range dest {
			row[i] = deref(p)
		}
		rs.rows = append(rs.rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rs, nil
}

func deref(p any) any {
	v := reflect.ValueOf(p).Elem()
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		if v.Type() == reflect.TypeFor[*big.Int]() {
			break
		}
		v = v.Elem()
	}
	return v.Interface()
}

func convert(v any, typ datasource.FieldType) any {
	if v == nil {
		return nil
	}
	switch typ {
	case datasource.FieldTime:
		if t, ok := v.(time.Time); ok {
			return t.UTC()
		}
	case datasource.FieldNumber:
		if f, ok := toFloat(v); ok {
			return datasource.Number(f)
		}
		return nil
	case datasource.FieldBool:
		if b, ok := v.(bool); ok {
			return b
		}
	case datasource.FieldString:
		return stringOf(v)
	}
	b, err := json.Marshal(jsonValue(reflect.ValueOf(v)))
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case *big.Int:
		f, _ := new(big.Float).SetInt(n).Float64()
		return f, true
	case big.Int:
		f, _ := new(big.Float).SetInt(&n).Float64()
		return f, true
	case interface{ InexactFloat64() float64 }:
		return n.InexactFloat64(), true
	}
	rv := reflect.ValueOf(v)
	switch {
	case rv.CanInt():
		return float64(rv.Int()), true
	case rv.CanUint():
		return float64(rv.Uint()), true
	case rv.CanFloat():
		return rv.Float(), true
	}
	return 0, false
}

func stringOf(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case fmt.Stringer:
		return s.String()
	}
	return fmt.Sprint(v)
}

func (rs *resultSet) tableFrame() datasource.Frame {
	fields := make([]datasource.Field, len(rs.columns))
	for i, c := range rs.columns {
		values := make([]any, len(rs.rows))
		for r, row := range rs.rows {
			values[r] = convert(row[i], c.typ)
		}
		fields[i] = datasource.Field{Name: c.name, Type: c.typ, Values: values}
	}
	return datasource.Frame{Fields: fields}
}

func (rs *resultSet) seriesFrames() []datasource.Frame {
	if len(rs.columns) == 0 || rs.columns[0].typ != datasource.FieldTime {
		return []datasource.Frame{rs.tableFrame()}
	}
	var labelCols, valueCols []int
	for i := 1; i < len(rs.columns); i++ {
		if rs.columns[i].typ == datasource.FieldString {
			labelCols = append(labelCols, i)
		} else {
			valueCols = append(valueCols, i)
		}
	}
	newFrame := func(labels map[string]string) *datasource.Frame {
		f := &datasource.Frame{Labels: labels, Fields: make([]datasource.Field, 0, 1+len(valueCols))}
		f.Fields = append(f.Fields, datasource.Field{Name: "Time", Type: datasource.FieldTime, Values: []any{}})
		for _, i := range valueCols {
			f.Fields = append(f.Fields, datasource.Field{Name: rs.columns[i].name, Type: rs.columns[i].typ, Values: []any{}})
		}
		return f
	}
	var order []string
	groups := map[string]*datasource.Frame{}
	for _, row := range rs.rows {
		var key strings.Builder
		for _, i := range labelCols {
			key.WriteString(stringOf(orEmpty(row[i])))
			key.WriteByte(0)
		}
		f, ok := groups[key.String()]
		if !ok {
			var labels map[string]string
			if len(labelCols) > 0 {
				labels = make(map[string]string, len(labelCols))
				for _, i := range labelCols {
					labels[rs.columns[i].name] = stringOf(orEmpty(row[i]))
				}
			}
			f = newFrame(labels)
			groups[key.String()] = f
			order = append(order, key.String())
		}
		f.Fields[0].Values = append(f.Fields[0].Values, convert(row[0], datasource.FieldTime))
		for n, i := range valueCols {
			f.Fields[n+1].Values = append(f.Fields[n+1].Values, convert(row[i], rs.columns[i].typ))
		}
	}
	frames := make([]datasource.Frame, 0, len(order))
	for _, k := range order {
		frames = append(frames, *groups[k])
	}
	if len(frames) == 0 {
		frames = append(frames, *newFrame(nil))
	}
	return frames
}

func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
)

func marshalable(v reflect.Value) (any, bool) {
	implements := func(t reflect.Type) bool {
		return t.Implements(jsonMarshalerType) || t.Implements(textMarshalerType)
	}
	if implements(v.Type()) {
		return v.Interface(), true
	}
	if v.CanAddr() && implements(reflect.PointerTo(v.Type())) {
		return v.Addr().Interface(), true
	}
	return nil, false
}

// encoding/json renders []uint8 as base64, but an Array(UInt8) must stay a list of numbers.
// Values that marshal themselves (UUID, IP, big.Int) are left to their marshaler.
func jsonValue(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Invalid:
		return nil
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
	}
	if v.Kind() == reflect.Interface {
		return jsonValue(v.Elem())
	}
	if m, ok := marshalable(v); ok {
		return m
	}
	switch v.Kind() {
	case reflect.Pointer:
		return jsonValue(v.Elem())
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := range out {
			out[i] = jsonValue(v.Index(i))
		}
		return out
	case reflect.Map:
		out := make(map[string]any, v.Len())
		for iter := v.MapRange(); iter.Next(); {
			out[stringOf(iter.Key().Interface())] = jsonValue(iter.Value())
		}
		return out
	}
	return v.Interface()
}
