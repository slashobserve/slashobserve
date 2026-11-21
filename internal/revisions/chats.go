package revisions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
)

type Chat struct {
	ID            pgtype.UUID
	Title         string
	DashboardID   pgtype.UUID
	ObservationID pgtype.UUID
	PinnedAt      *time.Time
	CreatedAt     time.Time
	LastTurnAt    time.Time
	Turns         int
	LastTurn      *ChatTurn
	Owner         ChatOwner
	ReadOnly      bool
}

type ChatOwner struct {
	Kind string
	ID   pgtype.UUID
	Name string
}

type ChatTurn struct {
	ID     pgtype.UUID
	State  State
	Prompt string
}

type ChatFilter struct {
	DashboardID   pgtype.UUID
	ObservationID pgtype.UUID
	Pinned        *bool
	Query         string
	Everyone      bool
}

type chatReader struct {
	kind  string
	id    pgtype.UUID
	admin bool
}

func readerOf(by auth.Principal) (chatReader, error) {
	id, err := db.ParseUUID(by.ID)
	if err != nil {
		return chatReader{}, fmt.Errorf("chat owner id: %w", err)
	}
	return chatReader{kind: string(by.Kind), id: id, admin: by.Role.AtLeast(auth.RoleAdmin)}, nil
}

func (r chatReader) only(owned bool) (pgtype.Text, pgtype.UUID) {
	if !owned {
		return pgtype.Text{}, pgtype.UUID{}
	}
	return pgtype.Text{String: r.kind, Valid: true}, r.id
}

func (r chatReader) owns(kind string, id pgtype.UUID) bool {
	return kind == r.kind && id == r.id
}

func chatTitle(title pgtype.Text, firstPrompt string) string {
	if title.Valid {
		return title.String
	}
	return DefaultTitle(firstPrompt)
}

func (s *Service) Chats(ctx context.Context, by auth.Principal, f ChatFilter, limit int, cursor string) ([]Chat, string, error) {
	reader, err := readerOf(by)
	if err != nil {
		return nil, "", err
	}
	if f.Everyone && !reader.admin {
		return nil, "", ErrForbidden
	}
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	ownerKind, ownerID := reader.only(!f.Everyone)
	params := dbsqlc.ListChatsParams{
		OwnerKind:     ownerKind,
		OwnerID:       ownerID,
		DashboardID:   f.DashboardID,
		ObservationID: f.ObservationID,
		Pattern:       pgtype.Text{String: "%" + escapeLike(f.Query) + "%", Valid: f.Query != ""},
		RowLimit:      httpapi.Int32(limit + 1),
	}
	if f.Pinned != nil {
		params.Pinned = pgtype.Bool{Bool: *f.Pinned, Valid: true}
	}
	if after != nil {
		params.CursorAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	rows, err := s.db.Queries().ListChats(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("list chats: %w", err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = httpapi.EncodeTimeCursor(rows[limit-1].LastTurnAt.Time, rows[limit-1].ID.String())
	}
	chats := make([]Chat, len(rows))
	for i, row := range rows {
		chats[i] = Chat{
			ID:            row.ID,
			Title:         chatTitle(row.Title, row.FirstPrompt),
			DashboardID:   row.DashboardID,
			ObservationID: row.ObservationID,
			PinnedAt:      optionalTime(row.PinnedAt),
			CreatedAt:     row.CreatedAt.Time.UTC(),
			LastTurnAt:    row.LastTurnAt.Time.UTC(),
			Turns:         int(row.Turns),
			Owner:         ChatOwner{Kind: row.OwnerKind, ID: row.OwnerID, Name: row.OwnerName},
			ReadOnly:      !reader.owns(row.OwnerKind, row.OwnerID),
		}
		if row.LastTurnID.Valid {
			chats[i].LastTurn = &ChatTurn{ID: row.LastTurnID, State: State(row.LastTurnState), Prompt: row.LastTurnPrompt}
		}
	}
	return chats, next, nil
}

func (s *Service) Chat(ctx context.Context, by auth.Principal, id pgtype.UUID) (Chat, []dbsqlc.AgentRequest, error) {
	reader, err := readerOf(by)
	if err != nil {
		return Chat{}, nil, err
	}
	q := s.db.Queries()
	ownerKind, ownerID := reader.only(!reader.admin)
	row, err := q.GetChat(ctx, dbsqlc.GetChatParams{ID: id, OwnerKind: ownerKind, OwnerID: ownerID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Chat{}, nil, ErrChatNotFound
	case err != nil:
		return Chat{}, nil, fmt.Errorf("get chat: %w", err)
	case row.ArchivedAt.Valid:
		return Chat{}, nil, ErrChatNotFound
	}
	turns, err := q.ListChatRequests(ctx, id)
	if err != nil {
		return Chat{}, nil, fmt.Errorf("list chat turns: %w", err)
	}
	chat := Chat{
		ID:            row.ID,
		DashboardID:   row.DashboardID,
		ObservationID: row.ObservationID,
		PinnedAt:      optionalTime(row.PinnedAt),
		CreatedAt:     row.CreatedAt.Time.UTC(),
		LastTurnAt:    row.LastTurnAt.Time.UTC(),
		Turns:         len(turns),
		Owner:         ChatOwner{Kind: row.OwnerKind, ID: row.OwnerID, Name: row.OwnerName},
		ReadOnly:      !reader.owns(row.OwnerKind, row.OwnerID),
	}
	first := ""
	if len(turns) > 0 {
		first = turns[0].Prompt
		last := turns[len(turns)-1]
		chat.LastTurn = &ChatTurn{ID: last.ID, State: State(last.State), Prompt: last.Prompt}
	}
	chat.Title = chatTitle(row.Title, first)
	return chat, turns, nil
}

func (s *Service) ArchiveChat(ctx context.Context, by auth.Principal, id pgtype.UUID) error {
	ownerID, err := db.ParseUUID(by.ID)
	if err != nil {
		return fmt.Errorf("chat owner id: %w", err)
	}
	columns, err := actor.Of(by, origin.From(ctx), "").Columns()
	if err != nil {
		return fmt.Errorf("chat owner: %w", err)
	}
	return s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		row, err := q.ArchiveChat(ctx, dbsqlc.ArchiveChatParams{ID: id, OwnerKind: string(by.Kind), OwnerID: ownerID})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrChatNotFound
		case err != nil:
			return fmt.Errorf("archive chat: %w", err)
		}
		target := audit.Target{Kind: "chat", ID: row.ID.String(), DashboardID: nullUUID(row.DashboardID), ObservationID: nullUUID(row.ObservationID)}
		return audit.Record(ctx, q, audit.Event{Action: actionChatDelete, Target: target, Chat: uuid.NullUUID{UUID: row.ID.Bytes, Valid: true}, By: columns.AuditBy()})
	})
}
