package export

import (
	"context"
	"errors"

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

func (c *Cursor) Load(ctx context.Context) (filter.Position, error) {
	row, err := c.queries.GetAuditExportCursor(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return filter.Position{}, nil
	}
	if err != nil {
		return filter.Position{}, err
	}
	return filter.Position{Xid: row.Xid.Uint64, Seq: row.Seq}, nil
}

func (c *Cursor) Advance(ctx context.Context, at filter.Position) error {
	return c.queries.AdvanceAuditExportCursor(ctx, dbsqlc.AdvanceAuditExportCursorParams{
		Xid: pgtype.Uint64{Uint64: at.Xid, Valid: true},
		Seq: at.Seq,
	})
}
