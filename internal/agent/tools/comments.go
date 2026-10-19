package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	defaultListComments = 50
	maxListComments     = 100
	maxCommentOutBytes  = 2000
	maxPanelFilterBytes = 64

	codeUnknownVersion = "unknown_version"
)

var errCommentsNotConfigured = errors.New("comments are not configured")

func (r *Registry) commentEntries() []entry {
	return []entry{
		{Tool{Name: nameListComments, Description: descListComments, InputSchema: json.RawMessage(schemaListComments), ReadOnly: true, MCPOnly: true}, r.listComments},
		{Tool{Name: nameReplyToComment, Description: descReplyToComment, InputSchema: json.RawMessage(schemaReplyToComment), MCPOnly: true}, r.replyToComment},
		{Tool{Name: nameResolveComment, Description: descResolveComment, InputSchema: json.RawMessage(schemaResolveComment), MCPOnly: true}, r.resolveComment},
	}
}

type commentOut struct {
	ID                string  `json:"id"`
	ParentID          *string `json:"parent_id"`
	PanelID           *string `json:"panel_id"`
	PanelPresent      bool    `json:"panel_present"`
	Version           int     `json:"version"`
	Status            string  `json:"status"`
	Author            string  `json:"author"`
	CreatedAt         string  `json:"created_at"`
	Body              string  `json:"body"`
	BodyTruncated     bool    `json:"body_truncated,omitempty"`
	ResolvedByVersion *int    `json:"resolved_by_version"`
}

type commentListOut struct {
	DashboardID string       `json:"dashboard_id"`
	Comments    []commentOut `json:"comments"`
	NextCursor  *string      `json:"next_cursor"`
}

type commentResultOut struct {
	Comment commentOut `json:"comment"`
}

type commentResolvedOut struct {
	Comment commentOut `json:"comment"`
	Changed bool       `json:"changed"`
}

func commentItem(c dashboards.Comment) commentOut {
	out := commentOut{
		ID:                c.ID,
		PanelPresent:      c.PanelPresent,
		Version:           c.Version,
		Status:            c.Status,
		Author:            authorLabel(c.Author),
		CreatedAt:         c.CreatedAt.UTC().Format(time.RFC3339),
		Body:              clip(c.Body, maxCommentOutBytes),
		BodyTruncated:     len(c.Body) > maxCommentOutBytes,
		ResolvedByVersion: c.ResolvedByVersion,
	}
	if c.ParentID != "" {
		out.ParentID = &c.ParentID
	}
	if c.PanelID != "" {
		out.PanelID = &c.PanelID
	}
	return out
}

func authorLabel(a dashboards.Author) string {
	if a.Name == "" {
		return a.Email
	}
	return a.Name
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
	var bad *dashboards.InvalidInputError
	switch {
	case errors.As(err, &bad):
		return invalid("%s", bad.Message), nil
	case errors.Is(err, dashboards.ErrInvalidCursor):
		return invalid("cursor is not valid"), nil
	case errors.Is(err, dashboards.ErrCommentNotFound), errors.Is(err, dashboards.ErrNotFound):
		return toolError(codeNotFound, "dashboard or comment not found"), nil
	}
	return Result{}, err
}

func (r *Registry) commentTarget(ctx context.Context, raw string) (pgtype.UUID, pgtype.UUID, Result, bool, error) {
	commentID, res, ok := parseCommentID(raw)
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, res, false, nil
	}
	dashboardID, err := r.deps.Comments.DashboardOf(ctx, commentID)
	if err != nil {
		res, err := commentFailure(err)
		return pgtype.UUID{}, pgtype.UUID{}, res, false, err
	}
	return commentID, dashboardID, Result{}, true, nil
}

type commentQuery struct {
	dashboardID pgtype.UUID
	status      string
	panelID     string
	limit       int
	cursor      string
}

