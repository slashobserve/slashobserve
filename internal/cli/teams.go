package cli

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type teamMemberView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type teamView struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Members []teamMemberView `json:"members"`
}

type teamList struct {
	Teams []teamView `json:"teams"`
}

func (a *app) teamsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "teams",
		Short: "Manage the teams that folders are shared with",
		Long:  "teams lists, creates and deletes teams and changes their members.\nA folder entry naming a team's name as a group gives its members that access.\nEvery verb needs an Admin key.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "List the teams and their members", Args: noArgs, RunE: a.runTeamsList},
		&cobra.Command{Use: "create <name>", Short: "Create a team", Args: exactArgs(1, "a team name"), RunE: a.runTeamsCreate},
		&cobra.Command{Use: "delete <team id>", Short: "Delete a team", Args: exactArgs(1, "a team id"), RunE: a.runTeamsDelete},
		&cobra.Command{Use: "add <team id> <person id>", Short: "Add a person to a team", Args: exactArgs(2, "a team id and a person id"), RunE: a.runTeamsMember(http.MethodPut, "Added to team %s")},
		&cobra.Command{Use: "remove <team id> <person id>", Short: "Remove a person from a team", Args: exactArgs(2, "a team id and a person id"), RunE: a.runTeamsMember(http.MethodDelete, "Removed from team %s")},
	)
	return cmd
}

func memberNames(members []teamMemberView) string {
	names := make([]string, 0, len(members))
	for _, m := range members {
		name := m.Name
		if name == "" {
			name = m.Email
		}
		names = append(names, text(name))
	}
	return strings.Join(names, ", ")
}

func (a *app) runTeamsList(cmd *cobra.Command, _ []string) error {
	data, err := a.fetch(cmd.Context(), "teams", nil)
	if err != nil {
		return err
	}
	out := a.output()
	return render(out, data, func(list teamList) error {
		if len(list.Teams) == 0 {
			out.notice("No teams yet")
			return nil
		}
		rows := make([][]string, 0, len(list.Teams))
		for _, t := range list.Teams {
			rows = append(rows, []string{text(t.ID), text(t.Name), strconv.Itoa(len(t.Members)), memberNames(t.Members)})
		}
		out.table([]string{"ID", "NAME", "MEMBERS", "PEOPLE"}, rows)
		return nil
	})
}

func (a *app) runTeamsCreate(cmd *cobra.Command, args []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "teams", map[string]any{"name": args[0]})
	if err != nil {
		return err
	}
	return a.printTeam(data, "Created team %s")
}

func (a *app) runTeamsDelete(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	if _, err := c.delete(cmd.Context(), "teams/"+id); err != nil {
		return err
	}
	if !a.json {
		a.output().confirm("Deleted team %s", text(id))
	}
	return nil
}

func (a *app) runTeamsMember(method, done string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		teamID, err := parseID(args[0])
		if err != nil {
			return err
		}
		userID, err := parseID(args[1])
		if err != nil {
			return err
		}
		c, err := a.newClient()
		if err != nil {
			return err
		}
		data, err := c.do(cmd.Context(), method, "teams/"+teamID+"/members/"+userID, nil, nil)
		if err != nil {
			return err
		}
		return a.printTeam(data, done)
	}
}

func (a *app) printTeam(data []byte, format string) error {
	out := a.output()
	return render(out, data, func(team teamView) error {
		out.confirm(format, text(team.Name))
		out.fields([][2]string{
			{"ID", text(team.ID)},
			{"Members", memberNames(team.Members)},
		})
		return nil
	})
}
