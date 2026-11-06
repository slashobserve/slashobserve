package kql_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/kql"
)

var demoFields = kql.Fields{
	"service.name": kql.Keyword,
	"log.level":    kql.Keyword,
	"message":      kql.Text,
	"http.status":  kql.Number,
	"latency":      kql.Number,
	"host.name":    kql.Keyword,
	"trace.id":     kql.Keyword,
	"@timestamp":   kql.Date,
}

func TestCompileEmitsTheQueryDSLOfEachConstruct(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{"and with a value group", "service.name:checkout and log.level:(error or warn)",
			`{"bool":{"filter":[{"match":{"service.name":"checkout"}},{"bool":{"should":[{"match":{"log.level":"error"}},{"match":{"log.level":"warn"}}],"minimum_should_match":1}}]}}`},
		{"quoted phrase on a text field", `message:"payment failed"`, `{"match_phrase":{"message":"payment failed"}}`},
		{"quoted value on a keyword field", `service.name:"checkout api"`, `{"match":{"service.name":"checkout api"}}`},
		{"not", "not http.status:200", `{"bool":{"must_not":[{"match":{"http.status":"200"}}]}}`},
		{"range", "latency > 250", `{"range":{"latency":{"gt":"250"}}}`},
		{"quoted range bound", `@timestamp >= "2026-11-06"`, `{"range":{"@timestamp":{"gte":"2026-11-06"}}}`},
		{"range operators", "latency >= 1 and latency < 2 and latency <= 3",
			`{"bool":{"filter":[{"range":{"latency":{"gte":"1"}}},{"range":{"latency":{"lt":"2"}}},{"range":{"latency":{"lte":"3"}}}]}}`},
		{"wildcard value", "host.name:web-*", `{"wildcard":{"host.name":{"value":"web-*","case_insensitive":true}}}`},
		{"escaped star and question mark in a wildcard", `host.name:a\*b?*`, `{"wildcard":{"host.name":{"value":"a\\*b\\?*","case_insensitive":true}}}`},
		{"free word", "timeout", `{"multi_match":{"query":"timeout","type":"best_fields","lenient":true}}`},
		{"free phrase", `"payment failed"`, `{"multi_match":{"query":"payment failed","type":"phrase","lenient":true}}`},
		{"free wildcard", "time*", `{"query_string":{"query":"time*","lenient":true,"analyze_wildcard":true}}`},
		{"free wildcard escapes query_string specials", `foo/bar\:*`, `{"query_string":{"query":"foo\\/bar\\:*","lenient":true,"analyze_wildcard":true}}`},
		{"exists", "trace.id:*", `{"exists":{"field":"trace.id"}}`},
		{"escaped star is a literal", `trace.id:\*`, `{"match":{"trace.id":"*"}}`},
		{"field pattern", "log.*:error", `{"multi_match":{"query":"error","fields":["log.*"],"type":"best_fields","lenient":true}}`},
		{"field pattern with a wildcard value", "log.*:err*", `{"query_string":{"query":"err*","fields":["log.*"],"lenient":true,"analyze_wildcard":true}}`},
		{"and and not inside a value group", "log.level:(error and not warn)",
			`{"bool":{"filter":[{"match":{"log.level":"error"}},{"bool":{"must_not":[{"match":{"log.level":"warn"}}]}}]}}`},
		{"parentheses only group", "(log.level:error or log.level:warn) and service.name:checkout",
			`{"bool":{"filter":[{"bool":{"should":[{"match":{"log.level":"error"}},{"match":{"log.level":"warn"}}],"minimum_should_match":1}},{"match":{"service.name":"checkout"}}]}}`},
		{"date math passes through", "@timestamp >= now-15m", `{"range":{"@timestamp":{"gte":"now-15m"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := kql.Compile(tc.text, demoFields)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Diagnostics) != 0 {
				t.Fatalf("diagnostics %+v", got.Diagnostics)
			}
			var want map[string]any
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(roundTrip(t, got.Query), want) {
				raw, _ := json.Marshal(got.Query)
				t.Fatalf("query\n got %s\nwant %s", raw, tc.want)
			}
		})
	}
}

func TestCompileReturnsNoQueryForAnEmptyText(t *testing.T) {
	got, err := kql.Compile("  ", demoFields)
	if err != nil || got.Query != nil || len(got.Diagnostics) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestCompileReturnsTheParseErrorAsBothErrorAndDiagnostic(t *testing.T) {
	got, err := kql.Compile("a:", demoFields)
	want := kql.Diagnostic{Severity: kql.SeverityError, Message: `expected a value after ":"`, Start: 1, End: 2}
	if err == nil || err.Error() != want.Message || got.Query != nil || len(got.Diagnostics) != 1 || got.Diagnostics[0] != want {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestCompileWarnsOnceForEachUnknownField(t *testing.T) {
	for _, tc := range []struct {
		name   string
		text   string
		fields kql.Fields
		want   []kql.Diagnostic
	}{
		{"unknown field named twice", "nope:1 and nope:2 and log.level:x", demoFields,
			[]kql.Diagnostic{{Severity: kql.SeverityWarning, Message: `unknown field "nope"`, Start: 0, End: 4}}},
		{"unknown fields in range exists and group", "lat > 1 and tid:* and lvl:(a or b)", demoFields,
			[]kql.Diagnostic{
				{Severity: kql.SeverityWarning, Message: `unknown field "lat"`, Start: 0, End: 3},
				{Severity: kql.SeverityWarning, Message: `unknown field "tid"`, Start: 12, End: 15},
				{Severity: kql.SeverityWarning, Message: `unknown field "lvl"`, Start: 22, End: 25},
			}},
		{"nil fields report nothing", "nope:1", nil, nil},
		{"pattern matching a known field", "log.*:x", demoFields, nil},
		{"pattern matching no field", "foo.*:x", demoFields,
			[]kql.Diagnostic{{Severity: kql.SeverityWarning, Message: `unknown field "foo.*"`, Start: 0, End: 5}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := kql.Compile(tc.text, tc.fields)
			if err != nil {
				t.Fatal(err)
			}
			if got.Query == nil || !slices.Equal(got.Diagnostics, tc.want) {
				t.Fatalf("query nil %v, diagnostics %+v, want %+v", got.Query == nil, got.Diagnostics, tc.want)
			}
		})
	}
}

func TestFieldsInListsEachFieldOnceInOrder(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"service.name:checkout and log.level:(error or warn) and service.name:x and latency > 1 and trace.id:*", []string{"service.name", "log.level", "latency", "trace.id"}},
		{"timeout", nil},
		{"", nil},
		{"a:", nil},
	} {
		if got := kql.FieldsIn(tc.text); !slices.Equal(got, tc.want) {
			t.Fatalf("%q: got %v, want %v", tc.text, got, tc.want)
		}
	}
}

func roundTrip(t *testing.T, query map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(query)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
