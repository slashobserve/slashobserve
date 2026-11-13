package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

const diffValueColumn = 60

type specChange struct {
	Op   string          `json:"op"`
	Path string          `json:"path"`
	From json.RawMessage `json:"from"`
	To   json.RawMessage `json:"to"`
}

type versionDiff struct {
	From    int          `json:"from"`
	To      int          `json:"to"`
	Changes []specChange `json:"changes"`
}

func (a *app) dashboardMoveCommand() *cobra.Command {
	var folder string
	cmd := &cobra.Command{
		Use:   "move <dashboard-id> --folder <folder-id>",
		Short: "Move a dashboard to another folder",
		Long:  "move files a dashboard in another folder without saving a version.\nYou need Edit on the dashboard and on the target folder, and a personal folder (My Dashboards) takes no moves.",
		Args:  exactArgs(1, "one dashboard id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if folder == "" {
				return usagef("--folder <folder-id> is required")
			}
			target, err := parseID(folder)
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodPatch, "dashboards/"+id, map[string]string{"folder_id": target}, func(out *output, data []byte) error {
				moved, err := decode[dashboardWithVersion](data)
				if err != nil {
					return err
				}
				out.confirm("Moved %s to folder %s", text(moved.Dashboard.Title), text(target))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&folder, "folder", "", "the folder id to move it to")
	return cmd
}

func (a *app) dashboardDuplicateCommand() *cobra.Command {
	var title, folder string
	cmd := &cobra.Command{
		Use:   "duplicate <dashboard-id> --title <title> [--folder <folder-id>]",
		Short: "Save a copy of a dashboard as a new dashboard",
		Long:  "duplicate saves the latest version as version 1 of a new dashboard with the title you give.\nThe copy goes to --folder, by default General for a key.\nThe copy's queries are not run, and the versions and comments stay with the original.",
		Args:  exactArgs(1, "one dashboard id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if title == "" {
				return usagef("--title <title> is required")
			}
			body := map[string]string{"title": title}
			if folder != "" {
				target, err := parseID(folder)
				if err != nil {
					return err
				}
				body["folder_id"] = target
			}
			return a.observationCall(cmd, http.MethodPost, "dashboards/"+id+"/duplicate", body, func(out *output, data []byte) error {
				copied, err := decode[dashboardWithVersion](data)
				if err != nil {
					return err
				}
				out.confirm("Duplicated as %s", text(copied.Dashboard.ID))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "the copy's title")
	cmd.Flags().StringVar(&folder, "folder", "", "the folder id to create the copy in")
	return cmd
}

func (a *app) dashboardRestoreCommand() *cobra.Command {
	var version, parent int
	var summary string
	cmd := &cobra.Command{
		Use:   "restore <dashboard-id> --version <n> [--parent <n>] [--summary <text>]",
		Short: "Save an earlier version as a new version",
		Long:  "restore saves a copy of an earlier version as the newest version, without running its queries.\n--parent is the version you based the restore on, by default the latest version now; a dashboard saved since that version answers version_conflict.",
		Args:  exactArgs(1, "one dashboard id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if version < 1 {
				return usagef("--version <n> is required, a version from observe dashboards versions")
			}
			if cmd.Flags().Changed("parent") && parent < 1 {
				return usagef("--parent must be 1 or higher")
			}
			if parent == 0 {
				if parent, err = a.latestVersion(cmd, id); err != nil {
					return err
				}
			}
			body := map[string]any{"parent_version": parent}
			if summary != "" {
				body["summary"] = summary
			}
			return a.observationCall(cmd, http.MethodPost, "dashboards/"+id+"/versions/"+strconv.Itoa(version)+"/restore", body, func(out *output, data []byte) error {
				restored, err := decode[dashboardWithVersion](data)
				if err != nil {
					return err
				}
				out.confirm("Restored version %d as version %d", version, restored.Version.Version)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&version, "version", 0, "the version to restore")
	cmd.Flags().IntVar(&parent, "parent", 0, "the version this restore builds on, by default the latest")
	cmd.Flags().StringVar(&summary, "summary", "", "the new version's summary")
	return cmd
}

func (a *app) latestVersion(cmd *cobra.Command, id string) (int, error) {
	c, err := a.newClient()
	if err != nil {
		return 0, err
	}
	data, err := c.get(cmd.Context(), "dashboards/"+id, nil)
	if err != nil {
		return 0, err
	}
	latest, err := decode[dashboardWithVersion](data)
	if err != nil {
		return 0, err
	}
	return latest.Version.Version, nil
}

func (a *app) dashboardDiffCommand() *cobra.Command {
	var from, to int
	cmd := &cobra.Command{
		Use:   "diff <dashboard-id> [--from <n>] [--to <n>]",
		Short: "List the changes between two versions of a dashboard",
		Long:  "diff lists the structural changes from one version to another as JSON pointers into the Spec.\n--to is by default the latest version, and --from the version before --to; --from 0 diffs against an empty document.",
		Args:  exactArgs(1, "one dashboard id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			query := url.Values{}
			if cmd.Flags().Changed("from") {
				if from < 0 {
					return usagef("--from must be 0 or higher")
				}
				query.Set("from", strconv.Itoa(from))
			}
			if cmd.Flags().Changed("to") {
				if to < 1 {
					return usagef("--to must be 1 or higher")
				}
				query.Set("to", strconv.Itoa(to))
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			data, err := c.get(cmd.Context(), "dashboards/"+id+"/diff", query)
			if err != nil {
				return err
			}
			out := a.output()
			if a.json {
				out.body(data)
				return nil
			}
			diff, err := decode[versionDiff](data)
			if err != nil {
				return err
			}
			if len(diff.Changes) == 0 {
				out.line("No changes between version %d and %d", diff.From, diff.To)
				return nil
			}
			rows := make([][]string, 0, len(diff.Changes))
			for _, change := range diff.Changes {
				rows = append(rows, []string{text(change.Op), text(change.Path), diffValue(change.From), diffValue(change.To)})
			}
			out.table([]string{"OP", "PATH", "FROM", "TO"}, rows)
			return nil
		},
	}
	cmd.Flags().IntVar(&from, "from", 0, "the older version, by default the one before --to")
	cmd.Flags().IntVar(&to, "to", 0, "the newer version, by default the latest")
	return cmd
}

func diffValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "-"
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		return truncate(text(string(raw)), diffValueColumn)
	}
	return truncate(text(compact.String()), diffValueColumn)
}
