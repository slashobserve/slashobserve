package queryproxy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logpatterns"
)

const (
	PatternSourceLoki   = "loki"
	PatternSourceServer = "server"

	patternSteps      = 60
	patternBodyField  = "Body"
	patternsNeedBody  = "patterns need a query that selects Body"
	patternsReadTypes = "patterns read Loki and ClickHouse datasources"
)

type PatternsRequest struct {
	Query  datasource.Query
	Range  datasource.TimeRange
	Limit  int
	Sample int
}

type LogPattern struct {
	Pattern string `json:"pattern"`
	Count   int64  `json:"count"`
	Sample  string `json:"sample"`
	First   string `json:"first"`
	Last    string `json:"last"`
}

type LogPatterns struct {
	Source       string       `json:"source"`
	LinesSampled int          `json:"lines_sampled"`
	LinesMatched int64        `json:"lines_matched"`
	Patterns     []LogPattern `json:"patterns"`
	Truncated    bool         `json:"truncated"`
}

func (r *Runner) LogPatterns(ctx context.Context, inst datasource.Instance, req PatternsRequest) (LogPatterns, error) {
	lines, ok := inst.Datasource.(datasource.LineSource)
	if !ok {
		return LogPatterns{}, fmt.Errorf("%w: %s", ErrInvalidRequest, patternsReadTypes)
	}
	if err := req.Range.Validate(); err != nil {
		return LogPatterns{}, err
	}
	if native, ok := inst.Datasource.(datasource.LogPatternSource); ok {
		found, err := r.nativePatterns(ctx, native, req)
		switch {
		case err == nil:
			return foldNative(found, req.Limit), nil
		case !errors.Is(err, datasource.ErrNotFound):
			return LogPatterns{}, err
		}
	}
	frames, err := r.sampleLines(ctx, lines, req)
	if err != nil {
		return LogPatterns{}, err
	}
	sampled, err := sampledLines(frames)
	if err != nil {
		return LogPatterns{}, err
	}
	patterns, truncated := logpatterns.Cluster(sampled, req.Limit)
	out := LogPatterns{Source: PatternSourceServer, LinesSampled: len(sampled), LinesMatched: int64(len(sampled)), Patterns: make([]LogPattern, len(patterns)), Truncated: truncated}
	for i, p := range patterns {
		out.Patterns[i] = LogPattern{Pattern: p.Pattern, Count: p.Count, Sample: p.Sample, First: patternTime(p.First), Last: patternTime(p.Last)}
	}
	return out, nil
}

func (r *Runner) nativePatterns(ctx context.Context, source datasource.LogPatternSource, req PatternsRequest) ([]datasource.NativePattern, error) {
	release, err := r.acquireUpstreamBounded(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return source.LogPatterns(ctx, datasource.PatternRequest{Query: req.Query, Range: req.Range, Step: max(req.Range.To.Sub(req.Range.From)/patternSteps, time.Second)})
}

func (r *Runner) sampleLines(ctx context.Context, source datasource.LineSource, req PatternsRequest) ([]datasource.Frame, error) {
	release, err := r.acquireUpstreamBounded(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	query := req.Query
	query.Limits.MaxRows = req.Sample
	return source.Lines(ctx, datasource.LinesRequest{Query: query, Range: req.Range, Direction: datasource.Backward, Limit: req.Sample})
}

func foldNative(found []datasource.NativePattern, limit int) LogPatterns {
	out := LogPatterns{Source: PatternSourceLoki, Patterns: []LogPattern{}}
	for _, p := range found {
		var count int64
		var first, last time.Time
		for _, s := range p.Samples {
			count += s.Count
			if first.IsZero() || s.At.Before(first) {
				first = s.At
			}
			if s.At.After(last) {
				last = s.At
			}
		}
		if count == 0 {
			continue
		}
		out.LinesMatched += count
		out.Patterns = append(out.Patterns, LogPattern{Pattern: p.Pattern, Count: count, First: patternTime(first), Last: patternTime(last)})
	}
	slices.SortStableFunc(out.Patterns, func(a, b LogPattern) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), strings.Compare(a.Pattern, b.Pattern))
	})
	if len(out.Patterns) > limit {
		out.Patterns, out.Truncated = out.Patterns[:limit], true
	}
	return out
}

func sampledLines(frames []datasource.Frame) ([]logpatterns.Line, error) {
	var lines []logpatterns.Line
	for _, f := range frames {
		rows := f.Rows()
		if rows == 0 {
			continue
		}
		timeField, bodyField := -1, -1
		for i, field := range f.Fields {
			switch {
			case timeField < 0 && field.Type == datasource.FieldTime:
				timeField = i
			case bodyField < 0 && strings.EqualFold(field.Name, patternBodyField):
				bodyField = i
			}
		}
		if bodyField < 0 {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", patternsNeedBody)
		}
		for row := range rows {
			text, ok := f.Fields[bodyField].Values[row].(string)
			if !ok {
				continue
			}
			lines = append(lines, logpatterns.Line{Time: lineTime(f, timeField, row), Text: text})
		}
	}
	return lines, nil
}

func lineTime(f datasource.Frame, timeField, row int) time.Time {
	if ns, ok := datasource.TimeNsOf(f, row); ok {
		return time.Unix(0, ns).UTC()
	}
	if timeField < 0 || row >= len(f.Fields[timeField].Values) {
		return time.Time{}
	}
	at, _ := f.Fields[timeField].Values[row].(time.Time)
	return at.UTC()
}

func patternTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
