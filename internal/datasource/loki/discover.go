package loki

import (
	"context"
	"encoding/json"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	discoverTimeout     = 10 * time.Second
	maxVariableValues   = 10_000
	variableQueryUsage  = "unsupported variable query; use label_values(label) or label_values({selector}, label)"
	sampleWindow        = 15 * time.Minute
	defaultSampleLines  = 5
	maxSampleLines      = 20
	inventoryWindow     = time.Hour
	maxInventoryStreams = 50_000
)

var preferredServiceLabels = []string{"service_name", "job"}

var labelValuesQuery = regexp.MustCompile(`(?s)^label_values\s*\((.*)\)$`)

func (d *Datasource) Discover(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	switch req.Kind {
	case datasource.DiscoverMetrics:
		return &datasource.Catalog{}, nil
	case datasource.DiscoverLabels, datasource.DiscoverLabelValues, datasource.DiscoverSampleRows, datasource.DiscoverInventory:
	default:
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "discover kind %q is not supported by loki", truncateText(string(req.Kind)))
	}
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	switch req.Kind {
	case datasource.DiscoverSampleRows:
		return d.discoverSample(ctx, req)
	case datasource.DiscoverInventory:
		return d.discoverStreams(ctx, req)
	}
	limit := req.EffectiveLimit()
	if req.Kind == datasource.DiscoverLabels {
		params := url.Values{}
		if req.Metric != "" {
			params.Set("query", req.Metric)
		}
		names, err := d.getStrings(ctx, "/loki/api/v1/labels", params)
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

func (d *Datasource) discoverSample(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	n := req.Limit
	if n <= 0 {
		n = defaultSampleLines
	}
	n = min(n, maxSampleLines)
	end := time.Now()
	start := end.Add(-sampleWindow)
	selector := strings.TrimSpace(req.Table)
	if selector != "" && !strings.HasPrefix(selector, "{") {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "on loki, table must be a stream selector such as {service_name=\"payments\"}")
	}
	if selector == "" {
		names, err := d.getStrings(ctx, "/loki/api/v1/labels", url.Values{"start": {lokiTime(start)}, "end": {lokiTime(end)}})
		if err != nil {
			return nil, err
		}
		name, ok := sampleLabel(names)
		if !ok {
			return &datasource.Catalog{Sample: &datasource.Frame{}}, nil
		}
		selector = "{" + name + `=~".+"}`
	}
	params := url.Values{
		"query":     {selector},
		"start":     {lokiTime(start)},
		"end":       {lokiTime(end)},
		"limit":     {strconv.Itoa(n)},
		"direction": {"backward"},
	}
	frames, err := d.run(ctx, "/loki/api/v1/query_range", params, d.opts.Limits)
	if err != nil {
		return nil, err
	}
	merged := linesFrame(frames, n)
	return &datasource.Catalog{Sample: &merged}, nil
}

func sampleLabel(names []string) (string, bool) {
	names = slices.DeleteFunc(slices.Clone(names), func(name string) bool { return strings.HasPrefix(name, "__") })
	for _, preferred := range preferredServiceLabels {
		if slices.Contains(names, preferred) {
			return preferred, true
		}
	}
	if len(names) == 0 {
		return "", false
	}
	return slices.Min(names), true
}

type lineRef struct {
	at    time.Time
	frame int
	row   int
}

func linesFrame(frames []datasource.Frame, n int) datasource.Frame {
	keys := map[string]struct{}{}
	withLevel := false
	var refs []lineRef
	for i, f := range frames {
		for k := range f.Labels {
			keys[k] = struct{}{}
		}
		if len(f.Fields) > 2 {
			withLevel = true
		}
		for row, v := range f.Fields[0].Values {
			at, _ := v.(time.Time)
			refs = append(refs, lineRef{at: at, frame: i, row: row})
		}
	}
	slices.SortStableFunc(refs, func(a, b lineRef) int { return b.at.Compare(a.at) })
	refs = refs[:min(n, len(refs))]
	names := slices.Sorted(maps.Keys(keys))
	column := func(name string) datasource.Field {
		return datasource.Field{Name: name, Type: datasource.FieldString, Values: make([]any, 0, len(refs))}
	}
	fields := []datasource.Field{{Name: "Time", Type: datasource.FieldTime, Values: make([]any, 0, len(refs))}, column("Body")}
	if withLevel {
		fields = append(fields, column("level"))
	}
	labelsAt := len(fields)
	for _, name := range names {
		fields = append(fields, column(name))
	}
	for _, ref := range refs {
		f := frames[ref.frame]
		fields[0].Values = append(fields[0].Values, f.Fields[0].Values[ref.row])
		fields[1].Values = append(fields[1].Values, f.Fields[1].Values[ref.row])
		if withLevel {
			var level any
			if len(f.Fields) > 2 {
				level = f.Fields[2].Values[ref.row]
			}
			fields[2].Values = append(fields[2].Values, level)
		}
		for i, name := range names {
			var value any
			if v, ok := f.Labels[name]; ok {
				value = v
			}
			fields[labelsAt+i].Values = append(fields[labelsAt+i].Values, value)
		}
	}
	return datasource.Frame{Fields: fields}
}

func (d *Datasource) discoverStreams(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	scope := strings.TrimSpace(req.Scope)
	if !strings.HasPrefix(scope, "{") {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "scope must be a stream selector such as {service_name=\"checkout\"}")
	}
	end := time.Now()
	raw, err := d.get(ctx, "/loki/api/v1/series", url.Values{
		"match[]": {scope},
		"start":   {lokiTime(end.Add(-inventoryWindow))},
		"end":     {lokiTime(end)},
	})
	if err != nil {
		return nil, err
	}
	var streams []map[string]string
	if err := json.Unmarshal(raw, &streams); err != nil {
		return nil, unreadable()
	}
	truncated := len(streams) > maxInventoryStreams
	streams = streams[:min(len(streams), maxInventoryStreams)]
	counts := map[string]int{}
	for _, labels := range streams {
		for k := range labels {
			if !strings.HasPrefix(k, "__") {
				counts[k]++
			}
		}
	}
	families := make([]datasource.Family, 0, len(counts))
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		families = append(families, datasource.Family{Name: name, Type: "label", Series: counts[name]})
	}
	return &datasource.Catalog{Families: families, FamiliesTotal: len(families), Truncated: truncated}, nil
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
