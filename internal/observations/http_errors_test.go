package observations

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestAHiddenDashboardAnswersTheSame404AsAnUnknownOne(t *testing.T) {
	gin.SetMode(gin.TestMode)
	answer := func(err error) (int, string) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		if !abortKnown(c, fmt.Errorf("wrapped: %w", err)) {
			t.Fatalf("abortKnown left %v unanswered", err)
		}
		return rec.Code, rec.Body.String()
	}
	hiddenStatus, _ := answer(folders.ErrNoAccess)
	unknownStatus, _ := answer(ErrNotFound)
	if hiddenStatus != http.StatusNotFound || unknownStatus != http.StatusNotFound {
		t.Fatalf("hidden %d, unknown %d, want 404 for both", hiddenStatus, unknownStatus)
	}
	_, hiddenBody := answer(folders.ErrNoAccess)
	_, unknownBody := answer(ErrNotFound)
	if hiddenBody != unknownBody {
		t.Fatalf("hidden body %s differs from unknown body %s", hiddenBody, unknownBody)
	}
}
