package clickhouse

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	traceWhere = "FROM otel_traces WHERE TraceId = {id:String} AND Timestamp >= {from:DateTime64(9, 'UTC')} AND Timestamp <= {to:DateTime64(9, 'UTC')}"
	traceQuery = "SELECT TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Timestamp AS StartTime, Duration, StatusCode, SpanAttributes, ResourceAttributes, SpanKind, " +
		"Events.Timestamp AS EventTimes, Events.Name AS EventNames, Events.Attributes AS EventAttributes, " +
		"Links.TraceId AS LinkTraceIds, Links.SpanId AS LinkSpanIds, Links.Attributes AS LinkAttributes\n" + traceWhere + "\nORDER BY Timestamp, SpanId LIMIT {n:UInt64}"
	traceCountQuery  = "SELECT count() " + traceWhere
	parameterTimeFmt = "2006-01-02 15:04:05.000000000"
)

var spanColumns = []string{"TraceId", "SpanId", "ParentSpanId", "SpanName", "ServiceName", "StartTime", "Duration", "StatusCode", "SpanAttributes", "ResourceAttributes", "SpanKind", "SpanEvents", "SpanLinks"}

func (d *Datasource) Trace(ctx context.Context, id string, r datasource.TimeRange, maxSpans int) (datasource.Frame, error) {
	lim := d.limits
	lim.MaxRows = maxSpans
	params := ch.Parameters{
		"id":   id,
		"from": r.From.UTC().Format(parameterTimeFmt),
		"to":   r.To.UTC().Format(parameterTimeFmt),
		"n":    strconv.Itoa(maxSpans),
	}
	var frame datasource.Frame
	err := d.read(ctx, lim, func(ctx context.Context) error {
		rs, err := d.fetch(ctx, traceQuery, lim, params)
		if err != nil {
			return err
		}
		if frame, err = spanFrame(rs); err != nil {
			return err
		}
		if len(rs.rows) < maxSpans {
			return nil
		}
		total, err := d.fetch(ctx, traceCountQuery, lim, params)
		if err != nil {
			return err
		}
		if n, ok := countOf(total); ok && n > maxSpans {
			frame.Meta = &datasource.FrameMeta{Truncated: true, RowsTotal: n}
		}
		return nil
	})
	return frame, err
}

func (d *Datasource) read(ctx context.Context, lim datasource.Limits, run func(ctx context.Context) error) error {
	if err := d.readOnly(ctx); err != nil {
		return err
	}
	lim.Timeout = d.clampTimeout(lim.Timeout)
	timeout := effectiveTimeout(ctx, lim.Timeout)
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()
	if err := run(ctx); err != nil {
		return mapError(ctx, err, timeout)
	}
	return nil
}

func countOf(rs *resultSet) (int, bool) {
	if len(rs.rows) == 0 || len(rs.rows[0]) == 0 {
		return 0, false
	}
	n, ok := toFloat(rs.rows[0][0])
	return int(n), ok
}

func spanFrame(rs *resultSet) (datasource.Frame, error) {
	if len(rs.rows) == 0 {
		return datasource.Frame{}, datasource.Errorf(datasource.ErrNotFound, "trace not found")
	}
	at := map[string]int{}
	for i, c := range rs.columns {
		at[c.name] = i
	}
	f := datasource.Frame{Fields: make([]datasource.Field, len(spanColumns))}
	for i, name := range spanColumns {
		typ := datasource.FieldString
		switch name {
		case "StartTime":
			typ = datasource.FieldTime
		case "Duration":
			typ = datasource.FieldNumber
		}
		f.Fields[i] = datasource.Field{Name: name, Type: typ, Values: make([]any, len(rs.rows))}
	}
	for r, row := range rs.rows {
		for i, name := range spanColumns {
			v, err := spanCell(name, row, at)
			if err != nil {
				return datasource.Frame{}, err
			}
			f.Fields[i].Values[r] = v
		}
	}
	return f, nil
}

func spanCell(name string, row []any, at map[string]int) (any, error) {
	cell := func(column string) any {
		if i, ok := at[column]; ok && i < len(row) {
			return row[i]
		}
		return nil
	}
	switch name {
	case "StartTime":
		return convert(cell(name), datasource.FieldTime), nil
	case "Duration":
		return convert(cell(name), datasource.FieldNumber), nil
	case "SpanAttributes", "ResourceAttributes":
		return convert(cell(name), datasource.FieldOther), nil
	case "SpanKind":
		return datasource.SpanKindName(stringOf(orEmpty(cell(name)))), nil
	case "SpanEvents":
		events, err := foldEvents(cell("EventTimes"), cell("EventNames"), cell("EventAttributes"))
		if err != nil {
			return nil, err
		}
		return datasource.SpanEventsJSON(events)
	case "SpanLinks":
		return datasource.SpanLinksJSON(foldLinks(cell("LinkTraceIds"), cell("LinkSpanIds"), cell("LinkAttributes")))
	}
	return stringOf(orEmpty(cell(name))), nil
}

func foldLinks(traceIDs, spanIDs, attributes any) []datasource.SpanLink {
	ts, ss, as := elements(traceIDs), elements(spanIDs), elements(attributes)
	links := make([]datasource.SpanLink, 0, len(ts))
	for i, t := range ts {
		l := datasource.SpanLink{TraceID: strings.ToLower(stringOf(t)), Attributes: map[string]any{}}
		if i < len(ss) {
			l.SpanID = strings.ToLower(stringOf(ss[i]))
		}
		if i < len(as) {
			if m, ok := jsonValue(reflect.ValueOf(as[i])).(map[string]any); ok {
				l.Attributes = m
			}
		}
		links = append(links, l)
	}
	return links
}

func foldEvents(times, names, attributes any) ([]datasource.SpanEvent, error) {
	ts, ns, as := elements(times), elements(names), elements(attributes)
	events := make([]datasource.SpanEvent, 0, len(ts))
	for i, t := range ts {
		at, ok := t.(time.Time)
		if !ok {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "span event time could not be read")
		}
		e := datasource.SpanEvent{TimeMS: at.UnixMilli(), Attributes: map[string]any{}}
		if i < len(ns) {
			e.Name = stringOf(ns[i])
		}
		if i < len(as) {
			if m, ok := jsonValue(reflect.ValueOf(as[i])).(map[string]any); ok {
				e.Attributes = m
			}
		}
		events = append(events, e)
	}
	return events, nil
}

func elements(v any) []any {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return nil
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out
}
