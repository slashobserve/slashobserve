package trail

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
)

var now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

func TestPrepareDefaultsToTheLastDay(t *testing.T) {
	p, err := Prepare(Request{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Window.To.Equal(now) || !p.Window.From.Equal(now.Add(-DefaultSpan)) || p.Query.Form != filter.FormKQL {
		t.Fatalf("prepared = %+v", p)
	}
	p, err = Prepare(Request{Since: "now-7d", Until: "2026-11-20T00:00:00Z", Text: "lucene: outcome:denied"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Window.From.Equal(now.Add(-7*24*time.Hour)) || !p.Window.To.Equal(time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)) || p.Query.Form != filter.FormLucene {
		t.Fatalf("prepared = %+v", p)
	}
}

func TestPrepareRefusesABadWindow(t *testing.T) {
	for _, tc := range []struct {
		req  Request
		want string
	}{
		{Request{Since: "yesterday"}, `since "yesterday" is not a time`},
		{Request{Until: "soon"}, `until "soon" is not a time`},
		{Request{Since: "now", Until: "now-1h"}, "since must be before until"},
		{Request{Since: "now-367d"}, "longer than 366 days"},
		{Request{Since: strings.Repeat("1", MaxTimeBytes+1)}, "longer than 128 bytes"},
	} {
		_, err := Prepare(tc.req, now)
		problem, ok := ProblemOf(err)
		if !ok || problem.Status != http.StatusBadRequest || problem.Code != CodeInvalidRequest || !strings.Contains(problem.Message, tc.want) {
			t.Errorf("%+v: problem %+v (%v), want 400 with %q", tc.req, problem, err, tc.want)
		}
	}
	if _, err := Prepare(Request{Since: "now-366d"}, now); err != nil {
		t.Errorf("a 366 day window: %v", err)
	}
}

func TestAParseErrorIsA422WithOffsets(t *testing.T) {
	_, err := Prepare(Request{Text: "outcome:denied and status:x"}, now)
	problem, ok := ProblemOf(err)
	if !ok || problem.Status != http.StatusUnprocessableEntity || problem.Code != CodeQueryInvalid || problem.Start == nil || *problem.Start != 19 || *problem.End != 27 {
		t.Fatalf("problem = %+v", problem)
	}
}

func TestProblemOf(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{&QueryRefused{Msg: "too many rows"}, http.StatusUnprocessableEntity, CodeQueryInvalid},
		{ErrTimeout, http.StatusUnprocessableEntity, CodeQueryTimeout},
		{ErrNotFound, http.StatusNotFound, CodeNotFound},
		{errCursorForeign, http.StatusBadRequest, CodeInvalidRequest},
	} {
		problem, ok := ProblemOf(tc.err)
		if !ok || problem.Status != tc.status || problem.Code != tc.code || problem.Start != nil {
			t.Errorf("%v: %+v", tc.err, problem)
		}
	}
	if _, ok := ProblemOf(errors.New("connection reset")); ok {
		t.Error("an unexpected error became a problem")
	}
	if problem, _ := ProblemOf(&QueryRefused{Msg: "the buckets multiply to more than 100000 rows"}); problem.Message != "the buckets multiply to more than 100000 rows" {
		t.Errorf("a refusal's message changed: %q", problem.Message)
	}
}

func TestACursorPinsTheWindowOfItsFirstPage(t *testing.T) {
	first, err := Prepare(Request{Text: "outcome:denied", Since: "now-2h"}, now)
	if err != nil {
		t.Fatal(err)
	}
	at := now.Add(-time.Hour)
	id := uuid.NewString()
	next, err := Prepare(Request{Text: "outcome:denied", Since: "now-1m", Until: "now", Cursor: encodeCursor(first, at, id)}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if next.Window != first.Window || next.after == nil || !next.after.OccurredAt.Equal(at) || next.after.ID != id {
		t.Fatalf("next = %+v, after %+v; want the window %+v", next, next.after, first.Window)
	}
}

func TestACursorIsRefusedWithAnotherQueryOrWhenUnreadable(t *testing.T) {
	first, err := Prepare(Request{Text: "outcome:denied"}, now)
	if err != nil {
		t.Fatal(err)
	}
	good := encodeCursor(first, now.Add(-time.Minute), uuid.NewString())
	outside := encodeCursor(first, now.Add(time.Minute), uuid.NewString())
	for _, tc := range []struct {
		text, cursor string
		want         error
	}{
		{"outcome:failed", good, errCursorForeign},
		{"outcome:denied", "not base64!", errCursorUnreadable},
		{"outcome:denied", "e30", errCursorUnreadable},
		{"outcome:denied", outside, errCursorUnreadable},
		{"outcome:denied", strings.Repeat("a", MaxCursorBytes+1), errCursorUnreadable},
	} {
		if _, err := Prepare(Request{Text: tc.text, Cursor: tc.cursor}, now); !errors.Is(err, tc.want) {
			t.Errorf("q %q cursor %.20q: %v, want %v", tc.text, tc.cursor, err, tc.want)
		}
	}
}

func TestStepFor(t *testing.T) {
	for _, tc := range []struct {
		span    time.Duration
		buckets int
		want    time.Duration
	}{
		{time.Minute, 60, time.Second},
		{time.Hour, 60, time.Minute},
		{time.Hour, 200, 30 * time.Second},
		{24 * time.Hour, 60, 30 * time.Minute},
		{7 * 24 * time.Hour, 60, 3 * time.Hour},
		{31 * 24 * time.Hour, 60, 24 * time.Hour},
		{31 * 24 * time.Hour, 1, 7 * 24 * time.Hour},
		{366 * 24 * time.Hour, 60, 7 * 24 * time.Hour},
		{366 * 24 * time.Hour, MaxBuckets, 7 * 24 * time.Hour},
	} {
		w := filter.Window{From: now.Add(-tc.span), To: now}
		if got := StepFor(w, tc.buckets); got != tc.want {
			t.Errorf("StepFor(%s, %d) = %s, want %s", tc.span, tc.buckets, got, tc.want)
		}
	}
}

func TestTheCSVHeaderIsTheColumnOrderOfARead(t *testing.T) {
	q, err := filter.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	c, err := q.Rows(filter.Window{From: now.Add(-time.Hour), To: now}, filter.Page{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(csvHeader, c.Columns) {
		t.Fatalf("csv header %v, columns %v", csvHeader, c.Columns)
	}
	if got := len(Event{Data: []byte(`{}`)}.csvRecord()); got != len(csvHeader) {
		t.Fatalf("a record has %d fields, the header %d", got, len(csvHeader))
	}
}

func TestBucketsStartWhereTheDatabaseBinsThem(t *testing.T) {
	if got := binStart(now, 7*24*time.Hour); got != time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC) {
		t.Errorf("a week bucket starts %s, want the Thursday date_bin gives from the epoch", got)
	}
	if got := binStart(now.Add(90*time.Minute), time.Hour); got != now.Add(time.Hour) {
		t.Errorf("an hour bucket starts %s", got)
	}
}

func TestCutKeepsACursorAfterTheLastKeptEvent(t *testing.T) {
	p, err := Prepare(Request{}, now)
	if err != nil {
		t.Fatal(err)
	}
	events := []Event{{ID: uuid.NewString(), at: now.Add(-time.Minute)}, {ID: uuid.NewString(), at: now.Add(-2 * time.Minute)}, {ID: uuid.NewString(), at: now.Add(-3 * time.Minute)}}
	page := p.Cut(Page{Events: events}, 2)
	if len(page.Events) != 2 || page.NextCursor == "" {
		t.Fatalf("page = %+v", page)
	}
	next, err := Prepare(Request{Cursor: page.NextCursor}, now)
	if err != nil {
		t.Fatal(err)
	}
	if next.after.ID != events[1].ID || !next.after.OccurredAt.Equal(events[1].at) {
		t.Fatalf("after = %+v", next.after)
	}
	if whole := p.Cut(Page{Events: events}, 3); whole.NextCursor != "" {
		t.Fatalf("a cut that keeps every event set a cursor")
	}
}
