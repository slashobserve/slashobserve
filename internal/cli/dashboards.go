package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slashobserve/slashobserve/internal/links"
)

type author struct {
	Name string `json:"name"`
}

type dashboardSummary struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	LatestVersion int    `json:"latest_version"`
	UpdatedAt     string `json:"updated_at"`
	UpdatedBy     author `json:"updated_by"`
	Folder        struct {
		Title string `json:"title"`
	} `json:"folder"`
	ViewCount int    `json:"view_count"`
	ManagedBy string `json:"managed_by"`
}

func (d dashboardSummary) managedByFields() [][2]string {
	if d.ManagedBy == "" {
		return nil
	}
	return [][2]string{{"Managed by", text(d.ManagedBy)}}
}

type dashboardViews struct {
	ViewCount  int `json:"view_count"`
	WindowDays int `json:"window_days"`
}

var dashboardOrders = []string{"updated", "views", "name"}

type actorLabel struct {
	Label string `json:"label"`
}

type versionMeta struct {
	Version       int        `json:"version"`
	ParentVersion *int       `json:"parent_version"`
	Summary       string     `json:"summary"`
	Actor         actorLabel `json:"actor"`
	CreatedAt     string     `json:"created_at"`
}

type versionBody struct {
	versionMeta
	Spec shownSpec `json:"spec"`
}

type shownSpec struct {
	Panels []shownPanel `json:"panels"`
}

type shownPanel struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Title      string          `json:"title"`
	Datasource json.RawMessage `json:"datasource"`
	Queries    []struct {
		Datasource json.RawMessage `json:"datasource"`
	} `json:"queries"`
	Panels []shownPanel `json:"panels"`
}

type showOptions struct {
	version int
}

type dashboardList struct {
	Dashboards []dashboardSummary `json:"dashboards"`
	NextCursor *string            `json:"next_cursor"`
	TotalCount *int               `json:"total_count"`
}

type dashboardWithVersion struct {
	Dashboard dashboardSummary `json:"dashboard"`
	Version   versionBody      `json:"version"`
	Warning   string           `json:"warning"`
}

type versionList struct {
	Versions   []versionMeta `json:"versions"`
	NextCursor *string       `json:"next_cursor"`
}

func (a *app) dashboardsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboards",
		Short: "List, inspect, delete, export and apply dashboards",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.listCommand(), a.showCommand(), a.versionsCommand(), a.deleteCommand(), a.exportCommand(), a.applyCommand(), a.dashboardMoveCommand(), a.dashboardDuplicateCommand(), a.dashboardRestoreCommand(), a.dashboardDiffCommand(), a.dashboardRevisionsCommand(), a.dashboardTagsCommand(), a.dashboardPanelsCommand(), a.dashboardViewersCommand(), a.schemaCommand(), a.sampleCommand())
	return cmd
}

func (a *app) listCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List dashboards, newest update first",
		Args:  noArgs,
		RunE:  a.runList,
	}
	addPageFlags(cmd, &a.list, maxDashboardPage)
	cmd.Flags().StringVar(&a.listOrder, "order", "", "updated (default), views (most opened in the last 30 days first) or name (by title)")
	cmd.Flags().BoolVar(&a.excludeAudit, "exclude-audit", false, "leave out the dashboards in the built-in Audit folder")
	return cmd
}

func (a *app) runList(cmd *cobra.Command, _ []string) error {
	if err := a.list.check(); err != nil {
		return err
	}
	if a.listOrder != "" && !slices.Contains(dashboardOrders, a.listOrder) {
		return usagef("--order must be one of %s, not %q", strings.Join(dashboardOrders, ", "), text(a.listOrder))
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	query := a.list.query()
	if a.listOrder != "" {
		query.Set("order", a.listOrder)
	}
	if a.excludeAudit {
		query.Set("exclude_audit", "true")
	}
	data, err := c.getPages(cmd.Context(), "dashboards", query, a.list.all)
	if err != nil {
		return err
	}
	out := a.output()
	return render(out, data, func(page dashboardList) error {
		rows := make([][]string, 0, len(page.Dashboards))
		for _, d := range page.Dashboards {
			rows = append(rows, []string{text(d.ID), text(d.Title), strconv.Itoa(d.LatestVersion), strconv.Itoa(d.ViewCount), when(d.UpdatedAt), text(d.UpdatedBy.Name), text(d.Folder.Title)})
		}
		out.table([]string{"ID", "TITLE", "VERSION", "VIEWS", "UPDATED", "UPDATED BY", "FOLDER"}, rows)
		pageFooter(out, len(page.Dashboards), page.TotalCount, page.NextCursor)
		return nil
	})
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
		if cmd.Flags().Changed("version") {
			return explainMissingVersion(cmd.Context(), c, id, a.show.version, err)
		}
		return err
	}
	viewData, err := c.get(cmd.Context(), "dashboards/"+id+"/views", nil)
	if err != nil {
		return err
	}
	views, err := decode[dashboardViews](viewData)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		withViews, err := withViewCount(data, views)
		if err != nil {
			return err
		}
		out.body(withViews)
		return nil
	}
	shown, err := decode[dashboardWithVersion](data)
	if err != nil {
		return err
	}
	out.fields(slices.Concat([][2]string{
		{"Title", text(shown.Dashboard.Title)},
		{"ID", text(shown.Dashboard.ID)},
		{"Folder", text(shown.Dashboard.Folder.Title)},
	}, shown.Dashboard.managedByFields(), [][2]string{
		{"URL", links.Dashboard(c.base, shown.Dashboard.ID, shown.Dashboard.Title)},
		{"Version", strconv.Itoa(shown.Version.Version)},
		{"Views", fmt.Sprintf("%d in the last %d days", views.ViewCount, views.WindowDays)},
		{"Author", text(shown.Version.Actor.Label)},
		{"Summary", text(shown.Version.Summary)},
	}))
	var rows [][]string
	for _, p := range shown.Version.Spec.Panels {
		rows = appendPanelRows(rows, p)
	}
	out.line("")
	out.table([]string{"PANEL", "TYPE", "DATASOURCE", "TITLE"}, rows)
	return nil
}

