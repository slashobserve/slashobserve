package threads

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type Author struct {
	Kind  string
	ID    string
	Name  string
	Email string
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
	Actor             actor.Actor
	CreatedAt         time.Time
	UpdatedAt         time.Time
	EditedAt          *time.Time
	ResolvedAt        *time.Time
	ResolvedBy        *Author
	ResolvedActor     *actor.Actor
	ResolvedByVersion *int
	PanelPresent      bool
	Mentions          []Mention
	CanEdit           bool
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
	service   *Service
	revisions *revisions.Service
}

func NewComments(service *Service, revisions *revisions.Service) *Comments {
	return &Comments{service: service, revisions: revisions}
}

func (cm *Comments) Create(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID, in NewComment) (Comment, error) {
	if in.RequestRevision {
		abilities, err := cm.service.Abilities(ctx, by, Target{Kind: KindDashboard, ID: dashboardID})
		if err != nil {
			return Comment{}, legacyError(err)
		}
		if !abilities.Edit {
			return Comment{}, folders.ErrForbidden
		}
	}
	if in.ParentID.Valid {
		return cm.reply(ctx, by, dashboardID, in)
	}
	return cm.open(ctx, by, dashboardID, in)
}

func (cm *Comments) open(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID, in NewComment) (Comment, error) {
	var requested *CommentRevision
	hook := func(q *dbsqlc.Queries, thread Thread, message Message) error {
		if !in.RequestRevision || !cm.revisions.Available() {
			return nil
		}
		threadID, err := db.ParseUUID(thread.ID)
		if err != nil {
			return fmt.Errorf("thread id: %w", err)
		}
		queued, err := cm.revisions.Enqueue(ctx, q, revisions.EnqueueParams{
			DashboardID: dashboardID,
			Source:      revisions.SourceComment,
			CommentID:   threadID,
			Prompt:      message.Body,
			PanelID:     thread.PanelID,
			BaseVersion: *thread.Version,
			By:          by,
		})
		if err != nil {
			return err
		}
		requested = &CommentRevision{ID: queued.ID.String(), Status: queued.Status}
		return nil
	}
	thread, message, err := cm.service.Open(ctx, by, OpenInput{
		Target:  Target{Kind: KindDashboard, ID: dashboardID},
		Version: &in.Version,
		PanelID: in.PanelID,
		Body:    in.Body,
	}, hook)
	if errors.Is(err, ErrUnknownVersion) {
		return Comment{}, dashboards.ErrNotFound
	}
	if err != nil {
		return Comment{}, legacyError(err)
	}
	comment := commentOf(thread, message)
	comment.Revision = requested
	return comment, nil
}

func (cm *Comments) reply(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID, in NewComment) (Comment, error) {
	if in.RequestRevision {
		return Comment{}, &InvalidInputError{Message: "a reply cannot request a revision"}
	}
	if _, err := cm.threadOn(ctx, dashboardID, in.ParentID); err != nil {
		return Comment{}, err
	}
	thread, message, err := cm.service.Reply(ctx, by, in.ParentID, in.Body)
	if err != nil {
		return Comment{}, legacyError(err)
	}
	return commentOf(thread, message), nil
}

func (cm *Comments) List(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID, status, panelID string, limit int, cursor string) ([]Comment, string, error) {
	page, err := cm.service.List(ctx, by, ListQuery{
		Target:  Target{Kind: KindDashboard, ID: dashboardID},
		Status:  status,
		PanelID: panelID,
		Limit:   limit,
		Cursor:  cursor,
	})
	if err != nil {
		return nil, "", legacyError(err)
	}
	threads := make(map[string]Thread, len(page.Threads))
	for _, t := range page.Threads {
		threads[t.ID] = t
	}
	items := make([]Comment, 0, len(page.Messages))
	for _, m := range page.Messages {
		items = append(items, commentOf(threads[m.ThreadID], m))
	}
	if err := cm.attachRevisions(ctx, items); err != nil {
		return nil, "", err
	}
	return items, page.Next, nil
}

