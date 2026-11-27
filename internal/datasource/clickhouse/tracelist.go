package clickhouse

import (
	"context"
	"strconv"
	"strings"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const listSelect = "SELECT TraceId, argMinIf(ServiceName, Timestamp, ParentSpanId = '') AS RootService, argMinIf(SpanName, Timestamp, ParentSpanId = '') AS Route, min(Timestamp) AS Start, " +
	"if(countIf(ParentSpanId = '') > 0, maxIf(Duration, ParentSpanId = ''), toUInt64(greatest(max(toUnixTimestamp64Nano(Timestamp) + Duration) - min(toUnixTimestamp64Nano(Timestamp)), 0))) AS TraceDuration\n" +
	"FROM otel_traces WHERE Timestamp >= {from:DateTime64(9, 'UTC')} AND Timestamp <= {to:DateTime64(9, 'UTC')}\nGROUP BY TraceId"

var _ datasource.TraceListSource = (*Datasource)(nil)

func ListQuery(req datasource.TraceListRequest) (string, ch.Parameters) {
	params := ch.Parameters{
		"from": req.Range.From.UTC().Format(parameterTimeFmt),
		"to":   req.Range.To.UTC().Format(parameterTimeFmt),
		"n":    strconv.Itoa(cmpPositive(req.Limit, datasource.TraceListBudget)),
	}
	var having []string
	if req.Errors {
		having = append(having, "countIf("+errorStatus+") > 0")
	}
	if req.Filter.Service != "" {
		having = append(having, "countIf(ServiceName = {service:String}) > 0")
		params["service"] = req.Filter.Service
	}
	if req.Filter.Operation != "" {
		having = append(having, "countIf(SpanName = {operation:String}) > 0")
		params["operation"] = req.Filter.Operation
	}
	if req.Filter.Route != "" {
		having = append(having, "Route = {route:String}")
		params["route"] = req.Filter.Route
	}
	if req.MinDuration > 0 {
		having = append(having, "TraceDuration >= {min:UInt64}")
		params["min"] = strconv.FormatInt(req.MinDuration.Nanoseconds(), 10)
	}
	text := listSelect
	if len(having) > 0 {
		text += " HAVING " + strings.Join(having, " AND ")
	}
	return text + "\nORDER BY Start DESC, TraceId LIMIT {n:UInt64}", params
}

func cmpPositive(n, fallback int) int {
	if n > 0 {
		return n
	}
	return fallback
}

func (d *Datasource) ListTraces(ctx context.Context, req datasource.TraceListRequest) (datasource.TraceList, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.TraceList{}, err
	}
	text, params := ListQuery(req)
	limit := cmpPositive(req.Limit, datasource.TraceListBudget)
	lim := d.limits.Tighten(req.Limits)
	lim.MaxRows = limit
	var out datasource.TraceList
	err := d.read(ctx, lim, func(ctx context.Context) error {
		rs, err := d.fetch(ctx, text, lim, params)
		if err != nil {
			return err
		}
		out = traceList(rs, limit)
		return nil
	})
	return out, err
}

func traceList(rs *resultSet, limit int) datasource.TraceList {
	columns := map[string]int{}
	for i, c := range rs.columns {
		columns[c.name] = i
	}
	out := datasource.TraceList{Traces: make([]datasource.TraceSummary, 0, len(rs.rows)), Complete: len(rs.rows) < limit}
	for _, row := range rs.rows {
		s := datasource.TraceSummary{Duration: time.Duration(cellInt(cellAt(row, columns, "TraceDuration")))}
		s.ID, _ = cellAt(row, columns, "TraceId").(string)
		s.RootService, _ = cellAt(row, columns, "RootService").(string)
		s.Route, _ = cellAt(row, columns, "Route").(string)
		s.Start, _ = cellAt(row, columns, "Start").(time.Time)
		s.ID = strings.ToLower(s.ID)
		out.Traces = append(out.Traces, s)
	}
	return out
}
