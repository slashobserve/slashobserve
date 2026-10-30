package dashboards

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func duplicateSource(t *testing.T, title string) Version {
	t.Helper()
	parsed, err := spec.Parse(panelsSpec(title, "rps", "errors"))
	if err != nil {
		t.Fatal(err)
	}
	return Version{
		VersionMeta: VersionMeta{Number: 4},
		Spec:        parsed,
		Unverified:  Unverified{Panels: []UnverifiedPanel{{PanelID: "errors"}}},
	}
}

func decodeObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDuplicateDraftChangesOnlyTheTitle(t *testing.T) {
	source := duplicateSource(t, "Checkout")
	before, err := json.Marshal(source.Spec)
	if err != nil {
		t.Fatal(err)
	}

	_, parsed, canonical, err := duplicateDraft(source, "Checkout copy")
	if err != nil {
		t.Fatal(err)
	}

	want := decodeObject(t, before)
	want["title"] = "Checkout copy"
	if got := decodeObject(t, canonical); !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical differs from the source beyond the title:\n got %v\nwant %v", got, want)
	}
	if parsed.Title != "Checkout copy" {
		t.Fatalf("parsed title = %q", parsed.Title)
	}
	if source.Spec.Title != "Checkout" {
		t.Fatalf("source title was mutated to %q", source.Spec.Title)
	}
}

func TestDuplicateDraftSummaryNamesSourceAndVersion(t *testing.T) {
	summary, _, _, err := duplicateDraft(duplicateSource(t, "Checkout"), "Checkout copy")
	if err != nil {
		t.Fatal(err)
	}
	if want := `Duplicated from "Checkout" version 4`; summary != want {
		t.Fatalf("summary = %q, want %q", summary, want)
	}
}

func TestDuplicateDraftRejectsBadTitles(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"201 characters":   strings.Repeat("a", 201),
		"201 wide letters": strings.Repeat("é", 201),
	}
	for name, title := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, _, err := duplicateDraft(duplicateSource(t, "Checkout"), title)
			var invalid *InvalidSpecError
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v, want *InvalidSpecError", err)
			}
			for _, issue := range invalid.Issues {
				if issue.Path == "/title" {
					return
				}
			}
			t.Fatalf("no issue at /title: %+v", invalid.Issues)
		})
	}
}

func TestDuplicateDraftAcceptsLongestTitles(t *testing.T) {
	long := strings.Repeat("a", 200)
	summary, _, _, err := duplicateDraft(duplicateSource(t, long), long)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(summary, `Duplicated from "`+long+`"`) {
		t.Fatalf("summary does not name the source: %q", summary)
	}
}

func duplicateBodyStatus(t *testing.T, body string) (int, string, bool, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/x/duplicate", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	title, folderID, ok := readDuplicate(c)
	return rec.Code, title, ok, folderID.String()
}

func TestReadDuplicateRejectsBadBodies(t *testing.T) {
	cases := map[string]string{
		"missing title":     `{}`,
		"bad folder id":     `{"title":"x","folder_id":"nope"}`,
		"unknown field":     `{"title":"x","extra":1}`,
		"title not string":  `{"title":3}`,
		"trailing document": `{"title":"x"}{"title":"y"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			code, _, ok, _ := duplicateBodyStatus(t, body)
			if ok || code != http.StatusBadRequest {
				t.Fatalf("ok = %v, status = %d, want a 400", ok, code)
			}
		})
	}
}

func TestReadDuplicateAcceptsTitleAndOptionalFolder(t *testing.T) {
	folder := "7d1c5c4e-3f0a-4a0b-9f64-0f4c2a6b8d11"
	parsedFolder, err := db.ParseUUID(folder)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("without folder", func(t *testing.T) {
		code, title, ok, got := duplicateBodyStatus(t, `{"title":"x"}`)
		if !ok || code != http.StatusOK || title != "x" {
			t.Fatalf("ok = %v, status = %d, title = %q", ok, code, title)
		}
		if got != "" {
			t.Fatalf("folder id should be unset, got %q", got)
		}
	})
	t.Run("with folder", func(t *testing.T) {
		code, title, ok, got := duplicateBodyStatus(t, `{"title":"x","folder_id":"`+folder+`"}`)
		if !ok || code != http.StatusOK || title != "x" {
			t.Fatalf("ok = %v, status = %d, title = %q", ok, code, title)
		}
		if got != parsedFolder.String() {
			t.Fatalf("folder id = %s, want %s", got, parsedFolder.String())
		}
	})
}
