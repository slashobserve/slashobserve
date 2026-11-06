package provision

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/clickhouse"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
	"github.com/slashobserve/slashobserve/internal/datasource/tempo"
)

// BuildError reports which datasource failed to build. Message is the part of the failure the server wrote itself,
// empty when the cause could carry a driver or library message.
type BuildError struct {
	UID     string
	Message string
	Err     error
}

func (e *BuildError) Error() string { return fmt.Sprintf("datasource %q: %v", e.UID, e.Err) }
func (e *BuildError) Unwrap() error { return e.Err }

func serverMessage(err error) string {
	var configErr *datasource.ConfigError
	if errors.As(err, &configErr) {
		return configErr.Message
	}
	return ""
}

func Build(logger *slog.Logger, sources []config.Datasource) (*datasource.Registry, error) {
	instances := make([]datasource.Instance, 0, len(sources))
	closeAll := func() {
		for _, in := range instances {
			_ = in.Datasource.Close()
		}
	}
	for _, src := range sources {
		ds, err := build(src)
		if err != nil {
			closeAll()
			return nil, &BuildError{UID: src.UID, Message: serverMessage(err), Err: err}
		}
		instances = append(instances, datasource.Instance{UID: src.UID, Name: src.Name, GrafanaUIDs: src.GrafanaUIDs, Datasource: ds})
	}
	registry, err := datasource.NewRegistry(logger, instances...)
	if err != nil {
		closeAll()
		return nil, err
	}
	return registry, nil
}

func mergeHeaders(plain map[string]string, secret map[string]datasource.Secret) map[string]string {
	if len(plain)+len(secret) == 0 {
		return nil
	}
	out := make(map[string]string, len(plain)+len(secret))
	maps.Copy(out, plain)
	for name, value := range secret {
		out[name] = value.Reveal()
	}
	return out
}

func build(src config.Datasource) (datasource.Datasource, error) {
	switch {
	case src.Prometheus != nil:
		p := src.Prometheus
		ds, err := prometheus.New(prometheus.Options{
			URL:               p.URL,
			ScrapeInterval:    p.ScrapeInterval,
			Headers:           mergeHeaders(p.Headers, p.SecretHeaders),
			BasicAuthUser:     p.BasicAuthUser,
			BasicAuthPassword: p.BasicAuthPassword,
			BearerToken:       p.BearerToken,
			Limits:            src.Limits,
		})
		if err != nil {
			return nil, err
		}
		return ds, nil
	case src.ClickHouse != nil:
		c := src.ClickHouse
		ds, err := clickhouse.New(clickhouse.Options{
			Addr:     c.Addr,
			Database: c.Database,
			Username: c.Username,
			Password: c.Password,
			TLS:      c.TLS,
			Limits:   src.Limits,
		})
		if err != nil {
			return nil, err
		}
		return ds, nil
	case src.Loki != nil:
		l := src.Loki
		ds, err := loki.New(loki.Options{
			URL:               l.URL,
			Headers:           mergeHeaders(l.Headers, l.SecretHeaders),
			BasicAuthUser:     l.BasicAuthUser,
			BasicAuthPassword: l.BasicAuthPassword,
			BearerToken:       l.BearerToken,
			TenantID:          l.TenantID,
			Limits:            src.Limits,
		})
		if err != nil {
			return nil, err
		}
		return ds, nil
	case src.Tempo != nil:
		t := src.Tempo
		ds, err := tempo.New(tempo.Options{
			URL:               t.URL,
			Headers:           mergeHeaders(t.Headers, t.SecretHeaders),
			BasicAuthUser:     t.BasicAuthUser,
			BasicAuthPassword: t.BasicAuthPassword,
			BearerToken:       t.BearerToken,
			Limits:            src.Limits,
		})
		if err != nil {
			return nil, err
		}
		return ds, nil
	case src.Elasticsearch != nil:
		return newSearch(elasticsearch.Elasticsearch, src.Elasticsearch, src.Limits)
	case src.OpenSearch != nil:
		return newSearch(elasticsearch.OpenSearch, src.OpenSearch, src.Limits)
	default:
		return nil, errors.New("no datasource block configured")
	}
}

func newSearch(flavor elasticsearch.Flavor, s *config.SearchDatasource, limits datasource.Limits) (datasource.Datasource, error) {
	ds, err := elasticsearch.New(elasticsearch.Options{
		Flavor:            flavor,
		URL:               s.URL,
		Headers:           mergeHeaders(s.Headers, s.SecretHeaders),
		BasicAuthUser:     s.BasicAuthUser,
		BasicAuthPassword: s.BasicAuthPassword,
		BearerToken:       s.BearerToken,
		APIKey:            s.APIKey,
		Index:             s.Index,
		TimeField:         s.TimeField,
		QueryLanguage:     s.QueryLanguage,
		Fields: elasticsearch.FieldRoles{
			Message: s.Fields.Message,
			Level:   s.Fields.Level,
			Service: s.Fields.Service,
			TraceID: s.Fields.TraceID,
			SpanID:  s.Fields.SpanID,
		},
		Limits: limits,
	})
	if err != nil {
		return nil, err
	}
	return ds, nil
}
