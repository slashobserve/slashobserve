package folders_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAnAnswerThatReadAdminOnlyDataIsShownOnlyInAFolderOnlyAdminsRead(t *testing.T) {
	for _, tc := range []struct {
		restricted, adminsOnly bool
		refused                bool
	}{
		{false, false, false},
		{false, true, false},
		{true, true, false},
		{true, false, true},
	} {
		if err := folders.ShowAnswer(tc.restricted, tc.adminsOnly); (err != nil) != tc.refused {
			t.Errorf("restricted %v, Only Admins folder %v: err = %v, want refused %v", tc.restricted, tc.adminsOnly, err, tc.refused)
		}
	}
}

func TestARestrictedAnswerRefusalAnswersRestrictedAnswerWithWhy(t *testing.T) {
	body := abortBody(t, fmt.Errorf("share: %w", folders.ErrRestrictedAnswer))
	message, _ := body["error"].(string)
	if body["code"] != folders.CodeRestrictedAnswer || !strings.Contains(message, "only Admins") || !strings.Contains(message, "private chat") {
		t.Fatalf("body = %v", body)
	}
}

func TestRestrictedAnswerMoveNamesTheAnswer(t *testing.T) {
	err := folders.RestrictedAnswerMove(folders.ContentObservation, folders.AnswerRef{RunID: "ede0314a", Prompt: "is anything\nbroken   right now?"})
	if !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("err = %v, want a restricted answer", err)
	}
	want := `this observation holds the answer to "is anything broken right now?" (run ede0314a) that read data only Admins can query`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("message = %q, want it to contain %q", err.Error(), want)
	}
}

func TestRestrictedAnswerMoveShortensALongQuestion(t *testing.T) {
	err := folders.RestrictedAnswerMove(folders.ContentDashboard, folders.AnswerRef{RunID: "r1", Prompt: strings.Repeat("é", 200)})
	if !strings.Contains(err.Error(), strings.Repeat("é", 79)+`…" (run r1)`) {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestRestrictedAnswerMoveWithoutARunStaysGeneral(t *testing.T) {
	err := folders.RestrictedAnswerMove(folders.ContentDashboard, folders.AnswerRef{})
	if !strings.Contains(err.Error(), "this dashboard holds an answer that read data only Admins can query") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestRestrictedAnswerRefusalTellsAnAdminWhatToDo(t *testing.T) {
	msg := folders.ErrRestrictedAnswer.Error()
	if strings.Contains(strings.ToLower(msg), "ask an admin") || !strings.Contains(msg, "Audit folder") {
		t.Fatalf("message = %q", msg)
	}
}
