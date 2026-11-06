package clickhouse

import (
	"context"
	"strconv"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	traceLogsSelect   = "SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes FROM otel_logs WHERE Timestamp >= {from:DateTime64(9, 'UTC')} AND Timestamp <= {to:DateTime64(9, 'UTC')} AND TraceId = {trace:String}"
	traceLogsSpan     = " AND SpanId = {span:String}"
	traceLogsOrder    = " ORDER BY Timestamp DESC LIMIT {n:UInt64}"
	clickHouseTimeFmt = "2006-01-02 15:04:05.000000000"
)

var _ datasource.TraceLogSource = (*Datasource)(nil)

func (d *Datasource) TraceLogs(ctx context.Context, req datasource.TraceLogsRequest) ([]datasource.Frame, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := d.readOnly(ctx); err != nil {
		return nil, err
	}
	lim := d.limits
	lim.MaxRows = req.Limit
	lim.Timeout = d.clampTimeout(lim.Timeout)
	timeout := effectiveTimeout(ctx, lim.Timeout)
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()
	text, params := traceLogsQuery(req)
	rs, err := d.fetchWith(ctx, text, lim, params, nil)
	if err != nil {
		return nil, mapError(ctx, err, timeout)
	}
	return markTruncated([]datasource.Frame{rs.tableFrame()}, rs, lim), nil
}

func traceLogsQuery(req datasource.TraceLogsRequest) (string, ch.Parameters) {
	text := traceLogsSelect
	params := ch.Parameters{
		"from":  clickHouseTime(req.Range.From),
		"to":    clickHouseTime(req.Range.To),
		"trace": req.TraceID,
		"n":     strconv.Itoa(req.Limit),
	}
	if req.SpanID != "" {
		text += traceLogsSpan
		params["span"] = req.SpanID
	}
	return text + traceLogsOrder, params
}

func clickHouseTime(t time.Time) string {
	return t.UTC().Format(clickHouseTimeFmt)
}
