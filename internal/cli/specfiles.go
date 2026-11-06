package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/specfiles"
)

const (
	applyTimeout       = 120 * time.Second
	exportPageSize     = 200
	fallbackSlug       = "dashboard"
	outcomeFailed      = "failed"
	codeInvalidJSON    = "invalid_json"
	codeFileUnreadable = "file_unreadable"
)

type folderRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	General  bool   `json:"general"`
	Personal bool   `json:"personal"`
}

type folderRows struct {
	Folders []folderRow `json:"folders"`
}

type appliedRows struct {
	Applied []struct {
		Path        string `json:"path"`
		DashboardID string `json:"dashboard_id"`
	} `json:"applied"`
}

type exportItem struct {
	ID    string
	Title string
}

type exported struct {
	Path        string `json:"path"`
	DashboardID string `json:"dashboard_id"`
	Title       string `json:"title"`
}

type applied struct {
	Path        string `json:"path"`
	Outcome     string `json:"outcome"`
	DashboardID string `json:"dashboard_id,omitempty"`
	Version     int    `json:"version,omitempty"`
	Code        string `json:"code,omitempty"`
	Message     string `json:"message,omitempty"`
}

type applyRequest struct {
	Path string          `json:"path"`
	Spec json.RawMessage `json:"spec"`
}

func exportNames(items []exportItem, bound map[string]string) map[string]string {
	sorted := slices.Clone(items)
	slices.SortFunc(sorted, func(a, b exportItem) int { return strings.Compare(a.ID, b.ID) })
	names := make(map[string]string, len(items))
	taken := map[string]bool{}
	claim := func(id, base string) {
		name := specfiles.FileName(base)
		for n := 2; taken[strings.ToLower(name)]; n++ {
			name = specfiles.FileName(base + "-" + strconv.Itoa(n))
		}
		taken[strings.ToLower(name)] = true
		names[id] = name
	}
	for _, it := range sorted {
		if name, ok := bound[it.ID]; ok {
			claim(it.ID, strings.TrimSuffix(name, specfiles.Ext))
		}
	}
	for _, it := range sorted {
		if _, ok := names[it.ID]; ok {
			continue
		}
		slug := links.Slug(it.Title)
		if slug == "" {
			slug = fallbackSlug
		}
		claim(it.ID, slug)
	}
	return names
}

func boundNames(rows appliedRows) map[string]string {
	bound := make(map[string]string, len(rows.Applied))
	for _, r := range rows.Applied {
		base := path.Base(r.Path)
		if strings.HasSuffix(base, specfiles.Ext) && base != specfiles.Ext {
			bound[r.DashboardID] = base
		}
	}
	return bound
}

func pickFolder(folders []folderRow, arg string) (folderRow, error) {
	for _, f := range folders {
		if f.ID == arg {
			return f, nil
		}
	}
	var matches []folderRow
	for _, f := range folders {
		if strings.EqualFold(f.Title, arg) {
			matches = append(matches, f)
		}
	}
	switch len(matches) {
	case 0:
		return folderRow{}, usagef("no folder you can see has the id or title %q", text(arg))
	case 1:
		return matches[0], nil
	}
	return folderRow{}, usagef("more than one folder is titled %q; pass the folder id", text(arg))
}

func (a *app) exportCommand() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "export <folder-id-or-title>",
		Short: "Write a folder's dashboards as ObserveSpec files",
		Long: "export writes the latest version of every dashboard in a folder to <dir>/<folder>/<name>.json, ready to commit to Git and send back with apply.\n" +
			"The folder directory is the folder title with % written as %25 and / as %2F. A dashboard applied before keeps the file name it was applied from; any other is named after its title.",
		Example: "  observe dashboards export Payments --dir dashboards",
		Args:    exactArgs(1, "one folder id or title"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runExport(cmd, args[0], dir)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "the directory to write the folder's directory into")
	return cmd
}

func (a *app) runExport(cmd *cobra.Command, arg, dir string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	data, err := c.get(ctx, "folders", nil)
	if err != nil {
		return err
	}
	list, err := decode[folderRows](data)
	if err != nil {
		return err
	}
	folder, err := pickFolder(list.Folders, arg)
	if err != nil {
		return err
	}
	items, err := folderDashboards(cmd, c, folder.ID)
	if err != nil {
		return err
	}
	data, err = c.get(ctx, "provision/applied", url.Values{"folder_id": {folder.ID}})
	if err != nil {
		return err
	}
	bindings, err := decode[appliedRows](data)
	if err != nil {
		return err
	}
	names := exportNames(items, boundNames(bindings))
	folderDir := specfiles.DirName(folder.Title)
	if err := os.MkdirAll(filepath.Join(dir, folderDir), 0o750); err != nil {
		return fmt.Errorf("create the folder directory: %w", err)
	}
	written := make([]exported, 0, len(items))
	for _, it := range items {
		rel := folderDir + "/" + names[it.ID]
		if err := exportOne(cmd, c, it.ID, filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return err
		}
		written = append(written, exported{Path: rel, DashboardID: it.ID, Title: it.Title})
	}
	out := a.output()
	if a.json {
		raw, err := json.Marshal(written)
		if err != nil {
			return err
		}
		out.body(raw)
		return nil
	}
	rows := make([][]string, 0, len(written))
	for _, w := range written {
		rows = append(rows, []string{text(w.Path), text(w.DashboardID), text(w.Title)})
	}
	out.table([]string{"FILE", "DASHBOARD", "TITLE"}, rows)
	return nil
}

