package cli

import (
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

type listedAnnotation struct {
	ID          string   `json:"id"`
	DashboardID *string  `json:"dashboard_id"`
	TimeFrom    string   `json:"time_from"`
	Text        string   `json:"text"`
	Tags        []string `json:"tags"`
}

type annotationListBody struct {
	Annotations []listedAnnotation `json:"annotations"`
	NextCursor  *string            `json:"next_cursor"`
}

const annotationTextColumn = 60

func (a *app) annotationsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "annotations",
		Short: "List and delete annotations",
		Long:  "annotations lists and deletes the markers charts draw, such as deploys.\nobserve annotate adds one.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.annotationsListCommand(), a.annotationsDeleteCommand())
	return cmd
}

func (a *app) annotationsListCommand() *cobra.Command {
	var dashboard, from, to string
	var tags []string
	var page pageOptions
	cmd := &cobra.Command{
		Use:   "list [--dashboard <id>] [--from <time>] [--to <time>] [--tag <tag>]...",
		Short: "List annotations, newest first",
		Long:  "list prints the annotations of a window, newest first.\nWith --dashboard it lists that dashboard's annotations and the global ones; without it, only the global ones.\n--from and --to are RFC 3339 times.",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := page.query()
			if dashboard != "" {
				id, err := parseID(dashboard)
				if err != nil {
					return err
				}
				query.Set("dashboard_id", id)
			}
			if from != "" {
				query.Set("from", from)
			}
			if to != "" {
				query.Set("to", to)
			}
			if len(tags) > 0 {
				query.Set("tags", strings.Join(tags, ","))
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			data, err := c.get(cmd.Context(), "annotations", query)
			if err != nil {
				return err
			}
			out := a.output()
			if a.json {
				out.body(data)
				return nil
			}
			listed, err := decode[annotationListBody](data)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(listed.Annotations))
			for _, n := range listed.Annotations {
				rows = append(rows, []string{text(n.ID), when(n.TimeFrom), orDash(n.DashboardID), tagsText(n.Tags), truncate(text(n.Text), annotationTextColumn)})
			}
			out.table([]string{"ID", "TIME", "DASHBOARD", "TAGS", "TEXT"}, rows)
			nextPageHint(out, listed.NextCursor)
			return nil
		},
	}
	addPageFlags(cmd, &page)
	cmd.Flags().StringVar(&dashboard, "dashboard", "", "the dashboard id; without it only global annotations")
	cmd.Flags().StringVar(&from, "from", "", "start of the window, an RFC 3339 time")
	cmd.Flags().StringVar(&to, "to", "", "end of the window, an RFC 3339 time")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "only annotations with this tag; repeat it for any of several")
	return cmd
}

func (a *app) annotationsDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <annotation-id>",
		Short: "Delete an annotation",
		Args:  exactArgs(1, "one annotation id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodDelete, "annotations/"+id, nil, func(out *output, _ []byte) error {
				out.confirm("Deleted annotation %s", text(id))
				return nil
			})
		},
	}
}

func tagsText(tags []string) string {
	if len(tags) == 0 {
		return "-"
	}
	return text(strings.Join(tags, ","))
}
