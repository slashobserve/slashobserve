package cli

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultContextLines = 50
	maxContextLines     = 500
	maxLogPatterns      = 100
)

type logInsight int

const (
	insightNone logInsight = iota
	insightFacets
	insightPatterns
	insightContext
)

type logFacetsRequest struct {
	Query  string   `json:"query"`
	From   string   `json:"from"`
	To     string   `json:"to"`
	Fields []string `json:"fields,omitempty"`
}

type logFacetsBody struct {
	Sampled      bool `json:"sampled"`
	LinesSampled int  `json:"lines_sampled"`
	Fields       []struct {
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Exact  bool   `json:"exact"`
		Unit   string `json:"unit"`
		Values []struct {
			Value string `json:"value"`
			Count int    `json:"count"`
		} `json:"values"`
	} `json:"fields"`
	Truncated bool `json:"truncated"`
}

type logPatternsRequest struct {
	Query string `json:"query"`
	From  string `json:"from"`
	To    string `json:"to"`
	Limit int    `json:"limit"`
}

type logPatternsBody struct {
	LinesSampled int `json:"lines_sampled"`
	LinesMatched int `json:"lines_matched"`
	Patterns     []struct {
		Pattern string `json:"pattern"`
		Count   int    `json:"count"`
	} `json:"patterns"`
	Truncated bool `json:"truncated"`
}

type logContextRequest struct {
	Query  string `json:"query"`
	Anchor string `json:"anchor"`
	Before int    `json:"before"`
	After  int    `json:"after"`
}

type logContextSide struct {
	Frames    []frame `json:"frames"`
	Truncated bool    `json:"truncated"`
}

type logContextBody struct {
	Anchor string         `json:"anchor"`
	Before logContextSide `json:"before"`
	After  logContextSide `json:"after"`
}

func (o logsOptions) insight(cmd *cobra.Command) (logInsight, error) {
	modes := []struct {
		flag string
		set  bool
		kind logInsight
	}{
		{"--facets", o.facets, insightFacets},
		{"--patterns", o.patterns, insightPatterns},
		{"--context", o.context != "", insightContext},
		{"--tail", o.tail, insightNone},
	}
	var chosen []string
	kind := insightNone
	for _, m := range modes {
		if m.set {
			chosen = append(chosen, m.flag)
			kind = m.kind
		}
	}
	if len(chosen) > 1 {
		return insightNone, usagef("%s and %s cannot be used together", chosen[0], chosen[1])
	}
	if len(o.fields) > 0 && !o.facets {
		return insightNone, usagef("--field needs --facets")
	}
	for _, name := range []string{"before", "after"} {
		if cmd.Flags().Changed(name) && o.context == "" {
			return insightNone, usagef("--%s needs --context", name)
		}
	}
	if o.context != "" {
		if o.before < 0 || o.before > maxContextLines || o.after < 0 || o.after > maxContextLines {
			return insightNone, usagef("--before and --after must be 0 to %d", maxContextLines)
		}
		if _, err := contextAnchor(o.context); err != nil {
			return insightNone, err
		}
	}
	return kind, nil
}

func contextAnchor(raw string) (string, error) {
	if isDigits(raw) {
		return raw, nil
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return "", usagef("--context %q is not a time; give a nanosecond epoch time or an RFC 3339 time such as 2026-11-01T10:00:00.123456789Z", text(raw))
	}
	return strconv.FormatInt(at.UnixNano(), 10), nil
}

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

