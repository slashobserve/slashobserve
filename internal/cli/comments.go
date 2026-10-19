package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const commentPreviewRunes = 80

type commentListOptions struct {
	pageOptions
	status string
	panel  string
}

type commentResolveOptions struct {
	version int
}

type comment struct {
	ID                string  `json:"id"`
	ParentID          *string `json:"parent_id"`
	PanelID           *string `json:"panel_id"`
	PanelPresent      bool    `json:"panel_present"`
	Status            string  `json:"status"`
	Author            author  `json:"author"`
	CreatedAt         string  `json:"created_at"`
	Body              string  `json:"body"`
	ResolvedByVersion *int    `json:"resolved_by_version"`
}

type commentList struct {
	Comments   []comment `json:"comments"`
	NextCursor *string   `json:"next_cursor"`
}

type commentEnvelope struct {
	Comment comment `json:"comment"`
}

func (a *app) commentsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comments",
		Short: "List, answer and resolve dashboard comments",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.commentsListCommand(), a.commentsReplyCommand(), a.commentsResolveCommand())
	return cmd
}

func (a *app) commentsListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <dashboard-id>",
		Short: "List the comments of a dashboard, oldest first",
		Args:  exactArgs(1, "one dashboard id"),
		RunE:  a.runCommentsList,
	}
	addPageFlags(cmd, &a.commentList.pageOptions)
	cmd.Flags().StringVar(&a.commentList.status, "status", "open", "open, resolved or all")
	cmd.Flags().StringVar(&a.commentList.panel, "panel", "", "only comments on this panel id")
	return cmd
}

func (a *app) runCommentsList(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	switch a.commentList.status {
	case "open", "resolved", "all":
	default:
		return usagef("--status must be open, resolved or all")
	}
	query := a.commentList.query()
	query.Set("status", a.commentList.status)
	if a.commentList.panel != "" {
		query.Set("panel_id", a.commentList.panel)
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "dashboards/"+id+"/comments", query)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	page, err := decode[commentList](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(page.Comments))
	for _, cm := range page.Comments {
		rows = append(rows, []string{
			text(cm.ID),
			orDash(cm.ParentID),
			commentPanel(cm),
			text(cm.Status),
			text(cm.Author.Name),
			when(cm.CreatedAt),
			commentPreview(cm.Body),
		})
	}
	out.table([]string{"ID", "THREAD", "PANEL", "STATUS", "AUTHOR", "CREATED", "COMMENT"}, rows)
	nextPageHint(out, page.NextCursor)
	return nil
}

func commentPanel(cm comment) string {
	if cm.PanelID == nil {
		return "dashboard"
	}
	panel := text(*cm.PanelID)
	if !cm.PanelPresent {
		panel += " (removed)"
	}
	return panel
}

func commentPreview(body string) string {
	first, _, _ := strings.Cut(body, "\n")
	runes := []rune(text(first))
	if len(runes) > commentPreviewRunes {
		return string(runes[:commentPreviewRunes]) + "..."
	}
	return string(runes)
}

func (a *app) commentsReplyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reply [flags] <dashboard-id> <comment-id> <text...>",
		Short: "Reply to a comment thread",
		Long:  "Everything after the two ids is the reply text, flag-like words included, so flags such as --json or -q must come before the ids.",
		Args:  cobra.ArbitraryArgs,
		RunE:  a.runCommentsReply,
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func (a *app) runCommentsReply(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return usagef("%s takes a dashboard id, a comment id and the reply text", cmd.CommandPath())
	}
	dashboardID, err := parseID(args[0])
	if err != nil {
		return err
	}
	commentID, err := parseID(args[1])
	if err != nil {
		return err
	}
	body := strings.TrimSpace(strings.Join(args[2:], " "))
	if body == "" {
		return usagef("reply needs text")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "dashboards/"+dashboardID+"/comments", map[string]string{"parent_id": commentID, "body": body})
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	created, err := decode[commentEnvelope](data)
	if err != nil {
		return err
	}
	thread := commentID
	if created.Comment.ParentID != nil {
		thread = *created.Comment.ParentID
	}
	out.line("Replied to thread %s as %s", text(thread), text(created.Comment.ID))
	return nil
}

func (a *app) commentsResolveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <dashboard-id> <comment-id>",
		Short: "Resolve a comment thread",
		Args:  exactArgs(2, "a dashboard id and a comment id"),
		RunE:  a.runCommentsResolve,
	}
	cmd.Flags().IntVar(&a.commentResolve.version, "version", 0, "record the version that fixed the thread")
	return cmd
}

func (a *app) runCommentsResolve(cmd *cobra.Command, args []string) error {
	dashboardID, err := parseID(args[0])
	if err != nil {
		return err
	}
	commentID, err := parseID(args[1])
	if err != nil {
		return err
	}
	payload := map[string]any{"status": "resolved"}
	if cmd.Flags().Changed("version") {
		if a.commentResolve.version < 1 {
			return usagef("--version must be 1 or higher")
		}
		payload["resolved_by_version"] = a.commentResolve.version
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.patch(cmd.Context(), "dashboards/"+dashboardID+"/comments/"+commentID, payload)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
	}
	resolved, err := decode[commentEnvelope](data)
	if err != nil {
		return err
	}
	thread := resolved.Comment
	if cmd.Flags().Changed("version") && (thread.ResolvedByVersion == nil || *thread.ResolvedByVersion != a.commentResolve.version) {
		return notAppliedError(thread)
	}
	if a.json {
		return nil
	}
	if thread.ResolvedByVersion != nil {
		out.confirm("Resolved thread %s by version %d", text(thread.ID), *thread.ResolvedByVersion)
		return nil
	}
	out.confirm("Resolved thread %s", text(thread.ID))
	return nil
}

func notAppliedError(thread comment) error {
	if thread.ResolvedByVersion == nil {
		return fmt.Errorf("thread %s is already resolved; --version was not applied", text(thread.ID))
	}
	return fmt.Errorf("thread %s is already resolved by version %d; --version was not applied", text(thread.ID), *thread.ResolvedByVersion)
}
