package cli

import (
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/width"
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
	column := cells(query[lineStart:start])
	return text(query[lineStart:lineEnd]), strings.Repeat(" ", column) + strings.Repeat("^", max(cells(query[start:end]), 1)), true
}

func onRuneStart(s string, i int) bool {
	return i == len(s) || utf8.RuneStart(s[i])
}

func cells(s string) int {
	n := 0
	for _, r := range s {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			n += 2
		default:
			n++
		}
	}
	return n
}
