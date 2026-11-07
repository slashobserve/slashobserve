package elasticsearch_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const validatePath = "/logs-*/_validate/query"

func lintServer(t *testing.T, validateReply string) (*fakeServer, func(string) []datasource.Diagnostic) {
	t.Helper()
	f := healthyServer(t)
	f.fixed(http.MethodPost, validatePath, http.StatusOK, validateReply)
	ds := serve(t, f)
	return f, func(text string) []datasource.Diagnostic {
		t.Helper()
		diags, err := ds.Lint(context.Background(), text)
		if err != nil {
			t.Fatalf("Lint(%q): %v", text, err)
		}
		return diags
	}
}

const (
	valid   = `{"valid":true}`
	invalid = `{"valid":false,"explanations":[{"index":"logs-shop-2026.11","valid":false,"error":"org.opensearch.common.ParsingException: Cannot parse 'level:[': Encountered \"<EOF>\"\n\tat org.apache.lucene.queryparser.classic.QueryParserBase.parse(QueryParserBase.java:114)"}]}`
)

func TestLintKQL(t *testing.T) {
	_, lint := lintServer(t, valid)
	for _, tc := range []struct {
		name, text string
		want       datasource.Diagnostic
	}{
		{"unterminated group", "service.name:(a or", datasource.Diagnostic{Severity: datasource.SeverityError, Message: `expected a value after "or"`, Start: 16, End: 18}},
		{"unknown field is a warning", "nosuch:1", datasource.Diagnostic{Severity: datasource.SeverityWarning, Message: `unknown field "nosuch"`, Start: 0, End: 6}},
		{"offsets include the kql prefix", "kql: service.name:a and nosuch:1", datasource.Diagnostic{Severity: datasource.SeverityWarning, Message: `unknown field "nosuch"`, Start: 24, End: 30}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diags := lint(tc.text)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %+v, want one", diags)
			}
			got := diags[0]
			if got.Severity != tc.want.Severity || got.Start != tc.want.Start || got.End != tc.want.End || (tc.want.Message != "" && got.Message != tc.want.Message) || got.Message == "" {
				t.Fatalf("diagnostic = %+v, want %+v", got, tc.want)
			}
		})
	}
	for _, text := range []string{"", "service.name:checkout and log.level:error", "message:pay*"} {
		if diags := lint(text); len(diags) != 0 {
			t.Errorf("%q: diagnostics = %+v, want none", text, diags)
		}
	}
}

func TestLintLuceneUsesTheValidateEndpoint(t *testing.T) {
	f, lint := lintServer(t, invalid)
	diags := lint("lucene:level:[")
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %+v", diags)
	}
	got := diags[0]
	if got.Severity != datasource.SeverityError || got.Start != 7 || got.End != 14 {
		t.Fatalf("diagnostic = %+v, want an error over 7..14", got)
	}
	if strings.Contains(got.Message, "\n") || len(got.Message) > 200 || !strings.HasPrefix(got.Message, "org.opensearch.common.ParsingException") {
		t.Fatalf("message = %q, want the first line cut to 200 bytes", got.Message)
	}
	req := f.last(t)
	if req.params.Get("explain") != "true" {
		t.Fatalf("params = %v", req.params)
	}
	assertJSONEqual(t, req.body, `{"query":{"query_string":{"query":"level:[","analyze_wildcard":true}}}`)
	if f.count(searchPath) != 0 {
		t.Fatal("lint ran a search")
	}
}

func TestLintLuceneAcceptsValidQueriesAndEmptyBodies(t *testing.T) {
	f, lint := lintServer(t, valid)
	if diags := lint("lucene:level:error"); len(diags) != 0 {
		t.Fatalf("diagnostics = %+v", diags)
	}
	n := f.count(validatePath)
	if diags := lint("lucene:  "); len(diags) != 0 || f.count(validatePath) != n {
		t.Fatalf("an empty body should not be validated: %+v", diags)
	}
}

