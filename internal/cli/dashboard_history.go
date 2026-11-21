package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

type revisionRow struct {
	ID          string  `json:"id"`
	Intent      string  `json:"intent"`
	Prompt      string  `json:"prompt"`
	State       string  `json:"state"`
	BaseVersion *int    `json:"base_version"`
	ResultVer   *int    `json:"result_version"`
	Error       *string `json:"error"`
	CreatedAt   string  `json:"created_at"`
}

type revisionListBody struct {
	Revisions  []revisionRow `json:"revisions"`
	NextCursor *string       `json:"next_cursor"`
}

type tagListBody struct {
	Tags []struct {
		Tag        string `json:"tag"`
		Dashboards int    `json:"dashboards"`
	} `json:"tags"`
}

func (a *app) dashboardRevisionsCommand() *cobra.Command {
	var page pageOptions
	cmd := &cobra.Command{
		Use:   "revisions <dashboard-id> [<revision-id>]",
		Short: "List the agent requests that changed a dashboard, or show one",
		Long:  "revisions lists the agent requests of a dashboard, newest first, including the request that created it, with their state, the version they were based on and the version they saved.\nWith a revision id it shows that one request in full.",
		Args:  rangeArgs(1, 2, "a dashboard id and an optional revision id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			out := a.output()
			if len(args) == 2 {
				revision, err := parseID(args[1])
				if err != nil {
					return err
				}
				data, err := c.get(cmd.Context(), "dashboards/"+id+"/revisions/"+revision, nil)
				if err != nil {
					return err
				}
				if a.json {
					out.body(data)
					return nil
				}
				shown, err := decode[struct {
					Revision revisionRow `json:"revision"`
				}](data)
				if err != nil {
					return err
				}
				printRevision(out, shown.Revision)
				return nil
			}
			if err := page.check(); err != nil {
				return err
			}
			data, err := c.getPages(cmd.Context(), "dashboards/"+id+"/revisions", page.query(), page.all)
			if err != nil {
				return err
			}
			if a.json {
				out.body(data)
				return nil
			}
			list, err := decode[revisionListBody](data)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(list.Revisions))
			for _, r := range list.Revisions {
				rows = append(rows, []string{text(r.ID), text(r.State), text(r.Intent), versionText(r.BaseVersion), versionText(r.ResultVer), firstLine(r.Prompt)})
			}
			out.table([]string{"ID", "STATE", "INTENT", "BASE", "SAVED", "PROMPT"}, rows)
			pageFooter(out, len(list.Revisions), nil, list.NextCursor)
			return nil
		},
	}
	addPageFlags(cmd, &page, maxDashboardPage)
	return cmd
}

func versionText(v *int) string {
	if v == nil {
		return "-"
	}
	return strconv.Itoa(*v)
}

func printRevision(out *output, r revisionRow) {
	out.fields([][2]string{
		{"ID", text(r.ID)},
		{"State", text(r.State)},
		{"Intent", text(r.Intent)},
		{"Base version", versionText(r.BaseVersion)},
		{"Saved version", versionText(r.ResultVer)},
		{"Created", when(r.CreatedAt)},
		{"Error", orDash(r.Error)},
		{"Prompt", text(r.Prompt)},
	})
}

type viewerListBody struct {
	Viewers []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"viewers"`
}

func (a *app) dashboardViewersCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "viewers <dashboard-id>",
		Short: "List the people who have a dashboard open now",
		Long:  "viewers lists, by name, the other people whose open dashboard page reported in during the last 45 seconds.\nYou need to be able to view the dashboard, and the list is empty when nobody is looking.",
		Args:  exactArgs(1, "a dashboard id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			data, err := c.get(cmd.Context(), "dashboards/"+id+"/viewers", nil)
			if err != nil {
				return err
			}
			out := a.output()
			if a.json {
				out.body(data)
				return nil
			}
			list, err := decode[viewerListBody](data)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(list.Viewers))
			for _, v := range list.Viewers {
				rows = append(rows, []string{text(v.Name)})
			}
			out.table([]string{"NAME"}, rows)
			return nil
		},
	}
}

func (a *app) dashboardTagsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tags",
		Short: "List every dashboard tag with the number of dashboards that carry it",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := a.newClient()
			if err != nil {
				return err
			}
			query := url.Values{}
			if a.excludeAudit {
				query.Set("exclude_audit", "true")
			}
			data, err := c.get(cmd.Context(), "dashboards/tags", query)
			if err != nil {
				return err
			}
			out := a.output()
			if a.json {
				out.body(data)
				return nil
			}
			list, err := decode[tagListBody](data)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(list.Tags))
			for _, t := range list.Tags {
				rows = append(rows, []string{text(t.Tag), strconv.Itoa(t.Dashboards)})
			}
			out.table([]string{"TAG", "DASHBOARDS"}, rows)
			return nil
		},
	}
	cmd.Flags().BoolVar(&a.excludeAudit, "exclude-audit", false, "leave out the dashboards in the built-in Audit folder")
	return cmd
}
