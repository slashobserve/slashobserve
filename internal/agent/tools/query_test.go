package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestSummarizeFrameShapes(t *testing.T) {
	tests := []struct {
		name        string
		frames      []datasource.Frame
		kind        queryproxy.Kind
		wantEmpty   bool
		wantSeries  int
		wantTable   bool
		wantColumns []string
		wantRows    int
	}{
		{
			name: "prometheus range",
			kind: queryproxy.KindRange,
			frames: []datasource.Frame{
				timeNumberFrame(map[string]string{"route": "/checkout"}, 1, 2, 3),
				timeNumberFrame(map[string]string{"route": "/cart"}, 4, 5, 6),
			},
			wantSeries: 2,
		},
		{
			name:       "prometheus instant",
			kind:       queryproxy.KindInstant,
			frames:     []datasource.Frame{timeNumberFrame(map[string]string{"pod": "checkout-0"}, 7)},
			wantSeries: 1,
		},
		{
			name: "prometheus instant string value",
			kind: queryproxy.KindInstant,
			frames: []datasource.Frame{{Fields: []datasource.Field{
				{Name: "Time", Type: datasource.FieldTime, Values: []any{time.Unix(0, 0)}},
				{Name: "Value", Type: datasource.FieldString, Values: []any{"up"}},
			}}},
			wantTable:   true,
			wantColumns: []string{"Time", "Value"},
			wantRows:    1,
		},
		{
			name: "clickhouse range with labels and two values",
			kind: queryproxy.KindRange,
			frames: []datasource.Frame{{
				Labels: map[string]string{"ServiceName": "checkout"},
				Fields: []datasource.Field{
					{Name: "Time", Type: datasource.FieldTime, Values: []any{time.Unix(0, 0), time.Unix(60, 0)}},
					{Name: "requests", Type: datasource.FieldNumber, Values: []any{1.0, 2.0}},
					{Name: "errors", Type: datasource.FieldNumber, Values: []any{0.0, nil}},
				},
			}},
			wantSeries: 2,
		},
		{
			name: "clickhouse instant time and number",
			kind: queryproxy.KindInstant,
			frames: []datasource.Frame{{Fields: []datasource.Field{
				{Name: "Timestamp", Type: datasource.FieldTime, Values: []any{time.Unix(0, 0)}},
				{Name: "count", Type: datasource.FieldNumber, Values: []any{9.0}},
			}}},
			wantSeries: 1,
		},
		{
			name:        "clickhouse instant log table",
			kind:        queryproxy.KindInstant,
			frames:      []datasource.Frame{logTableFrame()},
			wantTable:   true,
			wantColumns: []string{"Timestamp", "ServiceName", "Body"},
			wantRows:    2,
		},
		{
			name: "clickhouse instant table without time",
			kind: queryproxy.KindInstant,
			frames: []datasource.Frame{{Fields: []datasource.Field{
				{Name: "service", Type: datasource.FieldString, Values: []any{"checkout"}},
				{Name: "errors", Type: datasource.FieldNumber, Values: []any{3.0}},
			}}},
			wantTable:   true,
			wantColumns: []string{"service", "errors"},
			wantRows:    1,
		},
		{
			name: "time then bool is a table",
			kind: queryproxy.KindInstant,
			frames: []datasource.Frame{{Fields: []datasource.Field{
				{Name: "t", Type: datasource.FieldTime, Values: []any{time.Unix(0, 0)}},
				{Name: "ok", Type: datasource.FieldBool, Values: []any{true}},
			}}},
			wantTable:   true,
			wantColumns: []string{"t", "ok"},
			wantRows:    1,
		},
		{
			name:      "no frames",
			kind:      queryproxy.KindRange,
			wantEmpty: true,
		},
		{
			name: "all values nil",
			kind: queryproxy.KindRange,
			frames: []datasource.Frame{{Fields: []datasource.Field{
				{Name: "Time", Type: datasource.FieldTime, Values: []any{time.Unix(0, 0)}},
				{Name: "Value", Type: datasource.FieldNumber, Values: []any{nil}},
			}}},
			wantEmpty: true,
		},
		{
			name: "clickhouse range without rows",
			kind: queryproxy.KindRange,
			frames: []datasource.Frame{{Fields: []datasource.Field{
				{Name: "Time", Type: datasource.FieldTime, Values: []any{}},
				{Name: "count", Type: datasource.FieldNumber, Values: []any{}},
			}}},
			wantEmpty: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarize(queryproxy.Result{Queries: []queryproxy.QueryResult{{Ref: "A", Frames: tt.frames}}, Kind: tt.kind, Step: 15 * time.Second})
			if got.Empty != tt.wantEmpty {
				t.Fatalf("empty = %v, want %v", got.Empty, tt.wantEmpty)
			}
			if got.Frames != len(tt.frames) || got.Kind != tt.kind {
				t.Fatalf("frames/kind = %d/%s", got.Frames, got.Kind)
			}
			if len(got.Series) != tt.wantSeries {
				t.Fatalf("series = %d, want %d: %#v", len(got.Series), tt.wantSeries, got.Series)
			}
			if (got.Table != nil) != tt.wantTable {
				t.Fatalf("table present = %v, want %v", got.Table != nil, tt.wantTable)
			}
			if got.Table == nil {
				return
			}
			var columns []string
			for _, c := range got.Table.Columns {
				columns = append(columns, c.Name)
			}
			if strings.Join(columns, ",") != strings.Join(tt.wantColumns, ",") {
				t.Fatalf("columns = %v, want %v", columns, tt.wantColumns)
			}
			if got.Table.TotalRows != tt.wantRows || len(got.Table.Rows) != tt.wantRows {
				t.Fatalf("rows = %d/%d, want %d", len(got.Table.Rows), got.Table.TotalRows, tt.wantRows)
			}
		})
	}
}

