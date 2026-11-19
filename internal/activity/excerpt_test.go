package activity

import "testing"

func TestExcerpt(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"collapses whitespace", "  a\n\nb   c ", 20, "a b c"},
		{"shows a mention as @Name", "hi @[Ada Lovelace](user:7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f), look", 40, "hi @Ada Lovelace, look"},
		{"shows an agent mention as @Name", "@[Claude](grant:7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f) go", 40, "@Claude go"},
		{"keeps a short text whole", "short", 5, "short"},
		{"cuts at a word and adds an ellipsis", "the checkout latency doubled after the deploy", 30, "the checkout latency doubled..."},
		{"cuts a long word where it stands", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 10, "aaaaaaaaaa..."},
		{"counts runes, not bytes", "ééééééééééé", 5, "ééééé..."},
		{"is empty for nothing", " \n ", 10, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Excerpt(tc.in, tc.limit); got != tc.want {
				t.Fatalf("Excerpt(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
			}
		})
	}
}
