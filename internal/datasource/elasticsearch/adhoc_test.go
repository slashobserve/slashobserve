package elasticsearch_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func adhocFilterClauses(t *testing.T, filters []datasource.AdhocFilter) string {
	t.Helper()
	f, ds := searchServer(t, hitsOf(0))
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Filters: filters}, queryRange); err != nil {
		t.Fatal(err)
	}
	filter := searchBody(t, f.last(t))["query"].(map[string]any)["bool"].(map[string]any)["filter"].([]any)
	raw, err := json.Marshal(filter[2:])
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAdhocFiltersBecomeFilterClauses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		filters []datasource.AdhocFilter
		want    string
	}{
		{
			"equals and not equals on keyword fields",
			[]datasource.AdhocFilter{{Key: "service.name", Op: "=", Value: "checkout"}, {Key: "log.level", Op: "!=", Value: "info"}},
			`[{"term":{"service.name":"checkout"}},{"bool":{"must_not":[{"term":{"log.level":"info"}}]}}]`,
		},
		{
			"a text field with a keyword subfield matches the subfield",
			[]datasource.AdhocFilter{{Key: "message", Op: "=", Value: "hello"}},
			`[{"term":{"message.keyword":"hello"}}]`,
		},
		{
			"a text field without a keyword subfield matches the phrase",
			[]datasource.AdhocFilter{{Key: "error.stack_trace", Op: "=", Value: "boom"}},
			`[{"match_phrase":{"error.stack_trace":"boom"}}]`,
		},
		{
			"regex on a text field uses the keyword subfield",
			[]datasource.AdhocFilter{{Key: "message", Op: "=~", Value: "pay.*"}},
			`[{"regexp":{"message.keyword":{"value":"pay.*","case_insensitive":true}}}]`,
		},
		{
			"regex on a text field without a keyword uses the field",
			[]datasource.AdhocFilter{{Key: "error.stack_trace", Op: "=~", Value: "x.*"}},
			`[{"regexp":{"error.stack_trace":{"value":"x.*","case_insensitive":true}}}]`,
		},
		{
			"negated regex",
			[]datasource.AdhocFilter{{Key: "service.name", Op: "!~", Value: "web-.*"}},
			`[{"bool":{"must_not":[{"regexp":{"service.name":{"value":"web-.*","case_insensitive":true}}}]}}]`,
		},
		{
			"ordering uses range on the field itself",
			[]datasource.AdhocFilter{{Key: "http.response.status_code", Op: ">", Value: "499"}, {Key: "event.ingested", Op: "<", Value: "2026-11-06"}},
			`[{"range":{"http.response.status_code":{"gt":"499"}}},{"range":{"event.ingested":{"lt":"2026-11-06"}}}]`,
		},
		{
			"a field outside the mapping is matched as given",
			[]datasource.AdhocFilter{{Key: "custom.field", Op: "=", Value: "v"}},
			`[{"term":{"custom.field":"v"}}]`,
		},
		{
			"or conditions become a should group",
			[]datasource.AdhocFilter{{Key: "service.name", Op: "=", Value: "a"}, {Key: "service.name", Op: "=", Value: "b", Condition: "or"}},
			`[{"bool":{"should":[{"bool":{"filter":[{"term":{"service.name":"a"}}]}},{"bool":{"filter":[{"term":{"service.name":"b"}}]}}],"minimum_should_match":1}}]`,
		},
		{
			"or groups keep their and members and other scopes stay separate",
			[]datasource.AdhocFilter{
				{Key: "service.name", Op: "=", Value: "a"}, {Key: "log.level", Op: "=", Value: "error"},
				{Key: "service.name", Op: "=", Value: "b", Condition: "or"},
				{Key: "span.id", Op: "=", Value: "s", Scope: 1},
			},
			`[{"bool":{"should":[{"bool":{"filter":[{"term":{"service.name":"a"}},{"term":{"log.level":"error"}}]}},{"bool":{"filter":[{"term":{"service.name":"b"}}]}}],"minimum_should_match":1}},{"term":{"span.id":"s"}}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertJSONEqual(t, adhocFilterClauses(t, tc.filters), tc.want)
		})
	}
}

func TestAdhocFilterWithoutFiltersAddsNothing(t *testing.T) {
	assertJSONEqual(t, adhocFilterClauses(t, nil), `[]`)
}

func TestAdhocFiltersRefuseWhatTheyCannotExpress(t *testing.T) {
	for _, filter := range []datasource.AdhocFilter{
		{Key: "service.name", Op: "~", Value: "x"},
		{Key: "", Op: "=", Value: "x"},
	} {
		f, ds := searchServer(t, hitsOf(0))
		_, err := ds.QueryInstant(context.Background(), datasource.Query{Filters: []datasource.AdhocFilter{filter}}, queryRange)
		if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Errorf("%+v: error = %v, want ErrInvalidQuery", filter, err)
		}
		if f.count(searchPath) != 0 {
			t.Errorf("%+v: reached the search endpoint", filter)
		}
	}
}

func TestAdhocFiltersNarrowRangeCountsToo(t *testing.T) {
	f, ds := searchServer(t, `{"hits":{"hits":[]},"aggregations":{"count":{"buckets":[]}}}`)
	filters := []datasource.AdhocFilter{{Key: "service.name", Op: "=", Value: "checkout"}}
	if _, err := ds.QueryRange(context.Background(), datasource.Query{Filters: filters, Text: "lucene:a"}, queryRange, time.Minute); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.last(t).body, `{"term":{"service.name":"checkout"}}`) {
		t.Fatalf("filter missing from %s", f.last(t).body)
	}
}
