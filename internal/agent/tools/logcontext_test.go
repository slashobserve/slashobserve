package tools

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const anchorNs = "1793983689914935344"

var anchorTime = time.Unix(0, 1793983689914935344).UTC()

type fakeLineDS struct {
	*fakeDS
	linesFn func(req datasource.LinesRequest) ([]datasource.Frame, error)

	linesMu sync.Mutex
	lines   []datasource.LinesRequest
}

func (f *fakeLineDS) Lines(_ context.Context, req datasource.LinesRequest) ([]datasource.Frame, error) {
	f.linesMu.Lock()
	f.lines = append(f.lines, req)
	f.linesMu.Unlock()
	if f.linesFn == nil {
		return nil, nil
	}
	return f.linesFn(req)
}

func (f *fakeLineDS) side(direction datasource.LineDirection) (datasource.LinesRequest, bool) {
	f.linesMu.Lock()
	defer f.linesMu.Unlock()
	for _, req := range f.lines {
		if req.Direction == direction {
			return req, true
		}
	}
	return datasource.LinesRequest{}, false
}

func newLineRegistry(t *testing.T, ds *fakeLineDS) *Registry {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: ds},
		datasource.Instance{UID: "prom", Name: "Prometheus", Datasource: &fakeDS{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger})
}

func lokiLines(truncated bool, offsets ...int64) datasource.Frame {
	f := datasource.Frame{Labels: map[string]string{"service_name": "checkout"}, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime},
		{Name: "Body", Type: datasource.FieldString},
		{Name: "TimeNs", Type: datasource.FieldString},
		{Name: "level", Type: datasource.FieldString},
	}}
	for _, o := range offsets {
		at := anchorTime.Add(time.Duration(o))
		f.Fields[0].Values = append(f.Fields[0].Values, at)
		f.Fields[1].Values = append(f.Fields[1].Values, "line "+strconv.FormatInt(o, 10))
		f.Fields[2].Values = append(f.Fields[2].Values, strconv.FormatInt(at.UnixNano(), 10))
		f.Fields[3].Values = append(f.Fields[3].Values, "error")
	}
	if truncated {
		f.Meta = &datasource.FrameMeta{Truncated: true}
	}
	return f
}

func contextInput(extra string) string {
	return `{"datasource":"` + fakeUID + `","query":"{service_name=\"checkout\"}","anchor":"` + anchorNs + `"` + extra + `}`
}

func TestLogContextReadsBothSidesOrderedAwayFromTheAnchor(t *testing.T) {
	ds := &fakeLineDS{fakeDS: &fakeDS{typ: "loki"}}
	ds.linesFn = func(req datasource.LinesRequest) ([]datasource.Frame, error) {
		if req.Direction == datasource.Backward {
			return []datasource.Frame{lokiLines(false, -2, -1), lokiLines(false, -3)}, nil
		}
		return []datasource.Frame{lokiLines(true, 1, 3), lokiLines(false, 2)}, nil
	}
	r := newLineRegistry(t, ds)
	res, err := call(t, r, nameLogContext, contextInput(`,"before":3,"after":3`))
	if err != nil || res.IsError {
		t.Fatalf("res = %#v, err = %v", res, err)
	}
	if res.Step != "Read 6 lines around "+anchorNs+" on "+fakeUID {
		t.Fatalf("step = %q", res.Step)
	}
	out := res.Content.(logContextOut)
	if out.Anchor != anchorNs || out.BeforeTruncated || !out.AfterTruncated {
		t.Fatalf("out = %+v", out)
	}
	wantBefore := []string{"line -1", "line -2", "line -3"}
	wantAfter := []string{"line 1", "line 2", "line 3"}
	for i, line := range out.Before {
		if line.Body != wantBefore[i] {
			t.Fatalf("before bodies = %+v, want newest first %v", out.Before, wantBefore)
		}
	}
	for i, line := range out.After {
		if line.Body != wantAfter[i] {
			t.Fatalf("after bodies = %+v, want oldest first %v", out.After, wantAfter)
		}
	}
	first := out.After[0]
	if first.TimeNs != strconv.FormatInt(anchorTime.Add(1).UnixNano(), 10) || first.Time != anchorTime.Add(1).Format(time.RFC3339Nano) {
		t.Fatalf("after[0] = %+v, want the nanosecond time and its RFC 3339 form", first)
	}
	if first.Labels["service_name"] != "checkout" || first.Labels["level"] != "error" {
		t.Fatalf("labels = %v, want the stream labels and the level column", first.Labels)
	}
	before, _ := ds.side(datasource.Backward)
	after, _ := ds.side(datasource.Forward)
	if !before.Range.To.Equal(anchorTime) || !before.Range.From.Equal(anchorTime.Add(-contextWindow)) || before.Limit != 3 {
		t.Fatalf("before = %+v", before)
	}
	if !after.Range.From.Equal(anchorTime.Add(time.Nanosecond)) || !after.Range.To.Equal(anchorTime.Add(contextWindow)) || after.Limit != 3 {
		t.Fatalf("after = %+v", after)
	}
	if before.Query.Text != `{service_name="checkout"}` || before.Query.Limits != queryproxy.DraftLimits {
		t.Fatalf("query = %+v, want the text under DraftLimits", before.Query)
	}
}

