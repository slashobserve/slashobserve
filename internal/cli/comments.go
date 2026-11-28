package cli

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slashobserve/slashobserve/internal/anchor"
)

const commentPreviewRunes = 80

var commentTargetKinds = []string{"dashboard", "annotation", "observation", "proposal"}

type commentListOptions struct {
	pageOptions
	status string
	panel  string
	on     string
}

type commentAddOptions struct {
	on      string
	panel   string
	block   string
	at      string
	version int
	to      string
}

type commentReplyOptions struct {
	block   string
	replyTo string
	to      string
}

const (
	toAgent    = "agent"
	toEveryone = "everyone"
)

func deliveryOf(to string) (string, error) {
	switch to {
	case "":
		return toEveryone, nil
	case toAgent, toEveryone:
		return to, nil
	}
	return "", usagef("--to must be agent or everyone, not %q", to)
}

func (o commentReplyOptions) addTo(payload map[string]any) error {
	to, err := deliveryOf(o.to)
	if err != nil {
		return err
	}
	payload["to"] = to
	if err := setID(payload, "block_id", o.block); err != nil {
		return err
	}
	return setID(payload, "reply_to_id", o.replyTo)
}

type commentResolveOptions struct {
	version int
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
	Answer     *threadAnswer `json:"answer"`
}

type threadAnswer struct {
	Finding struct {
		Title string `json:"title"`
	} `json:"finding"`
}

func (m threadMessage) preview() string {
	if m.Answer == nil || m.Answer.Finding.Title == "" {
		return m.Body
	}
	return m.Body + " " + m.Answer.Finding.Title
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
		Long:  "comments works the comment threads on a dashboard, an observation, an annotation or a proposal.\nA target is written <kind>:<id> with kind dashboard, annotation, observation or proposal, for example observation:0a1b2c3d-0000-4000-8000-000000000001.\nMention a person in a comment as @[Name](user:<id>), the built-in agent as @observe, one of your connected agents as @[Name](grant:<id>) and a service account that takes runs as @[Name](service_account:<id>).\nA mention is recorded in the activity log, and add and reply ask no agent unless given --to agent.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		a.commentsListCommand(),
		a.commentsAddCommand(),
		a.commentsReplyCommand(),
		a.commentsEditCommand(),
		a.commentsDeleteCommand(),
		a.commentsWithdrawCommand(),
		a.commentsApplyCommand(),
		a.commentsPeopleCommand(),
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
		return "", usagef("--on %q is not <kind>:<id> with kind dashboard, annotation, observation or proposal", text(raw))
	}
	return raw, nil
}

func (a *app) commentsListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [<dashboard-id>] [--on <kind>:<id>]",
		Short: "List the comments of a dashboard, observation, annotation or proposal, oldest first",
		Args:  rangeArgs(0, 1, "a dashboard id, or none with --on"),
		RunE:  a.runCommentsList,
	}
	addPageFlags(cmd, &a.commentList.pageOptions, maxThreadPage)
	cmd.Flags().StringVar(&a.commentList.status, "status", "open", "open, resolved or all")
	cmd.Flags().StringVar(&a.commentList.panel, "panel", "", "only comments on this panel id")
	cmd.Flags().StringVar(&a.commentList.on, "on", "", "the target to list, as <kind>:<id>, instead of a dashboard id")
	return cmd
}

func (a *app) commentListRequest(args []string) (string, url.Values, error) {
	if err := a.commentList.check(); err != nil {
		return "", nil, err
	}
	if len(args) == 1 && a.commentList.on == "" && strings.Contains(args[0], ":") {
		a.commentList.on, args = args[0], nil
	}
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
	target, err := a.commentListTarget(args)
	if err != nil {
		return "", nil, err
	}
	query.Set("target", target)
	return "threads", query, nil
}

