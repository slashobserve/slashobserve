package annotations

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEventTitleIsOneShortLine(t *testing.T) {
	long := strings.Repeat("ü", 120)
	tests := []struct {
		name, text, want string
	}{
		{"short text stays", "deploy v2", "deploy v2"},
		{"lines and runs of spaces collapse", "  deploy\n\nv2   done ", "deploy v2 done"},
		{"nothing stays empty", " \n ", ""},
	}
	for _, tt := range tests {
		if got := eventTitle(tt.text); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
	got := eventTitle(long)
	if utf8.RuneCountInString(got) != maxEventTitleRunes || !strings.HasSuffix(got, "…") {
		t.Errorf("long text gave %d runes %q", utf8.RuneCountInString(got), got)
	}
}