func TestLintDSL(t *testing.T) {
	f, lint := lintServer(t, valid)
	for _, tc := range []struct {
		name, text string
		start, end int
		contains   string
	}{
		{"unknown top key", `{"size": 1, "foo": 2}`, 12, 17, `unsupported key "foo"`},
		{"unknown key after nested objects", `{"query": {"a": {"foo": 1}}, "bar": 2}`, 29, 34, `unsupported key "bar"`},
		{"syntax error", `{"size": }`, 9, 10, "not valid JSON"},
		{"unterminated", `{"size": 1`, 9, 10, "not valid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diags := lint(tc.text)
			if len(diags) != 1 || diags[0].Severity != datasource.SeverityError || diags[0].Start != tc.start || diags[0].End != tc.end || !strings.Contains(diags[0].Message, tc.contains) {
				t.Fatalf("diagnostics = %+v, want one error over %d..%d containing %q", diags, tc.start, tc.end, tc.contains)
			}
		})
	}
	if diags := lint(`{"size": 1, "aggs": {}}`); len(diags) != 0 {
		t.Fatalf("a body without a query was flagged: %+v", diags)
	}
	if f.count(validatePath) != 0 {
		t.Fatal("a body without a query reached the validate endpoint")
	}
}

func TestLintDSLValidatesTheQueryPart(t *testing.T) {
	f, lint := lintServer(t, invalid)
	text := `{"query": {"match": {"a": "b"}}}`
	diags := lint(text)
	if len(diags) != 1 || diags[0].Severity != datasource.SeverityError || diags[0].Start != 0 || diags[0].End != len(text) {
		t.Fatalf("diagnostics = %+v", diags)
	}
	assertJSONEqual(t, f.last(t).body, `{"query":{"match":{"a":"b"}}}`)
	if f.count(searchPath) != 0 {
		t.Fatal("lint ran a search")
	}
}

func TestLintReportsMacroProblemsAndWarnsAroundVariables(t *testing.T) {
	_, lint := lintServer(t, invalid)
	diags := lint("$__nosuch")
	if len(diags) != 1 || diags[0].Severity != datasource.SeverityError || !strings.Contains(diags[0].Message, "unknown macro") {
		t.Fatalf("macro diagnostics = %+v", diags)
	}
	diags = lint("lucene:$lvl")
	if len(diags) != 1 || diags[0].Severity != datasource.SeverityWarning || !strings.HasPrefix(diags[0].Message, "could not lint around a variable: ") {
		t.Fatalf("variable diagnostics = %+v", diags)
	}
}

func TestVariableValuesFilteredByAQuery(t *testing.T) {
	for _, tc := range []struct {
		name, text, clause string
	}{
		{"kql", `label_values(service.name:checkout, log.level)`, `{"match":{"service.name":"checkout"}}`},
		{"lucene", `label_values(lucene:service.name:checkout, log.level)`, `{"query_string":{"query":"service.name:checkout","analyze_wildcard":true}}`},
		{"dsl", `label_values({"query": {"term": {"service.name": "checkout"}}}, log.level)`, `{"term":{"service.name":"checkout"}}`},
		{"a comma inside the query", `label_values(message:"a, b", log.level)`, `{"match_phrase":{"message":"a, b"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := healthyServer(t)
			f.fixed(http.MethodPost, searchPath, http.StatusOK, termsReply)
			ds := serve(t, f)
			values, err := ds.VariableValues(context.Background(), datasource.Query{Text: tc.text}, queryRange)
			if err != nil || len(values) == 0 {
				t.Fatalf("values = %v, err = %v", values, err)
			}
			body := searchBody(t, f.last(t))
			if termsField(t, f.last(t)) != "log.level" {
				t.Fatalf("terms field = %s", termsField(t, f.last(t)))
			}
			filter := body["query"].(map[string]any)["bool"].(map[string]any)["filter"].([]any)
			if len(filter) != 2 {
				t.Fatalf("filter = %v", filter)
			}
			assertJSONEqual(t, mustJSON(t, filter[1]), tc.clause)
		})
	}
}

func TestVariableValuesRejectsABadFilterQuery(t *testing.T) {
	f := healthyServer(t)
	ds := serve(t, f)
	for _, text := range []string{`label_values(service.name:(a or, log.level)`, `label_values({"foo": 1}, log.level)`} {
		if _, err := ds.VariableValues(context.Background(), datasource.Query{Text: text}, queryRange); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Errorf("%q: error = %v, want ErrInvalidQuery", text, err)
		}
	}
	if f.count(searchPath) != 0 {
		t.Fatal("a bad filter query reached the search endpoint")
	}
}

func TestLintReturnsUpstreamFailures(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, validatePath, http.StatusServiceUnavailable, esError("node_not_connected_exception", "down"))
	ds := serve(t, f)
	if _, err := ds.Lint(context.Background(), "lucene:a"); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}
