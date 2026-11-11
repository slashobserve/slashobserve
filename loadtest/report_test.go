package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func fixedReport() report {
	return report{
		Label:   "servers2-workers8",
		Commit:  "abc1234",
		Target:  "http://localhost:48080",
		Started: time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC),
		Ended:   time.Date(2026, 11, 11, 10, 6, 30, 0, time.UTC),
		Sections: []section{
			{
				Title: "Viewers, shared range",
				Metrics: []metric{
					secondsMetric("server time p95", 0.12).under(300, "300 ms"),
					secondsMetric("server time p50", math.NaN()).under(300, "300 ms"),
					durationMetric("client p99", 1500*time.Millisecond),
					durationMetric("client max", 12*time.Second),
					percentMetric("cache hit share", 87.5),
					bytesMetric("memory per stream", 20480),
					countMetric("429", 3),
				},
			},
		},
		Gates: []gate{
			{Name: "no 5xx or transport errors in viewers", Pass: true, Detail: "0 of 9000 requests"},
			{Name: "every build succeeds", Pass: false, Detail: "1 of 20 failed"},
		},
	}
}

const wantMarkdown = `# Load test servers2-workers8

Target: http://localhost:48080

Commit: abc1234

Started: 2026-11-11T10:00:00Z

Ended: 2026-11-11T10:06:30Z

## Viewers, shared range

| Measure | Value | Page target | Result |
| --- | --- | --- | --- |
| server time p95 | 120 ms | < 300 ms | met |
| server time p50 | n/a | < 300 ms | no data |
| client p99 | 1500 ms |  |  |
| client max | 12.0 s |  |  |
| cache hit share | 87.5% |  |  |
| memory per stream | 20.0 KiB |  |  |
| 429 | 3 |  |  |

## Gates

| Gate | Result | Detail |
| --- | --- | --- |
| no 5xx or transport errors in viewers | pass | 0 of 9000 requests |
| every build succeeds | FAIL | 1 of 20 failed |
`

func TestMarkdown(t *testing.T) {
	if got := fixedReport().markdown(); got != wantMarkdown {
		t.Fatalf("markdown differs\n--- got ---\n%s\n--- want ---\n%s", got, wantMarkdown)
	}
}

func TestFailedFollowsGates(t *testing.T) {
	r := fixedReport()
	if !r.failed() {
		t.Fatal("a failing gate must fail the report")
	}
	r.Gates = r.Gates[:1]
	if r.failed() {
		t.Fatal("only passing gates must pass the report")
	}
}

func TestMetricResult(t *testing.T) {
	tests := []struct {
		name   string
		metric metric
		want   string
	}{
		{"below the limit", secondsMetric("x", 0.299).under(300, "300 ms"), "met"},
		{"at the limit", secondsMetric("x", 0.3).under(300, "300 ms"), "missed"},
		{"no observations", secondsMetric("x", math.NaN()).under(300, "300 ms"), "no data"},
	}
	for _, tt := range tests {
		if tt.metric.Result != tt.want {
			t.Errorf("%s: result %q, want %q", tt.name, tt.metric.Result, tt.want)
		}
	}
}

func TestJSONHoldsNumbersAndNoNaN(t *testing.T) {
	raw, err := json.Marshal(fixedReport())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, "NaN") {
		t.Fatalf("report.json carries NaN: %s", text)
	}
	if !strings.Contains(text, `"value":null`) {
		t.Fatalf("a missing measurement must be null: %s", text)
	}
}

func TestOutcomeGates(t *testing.T) {
	rec := newRecorder()
	for i := 0; i < 2000; i++ {
		rec.Observe(time.Millisecond)
	}
	rec.Fail(404, "not_found")
	gates := outcomeGates("viewers", rec)
	if !gates[0].Pass || !gates[1].Pass {
		t.Fatalf("1 of 2001 is under 0.1%%: %+v", gates)
	}
	for i := 0; i < 3; i++ {
		rec.Fail(404, "not_found")
	}
	rec.Fail(502, "")
	gates = outcomeGates("viewers", rec)
	if gates[0].Pass || gates[1].Pass {
		t.Fatalf("a 502 and 4 of 2005 4xx must fail both gates: %+v", gates)
	}
}

func TestRedact(t *testing.T) {
	got := redact("Authorization failed for sok_AbC-123_x and sok_zzz")
	if strings.Contains(got, "AbC") || strings.Contains(got, "zzz") {
		t.Fatalf("redact left a key: %s", got)
	}
}

func TestMarkdownListsErrorSamples(t *testing.T) {
	r := fixedReport()
	r.Sections[0].Errors = []errorSample{{Tool: "list_metrics", Code: "tool_error", Count: 7, Messages: []string{"a|b", "c"}}}
	got := r.markdown()
	want := "| Tool | Error code | Count | Messages |\n| --- | --- | --- | --- |\n| list_metrics | tool_error | 7 | a/b; c |\n"
	if !strings.Contains(got, want) {
		t.Fatalf("markdown lacks the error table\n%s", got)
	}
	raw, err := json.Marshal(r)
	if err != nil || !strings.Contains(string(raw), `"errors":[{"tool":"list_metrics"`) {
		t.Fatalf("report.json lacks the errors: %s %v", raw, err)
	}
}
