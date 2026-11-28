package otlphttp

import "testing"

func TestTraceIDOf(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{" 0AF7651916CD43DD8448EB211C80319C ", "0af7651916cd43dd8448eb211c80319c"},
		{`"b7ad6b7169203331"`, "b7ad6b7169203331"},
		{"`b7ad6b7169203331`", "b7ad6b7169203331"},
	} {
		if got, ok := TraceIDOf(tc.text); !ok || got != tc.want {
			t.Errorf("TraceIDOf(%q) = %q, %v", tc.text, got, ok)
		}
	}
	for _, text := range []string{"", "service=checkout", "not-hex", "b7ad"} {
		if id, ok := TraceIDOf(text); ok {
			t.Errorf("TraceIDOf(%q) = %q, want no id", text, id)
		}
	}
}