func TestLogContextDefaultsAndSkipsAnEmptySide(t *testing.T) {
	ds := &fakeLineDS{fakeDS: &fakeDS{typ: "clickhouse"}}
	r := newLineRegistry(t, ds)
	res, err := call(t, r, nameLogContext, contextInput(""))
	if err != nil || res.IsError {
		t.Fatalf("res = %#v, err = %v", res, err)
	}
	out := res.Content.(logContextOut)
	if len(out.Before) != 0 || len(out.After) != 0 || out.BeforeTruncated || out.AfterTruncated || out.Before == nil || out.After == nil {
		t.Fatalf("out = %+v, want two empty lists", out)
	}
	before, _ := ds.side(datasource.Backward)
	after, _ := ds.side(datasource.Forward)
	if before.Limit != defaultContextLines || after.Limit != defaultContextLines {
		t.Fatalf("limits = %d/%d, want %d", before.Limit, after.Limit, defaultContextLines)
	}

	ds = &fakeLineDS{fakeDS: &fakeDS{typ: "loki"}}
	r = newLineRegistry(t, ds)
	if res, err := call(t, r, nameLogContext, contextInput(`,"after":0`)); err != nil || res.IsError {
		t.Fatalf("res = %#v, err = %v", res, err)
	}
	if _, read := ds.side(datasource.Forward); read || len(ds.lines) != 1 {
		t.Fatalf("lines = %+v, want only the before side read", ds.lines)
	}
}

func TestLogContextFlattensClickHouseColumnsIntoLabels(t *testing.T) {
	at := anchorTime.Add(-time.Second)
	frame := datasource.Frame{Fields: []datasource.Field{
		{Name: "Timestamp", Type: datasource.FieldTime, Values: []any{at}},
		{Name: "SeverityText", Type: datasource.FieldString, Values: []any{"ERROR"}},
		{Name: "Body", Type: datasource.FieldString, Values: []any{strings.Repeat("b", maxSummaryCell+10)}},
		{Name: "TraceId", Type: datasource.FieldString, Values: []any{nil}},
		{Name: "LogAttributes", Type: datasource.FieldOther, Values: []any{map[string]any{"k": "v"}}},
	}}
	ds := &fakeLineDS{fakeDS: &fakeDS{typ: "clickhouse"}}
	ds.linesFn = func(datasource.LinesRequest) ([]datasource.Frame, error) { return []datasource.Frame{frame}, nil }
	r := newLineRegistry(t, ds)
	res, err := call(t, r, nameLogContext, contextInput(`,"after":0`))
	if err != nil || res.IsError {
		t.Fatalf("res = %#v, err = %v", res, err)
	}
	out := res.Content.(logContextOut)
	if len(out.Before) != 1 {
		t.Fatalf("before = %+v, want one line", out.Before)
	}
	line := out.Before[0]
	if line.TimeNs != strconv.FormatInt(at.UnixNano(), 10) || len(line.Body) >= maxSummaryCell+10 || !strings.HasPrefix(line.Body, "bbbb") {
		t.Fatalf("line = %+v, want the time field's nanoseconds and a clipped body", line)
	}
	if line.Labels["SeverityText"] != "ERROR" || line.Labels["LogAttributes"] != "map[k:v]" {
		t.Fatalf("labels = %v", line.Labels)
	}
	if _, present := line.Labels["TraceId"]; present {
		t.Fatalf("labels = %v, want no entry for a null column", line.Labels)
	}
}

