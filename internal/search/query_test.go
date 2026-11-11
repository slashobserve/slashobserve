package search

import (
	"errors"
	"strings"
	"testing"
)

func TestParseQuery(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantWords  string
		wantPrefix string
	}{
		{"a word being typed is a prefix", "chec", "chec", "chec"},
		{"the last of several words is the prefix", "orders chec", "orders chec", "chec"},
		{"a finished word has no prefix", "checkout ", "checkout", ""},
		{"an open quote has no prefix", `"chec`, `"chec`, ""},
		{"an open quote after words has no prefix", `"order chec`, `"order chec`, ""},
		{"a closed phrase has no prefix", `"order checkout"`, `"order checkout"`, ""},
		{"a word after a closed phrase keeps the prefix", `"order checkout" err`, `"order checkout" err`, "err"},
		{"an excluded last word has no prefix", "checkout -chec", "checkout -chec", ""},
		{"an exclusion before the last word keeps the prefix", "-spam chec", "-spam chec", "chec"},
		{"one letter is too short for a prefix", "a", "a", ""},
		{"surrounding space is trimmed", "  checkout errors", "checkout errors", "errors"},
		{"punctuation ends the prefix run", "http_requests_to", "http_requests_to", "to"},
		{"a word ending in punctuation has no prefix run", "checkout!", "checkout!", ""},
		{"digits count as word characters", "pod 42", "pod 42", "42"},
		{"letters of other scripts count", "café", "café", "café"},
		{"an exclusion first leaves the real word a prefix", "-spam checkout", "-spam checkout", "checkout"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseQuery(tt.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.Words != tt.wantWords || got.Prefix != tt.wantPrefix {
				t.Fatalf("ParseQuery(%q) = %+v, want words %q prefix %q", tt.raw, got, tt.wantWords, tt.wantPrefix)
			}
		})
	}
}

func TestParseQueryRefusesWhatCannotBeSearched(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", "q must be 1 to 200 characters"},
		{"only spaces", "   ", "q must be 1 to 200 characters"},
		{"too long", strings.Repeat("a", 201), "q must be 1 to 200 characters"},
		{"too long in runes, not bytes", strings.Repeat("é", 201), "q must be 1 to 200 characters"},
		{"a control character", "check\x00out", "q must not contain control characters"},
		{"a tab inside", "check\tout", "q must not contain control characters"},
		{"no letters or digits", "--", "q needs a word to search for"},
		{"only exclusions", "-spam -eggs", "q needs a word to search for"},
		{"only punctuation", `"!?"`, "q needs a word to search for"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseQuery(tt.raw)
			var invalid *InvalidQueryError
			if !errors.As(err, &invalid) || invalid.Message != tt.want {
				t.Fatalf("ParseQuery(%q) error = %v, want %q", tt.raw, err, tt.want)
			}
		})
	}
}

func TestParseQueryAcceptsTheLongestQuery(t *testing.T) {
	if _, err := ParseQuery(strings.Repeat("a", 200)); err != nil {
		t.Fatal(err)
	}
}
