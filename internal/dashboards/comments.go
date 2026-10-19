package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	CommentOpen     = "open"
	CommentResolved = "resolved"
	CommentAll      = "all"

	maxCommentRunes = revisions.MaxPromptRunes
	maxPanelIDRunes = revisions.MaxPanelIDRunes
)

var (
	ErrCommentNotFound = errors.New("comment not found")
	ErrUnknownPanel    = errors.New("panel does not exist in that version")
	ErrUnknownVersion  = errors.New("version does not exist on this dashboard")
)

type InvalidInputError struct {
	Message string
}

func (e *InvalidInputError) Error() string {
	return e.Message
}

type CommentRevision struct {
	ID     string
	Status string
}

type Comment struct {
	ID                string
	DashboardID       string
	ParentID          string
	Version           int
	PanelID           string
	Body              string
	Status            string
	Author            Author
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ResolvedAt        *time.Time
	ResolvedBy        *Author
	ResolvedByVersion *int
	PanelPresent      bool
	Revision          *CommentRevision
}

type NewComment struct {
	Version         int
	PanelID         *string
	ParentID        pgtype.UUID
	Body            string
	RequestRevision bool
}

type Comments struct {
	db         *db.DB
	dashboards *Service
	revisions  *revisions.Service
	panelSets  *panelCache
}

func NewComments(database *db.DB, dashboards *Service, revisions *revisions.Service) *Comments {
	return &Comments{db: database, dashboards: dashboards, revisions: revisions, panelSets: newPanelCache()}
}

type commentAnchor struct {
	version      int
	panelID      string
	parentID     pgtype.UUID
	threadStatus string
}

func (cm *Comments) anchor(ctx context.Context, dashboardID pgtype.UUID, in NewComment) (commentAnchor, error) {
	if in.ParentID.Valid {
		if in.RequestRevision {
			return commentAnchor{}, &InvalidInputError{Message: "a reply cannot request a revision"}
		}
		root, err := cm.threadRoot(ctx, dashboardID, in.ParentID)
		if err != nil {
			return commentAnchor{}, err
		}
		return commentAnchor{version: int(root.Version), panelID: root.PanelID.String, parentID: root.ID, threadStatus: root.Status}, nil
	}
	if !validVersion(in.Version) {
		return commentAnchor{}, &InvalidInputError{Message: "version must be a positive 32-bit integer"}
	}
	panelID := ""
	if in.PanelID != nil {
		if err := checkPanelID(*in.PanelID); err != nil {
			return commentAnchor{}, err
		}
		panelID = *in.PanelID
	}
	return commentAnchor{version: in.Version, panelID: panelID, threadStatus: CommentOpen}, nil
}

func (cm *Comments) anchorPanels(ctx context.Context, dashboardID pgtype.UUID, at commentAnchor, latest int, reply bool) (map[string]struct{}, error) {
	if at.panelID == "" {
		return nil, nil
	}
	if !reply {
		onVersion, err := cm.panels(ctx, dashboardID, at.version)
		if err != nil {
			return nil, err
		}
		if _, ok := onVersion[at.panelID]; !ok {
			return nil, ErrUnknownPanel
		}
	}
	return cm.panels(ctx, dashboardID, latest)
}

func (cm *Comments) Create(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID, in NewComment) (Comment, error) {
	body, err := checkCommentBody(in.Body)
	if err != nil {
		return Comment{}, err
	}
	authorID, err := db.ParseUUID(by.ID)
	if err != nil {
		return Comment{}, fmt.Errorf("author id: %w", err)
	}
	at, err := cm.anchor(ctx, dashboardID, in)
	if err != nil {
		return Comment{}, err
	}
	latest, err := cm.dashboards.LatestVersion(ctx, dashboardID)
	if err != nil {
		return Comment{}, err
	}
	if at.version > latest {
		return Comment{}, ErrNotFound
	}
	latestPanels, err := cm.anchorPanels(ctx, dashboardID, at, latest, in.ParentID.Valid)
	if err != nil {
		return Comment{}, err
	}
	author := authorOf(by)
	var row dbsqlc.DashboardComment
	var requested *CommentRevision
	threadStatus := at.threadStatus
	err = cm.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if at.parentID.Valid {
			root, err := q.GetCommentForShare(ctx, dbsqlc.GetCommentForShareParams{ID: at.parentID, DashboardID: dashboardID})
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCommentNotFound
			}
			if err != nil {
				return fmt.Errorf("lock thread root: %w", err)
			}
			threadStatus = root.Status
		}
		var err error
		row, err = q.CreateComment(ctx, dbsqlc.CreateCommentParams{
			DashboardID: dashboardID,
			Version:     httpapi.Int32(at.version),
			PanelID:     pgtype.Text{String: at.panelID, Valid: at.panelID != ""},
			ParentID:    at.parentID,
			Body:        body,
			AuthorKind:  author.Kind,
			AuthorID:    authorID,
			AuthorName:  author.Name,
			AuthorEmail: pgtype.Text{String: author.Email, Valid: author.Email != ""},
		})
		if err != nil {
			return fmt.Errorf("create comment: %w", err)
		}
		requested, err = cm.announce(ctx, q, by, row, threadStatus, in.RequestRevision)
		return err
	})
	if err != nil {
		return Comment{}, err
	}
	comment := commentFromRow(row, threadStatus, latestPanels)
	comment.Revision = requested
	return comment, nil
}

