package loki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxStepPoints = 11000
	maxLogLines   = 5000
)

var levelLabels = []string{"level", "lvl", "severity_text", "severity", "detected_level"}

type queryData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

type stream struct {
	Stream map[string]string   `json:"stream"`
	Values [][]json.RawMessage `json:"values"`
}

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
	Metric map[string]string `json:"metric"`
	Value  *sample           `json:"value"`
	Values []sample          `json:"values"`
}

func (d *Datasource) QueryRange(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if step < time.Millisecond {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "step must be at least 1ms")
	}
	return d.query(ctx, q, r, step, false)
}

func (d *Datasource) QueryInstant(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return d.query(ctx, q, r, instantInterval(r, q.MinInterval), true)
}

func instantInterval(r datasource.TimeRange, minimum time.Duration) time.Duration {
	return max(time.Second, (r.To.Sub(r.From) / 1000).Truncate(time.Second), minimum)
}

func (d *Datasource) query(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration, instant bool) ([]datasource.Frame, error) {
	lim := d.opts.Limits.Tighten(q.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := expand(q, r, step)
	if err != nil {
		return nil, err
	}
	params := url.Values{"query": {text}}
	switch {
	case spec.LogQLReturnsLines(text):
		params.Set("start", lokiTime(r.From))
		params.Set("end", lokiTime(r.To))
		params.Set("limit", strconv.Itoa(min(lim.MaxRows, maxLogLines)))
		params.Set("direction", "backward")
		return d.run(ctx, "/loki/api/v1/query_range", params, lim)
	case instant:
		params.Set("time", lokiTime(r.To))
		return d.run(ctx, "/loki/api/v1/query", params, lim)
	}
	steps := int64(r.To.Sub(r.From) / step)
	if steps > maxStepPoints {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "step too small: %d steps, max %d", steps, maxStepPoints)
	}
	if steps+1 > int64(lim.MaxPoints) {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "step too small: %d points per series, max points is %d", steps+1, lim.MaxPoints)
	}
	params.Set("start", lokiTime(r.From))
	params.Set("end", lokiTime(r.To))
	params.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))
	return d.run(ctx, "/loki/api/v1/query_range", params, lim)
}

func expand(q datasource.Query, r datasource.TimeRange, interval time.Duration) (string, error) {
	exp, err := macros.Expand(macros.LogQL, q.Text, macros.Env{
		Range:     r,
		Interval:  interval,
		Variables: q.Variables,
		Dashboard: q.Dashboard,
		User:      q.User,
	})
	if err != nil {
		return "", err
	}
	return exp.Text, nil
}

func lokiTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func (d *Datasource) run(ctx context.Context, path string, params url.Values, lim datasource.Limits) ([]datasource.Frame, error) {
	raw, err := d.get(ctx, path, params)
	if err != nil {
		return nil, err
	}
	var data queryData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, unreadable()
	}
	switch data.ResultType {
	case "streams":
		return streamFrames(data.Result, lim)
	case "matrix", "vector":
		return seriesFrames(data.Result, lim)
	}
	return nil, datasource.Errorf(datasource.ErrUnavailable, "upstream returned unsupported result type %q", truncateText(data.ResultType))
}

func decodeEach[T any](result json.RawMessage, maxItems int, each func(T) error) error {
	dec := json.NewDecoder(bytes.NewReader(result))
	open, err := dec.Token()
	if err != nil {
		return unreadable()
	}
	if open == nil {
		return nil
	}
	if open != json.Delim('[') {
		return unreadable()
	}
	for n := 0; dec.More(); n++ {
		if n == maxItems {
			return datasource.Errorf(datasource.ErrLimitExceeded, "series limit exceeded: more than %d series", maxItems)
		}
		var item T
		if err := dec.Decode(&item); err != nil {
			return unreadable()
		}
		if err := each(item); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return unreadable()
	}
	return nil
}

func streamFrames(result json.RawMessage, lim datasource.Limits) ([]datasource.Frame, error) {
	frames := []datasource.Frame{}
	rows := 0
	err := decodeEach(result, lim.MaxSeries, func(s stream) error {
		if rows += len(s.Values); rows > lim.MaxPoints {
			return datasource.Errorf(datasource.ErrLimitExceeded, "points limit exceeded: more than %d log lines", lim.MaxPoints)
		}
		f, err := streamFrame(s)
		if err != nil {
			return err
		}
		frames = append(frames, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortFrames(frames)
	return frames, nil
}

func streamFrame(s stream) (datasource.Frame, error) {
	times := make([]any, 0, len(s.Values))
	lines := make([]any, 0, len(s.Values))
	for _, entry := range s.Values {
		if len(entry) < 2 {
			return datasource.Frame{}, unreadable()
		}
		var ts, line string
		if json.Unmarshal(entry[0], &ts) != nil || json.Unmarshal(entry[1], &line) != nil {
			return datasource.Frame{}, unreadable()
		}
		ns, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			return datasource.Frame{}, unreadable()
		}
		times = append(times, time.Unix(0, ns).UTC())
		lines = append(lines, line)
	}
	f := datasource.Frame{
		Labels: s.Stream,
		Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: times},
			{Name: "Body", Type: datasource.FieldString, Values: lines},
		},
	}
	if level, ok := streamLevel(s.Stream); ok {
		levels := make([]any, len(lines))
		for i := range levels {
			levels[i] = level
		}
		f.Fields = append(f.Fields, datasource.Field{Name: "level", Type: datasource.FieldString, Values: levels})
	}
	return f, nil
}

func streamLevel(labels map[string]string) (string, bool) {
	for _, name := range levelLabels {
		if v := labels[name]; v != "" {
			return v, true
		}
	}
	return "", false
}

func seriesFrames(result json.RawMessage, lim datasource.Limits) ([]datasource.Frame, error) {
	frames := []datasource.Frame{}
	points := 0
	err := decodeEach(result, lim.MaxSeries, func(s series) error {
		samples := s.Values
		if s.Value != nil {
			samples = []sample{*s.Value}
		}
		f := sampleFrame(s.Metric, samples)
		if points += datasource.Points([]datasource.Frame{f}); points > lim.MaxPoints {
			return datasource.Errorf(datasource.ErrLimitExceeded, "points limit exceeded: more than %d points", lim.MaxPoints)
		}
		frames = append(frames, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortFrames(frames)
	return frames, nil
}

func sampleFrame(metric map[string]string, samples []sample) datasource.Frame {
	f := datasource.Frame{
		Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: make([]any, 0, len(samples))},
			{Name: "Value", Type: datasource.FieldNumber, Values: make([]any, 0, len(samples))},
		},
	}
	if len(metric) > 0 {
		f.Labels = metric
	}
	for _, s := range samples {
		f.Fields[0].Values = append(f.Fields[0].Values, s.at)
		v, err := strconv.ParseFloat(s.value, 64)
		if err != nil {
			f.Fields[1].Values = append(f.Fields[1].Values, nil)
			continue
		}
		f.Fields[1].Values = append(f.Fields[1].Values, datasource.Number(v))
	}
	return f
}

func sortFrames(frames []datasource.Frame) {
	slices.SortStableFunc(frames, func(a, b datasource.Frame) int {
		return strings.Compare(labelKey(a.Labels), labelKey(b.Labels))
	})
}

func labelKey(labels map[string]string) string {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		fmt.Fprintf(&b, "%s=%q,", k, labels[k])
	}
	return b.String()
}
