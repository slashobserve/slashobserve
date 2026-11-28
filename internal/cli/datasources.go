package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var errDatasourceUnhealthy = errors.New("the datasource is still unhealthy")

const (
	maxNamesLimit   = 500
	defaultNames    = 100
	maxVolumeBucket = 200
	defaultBuckets  = 60
)

type nameListOptions struct {
	search string
	metric string
	table  string
	from   string
	to     string
	limit  int
}

type rangeOptions struct {
	from    string
	to      string
	buckets int
}

type metricRow struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Unit  string `json:"unit"`
	Table string `json:"table"`
	Help  string `json:"help"`
}

type metricsBody struct {
	Metrics   []metricRow `json:"metrics"`
	Truncated bool        `json:"truncated"`
}

type namesBody struct {
	Labels    []string `json:"labels"`
	Values    []string `json:"values"`
	Truncated bool     `json:"truncated"`
}

type lintBody struct {
	Diagnostics []struct {
		Severity string `json:"severity"`
		Message  string `json:"message"`
		Start    int    `json:"start"`
		End      int    `json:"end"`
	} `json:"diagnostics"`
}

type volumeBody struct {
	Total       int    `json:"total"`
	Partial     bool   `json:"partial"`
	CountedFrom string `json:"counted_from"`
	Buckets     []struct {
		Start   string `json:"start"`
		Error   int    `json:"error"`
		Warning int    `json:"warning"`
		Info    int    `json:"info"`
		Other   int    `json:"other"`
	} `json:"buckets"`
}

type traceStatsBody struct {
	Spans   int  `json:"spans"`
	Errors  int  `json:"errors"`
	Partial bool `json:"partial"`
	Buckets []struct {
		Start  string   `json:"start"`
		Spans  int      `json:"spans"`
		Errors int      `json:"errors"`
		P50    *float64 `json:"p50_ms"`
		P95    *float64 `json:"p95_ms"`
		P99    *float64 `json:"p99_ms"`
	} `json:"buckets"`
}

func (a *app) datasourcesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "datasources",
		Short: "List, recheck and manage datasources, discover their metrics, labels and values, and lint, count and sample queries",
		Long:  "datasources is how a script or an agent with only a shell finds what to query before it writes a query.\nlist prints the datasources and recheck runs a fresh health check on one.\nget, add, edit, test and remove manage the datasources added in the app and need an Admin key; a datasource from the datasources file cannot be changed here.\nmetrics, labels, label-values and services name what a datasource holds, lint checks a query without running it, and log-volume, trace-stats and trace-logs summarise logs and traces.\nA datasource is named by its uid, the first column of datasources list.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.datasourcesListCommand(), a.datasourcesRecheckCommand(), a.datasourcesGetCommand(), a.datasourcesAddCommand(), a.datasourcesEditCommand(), a.datasourcesTestCommand(), a.datasourcesRemoveCommand(), a.datasourceSchemaCommand(), a.datasourceSampleCommand(), a.metricsCommand(), a.labelsCommand(), a.labelValuesCommand(), a.servicesCommand(), a.lintCommand(), a.logVolumeCommand(), a.traceStatsCommand(), a.traceLogsCommand())
	return cmd
}

func (a *app) datasourcesListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the datasources with their health",
		Long:  "list prints each datasource's uid, type, health, who can query it (for an Admin key), the dashboards that use it and when it was last checked.\nAn unhealthy one shows why.",
		Args:  noArgs,
		RunE:  a.runDatasourcesList,
	}
}

func (a *app) datasourcesRecheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "recheck <uid>",
		Short: "Run a fresh health check on one datasource",
		Long:  "recheck skips the server's 10 second health cache and prints the new status.\nIt exits 1 when the datasource is still unhealthy.",
		Args:  exactArgs(1, "a datasource uid"),
		RunE:  a.runDatasourcesRecheck,
	}
}

func (a *app) runDatasourcesList(cmd *cobra.Command, _ []string) error {
	data, err := a.fetch(cmd.Context(), "datasources", nil)
	if err != nil {
		return err
	}
	out := a.output()
	return render(out, data, func(list datasourceList) error {
		if len(list.Datasources) == 0 {
			out.notice("No datasources yet")
			return nil
		}
		withAudience := slices.ContainsFunc(list.Datasources, func(ds datasourceEntry) bool { return ds.Audience != "" })
		header := []string{"UID", "TYPE", "NAME", "HEALTH", "SOURCE", "DASHBOARDS", "CHECKED"}
		rows := make([][]string, 0, len(list.Datasources))
		for _, ds := range list.Datasources {
			row := []string{text(ds.UID), text(ds.Type), text(ds.Name), datasourceHealth(ds), text(ds.Source), strconv.Itoa(ds.DashboardCount), when(ds.CheckedAt)}
			if withAudience {
				row = slices.Insert(row, 5, text(ds.Audience))
			}
			rows = append(rows, row)
		}
		if withAudience {
			header = slices.Insert(header, 5, "AUDIENCE")
		}
		out.table(header, rows)
		return nil
	})
}