func TestLogContextCapsTheOutputByBytes(t *testing.T) {
	ds := &fakeLineDS{fakeDS: &fakeDS{typ: "loki"}}
	ds.linesFn = func(req datasource.LinesRequest) ([]datasource.Frame, error) {
		offsets := make([]int64, 100)
		for i := range offsets {
			offsets[i] = int64(i + 1)
		}
		return []datasource.Frame{lokiLines(false, offsets...)}, nil
	}
	r := newLineRegistry(t, ds)
	res, err := call(t, r, nameLogContext, contextInput(`,"before":0,"after":100`))
	if err != nil || res.IsError {
		t.Fatalf("res = %#v, err = %v", res, err)
	}
	out := res.Content.(logContextOut)
	if !out.AfterTruncated || len(out.After) == 0 || len(out.After) >= 100 {
		t.Fatalf("after = %d lines, truncated %v; want a byte-capped list", len(out.After), out.AfterTruncated)
	}
}

func TestLogContextReportsDatasourceErrors(t *testing.T) {
	ds := &fakeLineDS{fakeDS: &fakeDS{typ: "loki"}}
	ds.linesFn = func(req datasource.LinesRequest) ([]datasource.Frame, error) {
		if req.Direction == datasource.Forward {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "parse error")
		}
		return []datasource.Frame{lokiLines(false, -1)}, nil
	}
	r := newLineRegistry(t, ds)
	res, err := call(t, r, nameLogContext, contextInput(""))
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != string(queryproxy.CodeQueryInvalid) {
		t.Fatalf("code = %q", got)
	}
	if !strings.HasPrefix(res.Step, "Read lines around "+anchorNs+" on "+fakeUID+": ") {
		t.Fatalf("step = %q", res.Step)
	}
}

func TestLogContextRejectsInvalidInput(t *testing.T) {
	ds := &fakeLineDS{fakeDS: &fakeDS{typ: "loki"}}
	r := newLineRegistry(t, ds)
	tests := []struct {
		name  string
		input string
		code  string
	}{
		{"unknown datasource", `{"datasource":"nope","query":"{a=\"b\"}","anchor":"1"}`, string(queryproxy.CodeUnknownDatasource)},
		{"missing query", `{"datasource":"` + fakeUID + `","anchor":"1"}`, codeInvalidRequest},
		{"anchor not a number", contextInput(`,"anchor":"2026-11-06T12:00:00Z"`), codeInvalidRequest},
		{"before over 100", contextInput(`,"before":101`), codeInvalidRequest},
		{"after negative", contextInput(`,"after":-1`), codeInvalidRequest},
		{"unknown field", contextInput(`,"window_ms":1000`), codeInvalidRequest},
		{"not a line source", `{"datasource":"prom","query":"up","anchor":"1"}`, codeInvalidRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := call(t, r, nameLogContext, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := errorCode(t, res); got != tt.code {
				t.Fatalf("code = %q, want %q (%+v)", got, tt.code, res.Content)
			}
		})
	}
	if len(ds.lines) != 0 {
		t.Fatalf("%d Lines calls for refused input", len(ds.lines))
	}
	res, _ := call(t, r, nameLogContext, `{"datasource":"prom","query":"up","anchor":"1"}`)
	if msg := res.Content.(errorBody).Message; msg != notLineSource {
		t.Fatalf("message = %q", msg)
	}
}
