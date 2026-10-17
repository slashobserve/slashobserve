package auth

import "testing"

func TestHasControlChars(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"plain", "ci-bot", false},
		{"unicode letters", "café 日本", false},
		{"space", "a b", false},
		{"nul", "a\x00b", true},
		{"newline", "a\nb", true},
		{"tab", "a\tb", true},
		{"delete", "a\x7fb", true},
		{"c1 control", "a\u0085b", true},
		{"right-to-left override", "a\u202eb", true},
		{"zero width space", "a\u200bb", true},
		{"byte order mark", "\ufeffa", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasControlChars(tt.in); got != tt.want {
				t.Fatalf("hasControlChars(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
