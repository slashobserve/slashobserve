package prometheus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	discoverTimeout     = 10 * time.Second
	maxVariableValues   = 10_000
	maxMetadataMetrics  = 2000
	maxTargetedMetadata = 10
	metricNameLabel     = "__name__"
	variableQueryUsage  = "unsupported variable query; use label_values(...)"
)

var (
	labelValuesQuery = regexp.MustCompile(`(?s)^label_values\s*\((.*)\)$`)
)

type metricMetadata struct {
	Type string `json:"type"`
	Help string `json:"help"`
	Unit string `json:"unit"`
}

func (d *Datasource) Discover(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	switch req.Kind {
	case datasource.DiscoverMetrics, datasource.DiscoverLabels, datasource.DiscoverLabelValues:
	default:
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "discover kind %q is not supported by prometheus", truncateText(string(req.Kind)))
	}
	if req.Kind == datasource.DiscoverLabelValues {
		if err := validateLabel(req.Label); err != nil {
			return nil, err
		}
	}
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	now := time.Now()
	form := url.Values{
		"start": {unixSeconds(now.Add(-lookback))},
		"end":   {unixSeconds(now)},
	}
	if req.Metric != "" && req.Kind != datasource.DiscoverMetrics {
		form.Set("match[]", metricMatcher(req.Metric))
	}
	limit := req.EffectiveLimit()
	switch req.Kind {
	case datasource.DiscoverMetrics:
		return d.discoverMetrics(ctx, form, req.Search, limit)
	case datasource.DiscoverLabels:
		names, err := d.getStrings(ctx, "/api/v1/labels", form)
		if err != nil {
			return nil, err
		}
		names = slices.DeleteFunc(names, func(n string) bool { return n == metricNameLabel })
		return &datasource.Catalog{Labels: filterSortLimit(names, req.Search, limit)}, nil
	}
	values, err := d.getStrings(ctx, "/api/v1/label/"+url.PathEscape(req.Label)+"/values", form)
	if err != nil {
		return nil, err
	}
	return &datasource.Catalog{Values: filterSortLimit(values, req.Search, limit)}, nil
}

func (d *Datasource) discoverMetrics(ctx context.Context, form url.Values, search string, limit int) (*datasource.Catalog, error) {
	metaCtx, cancelMeta := context.WithCancel(ctx)
	defer cancelMeta()
	var (
		wg       sync.WaitGroup
		metadata map[string][]metricMetadata
	)
	wg.Go(func() {
		metadata = d.metadata(metaCtx, url.Values{"limit": {strconv.Itoa(maxMetadataMetrics)}})
	})
	names, err := d.getStrings(ctx, "/api/v1/label/__name__/values", form)
	if err != nil {
		cancelMeta()
		wg.Wait()
		return nil, err
	}
	names = filterSortLimit(names, search, limit)
	wg.Wait()
	if metadata == nil {
		metadata = map[string][]metricMetadata{}
	}
	d.completeMetadata(ctx, metadata, names)
	metrics := make([]datasource.Metric, len(names))
	for i, name := range names {
		metrics[i] = datasource.Metric{Name: name}
		if md, ok := metadata[name]; ok && len(md) > 0 {
			metrics[i].Type, metrics[i].Help, metrics[i].Unit = md[0].Type, md[0].Help, md[0].Unit
		}
	}
	return &datasource.Catalog{Metrics: metrics}, nil
}

func (d *Datasource) completeMetadata(ctx context.Context, metadata map[string][]metricMetadata, names []string) {
	var missing []string
	for _, name := range names {
		if len(metadata[name]) == 0 {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return
	}
	missing = missing[:min(len(missing), maxTargetedMetadata)]
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	for _, name := range missing {
		wg.Go(func() {
			found := d.metadata(ctx, url.Values{"metric": {name}})
			mu.Lock()
			defer mu.Unlock()
			if md := found[name]; len(md) > 0 {
				metadata[name] = md
			}
		})
	}
	wg.Wait()
}

func (d *Datasource) metadata(ctx context.Context, form url.Values) map[string][]metricMetadata {
	raw, err := d.do(ctx, http.MethodGet, "/api/v1/metadata", form)
	if err != nil {
		return nil
	}
	var out map[string][]metricMetadata
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
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

func validateLabel(label string) error {
	if !spec.IsLabelName(label) {
		return datasource.Errorf(datasource.ErrInvalidQuery, "label must match [A-Za-z_][A-Za-z0-9_]*")
	}
	return nil
}

func (d *Datasource) VariableValues(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]string, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(q.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	exp, err := macros.Expand(macros.PromQL, q.Text, macros.Env{
		Range:          r,
		Interval:       d.variableInterval(r),
		ScrapeInterval: d.opts.ScrapeInterval,
		Variables:      q.Variables,
		Dashboard:      q.Dashboard,
		User:           q.User,
	})
	if err != nil {
		return nil, err
	}
	match := labelValuesQuery.FindStringSubmatch(strings.TrimSpace(exp.Text))
	if match == nil {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", variableQueryUsage)
	}
	selector, label := splitLabelValuesArgs(match[1])
	if err := validateLabel(label); err != nil {
		return nil, err
	}
	form := url.Values{
		"start": {unixSeconds(r.From)},
		"end":   {unixSeconds(r.To)},
	}
	if selector != "" {
		if _, err := promParser.ParseMetricSelector(selector); err != nil {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", truncateText(err.Error()))
		}
		form.Set("match[]", selector)
	}
	values, err := d.getStrings(ctx, "/api/v1/label/"+url.PathEscape(label)+"/values", form)
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

func splitLabelValuesArgs(args string) (selector, label string) {
	last := lastTopLevelComma(args)
	if last < 0 {
		return "", strings.TrimSpace(args)
	}
	return strings.TrimSpace(args[:last]), strings.TrimSpace(args[last+1:])
}

func lastTopLevelComma(s string) int {
	last := -1
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
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
		case '"', '\'', '`':
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
	return last
}