func TestSummarizeSeriesStats(t *testing.T) {
	frame := timeNumberFrame(map[string]string{"route": "/checkout"}, 3, 1, 2)
	frame.Fields[1].Values = append(frame.Fields[1].Values, nil)
	got := summarize(queryproxy.Result{Queries: []queryproxy.QueryResult{{Ref: "A", Frames: []datasource.Frame{frame}}}, Kind: queryproxy.KindRange, Step: 18 * time.Second})
	if len(got.Series) != 1 {
		t.Fatalf("series = %d", len(got.Series))
	}
	s := got.Series[0]
	if s.Points != 4 || s.NonNull != 3 || *s.Min != 1 || *s.Max != 3 || *s.Last != 2 {
		t.Fatalf("stats = %+v", s)
	}
	if s.Labels["route"] != "/checkout" || got.StepMS != 18000 {
		t.Fatalf("labels/step = %v/%d", s.Labels, got.StepMS)
	}
}

func rangeSummary(t *testing.T, ds datasource.Datasource, kind queryproxy.Kind, step time.Duration, frames ...datasource.Frame) querySummary {
	t.Helper()
	s := summarize(queryproxy.Result{Queries: []queryproxy.QueryResult{{Ref: "A", Kind: kind, Step: step, Frames: frames}}, Kind: kind})
	s.noteSampling(ds)
	return s
}

func TestNoteSamplingSaysRangeExtremesAreOfSampledPoints(t *testing.T) {
	series := timeNumberFrame(nil, 1, 3, 2)
	coarse := rangeSummary(t, &fakeDS{}, queryproxy.KindRange, 216*time.Second, series)
	for _, want := range []string{"sampled every 3m36s", "min, max and last", "between samples", "max_over_time", "min_over_time", "query_instant"} {
		if !strings.Contains(coarse.Note, want) {
			t.Errorf("note = %q, want it to contain %q", coarse.Note, want)
		}
	}
	if coarse.StepMS != 216000 || *coarse.Series[0].Max != 3 {
		t.Fatalf("existing fields changed: %+v", coarse)
	}
	for name, got := range map[string]querySummary{
		"step at the default scrape": rangeSummary(t, &fakeDS{}, queryproxy.KindRange, 15*time.Second, series),
		"instant":                    rangeSummary(t, &fakeDS{}, queryproxy.KindInstant, 0, series),
		"no series":                  rangeSummary(t, &fakeDS{}, queryproxy.KindRange, 216*time.Second, logRows(2)),
		"empty result":               rangeSummary(t, &fakeDS{}, queryproxy.KindRange, 216*time.Second, timeNumberFrame(nil)),
		"not a PromQL datasource":    rangeSummary(t, &fakeDS{typ: "clickhouse"}, queryproxy.KindRange, 216*time.Second, series),
	} {
		if got.Note != "" {
			t.Errorf("%s: note = %q, want none", name, got.Note)
		}
	}
}

func TestNoteSamplingComparesTheStepWithTheDatasourceScrapeInterval(t *testing.T) {
	series := timeNumberFrame(nil, 1, 3, 2)
	if got := rangeSummary(t, &fakeDS{minStep: time.Minute}, queryproxy.KindRange, time.Minute, series); got.Note != "" {
		t.Fatalf("step equal to the scrape interval: note = %q", got.Note)
	}
	if got := rangeSummary(t, &fakeDS{minStep: time.Minute}, queryproxy.KindRange, 90*time.Second, series); !strings.Contains(got.Note, "sampled every 1m30s") {
		t.Fatalf("step above the scrape interval: note = %q", got.Note)
	}
	if got := rangeSummary(t, &fakeDS{}, queryproxy.KindRange, 18*time.Second, series); !strings.Contains(got.Note, "sampled every 18s") {
		t.Fatalf("step above the default 15s: note = %q", got.Note)
	}
}

func TestSummarizeTakesTheStepFromTheQueriesThatRan(t *testing.T) {
	series := timeNumberFrame(nil, 1, 3, 2)
	got := summarize(queryproxy.Result{
		Queries: []queryproxy.QueryResult{{Ref: "A", Kind: queryproxy.KindRange, Step: time.Minute, Frames: []datasource.Frame{series}}},
		Kind:    queryproxy.KindRange,
		Step:    18 * time.Second,
	})
	if got.StepMS != 60000 {
		t.Fatalf("step_ms = %d, want the query's own 60000", got.StepMS)
	}
}

func TestQueryRangeReportsAndNamesTheStepTheQueryRanAt(t *testing.T) {
	frames := func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
	}
	run := func(ds *fakeDS) querySummary {
		res, err := call(t, newTestRegistry(t, ds, nil), nameQueryRange, `{"datasource":"fake","query":"up","from":"now-1h"}`)
		if err != nil || res.IsError {
			t.Fatalf("res = %#v err = %v", res, err)
		}
		return res.Content.(querySummary)
	}
	held := run(&fakeDS{rangeFn: frames, minStep: time.Minute})
	if held.StepMS != 60000 || held.Note != "" {
		t.Fatalf("a 1m minimum over 1h: step_ms = %d note = %q, want 60000 and no note", held.StepMS, held.Note)
	}
	coarse := run(&fakeDS{rangeFn: frames, minStep: 5 * time.Second})
	if coarse.StepMS != 18000 || !strings.Contains(coarse.Note, "sampled every 18s") {
		t.Fatalf("step_ms = %d note = %q", coarse.StepMS, coarse.Note)
	}
}

