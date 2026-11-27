package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	tailInterval    = 2 * time.Second
	defaultLogLimit = 100
	maxLogLimit     = 1000
	logTimeLayout   = "2006-01-02T15:04:05.000Z07:00"
	inlineLevel     = -1
	searchPageMin   = 50
	searchPageMax   = 500
)

var (
	bodyFields  = []string{"body", "message", "msg", "line", "log"}
	levelFields = []string{"severitytext", "severity", "level", "lvl", "detected_level"}
)

type logsOptions struct {
	datasource string
	from       string
	to         string
	limit      int
	tail       bool
	ask        string
	dryRun     bool
	facets     bool
	fields     []string
	patterns   bool
	context    string
	before     int
	after      int
}

type logLine struct {
	At     time.Time
	Level  string
	Labels map[string]string
	Body   string
}

type logLineJSON struct {
	Time   string            `json:"time"`
	Level  string            `json:"level"`
	Labels map[string]string `json:"labels"`
	Line   string            `json:"line"`
}

func (a *app) logsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [<query>] --datasource <uid>",
		Short: "Print log lines from a Loki, ClickHouse, Elasticsearch or OpenSearch datasource",
		Long:  "logs runs a log query on a datasource and prints the lines oldest first.\nThe query is in the language of the datasource: LogQL on Loki, SQL on ClickHouse, and KQL on Elasticsearch and OpenSearch, where a lucene: prefix or a {...} Query DSL body also work.\nWith --ask the server writes the query from a question and prints it first.\nWith --tail it keeps printing new lines every 2 seconds until Ctrl-C.",
		Example: "  observe logs '{app=\"shop\"} |= \"error\"' --datasource loki --from now-15m\n" +
			"  observe logs '{app=\"shop\"}' --datasource loki --tail\n" +
			"  observe logs --ask \"checkout errors\" --datasource loki --dry-run\n" +
			"  observe logs '{app=\"shop\"}' --datasource loki --facets\n" +
			"  observe logs '{app=\"shop\"}' --datasource loki --patterns\n" +
			"  observe logs '{app=\"shop\"}' --datasource loki --context 2026-11-01T10:00:00.123456789Z",
		Args: cobra.ArbitraryArgs,
		RunE: a.runLogs,
	}
	flags := cmd.Flags()
	flags.StringVar(&a.logs.datasource, "datasource", "", "the datasource uid (observe doctor lists them)")
	flags.StringVar(&a.logs.from, "from", "now-1h", "start of the range, such as now-15m or an RFC 3339 time")
	flags.StringVar(&a.logs.to, "to", "now", "end of the range")
	flags.IntVar(&a.logs.limit, "limit", defaultLogLimit, "the newest lines to print, 1 to 1000")
	flags.BoolVar(&a.logs.tail, "tail", false, "keep printing new lines until Ctrl-C")
	flags.StringVar(&a.logs.ask, "ask", "", "a question the server turns into the log query, instead of the query argument")
	flags.BoolVar(&a.logs.dryRun, "dry-run", false, "with --ask, print the query without running it")
	flags.BoolVar(&a.logs.facets, "facets", false, "count the top values of each label or field instead of printing lines")
	flags.StringArrayVar(&a.logs.fields, "field", nil, "with --facets, count only this label or field; repeat it for several")
	flags.BoolVar(&a.logs.patterns, "patterns", false, "group the lines into patterns instead of printing them")
	flags.StringVar(&a.logs.context, "context", "", "print the lines around one line: its nanosecond time or an RFC 3339 time with nanoseconds")
	flags.IntVar(&a.logs.before, "before", defaultContextLines, "with --context, the lines to print before it, 0 to 500")
	flags.IntVar(&a.logs.after, "after", defaultContextLines, "with --context, the lines to print after it, 0 to 500")
	return cmd
}

func (a *app) runLogs(cmd *cobra.Command, args []string) error {
	opts := a.logs
	if err := askUsage(cmd.Name(), "a log query", args, opts.ask, opts.dryRun); err != nil {
		return err
	}
	if opts.datasource == "" {
		return usagef("%s needs --datasource <uid>; observe doctor lists them", cmd.CommandPath())
	}
	if opts.limit < 1 || opts.limit > maxLogLimit {
		return usagef("--limit must be 1 to %d", maxLogLimit)
	}
	insight, err := opts.insight(cmd)
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	out := a.output()
	query := strings.Join(args, " ")
	if opts.ask != "" {
		answer, err := draftQuery(ctx, c, opts.datasource, signalLogs, opts.ask, opts.from, opts.to)
		if err != nil {
			return err
		}
		printDraft(out, answer)
		if opts.dryRun {
			return nil
		}
		query = answer.Query
	}
	if insight != insightNone {
		return a.logInsight(ctx, c, out, insight, query)
	}
	req, err := logRequest(ctx, c, opts, query)
	if err != nil {
		return err
	}
	data, body, err := exploreLogs(ctx, c, opts.datasource, req, opts.limit)
	if err != nil {
		return err
	}
	lines := newestLines(logLines(body.Frames), opts.limit)
	if opts.tail {
		tail := &logTail{out: out, json: a.json}
		tail.print(lines)
		return tail.follow(ctx, c, opts.datasource, req, opts.limit, tailInterval)
	}
	if a.json {
		out.body(data)
		return nil
	}
	printLogResult(out, body.Frames, lines, opts.from, opts.to)
	return nil
}

