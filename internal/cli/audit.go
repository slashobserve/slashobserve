package cli

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	maxAuditPage        = 1000
	defaultAuditFacets  = 10
	maxAuditFacets      = 100
	defaultAuditBuckets = 60
	maxAuditBuckets     = 200
	auditTimeLayout     = "2006-01-02T15:04:05Z"
	exportRowLimitKey   = "X-Export-Row-Limit"
)

var auditExportFormats = map[string]string{"ndjson": "application/x-ndjson", "csv": "text/csv"}

type auditOptions struct {
	query   string
	from    string
	to      string
	page    pageOptions
	limit   int
	buckets int
	format  string
	file    string
}

type auditEvent struct {
	ID         string  `json:"id"`
	OccurredAt string  `json:"occurred_at"`
	Action     string  `json:"action"`
	Outcome    string  `json:"outcome"`
	Reason     *string `json:"reason"`
	Status     *int    `json:"status"`
	ActorKind  string  `json:"actor_kind"`
	ActorName  *string `json:"actor_name"`
	Initiator  string  `json:"initiator"`
	AgentName  *string `json:"agent_name"`
	Origin     string  `json:"origin"`
	Tool       *string `json:"tool"`
	Route      *string `json:"route"`
	TargetKind *string `json:"target_kind"`
	TargetID   *string `json:"target_id"`
	TargetName *string `json:"target_name"`
	RequestID  *string `json:"request_id"`
	RunID      *string `json:"run_id"`
	ClientIP   *string `json:"client_ip"`
}

type auditEventList struct {
	Events     []auditEvent `json:"events"`
	NextCursor *string      `json:"next_cursor"`
}

type auditEventDetail struct {
	Event       auditEvent   `json:"event"`
	SameRequest []auditEvent `json:"same_request"`
	SameRun     []auditEvent `json:"same_run"`
}

type auditFacets struct {
	Field  string `json:"field"`
	Values []struct {
		Value string `json:"value"`
		Count int64  `json:"count"`
	} `json:"values"`
}

type auditHistogram struct {
	StepMs  int64 `json:"step_ms"`
	Total   int64 `json:"total"`
	Buckets []struct {
		Time  string `json:"time"`
		Count int64  `json:"count"`
	} `json:"buckets"`
}

type auditAggregation struct {
	Levels []struct {
		Name string `json:"name"`
	} `json:"levels"`
	Metrics []struct {
		Name string  `json:"name"`
		Key  *string `json:"key"`
	} `json:"metrics"`
	Rows []struct {
		Keys     []*string  `json:"keys"`
		DocCount int64      `json:"doc_count"`
		Values   []*float64 `json:"values"`
	} `json:"rows"`
	Truncated bool `json:"truncated"`
}

type auditFieldList struct {
	Fields []struct {
		Name        string   `json:"name"`
		Alias       *string  `json:"alias"`
		Type        string   `json:"type"`
		Description string   `json:"description"`
		Values      []string `json:"values"`
	} `json:"fields"`
}

func (a *app) auditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Search the audit trail",
		Long:  "audit reads the audit trail: every sign-in, read, change, agent tool call and refusal, with who did it, through what, to what and how it ended.\n--query takes KQL by default, Lucene after a lucene: prefix, or a Query DSL JSON body that starts with {; observe audit fields lists the fields.\n--from and --to take an RFC 3339 time, epoch milliseconds or a relative time such as now-7d; the window is the last 24 hours by default and at most 31 days.\nEvery verb needs an Admin key, and every read is itself recorded in the trail.",
		Example: "  observe audit search --query 'outcome:denied and origin:mcp' --from now-7d\n" +
			"  observe audit facets actor_name --query 'action:dashboard.*'\n" +
			"  observe audit export --from 2026-11-01T00:00:00Z --to 2026-11-08T00:00:00Z --format csv --file audit.csv",
		Args: groupArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.auditSearchCommand(), a.auditShowCommand(), a.auditFacetsCommand(), a.auditHistogramCommand(), a.auditAggregateCommand(), a.auditFieldsCommand(), a.auditExportCommand())
	return cmd
}

func (a *app) auditWindowFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&a.audit.query, "query", "", "the filter: KQL, lucene:<query> or a Query DSL JSON body; empty matches every event")
	flags.StringVar(&a.audit.from, "from", "", "start of the window, such as now-7d or an RFC 3339 time; 24 hours before --to by default")
	flags.StringVar(&a.audit.to, "to", "", "end of the window; now by default")
}

