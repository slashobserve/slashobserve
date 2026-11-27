package observations

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestTheRCAMachineIsConsistentAndNamesKnownEvents(t *testing.T) {
	if err := RCAMachine.Check(); err != nil {
		t.Fatal(err)
	}
	for _, mv := range RCAMachine.Moves {
		if mv.Event != "" && !activity.Known(mv.Event) {
			t.Errorf("%s: unknown event %q", mv.Action, mv.Event)
		}
	}
	for state, event := range RCAMachine.Arrive {
		if !activity.Known(event) {
			t.Errorf("arrive %s: unknown event %q", state, event)
		}
	}
	if got := RCAMachine.EventFor(RCAWithdraw, RCAWithdrawn); got != activity.TypeObservationRCAWithdrawn {
		t.Errorf("withdraw records %q", got)
	}
}

func TestAnRCARevisionStateComesFromItsLinksAndTheNewestRevision(t *testing.T) {
	cases := []struct {
		links, revision, newest int
		want                    RCAState
	}{
		{0, 3, 3, RCAPublished},
		{0, 1, 3, RCASuperseded},
		{1, 3, 3, RCAShared},
		{2, 1, 3, RCAShared},
	}
	for _, c := range cases {
		if got := rcaState(c.links, c.revision, c.newest); got != c.want {
			t.Errorf("links %d, revision %d of %d: %s, want %s", c.links, c.revision, c.newest, got, c.want)
		}
	}
}

func TestAnRCARevisionOffersWithdrawOnlyWhileItHasNoLiveLink(t *testing.T) {
	cases := []struct {
		name      string
		state     RCAState
		abilities folders.Abilities
		want      []RCAAction
	}{
		{"viewer", RCAPublished, viewing, []RCAAction{}},
		{"commenter", RCAPublished, commenting, []RCAAction{}},
		{"editor on the newest", RCAPublished, editing, []RCAAction{RCAWithdraw}},
		{"editor on an older one", RCASuperseded, editing, []RCAAction{RCAWithdraw}},
		{"editor on a shared one", RCAShared, editing, []RCAAction{}},
		{"manager on the newest", RCAPublished, managing, []RCAAction{RCAShare, RCAWithdraw}},
		{"manager on a shared one", RCAShared, managing, []RCAAction{RCAShare, RCAUnshare}},
	}
	for _, c := range cases {
		if got := allowedRCAActions(c.state, c.abilities); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWithdrawingASharedRevisionIsRefusedWithItsStateAndOpenMoves(t *testing.T) {
	if err := withdrawable(RCASuperseded, editing); err != nil {
		t.Errorf("an editor withdrawing an unshared revision: %v", err)
	}
	if err := withdrawable(RCAPublished, commenting); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a commenter withdrawing: %v, want forbidden", err)
	}
	if err := withdrawable(RCAShared, viewing); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer withdrawing a shared revision: %v, want forbidden", err)
	}
	err := withdrawable(RCAShared, managing)
	var refusal *RCARefusal
	if !errors.As(err, &refusal) || !errors.Is(err, ErrRCAShared) {
		t.Fatalf("a manager withdrawing a shared revision: %v", err)
	}
	if refusal.Code != CodeRCAShared || refusal.State != string(RCAShared) || !slices.Equal(refusal.Allowed, []string{"share", "unshare"}) {
		t.Errorf("refusal %+v", refusal)
	}
	if refusal.Error() != ErrRCAShared.Error() {
		t.Errorf("message %q changed", refusal.Error())
	}
}

func TestAPublishRefusalKeepsItsCodeAndCarriesTheObservationsStateAndMoves(t *testing.T) {
	allowed := []Action{ActionUpdate, ActionPublish}
	unchanged := &RCAUnchangedError{Revision: 3}
	cases := []struct {
		err     error
		cause   error
		code    string
		message string
	}{
		{fmt.Errorf("in a transaction: %w", unchanged), unchanged, CodeRCAUnchanged, unchanged.Error()},
		{ErrRCALimit, ErrRCALimit, CodeRCALimit, ErrRCALimit.Error()},
		{fmt.Errorf("freeze: %w", ErrObservationChanged), ErrObservationChanged, CodeObservationChanged, ErrObservationChanged.Error()},
	}
	for _, c := range cases {
		err := refusedPublish(c.err, StateResolved, allowed)
		var refusal *RCARefusal
		if !errors.As(err, &refusal) {
			t.Fatalf("%s: %v is not a refusal", c.code, err)
		}
		if refusal.Code != c.code || refusal.Error() != c.message || refusal.State != "resolved" || !slices.Equal(refusal.Allowed, []string{"update", "publish"}) {
			t.Errorf("%s: refusal %+v, message %q", c.code, refusal, refusal.Error())
		}
		if !errors.Is(err, c.cause) {
			t.Errorf("%s: the refusal no longer matches its cause", c.code)
		}
	}
	var stillUnchanged *RCAUnchangedError
	if err := refusedPublish(unchanged, StateOpen, allowed); !errors.As(err, &stillUnchanged) || stillUnchanged.Revision != 3 {
		t.Errorf("the unchanged revision number is lost: %v", err)
	}
	var rewritten *RCARefusal
	if err := refusedPublish(ErrPublishPersonal, StateOpen, allowed); errors.As(err, &rewritten) || !errors.Is(err, ErrPublishPersonal) {
		t.Errorf("another failure was rewritten: %v", err)
	}
	if err := refusedPublish(nil, StateOpen, allowed); err != nil {
		t.Errorf("no failure became %v", err)
	}
}

func TestAnRCARefusalAnswers409WithStateAndAllowedActions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &RCAHandler{}
	cases := []struct {
		err   error
		code  string
		state string
	}{
		{withdrawable(RCAShared, managing), "rca_shared", "shared"},
		{refusedPublish(ErrRCALimit, StateOpen, []Action{ActionPublish}), "rca_limit", "open"},
		{refusedPublish(&RCAUnchangedError{Revision: 2}, StateResolved, []Action{}), "rca_unchanged", "resolved"},
		{refusedPublish(ErrObservationChanged, StateOpen, []Action{ActionPublish}), "observation_changed", "open"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		handler.fail(ctx, "test", c.err)
		var body struct {
			Error          string    `json:"error"`
			Code           string    `json:"code"`
			State          string    `json:"state"`
			AllowedActions *[]string `json:"allowed_actions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v", c.code, err)
		}
		if rec.Code != http.StatusConflict || body.Code != c.code || body.State != c.state || body.AllowedActions == nil || body.Error == "" {
			t.Errorf("%s: status %d, body %s", c.code, rec.Code, rec.Body.String())
		}
	}
}