func TestJoinNotesDropsEmptyNotesAndTrailingPeriods(t *testing.T) {
	if got := joinNotes("a.", "", "b"); got != "a. b" {
		t.Fatalf("got %q", got)
	}
	if got := joinNotes("", ""); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestNoteSamplingKeepsTheLimitNoteBesideIt(t *testing.T) {
	frame := timeNumberFrame(nil, 1, 2)
	frame.Meta = &datasource.FrameMeta{Partial: true}
	got := rangeSummary(t, &fakeDS{}, queryproxy.KindRange, time.Minute, frame)
	if !strings.HasPrefix(got.Note, queryproxy.PartialNote+". min, max and last") {
		t.Fatalf("note = %q", got.Note)
	}
}

func TestSummarizeCapsSeries(t *testing.T) {
	var frames []datasource.Frame
	for range maxSeriesOut + 5 {
		frames = append(frames, timeNumberFrame(map[string]string{"pod": "p"}, 1))
	}
	got := summarize(queryproxy.Result{Queries: []queryproxy.QueryResult{{Ref: "A", Frames: frames}}, Kind: queryproxy.KindRange})
	if len(got.Series) != maxSeriesOut || !got.Truncated {
		t.Fatalf("series = %d truncated = %v", len(got.Series), got.Truncated)
	}
}

func TestQueryInstantLogTableThroughCall(t *testing.T) {
	ds := &fakeDS{
		typ: "clickhouse",
		instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return []datasource.Frame{logTableFrame()}, nil
		},
	}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameQueryInstant, `{"datasource":"fake","query":"SELECT Timestamp, ServiceName, Body FROM otel_logs"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	if res.Step != "Ran a query on fake: 2 rows" {
		t.Fatalf("step = %q", res.Step)
	}
	summary := res.Content.(querySummary)
	if summary.Empty || summary.Table == nil || len(summary.Table.Columns) != 3 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestQueryInstantSaysMoreRowsExistAndPartial(t *testing.T) {
	cases := []struct {
		name string
		meta datasource.FrameMeta
		want string
	}{
		{"truncated", datasource.FrameMeta{Truncated: true}, "Ran a query on fake: 3 rows (" + queryproxy.NarrowNote + ")"},
		{"partial", datasource.FrameMeta{Partial: true}, "Ran a query on fake: 3 rows (" + queryproxy.PartialNote + ")"},
		{"warned", datasource.FrameMeta{Partial: true, Warning: "store a is down"}, "Ran a query on fake: 3 rows (" + queryproxy.PartialNote + "; the datasource warned: store a is down)"},
		{"both", datasource.FrameMeta{Truncated: true, Partial: true}, "Ran a query on fake: 3 rows (" + queryproxy.NarrowNote + ") (" + queryproxy.PartialNote + ")"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds := &fakeDS{
				typ: "clickhouse",
				instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
					f := logTableFrame()
					for i := range f.Fields {
						f.Fields[i].Values = append(f.Fields[i].Values, f.Fields[i].Values[0])
					}
					f.Meta = &tc.meta
					return []datasource.Frame{f}, nil
				},
			}
			r := newTestRegistry(t, ds, nil)
			res, err := call(t, r, nameQueryInstant, `{"datasource":"fake","query":"SELECT Timestamp, ServiceName, Body FROM otel_logs"}`)
			if err != nil || res.IsError {
				t.Fatalf("res = %#v err = %v", res, err)
			}
			if res.Step != tc.want {
				t.Fatalf("step = %q, want %q", res.Step, tc.want)
			}
			summary := res.Content.(querySummary)
			if summary.MoreRows != tc.meta.Truncated || summary.Partial != tc.meta.Partial {
				t.Fatalf("summary = %+v, want more_rows %v and partial %v", summary, tc.meta.Truncated, tc.meta.Partial)
			}
		})
	}
}

func TestQueryInstantExplainsAnEmptyResultWithTheDatasourceHint(t *testing.T) {
	hint := "No document has log.level = error in this range, but it has ERROR."
	ds := &fakeDS{
		typ: "clickhouse",
		instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			f := logTableFrame()
			for i := range f.Fields {
				f.Fields[i].Values = nil
			}
			f.Meta = &datasource.FrameMeta{Hint: hint}
			return []datasource.Frame{f}, nil
		},
	}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameQueryInstant, `{"datasource":"fake","query":"SELECT Timestamp, ServiceName, Body FROM otel_logs"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	if want := "Ran a query on fake: no data (" + hint + ")"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
	if summary := res.Content.(querySummary); !summary.Empty || summary.Hint != hint {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestQueryRangeNamesTheHintBesideAnAllZeroSeries(t *testing.T) {
	hint := `Field "Body" is not in the mapping; text fields to try: message.`
	ds := &fakeDS{rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		f := timeNumberFrame(nil, 0, 0)
		f.Meta = &datasource.FrameMeta{Hint: hint}
		return []datasource.Frame{f}, nil
	}}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameQueryRange, `{"datasource":"fake","query":"Body:error"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	if want := "Ran a query on fake: 1 series (" + hint + ")"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}

func TestQueryRangeThroughCall(t *testing.T) {
	ds := &fakeDS{rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return []datasource.Frame{timeNumberFrame(map[string]string{"route": "/a"}, 1, 2)}, nil
	}}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameQueryRange, `{"datasource":"fake","query":"up","from":"now-6h","max_data_points":200.0}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	if res.Step != "Ran a query on fake: 1 series" {
		t.Fatalf("step = %q", res.Step)
	}
	if got := res.Content.(querySummary); got.Kind != queryproxy.KindRange || got.StepMS != 108000 {
		t.Fatalf("summary = %+v", got)
	}
}

func TestQueryBoundsThroughCall(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	tests := []struct{ name, input string }{
		{"data points too low", `{"datasource":"fake","query":"up","max_data_points":9}`},
		{"data points too high", `{"datasource":"fake","query":"up","max_data_points":1001}`},
		{"data points fractional", `{"datasource":"fake","query":"up","max_data_points":20.5}`},
		{"empty query", `{"datasource":"fake","query":""}`},
		{"huge query", `{"datasource":"fake","query":"` + strings.Repeat("a", maxQueryBytes+1) + `"}`},
		{"range over 31 days", `{"datasource":"fake","query":"up","from":"now-32d"}`},
		{"from after to", `{"datasource":"fake","query":"up","from":"now","to":"now-1h"}`},
		{"bad time", `{"datasource":"fake","query":"up","from":"yesterday"}`},
		{"too many variables", `{"datasource":"fake","query":"up","variables":{` + manyVariables(maxVariables+1) + `}}`},
		{"bad variable name", `{"datasource":"fake","query":"up","variables":{"1bad":["x"]}}`},
		{"missing datasource", `{"query":"up"}`},
		{"unknown datasource", `{"datasource":"nope","query":"up"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := call(t, r, nameQueryRange, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError {
				t.Fatalf("expected a tool error, got %#v", res.Content)
			}
		})
	}
}

func TestQueryToolsExpandTheDashboardTitleMacro(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input string
		want  string
	}{
		{"range with a title", nameQueryRange, `{"datasource":"fake","query":"up{d=\"$__dashboard\"}","dashboard_title":"Checkout health"}`, "Checkout health"},
		{"instant with a title", nameQueryInstant, `{"datasource":"fake","query":"up{d=\"$__dashboard\"}","dashboard_title":"Checkout health"}`, "Checkout health"},
		{"range default", nameQueryRange, `{"datasource":"fake","query":"up{d=\"$__dashboard\"}"}`, defaultDashTitle},
		{"instant default", nameQueryInstant, `{"datasource":"fake","query":"up{d=\"$__dashboard\"}","dashboard_title":""}`, defaultDashTitle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen datasource.Query
			ds := &fakeDS{
				rangeFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
					seen = q
					return []datasource.Frame{timeNumberFrame(nil, 1)}, nil
				},
				instantFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]datasource.Frame, error) {
					seen = q
					return []datasource.Frame{timeNumberFrame(nil, 1)}, nil
				},
			}
			res, err := call(t, newTestRegistry(t, ds, nil), tt.tool, tt.input)
			if err != nil || res.IsError {
				t.Fatalf("res = %#v err = %v", res, err)
			}
			if seen.Dashboard != tt.want {
				t.Fatalf("the datasource saw dashboard %q, want %q", seen.Dashboard, tt.want)
			}
		})
	}
}

func TestQueryToolsRunTheirTextAsAnExploreQuery(t *testing.T) {
	for _, tool := range []string{nameQueryRange, nameQueryInstant} {
		var seen []datasource.Query
		ds := &fakeDS{
			typ: "clickhouse",
			rangeFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
				seen = append(seen, q)
				return []datasource.Frame{logTableFrame()}, nil
			},
			instantFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]datasource.Frame, error) {
				seen = append(seen, q)
				return []datasource.Frame{logTableFrame()}, nil
			},
		}
		res, err := call(t, newTestRegistry(t, ds, nil), tool, `{"datasource":"fake","query":"SELECT TraceId FROM otel_traces LIMIT 3"}`)
		if err != nil || res.IsError {
			t.Fatalf("%s: res = %#v err = %v", tool, res, err)
		}
		if len(seen) != 1 || !seen[0].Explore {
			t.Fatalf("%s: the datasource saw %+v, want one explore query", tool, seen)
		}
	}
}

func TestQueryRangePassesTheFormatToTheDatasource(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"table", `{"datasource":"fake","query":"SELECT 1","format":"table"}`, "table"},
		{"logs", `{"datasource":"fake","query":"SELECT 1","format":"logs"}`, "logs"},
		{"heatmap", `{"datasource":"fake","query":"SELECT 1","format":"heatmap"}`, "heatmap"},
		{"omitted", `{"datasource":"fake","query":"SELECT 1"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen datasource.Query
			ds := &fakeDS{
				typ: "clickhouse",
				rangeFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
					seen = q
					return []datasource.Frame{logTableFrame()}, nil
				},
			}
			res, err := call(t, newTestRegistry(t, ds, nil), nameQueryRange, tt.input)
			if err != nil || res.IsError {
				t.Fatalf("res = %#v err = %v", res, err)
			}
			if seen.Format != tt.want {
				t.Fatalf("the datasource saw format %q, want %q", seen.Format, tt.want)
			}
		})
	}
}

func TestQueryRangeRefusesAnUnknownFormat(t *testing.T) {
	res, err := call(t, newTestRegistry(t, &fakeDS{}, nil), nameQueryRange, `{"datasource":"fake","query":"up","format":"pie"}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != string(queryproxy.CodeInvalidRequest) {
		t.Fatalf("code = %q", code)
	}
}

func TestQueryToolsRefuseAnOversizedDashboardTitle(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	for _, tool := range []string{nameQueryRange, nameQueryInstant} {
		res, err := call(t, r, tool, `{"datasource":"fake","query":"up","dashboard_title":"`+strings.Repeat("a", maxDashTitleChars+1)+`"}`)
		if err != nil {
			t.Fatal(err)
		}
		if code := errorCode(t, res); code != string(queryproxy.CodeInvalidRequest) {
			t.Fatalf("%s code = %q", tool, code)
		}
	}
}

func manyVariables(n int) string {
	var parts []string
	for i := range n {
		parts = append(parts, fmt.Sprintf(`"v%d":["x"]`, i))
	}
	return strings.Join(parts, ",")
}

func TestResolveRange(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	t.Run("defaults to the last hour", func(t *testing.T) {
		rng, _, ok := resolveRange("", "", now)
		if !ok || !rng.To.Equal(now) || rng.To.Sub(rng.From) != time.Hour {
			t.Fatalf("range = %+v ok = %v", rng, ok)
		}
	})
	t.Run("accepts exactly 31 days", func(t *testing.T) {
		if _, _, ok := resolveRange("now-31d", "now", now); !ok {
			t.Fatal("31 days rejected")
		}
	})
	for name, args := range map[string][2]string{
		"over 31 days":     {"now-32d", "now"},
		"from equals to":   {"now", "now"},
		"from after to":    {"now", "now-1h"},
		"bad from":         {"soon", "now"},
		"bad to":           {"now-1h", "later"},
		"long expression":  {strings.Repeat("a", maxTimeExprBytes+1), "now"},
		"long to":          {"now-1h", strings.Repeat("a", maxTimeExprBytes+1)},
		"epoch digits big": {"99999999999999999999", "now"},
	} {
		t.Run(name, func(t *testing.T) {
			_, res, ok := resolveRange(args[0], args[1], now)
			if ok || errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("ok = %v res = %#v", ok, res)
			}
		})
	}
}

func TestBoundedInt(t *testing.T) {
	five, zero, hundred := 5, 0, 100
	if v, _, ok := boundedInt(nil, 7, 1, 10, "x"); !ok || v != 7 {
		t.Fatalf("default = %d %v", v, ok)
	}
	if v, _, ok := boundedInt(&five, 7, 1, 10, "x"); !ok || v != 5 {
		t.Fatalf("value = %d %v", v, ok)
	}
	for _, p := range []*int{&zero, &hundred} {
		if _, _, ok := boundedInt(p, 7, 1, 10, "x"); ok {
			t.Fatalf("%d accepted", *p)
		}
	}
}

func TestClassifyRouting(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCode  string
		wantErr   bool
		wantStep  string
		unchecked bool
	}{
		{"canceled", context.Canceled, "", true, "", false},
		{"internal", errors.New("dial tcp 10.0.0.5: password=hunter2"), "tool_failed", false, "Could not run query_instant: tool_failed", false},
		{"deadline", context.DeadlineExceeded, "query_timeout", false, "Ran a query on fake: could not check, query_timeout", true},
		{"upstream timeout", datasource.Errorf(datasource.ErrTimeout, "after 15s"), "query_timeout", false, "Ran a query on fake: could not check, query_timeout", true},
		{"invalid query", datasource.Errorf(datasource.ErrInvalidQuery, "bad promql"), "query_invalid", false, "Ran a query on fake: query_invalid", false},
		{"forbidden", datasource.Errorf(datasource.ErrForbiddenQuery, "INSERT"), "query_forbidden", false, "Ran a query on fake: query_forbidden", false},
		{"unavailable", datasource.Errorf(datasource.ErrUnavailable, "down"), "datasource_unavailable", false, "Ran a query on fake: could not check, datasource_unavailable", true},
		{"busy", queryproxy.ErrUpstreamBusy, "too_many_requests", false, "Ran a query on fake: could not check, too_many_requests", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &fakeDS{instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
				return nil, tt.err
			}}
			r := newTestRegistry(t, ds, nil)
			res, err := call(t, r, nameQueryInstant, `{"datasource":"fake","query":"up"}`)
			if tt.wantErr {
				if err == nil || res.Content != nil || res.IsError {
					t.Fatalf("res = %#v err = %v, want an internal error", res, err)
				}
				if errors.Is(tt.err, context.Canceled) != errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation not preserved: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := errorCode(t, res); got != tt.wantCode {
				t.Fatalf("code = %q, want %q", got, tt.wantCode)
			}
			if res.Step != tt.wantStep {
				t.Fatalf("step = %q, want %q", res.Step, tt.wantStep)
			}
			body, _ := res.Content.(errorBody)
			if got := res.Unchecked != ""; got != tt.unchecked || (body.Note == uncheckedNote) != tt.unchecked {
				t.Fatalf("unchecked = %q, note = %q, want unchecked %v", res.Unchecked, body.Note, tt.unchecked)
			}
		})
	}
}

func TestDiscoveryErrorsRouteThroughClassify(t *testing.T) {
	ds := &fakeDS{typ: "clickhouse", discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return nil, datasource.Errorf(datasource.ErrNotFound, "not supported")
	}}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameGetSchema, `{"datasource":"fake"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != "not_found" {
		t.Fatalf("code = %q", got)
	}
	ds.discoverFn = func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return nil, context.Canceled
	}
	if _, err := call(t, r, nameListMetrics, `{"datasource":"fake"}`); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestGetSchemaTruncation(t *testing.T) {
	tables := func(n int) *datasource.Catalog {
		c := &datasource.Catalog{}
		for i := range n {
			c.Tables = append(c.Tables, datasource.Table{Name: "t" + string(rune('a'+i%26)) + string(rune('a'+i/26%26))})
		}
		return c
	}
	columns := func(n int) *datasource.Catalog {
		c := &datasource.Catalog{}
		for i := range n {
			c.Columns = append(c.Columns, datasource.Column{Name: "c" + string(rune('a'+i%26)) + string(rune('a'+i/26%26))})
		}
		return c
	}
	tests := []struct {
		name      string
		input     string
		catalog   *datasource.Catalog
		wantLimit int
		wantKept  int
		wantCut   bool
	}{
		{"tables over the cap", `{"datasource":"fake"}`, tables(maxDiscoverItems + 1), maxDiscoverItems + 1, maxDiscoverItems, true},
		{"tables at the cap", `{"datasource":"fake"}`, tables(maxDiscoverItems), maxDiscoverItems + 1, maxDiscoverItems, false},
		{"columns over the cap", `{"datasource":"fake","table":"otel_logs"}`, columns(maxDiscoverItems + 1), maxDiscoverItems + 1, maxDiscoverItems, true},
		{"columns at the cap", `{"datasource":"fake","table":"otel_logs"}`, columns(maxDiscoverItems), maxDiscoverItems + 1, maxDiscoverItems, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &fakeDS{typ: "clickhouse", discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) { return tt.catalog, nil }}
			r := newTestRegistry(t, ds, nil)
			res, err := call(t, r, nameGetSchema, tt.input)
			if err != nil || res.IsError {
				t.Fatalf("res = %#v err = %v", res, err)
			}
			if got := ds.lastRequest().Limit; got != tt.wantLimit {
				t.Fatalf("limit = %d, want %d", got, tt.wantLimit)
			}
			var kept int
			var cut bool
			switch out := res.Content.(type) {
			case tablesOut:
				kept, cut = len(out.Tables), out.Truncated
			case columnsOut:
				kept, cut = len(out.Columns), out.Truncated
			}
			if kept != tt.wantKept || cut != tt.wantCut {
				t.Fatalf("kept = %d truncated = %v, want %d/%v", kept, cut, tt.wantKept, tt.wantCut)
			}
		})
	}
}

func TestListDatasourcesCanceled(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx, cancel := context.WithCancel(auth.WithPrincipal(reading(context.Background()), auth.Principal{Role: auth.RoleEditor}))
	cancel()
	res, err := r.Call(ctx, nameListDatasources, []byte(`{}`))
	if !errors.Is(err, context.Canceled) || res.Content != nil {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	res, err = call(t, r, nameListDatasources, `{}`)
	if err != nil || res.IsError || res.Step != "Listed 1 datasource" {
		t.Fatalf("res = %#v err = %v", res, err)
	}
}

type fakePageDS struct {
	*fakeDS
	byOffset bool
	onPage   func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error)
	pages    []datasource.PageRequest
}

func (f *fakePageDS) PagesByOffset() bool { return f.byOffset }

func (f *fakePageDS) QueryPage(_ context.Context, req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
	f.pages = append(f.pages, req)
	return f.onPage(req)
}

func newPageRegistry(t *testing.T, ds *fakePageDS) *Registry {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger})
}

