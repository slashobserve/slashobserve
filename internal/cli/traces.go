package cli

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const maxTraceSpans = 1000

var traceIDPattern = regexp.MustCompile(`^(?:[0-9a-fA-F]{16}|[0-9a-fA-F]{32})$`)

var spanColumns = []string{"TraceId", "SpanId", "ParentSpanId", "SpanName", "ServiceName", "StartTime", "Duration", "StatusCode"}

type tracesOptions struct {
	datasource string
	from       string
	to         string
}

type span struct {
	TraceID  string
	ID       string
	ParentID string
	Name     string
	Service  string
	Start    time.Time
	Duration time.Duration
	Status   string
}

type treeRow struct {
	span  span
	depth int
}

func (a *app) tracesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "traces <trace id> --datasource <uid>",
		Short: "Print the spans of a trace from a Tempo or ClickHouse datasource",
		Long:  "traces finds a trace by its 16 or 32 hexadecimal character id and prints its spans as a tree.",
		Example: "  observe traces 4bf92f3577b34da6a3ce929d0e0e4736 --datasource tempo\n" +
			"  observe traces 4bf92f3577b34da6a3ce929d0e0e4736 --datasource clickhouse --from now-7d",
		Args: exactArgs(1, "one trace id"),
		RunE: a.runTraces,
	}
	flags := cmd.Flags()
	flags.StringVar(&a.traces.datasource, "datasource", "", "the datasource uid (observe doctor lists them)")
	flags.StringVar(&a.traces.from, "from", "now-24h", "start of the range to search, such as now-7d or an RFC 3339 time")
	flags.StringVar(&a.traces.to, "to", "now", "end of the range")
	return cmd
}

func (a *app) runTraces(cmd *cobra.Command, args []string) error {
	opts := a.traces
	if !traceIDPattern.MatchString(args[0]) {
		return usagef("%q is not a trace id; a trace id is 16 or 32 hexadecimal characters", text(args[0]))
	}
	id := strings.ToLower(args[0])
	if opts.datasource == "" {
		return usagef("%s needs --datasource <uid>; observe doctor lists them", cmd.CommandPath())
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	kind, err := datasourceType(ctx, c, opts.datasource)
	if err != nil {
		return err
	}
	query, err := traceQuery(kind, id)
	if err != nil {
		return usagef("traces reads Tempo or ClickHouse datasources; %s is %s", text(opts.datasource), text(kind))
	}
	data, body, err := explore(ctx, c, opts.datasource, exploreRequest{Query: query, Kind: "range", From: opts.from, To: opts.to, Limit: maxTraceSpans})
	if err != nil {
		return err
	}
	notFound := fmt.Errorf("trace %s was not found between %s and %s", id, text(opts.from), text(opts.to))
	out := a.output()
	spans, err := spanRows(body.Frames)
	if a.json {
		out.body(data)
		if err == nil && len(spans) == 0 {
			return notFound
		}
		return nil
	}
	if err != nil {
		return err
	}
	if len(spans) == 0 {
		return notFound
	}
	printTrace(out, id, spanTree(spans))
	return nil
}

func traceQuery(kind, id string) (string, error) {
	switch kind {
	case "tempo":
		return id, nil
	case "clickhouse":
		return "SELECT TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Timestamp AS StartTime, Duration, StatusCode FROM otel_traces WHERE TraceId = '" + id + "' AND $__timeFilter(Timestamp) ORDER BY Timestamp LIMIT " + strconv.Itoa(maxTraceSpans), nil
	}
	return "", fmt.Errorf("unsupported datasource type %s", kind)
}

func spanRows(frames []frame) ([]span, error) {
	var spans []span
	for _, f := range frames {
		if f.rows() == 0 {
			continue
		}
		columns := make([]int, len(spanColumns))
		for i, name := range spanColumns {
			if columns[i] = fieldIndex(f, name); columns[i] < 0 {
				return nil, fmt.Errorf("the datasource answered without a %s column; traces needs %s", name, strings.Join(spanColumns, ", "))
			}
		}
		for row := range f.rows() {
			startMs, ok := f.numberAt(columns[5], row)
			if !ok {
				continue
			}
			spans = append(spans, span{
				TraceID:  f.stringAt(columns[0], row),
				ID:       f.stringAt(columns[1], row),
				ParentID: f.stringAt(columns[2], row),
				Name:     f.stringAt(columns[3], row),
				Service:  f.stringAt(columns[4], row),
				Start:    time.UnixMilli(int64(startMs)).UTC(),
				Duration: time.Duration(nanosAt(f, columns[6], row)),
				Status:   f.stringAt(columns[7], row),
			})
		}
	}
	return spans, nil
}

func nanosAt(f frame, field, row int) float64 {
	if n, ok := f.numberAt(field, row); ok {
		return n
	}
	n, _ := strconv.ParseFloat(f.stringAt(field, row), 64)
	return n
}

func byStart(a, b span) int {
	return a.Start.Compare(b.Start)
}

func spanTree(spans []span) []treeRow {
	known := map[string]bool{}
	for _, s := range spans {
		known[s.ID] = true
	}
	children := map[string][]span{}
	var roots []span
	for _, s := range spans {
		if s.ParentID == "" || !known[s.ParentID] || s.ParentID == s.ID {
			roots = append(roots, s)
			continue
		}
		children[s.ParentID] = append(children[s.ParentID], s)
	}
	slices.SortStableFunc(roots, byStart)
	rows := make([]treeRow, 0, len(spans))
	visited := map[string]bool{}
	var walk func(s span, depth int)
	walk = func(s span, depth int) {
		if visited[s.ID] {
			return
		}
		visited[s.ID] = true
		rows = append(rows, treeRow{span: s, depth: depth})
		kids := children[s.ID]
		slices.SortStableFunc(kids, byStart)
		for _, kid := range kids {
			walk(kid, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
	return rows
}

func formatSpanDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return strconv.FormatInt(d.Microseconds(), 10) + "µs"
	case d < time.Second:
		return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 1, 64) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 2, 64) + "s"
}

func spanStatus(code string) string {
	switch {
	case strings.Contains(strings.ToLower(code), "error"):
		return "error"
	case strings.EqualFold(code, "ok"):
		return "ok"
	}
	return "-"
}

func printTrace(out *output, id string, rows []treeRow) {
	start, end := rows[0].span.Start, rows[0].span.Start
	for _, row := range rows {
		if row.span.Start.Before(start) {
			start = row.span.Start
		}
		if finish := row.span.Start.Add(row.span.Duration); finish.After(end) {
			end = finish
		}
	}
	root := rows[0].span
	out.line("Trace %s  %d %s  %s  %s %s", id, len(rows), plural(len(rows), "span", "spans"), formatSpanDuration(end.Sub(start)), text(root.Service), text(root.Name))
	table := make([][]string, 0, len(rows))
	for _, row := range rows {
		s := row.span
		table = append(table, []string{
			text(s.Service),
			strings.Repeat("  ", row.depth) + text(s.Name),
			"+" + formatSpanDuration(s.Start.Sub(start)),
			formatSpanDuration(s.Duration),
			spanStatus(s.Status),
		})
	}
	out.table([]string{"SERVICE", "SPAN", "START", "DURATION", "STATUS"}, table)
}
