package savedsearches

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func ptr(s string) *string { return &s }

func TestValidateAppliesEveryRule(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	logs := func(edit func(*CreateInput)) CreateInput {
		in := CreateInput{Kind: KindLogs, Title: "Errors", DatasourceUID: "loki", Query: ptr(`{app="x"}`)}
		edit(&in)
		return in
	}
	traces := func(edit func(*CreateInput)) CreateInput {
		in := CreateInput{Kind: KindTraces, Title: "Slow", DatasourceUID: "tempo"}
		edit(&in)
		return in
	}
	cases := []struct {
		name string
		in   CreateInput
		want string
	}{
		{"a logs search", logs(func(*CreateInput) {}), ""},
		{"a logs search with columns", logs(func(in *CreateInput) { in.Filters = []byte(`{"columns":["level","msg"]}`) }), ""},
		{"a logs search with null filters", logs(func(in *CreateInput) { in.Filters = []byte(`null`) }), ""},
		{"a traces search with no query", traces(func(*CreateInput) {}), ""},
		{"a traces search with a query", traces(func(in *CreateInput) { in.Query = ptr(`{ duration > 1s }`) }), ""},
		{"a traces search with every filter", traces(func(in *CreateInput) {
			in.Filters = []byte(`{"service":"checkout","span":"POST /pay","min":"250ms","max":"2.5s","errors":true}`)
			in.From, in.To = "now-6h", "now-5m"
		}), ""},
		{"absolute times", logs(func(in *CreateInput) { in.From, in.To = "2026-11-01T08:00:00Z", "2026-11-01T09:00:00Z" }), ""},
		{"an unknown kind", logs(func(in *CreateInput) { in.Kind = "metrics" }), "kind must be"},
		{"an empty title", logs(func(in *CreateInput) { in.Title = "  " }), "title is required"},
		{"a long title", logs(func(in *CreateInput) { in.Title = strings.Repeat("é", 121) }), "title must be at most 120"},
		{"a title with a control character", logs(func(in *CreateInput) { in.Title = "a\nb" }), "control characters"},
		{"a datasource uid that is not one", logs(func(in *CreateInput) { in.DatasourceUID = "-bad" }), "datasource_uid"},
		{"a logs search without a query", logs(func(in *CreateInput) { in.Query = nil }), "query is required"},
		{"a blank query", traces(func(in *CreateInput) { in.Query = ptr(" ") }), "query must not be blank"},
		{"a long query", logs(func(in *CreateInput) { in.Query = ptr(strings.Repeat("x", 8001)) }), "query must be at most 8000"},
		{"an unknown filter key", logs(func(in *CreateInput) { in.Filters = []byte(`{"colums":["a"]}`) }), "only the keys"},
		{"filters that are not an object", logs(func(in *CreateInput) { in.Filters = []byte(`[1]`) }), "only the keys"},
		{"a trace filter on a logs search", logs(func(in *CreateInput) { in.Filters = []byte(`{"service":"x"}`) }), "only the filter key columns"},
		{"too many columns", logs(func(in *CreateInput) {
			in.Filters = []byte(`{"columns":["` + strings.Join(make([]string, 21), `","`) + `"]}`)
		}), "at most 20"},
		{"a column with a comma", logs(func(in *CreateInput) { in.Filters = []byte(`{"columns":["a,b"]}`) }), "column name"},
		{"an empty column", logs(func(in *CreateInput) { in.Filters = []byte(`{"columns":[""]}`) }), "column name"},
		{"columns on a traces search", traces(func(in *CreateInput) { in.Filters = []byte(`{"columns":["a"]}`) }), "only the filter keys"},
		{"a long service filter", traces(func(in *CreateInput) { in.Filters = []byte(`{"service":"` + strings.Repeat("s", 201) + `"}`) }), "service must be"},
		{"a span filter with a control character", traces(func(in *CreateInput) { in.Filters = []byte(`{"span":"a\u0007b"}`) }), "span must be"},
		{"a duration that is not one", traces(func(in *CreateInput) { in.Filters = []byte(`{"min":"fast"}`) }), "min must be a duration"},
		{"a maximum duration that is not one", traces(func(in *CreateInput) { in.Filters = []byte(`{"max":"5"}`) }), "max must be a duration"},
		{"a time that does not parse", logs(func(in *CreateInput) { in.From = "yesterday" }), "from:"},
		{"an end that does not parse", logs(func(in *CreateInput) { in.To = "now-" }), "to:"},
		{"a time that is too long", logs(func(in *CreateInput) { in.To = "now-" + strings.Repeat("1", 70) + "h" }), "to must be at most 64"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := validate(c.in, now)
			if c.want == "" {
				if err != nil {
					t.Fatalf("validate = %v, want nil", err)
				}
				return
			}
			var invalid *InvalidInputError
			if !errors.As(err, &invalid) || invalid.Code != CodeInvalid || !strings.Contains(invalid.Message, c.want) {
				t.Fatalf("validate = %v, want %s containing %q", err, CodeInvalid, c.want)
			}
		})
	}
}