func logRows(n int) datasource.Frame {
	start := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	times, bodies := make([]any, n), make([]any, n)
	for i := range n {
		times[i], bodies[i] = start.Add(-time.Duration(i)*time.Second), fmt.Sprintf("line %d", i)
	}
	return datasource.Frame{Fields: []datasource.Field{
		{Name: "Timestamp", Type: datasource.FieldTime, Values: times},
		{Name: "Body", Type: datasource.FieldString, Values: bodies},
	}}
}

func TestQueryInstantPagesAndKeepsTheSummaryCompact(t *testing.T) {
	ds := &fakePageDS{fakeDS: &fakeDS{typ: "loki"}, onPage: func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		return []datasource.Frame{logRows(300)}, datasource.PageInfo{Page: req.Page, PageSize: req.PageSize, Total: 1234, TotalRelation: datasource.TotalAtLeast, HasNext: true, Next: &datasource.PageCursor{Page: req.Page + 1, EndNs: 1}}, nil
	}}
	r := newPageRegistry(t, ds)
	res, err := call(t, r, nameQueryInstant, `{"datasource":"fake","query":"{a=\"b\"}","page_size":300}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	summary, ok := res.Content.(querySummary)
	if !ok {
		t.Fatalf("content is %T", res.Content)
	}
	if summary.Table == nil || len(summary.Table.Rows) != 300 || summary.Table.TotalRows != 300 || summary.Truncated {
		t.Fatalf("table = %+v, want the whole page of 300 rows", summary.Table)
	}
	if summary.Page == nil || summary.Page.Page != 1 || summary.Page.PageSize != 300 || summary.Page.Total == nil || *summary.Page.Total != 1234 || summary.Page.TotalRelation != "gte" || !summary.Page.HasNext || summary.Page.Next == "" {
		t.Fatalf("page = %+v", summary.Page)
	}
	if want := "Ran a query on fake: 300 rows (page 1 of at least 1,234 rows; pass next as cursor for more)"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
	if len(ds.pages) != 1 || ds.pages[0].PageSize != 300 || ds.pages[0].Page != 1 || ds.pages[0].Query.Limits != queryproxy.DraftLimits {
		t.Fatalf("pages = %+v", ds.pages)
	}
	ds.onPage = func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		return []datasource.Frame{logRows(2)}, datasource.PageInfo{Page: req.Page, PageSize: req.PageSize, Total: 302, TotalRelation: datasource.TotalExact}, nil
	}
	res, err = call(t, r, nameQueryInstant, `{"datasource":"fake","query":"{a=\"b\"}","page_size":300,"cursor":`+strconv.Quote(summary.Page.Next)+`}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	if want := "Ran a query on fake: 2 rows (page 2 of 302 rows)"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
	if last := ds.pages[1]; last.Page != 2 || last.Cursor == nil || last.Cursor.EndNs != 1 {
		t.Fatalf("page 2 request = %+v, want the cursor's page and end", last)
	}
}

