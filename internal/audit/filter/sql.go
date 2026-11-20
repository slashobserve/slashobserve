package filter

import (
	"errors"
	"fmt"
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
	Limit int
	After *Cursor
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
	limit, err := pageLimit(p.Limit, q.Size, DefaultRows, MaxRows)
	if err != nil {
		return Compiled{}, err
	}
	b := &builder{}
	where, err := b.where(q, w)
	if err != nil {
		return Compiled{}, err
	}
	if p.After != nil {
		id, err := uuid.Parse(p.After.ID)
		if err != nil || p.After.OccurredAt.IsZero() {
			return Compiled{}, errors.New("the cursor needs the occurred_at time and the id of the last row")
		}
		where += " AND (occurred_at, id) < (" + b.add(p.After.OccurredAt.UTC()) + ", " + b.add(id.String()) + "::uuid)"
	}
	sql := "SELECT " + strings.Join(eventColumns, ", ") + " FROM " + tableName + " WHERE " + where +
		" ORDER BY occurred_at DESC, id DESC LIMIT " + b.add(limit+1)
	return Compiled{SQL: sql, Args: b.args, Columns: eventColumns, Limit: limit}, nil
}

func (q Query) Facets(w Window, field string, limit int) (Compiled, error) {
	f, ok := lookupField(field)
	if !ok || !f.groupable() {
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
