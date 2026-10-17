package provision

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/clickhouse"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
)

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
			return nil, fmt.Errorf("datasource %q: %w", src.UID, err)
		}
		instances = append(instances, datasource.Instance{UID: src.UID, Name: src.Name, Datasource: ds})
	}
	registry, err := datasource.NewRegistry(logger, instances...)
	if err != nil {
		closeAll()
		return nil, err
	}
	return registry, nil
}

func build(src config.Datasource) (datasource.Datasource, error) {
	switch {
	case src.Prometheus != nil:
		p := src.Prometheus
		ds, err := prometheus.New(prometheus.Options{
			URL:               p.URL,
			ScrapeInterval:    p.ScrapeInterval,
			Headers:           p.Headers,
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
	default:
		return nil, errors.New("no datasource block configured")
	}
}
