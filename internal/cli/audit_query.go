package cli

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"
)

const (
	maxLineCells = 120
	leadCells    = 40
	ellipsis     = "..."
)

type queryError struct {
	*apiError
	query string
}

func (e queryError) Error() string {
	line, marks, ok := underline(e.query, *e.start, *e.end)
	if !ok {
		return e.apiError.Error()
	}
	return e.apiError.Error() + "\n" + line + "\n" + marks
}

func (e queryError) Unwrap() error { return e.apiError }

func (a *app) underlined(err error) error {
	var api *apiError
	if a.json || !errors.As(err, &api) || api.start == nil || api.end == nil {
		return err
	}
	return queryError{apiError: api, query: a.audit.query}
}

func underline(query string, start, end int) (line, marks string, ok bool) {
	end = min(max(end, start), len(query))
	if start < 0 || start > len(query) || !onRuneStart(query, start) || !onRuneStart(query, end) {
		return "", "", false
	}
	lineStart := strings.LastIndexByte(query[:start], '\n') + 1
	lineEnd := len(query)
	if i := strings.IndexByte(query[start:], '\n'); i >= 0 {
		lineEnd = start + i
	}
	end = min(end, lineEnd)
	shown := []rune(text(query[lineStart:lineEnd]))
	from := utf8.RuneCountInString(query[lineStart:start])
	to := utf8.RuneCountInString(query[lineStart:end])
	first, last := windowAround(shown, from)
	lead, trail := "", ""
	if first > 0 {
		lead = ellipsis
	}
	if last < len(shown) {
		trail = ellipsis
	}
	column := len(lead) + cells(string(shown[first:from]))
	span := cells(string(shown[from:max(min(to, last), from)]))
	return lead + string(shown[first:last]) + trail, strings.Repeat(" ", column) + strings.Repeat("^", max(span, 1)), true
}

func windowAround(shown []rune, at int) (first, last int) {
	if cells(string(shown)) <= maxLineCells {
		return 0, len(shown)
	}
	budget := maxLineCells - 2*len(ellipsis)
	first = at
	for first > 0 && cells(string(shown[first-1:at])) <= leadCells {
		first--
	}
	for first < at && cells(string(shown[first:first+1])) == 0 {
		first++
	}
	last = first
	for last < len(shown) && cells(string(shown[first:last+1])) <= budget {
		last++
	}
	for first > 0 && last == len(shown) && cells(string(shown[first-1:last])) <= budget {
		first--
	}
	return first, last
}

func onRuneStart(s string, i int) bool {
	return i == len(s) || utf8.RuneStart(s[i])
}

func cells(s string) int {
	n := 0
	for _, r := range s {
		if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf) {
			continue
		}
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			n += 2
		default:
			n++
		}
	}
	return n
}
