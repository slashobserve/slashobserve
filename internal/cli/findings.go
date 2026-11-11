package cli

import (
	"net/url"

	"github.com/spf13/cobra"
)

type findingListOptions struct {
	pageOptions
	dashboard string
	folder    string
}

type findingMoveOptions struct {
	folder string
}

type findingRow struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	DashboardID *string    `json:"dashboard_id"`
	FolderID    *string    `json:"folder_id"`
	Actor       actorLabel `json:"actor"`
	CreatedAt   string     `json:"created_at"`
}

type findingListBody struct {
	Findings   []findingRow `json:"findings"`
	NextCursor *string      `json:"next_cursor"`
}

type findingMovedBody struct {
	Finding findingRow `json:"finding"`
	Folder  *struct {
		Title string `json:"title"`
	} `json:"folder"`
}

func (a *app) findingsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "findings",
		Short: "List findings and move them between folders",
		Long:  "findings lists the findings you can read and moves a finding that is not on a dashboard into another folder, which shares it with that folder's readers.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.findingsListCommand(), a.findingsMoveCommand())
	return cmd
}

func (a *app) findingsListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [--dashboard <id> | --folder <id>]",
		Short: "List findings, newest first",
		Long:  "list prints your own findings, newest first.\nWith --dashboard it prints the findings on that dashboard, and with --folder the findings filed in that folder.",
		Args:  noArgs,
		RunE:  a.runFindingsList,
	}
	addPageFlags(cmd, &a.findingList.pageOptions)
	cmd.Flags().StringVar(&a.findingList.dashboard, "dashboard", "", "only findings on this dashboard id")
	cmd.Flags().StringVar(&a.findingList.folder, "folder", "", "only findings filed in this folder id")
	return cmd
}

func (o findingListOptions) query() (url.Values, error) {
	if o.dashboard != "" && o.folder != "" {
		return nil, usagef("--dashboard and --folder cannot be used together")
	}
	query := o.pageOptions.query()
	for _, filter := range []struct{ flag, param, value string }{{"--dashboard", "dashboard_id", o.dashboard}, {"--folder", "folder_id", o.folder}} {
		if filter.value == "" {
			continue
		}
		id, err := parseID(filter.value)
		if err != nil {
			return nil, err
		}
		query.Set(filter.param, id)
	}
	return query, nil
}

func (a *app) runFindingsList(cmd *cobra.Command, _ []string) error {
	query, err := a.findingList.query()
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "findings", query)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	page, err := decode[findingListBody](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(page.Findings))
	for _, f := range page.Findings {
		rows = append(rows, []string{text(f.ID), text(f.Title), f.where(), text(f.Actor.Label), when(f.CreatedAt)})
	}
	out.table([]string{"ID", "TITLE", "WHERE", "SAVED BY", "CREATED"}, rows)
	nextPageHint(out, page.NextCursor)
	return nil
}

func (f findingRow) where() string {
	switch {
	case f.DashboardID != nil:
		return "dashboard " + text(*f.DashboardID)
	case f.FolderID != nil:
		return "folder " + text(*f.FolderID)
	}
	return "-"
}

func (a *app) findingsMoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "move <finding-id> --folder <folder-id>",
		Short: "Move a finding without a dashboard into another folder",
		Long:  "move files the finding in the folder, which shares it with that folder's readers.\nIt needs Edit on the finding and on the folder; a finding on a dashboard follows the dashboard, and My Dashboards takes no moves.",
		Args:  exactArgs(1, "one finding id"),
		RunE:  a.runFindingsMove,
	}
	cmd.Flags().StringVar(&a.findingMove.folder, "folder", "", "the folder id to move the finding into")
	return cmd
}

func (a *app) runFindingsMove(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	if a.findingMove.folder == "" {
		return usagef("--folder <folder-id> is required")
	}
	folder, err := parseID(a.findingMove.folder)
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "findings/"+id+"/move", map[string]string{"folder_id": folder})
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	moved, err := decode[findingMovedBody](data)
	if err != nil {
		return err
	}
	target := folder
	if moved.Folder != nil {
		target = moved.Folder.Title
	}
	out.confirm("Moved %s to %s", text(moved.Finding.Title), text(target))
	return nil
}
