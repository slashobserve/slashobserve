package revisions

import (
	"strings"
	"testing"
)

func TestCleanContinueNote(t *testing.T) {
	ptr := func(s string) *string { return &s }
	cases := []struct {
		name string
		note *string
		want string
		ok   bool
	}{
		{"absent note is no note", nil, "", true},
		{"blank note is invalid", ptr("  \n\t "), "", false},
		{"empty note is invalid", ptr(""), "", false},
		{"surrounding space is trimmed", ptr("  use the pod label \n"), "use the pod label", true},
		{"4000 characters pass", ptr(strings.Repeat("a", 4000)), strings.Repeat("a", 4000), true},
		{"4001 characters fail", ptr(strings.Repeat("a", 4001)), "", false},
		{"4000 astral characters pass", ptr(strings.Repeat("\U0001F600", 4000)), strings.Repeat("\U0001F600", 4000), true},
		{"a control character fails", ptr("keep\x07going"), "", false},
		{"a bidirectional override fails", ptr("keep\u202egoing"), "", false},
		{"newlines and tabs are allowed", ptr("a\n\tb"), "a\n\tb", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := CleanContinueNote(c.note)
			if got != c.want || ok != c.ok {
				t.Fatalf("CleanContinueNote = (%q, %v), want (%q, %v)", got, ok, c.want, c.ok)
			}
		})
	}
}
