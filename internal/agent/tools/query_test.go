package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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
			got := summarize(queryproxy.Result{Frames: tt.frames, Kind: tt.kind, Step: 15 * time.Second})
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
	got := summarize(queryproxy.Result{Frames: []datasource.Frame{frame}, Kind: queryproxy.KindRange, Step: 18 * time.Second})
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
	got := summarize(queryproxy.Result{Frames: frames, Kind: queryproxy.KindRange})
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
		{"internal", errors.New("dial tcp 10.0.0.5: password=hunter2"), "", true, ""},
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
	ctx, cancel := context.WithCancel(context.Background())
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
