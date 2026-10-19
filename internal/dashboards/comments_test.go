package dashboards

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
)

const notifyPayloadLimit = 8000

func TestCommentEventPayloadStaysUnderNotifyLimit(t *testing.T) {
	id, err := db.ParseUUID("11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	row := dbsqlc.DashboardComment{ID: id, DashboardID: id, PanelID: pgtype.Text{String: strings.Repeat("\x01", maxPanelIDRunes), Valid: true}, ParentID: id, Status: CommentOpen}
	actor := Author{Kind: "service_account", ID: id.String(), Name: strings.Repeat("\x01", 100000)}
	for _, eventType := range []string{events.TypeCommentCreated, events.TypeCommentUpdated} {
		payload, err := commentEventPayload(eventType, row, CommentResolved, actor)
		if err != nil {
			t.Fatal(err)
		}
		if len(payload) >= notifyPayloadLimit/2 {
			t.Errorf("%s payload is %d bytes, want well under %d", eventType, len(payload), notifyPayloadLimit)
		}
	}
}

func TestCheckCommentBody(t *testing.T) {
	for name, in := range map[string]string{
		"padded":               "  hello \n",
		"4000 multibyte runes": strings.Repeat("\u00e9", maxCommentRunes),
		"tab newline CR":       "a\tb\r\nc",
		"ZWJ sequence":         "\U0001F469\u200d\U0001F4BB",
		"bidi marks":           "a\u200eb\u200fc\u061cd",
	} {
		got, err := checkCommentBody(in)
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if got != strings.TrimSpace(in) {
			t.Errorf("%s: got %q", name, got)
		}
	}
	invalid := map[string]string{
		"empty":           "",
		"whitespace only": " \t\n ",
		"NUL":             "a\x00b",
		"C0 bell":         "a\x07b",
		"escape":          "a\x1bb",
		"DEL":             "a\x7fb",
		"C1 CSI":          "a\u009bb",
		"C1 NEL inside":   "a\u0085b",
		"embedding start": "a\u202ab",
		"override":        "a\u202eb",
		"isolate start":   "a\u2066b",
		"isolate end":     "a\u2069b",
		"too long":        strings.Repeat("a", maxCommentRunes+1),
		"invalid UTF-8":   "a\xffb",
	}
	for name, in := range invalid {
		var target *InvalidInputError
		if _, err := checkCommentBody(in); !errors.As(err, &target) {
			t.Errorf("%s: got %v, want InvalidInputError", name, err)
		}
	}
}

func TestPanelCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := newPanelCache()
	key := func(n int) panelKey { return panelKey{version: n} }
	for n := 1; n <= panelCacheSize; n++ {
		cache.put(key(n), map[string]struct{}{})
	}
	if _, ok := cache.get(key(1)); !ok {
		t.Fatal("first entry evicted before the cache was full")
	}
	cache.put(key(panelCacheSize+1), map[string]struct{}{})
	if _, ok := cache.get(key(2)); ok {
		t.Error("least recently used entry survived")
	}
	if _, ok := cache.get(key(1)); !ok {
		t.Error("recently used entry was evicted")
	}
}
