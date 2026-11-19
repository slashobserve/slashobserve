package cli

import (
	"maps"
	"math"
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

type seriesSummary struct {
	Name   string
	Points int
	Last   float64
	Min    float64
	Max    float64
	Mean   float64
}

func (a *app) metricsGroupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Query a Prometheus datasource",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.metricsQueryCommand())
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
