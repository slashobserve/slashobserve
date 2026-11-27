package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

const maxPatchFileBytes = 4 << 20

type panelPatchFile struct {
	Upsert    []json.RawMessage `json:"upsert,omitempty"`
	Delete    []string          `json:"delete,omitempty"`
	Layout    []json.RawMessage `json:"layout,omitempty"`
	Variables json.RawMessage   `json:"variables,omitempty"`
	Dashboard json.RawMessage   `json:"dashboard,omitempty"`
}

type panelPatchBody struct {
	panelPatchFile
	Summary       string `json:"summary"`
	ParentVersion int    `json:"parent_version"`
}

func (a *app) dashboardPanelsCommand() *cobra.Command {
	var file, summary string
	var remove []string
	var parent int
	cmd := &cobra.Command{
		Use:   "panels <dashboard-id> --summary <text> [--file <patch.json>] [--remove <panel-id>]...",
		Short: "Add, replace, remove or move panels as a new version",
		Long: "panels saves a new version that changes only the panels you name, and runs only their queries.\n" +
			"--file holds a JSON object with upsert (complete panels matched by id; a new id adds or duplicates a panel), delete (panel ids), layout (moves as {\"id\",\"gridPos\",\"row\"}), variables and dashboard, the same fields as the update_panels MCP tool.\n" +
			"--remove adds a panel id to delete, and can be repeated.\n" +
			"--parent is the version you based the change on, by default the latest version now; a change that overlaps a newer version answers version_conflict.",
		Example: "  observe dashboards panels <id> --remove p_errors --summary \"Remove the errors panel\"\n" +
			"  observe dashboards panels <id> --file patch.json --summary \"Add a latency panel\"",
		Args: exactArgs(1, "one dashboard id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if summary == "" {
				return usagef("--summary <text> is required")
			}
			patch, err := readPanelPatch(file)
			if err != nil {
				return err
			}
			patch.Delete = append(patch.Delete, remove...)
			if len(patch.Upsert) == 0 && len(patch.Delete) == 0 && len(patch.Layout) == 0 && len(patch.Variables) == 0 && len(patch.Dashboard) == 0 {
				return usagef("nothing to change: pass --file with a patch or --remove <panel-id>")
			}
			if cmd.Flags().Changed("parent") && parent < 1 {
				return usagef("--parent must be 1 or higher")
			}
			if parent == 0 {
				if parent, err = a.latestVersion(cmd, id); err != nil {
					return err
				}
			}
			body := panelPatchBody{panelPatchFile: patch, Summary: summary, ParentVersion: parent}
			return a.observationCall(cmd, http.MethodPost, "dashboards/"+id+"/panels", body, func(out *output, data []byte) error {
				saved, err := decode[dashboardWithVersion](data)
				if err != nil {
					return err
				}
				out.confirm("Saved version %d of %s", saved.Version.Version, text(saved.Dashboard.Title))
				out.warnOfEdit(saved.Warning)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "a JSON file with the patch")
	cmd.Flags().StringArrayVar(&remove, "remove", nil, "a panel id to remove; repeat for more")
	cmd.Flags().StringVar(&summary, "summary", "", "the new version's summary")
	cmd.Flags().IntVar(&parent, "parent", 0, "the version this change builds on, by default the latest")
	return cmd
}

func readPanelPatch(path string) (panelPatchFile, error) {
	var patch panelPatchFile
	if path == "" {
		return patch, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return patch, usagef("cannot read %s: %v", text(path), err)
	}
	if info.Size() > maxPatchFileBytes {
		return patch, usagef("%s is larger than %d MiB", path, maxPatchFileBytes>>20)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // reading the file the user named is the command's purpose
	if err != nil {
		return patch, usagef("cannot read %s: %v", text(path), err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&patch); err != nil {
		return patch, usagef("%s is not a JSON patch object: %v", path, err)
	}
	return patch, nil
}
