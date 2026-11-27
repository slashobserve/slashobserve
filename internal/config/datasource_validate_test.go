package config

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func validProm() Datasource {
	return Datasource{UID: "prom", Type: "prometheus", Name: "Prom", Prometheus: &PrometheusDatasource{URL: "http://prom:9090"}}
}

func fieldsOf(errs []FieldError) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Field
	}
	slices.Sort(out)
	return out
}

func TestValidateAcceptsADefinitionTheFileLoaderWouldAccept(t *testing.T) {
	if errs := validProm().Validate(); len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
}

func TestValidateNamesTheFieldThatBreaksARule(t *testing.T) {
	longName := strings.Repeat("n", 101)
	tests := []struct {
		name   string
		change func(*Datasource)
		want   []string
	}{
		{"bad uid", func(d *Datasource) { d.UID = "has space" }, []string{"uid"}},
		{"long name", func(d *Datasource) { d.Name = longName }, []string{"name"}},
		{"unknown type", func(d *Datasource) { d.Type = "graphite" }, []string{"type"}},
		{"missing block", func(d *Datasource) { d.Prometheus = nil }, []string{"prometheus"}},
		{"extra block", func(d *Datasource) { d.Loki = &LokiDatasource{} }, []string{"loki"}},
		{"no url", func(d *Datasource) { d.Prometheus.URL = "" }, []string{"url"}},
		{"negative limit", func(d *Datasource) { d.Limits.MaxRows = -1 }, []string{"limits.maxRows"}},
		{"too many queries", func(d *Datasource) { d.MaxConcurrentQueries = maxConcurrentQueries + 1 }, []string{"limits.maxConcurrentQueries"}},
		{"basic and bearer", func(d *Datasource) {
			d.Prometheus.BasicAuthUser = "bob"
			d.Prometheus.BasicAuthPassword = datasource.NewSecret("pw")
			d.Prometheus.BearerToken = datasource.NewSecret("tok")
		}, []string{"basicAuth"}},
		{"basic user without password", func(d *Datasource) { d.Prometheus.BasicAuthUser = "bob" }, []string{"basicAuth.password"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validProm()
			tt.change(&d)
			if got := fieldsOf(d.Validate()); !slices.Equal(got, tt.want) {
				t.Fatalf("fields = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateSearchAndClickHouseRules(t *testing.T) {
	search := Datasource{UID: "logs", Type: "elasticsearch", Elasticsearch: &SearchDatasource{
		HTTPConnection: HTTPConnection{URL: "http://es:9200", BearerToken: datasource.NewSecret("tok")},
		Index:          "_all", QueryLanguage: "sql", APIKey: datasource.NewSecret("key"), Spans: "zipkin",
	}}
	if got, want := fieldsOf(search.Validate()), []string{"apiKey", "index", "queryLanguage", "spans"}; !slices.Equal(got, want) {
		t.Fatalf("search fields = %v, want %v", got, want)
	}
	ch := Datasource{UID: "ch", Type: "clickhouse", ClickHouse: &ClickHouseDatasource{}}
	if got, want := fieldsOf(ch.Validate()), []string{"addr", "password", "username"}; !slices.Equal(got, want) {
		t.Fatalf("clickhouse fields = %v, want %v", got, want)
	}
}
