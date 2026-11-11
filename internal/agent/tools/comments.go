package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/threads"
)

const (
	defaultListComments   = 50
	maxListComments       = 100
	maxCommentOutBytes    = 2000
	maxMentionNameBytes   = 48
	maxPanelFilterBytes   = 64
	codeUnknownVersion    = "unknown_version"
	codeUnknownPanel      = "unknown_panel"
	codeNotAuthor         = "not_author"
	commentNotFoundNotice = "comment or target not found"
)

var errCommentsNotConfigured = errors.New("comments are not configured")

func init() { registerEntries((*Registry).commentEntries) }

func (r *Registry) commentEntries() []entry {
	return []entry{
		{Tool{Name: nameCreateComment, Description: descCreateComment, InputSchema: json.RawMessage(schemaCreateComment), MCPOnly: true, MinRole: auth.RoleCommenter}, r.createComment},
		{Tool{Name: nameEditComment, Description: descEditComment, InputSchema: json.RawMessage(schemaEditComment), MCPOnly: true, MinRole: auth.RoleCommenter}, r.editComment},
		{Tool{Name: nameListComments, Description: descListComments, InputSchema: json.RawMessage(schemaListComments), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.listComments},
		{Tool{Name: nameReopenComment, Description: descReopenComment, InputSchema: json.RawMessage(schemaReopenComment), MCPOnly: true, MinRole: auth.RoleCommenter}, r.reopenComment},
		{Tool{Name: nameReplyToComment, Description: descReplyToComment, InputSchema: json.RawMessage(schemaReplyToComment), MCPOnly: true, MinRole: auth.RoleCommenter}, r.replyToComment},
		{Tool{Name: nameResolveComment, Description: descResolveComment, InputSchema: json.RawMessage(schemaResolveComment), MCPOnly: true, MinRole: auth.RoleCommenter}, r.resolveComment},
	}
}

type mentionOut struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type commentOut struct {
	ID                string       `json:"id"`
	ParentID          *string      `json:"parent_id"`
	Target            string       `json:"target,omitempty"`
	PanelID           *string      `json:"panel_id"`
	PanelPresent      bool         `json:"panel_present"`
	Version           *int         `json:"version"`
	Status            string       `json:"status"`
	Author            string       `json:"author"`
	CreatedAt         string       `json:"created_at"`
	EditedAt          *string      `json:"edited_at"`
	Body              string       `json:"body"`
	BodyTruncated     bool         `json:"body_truncated,omitempty"`
	ResolvedByVersion *int         `json:"resolved_by_version"`
	Mentions          []mentionOut `json:"mentions"`
}

type commentListOut struct {
	Target      string       `json:"target,omitempty"`
	DashboardID string       `json:"dashboard_id,omitempty"`
	Comments    []commentOut `json:"comments"`
	NextCursor  *string      `json:"next_cursor"`
}

type commentResultOut struct {
	Comment commentOut `json:"comment"`
}

type commentChangedOut struct {
	Comment commentOut `json:"comment"`
	Changed bool       `json:"changed"`
}

type commentRef struct {
	thread  threads.Thread
	message threads.Message
}

func commentItem(ref commentRef) commentOut {
	t, m := ref.thread, ref.message
	out := commentOut{
		ID:           m.ID,
		PanelPresent: t.PanelPresent,
		Version:      t.Version,
		Status:       t.Status,
		Author:       authorLabel(m),
		CreatedAt:    formatCommentTime(m.CreatedAt),
		Body:         clip(m.Body, maxCommentOutBytes),
		Mentions:     mentionItems(m.Mentions),
	}
	out.BodyTruncated = len(m.Body) > maxCommentOutBytes
	if m.EditedAt != nil {
		edited := formatCommentTime(*m.EditedAt)
		out.EditedAt = &edited
	}
	if m.ID != t.ID {
		out.ParentID = &t.ID
	} else {
		out.ResolvedByVersion = t.ResolvedByVersion
	}
	if t.PanelID != "" {
		out.PanelID = &t.PanelID
	}
	return out
}

func targetedCommentItem(ref commentRef) commentOut {
	out := commentItem(ref)
	out.Target = ref.thread.Target.String()
	return out
}

func formatCommentTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func mentionItems(mentions []threads.Mention) []mentionOut {
	out := make([]mentionOut, len(mentions))
	for i, m := range mentions {
		out[i] = mentionOut{Kind: m.Kind(), ID: m.RefID(), Name: clip(m.Name, maxMentionNameBytes)}
	}
	return out
}

func authorLabel(m threads.Message) string {
	if m.Author.Name == "" {
		return m.AuthorEmail
	}
	return m.Author.Name
}

func parseCommentID(raw string) (pgtype.UUID, Result, bool) {
	id, err := db.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, invalid("comment_id must be a UUID from list_comments"), false
	}
	return id, Result{}, true
}

