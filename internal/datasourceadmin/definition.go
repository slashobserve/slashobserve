package datasourceadmin

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	TypePrometheus    = "prometheus"
	TypeClickHouse    = "clickhouse"
	TypeLoki          = "loki"
	TypeTempo         = "tempo"
	TypeJaeger        = "jaeger"
	TypeElasticsearch = "elasticsearch"
	TypeOpenSearch    = "opensearch"

	secretPassword     = "password"
	secretBearerToken  = "bearer_token"
	secretAPIKey       = "api_key"
	secretHeaderPrefix = "header:"
)

type Limits struct {
	Timeout              string `json:"timeout,omitempty"`
	MaxSeries            int    `json:"max_series,omitempty"`
	MaxPoints            int    `json:"max_points,omitempty"`
	MaxRows              int    `json:"max_rows,omitempty"`
	MaxConcurrentQueries int    `json:"max_concurrent_queries,omitempty"`
}

type FieldRoles struct {
	Message string `json:"message,omitempty"`
	Level   string `json:"level,omitempty"`
	Service string `json:"service,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
}

type Definition struct {
	UID            string              `json:"uid"`
	Type           string              `json:"type"`
	Name           string              `json:"name"`
	Audience       datasource.Audience `json:"audience"`
	GrafanaUIDs    []string            `json:"grafana_uids,omitempty"`
	Address        string              `json:"address"`
	Username       string              `json:"username,omitempty"`
	Database       string              `json:"database,omitempty"`
	TLS            bool                `json:"tls,omitempty"`
	TenantID       string              `json:"tenant_id,omitempty"`
	ScrapeInterval string              `json:"scrape_interval,omitempty"`
	Index          string              `json:"index,omitempty"`
	TimeField      string              `json:"time_field,omitempty"`
	QueryLanguage  string              `json:"query_language,omitempty"`
	Fields         FieldRoles          `json:"fields,omitzero"`
	Spans          string              `json:"spans,omitempty"`
	Limits         Limits              `json:"limits,omitzero"`
}

type Secrets struct {
	Password    string            `json:"password,omitempty"`
	BearerToken string            `json:"bearer_token,omitempty"`
	APIKey      string            `json:"api_key,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

func (s Secrets) Names() []string {
	var names []string
	if s.Password != "" {
		names = append(names, secretPassword)
	}
	if s.BearerToken != "" {
		names = append(names, secretBearerToken)
	}
	if s.APIKey != "" {
		names = append(names, secretAPIKey)
	}
	for _, name := range slices.Sorted(maps.Keys(s.Headers)) {
		names = append(names, secretHeaderPrefix+name)
	}
	return names
}

func (s Secrets) Empty() bool { return len(s.Names()) == 0 }

func (d Definition) Config(secrets Secrets) (config.Datasource, []config.FieldError) {
	var problems []config.FieldError
	out := config.Datasource{
		UID:         d.UID,
		Type:        d.Type,
		Name:        d.Name,
		GrafanaUIDs: slices.Clone(d.GrafanaUIDs),
		Limits: datasource.Limits{
			MaxSeries: d.Limits.MaxSeries,
			MaxPoints: d.Limits.MaxPoints,
			MaxRows:   d.Limits.MaxRows,
		},
		MaxConcurrentQueries: d.Limits.MaxConcurrentQueries,
	}
	timeout, err := parseDuration(d.Limits.Timeout)
	if err != nil {
		problems = append(problems, config.FieldError{Field: "limits.timeout", Message: err.Error()})
	}
	out.Limits.Timeout = timeout

	conn := config.HTTPConnection{
		URL:               d.Address,
		BasicAuthUser:     d.Username,
		BasicAuthPassword: secret(secrets.Password),
		BearerToken:       secret(secrets.BearerToken),
		SecretHeaders:     secretHeaders(secrets.Headers),
	}
	switch d.Type {
	case TypePrometheus:
		interval, err := parseDuration(d.ScrapeInterval)
		if err != nil {
			problems = append(problems, config.FieldError{Field: "scrape_interval", Message: err.Error()})
		}
		out.Prometheus = &config.PrometheusDatasource{
			URL:               conn.URL,
			ScrapeInterval:    interval,
			SecretHeaders:     conn.SecretHeaders,
			BasicAuthUser:     conn.BasicAuthUser,
			BasicAuthPassword: conn.BasicAuthPassword,
			BearerToken:       conn.BearerToken,
		}
	case TypeLoki:
		out.Loki = &config.LokiDatasource{HTTPConnection: conn, TenantID: d.TenantID}
	case TypeTempo:
		out.Tempo = &config.TempoDatasource{HTTPConnection: conn}
	case TypeJaeger:
		out.Jaeger = &config.JaegerDatasource{HTTPConnection: conn}
	case TypeElasticsearch:
		search := d.search(conn, secrets)
		out.Elasticsearch = &search
	case TypeOpenSearch:
		search := d.search(conn, secrets)
		out.OpenSearch = &search
	case TypeClickHouse:
		out.ClickHouse = &config.ClickHouseDatasource{
			Addr:     d.Address,
			Database: d.Database,
			Username: d.Username,
			Password: secret(secrets.Password),
			TLS:      d.TLS,
		}
	}
	for _, problem := range out.Validate() {
		problems = append(problems, config.FieldError{Field: apiField(problem.Field), Message: problem.Message})
	}
	return out, problems
}

var apiFields = map[string]string{
	"url":                         "address",
	"addr":                        "address",
	"basicAuth.username":          "username",
	"basicAuth.password":          "secrets.password",
	"basicAuth":                   "secrets.bearer_token",
	"bearerToken":                 "secrets.bearer_token",
	"password":                    "secrets.password",
	"apiKey":                      "secrets.api_key",
	"scrapeInterval":              "scrape_interval",
	"tenantId":                    "tenant_id",
	"timeField":                   "time_field",
	"queryLanguage":               "query_language",
	"fields.traceId":              "fields.trace_id",
	"fields.spanId":               "fields.span_id",
	"limits.maxSeries":            "limits.max_series",
	"limits.maxPoints":            "limits.max_points",
	"limits.maxRows":              "limits.max_rows",
	"limits.maxConcurrentQueries": "limits.max_concurrent_queries",
}

func apiField(field string) string {
	if name, ok := apiFields[field]; ok {
		return name
	}
	return field
}

func (d Definition) search(conn config.HTTPConnection, secrets Secrets) config.SearchDatasource {
	return config.SearchDatasource{
		HTTPConnection: conn,
		APIKey:         secret(secrets.APIKey),
		Index:          d.Index,
		TimeField:      d.TimeField,
		QueryLanguage:  d.QueryLanguage,
		Fields: config.FieldRoles{
			Message: d.Fields.Message,
			Level:   d.Fields.Level,
			Service: d.Fields.Service,
			TraceID: d.Fields.TraceID,
			SpanID:  d.Fields.SpanID,
		},
		Spans: d.Spans,
	}
}

func secret(value string) datasource.Secret {
	if value == "" {
		return datasource.Secret{}
	}
	return datasource.NewSecret(value)
}

func secretHeaders(headers map[string]string) map[string]datasource.Secret {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]datasource.Secret, len(headers))
	for name, value := range headers {
		out[name] = datasource.NewSecret(value)
	}
	return out
}

func parseDuration(value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, errors.New("must be a duration such as 30s")
	}
	return d, nil
}
