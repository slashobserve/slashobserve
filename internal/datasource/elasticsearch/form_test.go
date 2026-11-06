package elasticsearch_test

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		name, text, language string
		form                 elasticsearch.Form
		body                 string
	}{
		{"plain text uses the datasource language", "service.name:checkout", "kql", elasticsearch.FormKQL, "service.name:checkout"},
		{"lucene datasource default", "service.name:checkout", "lucene", elasticsearch.FormLucene, "service.name:checkout"},
		{"empty default is kql", "a:b", "", elasticsearch.FormKQL, "a:b"},
		{"lucene prefix", "lucene: level:error", "kql", elasticsearch.FormLucene, "level:error"},
		{"lucene prefix is case insensitive", "  LUCENE:level:error  ", "kql", elasticsearch.FormLucene, "level:error"},
		{"kql prefix on a lucene datasource", "kql: a and b", "lucene", elasticsearch.FormKQL, "a and b"},
		{"json object is the query dsl", ` {"query":{"match_all":{}}} `, "lucene", elasticsearch.FormDSL, `{"query":{"match_all":{}}}`},
		{"empty body keeps the form", "lucene:", "kql", elasticsearch.FormLucene, ""},
		{"blank text", "   ", "kql", elasticsearch.FormKQL, ""},
		{"a field that starts like a prefix is plain text", "lucene_ratio:1", "kql", elasticsearch.FormKQL, "lucene_ratio:1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form, body := elasticsearch.Classify(tc.text, tc.language)
			if form != tc.form || body != tc.body {
				t.Fatalf("Classify(%q, %q) = %q, %q; want %q, %q", tc.text, tc.language, form, body, tc.form, tc.body)
			}
		})
	}
}
