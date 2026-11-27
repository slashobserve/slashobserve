package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

type durationFakeDS struct {
	*traceFakeDS
	durationsFn func(datasource.TraceDurationRequest) (datasource.TraceDurations, error)
}

func (d *durationFakeDS) TraceDurations(_ context.Context, req datasource.TraceDurationRequest) (datasource.TraceDurations, error) {
	return d.durationsFn(req)
}

func newDurationRig(t *testing.T, typ string, stats datasource.TraceStats, fn func(datasource.TraceDurationRequest) (datasource.TraceDurations, error)) *Registry {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	ds := &durationFakeDS{traceFakeDS: &traceFakeDS{fakeDS: &fakeDS{typ: typ}, statsFn: func(context.Context, datasource.TraceStatsRequest) (datasource.TraceStats, error) { return stats, nil }}, durationsFn: fn}
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: "traces", Name: "Traces", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger, queryproxy.WithUpstreamWait(30*time.Millisecond)), Logger: logger})
}

func traceStatsBody(t *testing.T, r *Registry, input string) (string, Result) {
	t.Helper()
	res, err := call(t, r, nameTraceStats, input)
	if err != nil || res.IsError {
		t.Fatalf("err = %v, result = %#v", err, res.Content)
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	return string(body), res
}

func TestTraceStatsReportsExactWindowAndGroupQuantiles(t *testing.T) {
	approx := 512.0
	stats := datasource.TraceStats{Spans: 7, Errors: 1, Window: &datasource.TraceQuantiles{P99: &approx},
		GroupBy: "resource.service.name", Groups: []datasource.TraceGroup{{Value: "cart", Spans: 5, Errors: 1, TraceQuantiles: datasource.TraceQuantiles{P99: &approx}}, {Value: "ad", Spans: 2}}}
	var asked datasource.TraceDurationRequest
	r := newDurationRig(t, "tempo", stats, func(req datasource.TraceDurationRequest) (datasource.TraceDurations, error) {
		asked = req
		return datasource.TraceDurations{Exact: true,
			Window: datasource.DurationQuantiles{Spans: 7, P50: 20, P95: 295.313, P99: 295.313},
			Groups: []datasource.DurationQuantiles{{Value: "cart", Spans: 5, P50: 30, P95: 295.313, P99: 295.313}, {Value: "ad", Spans: 2, P50: 5, P95: 9, P99: 9}}}, nil
	})
	body, res := traceStatsBody(t, r, `{"datasource":"traces","query":"{}","group_by":"resource.service.name"}`)
	if asked.GroupBy != "resource.service.name" || asked.MaxSpans != datasource.TraceDurationSpanBudget {
		t.Fatalf("durations request = %+v", asked)
	}
	for _, want := range []string{
		`"window":{"p50_ms":20,"p95_ms":295.313,"p99_ms":295.313}`,
		`{"value":"cart","spans":5,"errors":1,"p50_ms":30,"p95_ms":295.313,"p99_ms":295.313}`,
		`"quantiles":"exact"`,
		`"quantile_spans":7`,
		`"note":"Window and group quantiles are exact: they come from all 7 matching span durations`,
		`Per-bucket quantiles are interpolated`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s\nmissing %s", body, want)
		}
	}
	if strings.Contains(body, "512") {
		t.Fatalf("an approximate figure survived: %s", body)
	}
	if res.Step != "Read trace stats on traces: 7 spans, p95 295.3 ms" {
		t.Fatalf("step = %q", res.Step)
	}
}

func TestTraceStatsSaysWhenQuantilesAreASample(t *testing.T) {
	r := newDurationRig(t, "tempo", datasource.TraceStats{Spans: 30000}, func(datasource.TraceDurationRequest) (datasource.TraceDurations, error) {
		return datasource.TraceDurations{Exact: true, Window: datasource.DurationQuantiles{Spans: 20000, P50: 1, P95: 2, P99: 3}}, nil
	})
	body, _ := traceStatsBody(t, r, `{"datasource":"traces","query":"{}"}`)
	for _, want := range []string{`"quantiles":"sample"`, `"quantile_spans":20000`, `rest on 20000 of the 30000 matching spans`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s\nmissing %s", body, want)
		}
	}
}

func TestTraceStatsBuildsClickHouseGroupsFromTheExactRead(t *testing.T) {
	r := newDurationRig(t, "clickhouse", datasource.TraceStats{Spans: 3}, func(datasource.TraceDurationRequest) (datasource.TraceDurations, error) {
		return datasource.TraceDurations{Exact: true, GroupsTruncated: true,
			Window: datasource.DurationQuantiles{Spans: 3, Errors: 1, P50: 2, P95: 3, P99: 3},
			Groups: []datasource.DurationQuantiles{{Value: "checkout", Spans: 3, Errors: 1, P50: 2, P95: 3, P99: 3}}}, nil
	})
	body, _ := traceStatsBody(t, r, `{"datasource":"traces","query":"","group_by":"resource.service.name"}`)
	for _, want := range []string{
		`"group_by":"resource.service.name","groups":[{"value":"checkout","spans":3,"errors":1,"p50_ms":2,"p95_ms":3,"p99_ms":3}],"groups_truncated":true,"quantiles":"exact"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s\nmissing %s", body, want)
		}
	}
	if strings.Contains(body, "Tempo") {
		t.Fatalf("a ClickHouse result names Tempo: %s", body)
	}
}

func TestTraceStatsKeepsTempoFiguresWhenTheExactReadFails(t *testing.T) {
	approx := 512.0
	r := newDurationRig(t, "tempo", datasource.TraceStats{Spans: 2, Window: &datasource.TraceQuantiles{P99: &approx}}, func(datasource.TraceDurationRequest) (datasource.TraceDurations, error) {
		return datasource.TraceDurations{}, errors.New("search failed")
	})
	body, _ := traceStatsBody(t, r, `{"datasource":"traces","query":"{}"}`)
	for _, want := range []string{`"p99_ms":512`, `"quantiles":"approximate"`, `"note":"The exact quantiles could not be read`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s\nmissing %s", body, want)
		}
	}
}

func TestExactQuantilesKeepTheStoresNoteOnItsBuckets(t *testing.T) {
	out := traceStatsOut{Spans: 3}
	out.applyDurations(datasource.TraceDurations{Exact: true, Window: datasource.DurationQuantiles{Spans: 3, P50: 1, P95: 2, P99: 2}}, nil, "", bucketNoteOf("opensearch", "Per-bucket quantiles are estimates."))
	if out.Quantiles != quantilesExact || !strings.HasSuffix(out.Note, " Per-bucket quantiles are estimates.") {
		t.Fatalf("quantiles %q, note %q", out.Quantiles, out.Note)
	}
	if bucketNoteOf("tempo", "") != bucketQuantilesNote {
		t.Error("Tempo lost its bucket note")
	}
}
