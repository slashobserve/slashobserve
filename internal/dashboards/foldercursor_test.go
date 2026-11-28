package dashboards

import (
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const (
	cursorFolderID    = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	cursorDashboardID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestFolderCursorRoundTrips(t *testing.T) {
	updated := time.Date(2026, 10, 30, 12, 30, 45, 123456000, time.UTC)
	tests := []struct {
		name     string
		personal bool
		general  bool
		title    string
	}{
		{"a titled folder", false, false, "checkout"},
		{"the general folder", false, true, "general"},
		{"a personal folder", true, false, "My Dashboards"},
		{"a title with punctuation and unicode", false, false, `été: "q" / 日本`},
		{"an empty title", false, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := encodeFolderCursor(tt.personal, tt.general, tt.title, mustUUID(t, cursorFolderID), updated, cursorDashboardID)
			got, err := decodeFolderCursor(raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.Personal != tt.personal || got.General != tt.general || got.Title != tt.title || got.FolderID != mustUUID(t, cursorFolderID) ||
				!got.UpdatedAt.Equal(updated) || got.ID != mustUUID(t, cursorDashboardID) {
				t.Fatalf("decoded %+v", got)
			}
		})
	}
}

func TestFolderCursorRejectsCursorsOfOtherShapes(t *testing.T) {
	timeCursor := httpapi.EncodeTimeCursor(time.Now(), cursorDashboardID)
	encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tests := []struct {
		name string
		raw  string
	}{
		{"a time cursor", timeCursor},
		{"garbage", "!!not-base64!!"},
		{"valid base64 that is not JSON", encode("hello")},
		{"a JSON array", encode(`[1,2]`)},
		{"a missing personal flag", encode(`{"g":false,"t":"a","f":"` + cursorFolderID + `","u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a missing general flag", encode(`{"p":false,"t":"a","f":"` + cursorFolderID + `","u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a missing title", encode(`{"p":false,"g":false,"f":"` + cursorFolderID + `","u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a missing folder", encode(`{"p":false,"g":false,"t":"a","u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a missing time", encode(`{"p":false,"g":false,"t":"a","f":"` + cursorFolderID + `","i":"` + cursorDashboardID + `"}`)},
		{"a missing dashboard", encode(`{"p":false,"g":false,"t":"a","f":"` + cursorFolderID + `","u":1}`)},
		{"a folder that is not a UUID", encode(`{"p":false,"g":false,"t":"a","f":"x","u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a dashboard that is not a UUID", encode(`{"p":false,"g":false,"t":"a","f":"` + cursorFolderID + `","u":1,"i":"x"}`)},
		{"an empty cursor", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeFolderCursor(tt.raw); !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("err = %v, want ErrInvalidCursor", err)
			}
		})
	}
}

func TestFolderCursorIsRejectedAsATimeCursor(t *testing.T) {
	folderCursor := encodeFolderCursor(false, false, "a", mustUUID(t, cursorFolderID), time.Now(), cursorDashboardID)
	if _, ok := httpapi.DecodeTimeCursor(folderCursor); ok {
		t.Fatal("a folder cursor was accepted as a time cursor")
	}
}

func TestListFiltersParseFolderAndOrder(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantStatus int
		wantFolder pgtype.UUID
		wantBy     bool
		wantName   bool
		wantViews  bool
	}{
		{"defaults to recency", "", http.StatusOK, pgtype.UUID{}, false, false, false},
		{"order updated", "order=updated", http.StatusOK, pgtype.UUID{}, false, false, false},
		{"order folder", "order=folder", http.StatusOK, pgtype.UUID{}, true, false, false},
		{"order views", "order=views", http.StatusOK, pgtype.UUID{}, false, false, true},
		{"order name", "order=name", http.StatusOK, pgtype.UUID{}, false, true, false},
		{"a folder", "folder=" + cursorFolderID, http.StatusOK, mustUUID(t, cursorFolderID), false, false, false},
		{"a folder with the folder order", "folder=" + cursorFolderID + "&order=folder", http.StatusOK, mustUUID(t, cursorFolderID), true, false, false},
		{"an unknown order", "order=title", http.StatusBadRequest, pgtype.UUID{}, false, false, false},
		{"an empty order", "order=", http.StatusBadRequest, pgtype.UUID{}, false, false, false},
		{"a folder that is not a UUID", "folder=checkout", http.StatusBadRequest, pgtype.UUID{}, false, false, false},
		{"an empty folder", "folder=", http.StatusBadRequest, pgtype.UUID{}, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, params := listFilterStatus(t, tt.raw)
			if code != tt.wantStatus {
				t.Fatalf("status %d, want %d", code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK && (params.Folder != tt.wantFolder || params.ByFolder != tt.wantBy || params.ByName != tt.wantName || params.ByViews != tt.wantViews) {
				t.Fatalf("folder %v, by folder %t, by name %t, by views %t, want %v, %t, %t and %t", params.Folder, params.ByFolder, params.ByName, params.ByViews, tt.wantFolder, tt.wantBy, tt.wantName, tt.wantViews)
			}
		})
	}
}

func TestViewsCursorRoundTrips(t *testing.T) {
	updated := time.Date(2026, 10, 30, 12, 30, 45, 123456000, time.UTC)
	got, err := decodeViewsCursor(encodeViewsCursor(7, updated, cursorDashboardID))
	if err != nil {
		t.Fatal(err)
	}
	if got.Views != 7 || !got.UpdatedAt.Equal(updated) || got.ID != mustUUID(t, cursorDashboardID) {
		t.Fatalf("decoded %+v", got)
	}
}

func TestViewsCursorRejectsCursorsOfOtherShapes(t *testing.T) {
	encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tests := []struct {
		name string
		raw  string
	}{
		{"a time cursor", httpapi.EncodeTimeCursor(time.Now(), cursorDashboardID)},
		{"a folder cursor", encodeFolderCursor(false, false, "a", mustUUID(t, cursorFolderID), time.Now(), cursorDashboardID)},
		{"garbage", "!!not-base64!!"},
		{"a missing count", encode(`{"u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a negative count", encode(`{"v":-1,"u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a missing time", encode(`{"v":1,"i":"` + cursorDashboardID + `"}`)},
		{"a dashboard that is not a UUID", encode(`{"v":1,"u":1,"i":"x"}`)},
		{"an empty cursor", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeViewsCursor(tt.raw); !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("err = %v, want ErrInvalidCursor", err)
			}
		})
	}
}

func TestViewsCursorIsRejectedAsATimeCursorAndAFolderCursor(t *testing.T) {
	cursor := encodeViewsCursor(1, time.Now(), cursorDashboardID)
	if _, ok := httpapi.DecodeTimeCursor(cursor); ok {
		t.Error("a views cursor was accepted as a time cursor")
	}
	if _, err := decodeFolderCursor(cursor); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("a views cursor was accepted as a folder cursor: %v", err)
	}
}

func TestNameCursorIsRejectedAsATimeViewsAndFolderCursor(t *testing.T) {
	cursor := encodeNameCursor("checkout", cursorDashboardID)
	if _, ok := httpapi.DecodeTimeCursor(cursor); ok {
		t.Error("a name cursor was accepted as a time cursor")
	}
	if _, err := decodeViewsCursor(cursor); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("a name cursor was accepted as a views cursor: %v", err)
	}
	if _, err := decodeFolderCursor(cursor); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("a name cursor was accepted as a folder cursor: %v", err)
	}
}
