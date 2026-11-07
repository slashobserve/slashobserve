package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logpatterns"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	nameLogPatterns = "log_patterns"
	descLogPatterns = "Group the lines a logs query matches into patterns with counts, from Loki's pattern index or Elastic's categorize_text when it has one, otherwise from the newest sampled lines, on Loki, ClickHouse, Elastic or OpenSearch. Use it to see what a noisy service is saying before reading single lines."

	propPatternLimit  = `"limit":{"type":"integer","minimum":1,"maximum":30,"description":"Patterns to return, the most frequent first. Default 10."}`
	schemaLogPatterns = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propFrom + `,` + propTo + `,` + propPatternLimit + `},"required":["datasource","query"],"additionalProperties":false}`

	defaultPatternLimit  = 10
	maxPatternLimit      = 30
	defaultPatternSample = 2000
	patternsBudget       = 12 << 10
	patternsWorkOn       = "log_patterns works on Loki, ClickHouse, Elasticsearch and OpenSearch datasources; %s is a %s datasource"
)

type logPatternsInput struct {
	Datasource string `json:"datasource"`
	Query      string `json:"query"`
	From       string `json:"from"`
	To         string `json:"to"`
	Limit      *int   `json:"limit"`
}

type toolPattern struct {
	Pattern string  `json:"pattern"`
	Count   int64   `json:"count"`
	Sample  string  `json:"sample"`
	First   string  `json:"first"`
	Last    string  `json:"last"`
	Level   string  `json:"level"`
	Trend   []int64 `json:"trend"`
}

type toolPatterns struct {
	Source       string        `json:"source"`
	LinesSampled int           `json:"lines_sampled"`
	LinesMatched int64         `json:"lines_matched"`
	TrendStepMs  int64         `json:"trend_step_ms"`
	Patterns     []toolPattern `json:"patterns"`
	Truncated    bool          `json:"truncated"`
}

func compactPatterns(found queryproxy.LogPatterns, rng datasource.TimeRange) toolPatterns {
	out := toolPatterns{
		Source:       found.Source,
		LinesSampled: found.LinesSampled,
		LinesMatched: found.LinesMatched,
		TrendStepMs:  (rng.To.Sub(rng.From) / logpatterns.TrendBuckets).Milliseconds(),
		Patterns:     make([]toolPattern, len(found.Patterns)),
		Truncated:    found.Truncated,
	}
	for i, p := range found.Patterns {
		trend := make([]int64, len(p.Trend))
		for j, b := range p.Trend {
			trend[j] = b.Count
		}
		out.Patterns[i] = toolPattern{Pattern: p.Pattern, Count: p.Count, Sample: clip(p.Sample, maxSummaryCell), First: p.First, Last: p.Last, Level: p.Level, Trend: trend}
	}
	return out
}

func init() { registerEntries((*Registry).patternEntries) }

func (r *Registry) patternEntries() []entry {
	return []entry{
		{Tool{Name: nameLogPatterns, Description: descLogPatterns, InputSchema: json.RawMessage(schemaLogPatterns), ReadOnly: true, MinRole: auth.RoleEditor}, r.logPatterns},
	}
}

func (r *Registry) logPatterns(ctx context.Context, input json.RawMessage) (Result, error) {
	var in logPatternsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	if res, ok := checkQueryText(in.Query); !ok {
		return res, nil
	}
	rng, res, ok := resolveRange(in.From, in.To, time.Now())
	if !ok {
		return res, nil
	}
	limit, res, ok := boundedInt(in.Limit, defaultPatternLimit, 1, maxPatternLimit, "limit")
	if !ok {
		return res, nil
	}
	if _, ok := inst.Datasource.(datasource.LineSource); !ok {
		return invalid(patternsWorkOn, inst.UID, inst.Datasource.Type()), nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	found, err := r.deps.Runner.LogPatterns(ctx, inst, queryproxy.PatternsRequest{
		Query:  datasource.Query{Text: in.Query, Limits: queryproxy.DraftLimits, Dashboard: defaultDashTitle, User: queryproxy.UserFor(principal)},
		Range:  rng,
		Limit:  limit,
		Sample: defaultPatternSample,
	})
	if err != nil {
		return datasourceFailure("Finding patterns on "+inst.UID, err)
	}
	out := compactPatterns(found, rng)
	var cut bool
	out.Patterns, cut = fit(out.Patterns, patternsBudget)
	out.Truncated = out.Truncated || cut
	step := fmt.Sprintf("Found %s on %s", plural(len(out.Patterns), "pattern", "patterns"), inst.UID)
	switch out.Source {
	case queryproxy.PatternSourceLoki:
		step += " (from Loki)"
	case queryproxy.PatternSourceElasticsearch:
		step += " (from Elastic)"
	default:
		step += fmt.Sprintf(" (from %s)", plural(out.LinesSampled, "sampled line", "sampled lines"))
	}
	return success(step, out), nil
}