func TestValidateDefaultsAndTrimsAndDecodesFilters(t *testing.T) {
	got, err := validate(CreateInput{Kind: KindTraces, Title: "  Slow  ", DatasourceUID: "tempo", Filters: []byte(`{"service":"checkout","errors":true}`)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Slow" || got.From != "now-1h" || got.To != "now" || got.filters.Service != "checkout" || !got.filters.Errors {
		t.Fatalf("validated = %+v", got)
	}
}

func TestFitDatasourceMatchesTheTypeToTheKind(t *testing.T) {
	query := ptr("x")
	cases := []struct {
		name  string
		kind  Kind
		typ   spec.DatasourceType
		query *string
		want  string
	}{
		{"loki logs", KindLogs, spec.Loki, query, ""},
		{"clickhouse logs", KindLogs, spec.ClickHouse, query, ""},
		{"elasticsearch logs", KindLogs, spec.Elasticsearch, query, ""},
		{"opensearch logs", KindLogs, spec.OpenSearch, query, ""},
		{"prometheus logs", KindLogs, spec.Prometheus, query, CodeUnsupportedDatasource},
		{"tempo logs", KindLogs, spec.Tempo, query, CodeUnsupportedDatasource},
		{"tempo traces with a query", KindTraces, spec.Tempo, query, ""},
		{"tempo traces without one", KindTraces, spec.Tempo, nil, ""},
		{"clickhouse traces without a query", KindTraces, spec.ClickHouse, nil, ""},
		{"clickhouse traces with a query", KindTraces, spec.ClickHouse, query, CodeInvalid},
		{"loki traces", KindTraces, spec.Loki, nil, CodeUnsupportedDatasource},
		{"elasticsearch traces", KindTraces, spec.Elasticsearch, nil, CodeUnsupportedDatasource},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := fitDatasource(c.kind, c.typ, c.query)
			if c.want == "" {
				if err != nil {
					t.Fatalf("fitDatasource = %v, want nil", err)
				}
				return
			}
			var invalid *InvalidInputError
			if !errors.As(err, &invalid) || invalid.Code != c.want {
				t.Fatalf("fitDatasource = %v, want code %s", err, c.want)
			}
		})
	}
}

func TestAddressCarriesWhatReopensTheSearch(t *testing.T) {
	logs := SavedSearch{Kind: KindLogs, DatasourceUID: "loki", Query: ptr(`{app="x"}`), From: "now-1h", To: "now", Filters: Filters{Columns: []string{"level", "msg"}}}
	if got, want := logs.address().Encode(), "cols=level%2Cmsg&ds=loki&from=now-1h&q=%7Bapp%3D%22x%22%7D&to=now"; got != want {
		t.Errorf("logs address %q, want %q", got, want)
	}
	traces := SavedSearch{Kind: KindTraces, DatasourceUID: "tempo", From: "now-1h", To: "now", Filters: Filters{Service: "checkout", Min: "250ms", Errors: true}}
	if got, want := traces.address().Encode(), "ds=tempo&errors=1&from=now-1h&min=250ms&service=checkout&to=now"; got != want {
		t.Errorf("traces address %q, want %q", got, want)
	}
}
