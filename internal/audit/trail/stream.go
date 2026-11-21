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
	MaxResumeAge  = 24 * time.Hour
	FollowTimeout = 5 * time.Second
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
	start  string
}

func (r *Reader) Follow(ctx context.Context, text, lastEventID string) (*Follow, error) {
	f, err := r.follow(ctx, text, lastEventID)
	if err != nil {
		if problem, ok := ProblemOf(err); ok {
			r.write(ctx, audit.Event{
				Action: ActionSearch, Outcome: audit.OutcomeFailed, Reason: problem.Code, Status: problem.Status,
				Data: map[string]any{"query": audit.RecordedQuery(text), "view": "stream"},
			})
		}
		return nil, err
	}
	r.write(ctx, audit.Event{Action: ActionSearch, Status: http.StatusOK, Data: map[string]any{
		"form": string(f.query.Form), "since": f.from.Format(time.RFC3339Nano), "query": audit.RecordedQuery(text), "view": "stream",
	}})
	return f, nil
}

func (r *Reader) follow(ctx context.Context, text, lastEventID string) (*Follow, error) {
	q, err := filter.Parse(text)
	if err != nil {
		return nil, err
	}
	opened := r.now().UTC()
	f := &Follow{reader: r, query: q, from: opened.Add(-FollowSlack)}
	if lastEventID == "" {
		f.at, err = r.head(ctx, f.from)
		f.start = eventID(f.at, opened)
		return f, err
	}
	at, when, ok := parseEventID(lastEventID)
	if !ok || when.After(r.now().Add(followAhead)) {
		return nil, invalidf("the Last-Event-ID header must be the id of a frame of this stream")
	}
	if when.Before(r.now().Add(-MaxResumeAge)) {
		return nil, invalidf("the stream resumes only within 24 hours of the last event it sent, and that event is from %s; read the events since then with a search", when.UTC().Format(time.RFC3339))
	}
	f.from, f.at, f.start = when.UTC().Add(-FollowSlack), at, eventID(at, when)
	return f, nil
}

func (f *Follow) Start() string {
	return f.start
}

func (r *Reader) head(ctx context.Context, from time.Time) (filter.Position, error) {
	var at filter.Position
	err := r.readWithin(ctx, r.followTimeout, func(tx db.Rows) error {
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
	rows, err := f.reader.positioned(ctx, c, f.reader.followTimeout)
	if err != nil {
		return nil, err
	}
	out := make([]Followed, 0, len(rows))
	for _, row := range rows {
		out = append(out, Followed{ID: eventID(row.At, row.Event.at), Event: row.Event})
		f.at = row.At
	}
	return out, nil
}

func eventID(at filter.Position, when time.Time) string {
	return strconv.FormatUint(at.Xid, 10) + "-" + strconv.FormatInt(at.Seq, 10) + "-" + strconv.FormatInt(when.UnixMicro(), 10)
}

func parseEventID(raw string) (filter.Position, time.Time, bool) {
	parts := strings.Split(raw, "-")
	if len(parts) != 3 {
		return filter.Position{}, time.Time{}, false
	}
	var at filter.Position
	var err error
	if at.Xid, err = strconv.ParseUint(parts[0], 10, 64); err != nil {
		return filter.Position{}, time.Time{}, false
	}
	seq, seqErr := strconv.ParseUint(parts[1], 10, 63)
	micros, microsErr := strconv.ParseUint(parts[2], 10, 63)
	if seqErr != nil || microsErr != nil {
		return filter.Position{}, time.Time{}, false
	}
	at.Seq = int64(seq)
	return at, time.UnixMicro(int64(micros)).UTC(), true
}
