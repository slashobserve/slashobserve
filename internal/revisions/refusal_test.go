package revisions

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestARefusedContinueOrRetryNamesItsReason(t *testing.T) {
	stopped := runFacts{state: StateFailed, source: SourceObserve, requester: true, mayStart: true, mayEdit: true, buildPanels: 2}
	with := func(change func(*runFacts)) runFacts {
		f := stopped
		change(&f)
		return f
	}
	tests := []struct {
		name   string
		facts  runFacts
		action Action
		want   string
	}{
		{"a stopped build can be continued", stopped, ActionContinue, ""},
		{"a running run is refused by its state alone", with(func(f *runFacts) { f.state = StateRunning }), ActionContinue, ""},
		{"no build", with(func(f *runFacts) { f.buildPanels = 0 }), ActionContinue, ReasonNoBuild},
		{"the build was kept", with(func(f *runFacts) { f.buildPanels, f.buildSaved = 0, true }), ActionContinue, ReasonKept},
		{"already continued, so its build moved", with(func(f *runFacts) { f.buildPanels, f.continued = 0, true }), ActionContinue, ReasonContinued},
		{"already retried", with(func(f *runFacts) { f.retried = true }), ActionContinue, ReasonRetried},
		{"an investigation", with(func(f *runFacts) { f.source = SourceInvestigate }), ActionContinue, ReasonInvestigation},
		{"retry after a continue", with(func(f *runFacts) { f.continued = true }), ActionRetry, ReasonContinued},
		{"retry after a retry", with(func(f *runFacts) { f.retried = true }), ActionRetry, ReasonRetried},
		{"retry of a queued run", with(func(f *runFacts) { f.state, f.retried = StateQueued, true }), ActionRetry, ""},
		{"cancel has no reason beyond the state", with(func(f *runFacts) { f.continued = true }), ActionCancel, ""},
	}
	for _, tc := range tests {
		if got := tc.facts.reasonAgainst(tc.action); got != tc.want {
			t.Errorf("%s: reason %q, want %q", tc.name, got, tc.want)
		}
		if tc.want != "" && !tc.facts.refuses(tc.action) {
			t.Errorf("%s: a reason for an action the facts allow", tc.name)
		}
	}
}

func TestRunRefusalsKeepTheirCodesAndAddStateActionsAndReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	finished := runFacts{state: StateSucceeded, source: SourceObserve, requester: true, mayStart: true, mayEdit: true}
	kept := runFacts{state: StateFailed, source: SourceObserve, requester: true, mayStart: true, mayEdit: true, buildSaved: true}
	retried := runFacts{state: StateFailed, source: SourceObserve, requester: true, mayStart: true, mayEdit: true, buildPanels: 1, retried: true}
	tests := []struct {
		name  string
		err   error
		cause error
		body  string
	}{
		{"cancel of a finished run", refusalOf(finished, ActionCancel, ""), ErrRequestFinished,
			`{"error":"the request already finished","code":"request_finished","state":"succeeded","allowed_actions":["retry","pin","archive"]}`},
		{"continue after the build was kept", refusalOf(kept, ActionContinue, ""), ErrNotContinuable,
			`{"error":"the request has no stopped build to continue","code":"not_continuable","state":"failed","allowed_actions":["retry","pin","archive"],"reason":"kept"}`},
		{"continue that lost to another continue", refusalOf(retried, ActionContinue, ReasonLostRace), ErrNotContinuable,
			`{"error":"the request has no stopped build to continue","code":"not_continuable","state":"failed","allowed_actions":["keep_build","pin","archive"],"reason":"lost_race"}`},
		{"retry of a retried run", refusalOf(retried, ActionRetry, ""), nil,
			`{"error":"retry is not allowed while the run is failed","code":"invalid_transition","state":"failed","allowed_actions":["keep_build","pin","archive"],"reason":"retried"}`},
	}
	for _, tc := range tests {
		if tc.cause != nil && !errors.Is(tc.err, tc.cause) {
			t.Errorf("%s: the refusal is no longer %v", tc.name, tc.cause)
		}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		(&ObserveHandler{}).failBuild(c, "act on agent request", tc.err)
		if rec.Code != http.StatusConflict || rec.Body.String() != tc.body {
			t.Errorf("%s: %d %s, want 409 %s", tc.name, rec.Code, rec.Body, tc.body)
		}
	}
	if reply, ok := ReplyFor(refusalOf(kept, ActionContinue, "")); !ok || reply.Code != "not_continuable" || reply.Message != ErrNotContinuable.Error() {
		t.Errorf("a refused continue maps to %+v outside HTTP, want not_continuable with its message", reply)
	}
}

func TestCanCancelIsWhatAllowedActionsSay(t *testing.T) {
	desk := NewRunDesk(&Service{logger: slog.New(slog.DiscardHandler)}, ObserveOptions{})
	for _, state := range RunMachine.States {
		for _, source := range []Source{SourceObserve, SourceSuggest} {
			allowed := allowedActions(runFacts{state: state, source: source, requester: true})
			out := desk.view(t.Context(), dbsqlc.AgentRequest{State: string(state)}, auth.Principal{}, buildState{}, allowed, nil)
			if out.CanCancel != slices.Contains(out.AllowedActions, string(ActionCancel)) {
				t.Errorf("a %s %s run: can_cancel %t with allowed_actions %v", state, source, out.CanCancel, out.AllowedActions)
			}
		}
	}
}
