package datasourceadmin

import (
	"slices"
	"testing"
	"time"
)

func TestConfigCarriesEveryFieldOfEachType(t *testing.T) {
	def := Definition{
		UID: "logs", Type: TypeElasticsearch, Name: "Logs", GrafanaUIDs: []string{"es"},
		Address: "https://es:9200", Index: "logs-*", TimeField: "@timestamp", QueryLanguage: "kql",
		Fields: FieldRoles{Message: "msg", TraceID: "trace"},
		Limits: Limits{Timeout: "20s", MaxRows: 500, MaxConcurrentQueries: 8},
	}
	cfg, problems := def.Config(Secrets{APIKey: "k", Headers: map[string]string{"X-Org": "7"}})
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	es := cfg.Elasticsearch
	if es == nil || es.URL != "https://es:9200" || es.Index != "logs-*" || es.Fields.TraceID != "trace" || es.APIKey.Reveal() != "k" {
		t.Fatalf("search = %+v", es)
	}
	if es.SecretHeaders["X-Org"].Reveal() != "7" || len(es.Headers) != 0 {
		t.Fatalf("headers = %+v / %+v", es.SecretHeaders, es.Headers)
	}
	if cfg.Limits.Timeout != 20*time.Second || cfg.Limits.MaxRows != 500 || cfg.MaxConcurrentQueries != 8 {
		t.Fatalf("limits = %+v, %d", cfg.Limits, cfg.MaxConcurrentQueries)
	}

	ch, problems := Definition{UID: "ch", Type: TypeClickHouse, Address: "ch:9440", Username: "reader", Database: "logs", TLS: true}.Config(Secrets{Password: "pw"})
	if len(problems) != 0 || ch.ClickHouse == nil || ch.ClickHouse.Username != "reader" || !ch.ClickHouse.TLS || ch.ClickHouse.Password.Reveal() != "pw" {
		t.Fatalf("clickhouse = %+v, problems = %v", ch.ClickHouse, problems)
	}
}

func TestConfigReportsBadDurationsAndMissingFieldsByName(t *testing.T) {
	_, problems := Definition{UID: "p", Type: TypePrometheus, ScrapeInterval: "soon", Limits: Limits{Timeout: "-5s"}}.Config(Secrets{})
	var fields []string
	for _, p := range problems {
		fields = append(fields, p.Field)
	}
	slices.Sort(fields)
	if want := []string{"limits.timeout", "scrape_interval", "url"}; !slices.Equal(fields, want) {
		t.Fatalf("fields = %v, want %v", fields, want)
	}
}

func TestSecretNamesListWhatIsSetWithoutTheValues(t *testing.T) {
	names := Secrets{Password: "a", APIKey: "b", Headers: map[string]string{"X-B": "1", "X-A": "2"}}.Names()
	if want := []string{"password", "api_key", "header:X-A", "header:X-B"}; !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	if !(Secrets{}).Empty() {
		t.Fatal("empty secrets reported as set")
	}
}