func rejectAgentRun(ctx context.Context) (Result, bool) {
	if _, scoped := RunFrom(ctx); scoped {
		return invalid("comment tools are for MCP clients; the server answers and resolves threads after a run"), false
	}
	return Result{}, true
}

func commentFailure(err error) (Result, error) {
	var bad *threads.InvalidInputError
	switch {
	case errors.As(err, &bad):
		return invalid("%s", bad.Message), nil
	case errors.Is(err, dashboards.ErrInvalidCursor):
		return invalid("cursor is not valid"), nil
	case errors.Is(err, threads.ErrNotFound), errors.Is(err, threads.ErrTargetNotFound), errors.Is(err, dashboards.ErrNotFound), errors.Is(err, folders.ErrNoAccess):
		return toolError(codeNotFound, commentNotFoundNotice), nil
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, roleDeniedMessage), nil
	case errors.Is(err, threads.ErrNotAuthor):
		return toolError(codeNotAuthor, err.Error()), nil
	case errors.Is(err, threads.ErrUnknownPanel):
		return toolError(codeUnknownPanel, err.Error()), nil
	case errors.Is(err, threads.ErrUnknownVersion):
		return toolError(codeUnknownVersion, err.Error()), nil
	}
	return Result{}, err
}

func versionFailure(err error) (Result, error) {
	var bad *threads.InvalidInputError
	if errors.As(err, &bad) && bad.Code == threads.CodeInvalidVersion {
		return invalid("%s", strings.ReplaceAll(bad.Message, "resolved_by_version", "version")), nil
	}
	return commentFailure(err)
}

type commentQuery struct {
	target   threads.Target
	byTarget bool
	status   string
	panelID  string
	limit    int
	cursor   string
}