func (a *app) logInsight(ctx context.Context, c *client, out *output, insight logInsight, query string) error {
	opts := a.logs
	base := "datasources/" + url.PathEscape(opts.datasource) + "/"
	switch insight {
	case insightFacets:
		data, err := c.post(ctx, base+"log-facets", logFacetsRequest{Query: query, From: opts.from, To: opts.to, Fields: opts.fields})
		if err != nil {
			return err
		}
		return a.printInsight(out, data, printLogFacets)
	case insightPatterns:
		data, err := c.post(ctx, base+"log-patterns", logPatternsRequest{Query: query, From: opts.from, To: opts.to, Limit: min(opts.limit, maxLogPatterns)})
		if err != nil {
			return err
		}
		return a.printInsight(out, data, printLogPatterns)
	}
	anchor, err := contextAnchor(opts.context)
	if err != nil {
		return err
	}
	data, err := c.post(ctx, base+"log-context", logContextRequest{Query: query, Anchor: anchor, Before: opts.before, After: opts.after})
	if err != nil {
		return err
	}
	return a.printInsight(out, data, printLogContext)
}

func (a *app) printInsight(out *output, data []byte, show func(*output, []byte) error) error {
	if a.json {
		out.body(data)
		return nil
	}
	return show(out, data)
}

func printLogFacets(out *output, data []byte) error {
	facets, err := decode[logFacetsBody](data)
	if err != nil {
		return err
	}
	if len(facets.Fields) == 0 {
		out.notice("No fields to count")
		return nil
	}
	for i, field := range facets.Fields {
		if i > 0 {
			out.line("")
		}
		accuracy := "sampled"
		if field.Exact {
			accuracy = "exact"
		}
		header := text(field.Name) + " (" + text(field.Kind) + ", " + accuracy
		if field.Unit != "" {
			header += ", " + text(field.Unit)
		}
		out.line("%s)", header)
		width := 0
		for _, v := range field.Values {
			width = max(width, len(strconv.Itoa(v.Count)))
		}
		for _, v := range field.Values {
			out.line("  %*d  %s", width, v.Count, text(v.Value))
		}
	}
	if facets.Sampled {
		out.notice("Fields marked sampled count the newest %d lines", facets.LinesSampled)
	}
	if facets.Truncated {
		out.notice("More fields exist than are shown; name some with --field")
	}
	return nil
}

func printLogPatterns(out *output, data []byte) error {
	patterns, err := decode[logPatternsBody](data)
	if err != nil {
		return err
	}
	if len(patterns.Patterns) == 0 {
		out.notice("No log lines matched")
		return nil
	}
	rows := make([][]string, 0, len(patterns.Patterns))
	for _, p := range patterns.Patterns {
		rows = append(rows, []string{strconv.Itoa(p.Count), text(p.Pattern)})
	}
	out.table([]string{"COUNT", "PATTERN"}, rows)
	out.notice("%d lines matched, %d lines sampled", patterns.LinesMatched, patterns.LinesSampled)
	if patterns.Truncated {
		out.notice("More patterns exist than are shown; raise --limit, up to %d", maxLogPatterns)
	}
	return nil
}

func printLogContext(out *output, data []byte) error {
	around, err := decode[logContextBody](data)
	if err != nil {
		return err
	}
	earlier := collectLogLines(around.Before.Frames)
	slices.Reverse(earlier)
	slices.SortStableFunc(earlier, func(a, b logLine) int { return a.At.Compare(b.At) })
	later := logLines(around.After.Frames)
	all := slices.Concat(earlier, later)
	var keys []string
	if len(all) > 0 {
		keys = varyingLabels(all)
	}
	width := levelWidth(all)
	for _, line := range earlier {
		out.line("%s", logText(line, keys, width))
	}
	out.line("-- %s --", anchorTime(around.Anchor))
	for _, line := range later {
		out.line("%s", logText(line, keys, width))
	}
	if around.Before.Truncated {
		out.notice("More lines exist before it; raise --before, up to %d", maxContextLines)
	}
	if around.After.Truncated {
		out.notice("More lines exist after it; raise --after, up to %d", maxContextLines)
	}
	return nil
}

func anchorTime(anchor string) string {
	ns, err := strconv.ParseInt(anchor, 10, 64)
	if err != nil {
		return text(anchor)
	}
	return time.Unix(0, ns).UTC().Format(time.RFC3339Nano)
}