func (a *app) runDatasourcesRecheck(cmd *cobra.Command, args []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "datasources/"+url.PathEscape(args[0])+"/recheck", struct{}{})
	if err != nil {
		return err
	}
	ds, err := decode[datasourceEntry](data)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
	} else {
		out.fields([][2]string{
			{"UID", text(ds.UID)},
			{"Type", text(ds.Type)},
			{"Health", datasourceHealth(ds)},
			{"Checked", when(ds.CheckedAt)},
		})
	}
	if !ds.Healthy {
		return errDatasourceUnhealthy
	}
	return nil
}

func datasourceHealth(ds datasourceEntry) string {
	if ds.Healthy {
		return "healthy"
	}
	health := "unhealthy"
	if message := orEmpty(ds.Message); message != "" {
		health += ": " + text(message)
	}
	if detail := orEmpty(ds.Detail); detail != "" {
		health += " (" + text(detail) + ")"
	}
	return health
}

func (o *nameListOptions) addFlags(cmd *cobra.Command, withMetric, withWindow bool) {
	flags := cmd.Flags()
	flags.StringVar(&o.search, "search", "", "keep names that contain this text, case-insensitively")
	flags.IntVar(&o.limit, "limit", defaultNames, fmt.Sprintf("names to return, 1 to %d", maxNamesLimit))
	if withMetric {
		flags.StringVar(&o.metric, "metric", "", "keep names seen on this metric; on ClickHouse it also finds the table")
		flags.StringVar(&o.table, "table", "", "ClickHouse table such as otel_logs; Prometheus ignores it")
	}
	if withWindow {
		flags.StringVar(&o.from, "from", "", "start of the window the names were seen in, such as now-6h; give --to with it")
		flags.StringVar(&o.to, "to", "", "end of the window")
	}
}

func (o nameListOptions) query() (url.Values, error) {
	if o.limit < 1 || o.limit > maxNamesLimit {
		return nil, usagef("--limit must be 1 to %d", maxNamesLimit)
	}
	if (o.from == "") != (o.to == "") {
		return nil, usagef("--from and --to are given together or not at all")
	}
	query := url.Values{"limit": {strconv.Itoa(o.limit)}}
	for key, value := range map[string]string{"search": o.search, "metric": o.metric, "table": o.table, "from": o.from, "to": o.to} {
		if value != "" {
			query.Set(key, value)
		}
	}
	return query, nil
}

func (a *app) getNames(cmd *cobra.Command, uid, path string, opts nameListOptions) ([]byte, error) {
	query, err := opts.query()
	if err != nil {
		return nil, err
	}
	c, err := a.newClient()
	if err != nil {
		return nil, err
	}
	return c.get(cmd.Context(), "datasources/"+url.PathEscape(uid)+"/"+path, query)
}

func truncatedNotice(out *output, truncated bool, limit int) {
	if truncated {
		out.notice("More exist beyond the first %d. Raise --limit (at most %d) or narrow with --search.", limit, maxNamesLimit)
	}
}

func (a *app) metricsCommand() *cobra.Command {
	var opts nameListOptions
	cmd := &cobra.Command{
		Use:   "metrics <datasource-uid>",
		Short: "List the metric names of a Prometheus or ClickHouse datasource",
		Long:  "metrics lists the metric names of a datasource with their type and help, or on ClickHouse the OpenTelemetry metric names with their table.",
		Args:  exactArgs(1, "one datasource uid"),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := a.getNames(cmd, args[0], "metrics", opts)
			if err != nil {
				return err
			}
			out := a.output()
			return render(out, data, func(body metricsBody) error {
				rows := make([][]string, 0, len(body.Metrics))
				for _, m := range body.Metrics {
					rows = append(rows, []string{text(m.Name), dash(m.Type), dash(m.Unit), dash(m.Table), text(m.Help)})
				}
				out.table([]string{"NAME", "TYPE", "UNIT", "TABLE", "HELP"}, rows)
				truncatedNotice(out, body.Truncated, opts.limit)
				return nil
			})
		},
	}
	opts.addFlags(cmd, false, false)
	return cmd
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return text(s)
}

