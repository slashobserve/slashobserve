package threads

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestASharedAnswerThatAManagerDeletedCarriesTheAnswerWhenSharedAgain(t *testing.T) {
	r := newShareRig(t)
	_, run, first := r.sharedDashboardAnswer(t)
	if err := r.service.Delete(reading(t.Context()), admin, mustParse(t, first.Thread.ID)); err != nil {
		t.Fatalf("a manager deletes the shared thread: %v", err)
	}

	again, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil || again.Replayed {
		t.Fatalf("share after the thread was deleted = %+v, %v", again, err)
	}
	if replies := r.runReplies(t, again.Thread.ID); len(replies) != 1 || replies[0].RunID != run.String() {
		t.Fatalf("replies in the new thread = %+v, want the answer once", replies)
	}
}

func TestAnAnswerWhoseShareMessageAManagerDeletedCarriesTheAnswerWhenSharedAgain(t *testing.T) {
	r := newShareRig(t)
	dash := r.createDashboard(t, "rps", "errors")
	id := dash.String()
	earlier, _ := r.open(t, testUser, OpenInput{Target: Target{Kind: KindDashboard, ID: dash}, Body: "errors are up since noon"})
	run := r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "errors")
	first, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run, ThreadID: mustParse(t, earlier.ID)})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.service.Delete(reading(t.Context()), admin, mustParse(t, first.Message.ID)); err != nil {
		t.Fatalf("a manager deletes the share message: %v", err)
	}

	again, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil || again.Replayed {
		t.Fatalf("share after the share message was deleted = %+v, %v", again, err)
	}
	if replies := r.runReplies(t, again.Thread.ID); len(replies) != 1 || replies[0].RunID != run.String() {
		t.Fatalf("replies in the new thread = %+v, want the answer once", replies)
	}
}
