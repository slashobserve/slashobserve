package activity

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

var ErrInvalidLastEventID = errors.New("last event id is not a sequence number")

type Store struct {
	db     *db.DB
	access *folders.Access
}

func NewStore(database *db.DB, access *folders.Access) *Store {
	return &Store{db: database, access: access}
}

func (s *Store) Audience(ctx context.Context, p auth.Principal) (folders.Audience, error) {
	levels, err := s.access.Levels(ctx, p)
	if err != nil {
		return folders.Audience{}, err
	}
	return folders.AudienceOf(p, levels)
}

func (s *Store) List(ctx context.Context, a folders.Audience, f Filter, cursor string, limit int) (page []Event, next string, err error) {
	params := dbsqlc.ListActivityParams{
		Folders:     visibleFolders(a),
		Admin:       a.Admin,
		ReaderKind:  a.ReaderKind,
		ReaderID:    a.ReaderID,
		DashboardID: f.DashboardID,
		FolderID:    f.FolderID,
		Types:       typesOf(f),
		Since:       sinceOf(f),
		RowLimit:    httpapi.Int32(limit + 1),
	}
	if cursor != "" {
		from, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		params.HasCursor = true
		params.CursorXid = pgtype.Uint64{Uint64: from.xid, Valid: true}
		params.CursorSeq = from.seq
	}
	rows, err := s.db.Queries().ListActivity(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("list activity: %w", err)
	}
	if len(rows) > limit {
		rows = rows[:limit]
		next = encodeCursor(positionOf(rows[limit-1]))
	}
	return eventsOf(rows), next, nil
}

func (s *Store) Start(ctx context.Context, lastEventID string) (from Position, reset bool, err error) {
	if lastEventID == "" {
		from, err = s.head(ctx)
		return from, false, err
	}
	seq, err := strconv.ParseInt(lastEventID, 10, 64)
	if err != nil || seq < 0 {
		return Position{}, false, ErrInvalidLastEventID
	}
	if seq == 0 {
		return Position{}, false, nil
	}
	xid, err := s.db.Queries().ActivityXid(ctx, seq)
	if errors.Is(err, pgx.ErrNoRows) {
		from, err = s.head(ctx)
		return from, true, err
	}
	if err != nil {
		return Position{}, false, fmt.Errorf("get activity position: %w", err)
	}
	return Position{xid: xid.Uint64, seq: seq}, false, nil
}

func (s *Store) head(ctx context.Context) (Position, error) {
	row, err := s.db.Queries().ActivityHead(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return Position{}, nil
	}
	if err != nil {
		return Position{}, fmt.Errorf("get activity head: %w", err)
	}
	return Position{xid: row.Xid.Uint64, seq: row.Seq}, nil
}

func (s *Store) Next(ctx context.Context, a folders.Audience, f Filter, from Position, limit int) (page []Event, to Position, held bool, err error) {
	head, err := s.head(ctx)
	if err != nil {
		return nil, from, false, err
	}
	rows, err := s.db.Queries().ListActivityAfter(ctx, dbsqlc.ListActivityAfterParams{
		Folders:     visibleFolders(a),
		Admin:       a.Admin,
		ReaderKind:  a.ReaderKind,
		ReaderID:    a.ReaderID,
		DashboardID: f.DashboardID,
		FolderID:    f.FolderID,
		Types:       typesOf(f),
		Since:       sinceOf(f),
		AfterXid:    pgtype.Uint64{Uint64: from.xid, Valid: true},
		AfterSeq:    from.seq,
		RowLimit:    httpapi.Int32(limit),
	})
	if err != nil {
		return nil, from, false, fmt.Errorf("read activity after a position: %w", err)
	}
	to = from
	if len(rows) > 0 {
		to = positionOf(rows[len(rows)-1])
	}
	if len(rows) < limit {
		to = later(to, head)
		held, err = s.db.Queries().ActivityHeld(ctx)
		if err != nil {
			return nil, from, false, fmt.Errorf("check held activity: %w", err)
		}
	}
	return eventsOf(rows), to, held, nil
}

func visibleFolders(a folders.Audience) []pgtype.UUID {
	if a.Folders == nil {
		return []pgtype.UUID{}
	}
	return a.Folders
}

func typesOf(f Filter) []string {
	if f.Types == nil {
		return []string{}
	}
	return f.Types
}

func sinceOf(f Filter) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: f.Since, Valid: !f.Since.IsZero()}
}

func positionOf(r dbsqlc.Activity) Position {
	return Position{xid: r.Xid.Uint64, seq: r.Seq}
}

func eventsOf(rows []dbsqlc.Activity) []Event {
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		out = append(out, Event{
			Seq:         r.Seq,
			Type:        r.Type,
			TargetKind:  r.TargetKind,
			TargetID:    r.TargetID.String(),
			DashboardID: r.DashboardID.String(),
			FolderID:    r.FolderID.String(),
			Actor: actor.FromColumns(actor.Columns{
				Kind:   r.ActorKind,
				ID:     r.ActorID,
				Name:   r.ActorName,
				Origin: r.Origin,
				Client: r.OriginClient,
				RunID:  r.RunID,
			}),
			Data:      r.Data,
			CreatedAt: r.CreatedAt.Time,
			Position:  positionOf(r),
		})
	}
	return out
}
