package jaeger

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	discoverTimeout        = 10 * time.Second
	maxSchemaServices      = 30
	maxSpanNamesPerService = 50
	schemaConcurrency      = 4

	ServiceKey = keyService
)

var searchKeys = []string{keyOperation, keyService}

type servicesResponse struct {
	Services []string `json:"services"`
}

type operationsResponse struct {
	Operations []struct {
		Name string `json:"name"`
	} `json:"operations"`
}

func (d *Datasource) Discover(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	switch req.Kind {
	case datasource.DiscoverLabels, datasource.DiscoverLabelValues, datasource.DiscoverTables:
	default:
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "discover kind %q is not supported by jaeger", truncateText(string(req.Kind)))
	}
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	switch req.Kind {
	case datasource.DiscoverTables:
		return d.discoverSpans(ctx)
	case datasource.DiscoverLabels:
		return &datasource.Catalog{Labels: filterSortLimit(searchKeys, req.Search, req.EffectiveLimit())}, nil
	}
	values, err := d.labelValues(ctx, req)
	if err != nil {
		return nil, err
	}
	return &datasource.Catalog{Values: filterSortLimit(values, req.Search, req.EffectiveLimit())}, nil
}

func (d *Datasource) labelValues(ctx context.Context, req datasource.DiscoverRequest) ([]string, error) {
	switch req.Label {
	case keyService:
		return d.services(ctx)
	case keyOperation:
		s, err := searchOf(req.Metric)
		if err != nil {
			return nil, err
		}
		if s.service == "" {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "operation values need a service: pass metric as service=checkout")
		}
		return d.operations(ctx, s.service)
	}
	return nil, datasource.Errorf(datasource.ErrInvalidQuery, "a Jaeger datasource lists the values of %s and %s; other tags are free-form", keyService, keyOperation)
}

func (d *Datasource) services(ctx context.Context) ([]string, error) {
	var resp servicesResponse
	if err := d.getJSON(ctx, "/api/v3/services", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Services, nil
}

func (d *Datasource) operations(ctx context.Context, service string) ([]string, error) {
	var resp operationsResponse
	if err := d.getJSON(ctx, "/api/v3/operations", url.Values{"service": {service}}, &resp); err != nil {
		return nil, err
	}
	names := make([]string, len(resp.Operations))
	for i, op := range resp.Operations {
		names[i] = op.Name
	}
	return names, nil
}

func (d *Datasource) discoverSpans(ctx context.Context) (*datasource.Catalog, error) {
	services, err := d.services(ctx)
	if err != nil {
		return nil, err
	}
	services = sortedUnique(services)
	truncated := len(services) > maxSchemaServices
	services = services[:min(len(services), maxSchemaServices)]
	spanNames := make([][]string, len(services))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(schemaConcurrency)
	for i, service := range services {
		g.Go(func() error {
			names, err := d.operations(gctx, service)
			spanNames[i] = sortedUnique(names)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	serviceColumn := datasource.Field{Name: "service", Type: datasource.FieldString, Values: []any{}}
	spanColumn := datasource.Field{Name: "span_name", Type: datasource.FieldString, Values: []any{}}
	for i, service := range services {
		names := spanNames[i]
		if len(names) > maxSpanNamesPerService {
			names, truncated = names[:maxSpanNamesPerService], true
		}
		for _, name := range names {
			serviceColumn.Values = append(serviceColumn.Values, service)
			spanColumn.Values = append(spanColumn.Values, name)
		}
	}
	return &datasource.Catalog{
		Sample:    &datasource.Frame{Fields: []datasource.Field{serviceColumn, spanColumn}},
		Labels:    slices.Clone(searchKeys),
		Truncated: truncated,
	}, nil
}

func sortedUnique(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}

func (d *Datasource) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return nil, datasource.Errorf(datasource.ErrInvalidQuery, "query variables are not supported by jaeger")
}

func filterSortLimit(values []string, search string, limit int) []string {
	search = strings.ToLower(search)
	out := make([]string, 0, len(values))
	for _, v := range values {
		if search == "" || strings.Contains(strings.ToLower(v), search) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
