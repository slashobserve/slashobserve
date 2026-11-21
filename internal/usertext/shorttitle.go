package usertext

import (
	"regexp"
	"strings"
)

const (
	ShortTitleRunes    = 60
	minShortTitleRunes = 3
)

var observeMention = regexp.MustCompile(`(?i)(^|\s)@observe\b[:,]?`)

func ShortTitle(text string) string {
	var line string
	for l := range strings.Lines(text) {
		line = strings.Join(strings.Fields(observeMention.ReplaceAllString(l, " ")), " ")
		if line != "" {
			break
		}
	}
	runes := []rune(line)
	switch {
	case len(runes) < minShortTitleRunes:
		return ""
	case len(runes) <= ShortTitleRunes:
		return line
	}
	end := ShortTitleRunes
	if runes[end] != ' ' {
		for i := end - 1; i > end/2; i-- {
			if runes[i] == ' ' {
				end = i
				break
			}
		}
	}
	return strings.TrimRight(string(runes[:end]), " ,;:-") + "..."
}
