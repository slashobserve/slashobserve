package cli

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

const commentPreviewRunes = 80

var commentTargetKinds = []string{"dashboard", "annotation", "observation", "proposal", "finding"}

type commentListOptions struct {
	pageOptions
	status string
	panel  string
	on     string
}

type commentAddOptions struct {
	on        string
	panel     string
	version   int
	startRuns bool
}

type commentReplyOptions struct {
	startRuns bool
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

type commentThread struct {
	ID     string `json:"id"`
	Target struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"target"`
	PanelID           *string `json:"panel_id"`
	PanelPresent      bool    `json:"panel_present"`
	Status            string  `json:"status"`
	ResolvedByVersion *int    `json:"resolved_by_version"`
}

type threadMessage struct {
	ID         string        `json:"id"`
	ThreadID   string        `json:"thread_id"`
	Actor      activityActor `json:"actor"`
	CreatedAt  string        `json:"created_at"`
	Body       string        `json:"body"`
	Run        *startedRun   `json:"run"`
	RunRefusal *runRefusal   `json:"run_refusal"`
}

type startedRun struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	AssignedKind string `json:"assigned_kind"`
}

type runRefusal struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type threadPage struct {
	Threads    []commentThread `json:"threads"`
	Messages   []threadMessage `json:"messages"`
	NextCursor *string         `json:"next_cursor"`
}

type threadEnvelope struct {
	Thread commentThread `json:"thread"`
}

type threadMessageEnvelope struct {
	Thread  commentThread `json:"thread"`
	Message threadMessage `json:"message"`
}

type commentRow struct {
	id, thread, panel, status, author, created, body string
}

func (r commentRow) cells() []string {
	return []string{text(r.id), r.thread, r.panel, text(r.status), text(r.author), when(r.created), commentPreview(r.body)}
}

func (a *app) commentsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comments",
		Short: "Comment on dashboards, observations, annotations and proposals",
		Long:  "comments works the comment threads on a dashboard, an observation, an annotation or a proposal.\nA target is written <kind>:<id> with kind dashboard, annotation, observation or proposal, for example observation:0a1b2c3d-0000-4000-8000-000000000001.\nThe kind finding is deprecated: it resolves to the discussion of the finding's observation.\nMention a person in a comment as @[Name](user:<id>), the built-in agent as @observe, one of your connected agents as @[Name](grant:<id>) and a service account that takes runs as @[Name](service_account:<id>).\nA mention is recorded in the activity log and starts no run unless add or reply is given --start-runs.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		a.commentsListCommand(),
		a.commentsAddCommand(),
		a.commentsReplyCommand(),
		a.commentsEditCommand(),
		a.commentsResolveCommand(),
		a.commentsReopenCommand(),
	)
	return cmd
}

func parseOn(raw string) (string, error) {
	if raw == "" {
		return "", usagef("--on <kind>:<id> is required")
	}
	kind, id, found := strings.Cut(raw, ":")
	if !found || !slices.Contains(commentTargetKinds, kind) || !idPattern.MatchString(id) {
		return "", usagef("--on %q is not <kind>:<id> with kind dashboard, annotation, observation or proposal; finding is deprecated and means observation", text(raw))
	}
	return raw, nil
}

func (a *app) commentsListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [<dashboard-id>] [--on <kind>:<id>]",
		Short: "List the comments of a dashboard, observation, annotation or proposal, oldest first",
		Args:  cobra.MaximumNArgs(1),
		RunE:  a.runCommentsList,
	}
	addPageFlags(cmd, &a.commentList.pageOptions)
	cmd.Flags().StringVar(&a.commentList.status, "status", "open", "open, resolved or all")
	cmd.Flags().StringVar(&a.commentList.panel, "panel", "", "only comments on this panel id")
	cmd.Flags().StringVar(&a.commentList.on, "on", "", "the target to list, as <kind>:<id>, instead of a dashboard id")
	return cmd
}

func (a *app) commentListRequest(args []string) (string, url.Values, error) {
	if (len(args) == 1) == (a.commentList.on != "") {
		return "", nil, usagef("comments list takes a dashboard id or --on <kind>:<id>, one of them")
	}
	switch a.commentList.status {
	case "open", "resolved", "all":
	default:
		return "", nil, usagef("--status must be open, resolved or all")
	}
	query := a.commentList.query()
	query.Set("status", a.commentList.status)
	if a.commentList.panel != "" {
		query.Set("panel_id", a.commentList.panel)
	}
	if len(args) == 0 {
		target, err := parseOn(a.commentList.on)
		if err != nil {
			return "", nil, err
		}
		query.Set("target", target)
		return "threads", query, nil
	}
	id, err := parseID(args[0])
	if err != nil {
		return "", nil, err
	}
	return "dashboards/" + id + "/comments", query, nil
}

func (a *app) runCommentsList(cmd *cobra.Command, args []string) error {
	path, query, err := a.commentListRequest(args)
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), path, query)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	rows, next, err := commentRows(data, len(args) == 1)
	if err != nil {
		return err
	}
	cells := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells = append(cells, row.cells())
	}
	out.table([]string{"ID", "THREAD", "PANEL", "STATUS", "AUTHOR", "CREATED", "COMMENT"}, cells)
	nextPageHint(out, next)
	return nil
}

func commentRows(data []byte, byDashboard bool) ([]commentRow, *string, error) {
	if byDashboard {
		page, err := decode[commentList](data)
		if err != nil {
			return nil, nil, err
		}
		rows := make([]commentRow, 0, len(page.Comments))
		for _, cm := range page.Comments {
			rows = append(rows, commentRow{
				id:      cm.ID,
				thread:  orDash(cm.ParentID),
				panel:   panelLabel(cm.PanelID, cm.PanelPresent, "dashboard"),
				status:  cm.Status,
				author:  cm.Author.Name,
				created: cm.CreatedAt,
				body:    cm.Body,
			})
		}
		return rows, page.NextCursor, nil
	}
	page, err := decode[threadPage](data)
	if err != nil {
		return nil, nil, err
	}
	threads := make(map[string]commentThread, len(page.Threads))
	for _, th := range page.Threads {
		threads[th.ID] = th
	}
	rows := make([]commentRow, 0, len(page.Messages))
	for _, m := range page.Messages {
		th := threads[m.ThreadID]
		thread := "-"
		if m.ID != m.ThreadID {
			thread = text(m.ThreadID)
		}
		rows = append(rows, commentRow{
			id:      m.ID,
			thread:  thread,
			panel:   panelLabel(th.PanelID, th.PanelPresent, th.Target.Kind),
			status:  th.Status,
			author:  m.Actor.Label,
			created: m.CreatedAt,
			body:    m.Body,
		})
	}
	return rows, page.NextCursor, nil
}

func panelLabel(panelID *string, present bool, whole string) string {
	if panelID == nil {
		return text(whole)
	}
	panel := text(*panelID)
	if !present {
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

func (a *app) commentsAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [flags] --on <kind>:<id> <text...>",
		Short: "Start a comment thread on a dashboard, observation, annotation or proposal",
		Long:  "Everything after the flags is the comment text, flag-like words included, so --on, --panel, --version, --json and -q must come before it.\n--panel and --version anchor a dashboard comment to a panel and to a version, and the server refuses them on an observation, an annotation or a proposal.\n--start-runs starts a run for the one agent the text mentions: a dashboard thread starts a comment run, an observation thread starts an investigation, and an annotation or a proposal thread starts none.\nThe comment is stored even when the run cannot start, and the reason is printed.",
		Args:  cobra.ArbitraryArgs,
		RunE:  a.runCommentsAdd,
	}
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().StringVar(&a.commentAdd.on, "on", "", "what to comment on, as <kind>:<id>")
	cmd.Flags().StringVar(&a.commentAdd.panel, "panel", "", "the panel of the dashboard to comment on")
	cmd.Flags().IntVar(&a.commentAdd.version, "version", 0, "the dashboard version to comment on; the latest by default")
	cmd.Flags().BoolVar(&a.commentAdd.startRuns, "start-runs", false, "start a run for the one agent the text mentions")
	return cmd
}

func (a *app) runCommentsAdd(cmd *cobra.Command, args []string) error {
	target, err := parseOn(a.commentAdd.on)
	if err != nil {
		return err
	}
	payload := map[string]any{"target": target, "body": strings.TrimSpace(strings.Join(args, " "))}
	if payload["body"] == "" {
		return usagef("add needs text")
	}
	if a.commentAdd.panel != "" {
		payload["panel_id"] = a.commentAdd.panel
	}
	if cmd.Flags().Changed("version") {
		if a.commentAdd.version < 1 {
			return usagef("--version must be 1 or higher")
		}
		payload["version"] = a.commentAdd.version
	}
	if a.commentAdd.startRuns {
		payload["start_runs"] = true
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "threads", payload)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	created, err := decode[threadMessageEnvelope](data)
	if err != nil {
		return err
	}
	out.line("Started thread %s on %s:%s", text(created.Thread.ID), text(created.Thread.Target.Kind), text(created.Thread.Target.ID))
	reportRun(out, created.Message)
	return nil
}

func reportRun(out *output, message threadMessage) {
	if message.Run != nil {
		out.confirm("Started run %s, taken by %s", text(message.Run.ID), text(message.Run.AssignedKind))
	}
	if message.RunRefusal != nil {
		out.warn("No run started (%s): %s", text(message.RunRefusal.Code), text(message.RunRefusal.Message))
	}
}

func (a *app) commentsReplyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reply [flags] [<dashboard-id>] <comment-id> <text...>",
		Short: "Reply to a comment thread",
		Long:  "Everything after the ids is the reply text, flag-like words included, so flags such as --json, -q or --start-runs must come before the ids.\nThe comment id can be any comment of the thread, on a dashboard, an observation, an annotation or a proposal.\nWhen the first two arguments are both ids, the first is read as a dashboard id.\n--start-runs starts a run for the one agent the text mentions, with the earlier messages of the thread as its context; it needs the thread form, with no dashboard id.",
		Args:  cobra.ArbitraryArgs,
		RunE:  a.runCommentsReply,
	}
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().BoolVar(&a.commentReply.startRuns, "start-runs", false, "start a run for the one agent the text mentions")
	return cmd
}

func hasDashboardID(args []string) bool {
	return len(args) > 1 && idPattern.MatchString(args[0]) && idPattern.MatchString(args[1])
}

func (a *app) runCommentsReply(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return usagef("%s takes a comment id and the reply text", cmd.CommandPath())
	}
	withDashboard := hasDashboardID(args)
	ids := 1
	if withDashboard {
		ids = 2
	}
	body := strings.TrimSpace(strings.Join(args[ids:], " "))
	if body == "-h" || body == "--help" {
		return cmd.Help()
	}
	commentID, err := parseID(args[ids-1])
	if err != nil {
		return err
	}
	path, payload := "threads/"+commentID+"/messages", map[string]any{"body": body}
	if withDashboard {
		if a.commentReply.startRuns {
			return usagef("--start-runs needs a reply by comment id alone, without a dashboard id")
		}
		dashboardID, err := parseID(args[0])
		if err != nil {
			return err
		}
		path, payload = "dashboards/"+dashboardID+"/comments", map[string]any{"parent_id": commentID, "body": body}
	}
	if a.commentReply.startRuns {
		payload["start_runs"] = true
	}
	if body == "" {
		return usagef("reply needs text")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), path, payload)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	thread, message, err := repliedIDs(data, withDashboard, commentID)
	if err != nil {
		return err
	}
	out.line("Replied to thread %s as %s", text(thread), text(message.ID))
	reportRun(out, message)
	return nil
}

func repliedIDs(data []byte, legacy bool, commentID string) (thread string, message threadMessage, err error) {
	if !legacy {
		created, err := decode[threadMessageEnvelope](data)
		return created.Thread.ID, created.Message, err
	}
	created, err := decode[commentEnvelope](data)
	if err != nil {
		return "", threadMessage{}, err
	}
	thread = commentID
	if created.Comment.ParentID != nil {
		thread = *created.Comment.ParentID
	}
	return thread, threadMessage{ID: created.Comment.ID}, nil
}

func (a *app) commentsEditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit [flags] <comment-id> <text...>",
		Short: "Replace the text of one of your own comments",
		Long:  "Everything after the id is the new text, flag-like words included, so flags such as --json or -q must come before the id.\nOnly the author can edit a comment, and only while they can still comment on its target.",
		Args:  cobra.ArbitraryArgs,
		RunE:  a.runCommentsEdit,
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func (a *app) runCommentsEdit(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return usagef("%s takes a comment id and the new text", cmd.CommandPath())
	}
	body := strings.TrimSpace(strings.Join(args[1:], " "))
	if body == "-h" || body == "--help" {
		return cmd.Help()
	}
	commentID, err := parseID(args[0])
	if err != nil {
		return err
	}
	if body == "" {
		return usagef("edit needs text")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.patch(cmd.Context(), "messages/"+commentID, map[string]string{"body": body})
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	edited, err := decode[threadMessageEnvelope](data)
	if err != nil {
		return err
	}
	out.confirm("Edited comment %s", text(edited.Message.ID))
	return nil
}

type resolution struct {
	id                string
	resolvedByVersion *int
}

func (a *app) commentsResolveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve [<dashboard-id>] <comment-id>",
		Short: "Resolve a comment thread",
		Long:  "The comment id can be any comment of the thread, on a dashboard, an observation, an annotation or a proposal.\nWhen two ids are given, the first is read as a dashboard id.",
		Args:  rangeArgs(1, 2, "a comment id, after an optional dashboard id"),
		RunE:  a.runCommentsResolve,
	}
	cmd.Flags().IntVar(&a.commentResolve.version, "version", 0, "record the version that fixed the thread; dashboards only")
	return cmd
}

func (a *app) runCommentsResolve(cmd *cobra.Command, args []string) error {
	ids := make([]string, 0, len(args))
	for _, arg := range args {
		id, err := parseID(arg)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	versionGiven := cmd.Flags().Changed("version")
	if versionGiven && a.commentResolve.version < 1 {
		return usagef("--version must be 1 or higher")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	legacy := len(ids) == 2
	data, err := a.sendResolve(cmd, c, ids, versionGiven)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
	}
	resolved, err := resolutionOf(data, legacy)
	if err != nil {
		return err
	}
	if versionGiven && (resolved.resolvedByVersion == nil || *resolved.resolvedByVersion != a.commentResolve.version) {
		return notAppliedError(resolved)
	}
	if a.json {
		return nil
	}
	if resolved.resolvedByVersion != nil {
		out.confirm("Resolved thread %s by version %d", text(resolved.id), *resolved.resolvedByVersion)
		return nil
	}
	out.confirm("Resolved thread %s", text(resolved.id))
	return nil
}

func (a *app) sendResolve(cmd *cobra.Command, c *client, ids []string, versionGiven bool) ([]byte, error) {
	if len(ids) == 2 {
		payload := map[string]any{"status": "resolved"}
		if versionGiven {
			payload["resolved_by_version"] = a.commentResolve.version
		}
		return c.patch(cmd.Context(), "dashboards/"+ids[0]+"/comments/"+ids[1], payload)
	}
	payload := map[string]any{}
	if versionGiven {
		payload["version"] = a.commentResolve.version
	}
	return c.post(cmd.Context(), "threads/"+ids[0]+"/resolve", payload)
}

func resolutionOf(data []byte, legacy bool) (resolution, error) {
	if !legacy {
		resolved, err := decode[threadEnvelope](data)
		return resolution{resolved.Thread.ID, resolved.Thread.ResolvedByVersion}, err
	}
	resolved, err := decode[commentEnvelope](data)
	return resolution{resolved.Comment.ID, resolved.Comment.ResolvedByVersion}, err
}

func notAppliedError(r resolution) error {
	if r.resolvedByVersion == nil {
		return fmt.Errorf("thread %s is already resolved; --version was not applied", text(r.id))
	}
	return fmt.Errorf("thread %s is already resolved by version %d; --version was not applied", text(r.id), *r.resolvedByVersion)
}

func (a *app) commentsReopenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reopen <comment-id>",
		Short: "Reopen a resolved comment thread",
		Long:  "The comment id can be any comment of the thread.\nReopening clears who resolved the thread and the version that fixed it.",
		Args:  exactArgs(1, "a comment id"),
		RunE:  a.runCommentsReopen,
	}
}

func (a *app) runCommentsReopen(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.do(cmd.Context(), http.MethodPost, "threads/"+id+"/reopen", nil, nil)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	reopened, err := decode[threadEnvelope](data)
	if err != nil {
		return err
	}
	out.confirm("Reopened thread %s", text(reopened.Thread.ID))
	return nil
}