func parseCommentQuery(input json.RawMessage) (commentQuery, Result, bool) {
	var in struct {
		DashboardID string  `json:"dashboard_id"`
		Status      string  `json:"status"`
		PanelID     *string `json:"panel_id"`
		Limit       *int    `json:"limit"`
		Cursor      string  `json:"cursor"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return commentQuery{}, res, false
	}
	dashboardID, res, ok := parseDashboardID(in.DashboardID)
	if !ok {
		return commentQuery{}, res, false
	}
	limit, res, ok := boundedInt(in.Limit, defaultListComments, 1, maxListComments, "limit")
	if !ok {
		return commentQuery{}, res, false
	}
	q := commentQuery{dashboardID: dashboardID, status: in.Status, limit: limit, cursor: in.Cursor}
	if q.status == "" {
		q.status = dashboards.CommentOpen
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

func (r *Registry) listComments(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, ok := rejectAgentRun(ctx); !ok {
		return res, nil
	}
	if r.deps.Comments == nil {
		return Result{}, errCommentsNotConfigured
	}
	q, res, ok := parseCommentQuery(input)
	if !ok {
		return res, nil
	}
	for {
		list, cursor, err := r.deps.Comments.List(ctx, q.dashboardID, q.status, q.panelID, q.limit, q.cursor)
		if err != nil {
			return commentFailure(err)
		}
		out, kept, cut := commentPage(q.dashboardID.String(), list, cursor)
		if !cut {
			return success("Listed "+plural(len(out.Comments), "comment", "comments"), out), nil
		}
		q.limit = max(kept, 1)
	}
}

func commentPage(dashboardID string, list []dashboards.Comment, next string) (commentListOut, int, bool) {
	out := commentListOut{DashboardID: dashboardID, Comments: make([]commentOut, 0, len(list))}
	if next != "" {
		out.NextCursor = &next
	}
	envelope, err := json.Marshal(out)
	if err != nil {
		return out, 0, false
	}
	budget := listBudget - len(envelope)
	for _, c := range list {
		out.Comments = append(out.Comments, commentItem(c))
	}
	kept, cut := fit(out.Comments, budget)
	if !cut {
		return out, len(kept), false
	}
	if len(list) == 1 {
		out.Comments[0] = commentWithin(list[0], budget)
		return out, 1, false
	}
	return out, len(kept), true
}

func commentWithin(c dashboards.Comment, budget int) commentOut {
	out := commentItem(c)
	n := min(len(c.Body), maxCommentOutBytes)
	for n > 0 {
		b, err := json.Marshal(out)
		if err != nil || len(b)+1 <= budget {
			break
		}
		n = max(n-(len(b)+1-budget), 0)
		out.Body, out.BodyTruncated = clip(c.Body, n), true
	}
	return out
}

func (r *Registry) replyToComment(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, ok := rejectAgentRun(ctx); !ok {
		return res, nil
	}
	if r.deps.Comments == nil {
		return Result{}, errCommentsNotConfigured
	}
	principal, _, _, err := r.saveRun(ctx)
	if err != nil {
		return Result{}, err
	}
	var in struct {
		CommentID string `json:"comment_id"`
		Body      string `json:"body"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	commentID, dashboardID, res, ok, err := r.commentTarget(ctx, in.CommentID)
	if err != nil || !ok {
		return res, err
	}
	comment, err := r.deps.Comments.Create(ctx, principal, dashboardID, dashboards.NewComment{ParentID: commentID, Body: in.Body})
	if err != nil {
		return commentFailure(err)
	}
	return success("Replied to a comment", commentResultOut{Comment: commentItem(comment)}), nil
}

func (r *Registry) checkThreadVersion(ctx context.Context, dashboardID, commentID pgtype.UUID, version *int) (Result, bool, error) {
	if version == nil {
		return Result{}, true, nil
	}
	threadVersion, err := r.deps.Comments.ThreadVersion(ctx, dashboardID, commentID)
	if err != nil {
		res, err := commentFailure(err)
		return res, false, err
	}
	if *version < threadVersion {
		return invalid("version must be at least %d, the version the thread is on", threadVersion), false, nil
	}
	return Result{}, true, nil
}

func (r *Registry) resolveComment(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, ok := rejectAgentRun(ctx); !ok {
		return res, nil
	}
	if r.deps.Comments == nil {
		return Result{}, errCommentsNotConfigured
	}
	principal, _, _, err := r.saveRun(ctx)
	if err != nil {
		return Result{}, err
	}
	var in struct {
		CommentID string `json:"comment_id"`
		Version   *int   `json:"version"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if in.Version != nil {
		if _, res, ok := boundedInt(in.Version, 0, 1, math.MaxInt32, "version"); !ok {
			return res, nil
		}
	}
	commentID, dashboardID, res, ok, err := r.commentTarget(ctx, in.CommentID)
	if err != nil || !ok {
		return res, err
	}
	if res, ok, err := r.checkThreadVersion(ctx, dashboardID, commentID, in.Version); err != nil || !ok {
		return res, err
	}
	comment, changed, err := r.deps.Comments.SetStatus(ctx, principal, dashboardID, commentID, dashboards.CommentResolved, in.Version)
	if errors.Is(err, dashboards.ErrUnknownVersion) && in.Version != nil {
		return toolError(codeUnknownVersion, fmt.Sprintf("version %d is not a version of this dashboard", *in.Version)), nil
	}
	if err != nil {
		return commentFailure(err)
	}
	step := "Resolved a comment"
	if !changed {
		step = "Comment was already resolved"
	}
	return success(step, commentResolvedOut{Comment: commentItem(comment), Changed: changed}), nil
}