func (cm *Comments) threadRoot(ctx context.Context, dashboardID, commentID pgtype.UUID) (dbsqlc.DashboardComment, error) {
	row, err := cm.getComment(ctx, dashboardID, commentID)
	if err != nil {
		return dbsqlc.DashboardComment{}, err
	}
	if !row.ParentID.Valid {
		return row, nil
	}
	root, err := cm.getComment(ctx, dashboardID, row.ParentID)
	if err != nil {
		return dbsqlc.DashboardComment{}, err
	}
	if root.ParentID.Valid {
		return dbsqlc.DashboardComment{}, fmt.Errorf("comment %s: parent %s is itself a reply", row.ID, root.ID)
	}
	return root, nil
}

func (cm *Comments) getComment(ctx context.Context, dashboardID, commentID pgtype.UUID) (dbsqlc.DashboardComment, error) {
	row, err := cm.db.Queries().GetComment(ctx, dbsqlc.GetCommentParams{ID: commentID, DashboardID: dashboardID})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.DashboardComment{}, ErrCommentNotFound
	}
	if err != nil {
		return dbsqlc.DashboardComment{}, fmt.Errorf("get comment: %w", err)
	}
	return row, nil
}

func (cm *Comments) announce(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, row dbsqlc.DashboardComment, threadStatus string, requestRevision bool) (*CommentRevision, error) {
	if err := notifyComment(ctx, q, events.TypeCommentCreated, row, threadStatus, authorOf(by)); err != nil {
		return nil, err
	}
	if row.ParentID.Valid || !requestRevision || !cm.revisions.Available() {
		return nil, nil
	}
	queued, err := cm.revisions.Enqueue(ctx, q, revisions.EnqueueParams{
		DashboardID: row.DashboardID,
		Source:      revisions.SourceComment,
		CommentID:   row.ID,
		Prompt:      row.Body,
		PanelID:     row.PanelID.String,
		BaseVersion: int(row.Version),
		By:          by,
	})
	if err != nil {
		return nil, err
	}
	return &CommentRevision{ID: queued.ID.String(), Status: queued.Status}, nil
}

