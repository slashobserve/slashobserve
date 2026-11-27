package dashboards

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestSaveOutcomeNamesWhatTheSaveDid(t *testing.T) {
	merged := &MergedFrom{BaseVersion: 1, OntoVersion: 2}
	cases := []struct {
		name string
		res  SaveResult
		want string
	}{
		{"a new version", SaveResult{}, OutcomeCreated},
		{"a repeat of the latest version", SaveResult{Replayed: true}, OutcomeReplayed},
		{"a stale save merged into a new version", SaveResult{MergedFrom: merged}, OutcomeMerged},
		{"a merge that changed nothing", SaveResult{Replayed: true, MergedFrom: merged}, OutcomeReplayed},
		{"a proposal", SaveResult{Proposal: &ProposalRef{ID: "p"}}, OutcomeProposed},
	}
	for _, c := range cases {
		if got := c.res.Outcome(); got != c.want {
			t.Errorf("%s: outcome = %q, want %q", c.name, got, c.want)
		}
		if !SaveOutcomes.Has(c.want) {
			t.Errorf("%s: %q is not a declared save outcome", c.name, c.want)
		}
	}
}

func TestTheSaveAnswerCarriesItsOutcomeInTheBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saved := Version{Spec: &spec.Dashboard{}}
	cases := []struct {
		res    SaveResult
		status int
		want   string
	}{
		{SaveResult{Version: saved}, http.StatusCreated, OutcomeCreated},
		{SaveResult{Version: saved, Replayed: true}, http.StatusOK, OutcomeReplayed},
		{SaveResult{Version: saved, MergedFrom: &MergedFrom{BaseVersion: 1, OntoVersion: 2}}, http.StatusCreated, OutcomeMerged},
		{SaveResult{Proposal: &ProposalRef{ID: "p", Revision: 1, Opened: true}}, http.StatusAccepted, OutcomeProposed},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		(&Service{}).answerSave(ctx, c.res)
		if rec.Code != c.status {
			t.Errorf("%s: status = %d, want %d", c.want, rec.Code, c.status)
		}
		var body struct {
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Outcome != c.want {
			t.Errorf("body outcome = %q, want %q", body.Outcome, c.want)
		}
		if replay := rec.Header().Get("Idempotent-Replay"); (replay == "true") != c.res.Replayed {
			t.Errorf("%s: Idempotent-Replay = %q", c.want, replay)
		}
	}
}
