package tools

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const clickhouseLogsTemplate = "SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes FROM otel_logs WHERE $__timeFilter(Timestamp) ORDER BY Timestamp DESC LIMIT 1000"

func TestDraftWarnings(t *testing.T) {
	cases := []struct {
		name   string
		dsType spec.DatasourceType
		query  string
		want   []string
	}{
		{"loki any job", spec.Loki, `{job=~".+"}`, []string{warnEveryStream}},
		{"loki two match-all matchers", spec.Loki, `{a=~".*", b!=""}`, []string{warnEveryStream}},
		{"loki empty negated regex", spec.Loki, `{a!~""} |= "x"`, []string{warnEveryStream}},
		{"loki exact service", spec.Loki, `{service_name="checkout"}`, nil},
		{"loki one exact matcher", spec.Loki, `{a=~".+", b="x"}`, nil},
		{"loki brace inside a value", spec.Loki, `{a="}", b=~".*"}`, nil},
		{"loki brace inside a match-all pair", spec.Loki, `{a=~".*", b="x}y"} |= "}"`, nil},
		{"tempo empty spanset", spec.Tempo, `{ }`, []string{warnEveryTrace}},
		{"tempo true", spec.Tempo, `{ true }`, []string{warnEveryTrace}},
		{"tempo service", spec.Tempo, `{ resource.service.name = "checkout" }`, nil},
		{"elasticsearch kql", spec.Elasticsearch, `service.name:checkout and log.level:error`, nil},
		{"elasticsearch star", spec.Elasticsearch, `*`, []string{warnEveryDocument}},
		{"elasticsearch empty body", spec.Elasticsearch, "  ", []string{warnEveryDocument}},
		{"opensearch kql prefix alone", spec.OpenSearch, ` kql: `, []string{warnEveryDocument}},
		{"opensearch kql prefix and star", spec.OpenSearch, `KQL:*`, []string{warnEveryDocument}},
		{"elasticsearch lucene", spec.Elasticsearch, `lucene:service.name:checkout`, []string{warnLucene}},
		{"elasticsearch lucene star", spec.Elasticsearch, `lucene:*`, []string{warnEveryDocument, warnLucene}},
		{"opensearch dsl body", spec.OpenSearch, `{"query":{"match_all":{}}}`, nil},
		{"clickhouse template", spec.ClickHouse, clickhouseLogsTemplate, nil},
		{"clickhouse without the time filter", spec.ClickHouse, "SELECT Timestamp, Body FROM otel_logs ORDER BY Timestamp DESC LIMIT 1000", []string{warnNoTimeFilter}},
		{"clickhouse time filter only in a comment", spec.ClickHouse, "SELECT Body FROM otel_logs -- WHERE $__timeFilter(Timestamp)\nLIMIT 10", []string{warnNoTimeFilter}},
		{"clickhouse time filter only in a string", spec.ClickHouse, "SELECT Body FROM otel_logs WHERE Body = '$__timeFilter(Timestamp)' LIMIT 10", []string{warnNoTimeFilter}},
		{"clickhouse limit only in a subquery", spec.ClickHouse, "SELECT Body FROM (SELECT Body FROM otel_logs WHERE $__timeFilter(Timestamp) LIMIT 10)", []string{warnNoLimit}},
		{"clickhouse lower case limit", spec.ClickHouse, "select Body from otel_logs where $__timeFilter(Timestamp) limit 5", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := draftWarnings(c.dsType, c.query); !slices.Equal(got, c.want) {
				t.Fatalf("draftWarnings(%q) = %q, want %q", c.query, got, c.want)
			}
		})
	}
}
