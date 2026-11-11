package search

import (
	"strings"
	"unicode"
)

const (
	matchStart      = '\x02'
	matchStop       = '\x03'
	maxSnippetRunes = 300
)

type Segment struct {
	Text  string `json:"text"`
	Match bool   `json:"match"`
}

func Segments(headline string) []Segment {
	segments := []Segment{}
	var text strings.Builder
	var runes int
	match := false
	flush := func() {
		if text.Len() == 0 {
			return
		}
		if last := len(segments) - 1; last >= 0 && segments[last].Match == match {
			segments[last].Text += text.String()
		} else {
			segments = append(segments, Segment{Text: text.String(), Match: match})
		}
		text.Reset()
	}
	space := false
	for _, r := range headline {
		switch {
		case r == matchStart:
			flush()
			match = true
			continue
		case r == matchStop:
			flush()
			match = false
			continue
		case unicode.IsSpace(r):
			if space {
				continue
			}
			r, space = ' ', true
		default:
			space = false
		}
		if runes == maxSnippetRunes {
			break
		}
		text.WriteRune(r)
		runes++
	}
	flush()
	return segments
}

func PlainText(segments []Segment) string {
	var b strings.Builder
	for _, s := range segments {
		b.WriteString(s.Text)
	}
	return strings.TrimSpace(b.String())
}
