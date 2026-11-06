package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

type fakePatternDS struct {
	*fakeLineDS
	patternsFn func(req datasource.PatternRequest) ([]datasource.NativePattern, error)

	patternsMu sync.Mutex
	patterns   []datasource.PatternRequest
}

func (f *fakePatternDS) LogPatterns(_ context.Context, req datasource.PatternRequest) ([]datasource.NativePattern, error) {
	f.patternsMu.Lock()
	f.patterns = append(f.patterns, req)
	f.patternsMu.Unlock()
	if f.patternsFn == nil {
		return nil, nil
	}
	return f.patternsFn(req)
}

func (f *fakePatternDS) patternCalls() int {
	f.patternsMu.Lock()
	defer f.patternsMu.Unlock()
	return len(f.patterns)
}

func newPatternRegistry(t *testing.T, ds datasource.Datasource) *Registry {
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

func patternLines(texts ...string) []datasource.Frame {
	f := datasource.Frame{Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime},
		{Name: "Body", Type: datasource.FieldString},
	}}
	for i, text := range texts {
		f.Fields[0].Values = append(f.Fields[0].Values, anchorTime.Add(-time.Duration(i)*time.Second))
		f.Fields[1].Values = append(f.Fields[1].Values, text)
	}
	return []datasource.Frame{f}
}

func TestLogPatternsReadsLokisIndexWhenItAnswers(t *testing.T) {
	ds := &fakePatternDS{fakeLineDS: &fakeLineDS{fakeDS: &fakeDS{typ: "loki"}}}
	ds.patternsFn = func(datasource.PatternRequest) ([]datasource.NativePattern, error) {
		var out []datasource.NativePattern
		for i := range 12 {
			out = append(out, datasource.NativePattern{Pattern: "pattern " + strconv.Itoa(i) + " <*>", Samples: []datasource.PatternSample{{At: anchorTime, Count: int64(100 - i)}}})
		}
		return out, nil
	}
	r := newPatternRegistry(t, ds)
	res, err := call(t, r, nameLogPatterns, `{"datasource":"fake","query":"{app=\"shop\"}","from":"now-6h","to":"now"}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("res = %#v", res.Content)
	}
	if want := "Found 10 patterns on fake (from Loki)"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
	out, ok := res.Content.(toolPatterns)
	if !ok {
		t.Fatalf("content is %T", res.Content)
	}
	if out.Source != "loki" || len(out.Patterns) != 10 || !out.Truncated || out.LinesMatched != 1134 || out.Patterns[0].Pattern != "pattern 0 <*>" || out.Patterns[0].Count != 100 {
		t.Fatalf("patterns = %+v", out)
	}
	if out.TrendStepMs != 12*60*1000 {
		t.Fatalf("trend_step_ms = %d, want 6h over 30 buckets", out.TrendStepMs)
	}
	for _, p := range out.Patterns {
		if len(p.Trend) != 30 {
			t.Fatalf("pattern %q has %d trend buckets, want 30", p.Pattern, len(p.Trend))
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"start"`) {
		t.Fatalf("tool output carries bucket starts: %s", encoded)
	}
	if len(encoded) > 3<<10 {
		t.Fatalf("a 10 pattern answer is %d bytes, want it compact", len(encoded))
	}
	if ds.patternCalls() != 1 {
		t.Fatalf("%d pattern calls, want 1", ds.patternCalls())
	}
	if _, read := ds.side(datasource.Backward); read {
		t.Fatal("lines were read beside a native answer")
	}
	req := ds.patterns[0]
	if req.Query.Text != `{app="shop"}` || req.Query.Limits != queryproxy.DraftLimits || req.Step != 6*time.Minute {
		t.Fatalf("request = %+v", req)
	}
	if got := req.Range.To.Sub(req.Range.From); got.Hours() != 6 {
		t.Fatalf("range = %v, want 6h", got)
	}
}

