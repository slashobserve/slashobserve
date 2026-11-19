package cli

import (
	"context"
	"maps"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type metricsQueryOptions struct {
	datasource string
	from       string
	to         string
	instant    bool
	ask        string
	dryRun     bool
}

const (
	defaultNameLimit = 100
	maxNameLimit     = 500
)

type metricsNamesOptions struct {
	datasource string
	metric     string
	search     string
	limit      int
}

type metricNames struct {
	Metrics []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Help string `json:"help"`
	} `json:"metrics"`
	Truncated bool `json:"truncated"`
}

type nameList struct {
	Labels    []string `json:"labels"`
	Values    []string `json:"values"`
	Truncated bool     `json:"truncated"`
}

type seriesSummary struct {
	Name   string
	Points int
	Last   float64
	Min    float64
	Max    float64
	Mean   float64
}

func (a *app) metricsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Query a Prometheus datasource",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.metricsQueryCommand(), a.metricsNamesCommand(), a.metricsLabelsCommand(), a.metricsValuesCommand())
	return cmd
}

func (a *app) metricsQueryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query [<promql>] --datasource <uid>",
		Short: "Run a PromQL query and print one line per series",
		Long:  "query runs a PromQL query on a Prometheus datasource over a time range and prints each series with its point count, last, min, max and mean.\nWith --instant it evaluates the query at the end of the range and prints one value per series.\nWith --ask the server writes the PromQL from a question and prints it first.\nThe server caps every query by time, series and points and says which limit it hit.",
		Example: "  observe metrics query 'sum by (service) (rate(http_requests_total[5m]))' --datasource prom --from now-6h\n" +
			"  observe metrics query 'up == 0' --datasource prom --instant\n" +
			"  observe metrics query --ask \"requests per second by service\" --datasource prom --dry-run",
		Args: cobra.ArbitraryArgs,
		RunE: a.runMetricsQuery,
	}
	flags := cmd.Flags()
	flags.StringVar(&a.metricsQuery.datasource, "datasource", "", "the Prometheus datasource uid (observe doctor lists them)")
	flags.StringVar(&a.metricsQuery.from, "from", "now-1h", "start of the range, such as now-15m or an RFC 3339 time")
	flags.StringVar(&a.metricsQuery.to, "to", "now", "end of the range")
	flags.BoolVar(&a.metricsQuery.instant, "instant", false, "evaluate the query once at the end of the range instead of over the whole range")
	flags.StringVar(&a.metricsQuery.ask, "ask", "", "a question the server turns into the PromQL query, instead of the query argument")
	flags.BoolVar(&a.metricsQuery.dryRun, "dry-run", false, "with --ask, print the query without running it")
	return cmd
}

