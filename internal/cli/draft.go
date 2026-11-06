package cli

import (
	"context"
	"net/url"
	"strings"
)

const (
	signalLogs   = "logs"
	signalTraces = "traces"
)

type draftRequest struct {
	Question string `json:"question"`
	From     string `json:"from"`
	To       string `json:"to"`
	Signal   string `json:"signal"`
}

type draftAnswer struct {
	Query        string        `json:"query"`
	Language     string        `json:"language"`
	TraceFilters *traceFilters `json:"trace_filters"`
	Explanation  string        `json:"explanation"`
	Warnings     []string      `json:"warnings"`
}

func draftQuery(ctx context.Context, c *client, uid, signal, question, from, to string) (draftAnswer, error) {
	data, err := c.post(ctx, "datasources/"+url.PathEscape(uid)+"/draft-query", draftRequest{Question: question, From: from, To: to, Signal: signal})
	if err != nil {
		return draftAnswer{}, err
	}
	return decode[draftAnswer](data)
}

func printDraft(out *output, answer draftAnswer) {
	if answer.TraceFilters != nil {
		out.notice("Filters: %s", filtersText(*answer.TraceFilters))
	} else {
		out.notice("Query: %s", text(answer.Query))
	}
	out.notice("%s", text(answer.Explanation))
	for _, warning := range answer.Warnings {
		out.notice("Warning: %s", text(warning))
	}
}

func filtersText(f traceFilters) string {
	var parts []string
	for _, pair := range [][2]string{{"service", f.Service}, {"span", f.Span}, {"min", f.Min}, {"max", f.Max}} {
		if pair[1] != "" {
			parts = append(parts, pair[0]+"="+text(pair[1]))
		}
	}
	if f.Errors {
		parts = append(parts, "errors")
	}
	return strings.Join(parts, " ")
}

func askUsage(command, needs string, args []string, ask string, dryRun bool) error {
	switch {
	case ask == "" && dryRun:
		return usagef("--dry-run needs --ask")
	case ask == "" && len(args) == 0:
		return usagef("%s needs %s or --ask", command, needs)
	case ask != "" && len(args) > 0:
		return usagef("give %s or --ask, not both", needs)
	}
	return nil
}
