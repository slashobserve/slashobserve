package cli

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var folderLevels = []string{"view", "comment", "edit", "admin"}

type permissionEntry struct {
	Kind string `json:"kind"`
	User *struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"user"`
	Group       string `json:"group"`
	MemberCount *int   `json:"member_count"`
	Level       string `json:"level"`
}

type folderPermissions struct {
	Restricted bool              `json:"restricted"`
	Entries    []permissionEntry `json:"entries"`
}

type permissionTarget struct{ user, email, group string }

func (t permissionTarget) check() (string, error) {
	var given []string
	for flag, value := range map[string]string{"--user": t.user, "--email": t.email, "--group": t.group} {
		if value != "" {
			given = append(given, flag)
		}
	}
	if len(given) != 1 {
		return "", usagef("name exactly one of --user <id>, --email <address> and --group <name>")
	}
	if t.user != "" {
		if _, err := parseID(t.user); err != nil {
			return "", err
		}
	}
	return given[0], nil
}

func (t permissionTarget) label() string {
	return text(t.user + t.email + t.group)
}

func (t permissionTarget) matches(e permissionEntry) bool {
	switch {
	case t.user != "":
		return e.User != nil && e.User.ID == t.user
	case t.email != "":
		return e.User != nil && strings.EqualFold(e.User.Email, t.email)
	}
	return e.Kind == "group" && e.Group == t.group
}

func (t permissionTarget) entry(level string) map[string]string {
	entry := map[string]string{"level": level}
	switch {
	case t.user != "":
		entry["user_id"] = t.user
	case t.email != "":
		entry["email"] = t.email
	default:
		entry["group"] = t.group
	}
	return entry
}

func (e permissionEntry) payload() map[string]string {
	if e.User != nil {
		return map[string]string{"user_id": e.User.ID, "level": e.Level}
	}
	return map[string]string{"group": e.Group, "level": e.Level}
}

func (e permissionEntry) who() (string, string) {
	if e.User != nil {
		return text(e.User.Name), text(e.User.Email)
	}
	members := ""
	if e.MemberCount != nil {
		members = strconv.Itoa(*e.MemberCount) + " signed in"
	}
	return text(e.Group), members
}

func (a *app) foldersPermissionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "permissions",
		Short: "Show and change who a folder is restricted to",
		Long:  "permissions manages the access list of a folder, the people and groups that can see it at a level (view, comment, edit or admin).\nA folder with an empty list is open to everyone by their role, and the first grant restricts it.\nChanging the list needs Manage on the folder, which an Admin key has on every folder except another person's personal one.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.permissionsShowCommand(), a.permissionsGrantCommand(), a.permissionsRevokeCommand(), a.permissionsOpenCommand())
	return cmd
}

func (a *app) readPermissions(cmd *cobra.Command, c *client, id string) (folderPermissions, error) {
	data, err := c.get(cmd.Context(), "folders/"+id+"/permissions", nil)
	if err != nil {
		return folderPermissions{}, err
	}
	return decode[folderPermissions](data)
}

func (a *app) replacePermissions(cmd *cobra.Command, c *client, id string, entries []map[string]string, done func(*output)) error {
	data, err := c.send(cmd.Context(), http.MethodPut, "folders/"+id+"/permissions", map[string]any{"entries": entries})
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	done(out)
	return nil
}

func (a *app) permissionsShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <folder-id>",
		Short: "List who a folder is restricted to",
		Long:  "show lists the people and groups on a folder's access list with their level, users first and then groups.\nAn empty list means the folder is open to everyone by role.",
		Args:  exactArgs(1, "one folder id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodGet, "folders/"+id+"/permissions", nil, func(out *output, data []byte) error {
				perms, err := decode[folderPermissions](data)
				if err != nil {
					return err
				}
				if !perms.Restricted {
					out.line("Open to everyone by role: the folder has no access list.")
					return nil
				}
				rows := make([][]string, 0, len(perms.Entries))
				for _, e := range perms.Entries {
					name, detail := e.who()
					rows = append(rows, []string{text(e.Kind), name, detail, text(e.Level)})
				}
				out.table([]string{"KIND", "NAME", "EMAIL OR MEMBERS", "LEVEL"}, rows)
				return nil
			})
		},
	}
}

func (a *app) permissionsGrantCommand() *cobra.Command {
	var target permissionTarget
	var level string
	cmd := &cobra.Command{
		Use:     "grant <folder-id> (--user <id> | --email <address> | --group <name>) --level <level>",
		Short:   "Give a person or group a level on a folder, or change theirs",
		Long:    "grant adds one entry to the folder's access list, or changes the level of the entry that names the same person or group, and leaves the others as they are.\nThe first entry restricts the folder to the list.\n--email matches a person who has signed in, ignoring case, and --group an identity-provider group name exactly.",
		Example: "  observe folders permissions grant <folder-id> --email ada@example.com --level edit\n  observe folders permissions grant <folder-id> --group payments --level view",
		Args:    exactArgs(1, "one folder id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if _, err := target.check(); err != nil {
				return err
			}
			if !slices.Contains(folderLevels, level) {
				return usagef("--level must be one of %s", strings.Join(folderLevels, ", "))
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			current, err := a.readPermissions(cmd, c, id)
			if err != nil {
				return err
			}
			entries := []map[string]string{}
			replaced := false
			for _, e := range current.Entries {
				if target.matches(e) {
					entries = append(entries, target.entry(level))
					replaced = true
					continue
				}
				entries = append(entries, e.payload())
			}
			if !replaced {
				entries = append(entries, target.entry(level))
			}
			return a.replacePermissions(cmd, c, id, entries, func(out *output) {
				out.confirm("Granted %s on folder %s to %s", level, text(id), target.label())
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&target.user, "user", "", "the id of a person who has signed in")
	flags.StringVar(&target.email, "email", "", "the email of a person who has signed in")
	flags.StringVar(&target.group, "group", "", "an identity-provider group name")
	flags.StringVar(&level, "level", "", "view, comment, edit or admin")
	return cmd
}

func (a *app) permissionsRevokeCommand() *cobra.Command {
	var target permissionTarget
	var yes bool
	cmd := &cobra.Command{
		Use:   "revoke <folder-id> (--user <id> | --email <address> | --group <name>) [--yes]",
		Short: "Remove a person or group from a folder's access list",
		Long:  "revoke removes the entry that names the person or group and leaves the others as they are.\nRemoving the last entry opens the folder to everyone by role again, so that needs --yes.",
		Args:  exactArgs(1, "one folder id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if _, err := target.check(); err != nil {
				return err
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			current, err := a.readPermissions(cmd, c, id)
			if err != nil {
				return err
			}
			entries := []map[string]string{}
			for _, e := range current.Entries {
				if !target.matches(e) {
					entries = append(entries, e.payload())
				}
			}
			if len(entries) == len(current.Entries) {
				return errors.New(target.label() + " is not on the access list of folder " + text(id))
			}
			if len(entries) == 0 && !yes {
				return usagef("%s is the last entry, and removing it opens folder %s to everyone by role; rerun with --yes", target.label(), id)
			}
			return a.replacePermissions(cmd, c, id, entries, func(out *output) {
				out.confirm("Removed %s from folder %s", target.label(), text(id))
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&target.user, "user", "", "the id of a person who has signed in")
	flags.StringVar(&target.email, "email", "", "the email of a person who has signed in")
	flags.StringVar(&target.group, "group", "", "an identity-provider group name")
	flags.BoolVar(&yes, "yes", false, "confirm removing the last entry")
	return cmd
}

func (a *app) permissionsOpenCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "open <folder-id> --yes",
		Short: "Remove the access list so the folder is open to everyone by role",
		Long:  "open clears the folder's access list in one step, so everyone sees it at the level their role gives.\nThat widens access, so --yes is required.",
		Args:  exactArgs(1, "one folder id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if !yes {
				return usagef("opening folder %s removes its access list and shows it to everyone by role; rerun with --yes", id)
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			return a.replacePermissions(cmd, c, id, []map[string]string{}, func(out *output) {
				out.confirm("Folder %s is open to everyone by role", text(id))
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm opening the folder")
	return cmd
}
