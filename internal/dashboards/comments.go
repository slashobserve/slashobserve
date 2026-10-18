package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
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
	maxPanelIDRunes = 64
)

var (
	ErrCommentNotFound = errors.New("comment not found")
	ErrUnknownPanel    = errors.New("panel does not exist in that version")
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
	ID           string
	DashboardID  string
	Version      int
	PanelID      string
	Body         string
	Status       string
	Author       Author
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ResolvedAt   *time.Time
	ResolvedBy   *Author
	PanelPresent bool
	Revision     *CommentRevision
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

func (cm *Comments) Create(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID, version int, panelID, body string, requestRevision bool) (Comment, error) {
	if !validVersion(version) {
		return Comment{}, &InvalidInputError{Message: "version must be a positive 32-bit integer"}
	}
	if err := checkPanelID(panelID); err != nil {
		return Comment{}, err
	}
	body, err := checkCommentBody(body)
	if err != nil {
		return Comment{}, err
	}
	authorID, err := db.ParseUUID(by.ID)
	if err != nil {
		return Comment{}, fmt.Errorf("author id: %w", err)
	}
	latest, err := cm.dashboards.LatestVersion(ctx, dashboardID)
	if err != nil {
		return Comment{}, err
	}
	targetPanels, err := cm.panels(ctx, dashboardID, version)
	if err != nil {
		return Comment{}, err
	}
	if _, ok := targetPanels[panelID]; !ok {
		return Comment{}, ErrUnknownPanel
	}
	latestPanels := targetPanels
	if latest != version {
		if latestPanels, err = cm.panels(ctx, dashboardID, latest); err != nil {
			return Comment{}, err
		}
	}
	author := authorOf(by)
	var row dbsqlc.DashboardComment
	var requested *CommentRevision
	err = cm.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		row, err = q.CreateComment(ctx, dbsqlc.CreateCommentParams{
			DashboardID: dashboardID,
			Version:     toInt32(version),
			PanelID:     panelID,
			Body:        body,
			AuthorKind:  author.Kind,
			AuthorID:    authorID,
			AuthorName:  author.Name,
			AuthorEmail: pgtype.Text{String: author.Email, Valid: author.Email != ""},
		})
		if err != nil {
			return fmt.Errorf("create comment: %w", err)
		}
		requested, err = cm.announce(ctx, q, by, row, requestRevision)
		return err
	})
	if err != nil {
		return Comment{}, err
	}
	comment := commentFromRow(row, latestPanels)
	comment.Revision = requested
	return comment, nil
}

func (cm *Comments) announce(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, row dbsqlc.DashboardComment, requestRevision bool) (*CommentRevision, error) {
	if err := notifyComment(ctx, q, events.TypeCommentCreated, row, authorOf(by)); err != nil {
		return nil, err
	}
	if !requestRevision || !cm.revisions.Available() {
		return nil, nil
	}
	rev, err := cm.revisions.Enqueue(ctx, q, revisions.EnqueueParams{
		DashboardID: row.DashboardID,
		Source:      revisions.SourceComment,
		CommentID:   row.ID,
		Prompt:      row.Body,
		PanelID:     row.PanelID,
		BaseVersion: int(row.Version),
		By:          by,
	})
	if err != nil {
		return nil, err
	}
	return &CommentRevision{ID: rev.ID, Status: string(rev.Status)}, nil
}

func (cm *Comments) List(ctx context.Context, dashboardID pgtype.UUID, status, panelID string, limit int, cursor string) ([]Comment, string, error) {
	params := dbsqlc.ListCommentsParams{DashboardID: dashboardID, RowLimit: toInt32(limit + 1)}
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
		next = httpapi.EncodeTimeCursor(rows[limit-1].CreatedAt.Time, rows[limit-1].ID.String())
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
		items = append(items, commentFromRow(r, panels))
	}
	if err := cm.attachRevisions(ctx, items); err != nil {
		return nil, "", err
	}
	return items, next, nil
}