func TestQueryInstantPassesAsWrittenToThePageRequest(t *testing.T) {
	ds := &fakePageDS{fakeDS: &fakeDS{typ: "clickhouse"}, onPage: func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		return []datasource.Frame{logRows(2)}, datasource.PageInfo{Page: req.Page, PageSize: req.PageSize, Total: 2, TotalRelation: datasource.TotalExact}, nil
	}}
	r := newPageRegistry(t, ds)
	for _, input := range []string{
		`{"datasource":"fake","query":"SELECT 1","page_size":50,"as_written":true}`,
		`{"datasource":"fake","query":"SELECT 1","page_size":50}`,
	} {
		if res, err := call(t, r, nameQueryInstant, input); err != nil || res.IsError {
			t.Fatalf("res = %#v err = %v", res, err)
		}
	}
	if len(ds.pages) != 2 || !ds.pages[0].AsWritten || ds.pages[1].AsWritten {
		t.Fatalf("AsWritten of the page requests = %+v, want true then false", ds.pages)
	}
}

func TestQueryInstantRejectsBadPaging(t *testing.T) {
	ds := &fakePageDS{fakeDS: &fakeDS{typ: "loki"}, onPage: func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		return nil, datasource.PageInfo{Page: 1, PageSize: req.PageSize, TotalRelation: datasource.TotalExact}, nil
	}}
	r := newPageRegistry(t, ds)
	for name, input := range map[string]string{
		"page_size below 50":               `{"datasource":"fake","query":"up","page_size":10}`,
		"page_size above 500":              `{"datasource":"fake","query":"up","page_size":501}`,
		"page zero":                        `{"datasource":"fake","query":"up","page_size":50,"page":0}`,
		"page without page_size":           `{"datasource":"fake","query":"up","page":2}`,
		"cursor without page_size":         `{"datasource":"fake","query":"up","cursor":"x"}`,
		"sort without page_size":           `{"datasource":"fake","query":"up","sort":"-Duration"}`,
		"as_written without page_size":     `{"datasource":"fake","query":"up","as_written":true}`,
		"sort that is not a column":        `{"datasource":"fake","query":"up","page_size":50,"sort":"Duration; DROP"}`,
		"sort on a datasource that cannot": `{"datasource":"fake","query":"up","page_size":50,"sort":"-Duration"}`,
		"cursor over 2 KiB":                `{"datasource":"fake","query":"up","page_size":50,"cursor":"` + strings.Repeat("x", 2049) + `"}`,
		"page 2 without a cursor":          `{"datasource":"fake","query":"up","page_size":50,"page":2}`,
		"a cursor of another query":        `{"datasource":"fake","query":"up","page_size":50,"cursor":"` + datasource.EncodeCursor(datasource.PageCursor{Scope: datasource.CursorScope("fake", "other"), Page: 2}) + `"}`,
		"page_size on a plain instant":     `{"datasource":"fake","query":"up","page_size":"many"}`,
	} {
		t.Run(name, func(t *testing.T) {
			res, err := call(t, r, nameQueryInstant, input)
			if err != nil || errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("res = %#v err = %v", res, err)
			}
		})
	}
	if len(ds.pages) != 0 {
		t.Fatalf("%d pages ran for invalid inputs", len(ds.pages))
	}
	plain := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, plain, nameQueryInstant, `{"datasource":"fake","query":"up","page_size":50}`)
	if err != nil || errorCode(t, res) != codeInvalidRequest {
		t.Fatalf("res = %#v err = %v, want a refusal for a datasource that does not page", res, err)
	}
}