func TestLogPatternsSamplesLinesWhenTheDatasourceHasNoIndexAndClipsSamples(t *testing.T) {
	long := strings.Repeat("y", 300)
	ds := &fakeLineDS{fakeDS: &fakeDS{typ: "clickhouse"}}
	ds.linesFn = func(datasource.LinesRequest) ([]datasource.Frame, error) {
		return patternLines("payment failed order=1 "+long, "payment failed order=2 "+long, "cart emptied"), nil
	}
	r := newPatternRegistry(t, ds)
	res, err := call(t, r, nameLogPatterns, `{"datasource":"fake","query":"SELECT Timestamp, Body FROM otel_logs","limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("res = %#v", res.Content)
	}
	if want := "Found 1 pattern on fake (from 3 sampled lines)"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
	out := res.Content.(toolPatterns)
	if out.Source != "server" || out.LinesSampled != 3 || !out.Truncated || len(out.Patterns) != 1 || out.Patterns[0].Count != 2 {
		t.Fatalf("patterns = %+v", out)
	}
	if got := out.Patterns[0].Sample; len(got) != maxSummaryCell+len("...") || !strings.HasSuffix(got, "...") || !strings.HasPrefix(got, "payment failed order=1 ") {
		t.Fatalf("sample = %q, want the newest line cut to %d bytes", got, maxSummaryCell)
	}
	read, ok := ds.side(datasource.Backward)
	if !ok || read.Limit != defaultPatternSample || read.Query.Limits.MaxRows != defaultPatternSample || read.Query.Text != "SELECT Timestamp, Body FROM otel_logs" {
		t.Fatalf("line read = %+v", read)
	}
	if got := read.Range.To.Sub(read.Range.From); got != time.Hour {
		t.Fatalf("default range = %v, want 1h", got)
	}
}

func TestLogPatternsFitsTheAnswerInTheBudget(t *testing.T) {
	ds := &fakePatternDS{fakeLineDS: &fakeLineDS{fakeDS: &fakeDS{typ: "loki"}}}
	ds.patternsFn = func(datasource.PatternRequest) ([]datasource.NativePattern, error) {
		var out []datasource.NativePattern
		for i := range 30 {
			out = append(out, datasource.NativePattern{Pattern: strings.Repeat("w", 1000) + strconv.Itoa(i), Samples: []datasource.PatternSample{{At: anchorTime, Count: 1}}})
		}
		return out, nil
	}
	r := newPatternRegistry(t, ds)
	res, err := call(t, r, nameLogPatterns, `{"datasource":"fake","query":"{app=\"shop\"}","limit":30}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("res = %#v", res.Content)
	}
	out := res.Content.(toolPatterns)
	if len(out.Patterns) >= 30 || len(out.Patterns) == 0 || !out.Truncated {
		t.Fatalf("%d patterns, truncated %v, want fewer than 30 and truncated", len(out.Patterns), out.Truncated)
	}
}

func TestLogPatternsRefusesDatasourcesWithoutLines(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, nameLogPatterns, `{"datasource":"fake","query":"up"}`)
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != codeInvalidRequest {
		t.Fatalf("res = %#v", res.Content)
	}
	if want := "log_patterns works on Loki and ClickHouse datasources; fake is a prometheus datasource"; res.Content.(errorBody).Message != want {
		t.Fatalf("message = %q, want %q", res.Content.(errorBody).Message, want)
	}
}

func TestLogPatternsRejectsInvalidInput(t *testing.T) {
	ds := &fakePatternDS{fakeLineDS: &fakeLineDS{fakeDS: &fakeDS{typ: "loki"}}}
	r := newPatternRegistry(t, ds)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"missing query", `{"datasource":"fake"}`, "query is required"},
		{"limit zero", `{"datasource":"fake","query":"{a=\"b\"}","limit":0}`, "limit must be between 1 and 30"},
		{"limit over 30", `{"datasource":"fake","query":"{a=\"b\"}","limit":31}`, "limit must be between 1 and 30"},
		{"unknown field", `{"datasource":"fake","query":"{a=\"b\"}","sample":100}`, `unknown field "sample"`},
		{"from after to", `{"datasource":"fake","query":"{a=\"b\"}","from":"now","to":"now-1h"}`, "from must be before to"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := call(t, r, nameLogPatterns, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("code = %q", errorCode(t, res))
			}
			if got := res.Content.(errorBody).Message; !strings.HasPrefix(got, tt.want) {
				t.Fatalf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
	if ds.patternCalls() != 0 {
		t.Fatalf("the datasource was called for rejected input")
	}
}

func TestLogPatternsReportsDatasourceErrorsByCode(t *testing.T) {
	ds := &fakePatternDS{fakeLineDS: &fakeLineDS{fakeDS: &fakeDS{typ: "loki"}}}
	ds.patternsFn = func(datasource.PatternRequest) ([]datasource.NativePattern, error) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "patterns need a log query that starts with a stream selector")
	}
	r := newPatternRegistry(t, ds)
	res, err := call(t, r, nameLogPatterns, `{"datasource":"fake","query":"rate({a=\"b\"}[1m])"}`)
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != string(queryproxy.CodeQueryInvalid) {
		t.Fatalf("res = %#v", res.Content)
	}
	if want := "Finding patterns on fake: query_invalid"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}
