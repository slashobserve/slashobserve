package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

const maxSampleLimit = 20

type schemaBody struct {
	Datasource string          `json:"datasource"`
	Table      string          `json:"table"`
	Tables     []schemaTable   `json:"tables"`
	Columns    json.RawMessage `json:"columns"`
	Rows       [][]any         `json:"rows"`
	Attributes []string        `json:"attributes"`
	Truncated  bool            `json:"truncated"`
	NextOffset *int            `json:"next_offset"`
}

type schemaTable struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type schemaColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type sampleBody struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated"`
}

func (a *app) getDiscovery(cmd *cobra.Command, uid, path string, query url.Values) ([]byte, error) {
	c, err := a.newClient()
	if err != nil {
		return nil, err
	}
	return c.get(cmd.Context(), "datasources/"+url.PathEscape(uid)+"/"+path, query)
}

func (a *app) datasourceSchemaCommand() *cobra.Command {
	var table string
	var offset int
	cmd := &cobra.Command{
		Use:   "schema <datasource-uid>",
		Short: "List the tables, or the columns of one table, of a ClickHouse, Elasticsearch or OpenSearch datasource",
		Long:  "schema lists the tables of a datasource, or with --table the columns of one table with their types.\nA Tempo or Jaeger datasource has no tables: schema lists its services and span names, and on Tempo its attribute names.\nPrometheus and Loki datasources have none of these; labels reads their label names.\nWhen a list is cut, the notice names the --offset that reads the next page.",
		Args:  exactArgs(1, "one datasource uid"),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := url.Values{}
			if table != "" {
				query.Set("table", table)
			}
			if cmd.Flags().Changed("offset") {
				if offset < 0 {
					return usagef("--offset must be 0 or more")
				}
				query.Set("offset", strconv.Itoa(offset))
			}
			data, err := a.getDiscovery(cmd, args[0], "schema", query)
			if err != nil {
				return err
			}
			out := a.output()
			return render(out, data, func(body schemaBody) error {
				if err := printSchema(out, body); err != nil {
					return err
				}
				if body.Truncated {
					if body.NextOffset != nil {
						out.notice("More exist. Read the next page with --offset %d.", *body.NextOffset)
					} else {
						out.notice("The list was cut to fit.")
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&table, "table", "", "the table whose columns to list")
	cmd.Flags().IntVar(&offset, "offset", 0, "items to skip, from the next page notice")
	return cmd
}

func printSchema(out *output, body schemaBody) error {
	switch {
	case body.Table != "":
		var columns []schemaColumn
		if err := json.Unmarshal(body.Columns, &columns); err != nil {
			return fmt.Errorf("decode the columns: %w", err)
		}
		rows := make([][]string, 0, len(columns))
		for _, c := range columns {
			rows = append(rows, []string{text(c.Name), dash(c.Type)})
		}
		out.table([]string{"COLUMN", "TYPE"}, rows)
	case body.Datasource != "":
		var headers []string
		if err := json.Unmarshal(body.Columns, &headers); err != nil {
			return fmt.Errorf("decode the columns: %w", err)
		}
		printCells(out, headers, body.Rows)
		attributes := make([][]string, 0, len(body.Attributes))
		for _, name := range body.Attributes {
			attributes = append(attributes, []string{text(name)})
		}
		out.line("")
		out.table([]string{"ATTRIBUTE"}, attributes)
	default:
		rows := make([][]string, 0, len(body.Tables))
		for _, t := range body.Tables {
			rows = append(rows, []string{text(t.Name), dash(t.Kind)})
		}
		out.table([]string{"TABLE", "KIND"}, rows)
	}
	return nil
}

func printCells(out *output, headers []string, cells [][]any) {
	names := make([]string, len(headers))
	for i, h := range headers {
		names[i] = text(h)
	}
	rows := make([][]string, 0, len(cells))
	for _, cell := range cells {
		row := make([]string, len(cell))
		for i, v := range cell {
			if v == nil {
				row[i] = "-"
				continue
			}
			row[i] = text(fmt.Sprint(v))
		}
		rows = append(rows, row)
	}
	out.table(names, rows)
}

func (a *app) datasourceSampleCommand() *cobra.Command {
	var table string
	var limit int
	cmd := &cobra.Command{
		Use:   "sample <datasource-uid>",
		Short: "Read a few rows or log lines of a datasource to see real values",
		Long:  "sample prints up to 20 recent rows of a ClickHouse table, log lines of a Loki stream, or documents of an Elasticsearch or OpenSearch index pattern, 5 by default.\nClickHouse needs --table; on Loki --table is a stream selector such as {service_name=\"payments\"}, and leaving it out reads every stream.\nCells are cut to 300 bytes.",
		Args:  exactArgs(1, "one datasource uid"),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := url.Values{}
			if table != "" {
				query.Set("table", table)
			}
			if cmd.Flags().Changed("limit") {
				if limit < 1 || limit > maxSampleLimit {
					return usagef("--limit must be 1 to %d", maxSampleLimit)
				}
				query.Set("limit", strconv.Itoa(limit))
			}
			data, err := a.getDiscovery(cmd, args[0], "sample", query)
			if err != nil {
				return err
			}
			out := a.output()
			return render(out, data, func(body sampleBody) error {
				printCells(out, body.Columns, body.Rows)
				if body.Truncated {
					out.notice("Columns, cells or rows were cut to fit; ask for fewer rows with --limit.")
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&table, "table", "", "ClickHouse table, or a Loki stream selector")
	cmd.Flags().IntVar(&limit, "limit", 0, fmt.Sprintf("rows to return, 1 to %d, 5 when left out", maxSampleLimit))
	return cmd
}
