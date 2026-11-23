package jaeger

import (
	"testing"
	"time"
)

func TestParseSearch(t *testing.T) {
	s, bad := parseSearch(`service=checkout operation="POST /pay" minDuration=500ms maxDuration=2s http.status_code=500 note="say \"hi\""`)
	if bad != nil {
		t.Fatalf("problem = %+v", bad)
	}
	if s.service != "checkout" || s.operation != "POST /pay" || s.minDuration != 500*time.Millisecond || s.maxDuration != 2*time.Second {
		t.Fatalf("search = %+v", s)
	}
	if s.tags["http.status_code"] != "500" || s.tags["note"] != `say "hi"` || len(s.tags) != 2 {
		t.Fatalf("tags = %v", s.tags)
	}
	attributes, err := s.attributesJSON()
	if err != nil || attributes != `{"http.status_code":"500","note":"say \"hi\""}` {
		t.Fatalf("attributes = %s, %v", attributes, err)
	}
}

func TestParseSearchAllowsAnEmptyQuery(t *testing.T) {
	s, bad := parseSearch("  \n ")
	if bad != nil || s.service != "" || len(s.tags) != 0 {
		t.Fatalf("search = %+v, problem = %+v", s, bad)
	}
}

func TestParseSearchProblems(t *testing.T) {
	cases := []struct {
		name, text, message string
		start, end          int
	}{
		{"bare word", `service=a checkout`, "expected key=value", 10, 18},
		{"missing key", `=a`, "expected key=value", 0, 2},
		{"missing value", `service= operation=x`, "service needs a value", 0, 8},
		{"empty quoted value", `service=""`, "service needs a value", 0, 10},
		{"unterminated quote", `service="a`, "unterminated quoted value", 8, 10},
		{"duplicate key", `service=a service=b`, "service is given twice", 10, 19},
		{"bad duration", `minDuration=fast`, "minDuration must be a duration", 12, 16},
		{"negative duration", `maxDuration=-1s`, "maxDuration must be a duration", 12, 15},
		{"tags without a service", `minDuration=1s http.status_code=500`, "tags need service=<name>", 15, 35},
		{"operation without a service", `operation=pay`, "operation needs service=<name>", 0, 13},
		{"inverted durations", `minDuration=2s maxDuration=1s`, "minDuration is above maxDuration", 0, 29},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, bad := parseSearch(tc.text)
			if bad == nil {
				t.Fatal("want a problem")
			}
			if got := bad.message; len(got) < len(tc.message) || got[:len(tc.message)] != tc.message || bad.start != tc.start || bad.end != tc.end {
				t.Fatalf("problem = %+v, want %q at %d-%d", bad, tc.message, tc.start, tc.end)
			}
		})
	}
}

func TestTraceIDOf(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{" 0AF7651916CD43DD8448EB211C80319C ", "0af7651916cd43dd8448eb211c80319c"},
		{`"b7ad6b7169203331"`, "b7ad6b7169203331"},
		{"`b7ad6b7169203331`", "b7ad6b7169203331"},
	} {
		if got, ok := traceIDOf(tc.text); !ok || got != tc.want {
			t.Errorf("traceIDOf(%q) = %q, %v", tc.text, got, ok)
		}
	}
	for _, text := range []string{"", "service=checkout", "not-hex", "b7ad"} {
		if id, ok := traceIDOf(text); ok {
			t.Errorf("traceIDOf(%q) = %q, want no id", text, id)
		}
	}
}
