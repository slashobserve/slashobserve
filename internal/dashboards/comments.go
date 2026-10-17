package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	CommentOpen     = "open"
	CommentResolved = "resolved"
	CommentAll      = "all"

	maxCommentRunes  = 4000
	maxPanelIDRunes  = 64
	maxCommentCursor = 253402300799999999
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
}

type Comments struct {
	db         *db.DB
	dashboards *Service
	panelSets  *panelCache
}

func NewComments(database *db.DB, dashboards *Service) *Comments {
	return &Comments{db: database, dashboards: dashboards, panelSets: newPanelCache()}
}

func (cm *Comments) Create(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID, version int, panelID, body string) (Comment, error) {
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
		return notifyComment(ctx, q, events.TypeCommentCreated, row, author)
	})
	if err != nil {
		return Comment{}, err
	}
	return commentFromRow(row, latestPanels), nil
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
	after, err := decodeCommentCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.createdAt, Valid: true}
		params.CursorID = after.id
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
		next = encodeCommentCursor(rows[limit-1])
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
	return commentFromRow(row, panels), nil
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
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < 1 || n > maxCommentRunes || !utf8.ValidString(body) || strings.ContainsFunc(body, forbiddenBodyRune) {
		return "", &InvalidInputError{Message: fmt.Sprintf("body must be 1 to %d characters without control or bidirectional override characters", maxCommentRunes)}
	}
	return body, nil
}

func forbiddenBodyRune(r rune) bool {
	switch {
	case r == '\n', r == '\r', r == '\t':
		return false
	case unicode.IsControl(r):
		return true
	default:
		return r >= '\u202a' && r <= '\u202e' || r >= '\u2066' && r <= '\u2069'
	}
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

type commentCursor struct {
	createdAt time.Time
	id        pgtype.UUID
}

func encodeCommentCursor(row dbsqlc.DashboardComment) string {
	return encodeCursor(strconv.FormatInt(row.CreatedAt.Time.UnixMicro(), 10) + ":" + row.ID.String())
}

func decodeCommentCursor(cursor string) (*commentCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	value, err := decodeCursorValue(cursor)
	if err != nil {
		return nil, err
	}
	micros, idText, ok := strings.Cut(value, ":")
	if !ok {
		return nil, ErrInvalidCursor
	}
	n, err := strconv.ParseInt(micros, 10, 64)
	if err != nil || n < 0 || n > maxCommentCursor || strconv.FormatInt(n, 10) != micros {
		return nil, ErrInvalidCursor
	}
	id, err := db.ParseUUID(idText)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	return &commentCursor{createdAt: time.UnixMicro(n).UTC(), id: id}, nil
}
