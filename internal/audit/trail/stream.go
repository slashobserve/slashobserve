package trail

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	FollowSlack   = 10 * time.Minute
	followAhead   = time.Minute
	FollowBatch   = 100
	headStatement = "SELECT xid::text, seq FROM audit_events WHERE occurred_at >= $1 AND xid < pg_snapshot_xmin(pg_current_snapshot()) ORDER BY xid DESC, seq DESC LIMIT 1"
)

type Followed struct {
	ID    string
	Event Event
}

type Follow struct {
	reader *Reader
	query  filter.Query
	from   time.Time
	at     filter.Position
}

func (r *Reader) Follow(ctx context.Context, text, lastEventID string) (*Follow, error) {
	f, err := r.follow(ctx, text, lastEventID)
	if err != nil {
		if problem, ok := ProblemOf(err); ok {
			r.write(ctx, audit.Event{
				Action: ActionSearch, Outcome: audit.OutcomeFailed, Reason: problem.Code, Status: problem.Status,
				Data: map[string]any{"query": recordedQuery(text), "view": "stream"},
			})
		}
		return nil, err
	}
	r.write(ctx, audit.Event{Action: ActionSearch, Status: http.StatusOK, Data: map[string]any{
		"form": string(f.query.Form), "since": f.from.Format(time.RFC3339Nano), "query": recordedQuery(text), "view": "stream",
	}})
	return f, nil
}

func (r *Reader) follow(ctx context.Context, text, lastEventID string) (*Follow, error) {
	q, err := filter.Parse(text)
	if err != nil {
		return nil, err
	}
	f := &Follow{reader: r, query: q, from: r.now().UTC().Add(-FollowSlack)}
	if lastEventID == "" {
		f.at, err = r.head(ctx, f.from)
		return f, err
	}
	at, ok := parseEventID(lastEventID)
	if !ok {
		return nil, invalidf("the Last-Event-ID header must be the id of a frame of this stream")
	}
	f.at = at
	return f, nil
}

func (r *Reader) head(ctx context.Context, from time.Time) (filter.Position, error) {
	var at filter.Position
	err := r.read(ctx, func(tx db.Rows) error {
		rows, err := tx.Query(ctx, headStatement, from)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var xid string
			if err := rows.Scan(&xid, &at.Seq); err != nil {
				return err
			}
			if at.Xid, err = strconv.ParseUint(xid, 10, 64); err != nil {
				return fmt.Errorf("read audit position: %w", err)
			}
		}
		return rows.Err()
	})
	return at, err
}

func (f *Follow) Next(ctx context.Context) ([]Followed, error) {
	c, err := f.query.Follow(filter.Window{From: f.from, To: f.reader.now().UTC().Add(followAhead)}, f.at, FollowBatch)
	if err != nil {
		return nil, refused(err)
	}
	out := []Followed{}
	err = f.reader.read(ctx, func(tx db.Rows) error {
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
			out = append(out, Followed{ID: eventID(at), Event: e})
			f.at = at
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func eventID(at filter.Position) string {
	return strconv.FormatUint(at.Xid, 10) + "-" + strconv.FormatInt(at.Seq, 10)
}

func parseEventID(raw string) (filter.Position, bool) {
	xid, seq, ok := strings.Cut(raw, "-")
	if !ok {
		return filter.Position{}, false
	}
	var at filter.Position
	var err error
	if at.Xid, err = strconv.ParseUint(xid, 10, 64); err != nil {
		return filter.Position{}, false
	}
	if at.Seq, err = strconv.ParseInt(seq, 10, 64); err != nil || at.Seq < 0 {
		return filter.Position{}, false
	}
	return at, true
}
