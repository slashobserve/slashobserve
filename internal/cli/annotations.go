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
		Short: "List, edit and delete annotations",
		Long:  "annotations lists, edits and deletes the markers charts draw, such as deploys.\nobserve annotate adds one.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.annotationsListCommand(), a.annotationsUpdateCommand(), a.annotationsDeleteCommand())
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
			if err := page.check(); err != nil {
				return err
			}
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
			data, err := c.getPages(cmd.Context(), "annotations", query, page.all)
			if err != nil {
				return err
			}
			out := a.output()
			return render(out, data, func(listed annotationListBody) error {
				rows := make([][]string, 0, len(listed.Annotations))
				for _, n := range listed.Annotations {
					rows = append(rows, []string{text(n.ID), when(n.TimeFrom), orDash(n.DashboardID), tagsText(n.Tags), truncate(text(n.Text), annotationTextColumn)})
				}
				out.table([]string{"ID", "TIME", "DASHBOARD", "TAGS", "TEXT"}, rows)
				pageFooter(out, len(listed.Annotations), nil, listed.NextCursor)
				return nil
			})
		},
	}
	addPageFlags(cmd, &page, maxAnnotationPage)
	cmd.Flags().StringVar(&dashboard, "dashboard", "", "the dashboard id; without it only global annotations")
	cmd.Flags().StringVar(&from, "from", "", "start of the window, an RFC 3339 time")
	cmd.Flags().StringVar(&to, "to", "", "end of the window, an RFC 3339 time")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "only annotations with this tag; repeat it for any of several")
	return cmd
}

func (a *app) annotationsUpdateCommand() *cobra.Command {
	var newText, tags string
	cmd := &cobra.Command{
		Use:   "update <annotation-id> [--text <text>] [--tags <tags>]",
		Short: "Change the text or tags of an annotation",
		Long:  "update changes the text, the tags or both of an annotation you added; an Admin may change any.\nIts time, dashboard and author stay as they were.\n--tags replaces the tags, and --tags \"\" removes them all.",
		Args:  exactArgs(1, "one annotation id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			body := map[string]any{}
			if cmd.Flags().Changed("text") {
				body["text"] = newText
			}
			if cmd.Flags().Changed("tags") {
				body["tags"] = splitList(tags)
			}
			if len(body) == 0 {
				return usagef("give --text, --tags or both")
			}
			return a.observationCall(cmd, http.MethodPatch, "annotations/"+id, body, func(out *output, _ []byte) error {
				out.confirm("Updated annotation %s", text(id))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&newText, "text", "", "the new text")
	cmd.Flags().StringVar(&tags, "tags", "", "the new comma-separated tags, replacing the old ones")
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
