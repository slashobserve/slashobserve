package threads

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestAnAuthorDeletesTheirOwnReplyAndNobodyElsesButAManager(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	tg := target(KindDashboard, dash)
	thread, root := r.open(t, testUser, OpenInput{Target: tg, Body: "question"})
	_, mine := r.reply(t, commenter, thread.ID, "mine")
	_, other := r.reply(t, commenter, thread.ID, "mistake")

	if err := r.service.Delete(t.Context(), testUser, mustParse(t, mine.ID)); !errors.Is(err, ErrCannotDelete) {
		t.Fatalf("an editor deleting a commenter's reply: err = %v, want ErrCannotDelete", err)
	}
	if err := r.service.Delete(t.Context(), viewer, mustParse(t, mine.ID)); !errors.Is(err, ErrTargetNotFound) && !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("a viewer: err = %v, want a refusal", err)
	}
	if err := r.service.Delete(t.Context(), commenter, mustParse(t, other.ID)); err != nil {
		t.Fatalf("the author deleting their reply: %v", err)
	}
	if err := r.service.Delete(t.Context(), admin, mustParse(t, mine.ID)); err != nil {
		t.Fatalf("a manager deleting a reply: %v", err)
	}
	if err := r.service.Delete(t.Context(), admin, mustParse(t, other.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting twice: err = %v, want ErrNotFound", err)
	}

	page := r.list(t, testUser, tg)
	if len(page.Messages) != 1 || page.Messages[0].ID != root.ID || !page.Messages[0].CanDelete {
		t.Fatalf("messages left = %+v, want only the root, deletable by its author", page.Messages)
	}
	types := activityTypes(r.activity(t))
	deleted := 0
	for _, typ := range types {
		if typ == activity.TypeThreadDeleted {
			deleted++
		}
	}
	if deleted != 2 {
		t.Fatalf("%d thread.deleted events in %v, want 2", deleted, types)
	}
}

func TestDeletingTheFirstMessageDeletesTheThreadOnlyWhenNobodyElseWroteInIt(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	tg := target(KindDashboard, dash)
	alone, aloneRoot := r.open(t, testUser, OpenInput{Target: tg, Body: "alone"})
	r.reply(t, testUser, alone.ID, "still me")
	busy, busyRoot := r.open(t, testUser, OpenInput{Target: tg, Body: "busy"})
	r.reply(t, commenter, busy.ID, "someone else")

	want := map[string]bool{"alone": true, "still me": true, "busy": false, "someone else": false}
	for _, m := range r.list(t, testUser, tg).Messages {
		if m.CanDelete != want[m.Body] {
			t.Fatalf("can_delete of %q = %v, want %v", m.Body, m.CanDelete, want[m.Body])
		}
	}
	if err := r.service.Delete(t.Context(), testUser, mustParse(t, busyRoot.ID)); !errors.Is(err, ErrCannotDelete) {
		t.Fatalf("a thread others wrote in: err = %v, want ErrCannotDelete", err)
	}
	if err := r.service.Delete(t.Context(), testUser, mustParse(t, aloneRoot.ID)); err != nil {
		t.Fatalf("a thread only I wrote in: %v", err)
	}
	if err := r.service.Delete(t.Context(), admin, mustParse(t, busyRoot.ID)); err != nil {
		t.Fatalf("a manager deleting a busy thread: %v", err)
	}
	if page := r.list(t, testUser, tg); len(page.Threads) != 0 || len(page.Messages) != 0 {
		t.Fatalf("left %d threads and %d messages", len(page.Threads), len(page.Messages))
	}
	if got := r.count(t, `SELECT count(*) FROM thread_messages`); got != 0 {
		t.Fatalf("%d messages remain", got)
	}
}

func TestOnlyAManagerDeletesAnAgentReply(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t, "rps", "errors")
	thread, run := r.mentionRun(t, target(KindDashboard, dash))
	if err := r.runEnded(t, succeededWithVersion(run, 3)); err != nil {
		t.Fatal(err)
	}
	reply := r.runReplies(t, thread)[0]

	if err := r.service.Delete(t.Context(), testUser, mustParse(t, reply.ID)); !errors.Is(err, ErrCannotDelete) {
		t.Fatalf("the requester deleting the agent's reply: err = %v, want ErrCannotDelete", err)
	}
	for _, m := range r.list(t, testUser, target(KindDashboard, dash)).Messages {
		if m.ID == reply.ID && m.CanDelete {
			t.Fatalf("the agent reply is listed as deletable by its requester: %+v", m)
		}
	}
	if err := r.service.Delete(t.Context(), admin, mustParse(t, reply.ID)); err != nil {
		t.Fatalf("a manager: %v", err)
	}
}

func TestDeletingAMessageThroughTheRoute(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	thread, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "question"})
	_, reply := r.reply(t, commenter, thread.ID, "answer")

	r.call(t, testUser, http.MethodDelete, "/messages/"+reply.ID, nil).expect(t, http.StatusForbidden, CodeNotAuthor)
	listed := r.call(t, commenter, http.MethodGet, "/threads?target=dashboard:"+dash.String(), nil)
	messages, _ := listed.at(t, "messages").([]any)
	if len(messages) != 2 || messages[1].(map[string]any)["can_delete"] != true || messages[0].(map[string]any)["can_delete"] != false {
		t.Fatalf("messages = %v", messages)
	}
	if got := r.callStatus(t, commenter, http.MethodDelete, "/messages/"+reply.ID); got != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", got)
	}
	r.call(t, commenter, http.MethodDelete, "/messages/"+reply.ID, nil).expect(t, http.StatusNotFound, "not_found")
}

func (r *rig) callStatus(t *testing.T, who auth.Principal, method, path string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), who))
	})
	NewThreads(r.service, r.asks).Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, "/api/v1"+path, nil))
	return rec.Code
}
