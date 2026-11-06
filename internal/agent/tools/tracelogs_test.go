package tools

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	logsUID      = "logs"
	traceID      = "4bf92f3577b34da6a3ce929d0e0e4736"
	spanID       = "00f067aa0ba902b7"
	traceLogsRow = `{"datasource":"logs","trace_id":"` + traceID + `"}`
)

type traceLogFake struct {
	*fakeDS
	answer  func(datasource.TraceLogsRequest) ([]datasource.Frame, error)
	request datasource.TraceLogsRequest
}

func (f *traceLogFake) TraceLogs(_ context.Context, req datasource.TraceLogsRequest) ([]datasource.Frame, error) {
	f.mu.Lock()
	f.request = req
	f.mu.Unlock()
	if f.answer != nil {
		return f.answer(req)
	}
	return []datasource.Frame{logTableFrame()}, nil
}

func traceLogRegistry(t *testing.T, logs *traceLogFake) *Registry {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: &fakeDS{}},
		datasource.Instance{UID: logsUID, Name: "Logs", Datasource: logs})
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger})
}

func TestLogsForTraceReadsTheLinesOfATraceOrSpan(t *testing.T) {
	logs := &traceLogFake{fakeDS: &fakeDS{typ: "loki"}}
	r := traceLogRegistry(t, logs)
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "u1", Name: "Alice", Email: "alice@example.com", Role: auth.RoleEditor})
	before := time.Now()
	res, err := r.Call(ctx, nameLogsForTrace, []byte(`{"datasource":"logs","trace_id":"`+traceID+`","span_id":"`+spanID+`","service":"checkout","from":"now-30m","to":"now","limit":120}`))
	if err != nil || res.IsError {
		t.Fatalf("call: %v %#v", err, res.Content)
	}
	if want := "Read 2 log lines of trace 4bf92f35 on logs"; res.Step != want {
		t.Errorf("step = %q, want %q", res.Step, want)
	}
	summary, ok := res.Content.(querySummary)
	if !ok {
		t.Fatalf("content is %T", res.Content)
	}
	if summary.Kind != queryproxy.KindInstant || summary.Table == nil || summary.Table.TotalRows != 2 || summary.Empty {
		t.Errorf("summary = %+v", summary)
	}
	if got := summary.Table.Columns; len(got) != 3 || got[1].Name != "ServiceName" {
		t.Errorf("columns = %+v", got)
	}
	req := logs.request
	if req.TraceID != traceID || req.SpanID != spanID || req.Service != "checkout" || req.Limit != 120 {
		t.Errorf("request = %+v", req)
	}
	if req.User.Login != "alice@example.com" || req.User.ID != "u1" {
		t.Errorf("user = %+v", req.User)
	}
	if span := req.Range.To.Sub(req.Range.From); span < 29*time.Minute || span > 31*time.Minute || req.Range.To.Before(before) {
		t.Errorf("range = %v..%v", req.Range.From, req.Range.To)
	}
}

func TestLogsForTraceDefaultsTheWindowAndTheLimit(t *testing.T) {
	logs := &traceLogFake{fakeDS: &fakeDS{typ: "clickhouse"}}
	r := traceLogRegistry(t, logs)
	res, err := call(t, r, nameLogsForTrace, traceLogsRow)
	if err != nil || res.IsError {
		t.Fatalf("call: %v %#v", err, res.Content)
	}
	req := logs.request
	if req.Limit != defaultTraceLogsLimit || req.SpanID != "" || req.Service != "" {
		t.Errorf("request = %+v", req)
	}
	if span := req.Range.To.Sub(req.Range.From); span < 59*time.Minute || span > 61*time.Minute {
		t.Errorf("range = %v..%v", req.Range.From, req.Range.To)
	}
}

func TestLogsForTraceAnswersAnEmptyTrace(t *testing.T) {
	logs := &traceLogFake{fakeDS: &fakeDS{typ: "loki"}, answer: func(datasource.TraceLogsRequest) ([]datasource.Frame, error) {
		return []datasource.Frame{}, nil
	}}
	r := traceLogRegistry(t, logs)
	res, err := call(t, r, nameLogsForTrace, traceLogsRow)
	if err != nil || res.IsError {
		t.Fatalf("call: %v %#v", err, res.Content)
	}
	if want := "Read 0 log lines of trace 4bf92f35 on logs"; res.Step != want {
		t.Errorf("step = %q, want %q", res.Step, want)
	}
	if summary := res.Content.(querySummary); !summary.Empty || summary.Table != nil {
		t.Errorf("summary = %+v", summary)
	}
}

