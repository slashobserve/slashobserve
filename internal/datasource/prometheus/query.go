package prometheus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/prometheus/promql/parser"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const maxStepPoints = 11000

var promParser = parser.NewParser(parser.Options{})

type sample struct {
	at    time.Time
	value string
}

func (s *sample) UnmarshalJSON(b []byte) error {
	var pair [2]json.RawMessage
	if err := json.Unmarshal(b, &pair); err != nil {
		return err
	}
	ts, err := strconv.ParseFloat(string(pair[0]), 64)
	if err != nil {
		return err
	}
	s.at = time.UnixMilli(int64(math.Round(ts * 1000))).UTC()
	return json.Unmarshal(pair[1], &s.value)
}

type series struct {
	Metric     map[string]string `json:"metric"`
	Value      *sample           `json:"value"`
	Values     []sample          `json:"values"`
	Histogram  json.RawMessage   `json:"histogram"`
	Histograms json.RawMessage   `json:"histograms"`
}

type queryData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

func (d *Datasource) QueryRange(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if step < time.Millisecond {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "step must be at least 1ms")
	}
	lim := d.opts.Limits.Tighten(q.Limits)
	points := int64(r.To.Sub(r.From)/step) + 1
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := d.expand(q, r, step)
	if err != nil {
		return nil, err
	}
	if points > maxStepPoints {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "step too small: %d points per series, max %d", points, maxStepPoints)
	}
	if points > int64(lim.MaxPoints) {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "step too small: %d points per series, max points is %d", points, lim.MaxPoints)
	}
	form := url.Values{
		"query":   {text},
		"start":   {unixSeconds(r.From)},
		"end":     {unixSeconds(r.To)},
		"step":    {strconv.FormatFloat(step.Seconds(), 'f', -1, 64)},
		"timeout": {promDuration(lim.Timeout)},
	}
	return d.runQuery(ctx, "/api/v1/query_range", form, lim)
}

func (d *Datasource) QueryInstant(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(q.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := d.expand(q, r, d.variableInterval(r))
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"query":   {text},
		"time":    {unixSeconds(r.To)},
		"timeout": {promDuration(lim.Timeout)},
	}
	return d.runQuery(ctx, "/api/v1/query", form, lim)
}

func (d *Datasource) variableInterval(r datasource.TimeRange) time.Duration {
	return max(d.opts.ScrapeInterval, (r.To.Sub(r.From) / 1000).Truncate(time.Second))
}

func (d *Datasource) expand(q datasource.Query, r datasource.TimeRange, interval time.Duration) (string, error) {
	exp, err := macros.Expand(macros.PromQL, q.Text, macros.Env{
		Range:          r,
		Interval:       interval,
		ScrapeInterval: d.opts.ScrapeInterval,
		Variables:      q.Variables,
	})
	if err != nil {
		return "", err
	}
	if _, err := promParser.ParseExpr(exp.Text); err != nil {
		return "", datasource.Errorf(datasource.ErrInvalidQuery, "%s", truncateText(err.Error()))
	}
	return exp.Text, nil
}

func (d *Datasource) runQuery(ctx context.Context, path string, form url.Values, lim datasource.Limits) ([]datasource.Frame, error) {
	raw, err := d.do(ctx, http.MethodPost, path, form)
	if err != nil {
		return nil, err
	}
	frames, err := convert(raw)
	if err != nil {
		return nil, err
	}
	if len(frames) > lim.MaxSeries {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "series limit exceeded: %d series, max %d", len(frames), lim.MaxSeries)
	}
	if n := datasource.Points(frames); n > lim.MaxPoints {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "points limit exceeded: %d points, max %d", n, lim.MaxPoints)
	}
	return frames, nil
}

func convert(raw json.RawMessage) ([]datasource.Frame, error) {
	var data queryData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "upstream returned an unreadable result")
	}
	switch data.ResultType {
	case "matrix", "vector":
		var result []series
		if err := json.Unmarshal(data.Result, &result); err != nil {
			return nil, datasource.Errorf(datasource.ErrUnavailable, "upstream returned an unreadable result")
		}
		return seriesFrames(result)
	case "scalar", "string":
		var s sample
		if err := json.Unmarshal(data.Result, &s); err != nil {
			return nil, datasource.Errorf(datasource.ErrUnavailable, "upstream returned an unreadable result")
		}
		valueType := datasource.FieldNumber
		if data.ResultType == "string" {
			valueType = datasource.FieldString
		}
		return []datasource.Frame{sampleFrame(nil, valueType, []sample{s})}, nil
	}
	return nil, datasource.Errorf(datasource.ErrUnavailable, "upstream returned unsupported result type %q", truncateText(data.ResultType))
}

func hasContent(b json.RawMessage) bool {
	t := bytes.TrimSpace(b)
	return len(t) > 0 && !bytes.Equal(t, []byte("null"))
}

func seriesFrames(result []series) ([]datasource.Frame, error) {
	type keyed struct {
		key   string
		frame datasource.Frame
	}
	out := make([]keyed, 0, len(result))
	for _, s := range result {
		if hasContent(s.Histogram) || hasContent(s.Histograms) {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "native histogram samples are not supported; aggregate with histogram_quantile, histogram_count or histogram_sum")
		}
		samples := s.Values
		if s.Value != nil {
			samples = []sample{*s.Value}
		}
		f := sampleFrame(s.Metric, datasource.FieldNumber, samples)
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

func sampleFrame(metric map[string]string, valueType datasource.FieldType, samples []sample) datasource.Frame {
	f := datasource.Frame{
		Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: make([]any, 0, len(samples))},
			{Name: "Value", Type: valueType, Values: make([]any, 0, len(samples))},
		},
	}
	for k, v := range metric {
		if k == "__name__" {
			f.Name = v
			continue
		}
		if f.Labels == nil {
			f.Labels = map[string]string{}
		}
		f.Labels[k] = v
	}
	for _, s := range samples {
		f.Fields[0].Values = append(f.Fields[0].Values, s.at)
		if valueType == datasource.FieldString {
			f.Fields[1].Values = append(f.Fields[1].Values, s.value)
			continue
		}
		v, err := strconv.ParseFloat(s.value, 64)
		if err != nil {
			f.Fields[1].Values = append(f.Fields[1].Values, nil)
			continue
		}
		f.Fields[1].Values = append(f.Fields[1].Values, datasource.Number(v))
	}
	return f
}

func labelKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%q,", k, labels[k])
	}
	return b.String()
}