func TestPageDescribeNamesTheTotalAndTheWalk(t *testing.T) {
	exact, big := int64(42), int64(1234567)
	cases := []struct {
		name string
		page pageOut
		want string
	}{
		{"last page", pageOut{Page: 3, Total: &exact, TotalRelation: "eq"}, " (page 3 of 42 rows)"},
		{"more pages", pageOut{Page: 1, Total: &big, TotalRelation: "gte", HasNext: true}, " (page 1 of at least 1,234,567 rows; pass next as cursor for more)"},
		{"unknown", pageOut{Page: 2, TotalRelation: "unknown", HasNext: true}, " (page 2, total unknown; pass next as cursor for more)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.page.describe(); got != tc.want {
				t.Fatalf("describe = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSummaryNoteTellsHowToGetMore(t *testing.T) {
	frame := datasource.Frame{Fields: []datasource.Field{{Name: "TraceId", Type: datasource.FieldString, Values: []any{"a"}}}, Meta: &datasource.FrameMeta{Partial: true}}
	summary := summarize(queryproxy.Result{Queries: []queryproxy.QueryResult{{Ref: "A", Frames: []datasource.Frame{frame}}}})
	if summary.Note != queryproxy.PartialNote {
		t.Fatalf("note = %q", summary.Note)
	}
}

func TestQueryOffersPagingOnlyOnAnInstantQueryOfADatasourceThatPages(t *testing.T) {
	truncated := func() []datasource.Frame {
		f := logRows(3)
		f.Meta = &datasource.FrameMeta{Truncated: true}
		return []datasource.Frame{f}
	}
	ds := &fakePageDS{fakeDS: &fakeDS{
		typ: "loki",
		instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return truncated(), nil
		},
		rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
			return truncated(), nil
		},
	}}
	r := newPageRegistry(t, ds)
	for name, want := range map[string]struct{ tool, note string }{
		"instant": {nameQueryInstant, queryproxy.MoreRowsNote},
		"range":   {nameQueryRange, queryproxy.NarrowNote},
	} {
		res, err := call(t, r, want.tool, `{"datasource":"fake","query":"{a=\"b\"}"}`)
		if err != nil || res.IsError {
			t.Fatalf("%s: res = %#v err = %v", name, res, err)
		}
		if got := res.Content.(querySummary).Note; got != want.note {
			t.Errorf("%s: note = %q, want %q", name, got, want.note)
		}
		if !strings.Contains(res.Step, want.note) {
			t.Errorf("%s: step = %q, want the note %q", name, res.Step, want.note)
		}
	}
}

func TestSummarizeReportsTheExpandedQueryText(t *testing.T) {
	got := summarize(queryproxy.Result{Queries: []queryproxy.QueryResult{
		{Ref: "A", Executed: `rate(up{job="api"}[5m])`},
		{Ref: "B"},
	}})
	if len(got.Executed) != 1 || got.Executed[0].Ref != "A" || got.Executed[0].Text != `rate(up{job="api"}[5m])` {
		t.Fatalf("executed = %+v", got.Executed)
	}
}

func lokiStreamFrame(labels map[string]string, base time.Time, offsets ...int) datasource.Frame {
	times := make([]any, len(offsets))
	bodies := make([]any, len(offsets))
	for i, off := range offsets {
		times[i] = base.Add(-time.Duration(off) * time.Second)
		bodies[i] = fmt.Sprintf("error %s %d", labels["pod"], off)
	}
	return datasource.Frame{Labels: labels, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime, Values: times},
		{Name: "Body", Type: datasource.FieldString, Values: bodies},
		datasource.TimeNsField(times),
	}}
}

