package elasticsearch

import "strings"

type Form string

const (
	FormKQL    Form = "kql"
	FormLucene Form = "lucene"
	FormDSL    Form = "dsl"

	whitespace = " \t\r\n"
)

func Classify(text, defaultLanguage string) (Form, string) {
	form, body, _ := classify(text, defaultLanguage)
	return form, body
}

func classify(text, defaultLanguage string) (form Form, body string, offset int) {
	offset = len(text) - len(strings.TrimLeft(text, whitespace))
	rest := text[offset:]
	form = FormKQL
	if defaultLanguage == QueryLanguageLucene {
		form = FormLucene
	}
	switch {
	case strings.HasPrefix(rest, "{"):
		form = FormDSL
	case hasPrefixFold(rest, "lucene:"):
		form, offset, rest = FormLucene, offset+len("lucene:"), rest[len("lucene:"):]
	case hasPrefixFold(rest, "kql:"):
		form, offset, rest = FormKQL, offset+len("kql:"), rest[len("kql:"):]
	}
	skipped := len(rest) - len(strings.TrimLeft(rest, whitespace))
	return form, strings.TrimSpace(rest), offset + skipped
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}
