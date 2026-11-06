package tools

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

type facetFakeDS struct {
	*fakeDS
	onFacets func(ctx context.Context, req datasource.FacetRequest) (datasource.Facets, error)

	mu       sync.Mutex
	requests []datasource.FacetRequest
}

func (f *facetFakeDS) LogFacets(ctx context.Context, req datasource.FacetRequest) (datasource.Facets, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	if f.onFacets != nil {
		return f.onFacets(ctx, req)
	}
	return datasource.Facets{}, nil
}

func newFacetRegistry(t *testing.T, ds datasource.Datasource) *Registry {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger})
}

func manyFacets(fields, values int) datasource.Facets {
	out := datasource.Facets{Sampled: true, LinesSampled: 1000}
	for i := range fields {
		field := datasource.FacetField{Name: "f" + strconv.Itoa(fields-i), Kind: "field", Cardinality: values}
		for j := range values {
			field.Values = append(field.Values, datasource.FacetValue{Value: "v" + strconv.Itoa(j), Count: int64(j + 1)})
		}
		out.Fields = append(out.Fields, field)
	}
	return out
}

func TestLogFacetsCountsThroughTheDatasourceAndCapsTheAnswer(t *testing.T) {
	ds := &facetFakeDS{fakeDS: &fakeDS{typ: "loki"}}
	ds.onFacets = func(context.Context, datasource.FacetRequest) (datasource.Facets, error) {
		facets := manyFacets(25, 12)
		facets.Fields = append(facets.Fields, datasource.FacetField{Name: "service_name", Kind: "label", Exact: true, Unit: "bytes", Cardinality: 2, Values: []datasource.FacetValue{{Value: "checkout", Count: 1}, {Value: "frontend", Count: 2}}})
		return facets, nil
	}
	r := newFacetRegistry(t, ds)
	res, err := call(t, r, nameLogFacets, `{"datasource":"fake","query":"{app=\"shop\"}","from":"now-6h","to":"now","fields":["service_name","f1"],"limit":12}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("res = %#v", res.Content)
	}
	if want := "Counted 20 fields on fake (sampled from 1000 lines)"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
	facets, ok := res.Content.(datasource.Facets)
	if !ok {
		t.Fatalf("content is %T", res.Content)
	}
	if len(facets.Fields) != 20 || !facets.Truncated || !facets.Sampled || facets.LinesSampled != 1000 {
		t.Fatalf("facets = %+v", facets)
	}
	if first := facets.Fields[0]; first.Name != "service_name" || !first.Exact || first.Values[0].Value != "frontend" {
		t.Fatalf("first field = %+v, want the exact label with its top value first", first)
	}
	if second := facets.Fields[1]; second.Name != "f1" || len(second.Values) != 10 || !second.Truncated || second.Values[0].Count != 12 {
		t.Fatalf("second field = %+v, want f1 cut to its 10 top values", second)
	}
	ds.mu.Lock()
	req := ds.requests[0]
	ds.mu.Unlock()
	if req.Query.Text != `{app="shop"}` || req.Query.Limits != queryproxy.DraftLimits || req.Limit != 12 || req.Sample != 1000 || strings.Join(req.Fields, ",") != "service_name,f1" {
		t.Fatalf("request = %+v", req)
	}
	if got := req.Range.To.Sub(req.Range.From); got.Hours() != 6 {
		t.Fatalf("range = %v, want 6h", got)
	}
}

func TestLogFacetsRefusesDatasourcesWithoutFacets(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, nameLogFacets, `{"datasource":"fake","query":"up"}`)
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != codeInvalidRequest {
		t.Fatalf("res = %#v", res.Content)
	}
	if want := "log_facets works on Loki, ClickHouse, Elasticsearch and OpenSearch datasources; fake is a prometheus datasource"; res.Content.(errorBody).Message != want {
		t.Fatalf("message = %q, want %q", res.Content.(errorBody).Message, want)
	}
}

func TestLogFacetsRejectsInvalidInput(t *testing.T) {
	ds := &facetFakeDS{fakeDS: &fakeDS{typ: "loki"}}
	r := newFacetRegistry(t, ds)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"missing query", `{"datasource":"fake"}`, "query is required"},
		{"limit zero", `{"datasource":"fake","query":"{a=\"b\"}","limit":0}`, "limit must be between 1 and 50"},
		{"empty fields", `{"datasource":"fake","query":"{a=\"b\"}","fields":[]}`, "fields must name from 1 to 50 fields"},
		{"field too long", `{"datasource":"fake","query":"{a=\"b\"}","fields":["` + strings.Repeat("a", 201) + `"]}`, "each field must be from 1 to 200 bytes"},
		{"unknown field", `{"datasource":"fake","query":"{a=\"b\"}","sample":100}`, `unknown field "sample"`},
		{"from after to", `{"datasource":"fake","query":"{a=\"b\"}","from":"now","to":"now-1h"}`, "from must be before to"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := call(t, r, nameLogFacets, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("code = %q", errorCode(t, res))
			}
			if got := res.Content.(errorBody).Message; !strings.HasPrefix(got, tt.want) {
				t.Fatalf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if len(ds.requests) != 0 {
		t.Fatalf("the datasource was called for rejected input: %+v", ds.requests)
	}
}

func TestLogFacetsReportsDatasourceErrorsByCode(t *testing.T) {
	ds := &facetFakeDS{fakeDS: &fakeDS{typ: "clickhouse"}}
	ds.onFacets = func(context.Context, datasource.FacetRequest) (datasource.Facets, error) {
		return datasource.Facets{}, datasource.Errorf(datasource.ErrInvalidQuery, `field "Missing" is not a column of the query`)
	}
	r := newFacetRegistry(t, ds)
	res, err := call(t, r, nameLogFacets, `{"datasource":"fake","query":"SELECT 1","fields":["Missing"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != string(queryproxy.CodeQueryInvalid) {
		t.Fatalf("res = %#v", res.Content)
	}
	if want := "Counting facets on fake: query_invalid"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}
