package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) queryTraces(cmd *cobra.Command, args []string) error {
	opts := a.traces
	if err := traceQueryUsage(cmd, opts, args); err != nil {
		return err
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
	if kind != "clickhouse" {
		return usagef("--query runs SQL on a ClickHouse datasource; %s is %s", text(opts.datasource), text(kind))
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

func traceQueryUsage(cmd *cobra.Command, opts tracesOptions, args []string) error {
	if cmd.Flags().Changed("compare") {
		return usagef("give --compare or --query, not both")
	}
	if _, err := tracesFlagsUsage(cmd, opts); err != nil {
		return err
	}
	switch {
	case strings.TrimSpace(opts.query) == "":
		return usagef("--query needs SQL")
	case len(args) > 0:
		return usagef("give a trace id or --query, not both")
	case opts.ask != "":
		return usagef("give --ask or --query, not both")
	case opts.dryRun:
		return usagef("--dry-run needs --ask")
	case opts.datasource == "":
		return usagef("%s needs --datasource <uid>; observe doctor lists them", cmd.CommandPath())
	}
	return nil
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
