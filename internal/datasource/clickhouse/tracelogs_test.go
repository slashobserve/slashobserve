package clickhouse

import (
	"reflect"
	"testing"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestTraceLogsQueryBindsEveryValueAsAParameter(t *testing.T) {
	from := time.Date(2026, 11, 6, 11, 59, 0, 500_000_000, time.FixedZone("IST", 5*3600+1800))
	rng := datasource.TimeRange{From: from, To: from.Add(2 * time.Minute)}
	base := "SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes FROM otel_logs WHERE Timestamp >= {from:DateTime64(9, 'UTC')} AND Timestamp <= {to:DateTime64(9, 'UTC')} AND TraceId = {trace:String}"
	tests := []struct {
		name       string
		req        datasource.TraceLogsRequest
		wantText   string
		wantParams ch.Parameters
	}{
		{
			name:     "trace only",
			req:      datasource.TraceLogsRequest{TraceID: "0af7651916cd43dd8448eb211c80319c", Range: rng, Limit: 500},
			wantText: base + " ORDER BY Timestamp DESC LIMIT {n:UInt64}",
			wantParams: ch.Parameters{
				"from":  "2026-11-06 06:29:00.500000000",
				"to":    "2026-11-06 06:31:00.500000000",
				"trace": "0af7651916cd43dd8448eb211c80319c",
				"n":     "500",
			},
		},
		{
			name:     "trace and span",
			req:      datasource.TraceLogsRequest{TraceID: "0af7651916cd43dd8448eb211c80319c", SpanID: "b7ad6b7169203331", Range: rng, Limit: 7},
			wantText: base + " AND SpanId = {span:String} ORDER BY Timestamp DESC LIMIT {n:UInt64}",
			wantParams: ch.Parameters{
				"from":  "2026-11-06 06:29:00.500000000",
				"to":    "2026-11-06 06:31:00.500000000",
				"trace": "0af7651916cd43dd8448eb211c80319c",
				"span":  "b7ad6b7169203331",
				"n":     "7",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, params := traceLogsQuery(tt.req)
			if text != tt.wantText {
				t.Fatalf("text = %q, want %q", text, tt.wantText)
			}
			if !reflect.DeepEqual(params, tt.wantParams) {
				t.Fatalf("params = %v, want %v", params, tt.wantParams)
			}
		})
	}
}
