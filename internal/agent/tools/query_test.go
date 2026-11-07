package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
		{"truncated", datasource.FrameMeta{Truncated: true}, "Ran a query on fake: 3 rows (more rows exist; narrow from and to)"},
		{"partial", datasource.FrameMeta{Partial: true}, "Ran a query on fake: 3 rows (partial: the datasource did not search every block)"},
		{"both", datasource.FrameMeta{Truncated: true, Partial: true}, "Ran a query on fake: 3 rows (more rows exist; narrow from and to) (partial: the datasource did not search every block)"},
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
		name     string
		err      error
		wantCode string
		wantErr  bool
		wantStep string
	}{
		{"canceled", context.Canceled, "", true, ""},
		{"internal", errors.New("dial tcp 10.0.0.5: password=hunter2"), "tool_failed", false, "Could not run query_instant: tool_failed"},
		{"deadline", context.DeadlineExceeded, "query_timeout", false, "Ran a query on fake: query_timeout"},
		{"upstream timeout", datasource.Errorf(datasource.ErrTimeout, "after 15s"), "query_timeout", false, "Ran a query on fake: query_timeout"},
		{"invalid query", datasource.Errorf(datasource.ErrInvalidQuery, "bad promql"), "query_invalid", false, "Ran a query on fake: query_invalid"},
		{"forbidden", datasource.Errorf(datasource.ErrForbiddenQuery, "INSERT"), "query_forbidden", false, "Ran a query on fake: query_forbidden"},
		{"unavailable", datasource.Errorf(datasource.ErrUnavailable, "down"), "datasource_unavailable", false, "Ran a query on fake: datasource_unavailable"},
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
		})
	}
}

func TestDiscoveryErrorsRouteThroughClassify(t *testing.T) {
	ds := &fakeDS{discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
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
			ds := &fakeDS{discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) { return tt.catalog, nil }}
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
	ctx, cancel := context.WithCancel(auth.WithPrincipal(context.Background(), auth.Principal{Role: auth.RoleEditor}))
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
	if summary.Table == nil || len(summary.Table.Rows) != maxTableRows || summary.Table.TotalRows != 300 || !summary.Truncated {
		t.Fatalf("table = %+v, want %d rows shown of 300", summary.Table, maxTableRows)
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

func TestQueryInstantRejectsBadPaging(t *testing.T) {
	ds := &fakePageDS{fakeDS: &fakeDS{typ: "loki"}, onPage: func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		return nil, datasource.PageInfo{Page: 1, PageSize: req.PageSize, TotalRelation: datasource.TotalExact}, nil
	}}
	r := newPageRegistry(t, ds)
	for name, input := range map[string]string{
		"page_size below 50":           `{"datasource":"fake","query":"up","page_size":10}`,
		"page_size above 500":          `{"datasource":"fake","query":"up","page_size":501}`,
		"page zero":                    `{"datasource":"fake","query":"up","page_size":50,"page":0}`,
		"page without page_size":       `{"datasource":"fake","query":"up","page":2}`,
		"cursor without page_size":     `{"datasource":"fake","query":"up","cursor":"x"}`,
		"cursor over 2 KiB":            `{"datasource":"fake","query":"up","page_size":50,"cursor":"` + strings.Repeat("x", 2049) + `"}`,
		"page 2 without a cursor":      `{"datasource":"fake","query":"up","page_size":50,"page":2}`,
		"a cursor of another query":    `{"datasource":"fake","query":"up","page_size":50,"cursor":"` + datasource.EncodeCursor(datasource.PageCursor{Scope: datasource.CursorScope("fake", "other"), Page: 2}) + `"}`,
		"page_size on a plain instant": `{"datasource":"fake","query":"up","page_size":"many"}`,
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
