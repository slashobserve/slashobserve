package cli

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type listedFolder struct {
	folderRow
	DashboardCount int    `json:"dashboard_count"`
	Level          string `json:"level"`
	AdminsOnly     bool   `json:"admins_only"`
	Audit          bool   `json:"audit"`
}

type listedFolders struct {
	Folders []listedFolder `json:"folders"`
}

func (a *app) foldersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "folders",
		Short: "List, create, rename and delete folders, and set who can see them",
		Long:  "folders manages the folders that hold dashboards, observations and saved searches, and who each is restricted to.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.foldersListCommand(), a.foldersCreateCommand(), a.foldersRenameCommand(), a.foldersDeleteCommand(), a.foldersPermissionsCommand())
	return cmd
}

func (a *app) foldersListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the folders you can see",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.observationCall(cmd, http.MethodGet, "folders", nil, func(out *output, data []byte) error {
				listed, err := decode[listedFolders](data)
				if err != nil {
					return err
				}
				rows := make([][]string, 0, len(listed.Folders))
				for _, f := range listed.Folders {
					rows = append(rows, []string{text(f.ID), folderLabel(f), strconv.Itoa(f.DashboardCount), text(f.Level)})
				}
				out.table([]string{"ID", "TITLE", "DASHBOARDS", "LEVEL"}, rows)
				return nil
			})
		},
	}
}

func folderLabel(f listedFolder) string {
	switch {
	case f.General:
		return text(f.Title) + " (General)"
	case f.Personal:
		return text(f.Title) + " (yours)"
	case f.Audit:
		return text(f.Title) + " (Audit folder)"
	case f.AdminsOnly:
		return text(f.Title) + " (Admins only)"
	}
	return text(f.Title)
}

func (a *app) foldersCreateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "create <title...>",
		Short: "Create a folder",
		Long:  "create makes a folder with the given title.\nIt needs the Editor role, and a title another folder already uses is refused.",
		Args:  minimumArgs(1, "a title"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.observationCall(cmd, http.MethodPost, "folders", map[string]string{"title": strings.Join(args, " ")}, func(out *output, data []byte) error {
				created, err := decode[folderRow](data)
				if err != nil {
					return err
				}
				out.confirm("Created folder %s (%s)", text(created.Title), text(created.ID))
				return nil
			})
		},
	}
}

func (a *app) foldersRenameCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <folder-id> <title...>",
		Short: "Rename a folder",
		Long:  "rename joins every word after the id into the new title; flags such as --json or -q may go anywhere, and a title word that starts with - must come after --.\nThe General folder cannot be renamed.",
		Args:  minimumArgs(2, "a folder id and the new title"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodPatch, "folders/"+id, map[string]string{"title": strings.Join(args[1:], " ")}, func(out *output, data []byte) error {
				renamed, err := decode[folderRow](data)
				if err != nil {
					return err
				}
				out.confirm("Renamed folder to %s", text(renamed.Title))
				return nil
			})
		},
	}
}

func (a *app) foldersDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <folder-id>",
		Short: "Delete an empty folder",
		Long:  "delete removes a folder that holds nothing.\nA folder with dashboards, observations or saved searches in it is refused, and so is the General folder.",
		Args:  exactArgs(1, "one folder id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodDelete, "folders/"+id, nil, func(out *output, _ []byte) error {
				out.confirm("Deleted folder %s", text(id))
				return nil
			})
		},
	}
}