func (cm *Comments) SetStatus(ctx context.Context, by auth.Principal, dashboardID, commentID pgtype.UUID, status string) (Comment, error) {
	if status != CommentOpen && status != CommentResolved {
		return Comment{}, &InvalidInputError{Message: "status must be open or resolved"}
	}
	actorID, err := db.ParseUUID(by.ID)
	if err != nil {
		return Comment{}, fmt.Errorf("actor id: %w", err)
	}
	actor := authorOf(by)
	params := dbsqlc.SetCommentStatusParams{ID: commentID, DashboardID: dashboardID, Status: status}
	if status == CommentResolved {
		params.ResolvedByKind = pgtype.Text{String: actor.Kind, Valid: true}
		params.ResolvedByID = actorID
		params.ResolvedByName = pgtype.Text{String: actor.Name, Valid: true}
	}
	latest, err := cm.dashboards.LatestVersion(ctx, dashboardID)
	if err != nil {
		return Comment{}, err
	}
	panels, err := cm.panels(ctx, dashboardID, latest)
	if err != nil {
		return Comment{}, err
	}
	var row dbsqlc.DashboardComment
	err = cm.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		updated, err := q.SetCommentStatus(ctx, params)
		switch {
		case err == nil:
			row = updated
			return notifyComment(ctx, q, events.TypeCommentUpdated, row, actor)
		case errors.Is(err, pgx.ErrNoRows):
			row, err = q.GetComment(ctx, dbsqlc.GetCommentParams{ID: commentID, DashboardID: dashboardID})
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCommentNotFound
			}
			if err != nil {
				return fmt.Errorf("get comment: %w", err)
			}
			return nil
		default:
			return fmt.Errorf("set comment status: %w", err)
		}
	})
	if err != nil {
		return Comment{}, err
	}
	comments := []Comment{commentFromRow(row, panels)}
	if err := cm.attachRevisions(ctx, comments); err != nil {
		return Comment{}, err
	}
	return comments[0], nil
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
	set := make(map[string]struct{}, len(d.Panels))
	for _, p := range d.Panels {
		set[p.ID] = struct{}{}
	}
	return set
}

func checkPanelID(panelID string) error {
	if n := utf8.RuneCountInString(panelID); n < 1 || n > maxPanelIDRunes || !utf8.ValidString(panelID) || strings.ContainsRune(panelID, 0) {
		return &InvalidInputError{Message: fmt.Sprintf("panel_id must be 1 to %d characters without NUL", maxPanelIDRunes)}
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
func commentEventPayload(eventType string, row dbsqlc.DashboardComment, actor Author) ([]byte, error) {
	return json.Marshal(events.Event{
		Type:        eventType,
		DashboardID: row.DashboardID.String(),
		CommentID:   row.ID.String(),
		PanelID:     row.PanelID,
		Status:      row.Status,
		Author:      &events.Author{Kind: actor.Kind, ID: actor.ID, Name: capEventAuthorName(actor.Name)},
	})
}

func notifyComment(ctx context.Context, q *dbsqlc.Queries, eventType string, row dbsqlc.DashboardComment, actor Author) error {
	payload, err := commentEventPayload(eventType, row, actor)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: events.Channel, Payload: string(payload)}); err != nil {
		return fmt.Errorf("notify event: %w", err)
	}
	return nil
}

func commentFromRow(row dbsqlc.DashboardComment, latestPanels map[string]struct{}) Comment {
	_, present := latestPanels[row.PanelID]
	comment := Comment{
		ID:           row.ID.String(),
		DashboardID:  row.DashboardID.String(),
		Version:      int(row.Version),
		PanelID:      row.PanelID,
		Body:         row.Body,
		Status:       row.Status,
		Author:       authorFromRow(row.AuthorKind, row.AuthorID, row.AuthorName, row.AuthorEmail),
		CreatedAt:    row.CreatedAt.Time.UTC(),
		UpdatedAt:    row.UpdatedAt.Time.UTC(),
		PanelPresent: present,
	}
	if row.ResolvedAt.Valid {
		at := row.ResolvedAt.Time.UTC()
		comment.ResolvedAt = &at
	}
	if row.ResolvedByKind.Valid {
		by := Author{Kind: row.ResolvedByKind.String, ID: row.ResolvedByID.String(), Name: row.ResolvedByName.String}
		comment.ResolvedBy = &by
	}
	return comment
}