func (o auditOptions) window() url.Values {
	query := url.Values{}
	for key, value := range map[string]string{"q": o.query, "since": o.from, "until": o.to} {
		if value != "" {
			query.Set(key, value)
		}
	}
	return query
}

func (a *app) auditSearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [--query <filter>] [--from <time>] [--to <time>] [--limit N] [--cursor C | --all]",
		Short: "List the matching audit events, newest first",
		Long:  "search prints one line per event: when, who, through what, the action, how it ended and its target.\nPages never repeat or skip an event, even while new ones are recorded; --cursor keeps the window of the first page.",
		Args:  noArgs,
		RunE:  a.runAuditSearch,
	}
	a.auditWindowFlags(cmd)
	addPageFlags(cmd, &a.audit.page, maxAuditPage)
	return cmd
}

func (a *app) runAuditSearch(cmd *cobra.Command, _ []string) error {
	if err := a.audit.page.check(); err != nil {
		return err
	}
	query := a.audit.window()
	for key, values := range a.audit.page.query() {
		query[key] = values
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.getPages(cmd.Context(), "audit/events", query, a.audit.page.all)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	page, err := decode[auditEventList](data)
	if err != nil {
		return err
	}
	if len(page.Events) == 0 {
		out.notice("No audit events match in the window.")
		return nil
	}
	out.table(auditHeaders, auditRows(page.Events))
	pageFooter(out, len(page.Events), nil, page.NextCursor)
	return nil
}

var auditHeaders = []string{"WHEN", "WHO", "VIA", "ACTION", "OUTCOME", "TARGET", "ID"}

func auditRows(events []auditEvent) [][]string {
	rows := make([][]string, 0, len(events))
	for _, e := range events {
		rows = append(rows, []string{auditWhen(e.OccurredAt), auditWho(e), auditVia(e), text(e.Action), auditOutcome(e), auditTarget(e), e.ID})
	}
	return rows
}

func auditWhen(raw string) string {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return text(raw)
	}
	return t.UTC().Format(auditTimeLayout)
}

func auditWho(e auditEvent) string {
	who := e.ActorKind
	if e.ActorName != nil && *e.ActorName != "" {
		who = *e.ActorName
	}
	if e.AgentName != nil && *e.AgentName != "" {
		who = *e.AgentName + " for " + who
	}
	return text(who)
}

func auditVia(e auditEvent) string {
	if e.Tool != nil && *e.Tool != "" {
		return text(e.Origin + ":" + *e.Tool)
	}
	return text(e.Origin)
}

func auditOutcome(e auditEvent) string {
	if e.Reason != nil && *e.Reason != "" {
		return text(e.Outcome + " (" + *e.Reason + ")")
	}
	return text(e.Outcome)
}

func auditTarget(e auditEvent) string {
	if e.TargetKind == nil {
		return "-"
	}
	target := *e.TargetKind
	switch {
	case e.TargetName != nil && *e.TargetName != "":
		target += " " + *e.TargetName
	case e.TargetID != nil && *e.TargetID != "":
		target += " " + *e.TargetID
	}
	return text(target)
}

func (a *app) auditShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <event id>",
		Short: "Print one audit event and the other events of its request and agent run",
		Args:  exactArgs(1, "an audit event id"),
		RunE:  a.runAuditShow,
	}
}

func (a *app) runAuditShow(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "audit/events/"+id, nil)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	d, err := decode[auditEventDetail](data)
	if err != nil {
		return err
	}
	e := d.Event
	out.fields([][2]string{
		{"id", e.ID}, {"when", auditWhen(e.OccurredAt)}, {"action", text(e.Action)}, {"outcome", auditOutcome(e)},
		{"status", auditStatus(e.Status)}, {"who", auditWho(e)}, {"initiator", text(e.Initiator)}, {"via", auditVia(e)},
		{"route", orDash(e.Route)}, {"target", auditTarget(e)}, {"client ip", orDash(e.ClientIP)},
		{"request", orDash(e.RequestID)}, {"run", orDash(e.RunID)},
	})
	for _, related := range []struct {
		title  string
		events []auditEvent
	}{{"Same request", d.SameRequest}, {"Same agent run", d.SameRun}} {
		if len(related.events) < 2 {
			continue
		}
		out.line("")
		out.line("%s:", related.title)
		out.table(auditHeaders, auditRows(related.events))
	}
	return nil
}

func auditStatus(status *int) string {
	if status == nil {
		return "-"
	}
	return strconv.Itoa(*status)
}

