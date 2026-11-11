package main

import (
	"slices"
	"testing"
	"time"
)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestQuantileNearestRank(t *testing.T) {
	r := newRecorder()
	for _, n := range []int{50, 10, 40, 20, 30, 100, 90, 80, 70, 60} {
		r.Observe(ms(n))
	}
	tests := []struct {
		q    float64
		want time.Duration
	}{
		{0, ms(10)},
		{0.5, ms(50)},
		{0.95, ms(100)},
		{0.99, ms(100)},
		{1, ms(100)},
	}
	for _, tt := range tests {
		if got := r.Quantile(tt.q); got != tt.want {
			t.Errorf("Quantile(%v) = %v, want %v", tt.q, got, tt.want)
		}
	}
	if got := r.Max(); got != ms(100) {
		t.Errorf("Max = %v, want 100ms", got)
	}
}

func TestQuantileEmpty(t *testing.T) {
	if got := newRecorder().Quantile(0.95); got != 0 {
		t.Fatalf("empty quantile = %v, want 0", got)
	}
}

func TestFailureClasses(t *testing.T) {
	r := newRecorder()
	r.Observe(ms(5))
	r.Fail(0, "")
	r.Fail(502, "")
	r.Fail(503, "")
	r.Fail(429, "rate_limited")
	r.Fail(429, "agent_busy")
	r.Fail(404, "not_found")
	if got := r.ServerErrors(); got != 3 {
		t.Errorf("ServerErrors = %d, want 3 (transport errors count)", got)
	}
	if got := r.TooMany(); got != 2 {
		t.Errorf("TooMany = %d, want 2", got)
	}
	if got := r.OtherClientErrors(); got != 1 {
		t.Errorf("OtherClientErrors = %d, want 1", got)
	}
	if got := r.Total(); got != 7 {
		t.Errorf("Total = %d, want 7", got)
	}
	if got := r.Codes()["agent_busy"]; got != 1 {
		t.Errorf("agent_busy = %d, want 1", got)
	}
}

func TestMerge(t *testing.T) {
	a, b := newRecorder(), newRecorder()
	a.Observe(ms(1))
	b.Observe(ms(3))
	b.Fail(500, "internal")
	m := merge(a, b)
	if m.Count() != 2 || m.ServerErrors() != 1 || m.Quantile(1) != ms(3) {
		t.Fatalf("merge = count %d, server errors %d, max %v", m.Count(), m.ServerErrors(), m.Quantile(1))
	}
	if a.Count() != 1 {
		t.Fatalf("merge changed its input")
	}
}

func TestFanOut(t *testing.T) {
	t0 := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	writes := []written{{id: "a", at: t0}, {id: "b", at: t0.Add(time.Second)}}
	got := delivery{arrived: map[string]time.Time{}, opened: true}
	got.arrive("a", t0.Add(40*time.Millisecond))
	got.arrive("a", t0.Add(60*time.Millisecond))
	got.arrive("b", t0.Add(time.Second).Add(-5*time.Millisecond))
	missing := delivery{arrived: map[string]time.Time{}, opened: true}
	missing.arrive("a", t0.Add(100*time.Millisecond))
	refused := delivery{arrived: map[string]time.Time{}}

	delays, lost := fanOut([]delivery{got, missing, refused}, writes)
	if lost != 1 {
		t.Errorf("lost = %d, want 1 (stream 2 never saw b; the refused stream expects nothing)", lost)
	}
	want := []time.Duration{ms(40), 0, ms(100)}
	if len(delays) != len(want) {
		t.Fatalf("delays = %v, want %v", delays, want)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Errorf("delays[%d] = %v, want %v (a negative delay is clamped to 0)", i, delays[i], want[i])
		}
	}
	if got.dups != 1 {
		t.Errorf("dups = %d, want 1", got.dups)
	}
}

func TestParseActivity(t *testing.T) {
	typ, id := parseActivity(`{"seq":4,"type":"annotation.created","target":{"kind":"annotation","id":"abc"}}`)
	if typ != "annotation.created" || id != "abc" {
		t.Fatalf("got %q %q", typ, id)
	}
	if typ, id := parseActivity("not json"); typ != "" || id != "" {
		t.Fatalf("a non-JSON payload gave %q %q", typ, id)
	}
}

func TestErrorLogKeepsFewDistinctMessagesPerToolAndCode(t *testing.T) {
	l := newErrorLog()
	for _, msg := range []string{"a", "b", "a", "c", "d", "e"} {
		l.Add("list_metrics", "tool_error", msg)
	}
	l.Add("query_instant", "tool_error", "")
	got := l.Samples()
	if len(got) != 2 || got[0].Tool != "list_metrics" || got[1].Tool != "query_instant" {
		t.Fatalf("samples must be one per tool and code, sorted: %+v", got)
	}
	if got[0].Count != 6 {
		t.Errorf("count = %d, want 6", got[0].Count)
	}
	if want := []string{"a", "b", "c"}; !slices.Equal(got[0].Messages, want) {
		t.Errorf("messages = %v, want %v", got[0].Messages, want)
	}
	if len(got[1].Messages) != 0 {
		t.Errorf("an empty message must not be kept: %v", got[1].Messages)
	}
}
