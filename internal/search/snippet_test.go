package search

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSegmentsSplitOnTheMarkers(t *testing.T) {
	cases := []struct {
		name     string
		headline string
		want     []Segment
	}{
		{"a match in the middle", "the \x02checkout\x03 errors", []Segment{{"the ", false}, {"checkout", true}, {" errors", false}}},
		{"a match at each end", "\x02a\x03 b \x02c\x03", []Segment{{"a", true}, {" b ", false}, {"c", true}}},
		{"adjacent matches merge", "\x02a\x03\x02b\x03", []Segment{{"ab", true}}},
		{"no match is one plain segment", "just text", []Segment{{"just text", false}}},
		{"empty text has no segments", "", []Segment{}},
		{"markers alone have no segments", "\x02\x03", []Segment{}},
		{"whitespace runs collapse to one space", "a\n\n  b\t\x02c\x03", []Segment{{"a b ", false}, {"c", true}}},
		{"an unclosed match runs to the end", "x \x02open", []Segment{{"x ", false}, {"open", true}}},
		{"fragments keep their delimiter", "\x02a\x03 ... \x02b\x03", []Segment{{"a", true}, {" ... ", false}, {"b", true}}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := Segments(tt.headline)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Segments(%q) = %v, want %v", tt.headline, got, tt.want)
			}
		})
	}
}

func TestSegmentsAreCappedAtARuneBoundary(t *testing.T) {
	word := strings.Repeat("é", 200)
	got := Segments(word + "\x02" + word + "\x03")
	total := 0
	for _, s := range got {
		if !utf8.ValidString(s.Text) {
			t.Fatalf("segment %q is not valid UTF-8", s.Text)
		}
		total += utf8.RuneCountInString(s.Text)
	}
	if total != 300 {
		t.Fatalf("total runes = %d, want 300", total)
	}
	if len(got) != 2 || got[1].Match != true {
		t.Fatalf("segments = %d, want the plain part then the cut match", len(got))
	}
}

func TestPlainTextJoinsTheSegments(t *testing.T) {
	got := PlainText([]Segment{{" the ", false}, {"checkout", true}, {" errors ", false}})
	if got != "the checkout errors" {
		t.Fatalf("PlainText = %q", got)
	}
	if PlainText(nil) != "" {
		t.Fatal("PlainText of nothing is not empty")
	}
}
