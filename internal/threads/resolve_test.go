package threads

import (
	"errors"
	"net/http"
	"slices"
	"testing"
)

func TestACommenterResolvesOnlyTheThreadsTheyStarted(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	tg := target(KindDashboard, dash)
	others, othersRoot := r.open(t, testUser, OpenInput{Target: tg, Body: "an editor's question"})
	mine, mineRoot := r.open(t, commenter, OpenInput{Target: tg, Body: "my question"})

	if _, _, err := r.service.SetStatus(reading(t.Context()), commenter, mustParse(t, othersRoot.ID), StatusResolved, nil); !errors.Is(err, ErrNotStarter) {
		t.Fatalf("a commenter resolving an editor's thread: err = %v, want ErrNotStarter", err)
	}
	if got := r.list(t, commenter, tg); threadActions(got, others.ID) == nil || slices.Contains(threadActions(got, others.ID), "resolve") || !slices.Contains(threadActions(got, mine.ID), "resolve") {
		t.Fatalf("commenter's allowed actions: others %v, mine %v", threadActions(got, others.ID), threadActions(got, mine.ID))
	}
	if _, changed, err := r.service.SetStatus(reading(t.Context()), commenter, mustParse(t, mineRoot.ID), StatusResolved, nil); err != nil || !changed {
		t.Fatalf("a commenter resolving their own thread: changed %v, err %v", changed, err)
	}
	if _, changed, err := r.service.SetStatus(reading(t.Context()), testUser, mustParse(t, mineRoot.ID), StatusOpen, nil); err != nil || !changed {
		t.Fatalf("reopen: %v", err)
	}
	if _, changed, err := r.service.SetStatus(reading(t.Context()), testUser, mustParse(t, mineRoot.ID), StatusResolved, nil); err != nil || !changed {
		t.Fatalf("an editor resolving a commenter's thread: changed %v, err %v", changed, err)
	}
}

func TestResolvingSomeoneElsesThreadAsACommenterAnswersNotStarterOverHTTP(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	opened := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": "dashboard:" + dash.String(), "body": "mine"})
	threadID := opened.at(t, "thread", "id").(string)

	r.call(t, commenter, http.MethodPost, "/threads/"+threadID+"/resolve", nil).expect(t, http.StatusForbidden, CodeNotStarter)
	r.call(t, testUser, http.MethodPost, "/threads/"+threadID+"/resolve", nil).expect(t, http.StatusOK, "")
}

func threadActions(page Page, threadID string) []string {
	for _, th := range page.Threads {
		if th.ID == threadID {
			return th.AllowedActions
		}
	}
	return nil
}
