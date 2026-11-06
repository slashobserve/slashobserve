package revisions

import (
	"errors"
	"testing"
	"time"
)

func TestInvestigateWindowResolvesBothEnds(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	text := func(s string) *string { return &s }
	cases := []struct {
		name     string
		from, to *string
		want     error
		wantFrom time.Time
	}{
		{"neither", nil, nil, nil, time.Time{}},
		{"relative", text("now-1h"), text("now"), nil, now.Add(-time.Hour)},
		{"epoch milliseconds", text("1761987600000"), text("1761991200000"), nil, time.UnixMilli(1761987600000).UTC()},
		{"only from", text("now-1h"), nil, errWindowPair, time.Time{}},
		{"only to", nil, text("now"), errWindowPair, time.Time{}},
		{"not a time", text("yesterday"), text("now"), errWindowTime, time.Time{}},
		{"reversed", text("now"), text("now-1h"), errWindowOrder, time.Time{}},
		{"equal", text("now-1h"), text("now-1h"), errWindowOrder, time.Time{}},
		{"too long", text("now-401d"), text("now"), errWindowTooLong, time.Time{}},
	}
	for _, c := range cases {
		from, to, err := investigateWindow(c.from, c.to, now)
		switch {
		case !errors.Is(err, c.want):
			t.Errorf("%s: error %v, want %v", c.name, err, c.want)
		case c.want == nil && c.from == nil && (from != nil || to != nil):
			t.Errorf("%s: got a window %v to %v", c.name, from, to)
		case c.want == nil && c.from != nil && (from == nil || !from.Equal(c.wantFrom) || from.Location() != time.UTC):
			t.Errorf("%s: from = %v, want %v in UTC", c.name, from, c.wantFrom)
		}
	}
}

func TestListCursorsKeepWhetherTheLastRowWasPinned(t *testing.T) {
	at := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	const id = "11111111-1111-4111-8111-111111111111"
	for _, pinned := range []bool{true, false} {
		gotPinned, after, ok := decodeListCursor(encodeListCursor(pinned, at, id))
		if !ok || gotPinned != pinned || after == nil || !after.Time.Equal(at) || after.ID.String() != id {
			t.Errorf("pinned %v: decoded %v %+v %v", pinned, gotPinned, after, ok)
		}
	}
	if pinned, after, ok := decodeListCursor(encodeListCursor(false, at, id)[len(unpinnedCursorPrefix):]); !ok || pinned || after == nil {
		t.Errorf("an unprefixed cursor: %v %+v %v", pinned, after, ok)
	}
	if _, _, ok := decodeListCursor("p.!!"); ok {
		t.Errorf("a broken pinned cursor decoded")
	}
}

func TestEscapeLikeTreatsWildcardsLiterally(t *testing.T) {
	if got, want := escapeLike(`50%_off\`), `50\%\_off\\`; got != want {
		t.Fatalf("escapeLike = %q, want %q", got, want)
	}
}
