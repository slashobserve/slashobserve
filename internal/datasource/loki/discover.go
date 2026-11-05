package loki

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	discoverTimeout    = 10 * time.Second
	maxVariableValues  = 10_000
	variableQueryUsage = "unsupported variable query; use label_values(label) or label_values({selector}, label)"
)

var labelValuesQuery = regexp.MustCompile(`(?s)^label_values\s*\((.*)\)$`)

func (d *Datasource) Discover(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	switch req.Kind {
	case datasource.DiscoverMetrics:
		return &datasource.Catalog{}, nil
	case datasource.DiscoverLabels, datasource.DiscoverLabelValues:
	default:
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "discover kind %q is not supported by loki", truncateText(string(req.Kind)))
	}
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	limit := req.EffectiveLimit()
	if req.Kind == datasource.DiscoverLabels {
		names, err := d.getStrings(ctx, "/loki/api/v1/labels", nil)
		if err != nil {
			return nil, err
		}
		return &datasource.Catalog{Labels: filterSortLimit(names, req.Search, limit)}, nil
	}
	if err := validateLabel(req.Label); err != nil {
		return nil, err
	}
	params := url.Values{}
	if req.Metric != "" {
		params.Set("query", req.Metric)
	}
	values, err := d.getStrings(ctx, labelValuesPath(req.Label), params)
	if err != nil {
		return nil, err
	}
	return &datasource.Catalog{Values: filterSortLimit(values, req.Search, limit)}, nil
}

func (d *Datasource) VariableValues(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]string, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(q.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := expand(q, r, instantInterval(r, q.MinInterval))
	if err != nil {
		return nil, err
	}
	match := labelValuesQuery.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", variableQueryUsage)
	}
	selector, label := splitLabelValuesArgs(match[1])
	if err := validateLabel(label); err != nil {
		return nil, err
	}
	params := url.Values{"start": {lokiTime(r.From)}, "end": {lokiTime(r.To)}}
	if selector != "" {
		params.Set("query", selector)
	}
	values, err := d.getStrings(ctx, labelValuesPath(label), params)
	if err != nil {
		return nil, err
	}
	slices.Sort(values)
	values = slices.Compact(values)
	if len(values) > maxVariableValues {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "variable returned %d values, max %d", len(values), maxVariableValues)
	}
	return values, nil
}

func labelValuesPath(label string) string {
	return "/loki/api/v1/label/" + url.PathEscape(label) + "/values"
}

func validateLabel(label string) error {
	if !spec.IsLabelName(label) {
		return datasource.Errorf(datasource.ErrInvalidQuery, "label must match [A-Za-z_][A-Za-z0-9_]*")
	}
	return nil
}

func splitLabelValuesArgs(args string) (selector, label string) {
	depth := 0
	last := -1
	var quote byte
	for i := 0; i < len(args); i++ {
		c := args[i]
		if quote != 0 {
			switch {
			case c == '\\' && quote != '`':
				i++
			case c == quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '`':
			quote = c
		case '{', '(':
			depth++
		case '}', ')':
			depth--
		case ',':
			if depth == 0 {
				last = i
			}
		}
	}
	if last < 0 {
		return "", strings.TrimSpace(args)
	}
	return strings.TrimSpace(args[:last]), strings.TrimSpace(args[last+1:])
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

func (d *Datasource) getStrings(ctx context.Context, path string, params url.Values) ([]string, error) {
	raw, err := d.get(ctx, path, params)
	if err != nil {
		return nil, err
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, unreadable()
	}
	return values, nil
}
