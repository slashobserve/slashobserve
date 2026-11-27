package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const maxDatasourceNameRunes = 100

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e FieldError) Error() string { return e.Field + ": " + e.Message }

func (d Datasource) Validate() []FieldError {
	errs := append(d.validateCommon(), d.validateAliases()...)
	blocks := map[string]bool{
		typePrometheus:    d.Prometheus != nil,
		typeClickHouse:    d.ClickHouse != nil,
		typeLoki:          d.Loki != nil,
		typeTempo:         d.Tempo != nil,
		typeJaeger:        d.Jaeger != nil,
		typeElasticsearch: d.Elasticsearch != nil,
		typeOpenSearch:    d.OpenSearch != nil,
	}
	if _, known := blocks[d.Type]; !known {
		return append(errs, FieldError{"type", fmt.Sprintf("must be %s, %s, %s, %s, %s, %s or %s", typePrometheus, typeClickHouse, typeLoki, typeTempo, typeJaeger, typeElasticsearch, typeOpenSearch)})
	}
	if !blocks[d.Type] {
		return append(errs, FieldError{d.Type, "required for type " + d.Type})
	}
	for _, name := range slices.Sorted(maps.Keys(blocks)) {
		if blocks[name] && name != d.Type {
			errs = append(errs, FieldError{name, "not allowed for type " + d.Type})
		}
	}
	return append(errs, d.validateBlock()...)
}

func (d Datasource) validateCommon() []FieldError {
	var errs []FieldError
	if !datasource.ValidUID(d.UID) {
		errs = append(errs, FieldError{"uid", "must match " + datasource.UIDPattern})
	}
	if utf8.RuneCountInString(d.Name) > maxDatasourceNameRunes {
		errs = append(errs, FieldError{"name", fmt.Sprintf("must be at most %d characters", maxDatasourceNameRunes)})
	}
	if d.Limits.Timeout < 0 {
		errs = append(errs, FieldError{"limits.timeout", "must not be negative"})
	}
	for _, f := range []struct {
		name  string
		value int
	}{{"maxSeries", d.Limits.MaxSeries}, {"maxPoints", d.Limits.MaxPoints}, {"maxRows", d.Limits.MaxRows}, {"maxConcurrentQueries", d.MaxConcurrentQueries}} {
		if f.value < 0 {
			errs = append(errs, FieldError{"limits." + f.name, "must not be negative"})
		}
	}
	if d.MaxConcurrentQueries > maxConcurrentQueries {
		errs = append(errs, FieldError{"limits.maxConcurrentQueries", fmt.Sprintf("must be at most %d", maxConcurrentQueries)})
	}
	return errs
}

func (d Datasource) validateAliases() []FieldError {
	var errs []FieldError
	if len(d.GrafanaUIDs) > maxGrafanaUIDs {
		errs = append(errs, FieldError{"grafana_uids", fmt.Sprintf("at most %d entries", maxGrafanaUIDs)})
	}
	seen := map[string]bool{}
	for i, alias := range d.GrafanaUIDs {
		field := fmt.Sprintf("grafana_uids[%d]", i)
		switch problem := grafanaUIDProblem(alias); {
		case problem != "":
			errs = append(errs, FieldError{field, problem})
		case seen[alias]:
			errs = append(errs, FieldError{field, fmt.Sprintf("duplicate %q", alias)})
		}
		seen[alias] = true
	}
	return errs
}

func (d Datasource) validateBlock() []FieldError {
	switch d.Type {
	case typePrometheus:
		p := d.Prometheus
		errs := validateConnection(HTTPConnection{URL: p.URL, BasicAuthUser: p.BasicAuthUser, BasicAuthPassword: p.BasicAuthPassword, BearerToken: p.BearerToken})
		if p.ScrapeInterval < 0 {
			errs = append(errs, FieldError{"scrapeInterval", "must not be negative"})
		}
		return errs
	case typeLoki:
		return validateConnection(d.Loki.HTTPConnection)
	case typeTempo:
		return validateConnection(d.Tempo.HTTPConnection)
	case typeJaeger:
		return validateConnection(d.Jaeger.HTTPConnection)
	case typeElasticsearch:
		return validateSearch(*d.Elasticsearch)
	case typeOpenSearch:
		return validateSearch(*d.OpenSearch)
	default:
		return validateClickHouse(*d.ClickHouse)
	}
}

func authKinds(c HTTPConnection) (basic, bearer bool) {
	return c.BasicAuthUser != "" || !c.BasicAuthPassword.IsZero(), !c.BearerToken.IsZero()
}

func validateConnection(c HTTPConnection) []FieldError {
	var errs []FieldError
	if c.URL == "" {
		errs = append(errs, FieldError{"url", "required"})
	}
	hasBasic, hasBearer := authKinds(c)
	if hasBasic && hasBearer {
		errs = append(errs, FieldError{"basicAuth", "basicAuth and bearerToken are mutually exclusive"})
	}
	if hasBasic && c.BasicAuthUser == "" {
		errs = append(errs, FieldError{"basicAuth.username", "required"})
	}
	if hasBasic && c.BasicAuthPassword.IsZero() {
		errs = append(errs, FieldError{"basicAuth.password", "required"})
	}
	return errs
}

func validateSearch(s SearchDatasource) []FieldError {
	errs := validateConnection(s.HTTPConnection)
	if !s.APIKey.IsZero() {
		hasBasic, hasBearer := authKinds(s.HTTPConnection)
		if hasBasic {
			errs = append(errs, FieldError{"apiKey", "apiKey and basicAuth are mutually exclusive"})
		}
		if hasBearer {
			errs = append(errs, FieldError{"apiKey", "apiKey and bearerToken are mutually exclusive"})
		}
	}
	if problem := indexProblem(s.Index); problem != "" {
		errs = append(errs, FieldError{"index", problem})
	}
	for _, f := range []struct{ field, value string }{
		{"timeField", s.TimeField},
		{"fields.message", s.Fields.Message},
		{"fields.level", s.Fields.Level},
		{"fields.service", s.Fields.Service},
		{"fields.traceId", s.Fields.TraceID},
		{"fields.spanId", s.Fields.SpanID},
	} {
		if problem := fieldNameProblem(f.value); problem != "" {
			errs = append(errs, FieldError{f.field, problem})
		}
	}
	if s.QueryLanguage != "" && !slices.Contains(queryLanguages, s.QueryLanguage) {
		errs = append(errs, FieldError{"queryLanguage", "must be " + strings.Join(queryLanguages, " or ")})
	}
	if field, message, ok := strings.Cut(spansProblem(s.Spans, s.TimeField), ": "); ok {
		errs = append(errs, FieldError{field, message})
	}
	return errs
}

func validateClickHouse(c ClickHouseDatasource) []FieldError {
	var errs []FieldError
	if c.Addr == "" {
		errs = append(errs, FieldError{"addr", "required"})
	}
	if c.Username == "" {
		errs = append(errs, FieldError{"username", "required"})
	}
	if c.Password.IsZero() {
		errs = append(errs, FieldError{"password", "required"})
	}
	return errs
}
