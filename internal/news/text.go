package news

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const ellipsis = "…"

func plainText(s string, limit int) string {
	var b strings.Builder
	skipping := 0
	z := html.NewTokenizer(strings.NewReader(s))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return truncate(collapse(b.String()), limit)
		case html.TextToken:
			if skipping == 0 {
				b.Write(z.Text())
			}
		case html.StartTagToken:
			tag := tagName(z)
			if isHidden(tag) {
				skipping++
			}
			if isBoundary(tag) {
				b.WriteByte(' ')
			}
		case html.SelfClosingTagToken:
			if isBoundary(tagName(z)) {
				b.WriteByte(' ')
			}
		case html.EndTagToken:
			tag := tagName(z)
			if isHidden(tag) && skipping > 0 {
				skipping--
			}
			if isBoundary(tag) {
				b.WriteByte(' ')
			}
		}
	}
}

func tagName(z *html.Tokenizer) string {
	name, _ := z.TagName()
	return string(name)
}

func isHidden(tag string) bool {
	return tag == "script" || tag == "style" || tag == "iframe"
}

func isBoundary(tag string) bool {
	return tag == "br" || tag == "p" || tag == "li" || tag == "div"
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimRight(string(runes[:limit-1]), " ") + ellipsis
}
