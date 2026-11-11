package events

import (
	"strings"
	"testing"
)

func TestNewAuthorCapsTheNameInRunes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"short name", "Ada", "Ada"},
		{"empty name", "", ""},
		{"exactly the cap", strings.Repeat("a", 256), strings.Repeat("a", 256)},
		{"one over the cap", strings.Repeat("a", 257), strings.Repeat("a", 256)},
		{"multi-byte runes are cut whole", strings.Repeat("é", 300), strings.Repeat("é", 256)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NewAuthor("user", "id-1", tc.in)
			if got.Name != tc.want || got.Kind != "user" || got.ID != "id-1" {
				t.Fatalf("NewAuthor = %+v, want name of %d runes", got, len([]rune(tc.want)))
			}
		})
	}
}
