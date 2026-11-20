package cli

import (
	"net/http"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

var accessObjectKinds = []string{"dashboard", "observation"}

type accessRequest struct {
	ID     string `json:"id"`
	Object struct {
		Kind  string `json:"kind"`
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"object"`
	Folder struct {
		Title   string `json:"title"`
		General bool   `json:"general"`
	} `json:"folder"`
	RequestedBy struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"requested_by"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
}

func parseAccessObject(raw string) (string, string, error) {
	kind, id, found := strings.Cut(raw, ":")
	if !found || !slices.Contains(accessObjectKinds, kind) || !idPattern.MatchString(id) {
		return "", "", usagef("%q is not <kind>:<id> with kind dashboard or observation", text(raw))
	}
	return kind, id, nil
}

func (a *app) accessCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "access",
		Short: "Ask for comment access, and see who asked you",
		Long:  "access asks the people who manage a folder's sharing for Comment access to a dashboard or observation in it, and lists the open requests on the folders you manage.\nAsking changes no access: whoever manages the folder grants it by giving you Comment on the folder (observe folders permissions grant) or by raising your role.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.accessRequestCommand(), a.accessRequestsCommand())
	return cmd
}

func (a *app) accessRequestCommand() *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:     "request <kind>:<id> [--note <text>]",
		Short:   "Ask for Comment access to a dashboard or observation you can view",
		Long:    "request sends a request for Comment access to the inbox of the people who manage the sharing of the object's folder, with an optional one-line note of at most 200 characters.\nIt changes no access, and the answer does not name who received it.\nYou can have one open request per object: asking again while it is open answers request_open, and an object you can already comment on answers already_can_comment.\nThe request closes when you are given Comment, when a person who received it marks it done in their inbox, or after 30 days.",
		Example: "  observe access request dashboard:<id> --note \"I am on call for checkout this week\"\n  observe access request observation:<id>",
		Args:    exactArgs(1, "one object as <kind>:<id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, id, err := parseAccessObject(args[0])
			if err != nil {
				return err
			}
			body := map[string]any{"object": map[string]string{"kind": kind, "id": id}}
			if note != "" {
				body["note"] = note
			}
			return a.observationCall(cmd, http.MethodPost, "access-requests", body, func(out *output, data []byte) error {
				request, err := decode[accessRequest](data)
				if err != nil {
					return err
				}
				out.line("Asked for comment access to %s %s; the people who manage its folder will see it in their inbox", text(request.Object.Kind), text(request.Object.Title))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "one line for the people who can grant access, at most 200 characters")
	return cmd
}

func (a *app) accessRequestsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "requests",
		Short: "List the open comment access requests on folders you manage",
		Long:  "requests lists the open requests for Comment access on dashboards and observations whose folder you manage, newest first, at most 100.\nGrant one with observe folders permissions grant <folder-id> --user <id> --level comment, or by raising the person's role.\nIn General, which is open to everyone by role and takes no entry, only a role change or moving the object to a folder first can grant it.",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.observationCall(cmd, http.MethodGet, "access-requests", nil, func(out *output, data []byte) error {
				list, err := decode[struct {
					Items []accessRequest `json:"items"`
				}](data)
				if err != nil {
					return err
				}
				if len(list.Items) == 0 {
					out.line("No open access requests on folders you manage.")
					return nil
				}
				rows := make([][]string, 0, len(list.Items))
				inGeneral := false
				for _, r := range list.Items {
					inGeneral = inGeneral || r.Folder.General
					rows = append(rows, []string{when(r.CreatedAt), text(r.RequestedBy.Name), text(r.RequestedBy.Kind) + ":" + text(r.RequestedBy.ID), text(r.Object.Kind) + ":" + text(r.Object.ID), text(r.Object.Title), text(r.Folder.Title), text(r.Note)})
				}
				out.table([]string{"ASKED", "WHO", "WHO ID", "OBJECT", "TITLE", "FOLDER", "NOTE"}, rows)
				if inGeneral {
					out.notice("General takes no folder entry, so a request there cannot be granted with folders permissions: raise the person's role on the People page, or move the object to a folder first (observe dashboards move, observe observations move)")
				}
				return nil
			})
		},
	}
}
