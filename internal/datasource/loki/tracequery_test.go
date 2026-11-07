package loki

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestTraceQueriesQuoteIds(t *testing.T) {
	const selector = `{service_name="checkout"}`
	cases := []struct {
		name         string
		traceID      string
		spanID       string
		wantMetadata string
		wantLine     string
	}{
		{
			name:         "hex ids",
			traceID:      "0af7651916cd43dd8448eb211c80319c",
			spanID:       "b7ad6b7169203331",
			wantMetadata: `{service_name="checkout"} | trace_id="0af7651916cd43dd8448eb211c80319c" | span_id="b7ad6b7169203331"`,
			wantLine:     `{service_name="checkout"} |= "0af7651916cd43dd8448eb211c80319c" |= "b7ad6b7169203331"`,
		},
		{
			name:         "trace id only",
			traceID:      "0af7651916cd43dd8448eb211c80319c",
			wantMetadata: `{service_name="checkout"} | trace_id="0af7651916cd43dd8448eb211c80319c"`,
			wantLine:     `{service_name="checkout"} |= "0af7651916cd43dd8448eb211c80319c"`,
		},
		{
			name:         "quote and backslash stay inside the string",
			traceID:      `a"b\c`,
			spanID:       `x" | {job=~".+"} |= "y`,
			wantMetadata: `{service_name="checkout"} | trace_id="a\"b\\c" | span_id="x\" | {job=~\".+\"} |= \"y"`,
			wantLine:     `{service_name="checkout"} |= "a\"b\\c" |= "x\" | {job=~\".+\"} |= \"y"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := datasource.TraceLogsRequest{TraceID: c.traceID, SpanID: c.spanID}
			if got := metadataTraceQuery(selector, req); got != c.wantMetadata {
				t.Errorf("metadata query = %s, want %s", got, c.wantMetadata)
			}
			if got := lineFilterTraceQuery(selector, req); got != c.wantLine {
				t.Errorf("line filter query = %s, want %s", got, c.wantLine)
			}
		})
	}
}
