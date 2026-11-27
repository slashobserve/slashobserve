package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) queryTraces(cmd *cobra.Command, args []string) error {
	opts := a.traces
	sort, err := traceQueryUsage(cmd, opts, args)
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	ds, err := datasourceFor(ctx, c, opts.datasource)
	if err != nil {
		return err
	}
	if err := tracesKindUsage(ds); err != nil {
		return err
	}
	kind := ds.Type
	if kind != "clickhouse" {
		request := exploreRequest{Query: opts.query, Kind: "instant", From: opts.from, To: opts.to, Limit: traceSearchLimit}
		return a.printTraceSearch(ctx, c, request, sort)
	}
	if sort != nil {
		return usagef("--sort does not apply to SQL on a ClickHouse datasource; write an ORDER BY")
	}
	data, body, err := explore(ctx, c, opts.datasource, exploreRequest{Query: opts.query, Kind: "instant", From: opts.from, To: opts.to, PageSize: traceSearchLimit, Page: 1, AsWritten: true})
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	headers, rows := resultTableWith(body.Frames, traceQueryCell)
	if len(rows) == 0 {
		out.notice("No rows between %s and %s", text(opts.from), text(opts.to))
		return nil
	}
	out.table(headers, rows)
	if body.Meta.Page != nil && body.Meta.Page.HasNext {
		out.notice("Showing the first %d rows; narrow --from and --to or the query to see the rest", traceSearchLimit)
	}
	return nil
}

func traceQueryUsage(cmd *cobra.Command, opts tracesOptions, args []string) (*traceSort, error) {
	if cmd.Flags().Changed("compare") {
		return nil, usagef("give --compare or --query, not both")
	}
	sort, err := tracesFlagsUsage(cmd, opts)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.TrimSpace(opts.query) == "":
		return nil, usagef("--query needs a query: SQL on ClickHouse, TraceQL on Tempo, key=value pairs on Jaeger")
	case len(args) > 0:
		return nil, usagef("give a trace id or --query, not both")
	case opts.ask != "":
		return nil, usagef("give --ask or --query, not both")
	case opts.dryRun:
		return nil, usagef("--dry-run needs --ask")
	case opts.datasource == "":
		return nil, usagef("%s needs --datasource <uid>; observe doctor lists them", cmd.CommandPath())
	}
	return sort, nil
}

func resultTable(frames []frame) ([]string, [][]string) {
	return resultTableWith(frames, cellText)
}

func resultTableWith(frames []frame, cell func(f frame, field, row int) string) ([]string, [][]string) {
	for _, f := range frames {
		if f.rows() == 0 {
			continue
		}
		headers := make([]string, len(f.Fields))
		for i, field := range f.Fields {
			headers[i] = text(field.Name)
		}
		rows := make([][]string, f.rows())
		for row := range rows {
			cells := make([]string, len(f.Fields))
			for i := range f.Fields {
				cells[i] = cell(f, i, row)
			}
			rows[row] = cells
		}
		return headers, rows
	}
	return nil, nil
}

func cellText(f frame, field, row int) string {
	if f.Fields[field].Type == "time" {
		if ms, ok := f.numberAt(field, row); ok {
			return time.UnixMilli(int64(ms)).UTC().Format(time.RFC3339Nano)
		}
	}
	return text(f.stringAt(field, row))
}

func traceQueryCell(f frame, field, row int) string {
	if f.Fields[field].Type == "number" && strings.EqualFold(f.Fields[field].Name, "Duration") {
		return durationText(f, field, row)
	}
	return cellText(f, field, row)
}