func folderDashboards(cmd *cobra.Command, c *client, folderID string) ([]exportItem, error) {
	var items []exportItem
	query := url.Values{"folder": {folderID}, "limit": {strconv.Itoa(exportPageSize)}}
	for {
		data, err := c.get(cmd.Context(), "dashboards", query)
		if err != nil {
			return nil, err
		}
		page, err := decode[dashboardList](data)
		if err != nil {
			return nil, err
		}
		for _, d := range page.Dashboards {
			items = append(items, exportItem{ID: d.ID, Title: d.Title})
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			return items, nil
		}
		query.Set("cursor", *page.NextCursor)
	}
}

func exportOne(cmd *cobra.Command, c *client, id, file string) error {
	if _, err := parseID(id); err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "dashboards/"+id, nil)
	if err != nil {
		return err
	}
	shown, err := decode[struct {
		Version struct {
			Spec json.RawMessage `json:"spec"`
		} `json:"version"`
	}](data)
	if err != nil {
		return err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, shown.Version.Spec, "", "  "); err != nil {
		return errors.New("the server sent a Spec the CLI cannot read")
	}
	pretty.WriteByte('\n')
	if err := os.WriteFile(file, pretty.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}
	return nil
}

func (a *app) applyCommand() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Save every ObserveSpec file of a directory as dashboards",
		Long: "apply sends every .json file under --dir to the server, one at a time. A file directly in the directory goes to General, and a file in a subdirectory to the folder of that name, with %25 read as % and %2F as /.\n" +
			"A file applied before saves into the same dashboard; otherwise the one dashboard in the folder with the same title is adopted, and when there is none a new one is created. A version is saved only when the file changed. Apply never deletes a dashboard.\n" +
			"It exits 1 when any file failed.",
		Example: "  observe dashboards apply --dir dashboards",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runApply(cmd, dir)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "the directory to apply")
	return cmd
}

func (a *app) runApply(cmd *cobra.Command, dir string) error {
	files, err := specfiles.Scan(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	c = c.withTimeout(applyTimeout)
	results := make([]applied, 0, len(files))
	failed := 0
	for _, f := range files {
		res, err := applyOne(cmd, c, dir, f)
		if err != nil {
			return err
		}
		if res.Outcome == outcomeFailed {
			failed++
		}
		results = append(results, res)
	}
	out := a.output()
	if a.json {
		raw, err := json.Marshal(results)
		if err != nil {
			return err
		}
		out.body(raw)
	} else {
		rows := make([][]string, 0, len(results))
		for _, r := range results {
			version := ""
			if r.Version > 0 {
				version = strconv.Itoa(r.Version)
			}
			note := r.Message
			if r.Code != "" {
				note = r.Message + " (" + r.Code + ")"
			}
			rows = append(rows, []string{text(r.Path), r.Outcome, text(r.DashboardID), version, text(note)})
		}
		out.table([]string{"FILE", "OUTCOME", "DASHBOARD", "VERSION", "MESSAGE"}, rows)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d files failed", failed, len(files))
	}
	return nil
}

func applyOne(cmd *cobra.Command, c *client, dir string, f specfiles.File) (applied, error) {
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Path))) //nolint:gosec // reading the files under the directory the user named is the command's purpose
	if err != nil {
		return applied{Path: f.Path, Outcome: outcomeFailed, Code: codeFileUnreadable, Message: "the file cannot be read"}, nil
	}
	if !json.Valid(raw) {
		return applied{Path: f.Path, Outcome: outcomeFailed, Code: codeInvalidJSON, Message: "the file is not JSON"}, nil
	}
	data, err := c.post(cmd.Context(), "provision/apply", applyRequest{Path: f.Path, Spec: raw})
	var api *apiError
	if errors.As(err, &api) && api.status < 500 && api.code != "" {
		return applied{Path: f.Path, Outcome: outcomeFailed, Code: api.code, Message: api.message}, nil
	}
	if err != nil {
		return applied{}, err
	}
	res, err := decode[applied](data)
	if err != nil {
		return applied{}, err
	}
	return res, nil
}
