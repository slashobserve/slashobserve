package cli

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

var inviteRoles = []string{"admin", "editor", "commenter", "viewer"}

type peopleOptions struct {
	role string
	page pageOptions
}

type inviteView struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	InvitedBy string `json:"invited_by"`
	CreatedAt string `json:"created_at"`
	SignInURL string `json:"sign_in_url"`
}

type personView struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Email         string  `json:"email"`
	Role          string  `json:"role"`
	LastSeenAt    string  `json:"last_seen_at"`
	DeactivatedAt *string `json:"deactivated_at"`
}

type personList struct {
	Users      []personView `json:"users"`
	NextCursor *string      `json:"next_cursor"`
}

func (p personView) status() string {
	if p.DeactivatedAt != nil {
		return "deactivated"
	}
	return "active"
}

type inviteList struct {
	Invites    []inviteView `json:"invites"`
	NextCursor *string      `json:"next_cursor"`
}

func (a *app) peopleCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "people",
		Short: "List people, invite them and see who is invited",
		Long:  "people lists everyone who has signed in, invites people by email and lists pending invites.\nEvery verb needs an Admin key.\nDeactivating and removing people is done by an Admin in the web app, because keys never end another person's sessions.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.peopleListCommand(), a.peopleInviteCommand(), a.peopleInvitesCommand(), a.peopleUninviteCommand())
	return cmd
}

func (a *app) peopleListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List everyone who has signed in, newest first",
		Args:  noArgs,
		RunE:  a.runPeopleList,
	}
	addPageFlags(cmd, &a.people.page, maxPeoplePage)
	return cmd
}

func (a *app) peopleInviteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "invite <email> --role <role>",
		Short:   "Invite an email with a role, or change the role of a pending invite",
		Long:    "invite records a pending invite and prints the sign-in link to send; no email is sent.\nThe role applies when that email first signs in through the identity provider with a verified email.\nThe invite lets nobody in that the sign-in rules refuse, and inviting the same email again changes only its role.",
		Example: "  observe people invite new@example.com --role viewer",
		Args:    exactArgs(1, "one email address"),
		RunE:    a.runPeopleInvite,
	}
	cmd.Flags().StringVar(&a.people.role, "role", "", "the role to give at first sign-in: "+strings.Join(inviteRoles, ", "))
	return cmd
}

func (a *app) peopleInvitesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invites",
		Short: "List pending invites, newest first",
		Args:  noArgs,
		RunE:  a.runPeopleInvites,
	}
	addPageFlags(cmd, &a.people.page, maxPeoplePage)
	return cmd
}

func (a *app) peopleUninviteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninvite <invite id>",
		Short: "Withdraw a pending invite",
		Args:  exactArgs(1, "an invite id"),
		RunE:  a.runPeopleUninvite,
	}
}

func (a *app) runPeopleInvite(cmd *cobra.Command, args []string) error {
	role := strings.ToLower(strings.TrimSpace(a.people.role))
	if !slices.Contains(inviteRoles, role) {
		return usagef("pass --role %s", strings.Join(inviteRoles, ", "))
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "invites", map[string]any{"email": args[0], "role": role})
	if err != nil {
		return err
	}
	out := a.output()
	return render(out, data, func(invite inviteView) error {
		out.confirm("Invited %s as %s", text(invite.Email), text(invite.Role))
		out.fields([][2]string{
			{"ID", text(invite.ID)},
			{"Sign-in link", text(invite.SignInURL)},
		})
		return nil
	})
}

func (a *app) runPeopleList(cmd *cobra.Command, _ []string) error {
	if err := a.people.page.check(); err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.getPages(cmd.Context(), "users", a.people.page.query(), a.people.page.all)
	if err != nil {
		return err
	}
	out := a.output()
	return render(out, data, func(page personList) error {
		if len(page.Users) == 0 {
			out.notice("Nobody has signed in yet")
			return nil
		}
		rows := make([][]string, 0, len(page.Users))
		for _, p := range page.Users {
			rows = append(rows, []string{text(p.ID), text(p.Name), text(p.Email), text(p.Role), p.status(), when(p.LastSeenAt)})
		}
		out.table([]string{"ID", "NAME", "EMAIL", "ROLE", "STATUS", "LAST ACTIVE"}, rows)
		pageFooter(out, len(page.Users), nil, page.NextCursor)
		return nil
	})
}

func (a *app) runPeopleInvites(cmd *cobra.Command, _ []string) error {
	if err := a.people.page.check(); err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.getPages(cmd.Context(), "invites", a.people.page.query(), a.people.page.all)
	if err != nil {
		return err
	}
	out := a.output()
	return render(out, data, func(page inviteList) error {
		if len(page.Invites) == 0 {
			out.notice("No pending invites")
			return nil
		}
		rows := make([][]string, 0, len(page.Invites))
		for _, i := range page.Invites {
			rows = append(rows, []string{text(i.ID), text(i.Email), text(i.Role), text(i.InvitedBy), when(i.CreatedAt)})
		}
		out.table([]string{"ID", "EMAIL", "ROLE", "INVITED BY", "INVITED"}, rows)
		pageFooter(out, len(page.Invites), nil, page.NextCursor)
		return nil
	})
}

func (a *app) runPeopleUninvite(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	if _, err := c.delete(cmd.Context(), "invites/"+id); err != nil {
		return err
	}
	if !a.json {
		a.output().confirm("Withdrew invite %s", text(id))
	}
	return nil
}
