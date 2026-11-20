package dashboards

import (
	"errors"
	"testing"
	"time"
)

func TestNameCursorRoundTrips(t *testing.T) {
	for _, title := range []string{"checkout latency", `été: "q" / 日本`, ""} {
		raw := encodeNameCursor(title, cursorDashboardID)
		got, err := decodeNameCursor(raw)
		if err != nil {
			t.Fatalf("%q: %v", title, err)
		}
		if got.Title != title || got.ID.String() != cursorDashboardID {
			t.Errorf("%q: got %+v", title, got)
		}
	}
}

func TestNameCursorRejectsOtherCursors(t *testing.T) {
	other := encodeViewsCursor(3, time.Now(), cursorDashboardID)
	for name, raw := range map[string]string{
		"not base64":      "%%%",
		"not json":        "bm90IGpzb24",
		"a views cursor":  other,
		"missing the id":  "eyJuIjoiYSJ9",
		"an invalid uuid": encodeNameCursor("a", "nope"),
	} {
		if _, err := decodeNameCursor(raw); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("%s: err = %v, want ErrInvalidCursor", name, err)
		}
	}
}
