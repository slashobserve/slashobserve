package lifecycle_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

func TestAbortRefusalAnswers409WithTheAllowedActions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name    string
		refusal *lifecycle.Refusal
		body    string
	}{
		{
			"some allowed",
			&lifecycle.Refusal{Machine: "run", State: "running", Action: "retry", Allowed: []string{"cancel", "pin"}},
			`{"error":"retry is not allowed while the run is running","code":"invalid_transition","state":"running","allowed_actions":["cancel","pin"]}`,
		},
		{
			"none allowed",
			&lifecycle.Refusal{Machine: "run", State: "queued", Action: "retry", Allowed: []string{}},
			`{"error":"retry is not allowed while the run is queued","code":"invalid_transition","state":"queued","allowed_actions":[]}`,
		},
		{
			"nil is still an empty array",
			&lifecycle.Refusal{Machine: "run", State: "queued", Action: "retry"},
			`{"error":"retry is not allowed while the run is queued","code":"invalid_transition","state":"queued","allowed_actions":[]}`,
		},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		lifecycle.AbortRefusal(ctx, c.refusal)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: status %d", c.name, rec.Code)
		}
		if got := rec.Body.String(); got != c.body {
			t.Errorf("%s: body %s; want %s", c.name, got, c.body)
		}
	}
}
