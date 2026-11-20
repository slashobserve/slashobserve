package trail

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	DefaultSpan    = 24 * time.Hour
	MaxSpan        = 366 * 24 * time.Hour
	MaxCursorBytes = 1024
	MaxTimeBytes   = 128

	cursorVersion = 1
	scopeHexLen   = 16
)

type Request struct {
	Text   string
	Since  string
	Until  string
	Cursor string
}

type Prepared struct {
	Text   string
	Query  filter.Query
	Window filter.Window
	after  *filter.Cursor
}

type InvalidRequest struct{ Msg string }

func (e *InvalidRequest) Error() string { return e.Msg }

func invalidf(format string, args ...any) error {
	return &InvalidRequest{Msg: fmt.Sprintf(format, args...)}
}

func Prepare(req Request, now time.Time) (Prepared, error) {
	q, err := filter.Parse(req.Text)
	if err != nil {
		return Prepared{}, err
	}
	p := Prepared{Text: req.Text, Query: q}
	if req.Cursor != "" {
		c, err := decodeCursor(req.Cursor, scopeOf(req.Text))
		if err != nil {
			return Prepared{}, err
		}
		p.Window, p.after = c.window(), &filter.Cursor{OccurredAt: c.at(), ID: c.ID}
		return p, nil
	}
	if p.Window, err = resolveWindow(req.Since, req.Until, now); err != nil {
		return Prepared{}, err
	}
	return p, nil
}

func resolveWindow(since, until string, now time.Time) (filter.Window, error) {
	to := now.UTC()
	if until != "" {
		t, err := resolveTime("until", until, now, true)
		if err != nil {
			return filter.Window{}, err
		}
		to = t
	}
	from := to.Add(-DefaultSpan)
	if since != "" {
		t, err := resolveTime("since", since, now, false)
		if err != nil {
			return filter.Window{}, err
		}
		from = t
	}
	w := filter.Window{From: from.Truncate(time.Microsecond), To: to.Truncate(time.Microsecond)}
	return w, checkWindow(w)
}

func resolveTime(name, expr string, now time.Time, roundUp bool) (time.Time, error) {
	if len(expr) > MaxTimeBytes {
		return time.Time{}, invalidf("%s is longer than %d bytes", name, MaxTimeBytes)
	}
	t, err := spec.ResolveTime(expr, now, roundUp, spec.TimeOptions{})
	if err != nil {
		return time.Time{}, invalidf("%s %q is not a time: use an RFC 3339 time, epoch milliseconds or a relative time such as now-24h", name, expr)
	}
	return t.UTC(), nil
}

func checkWindow(w filter.Window) error {
	switch {
	case !w.From.Before(w.To):
		return invalidf("since must be before until")
	case w.To.Sub(w.From) > MaxSpan:
		return invalidf("the window from since to until is longer than %d days; search a shorter window", int(MaxSpan/(24*time.Hour)))
	}
	return nil
}

type cursor struct {
	Version int    `json:"v"`
	Scope   string `json:"scope"`
	FromUs  int64  `json:"from_us"`
	ToUs    int64  `json:"to_us"`
	AtUs    int64  `json:"at_us"`
	ID      string `json:"id"`
}

func (c cursor) window() filter.Window {
	return filter.Window{From: time.UnixMicro(c.FromUs).UTC(), To: time.UnixMicro(c.ToUs).UTC()}
}

func (c cursor) at() time.Time {
	return time.UnixMicro(c.AtUs).UTC()
}

var (
	errCursorUnreadable = &InvalidRequest{Msg: "cursor is not readable; start again without it"}
	errCursorForeign    = &InvalidRequest{Msg: "cursor belongs to another query; send the q it was returned with, or start again without it"}
)

func encodeCursor(p Prepared, at time.Time, id string) string {
	raw, err := json.Marshal(cursor{
		Version: cursorVersion, Scope: scopeOf(p.Text),
		FromUs: p.Window.From.UnixMicro(), ToUs: p.Window.To.UnixMicro(), AtUs: at.UnixMicro(), ID: id,
	})
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(s, scope string) (cursor, error) {
	if len(s) > MaxCursorBytes {
		return cursor{}, errCursorUnreadable
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursor{}, errCursorUnreadable
	}
	var c cursor
	if err := json.Unmarshal(raw, &c); err != nil || c.Version != cursorVersion {
		return cursor{}, errCursorUnreadable
	}
	if c.Scope != scope {
		return cursor{}, errCursorForeign
	}
	if _, err := uuid.Parse(c.ID); err != nil {
		return cursor{}, errCursorUnreadable
	}
	w := c.window()
	if checkWindow(w) != nil || c.AtUs < c.FromUs || c.AtUs >= c.ToUs {
		return cursor{}, errCursorUnreadable
	}
	return c, nil
}

func scopeOf(text string) string {
	sum := sha256.Sum256([]byte(strconv.Itoa(cursorVersion) + "\n" + text))
	return hex.EncodeToString(sum[:])[:scopeHexLen]
}

var ErrNotFound = errors.New("audit event not found")

type QueryRefused struct{ Msg string }

func (e *QueryRefused) Error() string { return e.Msg }

var ErrTimeout = errors.New("the audit query ran longer than the statement timeout; narrow the window or the filter")