func (a *app) labelsCommand() *cobra.Command {
	var opts nameListOptions
	cmd := &cobra.Command{
		Use:   "labels <datasource-uid>",
		Short: "List the label names of a datasource, or its attribute keys on ClickHouse",
		Long:  "labels lists the label names of a datasource.\nOn ClickHouse it lists the OpenTelemetry attribute keys of a table, so it needs --table or --metric.",
		Args:  exactArgs(1, "one datasource uid"),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := a.getNames(cmd, args[0], "labels", opts)
			if err != nil {
				return err
			}
			return a.printNames(data, opts.limit, func(b namesBody) []string { return b.Labels })
		},
	}
	opts.addFlags(cmd, true, true)
	return cmd
}

func (a *app) labelValuesCommand() *cobra.Command {
	var opts nameListOptions
	cmd := &cobra.Command{
		Use:   "label-values <datasource-uid> <label>",
		Short: "List the values of one label of a datasource",
		Long:  "label-values lists the values a label takes on a datasource, one per line.\nOn ClickHouse it needs --table or --metric, as labels does.",
		Args:  exactArgs(2, "a datasource uid and a label name"),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := a.getNames(cmd, args[0], "labels/"+url.PathEscape(args[1])+"/values", opts)
			if err != nil {
				return err
			}
			return a.printNames(data, opts.limit, func(b namesBody) []string { return b.Values })
		},
	}
	opts.addFlags(cmd, true, true)
	return cmd
}

func (a *app) printNames(data []byte, limit int, pick func(namesBody) []string) error {
	out := a.output()
	return render(out, data, func(body namesBody) error {
		for _, name := range pick(body) {
			out.line("%s", text(name))
		}
		truncatedNotice(out, body.Truncated, limit)
		return nil
	})
}

func (a *app) lintCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "lint <datasource-uid> <query>",
		Short: "Check a query against a datasource without running it",
		Long:  "lint parses a query and prints its diagnostics, each with the byte range it covers.\nPrometheus queries are also checked for metric names that do not exist, and ClickHouse queries with EXPLAIN SYNTAX.\nThe exit code is 1 when a diagnostic is an error, and 0 for a clean query or warnings only.",
		Args:  minimumArgs(2, "a datasource uid and a query"),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.newClient()
			if err != nil {
				return err
			}
			data, err := c.post(cmd.Context(), "datasources/"+url.PathEscape(args[0])+"/lint", map[string]string{"query": strings.Join(args[1:], " ")})
			if err != nil {
				return err
			}
			body, err := decode[lintBody](data)
			if err != nil {
				return err
			}
			out := a.output()
			failures := 0
			for _, d := range body.Diagnostics {
				if d.Severity == "error" {
					failures++
				}
			}
			switch {
			case a.json:
				out.body(data)
			case len(body.Diagnostics) == 0:
				out.confirm("No diagnostics")
			default:
				for _, d := range body.Diagnostics {
					out.line("%s %d-%d  %s", text(d.Severity), d.Start, d.End, text(d.Message))
				}
			}
			if failures > 0 {
				return fmt.Errorf("the query has %d errors", failures)
			}
			return nil
		},
	}
}

func (o *rangeOptions) addFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&o.from, "from", "now-1h", "start of the range, such as now-15m or an RFC 3339 time")
	flags.StringVar(&o.to, "to", "now", "end of the range")
	flags.IntVar(&o.buckets, "buckets", defaultBuckets, fmt.Sprintf("the most buckets wanted, 1 to %d", maxVolumeBucket))
}

func (o rangeOptions) check() error {
	if o.buckets < 1 || o.buckets > maxVolumeBucket {
		return usagef("--buckets must be 1 to %d", maxVolumeBucket)
	}
	return nil
}

type rangeRequest struct {
	Query   string `json:"query"`
	From    string `json:"from"`
	To      string `json:"to"`
	Buckets int    `json:"buckets"`
}

func (a *app) postRange(ctx context.Context, uid, path string, opts rangeOptions, query string) ([]byte, error) {
	c, err := a.newClient()
	if err != nil {
		return nil, err
	}
	return c.post(ctx, "datasources/"+url.PathEscape(uid)+"/"+path, rangeRequest{Query: query, From: opts.from, To: opts.to, Buckets: opts.buckets})
}

