package trail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	StatementTimeout = 30 * time.Second
	DefaultBuckets   = 60
	MaxBuckets       = 200
	MaxRelated       = 50

	queryCanceled   = "57014"
	sameRequestSpan = time.Hour
	sameRunSpan     = 24 * time.Hour
	idClockSkew     = time.Minute
)

type Reader struct {
	db            *db.DB
	logger        *slog.Logger
	now           func() time.Time
	timeout       time.Duration
	followTimeout time.Duration
	searches      *audit.Repeats
}

func NewReader(d *db.DB, logger *slog.Logger) *Reader {
	return &Reader{db: d, logger: logger, now: time.Now, timeout: StatementTimeout, followTimeout: FollowTimeout, searches: audit.NewRepeats()}
}

func (r *Reader) Now() time.Time {
	return r.now()
}

type Page struct {
	Events     []Event
	NextCursor string
	Window     filter.Window
}

type Facet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type Bucket struct {
	Time  string `json:"time"`
	Count int64  `json:"count"`
}

type Histogram struct {
	StepMs  int64    `json:"step_ms"`
	Total   int64    `json:"total"`
	Buckets []Bucket `json:"buckets"`
}

type AggRow struct {
	Keys     []any      `json:"keys"`
	DocCount int64      `json:"doc_count"`
	Values   []*float64 `json:"values"`
}

type Aggregation struct {
	Levels    []filter.Level  `json:"-"`
	Metrics   []filter.Metric `json:"-"`
	Rows      []AggRow        `json:"rows"`
	Truncated bool            `json:"truncated"`
}

type Detail struct {
	Event       Event
	SameRequest []Event
	SameRun     []Event
}

func (r *Reader) read(ctx context.Context, fn func(db.Rows) error) error {
	return r.readWithin(ctx, r.timeout, fn)
}

func (r *Reader) readWithin(ctx context.Context, timeout time.Duration, fn func(db.Rows) error) error {
	err := r.db.ReadWithin(ctx, timeout, fn)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == queryCanceled && ctx.Err() == nil {
		return ErrTimeout
	}
	return err
}

func refused(err error) error {
	var parse *filter.Error
	if err == nil || errors.As(err, &parse) {
		return err
	}
	return &QueryRefused{Msg: err.Error()}
}

func (r *Reader) Search(ctx context.Context, p Prepared, limit int) (Page, error) {
	page, err := r.events(ctx, p, limit)
	if err != nil {
		return Page{}, r.readFailed(ctx, p, read{view: viewEvents}, err)
	}
	r.recordRead(ctx, p, read{view: viewEvents}, len(page.Events))
	return page, nil
}

func (r *Reader) events(ctx context.Context, p Prepared, limit int) (Page, error) {
	c, err := p.Query.Rows(p.Window, filter.Page{Limit: limit, After: p.after})
	if err != nil {
		return Page{}, refused(err)
	}
	events, err := r.compiledEvents(ctx, c)
	if err != nil {
		return Page{}, err
	}
	page := Page{Events: events, Window: p.Window}
	if len(events) > c.Limit {
		page.Events = events[:c.Limit]
		last := page.Events[len(page.Events)-1]
		page.NextCursor = encodeCursor(p, last.at, last.ID)
	}
	return page, nil
}

func (r *Reader) compiledEvents(ctx context.Context, c filter.Compiled) ([]Event, error) {
	var events []Event
	err := r.read(ctx, func(tx db.Rows) error {
		var err error
		events, err = collectEvents(ctx, tx, c)
		return err
	})
	return events, err
}

func (r *Reader) Rows(ctx context.Context, q filter.Query, w filter.Window, page filter.Page) ([]Event, error) {
	c, err := q.Rows(w, page)
	if err != nil {
		return nil, refused(err)
	}
	events, err := r.compiledEvents(ctx, c)
	if err != nil {
		return nil, err
	}
	return events[:min(len(events), c.Limit)], nil
}

func (r *Reader) Count(ctx context.Context, q filter.Query, w filter.Window, upTo int) (int64, error) {
	c, err := q.Count(w, upTo)
	if err != nil {
		return 0, refused(err)
	}
	var n int64
	err = r.read(ctx, func(tx db.Rows) error {
		rows, err := tx.Query(ctx, c.SQL, c.Args...)
		if err != nil {
			return err
		}
		_, err = pgx.ForEachRow(rows, []any{&n}, func() error { return nil })
		return err
	})
	return n, err
}

func (p Prepared) Cut(page Page, n int) Page {
	if n < 1 || n >= len(page.Events) {
		return page
	}
	page.Events = page.Events[:n]
	last := page.Events[n-1]
	page.NextCursor = encodeCursor(p, last.at, last.ID)
	return page
}