func (a *app) commentListTarget(args []string) (string, error) {
	if len(args) == 0 {
		return parseOn(a.commentList.on)
	}
	id, err := parseID(args[0])
	if err != nil {
		return "", err
	}
	return "dashboard:" + id, nil
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
	data, err := c.getPages(cmd.Context(), path, query, a.commentList.all)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	rows, next, err := commentRows(data)
	if err != nil {
		return err
	}
	cells := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells = append(cells, row.cells())
	}
	out.table([]string{"ID", "THREAD", "PANEL", "STATUS", "AUTHOR", "CREATED", "COMMENT"}, cells)
	pageFooter(out, len(cells), nil, next)
	return nil
}

func commentRows(data []byte) ([]commentRow, *string, error) {
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
			body:    m.preview(),
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
		Long:  "Everything after the flags is the comment text, flag-like words included, so --on, --panel, --block, --at, --version, --to, --json and -q must come before it.\n--panel and --version anchor a dashboard comment to a panel and to a version, and the server refuses them on an observation, an annotation or a proposal.\n--block anchors an observation comment to one of its blocks, from observe o show, and the server refuses it on any other target; without it the comment is about the whole observation.\n--at points the thread at one spot, as the anchor JSON object, for example {\"kind\":\"position\",\"pos\":{\"x\":0.4,\"y\":0.3}}, or as a short form: row:service=payments,span=authorize_card, time:2026-11-20T23:56Z, range:23:50Z..23:58Z, series:service=checkout or pos:0.4,0.3; with --block it points into that block, and an observation then gets a thread of its own beside its discussion.\nThe comment is for everyone and asks no agent; --to agent asks the agent the text mentions, else the thread's agent, else @observe: a dashboard thread starts a comment run, an observation thread starts a read-only ask that writes nothing on the page, and an annotation or a proposal thread starts none.\nThe comment is stored even when no run starts, and a refused run prints its code.\nAn observation has one discussion, so a second add on it without --at adds the comment to that discussion.",
		Args:  cobra.ArbitraryArgs,
		RunE:  a.runCommentsAdd,
	}
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().StringVar(&a.commentAdd.on, "on", "", "what to comment on, as <kind>:<id>")
	cmd.Flags().StringVar(&a.commentAdd.panel, "panel", "", "the panel of the dashboard to comment on")
	cmd.Flags().StringVar(&a.commentAdd.block, "block", "", "the block of the observation to comment on; the whole observation by default")
	cmd.Flags().IntVar(&a.commentAdd.version, "version", 0, "the dashboard version to comment on; the latest by default")
	cmd.Flags().StringVar(&a.commentAdd.at, "at", "", "the spot the thread points at, as the anchor JSON object or a short form such as pos:0.4,0.3")
	cmd.Flags().StringVar(&a.commentAdd.to, "to", "", "agent to ask an agent, or everyone (the default) for a plain comment")
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
	if err := setID(payload, "block_id", a.commentAdd.block); err != nil {
		return err
	}
	if cmd.Flags().Changed("version") {
		if a.commentAdd.version < 1 {
			return usagef("--version must be 1 or higher")
		}
		payload["version"] = a.commentAdd.version
	}
	if a.commentAdd.at != "" {
		at, err := anchor.Parse(a.commentAdd.at)
		if err != nil {
			return usagef("--at: %v", err)
		}
		payload["anchor"] = at
	}
	to, err := deliveryOf(a.commentAdd.to)
	if err != nil {
		return err
	}
	payload["to"] = to
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "threads", payload)
	if err != nil {
		return err
	}
	out := a.output()
	return render(out, data, func(created threadMessageEnvelope) error {
		out.line("Started thread %s on %s:%s", text(created.Thread.ID), text(created.Thread.Target.Kind), text(created.Thread.Target.ID))
		reportRun(out, created.Message)
		return nil
	})
}

func setID(payload map[string]any, key, value string) error {
	if value == "" {
		return nil
	}
	id, err := parseID(value)
	if err != nil {
		return err
	}
	payload[key] = id
	return nil
}

