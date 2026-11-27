package folders

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func restrictedRefusalBody(t *testing.T, refusal error) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/x/move", nil)
	if !AbortRestricted(c, refusal) {
		t.Fatalf("%v was not refused", refusal)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	if rec.Code != http.StatusConflict || body["code"] != CodeRestrictedAnswer || body["error"] != refusal.Error() {
		t.Fatalf("refusal = %d %v", rec.Code, body)
	}
	return body
}

func TestARestrictedAnswerRefusalSaysWhetherWaitingHelps(t *testing.T) {
	item := []ContentItem{{Kind: ContentDashboard, Title: "Checkout"}}
	cases := []struct {
		name    string
		refusal error
		reason  string
	}{
		{"a move of content that holds a kept answer", RestrictedAnswerMove(ContentDashboard, AnswerRef{}), "permanent"},
		{"a copy of content that holds a kept answer", RestrictedAnswerCopy(ContentObservation), "permanent"},
		{"opening a folder that holds a kept answer", RestrictedAnswerOpening(item), "permanent"},
		{"a write by a run that read Admin-only data", ErrRestrictedAnswer, "permanent"},
		{"a move while a run is in flight", RunInFlightMove(ContentObservation), "run_in_flight"},
		{"opening a folder while a run is in flight", RunInFlightOpening(item), "run_in_flight"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := restrictedRefusalBody(t, tc.refusal)
			if body["reason"] != tc.reason {
				t.Errorf("reason = %v, want %s", body["reason"], tc.reason)
			}
		})
	}
}

func TestARunInFlightRefusalSaysToWaitForTheRunToEnd(t *testing.T) {
	item := []ContentItem{{Kind: ContentDashboard, Title: "Checkout"}}
	for _, refusal := range []error{RunInFlightMove(ContentDashboard), RunInFlightMove(ContentObservation), RunInFlightOpening(item), RunInFlightOpening(append(item, item...))} {
		message, _ := restrictedRefusalBody(t, refusal)["error"].(string)
		if !strings.Contains(message, "not finished") || !strings.Contains(message, "to end before you") {
			t.Errorf("message %q does not say to wait for the run to end", message)
		}
	}
}

func TestAKeptAnswerConfinesContentForGoodAndARunInFlightUntilItEnds(t *testing.T) {
	cases := []struct {
		name                 string
		keptAnswer, inFlight bool
		want                 Confinement
	}{
		{"neither", false, false, NotConfined},
		{"a run in flight", false, true, ConfinedRunInFlight},
		{"a kept answer", true, false, ConfinedPermanent},
		{"a kept answer and a run in flight", true, true, ConfinedPermanent},
	}
	for _, tc := range cases {
		if got := ConfinementOf(tc.keptAnswer, tc.inFlight); got != tc.want {
			t.Errorf("%s: confinement = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEveryConfinementIsNamedOnce(t *testing.T) {
	if got := Confinements.Names(); !slices.Equal(got, []string{string(ConfinedPermanent), string(ConfinedRunInFlight)}) {
		t.Errorf("Confinements = %v", got)
	}
}
