package clickhouse

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	comparePairsCap   = "10000"
	compareSpansQuery = "SELECT TraceId, SpanName, SpanKind, StatusCode, SpanAttributes, ResourceAttributes\n" +
		"FROM otel_traces WHERE TraceId IN {ids:Array(String)} AND Timestamp >= {from:DateTime64(9, 'UTC')} AND Timestamp <= {to:DateTime64(9, 'UTC')}\n" +
		"ORDER BY Timestamp LIMIT {spans:UInt64} BY TraceId"
	comparePairsQuery = "SELECT TraceId, arrayJoin(arrayConcat(\n" +
		"  [('name', SpanName), ('kind', toString(SpanKind)), ('status', toString(StatusCode))],\n" +
		"  arrayMap((k, v) -> (concat('span.', k), v), mapKeys(SpanAttributes), mapValues(SpanAttributes)),\n" +
		"  arrayMap((k, v) -> (concat('resource.', k), v), mapKeys(ResourceAttributes), mapValues(ResourceAttributes)))) AS Pair\n" +
		"FROM (\n" + compareSpansQuery + "\n)"
	compareQuery = "SELECT TraceId, groupUniqArray(" + comparePairsCap + ")(Pair) AS Pairs\nFROM (\n" + comparePairsQuery + "\n)\nGROUP BY TraceId"
)

var _ datasource.TraceAttributeSource = (*Datasource)(nil)

func (d *Datasource) TraceAttributes(ctx context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
	if err := req.Range.Validate(); err != nil {
		return nil, err
	}
	lim := d.limits
	lim.MaxRows = len(req.IDs)
	params := ch.Parameters{
		"ids":   arrayLiteral(req.IDs),
		"from":  req.Range.From.UTC().Format(parameterTimeFmt),
		"to":    req.Range.To.UTC().Format(parameterTimeFmt),
		"spans": strconv.Itoa(req.MaxSpans),
	}
	var rs *resultSet
	err := d.read(ctx, lim, func(ctx context.Context) error {
		var err error
		rs, err = d.fetch(ctx, compareQuery, lim, params)
		return err
	})
	if err != nil {
		return nil, err
	}
	return traceAttributeSets(rs, req.IDs), nil
}

func arrayLiteral(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = quoteString(v)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

func traceAttributeSets(rs *resultSet, ids []string) []datasource.TraceAttributeSet {
	columns := map[string]int{}
	for i, c := range rs.columns {
		columns[c.name] = i
	}
	byID := make(map[string][]datasource.TraceAttribute, len(rs.rows))
	for _, row := range rs.rows {
		id := strings.ToLower(stringOf(orEmpty(cellAt(row, columns, "TraceId"))))
		byID[id] = pairsFrom(cellAt(row, columns, "Pairs"))
	}
	sets := make([]datasource.TraceAttributeSet, 0, len(byID))
	for _, id := range ids {
		if pairs, ok := byID[id]; ok {
			sets = append(sets, datasource.TraceAttributeSet{ID: id, Attributes: pairs})
		}
	}
	return sets
}

func pairsFrom(cell any) []datasource.TraceAttribute {
	tuples := elements(cell)
	pairs := make([]datasource.TraceAttribute, 0, len(tuples))
	for _, tuple := range tuples {
		kv := elements(tuple)
		if len(kv) != 2 {
			continue
		}
		key, value := stringOf(orEmpty(kv[0])), stringOf(orEmpty(kv[1]))
		switch key {
		case "kind":
			value = datasource.SpanKindName(value)
		case "status":
			value = datasource.SpanStatusName(value)
		}
		pairs = append(pairs, datasource.TraceAttribute{Key: key, Value: value})
	}
	slices.SortFunc(pairs, func(a, b datasource.TraceAttribute) int {
		return cmp.Or(cmp.Compare(a.Key, b.Key), cmp.Compare(a.Value, b.Value))
	})
	return slices.Compact(pairs)
}