func TestLogsForTraceRefusesBadInput(t *testing.T) {
	logs := &traceLogFake{fakeDS: &fakeDS{typ: "loki"}}
	r := traceLogRegistry(t, logs)
	cases := map[string]struct{ input, code, message string }{
		"short trace id":     {`{"datasource":"logs","trace_id":"abc"}`, codeInvalidRequest, "trace_id must be 16 or 32 hex characters"},
		"missing trace id":   {`{"datasource":"logs"}`, codeInvalidRequest, "trace_id must be 16 or 32 hex characters"},
		"bad span id":        {`{"datasource":"logs","trace_id":"` + traceID + `","span_id":"xyz"}`, codeInvalidRequest, "span_id must be 16 hex characters"},
		"long service":       {`{"datasource":"logs","trace_id":"` + traceID + `","service":"` + strings.Repeat("s", 201) + `"}`, codeInvalidRequest, "service must be at most 200 bytes"},
		"limit over the cap": {`{"datasource":"logs","trace_id":"` + traceID + `","limit":201}`, codeInvalidRequest, "limit must be between 1 and 200"},
		"from after to":      {`{"datasource":"logs","trace_id":"` + traceID + `","from":"now","to":"now-1h"}`, codeInvalidRequest, "from must be before to"},
		"unknown field":      {`{"datasource":"logs","trace_id":"` + traceID + `","query":"x"}`, codeInvalidRequest, `unknown field "query"; logs_for_trace takes datasource, from, limit, service, span_id, to and trace_id`},
		"metrics datasource": {`{"datasource":"fake","trace_id":"` + traceID + `"}`, codeInvalidRequest, "fake is a prometheus datasource; logs_for_trace reads Loki, ClickHouse, Elasticsearch and OpenSearch datasources"},
		"unknown datasource": {`{"datasource":"nope","trace_id":"` + traceID + `"}`, string(queryproxy.CodeUnknownDatasource), `unknown datasource "nope"; call list_datasources`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := call(t, r, nameLogsForTrace, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := errorCode(t, res); got != tc.code {
				t.Errorf("code = %q, want %q", got, tc.code)
			}
			if got := res.Content.(errorBody).Message; got != tc.message {
				t.Errorf("message = %q, want %q", got, tc.message)
			}
		})
	}
	if logs.request.TraceID != "" {
		t.Errorf("a refused call reached the datasource: %+v", logs.request)
	}
}

func TestLogsForTraceReportsDatasourceErrors(t *testing.T) {
	var answer error
	logs := &traceLogFake{fakeDS: &fakeDS{typ: "loki"}, answer: func(datasource.TraceLogsRequest) ([]datasource.Frame, error) {
		return nil, answer
	}}
	r := traceLogRegistry(t, logs)

	answer = datasource.Errorf(datasource.ErrInvalidQuery, "parse error")
	res, err := call(t, r, nameLogsForTrace, traceLogsRow)
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != string(queryproxy.CodeQueryInvalid) {
		t.Errorf("code = %q", got)
	}
	if want := "Read log lines of trace 4bf92f35 on logs: query_invalid"; res.Step != want {
		t.Errorf("step = %q, want %q", res.Step, want)
	}

	answer = errors.New("boom at 10.0.0.5")
	res, err = call(t, r, nameLogsForTrace, traceLogsRow)
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != FailureToolFailed {
		t.Errorf("code = %q", got)
	}
	if msg := res.Content.(errorBody).Message; strings.Contains(msg, "10.0.0.5") {
		t.Errorf("message leaks the error: %q", msg)
	}
}

func TestLogsForTraceIsOfferedToMCPAndEveryRunMode(t *testing.T) {
	tools := New(Deps{}).List()
	i := slices.IndexFunc(tools, func(tool Tool) bool { return tool.Name == nameLogsForTrace })
	if i < 0 {
		t.Fatal("logs_for_trace is not listed")
	}
	tool := tools[i]
	if !tool.ReadOnly || tool.MinRole != auth.RoleEditor || tool.MCPOnly || tool.AgentOnly {
		t.Errorf("tool = %+v", tool)
	}
	for _, mode := range []Mode{ModeCreate, ModeInvestigate} {
		if !tool.OfferedIn(mode) {
			t.Errorf("not offered in %s runs", mode)
		}
	}
}