func (a *app) logVolumeCommand() *cobra.Command {
	var opts rangeOptions
	cmd := &cobra.Command{
		Use:   "log-volume <datasource-uid> <query>",
		Short: "Count the log lines a query matches over a range, by level",
		Long:  "log-volume counts every line a logs query matches over the range, not only the lines logs prints, and files them as error, warning, info or other per bucket.\nThe query is in the language of the datasource, as for observe logs.\nWhen the datasource cannot count the whole range in time, the output says from when it counted.",
		Args:  minimumArgs(2, "a datasource uid and a log query"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.check(); err != nil {
				return err
			}
			data, err := a.postRange(cmd.Context(), args[0], "log-volume", opts, strings.Join(args[1:], " "))
			if err != nil {
				return err
			}
			out := a.output()
			return render(out, data, func(body volumeBody) error {
				rows := make([][]string, 0, len(body.Buckets))
				for _, b := range body.Buckets {
					rows = append(rows, []string{text(b.Start), strconv.Itoa(b.Error), strconv.Itoa(b.Warning), strconv.Itoa(b.Info), strconv.Itoa(b.Other)})
				}
				out.table([]string{"START", "ERROR", "WARNING", "INFO", "OTHER"}, rows)
				out.line("Total %d", body.Total)
				if body.Partial {
					out.warn("The datasource ran out of time, so only the lines from %s on were counted; the earlier buckets read 0.", body.CountedFrom)
				}
				return nil
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func (a *app) traceStatsCommand() *cobra.Command {
	var opts rangeOptions
	cmd := &cobra.Command{
		Use:   "trace-stats <datasource-uid> [<filter>]",
		Short: "Count spans and errors per bucket and give their duration percentiles",
		Long:  "trace-stats counts the spans a filter matches per bucket, with their errors and the p50, p95 and p99 durations in milliseconds.\nThe filter is a TraceQL spanset on Tempo, such as { resource.service.name = \"checkout\" }, and a SQL condition over otel_traces on ClickHouse; leave it out to match every span.\nWhen Tempo did not search every block the counts are a lower bound, and the output says so.",
		Args:  minimumArgs(1, "a datasource uid and an optional filter"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.check(); err != nil {
				return err
			}
			query := strings.Join(args[1:], " ")
			if query == "" {
				var err error
				if query, err = a.matchEverySpan(cmd.Context(), args[0]); err != nil {
					return err
				}
			}
			data, err := a.postRange(cmd.Context(), args[0], "trace-stats", opts, query)
			if err != nil {
				return err
			}
			out := a.output()
			return render(out, data, func(body traceStatsBody) error {
				rows := make([][]string, 0, len(body.Buckets))
				for _, b := range body.Buckets {
					rows = append(rows, []string{text(b.Start), strconv.Itoa(b.Spans), strconv.Itoa(b.Errors), millisText(b.P50), millisText(b.P95), millisText(b.P99)})
				}
				out.table([]string{"START", "SPANS", "ERRORS", "P50", "P95", "P99"}, rows)
				out.line("Spans %d, errors %d", body.Spans, body.Errors)
				if body.Partial {
					out.warn("Tempo did not search every block, so the counts are a lower bound.")
				}
				return nil
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func (a *app) matchEverySpan(ctx context.Context, uid string) (string, error) {
	c, err := a.newClient()
	if err != nil {
		return "", err
	}
	kind, err := datasourceType(ctx, c, uid)
	if err != nil {
		return "", err
	}
	if kind == "tempo" {
		return "{}", nil
	}
	return "", nil
}

func millisText(ms *float64) string {
	if ms == nil {
		return "-"
	}
	return strconv.FormatFloat(*ms, 'f', -1, 64) + "ms"
}

func (a *app) traceLogsCommand() *cobra.Command {
	var from, to, span, service string
	var limit int
	cmd := &cobra.Command{
		Use:   "trace-logs <datasource-uid> <trace-id>",
		Short: "Print the log lines that carry a trace id",
		Long:  "trace-logs reads the log lines of a Loki, ClickHouse, Elasticsearch or OpenSearch datasource that carry a trace id, or one span id with --span.\nGive --from and --to a minute either side of the trace.\nWith --service, Loki reads only the streams of that service and Elasticsearch and OpenSearch only the documents of that service.",
		Args:  exactArgs(2, "a logs datasource uid and a trace id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > maxLogLimit {
				return usagef("--limit must be 1 to %d", maxLogLimit)
			}
			body := map[string]any{"trace_id": args[1], "from": from, "to": to, "limit": limit}
			if span != "" {
				body["span_id"] = span
			}
			if service != "" {
				body["service"] = service
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			data, err := c.post(cmd.Context(), "datasources/"+url.PathEscape(args[0])+"/trace-logs", body)
			if err != nil {
				return err
			}
			out := a.output()
			return render(out, data, func(parsed exploreResponse) error {
				printLogLines(out, newestLines(logLines(parsed.Frames), limit), from, to)
				return nil
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&from, "from", "now-1h", "start of the range, such as now-15m or an RFC 3339 time")
	flags.StringVar(&to, "to", "now", "end of the range")
	flags.StringVar(&span, "span", "", "keep only the lines of this span id")
	flags.StringVar(&service, "service", "", "the span's service name, to narrow Loki streams")
	flags.IntVar(&limit, "limit", defaultLogLimit, fmt.Sprintf("the newest lines to print, 1 to %d", maxLogLimit))
	return cmd
}
