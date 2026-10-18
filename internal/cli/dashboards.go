package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const defaultPageSize = 50

type author struct {
	Name string `json:"name"`
}

type dashboardSummary struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	LatestVersion int    `json:"latest_version"`
	UpdatedAt     string `json:"updated_at"`
	UpdatedBy     author `json:"updated_by"`
}

type versionMeta struct {
	Version       int    `json:"version"`
	ParentVersion *int   `json:"parent_version"`
	Summary       string `json:"summary"`
	Author        author `json:"author"`
	CreatedAt     string `json:"created_at"`
}

type versionBody struct {
	versionMeta
	Spec spec.Dashboard `json:"spec"`
}

type pageOptions struct {
	limit  int
	cursor string
}

type showOptions struct {
	version int
}

type dashboardList struct {
	Dashboards []dashboardSummary `json:"dashboards"`
	NextCursor *string            `json:"next_cursor"`
}

type dashboardWithVersion struct {
	Dashboard dashboardSummary `json:"dashboard"`
	Version   versionBody      `json:"version"`
}

type versionList struct {
	Versions   []versionMeta `json:"versions"`
	NextCursor *string       `json:"next_cursor"`
}

func (a *app) dashboardsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboards",
		Short: "List and inspect dashboards",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usagef("unknown command %q for %s", text(args[0]), cmd.CommandPath())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.listCommand(), a.showCommand(), a.versionsCommand())
	return cmd
}

func addPageFlags(cmd *cobra.Command, opts *pageOptions) {
	cmd.Flags().IntVar(&opts.limit, "limit", defaultPageSize, "rows per page")
	cmd.Flags().StringVar(&opts.cursor, "cursor", "", "continue from a next_cursor")
}

func (o pageOptions) query() url.Values {
	query := url.Values{"limit": {strconv.Itoa(o.limit)}}
	if o.cursor != "" {
		query.Set("cursor", o.cursor)
	}
	return query
}

func (a *app) listCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List dashboards, newest update first",
		Args:  noArgs,
		RunE:  a.runList,
	}
	addPageFlags(cmd, &a.list)
	return cmd
}

func (a *app) runList(cmd *cobra.Command, _ []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "dashboards", a.list.query())
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	page, err := decode[dashboardList](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(page.Dashboards))
	for _, d := range page.Dashboards {
		rows = append(rows, []string{text(d.ID), text(d.Title), strconv.Itoa(d.LatestVersion), when(d.UpdatedAt), text(d.UpdatedBy.Name)})
	}
	out.table([]string{"ID", "TITLE", "VERSION", "UPDATED", "UPDATED BY"}, rows)
	nextPageHint(out, page.NextCursor)
	return nil
}

func nextPageHint(out *output, cursor *string) {
	if cursor != nil && *cursor != "" {
		out.notice("more results: --cursor %s", text(*cursor))
	}
}

func (a *app) showCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <dashboard-id>",
		Short: "Show a dashboard with its latest or a given version",
		Args:  exactArgs(1, "one dashboard id"),
		RunE:  a.runShow,
	}
	cmd.Flags().IntVar(&a.show.version, "version", 0, "show this version instead of the latest")
	return cmd
}

func (a *app) runShow(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	path := "dashboards/" + id
	if cmd.Flags().Changed("version") {
		if a.show.version < 1 {
			return usagef("--version must be 1 or higher")
		}
		path += "/versions/" + strconv.Itoa(a.show.version)
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), path, nil)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	shown, err := decode[dashboardWithVersion](data)
	if err != nil {
		return err
	}
	out.fields([][2]string{
		{"Title", text(shown.Dashboard.Title)},
		{"ID", text(shown.Dashboard.ID)},
		{"URL", c.base.JoinPath("dashboards", shown.Dashboard.ID).String()},
		{"Version", strconv.Itoa(shown.Version.Version)},
		{"Author", text(shown.Version.Author.Name)},
		{"Summary", text(shown.Version.Summary)},
	})
	entries := shown.Version.Spec.AllPanels()
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{text(e.Panel.ID), text(string(e.Panel.Type)), text(panelDatasource(e.Panel)), text(e.Panel.Title)})
	}
	out.line("")
	out.table([]string{"PANEL", "TYPE", "DATASOURCE", "TITLE"}, rows)
	return nil
}

func (a *app) versionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "versions <dashboard-id>",
		Short: "List the versions of a dashboard, newest first",
		Args:  exactArgs(1, "one dashboard id"),
		RunE:  a.runVersions,
	}
	addPageFlags(cmd, &a.versions)
	return cmd
}

func (a *app) runVersions(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "dashboards/"+id+"/versions", a.versions.query())
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	page, err := decode[versionList](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(page.Versions))
	for _, v := range page.Versions {
		parent := "-"
		if v.ParentVersion != nil {
			parent = strconv.Itoa(*v.ParentVersion)
		}
		rows = append(rows, []string{strconv.Itoa(v.Version), parent, text(v.Author.Name), when(v.CreatedAt), text(v.Summary)})
	}
	out.table([]string{"VERSION", "PARENT", "AUTHOR", "CREATED", "SUMMARY"}, rows)
	nextPageHint(out, page.NextCursor)
	return nil
}

func panelDatasource(p *spec.Panel) string {
	if len(p.Queries) > 0 {
		return p.QueryDatasource(0)
	}
	return p.Datasource
}
