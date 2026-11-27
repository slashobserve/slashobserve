package elasticsearch

import "github.com/slashobserve/slashobserve/internal/kql"

type Form = kql.Form

const (
	FormKQL    = kql.FormKQL
	FormLucene = kql.FormLucene
	FormDSL    = kql.FormDSL
)

func Classify(text, defaultLanguage string) (Form, string) {
	form, body, _ := classify(text, defaultLanguage)
	return form, body
}

func classify(text, defaultLanguage string) (Form, string, int) {
	plain := kql.FormKQL
	if defaultLanguage == QueryLanguageLucene {
		plain = kql.FormLucene
	}
	return kql.Classify(text, plain)
}