func (a *app) auditFacetsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "facets <field> [--query <filter>] [--from <time>] [--to <time>] [--limit 10]",
		Short:   "Count the most frequent values of one field among the matching events",
		Example: "  observe audit facets actor_name --query outcome:denied --from now-7d",
		Args:    exactArgs(1, "a field name, such as actor_name; observe audit fields lists them"),
		RunE:    a.runAuditFacets,
	}
	a.auditWindowFlags(cmd)
	cmd.Flags().IntVar(&a.audit.limit, "limit", defaultAuditFacets, fmt.Sprintf("values to print, 1 to %d", maxAuditFacets))
	return cmd
}

func (a *app) runAuditFacets(cmd *cobra.Command, args []string) error {
	if a.audit.limit < 1 || a.audit.limit > maxAuditFacets {
		return usagef("--limit must be 1 to %d", maxAuditFacets)
	}
	query := a.audit.window()
	query.Set("field", args[0])
	query.Set("limit", strconv.Itoa(a.audit.limit))
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "audit/facets", query)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	facets, err := decode[auditFacets](data)
	if err != nil {
		return err
	}
	if len(facets.Values) == 0 {
		out.notice("No audit events with %s match in the window.", text(facets.Field))
		return nil
	}
	rows := make([][]string, 0, len(facets.Values))
	for _, v := range facets.Values {
		rows = append(rows, []string{strconv.FormatInt(v.Count, 10), text(v.Value)})
	}
	out.table([]string{"COUNT", strings.ToUpper(text(facets.Field))}, rows)
	return nil
}

func (a *app) auditHistogramCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "histogram [--query <filter>] [--from <time>] [--to <time>] [--buckets 60]",
		Short: "Count the matching events over time",
		Long:  "histogram splits the window into at most --buckets buckets of one step, from 1s to 1d, and prints the count of each, oldest first, empty buckets included.",
		Args:  noArgs,
		RunE:  a.runAuditHistogram,
	}
	a.auditWindowFlags(cmd)
	cmd.Flags().IntVar(&a.audit.buckets, "buckets", defaultAuditBuckets, fmt.Sprintf("the most buckets, 1 to %d", maxAuditBuckets))
	return cmd
}

func (a *app) runAuditHistogram(cmd *cobra.Command, _ []string) error {
	if a.audit.buckets < 1 || a.audit.buckets > maxAuditBuckets {
		return usagef("--buckets must be 1 to %d", maxAuditBuckets)
	}
	query := a.audit.window()
	query.Set("buckets", strconv.Itoa(a.audit.buckets))
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "audit/histogram", query)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	h, err := decode[auditHistogram](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(h.Buckets))
	for _, b := range h.Buckets {
		rows = append(rows, []string{auditWhen(b.Time), strconv.FormatInt(b.Count, 10)})
	}
	out.table([]string{"FROM", "COUNT"}, rows)
	out.notice("%d events, one bucket per %s.", h.Total, time.Duration(h.StepMs)*time.Millisecond)
	return nil
}

func (a *app) auditAggregateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "aggregate --query <Query DSL body with aggs> [--from <time>] [--to <time>]",
		Short:   "Run the aggs of a Query DSL body over the matching events",
		Long:    "aggregate runs date_histogram and terms buckets, up to three levels, with cardinality, value_count, min, max, avg, sum and percentiles metrics, and prints one row per bucket: its keys, doc_count and metrics.",
		Example: `  observe audit aggregate --from now-7d --query '{"aggs":{"by":{"terms":{"field":"origin"},"aggs":{"people":{"cardinality":{"field":"actor_id"}}}}}}'`,
		Args:    noArgs,
		RunE:    a.runAuditAggregate,
	}
	a.auditWindowFlags(cmd)
	return cmd
}

