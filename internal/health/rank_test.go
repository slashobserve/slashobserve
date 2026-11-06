package health

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const cursorID = "0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61"

func TestRankCursorRoundTrips(t *testing.T) {
	cases := []struct {
		rank  int
		seen  time.Time
		title string
	}{
		{0, time.Date(2026, 11, 6, 9, 30, 15, 123456000, time.UTC), "Checkout"},
		{3, time.Unix(0, 0).UTC(), `Latency: p99 "slow" pods`},
	}
	for _, tc := range cases {
		got, ok := decodeRankCursor(encodeRankCursor(tc.rank, tc.seen, tc.title, cursorID))
		if !ok || got == nil {
			t.Fatalf("cursor for rank %d did not decode", tc.rank)
		}
		if got.Rank != tc.rank || !got.Seen.Equal(tc.seen) || got.Title != tc.title || got.ID.String() != cursorID {
			t.Errorf("decoded %+v, want rank %d seen %v title %q", got, tc.rank, tc.seen, tc.title)
		}
	}
}

func TestRankCursorIsAbsentOnTheFirstPage(t *testing.T) {
	got, ok := decodeRankCursor("")
	if !ok || got != nil {
		t.Errorf("empty cursor = %+v, %v", got, ok)
	}
}

func TestRankCursorRejectsMalformedCursors(t *testing.T) {
	encode := func(raw string) string { return base64.RawURLEncoding.EncodeToString([]byte(raw)) }
	cases := map[string]string{
		"rank 4":           encodeRankCursor(4, time.Unix(0, 0), "a", cursorID),
		"rank -1":          encodeRankCursor(-1, time.Unix(0, 0), "a", cursorID),
		"unknown field":    encode(`{"r":0,"s":0,"t":"a","i":"` + cursorID + `","x":1}`),
		"missing field":    encode(`{"r":0,"s":0,"t":"a"}`),
		"bad uuid":         encodeRankCursor(0, time.Unix(0, 0), "a", "not-a-uuid"),
		"negative seen":    encode(`{"r":0,"s":-1,"t":"a","i":"` + cursorID + `"}`),
		"trailing data":    encode(`{"r":0,"s":0,"t":"a","i":"` + cursorID + `"} {}`),
		"long title":       encodeRankCursor(0, time.Unix(0, 0), string(make([]byte, maxCursorTitleLen+1)), cursorID),
		"not base64":       "not a cursor!",
		"old time cursor":  httpapi.EncodeTimeCursor(time.Now(), cursorID),
		"padded base64url": base64.URLEncoding.EncodeToString([]byte(`{"r":0,"s":0,"t":"ab","i":"` + cursorID + `"}`)),
	}
	for name, raw := range cases {
		if got, ok := decodeRankCursor(raw); ok {
			t.Errorf("%s: decoded %+v", name, got)
		}
	}
}
