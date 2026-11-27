package grounding

import (
	"slices"
	"testing"
)

func uncited(text, question string) []string {
	var out []string
	for _, f := range UncitedFigures(text, question) {
		out = append(out, f.Text)
	}
	return out
}

func TestAFigureNeedsAFactUnlessItIsATimeANameOrTheQuestionsOwn(t *testing.T) {
	cases := []struct {
		name, text, question string
		want                 []string
	}{
		{"a latency", "p99 rose to 523 ms.", "", []string{"523 ms"}},
		{"a count and a percent", "1,284 requests failed, 4.1% of all.", "", []string{"1,284", "4.1%"}},
		{"a cited fact", "p99 rose to {{f3:ms}} from {{f4}}.", "", nil},
		{"a rounded guess beside a fact", "p99 rose to {{f3}}, about 0.5 s.", "", []string{"0.5 s"}},
		{"times and dates", "Since 2026-11-20T10:05:00Z, Nov 20 at 9:30 am and 10:05 UTC.", "", nil},
		{"window and step durations", "Over the last 6 hours, at a 30s step, within 90 seconds, every 15 s, with a 60 s window.", "", nil},
		{"a latency in seconds", "Calls took 30 s at peak.", "", []string{"30 s"}},
		{"versions", "Version 2.4 and release 18 and v1.2.3 and 1.27.0.", "", nil},
		{"ports", "It listens on port 8080 and on cart:7070.", "", nil},
		{"status codes", "It returned HTTP 503 and status 500, 502 or 504, and code 429.", "", nil},
		{"plural status codes", "The proxy returned 503s.", "", nil},
		{"a count that looks like a status", "There were 503 errors.", "", []string{"503"}},
		{"percentile names", "p99 and p99.9 and the 95th percentile and quantile 0.99.", "", nil},
		{"list numbering and small counts", "1. First\n2. Second, on 3 pods across 7 services.", "", nil},
		{"top N", "The top 20 routes and the first 50 lines.", "", nil},
		{"a year", "Seen since 2025 and in 2026.", "", nil},
		{"a number from the question", "Of requests over 500 ms, 12 percent were slow at 250 ms.", "how many requests took over 500 ms, and how many over 250 ms?", []string{"12 percent"}},
		{"code and links", "See `rate(x[5m]) > 0.25` and [run 42](/o/0192) and https://example.com/a/123.", "", nil},
		{"trace and span ids", "Trace 4bf92f3577b34da6a3ce929d0e0e4736 and span 00f067aa0ba902b7 and 4bf92f35.", "", nil},
		{"identifiers", "Pod checkout-7f9c8d6b5-x2x on node ip-10-0-1-12 ran http2.", "", nil},
	}
	for _, c := range cases {
		if got := uncited(c.text, c.question); !slices.Equal(got, c.want) {
			t.Errorf("%s: UncitedFigures(%q) = %q, want %q", c.name, c.text, got, c.want)
		}
	}
}

func TestACutOrUnreadTraceIDIsAProblem(t *testing.T) {
	known := []string{"4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7"}
	cases := []struct {
		text string
		want []string
	}{
		{"Trace 4bf92f3577b34da6a3ce929d0e0e4736 and span 00F067AA0BA902B7.", nil},
		{"Trace `4bf92f3577b34da6a3ce929d0e0e4736`.", nil},
		{"Trace 4bf92f35 failed.", []string{"trace_id 4bf92f35"}},
		{"Trace 4bf92f3577b34da6… failed.", []string{"trace_id 4bf92f3577b34da6"}},
		{"Trace 0af7651916cd43dd8448eb211c80319c failed.", []string{"trace_id 0af7651916cd43dd8448eb211c80319c"}},
		{"Commit 9fceb02d and pod checkout-7f9c8d6b5-x2x and request 3f2a9b1c-5d4e-4f6a-8b7c-9d0e1f2a3b4c.", nil},
		{"Open [the trace](/trace/0af7651916cd43dd8448eb211c80319c).", nil},
	}
	for _, c := range cases {
		if got := texts(traceIDProblems(c.text, known)); !slices.Equal(got, c.want) {
			t.Errorf("traceIDProblems(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
