package export

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type Cursor struct {
	queries *dbsqlc.Queries
}

func NewCursor(queries *dbsqlc.Queries) *Cursor {
	return &Cursor{queries: queries}
}

type Stored struct {
	At         filter.Position
	OccurredAt time.Time
	Exists     bool
}

func (c *Cursor) Load(ctx context.Context) (Stored, error) {
	row, err := c.queries.GetAuditExportCursor(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return Stored{}, nil
	}
	if err != nil {
		return Stored{}, err
	}
	return Stored{At: filter.Position{Xid: row.Xid.Uint64, Seq: row.Seq}, OccurredAt: row.OccurredAt.Time, Exists: true}, nil
}

func (c *Cursor) Oldest(ctx context.Context) (time.Time, error) {
	oldest, err := c.queries.GetOldestAuditEventAt(ctx)
	if err != nil || !oldest.Valid {
		return time.Time{}, err
	}
	return oldest.Time, nil
}

func (c *Cursor) Advance(ctx context.Context, at filter.Position, occurredAt time.Time) error {
	return c.queries.AdvanceAuditExportCursor(ctx, dbsqlc.AdvanceAuditExportCursorParams{
		Xid:        pgtype.Uint64{Uint64: at.Xid, Valid: true},
		Seq:        at.Seq,
		OccurredAt: pgtype.Timestamptz{Time: occurredAt, Valid: true},
	})
}

func (c *Cursor) Lag(ctx context.Context) (time.Duration, error) {
	seconds, err := c.queries.GetAuditExportLag(ctx)
	if err != nil {
		return 0, err
	}
	return time.Duration(seconds * float64(time.Second)).Round(time.Millisecond), nil
}
