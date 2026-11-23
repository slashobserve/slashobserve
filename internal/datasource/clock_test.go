package datasource

import (
	"testing"
	"time"
)

func TestNowIsThePinnedClockWhenSet(t *testing.T) {
	at := time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
	ctx := WithNow(t.Context(), at)
	if got := Now(ctx); !got.Equal(at) {
		t.Fatalf("Now = %s, want %s", got, at)
	}
	if got, ok := PinnedNow(ctx); !ok || !got.Equal(at) {
		t.Fatalf("PinnedNow = %s, %v", got, ok)
	}
	if _, ok := PinnedNow(t.Context()); ok {
		t.Fatal("a plain context is pinned")
	}
	if _, ok := PinnedNow(WithNow(t.Context(), time.Time{})); ok {
		t.Fatal("a zero time pins the clock")
	}
	if d := time.Since(Now(t.Context())); d < 0 || d > time.Minute {
		t.Fatalf("a plain context's Now is %s old", d)
	}
}