func reportRun(out *output, message threadMessage) {
	switch {
	case message.Run != nil:
		out.confirm("Started run %s, taken by %s", text(message.Run.ID), text(message.Run.AssignedKind))
	case message.RunRefusal != nil:
		out.warn("No run started (%s): %s", text(message.RunRefusal.Code), text(message.RunRefusal.Message))
	}
}

func (a *app) commentsReplyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reply [flags] <comment-id> <text...>",
		Short: "Reply to a comment thread",
		Long:  "Everything after the id is the reply text, flag-like words included, so flags such as --json, -q, --block, --reply-to or --to must come before the id.\nThe comment id can be any comment of the thread, on a dashboard, an observation, an annotation or a proposal.\nThe reply is for everyone and asks no agent; --to agent asks the agent the text mentions, else the thread's agent, else @observe, with the earlier messages of the thread as its context.\n--block anchors the reply to one block of an observation and --reply-to names the comment of the thread the reply answers, which the web app shows quoted.",
		Args:  cobra.ArbitraryArgs,
		RunE:  a.runCommentsReply,
	}
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().StringVar(&a.commentReply.block, "block", "", "the block of the observation the reply is about")
	cmd.Flags().StringVar(&a.commentReply.replyTo, "reply-to", "", "the comment of this thread the reply answers")
	cmd.Flags().StringVar(&a.commentReply.to, "to", "", "agent to ask an agent, or everyone (the default) for a plain reply")
	return cmd
}

func (a *app) runCommentsReply(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return usagef("%s takes a comment id and the reply text", cmd.CommandPath())
	}
	body := strings.TrimSpace(strings.Join(args[1:], " "))
	if body == "-h" || body == "--help" {
		return cmd.Help()
	}
	commentID, err := parseID(args[0])
	if err != nil {
		return err
	}
	payload := map[string]any{"body": body}
	if err := a.commentReply.addTo(payload); err != nil {
		return err
	}
	if body == "" {
		return usagef("reply needs text")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "threads/"+commentID+"/messages", payload)
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
	out.line("Replied to thread %s as %s", text(created.Thread.ID), text(created.Message.ID))
	reportRun(out, created.Message)
	return nil
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
	return render(out, data, func(edited threadMessageEnvelope) error {
		out.confirm("Edited comment %s", text(edited.Message.ID))
		return nil
	})
}

func (a *app) commentsDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <comment-id>",
		Short: "Delete one comment",
		Long:  "Deleting the first comment of a thread deletes the whole thread.\nThe author can delete their own comment while they can still comment on its target, unless an agent run wrote it, and anyone who manages the target's folder can delete any comment.",
		Args:  exactArgs(1, "one comment id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodDelete, "messages/"+id, nil, func(out *output, _ []byte) error {
				out.confirm("Deleted comment %s", text(id))
				return nil
			})
		},
	}
}

func (a *app) commentsWithdrawCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "withdraw <comment-id>",
		Short: "Withdraw an answer you shared, back to only you",
		Long:  "withdraw undoes observe share-answer: it deletes your shared question and its answer, or the whole thread when the share opened it, and the run and its answer are read again only by you and Admins.\nIt takes the id of the thread or of any of its comments, and only the person who asked can withdraw, while nobody else has written in the thread.\nOn an observation the answer's text that was written onto the page stays there as page content, while the run and its answer leave the observation's lists.\nYou can share the answer again later with observe share-answer.",
		Args:  exactArgs(1, "one comment id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodDelete, "threads/"+id+"/share", nil, func(out *output, _ []byte) error {
				out.confirm("Withdrew the answer shared in thread %s", text(id))
				return nil
			})
		},
	}
}

