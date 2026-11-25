package grounding

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Figure struct {
	Text  string
	Value float64
	Step  float64
	Unit  string
}

type unit struct {
	written  []string
	name     string
	scales   []float64
	attached bool
}

var units = []unit{
	{written: []string{"milliseconds", "millisecond", "msec", "ms"}, name: "ms", scales: []float64{1, 1e-3, 1e3, 1e6}},
	{written: []string{"µs", "μs"}, name: "us", scales: []float64{1, 1e-3, 1e-6, 1e3}},
	{written: []string{"us"}, name: "us", scales: []float64{1, 1e-3, 1e-6, 1e3}, attached: true},
	{written: []string{"seconds", "second", "secs", "sec", "s"}, name: "s", scales: []float64{1, 1e3, 1e6, 1e9}},
	{written: []string{"percent", "%"}, name: "%", scales: []float64{1, 1e-2}},
	{written: []string{"KiB", "KB", "kB"}, name: "KB", scales: []float64{1e3, 1 << 10, 1}},
	{written: []string{"MiB", "MB"}, name: "MB", scales: []float64{1e6, 1 << 20, 1}},
	{written: []string{"GiB", "GB"}, name: "GB", scales: []float64{1e9, 1 << 30, 1}},
	{written: []string{"K", "k"}, name: "K", scales: []float64{1e3}, attached: true},
	{written: []string{"M"}, name: "M", scales: []float64{1e6}, attached: true},
	{written: []string{"x", "×"}, name: "x", scales: []float64{1}, attached: true},
}

var plainScales = []float64{1}

var (
	number = regexp.MustCompile(`\d{1,3}(?:,\d{3})+(?:\.\d+)?|\d+(?:\.\d+)?`)

	fencedCode = regexp.MustCompile("(?s)(```|~~~).*?(```|~~~|$)")
	inlineCode = regexp.MustCompile("`[^`]*`")
	mdLink     = regexp.MustCompile(`!?\[[^\]]*\]\([^)]*\)`)
	angleLink  = regexp.MustCompile(`<[a-z]+://[^>]*>`)
	bareURL    = regexp.MustCompile(`(?i)\bhttps?://\S*|(?:^|[\s(])/[ocd]/\S*`)
	listMarker = regexp.MustCompile(`(?m)^\s*\d+[.)]\s`)

	isoTime  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}(?:[T ]\d{1,2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?)?`)
	slashDay = regexp.MustCompile(`\b\d{1,2}/\d{1,2}/\d{2,4}\b`)
	clock    = regexp.MustCompile(`(?i)\b\d{1,2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:\s?[ap]\.?m\.?\b)?`)
	months   = `(?:jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec)[a-z]*\.?`
	monthDay = regexp.MustCompile(`(?i)\b` + months + `\s+\d{1,2}(?:st|nd|rd|th)?(?:,?\s+\d{4})?\b|\b\d{1,2}(?:st|nd|rd|th)?\s+` + months + `(?:,?\s+\d{4})?\b`)
	duration = regexp.MustCompile(`(?i:\b\d+(?:\.\d+)?\s?(?:minutes?|mins?|hours?|hrs?|days?|weeks?|months?|years?)\b)|\b\d+(?:\.\d+)?[mhdw]\b`)
)

var spansToSkip = []*regexp.Regexp{fencedCode, inlineCode, mdLink, angleLink, bareURL, listMarker, isoTime, slashDay, monthDay, clock, duration}

func blank(text string) string {
	for _, re := range spansToSkip {
		text = re.ReplaceAllStringFunc(text, func(s string) string { return strings.Repeat(" ", len(s)) })
	}
	return text
}

func Figures(text string) []Figure {
	var out []Figure
	for _, f := range scan(blank(text)) {
		if f.Unit == "" && f.Step >= 1 && f.Value <= 10 {
			continue
		}
		out = append(out, f)
	}
	return out
}

func valuesIn(text string) []float64 {
	var out []float64
	for _, f := range scan(blank(text)) {
		scale := 1.0
		switch f.Unit {
		case "K":
			scale = 1e3
		case "M":
			scale = 1e6
		}
		out = append(out, f.Value*scale)
	}
	return out
}

func scan(text string) []Figure {
	var out []Figure
	for _, loc := range number.FindAllStringIndex(text, -1) {
		start, end := loc[0], loc[1]
		if !standsAlone(text, start) {
			continue
		}
		written := text[start:end]
		u, after, ok := unitAt(text, end)
		if !ok {
			continue
		}
		value, step, ok := parseNumber(written)
		if !ok {
			continue
		}
		out = append(out, Figure{Text: strings.TrimSpace(text[start:after]), Value: value, Step: step, Unit: u.name})
	}
	return out
}

func standsAlone(text string, start int) bool {
	if start == 0 {
		return true
	}
	prev, size := utf8.DecodeLastRuneInString(text[:start])
	switch {
	case isWord(prev), prev == '#', prev == '.' && start-size > 0 && digitBefore(text, start-size):
		return false
	case prev == '-' || prev == '/':
		before, _ := utf8.DecodeLastRuneInString(text[:start-size])
		return start-size == 0 || !isWord(before)
	}
	return true
}

func digitBefore(text string, i int) bool {
	r, _ := utf8.DecodeLastRuneInString(text[:i])
	return unicode.IsDigit(r)
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func unitAt(text string, end int) (unit, int, bool) {
	rest := text[end:]
	if strings.HasPrefix(rest, ".") && len(rest) > 1 && rest[1] >= '0' && rest[1] <= '9' {
		return unit{}, 0, false
	}
	spaced := strings.TrimPrefix(rest, " ")
	gap := len(rest) - len(spaced)
	for _, u := range units {
		if u.attached && gap > 0 {
			continue
		}
		for _, w := range u.written {
			if strings.HasPrefix(spaced, w) && wordEnds(spaced[len(w):]) {
				return u, end + gap + len(w), true
			}
		}
	}
	if gap == 0 {
		next, _ := utf8.DecodeRuneInString(rest)
		if isWord(next) {
			return unit{}, 0, false
		}
	}
	return unit{scales: plainScales}, end, true
}

func wordEnds(rest string) bool {
	if rest == "" {
		return true
	}
	next, _ := utf8.DecodeRuneInString(rest)
	return !isWord(next)
}

func parseNumber(written string) (value, step float64, ok bool) {
	plain := strings.ReplaceAll(written, ",", "")
	value, err := strconv.ParseFloat(plain, 64)
	if err != nil {
		return 0, 0, false
	}
	if dot := strings.IndexByte(plain, '.'); dot >= 0 {
		return value, math.Pow10(-(len(plain) - dot - 1)), true
	}
	zeros := len(plain) - len(strings.TrimRight(plain, "0"))
	if zeros >= 2 && zeros < len(plain) {
		return value, math.Pow10(zeros), true
	}
	return value, 1, true
}

func scalesOf(name string) []float64 {
	for _, u := range units {
		if u.name == name {
			return u.scales
		}
	}
	return plainScales
}