func printLogResult(out *output, frames []frame, lines []logLine, from, to string) {
	if headers, rows := resultTable(frames); len(lines) == 0 && len(rows) > 0 {
		out.table(headers, rows)
		return
	}
	printLogLines(out, lines, from, to)
}

func logRequest(ctx context.Context, c *client, opts logsOptions, query string) (exploreRequest, error) {
	typ, err := datasourceType(ctx, c, opts.datasource)
	if err != nil {
		return exploreRequest{}, err
	}
	req := exploreRequest{Query: query, Kind: "range", From: opts.from, To: opts.to, Limit: opts.limit}
	switch typ {
	case "clickhouse":
		req.Kind = "instant"
	case "elasticsearch", "opensearch":
		if opts.limit > searchPageMax {
			return exploreRequest{}, usagef("--limit is at most %d on an Elasticsearch or OpenSearch datasource", searchPageMax)
		}
		req.Kind, req.Limit, req.PageSize, req.Page = "instant", 0, max(opts.limit, searchPageMin), 1
	}
	return req, nil
}

func exploreLogs(ctx context.Context, c *client, uid string, req exploreRequest, keep int) ([]byte, exploreResponse, error) {
	data, body, err := explore(ctx, c, uid, req)
	if err != nil || req.PageSize == 0 {
		return data, body, err
	}
	if data, err = newestRowsOldestFirst(data, keep); err != nil {
		return nil, exploreResponse{}, err
	}
	body, err = decode[exploreResponse](data)
	return data, body, err
}

func newestRowsOldestFirst(data []byte, keep int) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	if len(body["frames"]) == 0 {
		return data, nil
	}
	var frames []map[string]json.RawMessage
	if err := json.Unmarshal(body["frames"], &frames); err != nil {
		return nil, err
	}
	for _, f := range frames {
		if err := reorderRows(f, keep); err != nil {
			return nil, err
		}
	}
	var err error
	if body["frames"], err = json.Marshal(frames); err != nil {
		return nil, err
	}
	return json.Marshal(body)
}

func reorderRows(f map[string]json.RawMessage, keep int) error {
	if len(f["fields"]) == 0 {
		return nil
	}
	var fields []map[string]json.RawMessage
	if err := json.Unmarshal(f["fields"], &fields); err != nil {
		return err
	}
	columns := make([][]json.RawMessage, len(fields))
	for i, field := range fields {
		if len(field["values"]) == 0 {
			continue
		}
		if err := json.Unmarshal(field["values"], &columns[i]); err != nil {
			return err
		}
	}
	order := rowOrder(fields, columns)
	order = order[max(0, len(order)-keep):]
	for i, field := range fields {
		picked := make([]json.RawMessage, 0, len(order))
		for _, row := range order {
			if row < len(columns[i]) {
				picked = append(picked, columns[i][row])
			}
		}
		var err error
		if field["values"], err = json.Marshal(picked); err != nil {
			return err
		}
	}
	var err error
	f["fields"], err = json.Marshal(fields)
	return err
}

func rowOrder(fields []map[string]json.RawMessage, columns [][]json.RawMessage) []int {
	rows := 0
	at := -1
	for i, field := range fields {
		rows = max(rows, len(columns[i]))
		if at < 0 && string(field["type"]) == `"time"` {
			at = i
		}
	}
	order := make([]int, rows)
	for i := range order {
		order[i] = i
	}
	if at >= 0 {
		slices.SortStableFunc(order, func(a, b int) int {
			return cmp.Compare(rowTime(columns[at], a), rowTime(columns[at], b))
		})
	}
	return order
}

func rowTime(values []json.RawMessage, row int) float64 {
	var ms float64
	if row < len(values) {
		_ = json.Unmarshal(values[row], &ms)
	}
	return ms
}

