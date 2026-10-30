package prometheus

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type exemplar struct {
	Labels    map[string]string `json:"labels"`
	Value     string            `json:"value"`
	Timestamp float64           `json:"timestamp"`
}

type exemplarSeries struct {
	SeriesLabels map[string]string `json:"seriesLabels"`
	Exemplars    []exemplar        `json:"exemplars"`
}

func (d *Datasource) QueryExemplars(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if step < time.Millisecond {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "step must be at least 1ms")
	}
	lim := d.opts.Limits.Tighten(q.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := d.expand(q, r, step)
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"query": {text},
		"start": {unixSeconds(r.From)},
		"end":   {unixSeconds(r.To)},
	}
	raw, err := d.do(ctx, http.MethodPost, "/api/v1/query_exemplars", form)
	if err != nil {
		return nil, err
	}
	return exemplarFrames(raw)
}

func exemplarFrames(raw json.RawMessage) ([]datasource.Frame, error) {
	var groups []exemplarSeries
	if hasContent(raw) {
		if err := json.Unmarshal(raw, &groups); err != nil {
			return nil, datasource.Errorf(datasource.ErrUnavailable, "upstream returned an unreadable result")
		}
	}
	type keyed struct {
		key   string
		frame datasource.Frame
	}
	var out []keyed
	for _, g := range groups {
		if len(g.Exemplars) == 0 {
			continue
		}
		f := exemplarFrame(g)
		out = append(out, keyed{key: labelKey(f.Labels), frame: f})
	}
	slices.SortStableFunc(out, func(a, b keyed) int {
		if c := strings.Compare(a.frame.Name, b.frame.Name); c != 0 {
			return c
		}
		return strings.Compare(a.key, b.key)
	})
	frames := make([]datasource.Frame, len(out))
	for i, k := range out {
		frames[i] = k.frame
	}
	return frames, nil
}

func exemplarFrame(g exemplarSeries) datasource.Frame {
	f := datasource.Frame{}
	for k, v := range g.SeriesLabels {
		if k == "__name__" {
			f.Name = v
			continue
		}
		if f.Labels == nil {
			f.Labels = map[string]string{}
		}
		f.Labels[k] = v
	}
	keys := exemplarKeys(g.Exemplars)
	f.Fields = make([]datasource.Field, 0, 2+len(keys))
	f.Fields = append(f.Fields,
		datasource.Field{Name: "Time", Type: datasource.FieldTime, Values: make([]any, 0, len(g.Exemplars))},
		datasource.Field{Name: "Value", Type: datasource.FieldNumber, Values: make([]any, 0, len(g.Exemplars))},
	)
	for _, k := range keys {
		f.Fields = append(f.Fields, datasource.Field{Name: k, Type: datasource.FieldString, Values: make([]any, 0, len(g.Exemplars))})
	}
	for _, e := range sortedByTime(g.Exemplars) {
		f.Fields[0].Values = append(f.Fields[0].Values, time.UnixMilli(int64(math.Round(e.Timestamp*1000))).UTC())
		f.Fields[1].Values = append(f.Fields[1].Values, exemplarValue(e.Value))
		for i, k := range keys {
			f.Fields[2+i].Values = append(f.Fields[2+i].Values, e.Labels[k])
		}
	}
	return f
}

func exemplarKeys(exemplars []exemplar) []string {
	seen := map[string]struct{}{}
	for _, e := range exemplars {
		for k := range e.Labels {
			seen[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func sortedByTime(exemplars []exemplar) []exemplar {
	sorted := slices.Clone(exemplars)
	slices.SortStableFunc(sorted, func(a, b exemplar) int {
		switch {
		case a.Timestamp < b.Timestamp:
			return -1
		case a.Timestamp > b.Timestamp:
			return 1
		}
		return 0
	})
	return sorted
}

func exemplarValue(s string) any {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return datasource.Number(v)
}