func collectEvents(ctx context.Context, tx db.Rows, c filter.Compiled) ([]Event, error) {
	rows, err := tx.Query(ctx, c.SQL, c.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *Reader) Facets(ctx context.Context, p Prepared, field string, limit int) ([]Facet, error) {
	this := read{view: viewFacets, field: field}
	c, err := p.Query.Facets(p.Window, field, limit)
	if err != nil {
		return nil, r.readFailed(ctx, p, this, refused(err))
	}
	facets := []Facet{}
	err = r.read(ctx, func(tx db.Rows) error {
		rows, err := tx.Query(ctx, c.SQL, c.Args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var f Facet
			if err := rows.Scan(&f.Value, &f.Count); err != nil {
				return err
			}
			facets = append(facets, f)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, r.readFailed(ctx, p, this, err)
	}
	r.recordRead(ctx, p, this, len(facets))
	return facets, nil
}

var steps = []time.Duration{
	time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second,
	time.Minute, 5 * time.Minute, 10 * time.Minute, 30 * time.Minute,
	time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour,
}

func StepFor(w filter.Window, buckets int) time.Duration {
	span := w.To.Sub(w.From)
	for _, step := range steps {
		if span <= step*time.Duration(buckets) {
			return step
		}
	}
	return steps[len(steps)-1]
}

var binOrigin = time.Unix(0, 0).UTC()

func binStart(t time.Time, step time.Duration) time.Time {
	return binOrigin.Add(t.Sub(binOrigin).Truncate(step))
}

func (r *Reader) Histogram(ctx context.Context, p Prepared, buckets int) (Histogram, error) {
	this := read{view: viewHistogram}
	step := StepFor(p.Window, buckets)
	agg, err := p.Query.Histogram(p.Window, step)
	if err != nil {
		return Histogram{}, r.readFailed(ctx, p, this, refused(err))
	}
	counts := map[int64]int64{}
	err = r.read(ctx, func(tx db.Rows) error {
		rows, err := tx.Query(ctx, agg.SQL, agg.Args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var at time.Time
			var n float64
			if err := rows.Scan(&at, &n); err != nil {
				return err
			}
			counts[at.UnixMilli()] = int64(n)
		}
		return rows.Err()
	})
	if err != nil {
		return Histogram{}, r.readFailed(ctx, p, this, err)
	}
	h := Histogram{StepMs: step.Milliseconds(), Buckets: []Bucket{}}
	for at := binStart(p.Window.From, step); at.Before(p.Window.To); at = at.Add(step) {
		n := counts[at.UnixMilli()]
		h.Total += n
		h.Buckets = append(h.Buckets, Bucket{Time: at.UTC().Format(time.RFC3339), Count: n})
	}
	r.recordRead(ctx, p, this, len(h.Buckets))
	return h, nil
}

func (r *Reader) Aggregate(ctx context.Context, p Prepared) (Aggregation, error) {
	this := read{view: viewAggregate}
	out, err := r.ReadAggregation(ctx, p)
	if err != nil {
		return Aggregation{}, r.readFailed(ctx, p, this, err)
	}
	r.recordRead(ctx, p, this, len(out.Rows))
	return out, nil
}

func (r *Reader) ReadAggregation(ctx context.Context, p Prepared) (Aggregation, error) {
	agg, err := p.Query.Aggregate(p.Window)
	if err != nil {
		return Aggregation{}, refused(err)
	}
	out := Aggregation{Levels: agg.Levels, Metrics: agg.Metrics, Rows: []AggRow{}}
	err = r.read(ctx, func(tx db.Rows) error {
		rows, err := tx.Query(ctx, agg.SQL, agg.Args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				return err
			}
			if len(out.Rows) == agg.RowLimit-1 {
				out.Truncated = true
				break
			}
			out.Rows = append(out.Rows, aggRow(values, len(agg.Levels)))
		}
		return rows.Err()
	})
	if err != nil {
		return Aggregation{}, err
	}
	return out, nil
}

func aggRow(values []any, levels int) AggRow {
	row := AggRow{Keys: make([]any, levels), Values: make([]*float64, 0, len(values)-levels-1)}
	for i := range levels {
		row.Keys[i] = aggKey(values[i])
	}
	if n, ok := values[levels].(float64); ok {
		row.DocCount = int64(n)
	}
	for _, v := range values[levels+1:] {
		if n, ok := v.(float64); ok {
			row.Values = append(row.Values, &n)
		} else {
			row.Values = append(row.Values, nil)
		}
	}
	return row
}

func aggKey(v any) any {
	switch k := v.(type) {
	case time.Time:
		return k.UTC().Format(time.RFC3339Nano)
	case nil:
		return nil
	case string:
		return k
	}
	return fmt.Sprint(v)
}

func (r *Reader) Event(ctx context.Context, id string) (Detail, error) {
	this := read{view: viewEvent}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.Version() != 7 {
		r.writeFailed(ctx, this, "id:"+id, ErrNotFound)
		return Detail{}, ErrNotFound
	}
	sec, nsec := parsed.Time().UnixTime()
	own, err := r.around(ctx, "id:"+parsed.String(), time.Unix(sec, nsec).UTC(), idClockSkew, 1)
	if err != nil {
		return Detail{}, r.readFailed(ctx, own.p, this, err)
	}
	if len(own.page.Events) == 0 {
		return Detail{}, r.readFailed(ctx, own.p, this, ErrNotFound)
	}
	d := Detail{Event: own.page.Events[0], SameRequest: []Event{}, SameRun: []Event{}}
	if d.Event.RequestID != nil {
		page, err := r.around(ctx, "request_id:"+*d.Event.RequestID, d.Event.at, sameRequestSpan, MaxRelated)
		if err != nil {
			return Detail{}, r.readFailed(ctx, own.p, this, err)
		}
		d.SameRequest = page.page.Events
	}
	if d.Event.RunID != nil {
		page, err := r.around(ctx, "run_id:"+*d.Event.RunID, d.Event.at, sameRunSpan, MaxRelated)
		if err != nil {
			return Detail{}, r.readFailed(ctx, own.p, this, err)
		}
		d.SameRun = page.page.Events
	}
	r.recordRead(ctx, own.p, this, 1)
	return d, nil
}

type nearby struct {
	p    Prepared
	page Page
}

func (r *Reader) around(ctx context.Context, text string, at time.Time, span time.Duration, limit int) (nearby, error) {
	q, err := filter.Parse(text)
	if err != nil {
		return nearby{}, err
	}
	p := Prepared{Text: text, Query: q, Window: filter.Window{From: at.Add(-span), To: at.Add(span)}}
	page, err := r.events(ctx, p, limit)
	return nearby{p: p, page: page}, err
}
