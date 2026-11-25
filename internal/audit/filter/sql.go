package filter

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultRows   = 100
	MaxRows       = 1000
	DefaultFacets = 10
	MaxFacets     = 100
)

var (
	ErrWindow = errors.New("the time window needs a start before its end")
	ErrStep   = errors.New("the step must be at least one millisecond")
)

type Window struct {
	From time.Time
	To   time.Time
}

type Cursor struct {
	OccurredAt time.Time
	ID         string
}

type Page struct {
	Limit     int
	Offset    int
	Ascending bool
	After     *Cursor
}

type Compiled struct {
	SQL     string
	Args    []any
	Columns []string
	Limit   int
}

type builder struct {
	args []any
}

func (b *builder) add(v any) string {
	b.args = append(b.args, v)
	return "$" + strconv.Itoa(len(b.args))
}

func (b *builder) where(q Query, w Window) (string, error) {
	if w.From.IsZero() || w.To.IsZero() || !w.From.Before(w.To) {
		return "", ErrWindow
	}
	conds := []string{
		"occurred_at >= " + b.add(w.From.UTC()),
		"occurred_at < " + b.add(w.To.UTC()),
	}
	if q.Clause != nil {
		sql, err := compileClause(b, q.Clause)
		if err != nil {
			return "", err
		}
		conds = append(conds, sql)
	}
	return strings.Join(conds, " AND "), nil
}

func (q Query) Rows(w Window, p Page) (Compiled, error) {
	var fromBody int
	if q.Size != nil {
		fromBody = *q.Size
	}
	limit, err := pageLimit(p.Limit, fromBody, DefaultRows, MaxRows)
	if err != nil {
		return Compiled{}, err
	}
	fetch := limit + 1
	if q.Size != nil && *q.Size == 0 {
		limit, fetch = 0, 0
	}
	b := &builder{}
	where, err := b.where(q, w)
	if err != nil {
		return Compiled{}, err
	}
	if p.Offset < 0 {
		return Compiled{}, errors.New("the offset cannot be negative")
	}
	direction, past := "DESC", "<"
	if p.Ascending {
		direction, past = "ASC", ">"
	}
	if p.After != nil {
		id, err := uuid.Parse(p.After.ID)
		if err != nil || p.After.OccurredAt.IsZero() {
			return Compiled{}, errors.New("the cursor needs the occurred_at time and the id of the last row")
		}
		where += " AND (occurred_at, id) " + past + " (" + b.add(p.After.OccurredAt.UTC()) + ", " + b.add(id.String()) + "::uuid)"
	}
	sql := "SELECT " + strings.Join(eventColumns, ", ") + " FROM " + tableName + " WHERE " + where +
		" ORDER BY occurred_at " + direction + ", id " + direction + " LIMIT " + b.add(fetch)
	if p.Offset > 0 {
		sql += " OFFSET " + b.add(p.Offset)
	}
	return Compiled{SQL: sql, Args: b.args, Columns: eventColumns, Limit: limit}, nil
}

func (q Query) Count(w Window, upTo int) (Compiled, error) {
	if upTo < 0 {
		return Compiled{}, errors.New("the count limit cannot be negative")
	}
	b := &builder{}
	where, err := b.where(q, w)
	if err != nil {
		return Compiled{}, err
	}
	matched := "SELECT 1 FROM " + tableName + " WHERE " + where
	if upTo > 0 {
		matched += " LIMIT " + b.add(upTo)
	}
	return Compiled{SQL: "SELECT count(*) FROM (" + matched + ") matched", Args: b.args, Columns: []string{"count"}, Limit: upTo}, nil
}

type Position struct {
	Xid uint64
	Seq int64
}

func (q Query) Follow(w Window, after Position, limit int) (Compiled, error) {
	if limit < 1 || limit > MaxRows {
		return Compiled{}, fmt.Errorf("a follow reads 1 to %d rows at a time", MaxRows)
	}
	b := &builder{}
	where, err := b.where(q, w)
	if err != nil {
		return Compiled{}, err
	}
	columns := append(slices.Clone(eventColumns), "xid::text")
	sql := "SELECT " + strings.Join(columns, ", ") + " FROM " + tableName + " WHERE " + where +
		" AND xid < pg_snapshot_xmin(pg_current_snapshot())" +
		" AND (xid, seq) > (" + b.add(strconv.FormatUint(after.Xid, 10)) + "::text::xid8, " + b.add(after.Seq) + "::bigint)" +
		" ORDER BY " + tableName + ".xid, " + tableName + ".seq LIMIT " + b.add(limit)
	return Compiled{SQL: sql, Args: b.args, Columns: columns, Limit: limit}, nil
}

func (q Query) Facets(w Window, field string, limit int) (Compiled, error) {
	f, ok := lookupField(field)
	if ok && !f.groupable() {
		return Compiled{}, fmt.Errorf("cannot count %q: it is not a field with values to count", truncate(field))
	}
	if !ok {
		return Compiled{}, fmt.Errorf("cannot count %q: %s", truncate(field), unknownFieldMessage(field))
	}
	limit, err := pageLimit(limit, 0, DefaultFacets, MaxFacets)
	if err != nil {
		return Compiled{}, err
	}
	b := &builder{}
	where, err := b.where(q, w)
	if err != nil {
		return Compiled{}, err
	}
	sql := "SELECT " + f.textExpr() + " AS value, count(*) AS n FROM " + tableName + " WHERE " + where +
		" AND " + f.column + " IS NOT NULL GROUP BY 1 ORDER BY n DESC, value ASC LIMIT " + b.add(limit)
	return Compiled{SQL: sql, Args: b.args, Columns: []string{"value", "n"}, Limit: limit}, nil
}

func pageLimit(asked, fromBody, fallback, ceiling int) (int, error) {
	switch {
	case asked < 0:
		return 0, errors.New("the limit cannot be negative")
	case asked == 0:
		asked = fromBody
	}
	if asked == 0 {
		asked = fallback
	}
	if asked > ceiling {
		return 0, fmt.Errorf("the limit is at most %d", ceiling)
	}
	return asked, nil
}