func (cm *Comments) List(ctx context.Context, dashboardID pgtype.UUID, status, panelID string, limit int, cursor string) ([]Comment, string, error) {
	params := dbsqlc.ListCommentsParams{DashboardID: dashboardID, RowLimit: httpapi.Int32(limit + 1)}
	switch status {
	case CommentOpen, CommentResolved:
		params.Status = pgtype.Text{String: status, Valid: true}
	case CommentAll, "":
	default:
		return nil, "", &InvalidInputError{Message: "status must be open, resolved or all"}
	}
	if panelID != "" {
		if err := checkPanelID(panelID); err != nil {
			return nil, "", err
		}
		params.PanelID = pgtype.Text{String: panelID, Valid: true}
	}
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	latest, err := cm.dashboards.LatestVersion(ctx, dashboardID)
	if err != nil {
		return nil, "", err
	}
	rows, err := cm.db.Queries().ListComments(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("list comments: %w", err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = httpapi.EncodeTimeCursor(rows[limit-1].DashboardComment.CreatedAt.Time, rows[limit-1].DashboardComment.ID.String())
	}
	items := make([]Comment, 0, len(rows))
	if len(rows) == 0 {
		return items, next, nil
	}
	panels, err := cm.panels(ctx, dashboardID, latest)
	if err != nil {
		return nil, "", err
	}
	for _, r := range rows {
		items = append(items, commentFromRow(r.DashboardComment, r.ThreadStatus, panels))
	}
	if err := cm.attachRevisions(ctx, items); err != nil {
		return nil, "", err
	}
	return items, next, nil
}

func checkResolvedByVersion(status string, resolvedByVersion *int) error {
	if status != CommentOpen && status != CommentResolved {
		return &InvalidInputError{Message: "status must be open or resolved"}
	}
	if resolvedByVersion == nil {
		return nil
	}
	if status != CommentResolved {
		return &InvalidInputError{Message: "resolved_by_version is only allowed with status resolved"}
	}
	if !validVersion(*resolvedByVersion) {
		return &InvalidInputError{Message: "resolved_by_version must be a positive 32-bit integer"}
	}
	return nil
}

func (cm *Comments) DashboardOf(ctx context.Context, commentID pgtype.UUID) (pgtype.UUID, error) {
	id, err := cm.db.Queries().GetCommentDashboard(ctx, commentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, ErrCommentNotFound
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("get comment dashboard: %w", err)
	}
	return id, nil
}

func (cm *Comments) ThreadVersion(ctx context.Context, dashboardID, commentID pgtype.UUID) (int, error) {
	root, err := cm.threadRoot(ctx, dashboardID, commentID)
	if err != nil {
		return 0, err
	}
	return int(root.Version), nil
}

func (cm *Comments) SetStatus(ctx context.Context, by auth.Principal, dashboardID, commentID pgtype.UUID, status string, resolvedByVersion *int) (Comment, bool, error) {
	if err := checkResolvedByVersion(status, resolvedByVersion); err != nil {
		return Comment{}, false, err
	}
	actorID, err := db.ParseUUID(by.ID)
	if err != nil {
		return Comment{}, false, fmt.Errorf("actor id: %w", err)
	}
	actor := authorOf(by)
	target, err := cm.threadRoot(ctx, dashboardID, commentID)
	if err != nil {
		return Comment{}, false, err
	}
	latest, err := cm.dashboards.LatestVersion(ctx, dashboardID)
	if err != nil {
		return Comment{}, false, err
	}
	if resolvedByVersion != nil {
		if *resolvedByVersion > latest {
			return Comment{}, false, ErrUnknownVersion
		}
		if *resolvedByVersion < int(target.Version) {
			return Comment{}, false, &InvalidInputError{Message: fmt.Sprintf("resolved_by_version must be at least the version the thread is on (%d)", target.Version)}
		}
	}
	params := dbsqlc.SetCommentStatusParams{ID: target.ID, DashboardID: dashboardID, Status: status}
	if status == CommentResolved {
		params.ResolvedByKind = pgtype.Text{String: actor.Kind, Valid: true}
		params.ResolvedByID = actorID
		params.ResolvedByName = pgtype.Text{String: actor.Name, Valid: true}
		if resolvedByVersion != nil {
			params.ResolvedByVersion = pgtype.Int4{Int32: httpapi.Int32(*resolvedByVersion), Valid: true}
		}
	}
	panels, err := cm.panels(ctx, dashboardID, latest)
	if err != nil {
		return Comment{}, false, err
	}
	var row dbsqlc.DashboardComment
	var changed bool
	err = cm.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		row, changed, err = cm.applyStatus(ctx, q, params, actor)
		return err
	})
	if err != nil {
		return Comment{}, false, err
	}
	comments := []Comment{commentFromRow(row, row.Status, panels)}
	if err := cm.attachRevisions(ctx, comments); err != nil {
		return Comment{}, false, err
	}
	return comments[0], changed, nil
}

