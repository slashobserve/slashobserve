package spec

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	maxDatePatternBytes = 64
	isoMilliLayout      = "2006-01-02T15:04:05.000Z"
)

type dateToken struct {
	name   string
	render func(t time.Time) string
}

func padded(format string, value func(t time.Time) int) func(t time.Time) string {
	return func(t time.Time) string { return fmt.Sprintf(format, value(t)) }
}

func plain(value func(t time.Time) int) func(t time.Time) string {
	return func(t time.Time) string { return strconv.Itoa(value(t)) }
}

func constant(value string) func(t time.Time) string {
	return func(time.Time) string { return value }
}

func hour12(t time.Time) int {
	if h := t.Hour() % 12; h != 0 {
		return h
	}
	return 12
}

func meridiem(am, pm string) func(t time.Time) string {
	return func(t time.Time) string {
		if t.Hour() < 12 {
			return am
		}
		return pm
	}
}

var dateTokens = []dateToken{
	{"YYYY", padded("%04d", func(t time.Time) int { return t.Year() })},
	{"MMMM", func(t time.Time) string { return t.Month().String() }},
	{"dddd", func(t time.Time) string { return t.Weekday().String() }},
	{"MMM", func(t time.Time) string { return t.Month().String()[:3] }},
	{"ddd", func(t time.Time) string { return t.Weekday().String()[:3] }},
	{"SSS", padded("%03d", func(t time.Time) int { return t.Nanosecond() / int(time.Millisecond) })},
	{"YY", padded("%02d", func(t time.Time) int { return t.Year() % 100 })},
	{"MM", padded("%02d", func(t time.Time) int { return int(t.Month()) })},
	{"DD", padded("%02d", func(t time.Time) int { return t.Day() })},
	{"HH", padded("%02d", func(t time.Time) int { return t.Hour() })},
	{"hh", padded("%02d", hour12)},
	{"mm", padded("%02d", func(t time.Time) int { return t.Minute() })},
	{"ss", padded("%02d", func(t time.Time) int { return t.Second() })},
	{"ZZ", constant("+0000")},
	{"M", plain(func(t time.Time) int { return int(t.Month()) })},
	{"D", plain(func(t time.Time) int { return t.Day() })},
	{"H", plain(func(t time.Time) int { return t.Hour() })},
	{"h", plain(hour12)},
	{"m", plain(func(t time.Time) int { return t.Minute() })},
	{"s", plain(func(t time.Time) int { return t.Second() })},
	{"A", meridiem("AM", "PM")},
	{"a", meridiem("am", "pm")},
	{"X", func(t time.Time) string { return strconv.FormatInt(t.Unix(), 10) }},
	{"x", func(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }},
	{"Z", constant("+00:00")},
}

type datePart struct {
	token   *dateToken
	literal byte
}

func ValidTimeMacroFormat(format string) bool {
	switch format {
	case "date", "date:iso", "date:seconds":
		return true
	}
	pattern, ok := strings.CutPrefix(format, "date:")
	if !ok {
		return false
	}
	_, ok = parseDatePattern(pattern)
	return ok
}

func FormatTimeMacro(t time.Time, format string) (string, error) {
	t = t.UTC()
	switch format {
	case "":
		return strconv.FormatInt(t.UnixMilli(), 10), nil
	case "date", "date:iso":
		return t.Format(isoMilliLayout), nil
	case "date:seconds":
		return strconv.FormatInt(t.Unix(), 10), nil
	}
	if pattern, ok := strings.CutPrefix(format, "date:"); ok {
		if parts, ok := parseDatePattern(pattern); ok {
			return renderDate(t, parts), nil
		}
	}
	return "", fmt.Errorf("invalid date format %q", format)
}

func parseDatePattern(pattern string) ([]datePart, bool) {
	if pattern == "" || len(pattern) > maxDatePatternBytes {
		return nil, false
	}
	var parts []datePart
	for i := 0; i < len(pattern); {
		if token := matchDateToken(pattern[i:]); token != nil {
			parts = append(parts, datePart{token: token})
			i += len(token.name)
			continue
		}
		if !isDateLiteral(pattern[i]) {
			return nil, false
		}
		parts = append(parts, datePart{literal: pattern[i]})
		i++
	}
	return parts, true
}

func matchDateToken(s string) *dateToken {
	run := 1
	for run < len(s) && s[run] == s[0] {
		run++
	}
	for i := range dateTokens {
		if dateTokens[i].name == s[:run] {
			return &dateTokens[i]
		}
	}
	return nil
}

func isDateLiteral(b byte) bool {
	switch b {
	case '-', ':', '.', '_', '/', ',', ' ', 'T':
		return true
	}
	return b >= '0' && b <= '9'
}

func renderDate(t time.Time, parts []datePart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.token == nil {
			b.WriteByte(p.literal)
			continue
		}
		b.WriteString(p.token.render(t))
	}
	return b.String()
}
