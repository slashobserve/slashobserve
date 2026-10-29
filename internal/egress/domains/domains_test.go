package domains

import (
	"errors"
	"strings"
	"testing"
)

func TestParsePatternAccepts(t *testing.T) {
	cases := map[string]string{
		"example.com":                    "example.com",
		"Example.COM.":                   "example.com",
		"  example.com  ":                "example.com",
		"*.Example.com":                  "*.example.com",
		"*.co.uk":                        "*.co.uk",
		"bücher.example":                 "xn--bcher-kva.example",
		"*.bücher.example":               "*.xn--bcher-kva.example",
		"feed-1.example.org":             "feed-1.example.org",
		"a.b.c.example.net.":             "a.b.c.example.net",
		strings.Repeat("a", 63) + ".com": strings.Repeat("a", 63) + ".com",
	}
	for entry, want := range cases {
		p, err := ParsePattern(entry)
		if err != nil {
			t.Errorf("ParsePattern(%q) error: %v", entry, err)
			continue
		}
		if got := p.String(); got != want {
			t.Errorf("ParsePattern(%q).String() = %q, want %q", entry, got, want)
		}
	}
}

func TestParsePatternRefuses(t *testing.T) {
	longName := strings.Repeat(strings.Repeat("a", 63)+".", 3) + strings.Repeat("a", 58) + ".com"
	refused := []string{
		"",
		"localhost",
		"com",
		"*.com",
		"*",
		"**.example.com",
		"a.*.example.com",
		"https://example.com",
		"example.com:443",
		"example.com/feed",
		"user@example.com",
		"example.com?x=1",
		"example.com#frag",
		"10.0.0.1",
		"[::1]",
		"::1",
		"exa mple.com",
		"exa\tmple.com",
		"-a.example.com",
		"a-.example.com",
		"a..example.com",
		"under_score.example.com",
		strings.Repeat("a", 64) + ".example.com",
		longName,
	}
	if len(longName) != 254 {
		t.Fatalf("test name is %d bytes, want 254", len(longName))
	}
	for _, entry := range refused {
		if _, err := ParsePattern(entry); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParsePattern(%q) error = %v, want ErrInvalid", entry, err)
		}
	}
}

func TestPatternMatches(t *testing.T) {
	cases := []struct {
		pattern string
		host    string
		want    bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "a.example.com", false},
		{"example.com", "notexample.com", false},
		{"*.example.com", "a.example.com", true},
		{"*.example.com", "a.b.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.example.com", "notexample.com", false},
		{"*.example.com", "xexample.com", false},
		{"*.co.uk", "feeds.co.uk", true},
	}
	for _, c := range cases {
		p, err := ParsePattern(c.pattern)
		if err != nil {
			t.Fatalf("ParsePattern(%q): %v", c.pattern, err)
		}
		if got := p.Matches(c.host); got != c.want {
			t.Errorf("%q.Matches(%q) = %v, want %v", c.pattern, c.host, got, c.want)
		}
	}
}

func TestListAllows(t *testing.T) {
	var list List
	for _, entry := range []string{"example.com", "*.example.com", "news.org"} {
		p, err := ParsePattern(entry)
		if err != nil {
			t.Fatal(err)
		}
		list = append(list, p)
	}
	cases := map[string]bool{
		"example.com":    true,
		"a.example.com":  true,
		"news.org":       true,
		"a.news.org":     false,
		"other.com":      false,
		"notexample.com": false,
		"":               false,
	}
	for host, want := range cases {
		if got := list.Allows(host); got != want {
			t.Errorf("Allows(%q) = %v, want %v", host, got, want)
		}
	}
	if List(nil).Allows("example.com") {
		t.Error("empty list allowed a host")
	}
}

func TestNormalizeHost(t *testing.T) {
	good := map[string]string{
		"Feed.Example.COM.": "feed.example.com",
		"bücher.example":    "xn--bcher-kva.example",
	}
	for in, want := range good {
		got, err := NormalizeHost(in)
		if err != nil || got != want {
			t.Errorf("NormalizeHost(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "127.0.0.1", "::1", "[::1]", "localhost", "a b.example", "example.com:443", "*.example.com"} {
		if _, err := NormalizeHost(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("NormalizeHost(%q) error = %v, want ErrInvalid", in, err)
		}
	}
}