func (cm *Comments) applyStatus(ctx context.Context, q *dbsqlc.Queries, params dbsqlc.SetCommentStatusParams, actor Author) (dbsqlc.DashboardComment, bool, error) {
	updated, err := q.SetCommentStatus(ctx, params)
	switch {
	case err == nil:
		return updated, true, notifyComment(ctx, q, events.TypeCommentUpdated, updated, updated.Status, actor)
	case errors.Is(err, pgx.ErrNoRows):
		row, err := q.GetComment(ctx, dbsqlc.GetCommentParams{ID: params.ID, DashboardID: params.DashboardID})
		if errors.Is(err, pgx.ErrNoRows) {
			return row, false, ErrCommentNotFound
		}
		if err != nil {
			return row, false, fmt.Errorf("get comment: %w", err)
		}
		return row, false, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "dashboard_comments_resolved_by_version_fkey" {
		return updated, false, ErrUnknownVersion
	}
	return updated, false, fmt.Errorf("set comment status: %w", err)
}

func (cm *Comments) attachRevisions(ctx context.Context, comments []Comment) error {
	ids := make([]pgtype.UUID, 0, len(comments))
	for _, c := range comments {
		id, err := db.ParseUUID(c.ID)
		if err != nil {
			return fmt.Errorf("comment id: %w", err)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := cm.db.Queries().LatestAgentRequestsForComments(ctx, ids)
	if err != nil {
		return fmt.Errorf("latest agent requests for comments: %w", err)
	}
	byComment := make(map[string]*CommentRevision, len(rows))
	for _, r := range rows {
		byComment[r.CommentID.String()] = &CommentRevision{ID: r.ID.String(), Status: r.Status}
	}
	for i := range comments {
		comments[i].Revision = byComment[comments[i].ID]
	}
	return nil
}

func (cm *Comments) panels(ctx context.Context, dashboardID pgtype.UUID, version int) (map[string]struct{}, error) {
	key := panelKey{dashboard: dashboardID, version: version}
	if set, ok := cm.panelSets.get(key); ok {
		return set, nil
	}
	d, err := cm.dashboards.VersionSpec(ctx, dashboardID, version)
	if err != nil {
		return nil, err
	}
	set := panelSet(d)
	cm.panelSets.put(key, set)
	return set, nil
}

func panelSet(d *spec.Dashboard) map[string]struct{} {
	entries := d.AllPanels()
	set := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		set[e.Panel.ID] = struct{}{}
	}
	return set
}

func checkPanelID(panelID string) error {
	if err := revisions.ValidPanelID(panelID); err != nil {
		return &InvalidInputError{Message: err.Error()}
	}
	return nil
}

func checkCommentBody(body string) (string, error) {
	cleaned, ok := revisions.CleanPrompt(body)
	if !ok {
		return "", &InvalidInputError{Message: fmt.Sprintf("body must be 1 to %d characters without control or bidirectional override characters", maxCommentRunes)}
	}
	return cleaned, nil
}

// pg_notify refuses payloads of 8000 bytes or more. The payload holds no comment text; with the panel id
// at 64 characters and the author name cut to 256, even all-escaped characters (6 bytes each) stay under 2500 bytes.
func commentEventPayload(eventType string, row dbsqlc.DashboardComment, threadStatus string, actor Author) ([]byte, error) {
	event := events.Event{
		Type:        eventType,
		DashboardID: row.DashboardID.String(),
		CommentID:   row.ID.String(),
		PanelID:     row.PanelID.String,
		Status:      threadStatus,
		Author:      &events.Author{Kind: actor.Kind, ID: actor.ID, Name: capEventAuthorName(actor.Name)},
	}
	if row.ParentID.Valid {
		event.ParentID = row.ParentID.String()
	}
	return json.Marshal(event)
}

func notifyComment(ctx context.Context, q *dbsqlc.Queries, eventType string, row dbsqlc.DashboardComment, threadStatus string, actor Author) error {
	payload, err := commentEventPayload(eventType, row, threadStatus, actor)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: events.Channel, Payload: string(payload)}); err != nil {
		return fmt.Errorf("notify event: %w", err)
	}
	return nil
}

func commentFromRow(row dbsqlc.DashboardComment, threadStatus string, latestPanels map[string]struct{}) Comment {
	_, present := latestPanels[row.PanelID.String]
	comment := Comment{
		ID:           row.ID.String(),
		DashboardID:  row.DashboardID.String(),
		Version:      int(row.Version),
		PanelID:      row.PanelID.String,
		Body:         row.Body,
		Status:       threadStatus,
		Author:       authorFromRow(row.AuthorKind, row.AuthorID, row.AuthorName, row.AuthorEmail),
		CreatedAt:    row.CreatedAt.Time.UTC(),
		UpdatedAt:    row.UpdatedAt.Time.UTC(),
		PanelPresent: !row.PanelID.Valid || present,
	}
	if row.ParentID.Valid {
		comment.ParentID = row.ParentID.String()
		return comment
	}
	if row.ResolvedAt.Valid {
		at := row.ResolvedAt.Time.UTC()
		comment.ResolvedAt = &at
	}
	if row.ResolvedByKind.Valid {
		by := Author{Kind: row.ResolvedByKind.String, ID: row.ResolvedByID.String(), Name: row.ResolvedByName.String}
		comment.ResolvedBy = &by
	}
	if row.ResolvedByVersion.Valid {
		version := int(row.ResolvedByVersion.Int32)
		comment.ResolvedByVersion = &version
	}
	return comment
}
