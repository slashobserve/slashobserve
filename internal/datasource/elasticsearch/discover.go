package elasticsearch

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	discoverTimeout    = 10 * time.Second
	discoverWindow     = 24 * time.Hour
	sampleWindow       = 15 * time.Minute
	defaultSampleRows  = 5
	maxSampleRows      = 20
	maxVariableValues  = 10_000
	variableQueryUsage = "unsupported variable query; use label_values(field)"
)

var labelValuesQuery = regexp.MustCompile(`(?s)^label_values\s*\((.*)\)$`)

func (d *Datasource) Discover(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	switch req.Kind {
	case datasource.DiscoverMetrics, datasource.DiscoverInventory:
		return &datasource.Catalog{}, nil
	case datasource.DiscoverLabels, datasource.DiscoverLabelValues, datasource.DiscoverTables, datasource.DiscoverColumns, datasource.DiscoverSampleRows:
	default:
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "discover kind %q is not supported by %s", truncateText(string(req.Kind)), d.opts.Flavor)
	}
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	switch req.Kind {
	case datasource.DiscoverTables:
		return d.discoverTables(ctx, req)
	case datasource.DiscoverSampleRows:
		return d.discoverSample(ctx, req)
	}
	m, err := d.Mapping(ctx)
	if err != nil {
		return nil, err
	}
	switch req.Kind {
	case datasource.DiscoverLabels:
		var names []string
		for name, f := range m.Fields {
			if f.Aggregatable || f.Keyword != "" {
				names = append(names, name)
			}
		}
		return &datasource.Catalog{Labels: filterSortLimit(names, req.Search, req.EffectiveLimit())}, nil
	case datasource.DiscoverColumns:
		names := filterSortLimit(m.Names(), req.Search, req.EffectiveLimit())
		columns := make([]datasource.Column, len(names))
		for i, name := range names {
			columns[i] = datasource.Column{Name: name, Type: string(m.Fields[name].Kind)}
		}
		return &datasource.Catalog{Columns: columns}, nil
	}
	return d.discoverLabelValues(ctx, req, m)
}

func (d *Datasource) discoverLabelValues(ctx context.Context, req datasource.DiscoverRequest, m *Mapping) (*datasource.Catalog, error) {
	field, err := m.termsField(req.Label)
	if err != nil {
		return nil, err
	}
	r := req.Range
	if r.From.IsZero() {
		r.To = time.Now()
		r.From = r.To.Add(-discoverWindow)
	}
	limit := req.EffectiveLimit()
	size := limit
	if req.Search != "" {
		size = datasource.MaxDiscoverLimit
	}
	values, _, err := d.termsValues(ctx, field, r, size)
	if err != nil {
		return nil, err
	}
	return &datasource.Catalog{Values: filterSortLimit(values, req.Search, limit)}, nil
}

func (d *Datasource) discoverTables(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	tables, err := d.resolveIndex(ctx)
	if err != nil {
		return nil, err
	}
	search := strings.ToLower(req.Search)
	tables = slices.DeleteFunc(tables, func(t datasource.Table) bool { return !strings.Contains(strings.ToLower(t.Name), search) })
	return &datasource.Catalog{Tables: tables[:min(len(tables), req.EffectiveLimit())]}, nil
}

func (d *Datasource) discoverSample(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	n := req.Limit
	if n <= 0 {
		n = defaultSampleRows
	}
	n = min(n, maxSampleRows)
	roles, err := d.Roles(ctx)
	if err != nil {
		return nil, err
	}
	end := time.Now()
	raw, err := d.search(ctx, map[string]any{
		"size":             n,
		"track_total_hits": false,
		"query":            d.rangeQuery(datasource.TimeRange{From: end.Add(-sampleWindow), To: end}),
		"sort":             []map[string]any{{d.opts.TimeField: map[string]any{"order": "desc", "unmapped_type": "date"}}},
	})
	if err != nil {
		return nil, err
	}
	hits, err := parseHits(raw)
	if err != nil {
		return nil, err
	}
	frame := hitsFrame(hits, roles)
	return &datasource.Catalog{Sample: &frame}, nil
}

func (d *Datasource) termsValues(ctx context.Context, field string, r datasource.TimeRange, size int) ([]string, bool, error) {
	raw, err := d.search(ctx, map[string]any{
		"size":             0,
		"track_total_hits": false,
		"query":            d.rangeQuery(r),
		"aggs":             map[string]any{"values": map[string]any{"terms": map[string]any{"field": field, "size": size}}},
	})
	if err != nil {
		return nil, false, err
	}
	var resp struct {
		Aggregations struct {
			Values struct {
				Buckets []struct {
					Key         json.RawMessage `json:"key"`
					KeyAsString string          `json:"key_as_string"`
				} `json:"buckets"`
				SumOtherDocCount int64 `json:"sum_other_doc_count"`
			} `json:"values"`
		} `json:"aggregations"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, false, unreadable()
	}
	values := make([]string, 0, len(resp.Aggregations.Values.Buckets))
	for _, b := range resp.Aggregations.Values.Buckets {
		values = append(values, bucketKey(b.Key, b.KeyAsString))
	}
	return values, resp.Aggregations.Values.SumOtherDocCount > 0, nil
}

func bucketKey(key json.RawMessage, asString string) string {
	if asString != "" {
		return asString
	}
	var s string
	if json.Unmarshal(key, &s) == nil {
		return s
	}
	return string(key)
}

func (d *Datasource) VariableValues(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]string, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(q.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := d.expand(q, r, instantInterval(r, q.MinInterval))
	if err != nil {
		return nil, err
	}
	match := labelValuesQuery.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", variableQueryUsage)
	}
	name := strings.TrimSpace(match[1])
	if name == "" || strings.ContainsAny(name, ", \t\n\"()") {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", variableQueryUsage)
	}
	m, err := d.Mapping(ctx)
	if err != nil {
		return nil, err
	}
	field, err := m.termsField(name)
	if err != nil {
		return nil, err
	}
	values, more, err := d.termsValues(ctx, field, r, maxVariableValues)
	if err != nil {
		return nil, err
	}
	if more {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "variable returned more than %d values", maxVariableValues)
	}
	slices.Sort(values)
	return slices.Compact(values), nil
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
	return out[:min(len(out), limit)]
}