func (a *app) commentsApplyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "apply <comment-id> <finding-id>",
		Short: "Ask the agent to make the change an answer in a dashboard thread suggests",
		Long:  "apply takes a dashboard comment thread (its id or the id of any of its comments) and the id of an answer given in it, the id of the finding of the run that answered, which observe status <request-id> --json prints.\nIt posts \"Revise the dashboard: <suggested change>\" in the thread as you and starts a run that makes the change: a new version when you can edit the dashboard, a proposal when you can only comment.\nThe run is yours and goes to the agent that answered; applying the same answer again while that run is queued or running, or after it succeeded, starts nothing and prints the first run.",
		Args:  exactArgs(2, "a comment id and a finding id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			threadID, err := parseID(args[0])
			if err != nil {
				return err
			}
			findingID, err := parseID(args[1])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodPost, "threads/"+threadID+"/apply", map[string]any{"finding_id": findingID}, func(out *output, data []byte) error {
				applied, err := decode[appliedChange](data)
				if err != nil {
					return err
				}
				if applied.Replayed && applied.Message.Run != nil {
					out.line("Already applied by run %s", text(applied.Message.Run.ID))
					return nil
				}
				out.line("Asked to revise the dashboard in thread %s", text(applied.Thread.ID))
				reportRun(out, applied.Message)
				return nil
			})
		},
	}
}

type appliedChange struct {
	threadMessageEnvelope
	Replayed bool `json:"replayed"`
}

type mentionablePeople struct {
	People []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"people"`
	More bool `json:"more"`
}

func (a *app) commentsPeopleCommand() *cobra.Command {
	var on, search string
	var limit int
	cmd := &cobra.Command{
		Use:   "people --on <kind>:<id> [--search <text>]",
		Short: "List the people you can mention on a target",
		Long:  "people lists the people, other than you, who can see the target, ordered by name, with the mention to paste into a comment.\nA person who cannot see the target is never listed, and mentioning one is refused with invalid_mention.\n--search keeps the people whose name or email contains the text.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, err := parseOn(on)
			if err != nil {
				return err
			}
			query := url.Values{"target": {target}, "q": {search}, "limit": {strconv.Itoa(limit)}}
			data, err := a.fetch(cmd.Context(), "threads/mentionable", query)
			if err != nil {
				return err
			}
			out := a.output()
			return render(out, data, func(page mentionablePeople) error {
				rows := make([][]string, 0, len(page.People))
				for _, p := range page.People {
					rows = append(rows, []string{text(p.Name), text(p.Email), "@[" + text(p.Name) + "](user:" + p.ID + ")"})
				}
				out.table([]string{"NAME", "EMAIL", "MENTION"}, rows)
				if page.More {
					out.notice("more people match: narrow the list with --search or raise --limit")
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&on, "on", "", "the target, as <kind>:<id>")
	cmd.Flags().StringVar(&search, "search", "", "only people whose name or email contains this text")
	cmd.Flags().IntVar(&limit, "limit", 20, "people to list, at most 20")
	return cmd
}

type resolution struct {
	id                string
	resolvedByVersion *int
}

func (a *app) commentsResolveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <comment-id>",
		Short: "Resolve a comment thread",
		Long:  "The comment id can be any comment of the thread, on a dashboard, an observation, an annotation or a proposal.",
		Args:  exactArgs(1, "a comment id"),
		RunE:  a.runCommentsResolve,
	}
	cmd.Flags().IntVar(&a.commentResolve.version, "version", 0, "record the version that fixed the thread; dashboards only")
	return cmd
}

func (a *app) runCommentsResolve(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	versionGiven := cmd.Flags().Changed("version")
	if versionGiven && a.commentResolve.version < 1 {
		return usagef("--version must be 1 or higher")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	payload := map[string]any{}
	if versionGiven {
		payload["version"] = a.commentResolve.version
	}
	data, err := c.post(cmd.Context(), "threads/"+id+"/resolve", payload)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
	}
	envelope, err := decode[threadEnvelope](data)
	if err != nil {
		return err
	}
	resolved := resolution{envelope.Thread.ID, envelope.Thread.ResolvedByVersion}
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
	return render(out, data, func(reopened threadEnvelope) error {
		out.confirm("Reopened thread %s", text(reopened.Thread.ID))
		return nil
	})
}
