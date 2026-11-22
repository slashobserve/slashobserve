package revisions

import (
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestCheckAsOf(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	admin := auth.Principal{Kind: auth.KindServiceAccount, Role: auth.RoleAdmin}
	editor := auth.Principal{Kind: auth.KindUser, Role: auth.RoleEditor}
	past := time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		raw  string
		by   auth.Principal
		want time.Time
		err  error
	}{
		{"rfc 3339", "2026-11-20T16:00:00+05:30", admin, past, nil},
		{"epoch milliseconds", "1795170600000", admin, past, nil},
		{"an editor", "2026-11-20T10:30:00Z", editor, time.Time{}, ErrForbidden},
		{"a relative time", "now-1h", admin, time.Time{}, errAsOfTime},
		{"text", "yesterday", admin, time.Time{}, errAsOfTime},
		{"the future", "2026-11-24T00:00:00Z", admin, time.Time{}, errAsOfFuture},
	} {
		got, err := CheckAsOf(tc.raw, tc.by, now)
		if !errors.Is(err, tc.err) || !got.Equal(tc.want) {
			t.Errorf("%s: CheckAsOf(%q) = %s, %v; want %s, %v", tc.name, tc.raw, got, err, tc.want, tc.err)
		}
		if err == nil && got.Location() != time.UTC {
			t.Errorf("%s: as_of is in %s, want UTC", tc.name, got.Location())
		}
	}
}

func TestTheRunClockIsAsOfWhenSet(t *testing.T) {
	at := time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
	ctx := WithAsOf(t.Context(), at)
	if got := clockOf(ctx); !got.Equal(at) {
		t.Fatalf("clock = %s, want %s", got, at)
	}
	if col := asOfColumn(ctx); !col.Valid || !col.Time.Equal(at) {
		t.Fatalf("column = %+v", col)
	}
	if col := asOfColumn(t.Context()); col.Valid {
		t.Fatalf("a live run stores as_of %+v", col)
	}
}
