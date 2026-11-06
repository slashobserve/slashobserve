package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	nameLogPatterns = "log_patterns"
	descLogPatterns = "Group the lines a logs query matches into patterns with counts, from Loki's pattern index when it has one, otherwise from the newest sampled lines, on Loki or ClickHouse. Use it to see what a noisy service is saying before reading single lines."

	propPatternLimit  = `"limit":{"type":"integer","minimum":1,"maximum":30,"description":"Patterns to return, the most frequent first. Default 10."}`
	schemaLogPatterns = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propFrom + `,` + propTo + `,` + propPatternLimit + `},"required":["datasource","query"],"additionalProperties":false}`

	defaultPatternLimit  = 10
	maxPatternLimit      = 30
	defaultPatternSample = 2000
	patternsBudget       = 12 << 10
	patternsWorkOn       = "log_patterns works on Loki and ClickHouse datasources; %s is a %s datasource"
)

type logPatternsInput struct {
	Datasource string `json:"datasource"`
	Query      string `json:"query"`
	From       string `json:"from"`
	To         string `json:"to"`
	Limit      *int   `json:"limit"`
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
	out, err := r.deps.Runner.LogPatterns(ctx, inst, queryproxy.PatternsRequest{
		Query:  datasource.Query{Text: in.Query, Limits: queryproxy.DraftLimits, Dashboard: defaultDashTitle, User: queryproxy.UserFor(principal)},
		Range:  rng,
		Limit:  limit,
		Sample: defaultPatternSample,
	})
	if err != nil {
		return datasourceFailure("Finding patterns on "+inst.UID, err)
	}
	for i := range out.Patterns {
		out.Patterns[i].Sample = clip(out.Patterns[i].Sample, maxSummaryCell)
	}
	var cut bool
	out.Patterns, cut = fit(out.Patterns, patternsBudget)
	out.Truncated = out.Truncated || cut
	step := fmt.Sprintf("Found %s on %s", plural(len(out.Patterns), "pattern", "patterns"), inst.UID)
	if out.Source == queryproxy.PatternSourceLoki {
		step += " (from Loki)"
	} else {
		step += fmt.Sprintf(" (from %s)", plural(out.LinesSampled, "sampled line", "sampled lines"))
	}
	return success(step, out), nil
}
