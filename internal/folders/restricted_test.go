package folders_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/folders"
)

func abortBody(t *testing.T, err error) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	if !folders.AbortRestricted(c, err) {
		t.Fatalf("AbortRestricted refused to handle %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestAudienceRefusalCountsTheSharedItemsItDoesNotList(t *testing.T) {
	items := []folders.ContentItem{{Kind: folders.ContentDashboard, ID: "d1", Title: "Checkout", FolderID: "f1", FolderTitle: "Team"}}
	body := abortBody(t, folders.RestrictedAudience("payroll", items, 7, 2))
	if body["unlisted"] != float64(7) || body["hidden"] != float64(2) {
		t.Fatalf("unlisted = %v, hidden = %v", body["unlisted"], body["hidden"])
	}
}

func TestAudienceRefusalWithACompleteListCountsZeroUnlisted(t *testing.T) {
	items := []folders.ContentItem{{Kind: folders.ContentDashboard, ID: "d1", Title: "Checkout", FolderID: "f1", FolderTitle: "Team"}}
	body := abortBody(t, folders.RestrictedAudience("payroll", items, 0, 0))
	if got, ok := body["unlisted"]; !ok || got != float64(0) {
		t.Fatalf("unlisted = %v, present = %v", got, ok)
	}
}

func TestRefusalWithoutItemsCarriesNoCounts(t *testing.T) {
	body := abortBody(t, folders.RestrictedSave(folders.ContentDashboard, "payroll"))
	if _, ok := body["unlisted"]; ok {
		t.Fatalf("body = %v", body)
	}
}
