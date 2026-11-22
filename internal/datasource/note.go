package datasource

import (
	"context"
	"strings"

	"github.com/slashobserve/slashobserve/internal/audit"
)

func NoteQuery(ctx context.Context, uid, text string, rng TimeRange, rows int) {
	audit.NoteQuery(ctx, audit.Query{Datasource: uid, Text: text, From: rng.From, To: rng.To, Rows: rows})
}

func FrameRows(frames []Frame) int {
	var rows int
	for _, f := range frames {
		rows += f.Rows()
	}
	return rows
}

func FacetValues(f Facets) int {
	var values int
	for _, field := range f.Fields {
		values += len(field.Values)
	}
	return values
}

func (r TraceLogsRequest) Text() string {
	parts := []string{"trace logs " + r.TraceID}
	if r.SpanID != "" {
		parts = append(parts, "span "+r.SpanID)
	}
	if r.Service != "" {
		parts = append(parts, "service "+r.Service)
	}
	return strings.Join(parts, " ")
}

func (r TraceAttributesRequest) Text() string {
	return "trace attributes " + strings.Join(r.IDs, ",")
}