func (cm *Comments) SetStatus(ctx context.Context, by auth.Principal, dashboardID, commentID pgtype.UUID, status string, resolvedByVersion *int) (Comment, bool, error) {
	if err := checkStatusChange(status, resolvedByVersion); err != nil {
		return Comment{}, false, err
	}
	row, err := cm.threadOn(ctx, dashboardID, commentID)
	if err != nil {
		return Comment{}, false, err
	}
	thread, changed, err := cm.service.SetStatus(ctx, by, row.ID, status, resolvedByVersion)
	if err != nil {
		return Comment{}, false, legacyError(err)
	}
	first, err := cm.service.ReadMessage(ctx, by, row.ID)
	if err != nil {
		return Comment{}, false, err
	}
	comments := []Comment{commentOf(thread, first)}
	if err := cm.attachRevisions(ctx, comments); err != nil {
		return Comment{}, false, err
	}
	return comments[0], changed, nil
}

func (cm *Comments) DashboardOf(ctx context.Context, commentID pgtype.UUID) (pgtype.UUID, error) {
	row, err := cm.service.db.Queries().GetThreadOfMessage(ctx, commentID)
	if err != nil {
		return pgtype.UUID{}, threadLookup(err)
	}
	if !row.DashboardID.Valid {
		return pgtype.UUID{}, ErrNotFound
	}
	return row.DashboardID, nil
}

func (cm *Comments) ThreadVersion(ctx context.Context, dashboardID, commentID pgtype.UUID) (int, error) {
	row, err := cm.threadOn(ctx, dashboardID, commentID)
	if err != nil {
		return 0, err
	}
	return int(row.DashboardVersion.Int32), nil
}

func (cm *Comments) threadOn(ctx context.Context, dashboardID, commentID pgtype.UUID) (dbsqlc.Thread, error) {
	row, err := cm.service.db.Queries().GetThreadOfMessage(ctx, commentID)
	if err != nil {
		return dbsqlc.Thread{}, threadLookup(err)
	}
	if row.DashboardID != dashboardID {
		return dbsqlc.Thread{}, ErrNotFound
	}
	return row, nil
}

func (cm *Comments) attachRevisions(ctx context.Context, comments []Comment) error {
	ids := make([]string, len(comments))
	for i, c := range comments {
		ids[i] = c.ID
	}
	byComment, err := cm.service.latestRevisions(ctx, ids)
	if err != nil {
		return err
	}
	for i := range comments {
		comments[i].Revision = byComment[comments[i].ID]
	}
	return nil
}

func commentOf(thread Thread, message Message) Comment {
	comment := Comment{
		ID:           message.ID,
		DashboardID:  thread.DashboardID,
		Version:      *thread.Version,
		PanelID:      thread.PanelID,
		Body:         message.Body,
		Status:       thread.Status,
		Author:       authorOf(message.Author, message.AuthorEmail),
		Actor:        message.Author,
		CreatedAt:    message.CreatedAt,
		UpdatedAt:    message.UpdatedAt,
		EditedAt:     message.EditedAt,
		PanelPresent: thread.PanelPresent,
		Mentions:     message.Mentions,
		CanEdit:      message.CanEdit,
	}
	if message.ID != thread.ID {
		comment.ParentID = thread.ID
		return comment
	}
	if thread.UpdatedAt.After(comment.UpdatedAt) {
		comment.UpdatedAt = thread.UpdatedAt
	}
	comment.ResolvedAt = thread.ResolvedAt
	comment.ResolvedByVersion = thread.ResolvedByVersion
	if thread.ResolvedBy != nil {
		by := authorOf(*thread.ResolvedBy, "")
		comment.ResolvedBy = &by
		comment.ResolvedActor = thread.ResolvedBy
	}
	return comment
}

func authorOf(a actor.Actor, email string) Author {
	return Author{Kind: string(a.Kind), ID: a.ID, Name: a.Name, Email: email}
}

func legacyError(err error) error {
	if errors.Is(err, ErrTargetNotFound) {
		return dashboards.ErrNotFound
	}
	return err
}
