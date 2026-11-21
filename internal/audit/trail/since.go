package trail

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/db"
)

var (
	sinceFrom  = time.Unix(0, 0)
	sinceUntil = time.Date(9000, time.January, 1, 0, 0, 0, 0, time.UTC)
)

type Positioned struct {
	Event Event
	At    filter.Position
}

func (r *Reader) Since(ctx context.Context, after filter.Position, limit int) ([]Positioned, error) {
	c, err := filter.Query{}.Follow(filter.Window{From: sinceFrom, To: sinceUntil}, after, limit)
	if err != nil {
		return nil, refused(err)
	}
	return r.positioned(ctx, c, r.timeout)
}

func (r *Reader) positioned(ctx context.Context, c filter.Compiled, timeout time.Duration) ([]Positioned, error) {
	out := []Positioned{}
	err := r.readWithin(ctx, timeout, func(tx db.Rows) error {
		rows, err := tx.Query(ctx, c.SQL, c.Args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var xid string
			e, err := scanEvent(rows, &xid)
			if err != nil {
				return err
			}
			at := filter.Position{Seq: e.Seq}
			if at.Xid, err = strconv.ParseUint(xid, 10, 64); err != nil {
				return fmt.Errorf("read audit position: %w", err)
			}
			out = append(out, Positioned{Event: e, At: at})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
