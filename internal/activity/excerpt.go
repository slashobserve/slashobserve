package activity

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ellipsis = "..."

var mentionToken = regexp.MustCompile(`@\[([^\]\n]*)\]\((?:user|grant|service_account):[0-9a-fA-F-]{36}\)`)

func plainMentions(text string) string {
	return mentionToken.ReplaceAllString(text, "@$1")
}

func Excerpt(text string, limit int) string {
	plain := strings.Join(strings.Fields(plainMentions(text)), " ")
	if utf8.RuneCountInString(plain) <= limit {
		return plain
	}
	cut := []rune(plain)[:limit]
	if i := lastSpace(cut); i > limit/2 {
		cut = cut[:i]
	}
	return strings.TrimRightFunc(string(cut), func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(",;:", r) }) + ellipsis
}

func lastSpace(runes []rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if unicode.IsSpace(runes[i]) {
			return i
		}
	}
	return -1
}