func parseCommentQuery(input json.RawMessage) (commentQuery, Result, bool) {
	var in struct {
		Target      string  `json:"target"`
		DashboardID string  `json:"dashboard_id"`
		Status      string  `json:"status"`
		PanelID     *string `json:"panel_id"`
		Limit       *int    `json:"limit"`
		Cursor      string  `json:"cursor"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return commentQuery{}, res, false
	}
	q := commentQuery{byTarget: in.Target != "", status: in.Status, cursor: in.Cursor}
	switch {
	case (in.Target == "") == (in.DashboardID == ""):
		return commentQuery{}, invalid("give exactly one of target and dashboard_id"), false
	case q.byTarget:
		target, err := threads.ParseTarget(in.Target)
		if err != nil {
			res, _ := commentFailure(err)
			return commentQuery{}, res, false
		}
		q.target = target
	default:
		id, res, ok := parseDashboardID(in.DashboardID)
		if !ok {
			return commentQuery{}, res, false
		}
		q.target = threads.Target{Kind: threads.KindDashboard, ID: id}
	}
	limit, res, ok := boundedInt(in.Limit, defaultListComments, 1, maxListComments, "limit")
	if !ok {
		return commentQuery{}, res, false
	}
	q.limit = limit
	if q.status == "" {
		q.status = threads.StatusOpen
	}
	if in.PanelID != nil {
		if *in.PanelID == "" {
			return commentQuery{}, invalid("panel_id must be at least 1 character"), false
		}
		if res, ok := checkLength("panel_id", *in.PanelID, maxPanelFilterBytes); !ok {
			return commentQuery{}, res, false
		}
		q.panelID = *in.PanelID
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return commentQuery{}, res, false
	}
	return q, Result{}, true
}

func (r *Registry) commentCaller(ctx context.Context) (auth.Principal, Result, bool, error) {
	if res, ok := rejectAgentRun(ctx); !ok {
		return auth.Principal{}, res, false, nil
	}
	if r.deps.Threads == nil {
		return auth.Principal{}, Result{}, false, errCommentsNotConfigured
	}
	principal, _, _, err := r.saveRun(ctx)
	return principal, Result{}, err == nil, err
}

func (r *Registry) listComments(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, res, ok, err := r.commentCaller(ctx)
	if err != nil || !ok {
		return res, err
	}
	q, res, ok := parseCommentQuery(input)
	if !ok {
		return res, nil
	}
	for {
		page, err := r.deps.Threads.List(ctx, principal, threads.ListQuery{Target: q.target, Status: q.status, PanelID: q.panelID, Limit: q.limit, Cursor: q.cursor})
		if err != nil {
			return commentFailure(err)
		}
		out, kept, cut := commentPage(q, commentRefs(page), page.Next)
		if !cut {
			return success("Listed "+plural(len(out.Comments), "comment", "comments"), out), nil
		}
		q.limit = max(kept, 1)
	}
}

func commentRefs(page threads.Page) []commentRef {
	byID := make(map[string]threads.Thread, len(page.Threads))
	for _, t := range page.Threads {
		byID[t.ID] = t
	}
	refs := make([]commentRef, len(page.Messages))
	for i, m := range page.Messages {
		refs[i] = commentRef{thread: byID[m.ThreadID], message: m}
	}
	return refs
}

func commentPage(q commentQuery, refs []commentRef, next string) (commentListOut, int, bool) {
	out := commentListOut{Comments: make([]commentOut, 0, len(refs))}
	if q.target.Kind == threads.KindDashboard {
		out.DashboardID = q.target.ID.String()
	}
	if q.byTarget {
		out.Target = q.target.String()
	}
	if next != "" {
		out.NextCursor = &next
	}
	envelope, err := json.Marshal(out)
	if err != nil {
		return out, 0, false
	}
	budget := listBudget - len(envelope)
	for _, ref := range refs {
		out.Comments = append(out.Comments, commentItem(ref))
	}
	kept, cut := fit(out.Comments, budget)
	if !cut {
		return out, len(kept), false
	}
	if len(refs) == 1 {
		out.Comments[0] = commentWithin(refs[0], budget)
		return out, 1, false
	}
	return out, len(kept), true
}

func commentWithin(ref commentRef, budget int) commentOut {
	out := commentItem(ref)
	body := ref.message.Body
	n := min(len(body), maxCommentOutBytes)
	for n > 0 {
		b, err := json.Marshal(out)
		if err != nil || len(b)+1 <= budget {
			break
		}
		n = max(n-(len(b)+1-budget), 0)
		out.Body, out.BodyTruncated = clip(body, n), true
	}
	return out
}

func (r *Registry) createComment(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, res, ok, err := r.commentCaller(ctx)
	if err != nil || !ok {
		return res, err
	}
	var in struct {
		Target  string  `json:"target"`
		Body    string  `json:"body"`
		PanelID *string `json:"panel_id"`
		Version *int    `json:"version"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	target, err := threads.ParseTarget(in.Target)
	if err != nil {
		return commentFailure(err)
	}
	thread, message, err := r.deps.Threads.Open(ctx, principal, threads.OpenInput{Target: target, Version: in.Version, PanelID: in.PanelID, Body: in.Body}, nil)
	if err != nil {
		return commentFailure(err)
	}
	return success("Opened a comment", commentResultOut{Comment: targetedCommentItem(commentRef{thread, message})}), nil
}

func (r *Registry) replyToComment(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, res, ok, err := r.commentCaller(ctx)
	if err != nil || !ok {
		return res, err
	}
	var in struct {
		CommentID string `json:"comment_id"`
		Body      string `json:"body"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	commentID, res, ok := parseCommentID(in.CommentID)
	if !ok {
		return res, nil
	}
	thread, message, err := r.deps.Threads.Reply(ctx, principal, commentID, in.Body)
	if err != nil {
		return commentFailure(err)
	}
	return success("Replied to a comment", commentResultOut{Comment: targetedCommentItem(commentRef{thread, message})}), nil
}

func (r *Registry) editComment(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, res, ok, err := r.commentCaller(ctx)
	if err != nil || !ok {
		return res, err
	}
	var in struct {
		CommentID string `json:"comment_id"`
		Body      string `json:"body"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	commentID, res, ok := parseCommentID(in.CommentID)
	if !ok {
		return res, nil
	}
	thread, message, changed, err := r.deps.Threads.Edit(ctx, principal, commentID, in.Body)
	if err != nil {
		return commentFailure(err)
	}
	step := "Edited a comment"
	if !changed {
		step = "Comment already had that text"
	}
	return success(step, commentChangedOut{Comment: targetedCommentItem(commentRef{thread, message}), Changed: changed}), nil
}

func (r *Registry) resolveComment(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, res, ok, err := r.commentCaller(ctx)
	if err != nil || !ok {
		return res, err
	}
	var in struct {
		CommentID string `json:"comment_id"`
		Version   *int   `json:"version"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	return r.changeStatus(ctx, principal, in.CommentID, threads.StatusResolved, in.Version)
}

func (r *Registry) reopenComment(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, res, ok, err := r.commentCaller(ctx)
	if err != nil || !ok {
		return res, err
	}
	var in struct {
		CommentID string `json:"comment_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	return r.changeStatus(ctx, principal, in.CommentID, threads.StatusOpen, nil)
}

func (r *Registry) changeStatus(ctx context.Context, principal auth.Principal, rawID, status string, version *int) (Result, error) {
	commentID, res, ok := parseCommentID(rawID)
	if !ok {
		return res, nil
	}
	thread, changed, err := r.deps.Threads.SetStatus(ctx, principal, commentID, status, version)
	if err != nil {
		return versionFailure(err)
	}
	threadID, err := db.ParseUUID(thread.ID)
	if err != nil {
		return Result{}, err
	}
	first, err := r.deps.Threads.ReadMessage(ctx, principal, threadID)
	if err != nil {
		return commentFailure(err)
	}
	step := "Resolved a comment"
	if status == threads.StatusOpen {
		step = "Reopened a comment"
	}
	if !changed {
		step = "Comment was already " + status
	}
	return success(step, commentChangedOut{Comment: targetedCommentItem(commentRef{thread, first}), Changed: changed}), nil
}