func TestSummarizeMergesLogStreamsIntoOneNewestFirstTable(t *testing.T) {
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	frames := []datasource.Frame{
		lokiStreamFrame(map[string]string{"pod": "a"}, base, 9),
		lokiStreamFrame(map[string]string{"pod": "b"}, base, 1, 4, 7),
		lokiStreamFrame(map[string]string{"pod": "c"}, base, 2, 3, 5, 6, 8),
	}
	summary := summarize(queryproxy.Result{Kind: queryproxy.KindInstant, Queries: []queryproxy.QueryResult{{Ref: "A", Frames: frames}}})
	if summary.Table == nil || summary.Table.TotalRows != 9 || len(summary.Table.Rows) != maxTableRows-1 {
		t.Fatalf("table = %+v, want 9 rows from 3 streams", summary.Table)
	}
	names := make([]string, len(summary.Table.Columns))
	for i, c := range summary.Table.Columns {
		names[i] = c.Name
	}
	if want := []string{"Time", "Body", "TimeNs", "labels"}; !slices.Equal(names, want) {
		t.Fatalf("columns = %v, want %v", names, want)
	}
	for i, row := range summary.Table.Rows {
		wantPod := map[int]string{1: "b", 2: "c", 3: "c", 4: "b", 5: "c", 6: "c", 7: "b", 8: "c", 9: "a"}[i+1]
		if row[3] != "pod="+wantPod || row[1] != fmt.Sprintf("error %s %d", wantPod, i+1) {
			t.Fatalf("row %d = %v, want newest first from pod %s", i, row, wantPod)
		}
	}
}

func TestSummarizeFitsTenLongLogLines(t *testing.T) {
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	long := strings.Repeat("x", 300)
	f := lokiStreamFrame(map[string]string{"namespace": "kafka-system", "pod": "kafka-broker-0", "container": "kafka", "service_name": "kafka"}, base, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12)
	for i := range f.Fields[1].Values {
		f.Fields[1].Values[i] = long
	}
	other := lokiStreamFrame(map[string]string{"pod": "z"}, base, 13)
	summary := summarize(queryproxy.Result{Kind: queryproxy.KindInstant, Queries: []queryproxy.QueryResult{{Ref: "A", Frames: []datasource.Frame{other, f}}}})
	if summary.Table == nil || len(summary.Table.Rows) != maxTableRows {
		t.Fatalf("rows = %d, want %d", len(summary.Table.Rows), maxTableRows)
	}
}

