package tools

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func slowTrace(route string, dbTime time.Duration) []traceSpan {
	return []traceSpan{
		span("r", "", "frontend", route, "Unset", 0, 20*time.Millisecond+dbTime),
		span("c", "r", "cart", "GetCart", "Unset", 5*time.Millisecond, 10*time.Millisecond+dbTime),
		span("d", "c", "redis", "HGET", "Unset", 10*time.Millisecond, dbTime),
	}
}

func TestTraceSlowPathReportsEachRouteOnItsOwn(t *testing.T) {
	ds := &sampleFakeDS{traces: map[string][]traceSpan{}, list: datasource.TraceList{Complete: true}}
	add := func(n int, route string, d time.Duration) {
		ds.list.Traces = append(ds.list.Traces, datasource.TraceSummary{ID: sampleID(n), Route: route, Duration: d})
		ds.traces[sampleID(n)] = slowTrace(route, d-20*time.Millisecond)
	}
	for i := range 5 {
		add(i, "GET /cart", 30*time.Millisecond)
	}
	add(10, "GET /cart", 400*time.Millisecond)
	add(11, "GET /cart", 300*time.Millisecond)
	add(20, "POST /checkout", 50*time.Millisecond)
	add(21, "POST /checkout", 50*time.Millisecond)
	add(22, "POST /checkout", 500*time.Millisecond)
	r := newSampleRig(t, ds)
	res, err := call(t, r, nameTraceSlowPath, `{"datasource":"traces","service":"frontend"}`)
	if err != nil || res.IsError {
		t.Fatalf("err = %v, result = %#v", err, res.Content)
	}
	if req := ds.listReqs[0]; req.MinDuration != 0 || req.Errors || req.Filter.Service != "frontend" {
		t.Fatalf("list request = %+v", req)
	}
	out := res.Content.(slowPathOut)
	if out.Threshold != thresholdP50 || out.Listed != 10 || !out.Complete || out.Sample || len(out.Routes) != 2 {
		t.Fatalf("out = %+v", out)
	}
	cart, checkout := out.Routes[0], out.Routes[1]
	if cart.Route != "GET /cart" || cart.SlowTraces != 2 || cart.Examined != 2 || *cart.P50MS != 30 || cart.ThresholdMS != 60 {
		t.Fatalf("cart = %+v", cart)
	}
	if top := cart.TopSelfTime[0]; top.Service != "redis" || top.Operation != "HGET" || top.SelfTimeMS != 660 || top.Traces != 2 {
		t.Fatalf("cart top = %+v", cart.TopSelfTime)
	}
	if cart.Example == nil || cart.Example.TraceID != sampleID(10) || cart.Example.DurationMS != 400 {
		t.Fatalf("cart example = %+v", cart.Example)
	}
	if !slices.Equal(cart.Services, []serviceCountOut{{"cart", 2}, {"frontend", 2}, {"redis", 2}}) {
		t.Fatalf("cart services = %+v", cart.Services)
	}
	if checkout.Route != "POST /checkout" || checkout.SlowTraces != 1 || checkout.Example.TraceID != sampleID(22) {
		t.Fatalf("checkout = %+v", checkout)
	}
	if !strings.Contains(out.Note, "cover all 3 slow traces") {
		t.Fatalf("note = %q", out.Note)
	}
}

func TestTraceSlowPathPassesAGivenThresholdToTheListing(t *testing.T) {
	ds := &sampleFakeDS{traces: map[string][]traceSpan{}}
	ds.list.Traces = []datasource.TraceSummary{{ID: sampleID(1), Route: "GET /", Duration: time.Second}}
	ds.traces[sampleID(1)] = slowTrace("GET /", 980*time.Millisecond)
	r := newSampleRig(t, ds)
	res, err := call(t, r, nameTraceSlowPath, `{"datasource":"traces","route":"GET /","threshold_ms":250}`)
	if err != nil || res.IsError {
		t.Fatalf("err = %v, result = %#v", err, res.Content)
	}
	if req := ds.listReqs[0]; req.MinDuration != 250*time.Millisecond || req.Filter.Route != "GET /" {
		t.Fatalf("list request = %+v", req)
	}
	out := res.Content.(slowPathOut)
	if out.Threshold != thresholdGiven || !out.Sample || out.Routes[0].ThresholdMS != 250 || out.Routes[0].P50MS != nil {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.Note, "at least 1 slow traces") || !strings.Contains(out.Note, "whole milliseconds") {
		t.Fatalf("note = %q", out.Note)
	}
	res, err = call(t, r, nameTraceSlowPath, `{"datasource":"traces","threshold_ms":0}`)
	if err != nil || !res.IsError {
		t.Fatalf("a zero threshold was accepted: %#v", res.Content)
	}
}
