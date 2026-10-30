package dashboards

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestDeletedEventPayloadCarriesOnlyTypeIDAndAuthor(t *testing.T) {
	id, err := db.ParseUUID("0b8f6f0e-3c9a-4a43-9d0a-6f2b0c1d2e3f")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := deletedEventPayload(id, Author{Kind: "user", ID: "u-1", Name: "Ada", Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["type"] != "dashboard.deleted" || got["dashboard_id"] != id.String() {
		t.Fatalf("payload = %s, want exactly type, dashboard_id and author", payload)
	}
	author, _ := got["author"].(map[string]any)
	if len(author) != 3 || author["kind"] != "user" || author["id"] != "u-1" || author["name"] != "Ada" {
		t.Fatalf("author = %v, want kind, id and name only", author)
	}
}

func TestDeletedEventPayloadCapsAuthorName(t *testing.T) {
	payload, err := deletedEventPayload(pgtype.UUID{}, Author{Kind: "user", ID: "u-1", Name: strings.Repeat("é", 300)})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Author struct {
			Name string `json:"name"`
		} `json:"author"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if n := utf8.RuneCountInString(got.Author.Name); n != 256 {
		t.Fatalf("author name has %d runes, want 256", n)
	}
}

func TestFailMapsDeleteOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantBody string
	}{
		{"dashboard still being created", ErrBeingCreated, http.StatusConflict, `"code":"dashboard_being_created"`},
		{"dashboard missing", ErrNotFound, http.StatusNotFound, `"code":"not_found"`},
		{"wrapped forbidden", fmt.Errorf("x: %w", folders.ErrForbidden), http.StatusForbidden, `"code":"forbidden_role"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/dashboards/x", nil)
			(&Service{logger: slog.New(slog.DiscardHandler)}).fail(c, "delete dashboard", tt.err)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want it to contain %s", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