func printLogLines(out *output, lines []logLine, from, to string) {
	if len(lines) == 0 {
		out.notice("No log lines between %s and %s", text(from), text(to))
		return
	}
	keys := varyingLabels(lines)
	width := levelWidth(lines)
	for _, line := range lines {
		out.line("%s", logText(line, keys, width))
	}
}

func logLines(frames []frame) []logLine {
	lines := collectLogLines(frames)
	slices.SortStableFunc(lines, func(a, b logLine) int { return a.At.Compare(b.At) })
	return lines
}

func collectLogLines(frames []frame) []logLine {
	var lines []logLine
	for _, f := range frames {
		at := firstOfType(f, "time")
		body := fieldIndex(f, bodyFields...)
		if body < 0 {
			body = firstOfType(f, "string")
		}
		if at < 0 || body < 0 {
			continue
		}
		level := fieldIndex(f, levelFields...)
		for row := range f.rows() {
			ms, ok := f.numberAt(at, row)
			if !ok {
				continue
			}
			line := logLine{At: time.UnixMilli(int64(ms)).UTC(), Labels: f.Labels, Body: f.stringAt(body, row)}
			if level >= 0 {
				line.Level = f.stringAt(level, row)
			}
			lines = append(lines, line)
		}
	}
	return lines
}

func newestLines(lines []logLine, limit int) []logLine {
	if len(lines) > limit {
		return lines[len(lines)-limit:]
	}
	return lines
}

func varyingLabels(lines []logLine) []string {
	keys := map[string]bool{}
	for _, line := range lines {
		for key := range line.Labels {
			keys[key] = true
		}
	}
	var varying []string
	for key := range keys {
		first, firstOK := lines[0].Labels[key]
		for _, line := range lines[1:] {
			if value, ok := line.Labels[key]; ok != firstOK || value != first {
				varying = append(varying, key)
				break
			}
		}
	}
	slices.Sort(varying)
	return varying
}

func levelWidth(lines []logLine) int {
	width := 0
	for _, line := range lines {
		width = max(width, len(text(line.Level)))
	}
	return width
}

func labelText(labels map[string]string, keys []string) string {
	var parts []string
	for _, key := range keys {
		if value, ok := labels[key]; ok {
			parts = append(parts, fmt.Sprintf("%s=%q", key, value))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return text("{" + strings.Join(parts, ", ") + "}")
}

func logText(line logLine, keys []string, width int) string {
	parts := []string{line.At.UTC().Format(logTimeLayout)}
	switch {
	case width > 0:
		parts = append(parts, fmt.Sprintf("%-*s", width, cmp.Or(text(line.Level), "-")))
	case width == inlineLevel && line.Level != "":
		parts = append(parts, text(line.Level))
	}
	if labels := labelText(line.Labels, keys); labels != "" {
		parts = append(parts, labels)
	}
	return strings.Join(append(parts, text(line.Body)), "  ")
}

type logTail struct {
	out    *output
	json   bool
	newest time.Time
	atEdge map[string]bool
}

func logKey(line logLine) string {
	keys := slices.Sorted(maps.Keys(line.Labels))
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key + "\x00" + line.Labels[key] + "\x00")
	}
	return line.At.Format(time.RFC3339Nano) + "\x01" + b.String() + "\x01" + line.Body
}

func (t *logTail) print(lines []logLine) {
	for _, line := range lines {
		switch {
		case line.At.Before(t.newest):
			continue
		case line.At.After(t.newest):
			t.newest = line.At
			t.atEdge = map[string]bool{}
		}
		key := logKey(line)
		if t.atEdge[key] {
			continue
		}
		t.atEdge[key] = true
		t.write(line)
	}
}

func (t *logTail) write(line logLine) {
	if t.json {
		data, err := json.Marshal(logLineJSON{Time: line.At.UTC().Format(logTimeLayout), Level: line.Level, Labels: line.Labels, Line: line.Body})
		if err == nil {
			t.out.body(data)
		}
		return
	}
	t.out.line("%s", logText(line, slices.Sorted(maps.Keys(line.Labels)), inlineLevel))
}

func (t *logTail) follow(ctx context.Context, c *client, uid string, req exploreRequest, keep int, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		if !t.newest.IsZero() {
			req.From = t.newest.Format(time.RFC3339Nano)
		}
		req.To = "now"
		_, body, err := explore(ctx, c, uid, req)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			failures++
			if !isTransient(err) || failures >= maxPollFailures {
				return err
			}
			continue
		}
		failures = 0
		polled := logLines(body.Frames)
		if len(polled) >= keep {
			t.out.notice("A poll returned %d lines, the --limit, so newer lines may have been skipped; raise --limit or narrow the query", keep)
		}
		t.print(newestLines(polled, keep))
	}
}