func (a *app) runMetricsQuery(cmd *cobra.Command, args []string) error {
	opts := a.metricsQuery
	if err := askUsage(cmd.Name(), "a PromQL query", args, opts.ask, opts.dryRun); err != nil {
		return err
	}
	if opts.datasource == "" {
		return usagef("%s needs --datasource <uid>; observe doctor lists them", cmd.CommandPath())
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	out := a.output()
	query := strings.Join(args, " ")
	if opts.ask != "" {
		answer, err := draftQuery(cmd.Context(), c, opts.datasource, signalMetrics, opts.ask, opts.from, opts.to)
		if err != nil {
			return err
		}
		printDraft(out, answer)
		if opts.dryRun {
			return nil
		}
		query = answer.Query
	}
	kind := "range"
	if opts.instant {
		kind = "instant"
	}
	data, body, err := explore(cmd.Context(), c, opts.datasource, exploreRequest{Query: query, Kind: kind, From: opts.from, To: opts.to})
	if err != nil {
		return err
	}
	if a.json {
		out.body(data)
		return nil
	}
	series := summarizeSeries(body.Frames)
	if len(series) == 0 {
		out.notice("No series between %s and %s", text(opts.from), text(opts.to))
		return nil
	}
	printSeries(out, series, opts.instant)
	return nil
}

func summarizeSeries(frames []frame) []seriesSummary {
	var out []seriesSummary
	for _, f := range frames {
		value := fieldIndex(f, "Value")
		if value < 0 {
			value = firstOfType(f, "number")
		}
		s := seriesSummary{Name: seriesName(f), Min: math.Inf(1), Max: math.Inf(-1)}
		sum := 0.0
		for row := range f.rows() {
			n, ok := f.numberAt(value, row)
			if !ok {
				continue
			}
			s.Points++
			s.Last = n
			s.Min = math.Min(s.Min, n)
			s.Max = math.Max(s.Max, n)
			sum += n
		}
		if s.Points == 0 {
			continue
		}
		s.Mean = sum / float64(s.Points)
		out = append(out, s)
	}
	return out
}

func seriesName(f frame) string {
	keys := slices.Sorted(maps.Keys(f.Labels))
	return text(f.Name + labelText(f.Labels, keys))
}

func printSeries(out *output, series []seriesSummary, instant bool) {
	if instant {
		rows := make([][]string, len(series))
		for i, s := range series {
			rows[i] = []string{s.Name, number(s.Last)}
		}
		out.table([]string{"SERIES", "VALUE"}, rows)
		return
	}
	rows := make([][]string, len(series))
	for i, s := range series {
		rows[i] = []string{s.Name, strconv.Itoa(s.Points), number(s.Last), number(s.Min), number(s.Max), number(s.Mean)}
	}
	out.table([]string{"SERIES", "POINTS", "LAST", "MIN", "MAX", "MEAN"}, rows)
}

func number(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

func (a *app) nameFlags(cmd *cobra.Command, withMetric bool) {
	flags := cmd.Flags()
	flags.StringVar(&a.metricsNames.datasource, "datasource", "", "the Prometheus datasource uid (observe doctor lists them)")
	if withMetric {
		flags.StringVar(&a.metricsNames.metric, "metric", "", "only names seen on this metric")
	}
	flags.StringVar(&a.metricsNames.search, "search", "", "keep only names that contain this text, ignoring case")
	flags.IntVar(&a.metricsNames.limit, "limit", defaultNameLimit, "the most names to print, 1 to 500")
}

func (a *app) metricsNamesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "names --datasource <uid>",
		Short:   "List the metric names of a Prometheus datasource",
		Long:    "names lists the metrics a Prometheus datasource knows with their type and help text, up to --limit, and says when more exist.\nNarrow a long list with --search.",
		Example: "  observe metrics names --datasource prom --search http_",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runNames(cmd, "metrics", func(out *output, data []byte) error {
				list, err := decode[metricNames](data)
				if err != nil {
					return err
				}
				rows := make([][]string, len(list.Metrics))
				for i, m := range list.Metrics {
					rows[i] = []string{text(m.Name), text(m.Type), text(m.Help)}
				}
				if len(rows) == 0 {
					out.notice("No metrics found")
					return nil
				}
				out.table([]string{"NAME", "TYPE", "HELP"}, rows)
				a.truncatedNotice(out, list.Truncated)
				return nil
			})
		},
	}
	a.nameFlags(cmd, false)
	return cmd
}

func (a *app) metricsLabelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "labels --datasource <uid>",
		Short:   "List the label names of a Prometheus datasource",
		Long:    "labels lists the label names of a Prometheus datasource, or only those seen on one metric with --metric, up to --limit.",
		Example: "  observe metrics labels --datasource prom --metric http_requests_total",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runNames(cmd, "labels", func(out *output, data []byte) error {
				list, err := decode[nameList](data)
				if err != nil {
					return err
				}
				return a.printNameList(out, list.Labels, "No labels found", list.Truncated)
			})
		},
	}
	a.nameFlags(cmd, true)
	return cmd
}

func (a *app) metricsValuesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "values <label> --datasource <uid>",
		Short:   "List the values of one label of a Prometheus datasource",
		Long:    "values lists the values of one label, or only those seen on one metric with --metric, up to --limit.",
		Example: "  observe metrics values service --datasource prom --metric http_requests_total",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runNames(cmd, "labels/"+url.PathEscape(args[0])+"/values", func(out *output, data []byte) error {
				list, err := decode[nameList](data)
				if err != nil {
					return err
				}
				return a.printNameList(out, list.Values, "No values found", list.Truncated)
			})
		},
	}
	a.nameFlags(cmd, true)
	return cmd
}

func (a *app) runNames(cmd *cobra.Command, path string, show func(*output, []byte) error) error {
	opts := a.metricsNames
	if opts.datasource == "" {
		return usagef("%s needs --datasource <uid>; observe doctor lists them", cmd.CommandPath())
	}
	if opts.limit < 1 || opts.limit > maxNameLimit {
		return usagef("--limit must be 1 to %d", maxNameLimit)
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	query := url.Values{"limit": {strconv.Itoa(opts.limit)}}
	if opts.search != "" {
		query.Set("search", opts.search)
	}
	if opts.metric != "" {
		query.Set("metric", opts.metric)
	}
	data, err := getNames(cmd.Context(), c, opts.datasource, path, query)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	return show(out, data)
}

func getNames(ctx context.Context, c *client, uid, path string, query url.Values) ([]byte, error) {
	return c.get(ctx, "datasources/"+url.PathEscape(uid)+"/"+path, query)
}

func (a *app) printNameList(out *output, names []string, empty string, truncated bool) error {
	if len(names) == 0 {
		out.notice("%s", empty)
		return nil
	}
	for _, name := range names {
		out.line("%s", text(name))
	}
	a.truncatedNotice(out, truncated)
	return nil
}

func (a *app) truncatedNotice(out *output, truncated bool) {
	if truncated {
		out.notice("There are more; narrow with --search or raise --limit (at most %d)", maxNameLimit)
	}
}
