package healthrule

import (
	"testing"
	"time"
)

func TestValidKindAcceptsOnlyTheFiveKinds(t *testing.T) {
	for _, k := range Kinds {
		if got, ok := ValidKind(string(k)); !ok || got != k {
			t.Errorf("ValidKind(%q) = %q, %v", k, got, ok)
		}
	}
	for _, raw := range []string{"", "any", "slow", "No_Data", " no_data"} {
		if got, ok := ValidKind(raw); ok {
			t.Errorf("ValidKind(%q) = %q, true", raw, got)
		}
	}
}

func TestValidFilterAcceptsAnyAndEveryKind(t *testing.T) {
	for _, raw := range []string{"any", "no_data", "query_failed", "variable_failed", "duplicate_title", "unopened"} {
		if got, ok := ValidFilter(raw); !ok || got != raw {
			t.Errorf("ValidFilter(%q) = %q, %v", raw, got, ok)
		}
	}
	for _, raw := range []string{"", "bogus", "ANY", "any "} {
		if got, ok := ValidFilter(raw); ok {
			t.Errorf("ValidFilter(%q) = %q, true", raw, got)
		}
	}
}

func TestUnopenedBeforeIsNinetyDaysAgo(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	want := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	if got := UnopenedBefore(now); !got.Equal(want) {
		t.Errorf("UnopenedBefore(%v) = %v, want %v", now, got, want)
	}
}

func TestViewsSinceIsTheStartOfTheThirtyDayWindowIncludingToday(t *testing.T) {
	now := time.Date(2026, 11, 19, 23, 30, 0, 0, time.UTC)
	want := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	if got := ViewsSince(now); !got.Equal(want) {
		t.Errorf("ViewsSince(%v) = %v, want %v", now, got, want)
	}
}

func TestViewsSinceReadsTheUTCDayOfAZonedTime(t *testing.T) {
	zone := time.FixedZone("late", -10*3600)
	now := time.Date(2026, 11, 19, 20, 0, 0, 0, zone)
	want := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	if got := ViewsSince(now); !got.Equal(want) {
		t.Errorf("ViewsSince(%v) = %v, want %v", now, got, want)
	}
}

func TestViewsKeptSinceIsNinetyDaysBack(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	want := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	if got := ViewsKeptSince(now); !got.Equal(want) {
		t.Errorf("ViewsKeptSince(%v) = %v, want %v", now, got, want)
	}
}