func (a *app) runAuditAggregate(cmd *cobra.Command, _ []string) error {
	if !strings.HasPrefix(strings.TrimSpace(a.audit.query), "{") {
		return usagef("--query must be a Query DSL JSON body with aggs, such as '{\"aggs\":{\"by\":{\"terms\":{\"field\":\"origin\"}}}}'")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "audit/aggregate", a.audit.window())
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	agg, err := decode[auditAggregation](data)
	if err != nil {
		return err
	}
	headers := make([]string, 0, len(agg.Levels)+1+len(agg.Metrics))
	for _, l := range agg.Levels {
		headers = append(headers, strings.ToUpper(text(l.Name)))
	}
	headers = append(headers, "DOC_COUNT")
	for _, m := range agg.Metrics {
		name := m.Name
		if m.Key != nil {
			name += "[" + *m.Key + "]"
		}
		headers = append(headers, strings.ToUpper(text(name)))
	}
	rows := make([][]string, 0, len(agg.Rows))
	for _, r := range agg.Rows {
		row := make([]string, 0, len(headers))
		for _, k := range r.Keys {
			row = append(row, orDash(k))
		}
		row = append(row, strconv.FormatInt(r.DocCount, 10))
		for _, v := range r.Values {
			if v == nil {
				row = append(row, "-")
				continue
			}
			row = append(row, strconv.FormatFloat(*v, 'g', -1, 64))
		}
		rows = append(rows, row)
	}
	out.table(headers, rows)
	if agg.Truncated {
		out.notice("The rows stop at the server's limit and more exist; use a larger interval, a smaller size or a shorter window.")
	}
	return nil
}

func (a *app) auditFieldsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "fields",
		Short: "List the fields an audit query can use",
		Args:  noArgs,
		RunE:  a.runAuditFields,
	}
}

func (a *app) runAuditFields(cmd *cobra.Command, _ []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "audit/fields", nil)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	list, err := decode[auditFieldList](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(list.Fields))
	for _, f := range list.Fields {
		rows = append(rows, []string{text(f.Name), orDash(f.Alias), text(f.Type), text(f.Description)})
	}
	out.table([]string{"FIELD", "ALIAS", "TYPE", "DESCRIPTION"}, rows)
	return nil
}

func (a *app) auditExportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export [--query <filter>] [--from <time>] [--to <time>] [--format ndjson|csv] [--file <path>]",
		Short: "Download the matching events as NDJSON or CSV",
		Long:  "export streams every matching event in the window, newest first, to --file or to standard output: one JSON object per line, or CSV with a header row and data as JSON text.\nAn export stops at the server's row limit, one million rows, and says so; narrow the window to export the rest.\nThe export is itself recorded in the trail.",
		Args:  noArgs,
		RunE:  a.runAuditExport,
	}
	a.auditWindowFlags(cmd)
	cmd.Flags().StringVar(&a.audit.format, "format", "ndjson", "ndjson or csv")
	cmd.Flags().StringVar(&a.audit.file, "file", "", "write to this file instead of standard output")
	return cmd
}

func (a *app) runAuditExport(cmd *cobra.Command, _ []string) (err error) {
	accept, ok := auditExportFormats[a.audit.format]
	if !ok {
		return usagef("--format must be ndjson or csv, not %q", text(a.audit.format))
	}
	query := a.audit.window()
	query.Set("format", a.audit.format)
	c, err := a.newClient()
	if err != nil {
		return err
	}
	resp, err := c.download(cmd.Context(), "audit/export", query, accept)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	dst := a.std.Out
	if a.audit.file != "" {
		f, err := os.Create(a.audit.file)
		if err != nil {
			return usagef("cannot write %s: %v", a.audit.file, err)
		}
		defer func() {
			if cerr := f.Close(); err == nil && cerr != nil {
				err = fmt.Errorf("write %s: %w", a.audit.file, cerr)
			}
		}()
		dst = f
	}
	rows, err := copyExport(dst, resp.Body, a.audit.format)
	if err != nil {
		if cmd.Context().Err() != nil {
			return errInterrupted
		}
		return fmt.Errorf("the export stopped after %d rows: %w", rows, err)
	}
	out := a.output()
	if limit, convErr := strconv.Atoi(resp.Header.Get(exportRowLimitKey)); convErr == nil && rows >= limit {
		out.warn("The export stopped at the server's limit of %d rows; narrow --from and --to to export the rest.", limit)
	}
	if a.audit.file != "" {
		out.confirm("Wrote %d events to %s.", rows, a.audit.file)
	}
	return nil
}

func copyExport(dst io.Writer, src io.Reader, format string) (int, error) {
	if format == "csv" {
		r := csv.NewReader(io.TeeReader(src, dst))
		r.FieldsPerRecord = -1
		rows := -1
		for {
			_, err := r.Read()
			if errors.Is(err, io.EOF) {
				return max(rows, 0), nil
			}
			if err != nil {
				return max(rows, 0), err
			}
			rows++
		}
	}
	dec := json.NewDecoder(io.TeeReader(src, dst))
	rows := 0
	for {
		var raw json.RawMessage
		err := dec.Decode(&raw)
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return rows, err
		}
		rows++
	}
}
