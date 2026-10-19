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
		{Tool{Name: nameListComments, Description: descListComments, InputSchema: json.RawMessage(schemaListComments), ReadOnly: true}, r.listComments},
		{Tool{Name: nameReplyToComment, Description: descReplyToComment, InputSchema: json.RawMessage(schemaReplyToComment)}, r.replyToComment},
		{Tool{Name: nameResolveComment, Description: descResolveComment, InputSchema: json.RawMessage(schemaResolveComment)}, r.resolveComment},
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
		Author:            c.Author.Name,
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

func parseCommentID(raw string) (pgtype.UUID, Result, bool) {
	id, err := db.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, invalid("comment_id must be a UUID from list_comments"), false
	}
	return id, Result{}, true
}

func checkCommentScope(ctx context.Context, dashboardID pgtype.UUID) (Result, bool, error) {
	run, scoped := RunFrom(ctx)
	if !scoped {
		return Result{}, true, nil
	}
	runID, err := validateRun(run)
	if err != nil {
		return Result{}, false, err
	}
	if run.Mode == ModeCreate {
		return invalid("this run creates a new dashboard; it has no comments yet"), false, nil
	}
	if dashboardID != runID {
		return invalid("this run revises dashboard %s; comments on other dashboards are out of scope", run.DashboardID), false, nil
	}
	return Result{}, true, nil
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
	res, ok, err = checkCommentScope(ctx, dashboardID)
	return commentID, dashboardID, res, ok, err
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
	if r.deps.Comments == nil {
		return Result{}, errCommentsNotConfigured
	}
	q, res, ok := parseCommentQuery(input)
	if !ok {
		return res, nil
	}
	res, ok, err := checkCommentScope(ctx, q.dashboardID)
	if err != nil || !ok {
		return res, err
	}
	var (
		items []commentOut
		next  string
	)
	for {
		list, cursor, err := r.deps.Comments.List(ctx, q.dashboardID, q.status, q.panelID, q.limit, q.cursor)
		if err != nil {
			return commentFailure(err)
		}
		items = make([]commentOut, 0, len(list))
		for _, c := range list {
			items = append(items, commentItem(c))
		}
		next = cursor
		kept, cut := fit(items, listBudget)
		if !cut || q.limit <= 1 {
			break
		}
		q.limit = max(len(kept), 1)
	}
	out := commentListOut{DashboardID: q.dashboardID.String(), Comments: items}
	if next != "" {
		out.NextCursor = &next
	}
	return success("Listed "+plural(len(items), "comment", "comments"), out), nil
}

func (r *Registry) replyToComment(ctx context.Context, input json.RawMessage) (Result, error) {
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

func (r *Registry) resolveComment(ctx context.Context, input json.RawMessage) (Result, error) {
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