func framesSummary(frames ...datasource.Frame) querySummary {
	return summarize(queryproxy.Result{Kind: queryproxy.KindInstant, Queries: []queryproxy.QueryResult{{Ref: "A", Frames: frames}}})
}

func columnNames(t *testing.T, s querySummary) []string {
	t.Helper()
	if s.Table == nil {
		t.Fatalf("no table in %+v", s)
	}
	names := make([]string, len(s.Table.Columns))
	for i, c := range s.Table.Columns {
		names[i] = c.Name
	}
	return names
}

func TestSummarizeGivesOneLogStreamTheLabelsColumnNewestFirst(t *testing.T) {
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	summary := framesSummary(lokiStreamFrame(map[string]string{"pod": "a", "app": "x"}, base, 5, 1, 3))
	if got, want := columnNames(t, summary), []string{"Time", "Body", "TimeNs", "labels"}; !slices.Equal(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	var bodies []any
	for _, row := range summary.Table.Rows {
		if row[3] != "app=x,pod=a" {
			t.Fatalf("labels = %v", row[3])
		}
		bodies = append(bodies, row[1])
	}
	if want := []any{"error a 1", "error a 3", "error a 5"}; !slices.Equal(bodies, want) {
		t.Fatalf("bodies = %v, want %v", bodies, want)
	}
}

func TestSummarizeLeavesATableWithTimeNsButNoStreamLabelsAlone(t *testing.T) {
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	f := lokiStreamFrame(nil, base, 5, 1, 3)
	summary := framesSummary(f)
	if got, want := columnNames(t, summary), []string{"Time", "Body", "TimeNs"}; !slices.Equal(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	if summary.Table.Rows[0][1] != "error  5" || summary.Table.Rows[2][1] != "error  3" {
		t.Fatalf("rows = %v, want the frame's own order", summary.Table.Rows)
	}
}

func TestSummarizeKeepsFrameOrderBetweenLogStreamsAndTables(t *testing.T) {
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	stream := lokiStreamFrame(map[string]string{"pod": "a"}, base, 1, 2)
	table := logTableFrame()

	logsFirst := framesSummary(stream, table)
	if got := columnNames(t, logsFirst); !slices.Contains(got, "labels") || !logsFirst.Truncated {
		t.Fatalf("logs first: columns = %v truncated = %v, want the log table and truncated", got, logsFirst.Truncated)
	}
	tableFirst := framesSummary(table, stream)
	if got := columnNames(t, tableFirst); slices.Contains(got, "labels") || !tableFirst.Truncated {
		t.Fatalf("table first: columns = %v truncated = %v, want the plain table and truncated", got, tableFirst.Truncated)
	}
	if only := framesSummary(stream); only.Truncated {
		t.Fatalf("a lone stream is truncated: %+v", only)
	}
}

func TestSummarizeSortsALineWithoutTimeNsByItsTimeOrKeepsItsPlace(t *testing.T) {
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	a := lokiStreamFrame(map[string]string{"pod": "a"}, base, 2, 6)
	a.Fields[2].Values[0] = nil
	b := lokiStreamFrame(map[string]string{"pod": "b"}, base, 4, 8, 9)
	b.Fields[0].Values[1], b.Fields[2].Values[1] = nil, nil
	summary := framesSummary(a, b)
	var bodies []any
	for _, row := range summary.Table.Rows {
		bodies = append(bodies, row[1])
	}
	want := []any{"error a 2", "error b 4", "error b 8", "error a 6", "error b 9"}
	if !slices.Equal(bodies, want) {
		t.Fatalf("bodies = %v, want %v", bodies, want)
	}
}

func TestSummarizeBudgetsLogTablesAboveOtherTables(t *testing.T) {
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	long := strings.Repeat("x", 300)
	stream := lokiStreamFrame(map[string]string{"pod": "a"}, base, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	plain := lokiStreamFrame(nil, base, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	for _, f := range []datasource.Frame{stream, plain} {
		for i := range f.Fields[1].Values {
			f.Fields[1].Values[i] = long
		}
	}
	plain.Fields = append(plain.Fields, plain.Fields[1], plain.Fields[1])
	if got := len(framesSummary(stream).Table.Rows); got != maxTableRows {
		t.Fatalf("log rows = %d, want %d", got, maxTableRows)
	}
	if got := len(framesSummary(plain).Table.Rows); got >= maxTableRows {
		t.Fatalf("plain rows = %d, want fewer than %d within the 4 KiB budget", got, maxTableRows)
	}
	if tableRowsBudget != 4<<10 {
		t.Fatalf("tableRowsBudget = %d, want 4096 for log_context and plain tables", tableRowsBudget)
	}
}

func TestQueryInstantShowsTheWholePageOfLogLines(t *testing.T) {
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	long := strings.Repeat("x", 300)
	offsets := make([]int, 50)
	for i := range offsets {
		offsets[i] = i + 1
	}
	frame := lokiStreamFrame(map[string]string{"pod": "a", "namespace": "kafka"}, base, offsets...)
	for i := range frame.Fields[1].Values {
		frame.Fields[1].Values[i] = long
	}
	ds := &fakePageDS{fakeDS: &fakeDS{typ: "loki"}, onPage: func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		return []datasource.Frame{frame}, datasource.PageInfo{Page: req.Page, PageSize: req.PageSize, TotalRelation: datasource.TotalUnknown}, nil
	}}
	res, err := call(t, newPageRegistry(t, ds), nameQueryInstant, `{"datasource":"fake","query":"{a=\"b\"}","page_size":50}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	summary := res.Content.(querySummary)
	if summary.Table == nil || len(summary.Table.Rows) != 50 || summary.Truncated {
		t.Fatalf("rows = %v truncated = %v, want all 50 lines of the page", summary.Table, summary.Truncated)
	}
}
