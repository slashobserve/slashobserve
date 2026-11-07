package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	defaultTraceLogsLimit = 50
	maxTraceLogsLimit     = 200
	traceLogsRef          = "A"
	traceStepIDChars      = 8
)

type traceLogsInput struct {
	Datasource string `json:"datasource"`
	TraceID    string `json:"trace_id"`
	SpanID     string `json:"span_id"`
	Service    string `json:"service"`
	From       string `json:"from"`
	To         string `json:"to"`
	Limit      *int   `json:"limit"`
}

func init() { registerEntries((*Registry).traceLogEntries) }

func (r *Registry) traceLogEntries() []entry {
	return []entry{
		{Tool{Name: nameLogsForTrace, Description: descLogsForTrace, InputSchema: json.RawMessage(schemaLogsForTrace), ReadOnly: true, MinRole: auth.RoleEditor}, r.logsForTrace},
	}
}

func (r *Registry) logsForTrace(ctx context.Context, input json.RawMessage) (Result, error) {
	var in traceLogsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := in.check(); !ok {
		return res, nil
	}
	limit, res, ok := boundedInt(in.Limit, defaultTraceLogsLimit, 1, maxTraceLogsLimit, "limit")
	if !ok {
		return res, nil
	}
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	source, supported := inst.Datasource.(datasource.TraceLogSource)
	if !supported {
		return invalid("%s is a %s datasource; logs_for_trace reads Loki, ClickHouse, Elasticsearch and OpenSearch datasources", inst.UID, typeOf(inst)), nil
	}
	rng, res, ok := resolveRange(in.From, in.To, time.Now())
	if !ok {
		return res, nil
	}
	step := "Read log lines of trace " + in.TraceID[:traceStepIDChars] + " on " + inst.UID
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	release, err := r.deps.Runner.AcquireUpstream(ctx)
	if err != nil {
		return datasourceFailure(step, err)
	}
	defer release()
	principal, _ := auth.FromContext(ctx)
	frames, err := source.TraceLogs(ctx, datasource.TraceLogsRequest{
		TraceID: in.TraceID,
		SpanID:  in.SpanID,
		Service: in.Service,
		Range:   rng,
		Limit:   limit,
		User:    queryproxy.UserFor(principal),
	})
	if err != nil {
		return datasourceFailure(step, err)
	}
	summary := summarize(queryproxy.Result{
		Queries: []queryproxy.QueryResult{{Ref: traceLogsRef, Kind: queryproxy.KindInstant, Frames: frames}},
		Kind:    queryproxy.KindInstant,
		Range:   rng,
	})
	return success(fmt.Sprintf("Read %s of trace %s on %s", plural(lineCount(summary), "log line", "log lines"), in.TraceID[:traceStepIDChars], inst.UID), summary), nil
}

func (in traceLogsInput) check() (Result, bool) {
	switch {
	case !datasource.ValidTraceID(in.TraceID):
		return invalid("trace_id must be 16 or 32 hex characters"), false
	case in.SpanID != "" && !datasource.ValidSpanID(in.SpanID):
		return invalid("span_id must be 16 hex characters"), false
	}
	return checkLength("service", in.Service, datasource.MaxTraceLogsServiceBytes)
}

func lineCount(s querySummary) int {
	if s.Table == nil {
		return 0
	}
	return s.Table.TotalRows
}
