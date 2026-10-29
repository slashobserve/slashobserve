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
		name    string
		general bool
		title   string
	}{
		{"a titled folder", false, "checkout"},
		{"the general folder", true, "general"},
		{"a title with punctuation and unicode", false, `été: "q" / 日本`},
		{"an empty title", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := encodeFolderCursor(tt.general, tt.title, mustUUID(t, cursorFolderID), updated, cursorDashboardID)
			got, err := decodeFolderCursor(raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.General != tt.general || got.Title != tt.title || got.FolderID != mustUUID(t, cursorFolderID) ||
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
		{"a missing general flag", encode(`{"t":"a","f":"` + cursorFolderID + `","u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a missing title", encode(`{"g":false,"f":"` + cursorFolderID + `","u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a missing folder", encode(`{"g":false,"t":"a","u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a missing time", encode(`{"g":false,"t":"a","f":"` + cursorFolderID + `","i":"` + cursorDashboardID + `"}`)},
		{"a missing dashboard", encode(`{"g":false,"t":"a","f":"` + cursorFolderID + `","u":1}`)},
		{"a folder that is not a UUID", encode(`{"g":false,"t":"a","f":"x","u":1,"i":"` + cursorDashboardID + `"}`)},
		{"a dashboard that is not a UUID", encode(`{"g":false,"t":"a","f":"` + cursorFolderID + `","u":1,"i":"x"}`)},
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
	folderCursor := encodeFolderCursor(false, "a", mustUUID(t, cursorFolderID), time.Now(), cursorDashboardID)
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
	}{
		{"defaults to recency", "", http.StatusOK, pgtype.UUID{}, false},
		{"order updated", "order=updated", http.StatusOK, pgtype.UUID{}, false},
		{"order folder", "order=folder", http.StatusOK, pgtype.UUID{}, true},
		{"a folder", "folder=" + cursorFolderID, http.StatusOK, mustUUID(t, cursorFolderID), false},
		{"a folder with the folder order", "folder=" + cursorFolderID + "&order=folder", http.StatusOK, mustUUID(t, cursorFolderID), true},
		{"an unknown order", "order=title", http.StatusBadRequest, pgtype.UUID{}, false},
		{"an empty order", "order=", http.StatusBadRequest, pgtype.UUID{}, false},
		{"a folder that is not a UUID", "folder=checkout", http.StatusBadRequest, pgtype.UUID{}, false},
		{"an empty folder", "folder=", http.StatusBadRequest, pgtype.UUID{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, params := listFilterStatus(t, tt.raw)
			if code != tt.wantStatus {
				t.Fatalf("status %d, want %d", code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK && (params.Folder != tt.wantFolder || params.ByFolder != tt.wantBy) {
				t.Fatalf("folder %v by folder %t, want %v and %t", params.Folder, params.ByFolder, tt.wantFolder, tt.wantBy)
			}
		})
	}
}
