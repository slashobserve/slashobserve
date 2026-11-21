package usertext

import "testing"

func TestShortTitle(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"a short line stays", "why is p99 up?", "why is p99 up?"},
		{"only the first non-empty line", "\n  first   line \nsecond", "first line"},
		{"the agent mention goes", "@observe: why is p99 up?", "why is p99 up?"},
		{"too short for a title", "ok", ""},
		{"cut at a word", "why did checkout slow down after the deploy at 10:02, and did the cart service follow it?", "why did checkout slow down after the deploy at 10:02, and..."},
		{"cut at the limit when the next rune is a space", "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffff more", "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffff..."},
	}
	for _, c := range cases {
		if got := ShortTitle(c.text); got != c.want {
			t.Errorf("%s: ShortTitle(%q) = %q, want %q", c.name, c.text, got, c.want)
		}
	}
}
