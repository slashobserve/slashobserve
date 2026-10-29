package news

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPlainText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"tags and entities", "<p>Hi&nbsp;<b>there</b></p><script>x()</script>", 500, "Hi there"},
		{"escaped ampersand", "Fish &amp; chips &lt;3", 500, "Fish & chips <3"},
		{"style and iframe contents dropped", "a<style>p{color:red}</style>b<iframe>inner</iframe>c", 500, "abc"},
		{"block boundaries become spaces", "one<br>two<br/>three<li>four</li><div>five</div>", 500, "one two three four five"},
		{"whitespace collapsed", "  a \n\t b   c  ", 500, "a b c"},
		{"attribute text is not kept", `<a href="https://x.example/?q=1" title="secret">link</a>`, 500, "link"},
		{"unclosed script drops the rest", "keep<script>drop", 500, "keep"},
		{"empty", "", 500, ""},
		{"under the cap", strings.Repeat("a", 300), 300, strings.Repeat("a", 300)},
		{"over the cap ends with an ellipsis", strings.Repeat("a", 400), 300, strings.Repeat("a", 299) + "…"},
		{"cap counts runes", strings.Repeat("é", 10), 5, strings.Repeat("é", 4) + "…"},
		{"trailing space before the ellipsis is trimmed", "abcd efgh", 6, "abcd…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := plainText(tc.in, tc.max)
			if got != tc.want {
				t.Errorf("plainText(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
			if utf8.RuneCountInString(got) > tc.max {
				t.Errorf("result has %d runes, cap %d", utf8.RuneCountInString(got), tc.max)
			}
		})
	}
}
