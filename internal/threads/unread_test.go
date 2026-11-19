package threads

import (
	"net/http"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func (r *rig) seedReaders(t *testing.T, readers ...auth.Principal) {
	t.Helper()
	for _, p := range readers {
		r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'issuer', $2, $2, $3) ON CONFLICT (id) DO NOTHING`, p.ID, p.ID+"@example.com", p.Name)
	}
}

func unreadOf(page Page, threadID string) bool {
	for _, th := range page.Threads {
		if th.ID == threadID {
			return th.Unread
		}
	}
	return false
}

func TestAThreadIsUnreadUntilTheReaderOpensItOrAnswersInIt(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	r.seedReaders(t, testUser, commenter, viewer)
	tg := target(KindDashboard, dash)
	thread, _ := r.open(t, testUser, OpenInput{Target: tg, Body: "question"})

	if unreadOf(r.list(t, testUser, tg), thread.ID) {
		t.Error("the author's own thread is unread for them")
	}
	if !unreadOf(r.list(t, commenter, tg), thread.ID) {
		t.Error("someone else's new thread is not unread")
	}
	if err := r.service.MarkRead(t.Context(), commenter, mustParse(t, thread.ID)); err != nil {
		t.Fatal(err)
	}
	if unreadOf(r.list(t, commenter, tg), thread.ID) {
		t.Error("the thread is still unread after being marked read")
	}
	if !unreadOf(r.list(t, viewer, tg), thread.ID) {
		t.Error("one reader's mark cleared another reader's dot")
	}

	r.reply(t, testUser, thread.ID, "more detail")
	if !unreadOf(r.list(t, commenter, tg), thread.ID) {
		t.Error("a reply from someone else did not bring the dot back")
	}
	got, _ := r.reply(t, commenter, thread.ID, "thanks")
	if got.Unread || unreadOf(r.list(t, commenter, tg), thread.ID) {
		t.Errorf("answering in a thread leaves it unread for the answerer: %+v", got)
	}
	if !unreadOf(r.list(t, testUser, tg), thread.ID) {
		t.Error("the reply did not mark the thread unread for the person who wrote before")
	}
}

func TestMessagesFromBeforeAPersonJoinedAreNotUnreadForThem(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	r.seedReaders(t, testUser, commenter)
	tg := target(KindDashboard, dash)
	thread, _ := r.open(t, testUser, OpenInput{Target: tg, Body: "old news"})
	r.db.Exec(t, `UPDATE users SET threads_seen_since = now() + interval '1 hour' WHERE id = $1::uuid`, commenterID)

	if unreadOf(r.list(t, commenter, tg), thread.ID) {
		t.Error("a message from before the reader's baseline is unread")
	}
	r.db.Exec(t, `UPDATE users SET threads_seen_since = $2 WHERE id = $1::uuid`, commenterID, time.Now().Add(-time.Hour))
	if !unreadOf(r.list(t, commenter, tg), thread.ID) {
		t.Error("a message after the baseline is not unread")
	}
}

func TestAnAgentReplyIsUnreadForTheRequesterWhoAskedForIt(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t, "rps", "errors")
	r.seedReaders(t, testUser)
	thread, run := r.mentionRun(t, target(KindDashboard, dash))
	if err := r.service.MarkRead(t.Context(), testUser, mustParse(t, thread)); err != nil {
		t.Fatal(err)
	}
	if err := r.runEnded(t, succeededWithVersion(run, 3)); err != nil {
		t.Fatal(err)
	}
	if !unreadOf(r.list(t, testUser, target(KindDashboard, dash)), thread) {
		t.Error("the agent's reply is not unread for the person who asked")
	}
}

func TestMarkingAThreadReadThroughTheRouteIsForPeopleWhoCanSeeIt(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	r.seedReaders(t, testUser, commenter, viewer)
	opened := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": "dashboard:" + dash.String(), "body": "mine"})
	threadID := opened.at(t, "thread", "id").(string)
	path := "/threads?target=dashboard:" + dash.String()

	listed := r.call(t, viewer, http.MethodGet, path, nil)
	if threads, _ := listed.at(t, "threads").([]any); len(threads) != 1 || threads[0].(map[string]any)["unread"] != true {
		t.Fatalf("threads = %v", listed.body)
	}
	if got := r.callStatus(t, viewer, http.MethodPost, "/threads/"+threadID+"/read"); got != http.StatusNoContent {
		t.Fatalf("a viewer marking a thread read: status %d", got)
	}
	listed = r.call(t, viewer, http.MethodGet, path, nil)
	if threads, _ := listed.at(t, "threads").([]any); threads[0].(map[string]any)["unread"] != false {
		t.Errorf("threads after reading = %v", listed.body)
	}
	if got := r.callStatus(t, viewer, http.MethodPost, "/threads/"+opened.at(t, "message", "id").(string)+"/read"); got != http.StatusNoContent {
		t.Errorf("marking read through the first message's id: status %d", got)
	}
	r.call(t, viewer, http.MethodPost, "/threads/00000000-0000-4000-8000-000000000000/read", nil).expect(t, http.StatusNotFound, "not_found")
	account := auth.Principal{Kind: auth.KindServiceAccount, ID: "66666666-6666-4666-8666-666666666666", Name: "ci", Role: auth.RoleViewer}
	r.call(t, account, http.MethodPost, "/threads/"+threadID+"/read", nil).expect(t, http.StatusForbidden, "forbidden")
	if unreadOf(r.list(t, account, target(KindDashboard, dash)), threadID) {
		t.Error("a service account has an unread dot")
	}
}