func withViewCount(data []byte, views dashboardViews) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("read the dashboard: %w", err)
	}
	count, _ := json.Marshal(views.ViewCount)
	window, _ := json.Marshal(views.WindowDays)
	fields["view_count"], fields["window_days"] = count, window
	return json.Marshal(fields)
}

func explainMissingVersion(ctx context.Context, c *client, id string, version int, cause error) error {
	latest, err := latestOfReadable(ctx, c, id, cause)
	if err != nil {
		return err
	}
	return versionNotFound(id, version, latest)
}

func explainMissingDiffVersion(ctx context.Context, c *client, id string, from, to int, cause error) error {
	latest, err := latestOfReadable(ctx, c, id, cause)
	if err != nil {
		return err
	}
	for _, version := range []int{to, from} {
		if version > latest {
			return versionNotFound(id, version, latest)
		}
	}
	return cause
}

func latestOfReadable(ctx context.Context, c *client, id string, cause error) (int, error) {
	var api *apiError
	if !errors.As(cause, &api) || api.status != http.StatusNotFound {
		return 0, cause
	}
	data, err := c.get(ctx, "dashboards/"+id, nil)
	if err != nil {
		return 0, cause
	}
	latest, err := decode[dashboardWithVersion](data)
	if err != nil {
		return 0, cause
	}
	return latest.Version.Version, nil
}

func versionNotFound(id string, version, latest int) *apiError {
	return &apiError{status: http.StatusNotFound, code: "version_not_found", message: fmt.Sprintf("dashboard %s has no version %d; its latest version is %d, and observe dashboards versions %s lists them", text(id), version, latest, text(id))}
}

func (a *app) versionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "versions <dashboard-id>",
		Short: "List the versions of a dashboard, newest first",
		Args:  exactArgs(1, "one dashboard id"),
		RunE:  a.runVersions,
	}
	addPageFlags(cmd, &a.versions, maxDashboardPage)
	return cmd
}

func (a *app) runVersions(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	if err := a.versions.check(); err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.getPages(cmd.Context(), "dashboards/"+id+"/versions", a.versions.query(), a.versions.all)
	if err != nil {
		return err
	}
	out := a.output()
	return render(out, data, func(page versionList) error {
		rows := make([][]string, 0, len(page.Versions))
		for _, v := range page.Versions {
			parent := "-"
			if v.ParentVersion != nil {
				parent = strconv.Itoa(*v.ParentVersion)
			}
			rows = append(rows, []string{strconv.Itoa(v.Version), parent, text(v.Actor.Label), when(v.CreatedAt), text(v.Summary)})
		}
		out.table([]string{"VERSION", "PARENT", "AUTHOR", "CREATED", "SUMMARY"}, rows)
		pageFooter(out, len(page.Versions), nil, page.NextCursor)
		return nil
	})
}

func appendPanelRows(rows [][]string, p shownPanel) [][]string {
	rows = append(rows, []string{text(p.ID), text(p.Type), text(panelDatasource(p)), text(p.Title)})
	for _, child := range p.Panels {
		rows = appendPanelRows(rows, child)
	}
	return rows
}

func panelDatasource(p shownPanel) string {
	if len(p.Queries) > 0 {
		if uid := datasourceUID(p.Queries[0].Datasource); uid != "" {
			return uid
		}
	}
	return datasourceUID(p.Datasource)
}

func datasourceUID(raw json.RawMessage) string {
	var uid string
	if json.Unmarshal(raw, &uid) == nil {
		return uid
	}
	var legacy struct {
		UID string `json:"uid"`
	}
	if json.Unmarshal(raw, &legacy) == nil {
		return legacy.UID
	}
	return ""
}

type deleteOptions struct{ yes bool }

func (a *app) deleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <dashboard-id>",
		Short: "Delete a dashboard with every version and comment",
		Args:  exactArgs(1, "one dashboard id"),
		RunE:  a.runDelete,
	}
	cmd.Flags().BoolVar(&a.remove.yes, "yes", false, "confirm that the dashboard, its versions, comments and agent history are deleted for everyone")
	return cmd
}

func (a *app) runDelete(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	if !a.remove.yes {
		return usagef("deleting %s removes it with every version, comment and agent request for everyone and cannot be undone; rerun with --yes", id)
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	if _, err := c.delete(cmd.Context(), "dashboards/"+id); err != nil {
		return err
	}
	if !a.json {
		a.output().confirm("Deleted dashboard %s", text(id))
	}
	return nil
}
