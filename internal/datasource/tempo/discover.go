package tempo

import (
	"context"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	discoverTimeout        = 10 * time.Second
	intrinsicScope         = "intrinsic"
	serviceTag             = "resource.service.name"
	maxSchemaServices      = 30
	maxSpanNamesPerService = 50
	schemaConcurrency      = 4
)

var tagName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.:-]*$`)

type tagsResponse struct {
	Scopes []struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	} `json:"scopes"`
}

type tagValuesResponse struct {
	TagValues []struct {
		Value string `json:"value"`
	} `json:"tagValues"`
}

func (d *Datasource) Discover(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	switch req.Kind {
	case datasource.DiscoverLabels, datasource.DiscoverLabelValues, datasource.DiscoverTables:
	default:
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "discover kind %q is not supported by tempo", truncateText(string(req.Kind)))
	}
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	limit := req.EffectiveLimit()
	switch req.Kind {
	case datasource.DiscoverTables:
		return d.discoverSpans(ctx, req.Range)
	case datasource.DiscoverLabels:
		names, err := d.tagNames(ctx, req.Metric, req.Range)
		if err != nil {
			return nil, err
		}
		return &datasource.Catalog{Labels: filterSortLimit(names, req.Search, limit)}, nil
	}
	values, err := d.tagValues(ctx, req.Label, req.Metric, req.Range)
	if err != nil {
		return nil, err
	}
	return &datasource.Catalog{Values: filterSortLimit(values, req.Search, limit)}, nil
}

func tagParams(q string, r datasource.TimeRange) url.Values {
	params := url.Values{}
	if q != "" {
		params.Set("q", q)
	}
	if !r.From.IsZero() {
		params.Set("start", strconv.FormatInt(r.From.Unix(), 10))
		params.Set("end", strconv.FormatInt(ceilSeconds(r.To), 10))
	}
	return params
}

func (d *Datasource) tagNames(ctx context.Context, q string, r datasource.TimeRange) ([]string, error) {
	params := tagParams(q, r)
	var resp tagsResponse
	if err := d.getJSON(ctx, "/api/v2/search/tags", params, &resp); err != nil {
		return nil, err
	}
	var names []string
	for _, scope := range resp.Scopes {
		for _, tag := range scope.Tags {
			if scope.Name == intrinsicScope {
				names = append(names, tag)
			} else {
				names = append(names, scope.Name+"."+tag)
			}
		}
	}
	return names, nil
}

func (d *Datasource) tagValues(ctx context.Context, tag, q string, r datasource.TimeRange) ([]string, error) {
	if !tagName.MatchString(tag) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "tag must be a scoped TraceQL attribute such as resource.service.name")
	}
	params := tagParams(q, r)
	var resp tagValuesResponse
	if err := d.getJSON(ctx, "/api/v2/search/tag/"+url.PathEscape(tag)+"/values", params, &resp); err != nil {
		return nil, err
	}
	values := make([]string, 0, len(resp.TagValues))
	for _, v := range resp.TagValues {
		values = append(values, v.Value)
	}
	return values, nil
}

func (d *Datasource) discoverSpans(ctx context.Context, r datasource.TimeRange) (*datasource.Catalog, error) {
	attributes, err := d.tagNames(ctx, "", r)
	if err != nil {
		return nil, err
	}
	services, err := d.tagValues(ctx, serviceTag, "", r)
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
			names, err := d.tagValues(gctx, "name", "{ "+serviceTag+` = "`+traceQLString(service)+`" }`, r)
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
		Labels:    sortedUnique(attributes),
		Truncated: truncated,
	}, nil
}

func traceQLString(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

func sortedUnique(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}

func (d *Datasource) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return nil, datasource.Errorf(datasource.ErrInvalidQuery, "query variables are not supported by tempo")
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
