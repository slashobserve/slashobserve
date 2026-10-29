package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const notifyPayloadLimit = 8000

func TestCommentEventPayloadStaysUnderNotifyLimit(t *testing.T) {
	id, err := db.ParseUUID("11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	row := dbsqlc.DashboardComment{ID: id, DashboardID: id, PanelID: pgtype.Text{String: strings.Repeat("\x01", maxPanelIDRunes), Valid: true}, ParentID: id, Status: CommentOpen}
	actor := Author{Kind: "service_account", ID: id.String(), Name: strings.Repeat("\x01", 100000)}
	for _, eventType := range []string{events.TypeCommentCreated, events.TypeCommentUpdated} {
		payload, err := commentEventPayload(eventType, row, CommentResolved, actor)
		if err != nil {
			t.Fatal(err)
		}
		if len(payload) >= notifyPayloadLimit/2 {
			t.Errorf("%s payload is %d bytes, want well under %d", eventType, len(payload), notifyPayloadLimit)
		}
	}
}

func TestCheckCommentBody(t *testing.T) {
	for name, in := range map[string]string{
		"padded":               "  hello \n",
		"4000 multibyte runes": strings.Repeat("\u00e9", maxCommentRunes),
		"tab newline CR":       "a\tb\r\nc",
		"ZWJ sequence":         "\U0001F469\u200d\U0001F4BB",
		"bidi marks":           "a\u200eb\u200fc\u061cd",
	} {
		got, err := checkCommentBody(in)
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if got != strings.TrimSpace(in) {
			t.Errorf("%s: got %q", name, got)
		}
	}
	invalid := map[string]string{
		"empty":           "",
		"whitespace only": " \t\n ",
		"NUL":             "a\x00b",
		"C0 bell":         "a\x07b",
		"escape":          "a\x1bb",
		"DEL":             "a\x7fb",
		"C1 CSI":          "a\u009bb",
		"C1 NEL inside":   "a\u0085b",
		"embedding start": "a\u202ab",
		"override":        "a\u202eb",
		"isolate start":   "a\u2066b",
		"isolate end":     "a\u2069b",
		"too long":        strings.Repeat("a", maxCommentRunes+1),
		"invalid UTF-8":   "a\xffb",
	}
	for name, in := range invalid {
		var target *InvalidInputError
		if _, err := checkCommentBody(in); !errors.As(err, &target) {
			t.Errorf("%s: got %v, want InvalidInputError", name, err)
		}
	}
}

func TestPanelCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := newPanelCache()
	key := func(n int) panelKey { return panelKey{version: n} }
	for n := 1; n <= panelCacheSize; n++ {
		cache.put(key(n), map[string]struct{}{})
	}
	if _, ok := cache.get(key(1)); !ok {
		t.Fatal("first entry evicted before the cache was full")
	}
	cache.put(key(panelCacheSize+1), map[string]struct{}{})
	if _, ok := cache.get(key(2)); ok {
		t.Error("least recently used entry survived")
	}
	if _, ok := cache.get(key(1)); !ok {
		t.Error("recently used entry was evicted")
	}
}

func TestCommentEventPayloadOmitsAbsentPanelAndParent(t *testing.T) {
	id := mustParse(t, "11111111-2222-3333-4444-555555555555")
	parent := mustParse(t, "66666666-7777-4888-8999-aaaaaaaaaaaa")
	tests := []struct {
		name       string
		row        dbsqlc.DashboardComment
		wantPanel  bool
		wantParent bool
	}{
		{"panel root", dbsqlc.DashboardComment{ID: id, DashboardID: id, PanelID: pgtype.Text{String: "rps", Valid: true}}, true, false},
		{"whole-dashboard root", dbsqlc.DashboardComment{ID: id, DashboardID: id}, false, false},
		{"reply on a panel", dbsqlc.DashboardComment{ID: id, DashboardID: id, PanelID: pgtype.Text{String: "rps", Valid: true}, ParentID: parent}, true, true},
		{"reply on the whole dashboard", dbsqlc.DashboardComment{ID: id, DashboardID: id, ParentID: parent}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := commentEventPayload(events.TypeCommentCreated, tt.row, CommentOpen, Author{Kind: "user", ID: id.String(), Name: "Ada"})
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatal(err)
			}
			_, hasPanel := fields["panel_id"]
			if hasPanel != tt.wantPanel {
				t.Errorf("panel_id present = %v in %s", hasPanel, payload)
			}
			got, hasParent := fields["parent_id"]
			if hasParent != tt.wantParent || (hasParent && got != parent.String()) {
				t.Errorf("parent_id = %v (present %v) in %s", got, hasParent, payload)
			}
		})
	}
}

func (r *rig) addVersion(t *testing.T, dash pgtype.UUID, parent int) {
	t.Helper()
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, parent, panelsSpec(fmt.Sprintf("t%d", parent), "rps", "errors"), "next"); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) createComment(t *testing.T, dash pgtype.UUID, in NewComment) Comment {
	t.Helper()
	c, err := r.comments.Create(t.Context(), testUser, dash, in)
	if err != nil {
		t.Fatalf("create %+v: %v", in, err)
	}
	return c
}

func TestReplyToAReplyJoinsTheRootsThread(t *testing.T) {
	r := newRig(t, false)
	dash := r.createDashboard(t, "rps", "errors")
	r.addVersion(t, dash, 1)
	root := r.createComment(t, dash, NewComment{Version: 1, PanelID: new("errors"), Body: "root"})
	first := r.createComment(t, dash, NewComment{ParentID: mustParse(t, root.ID), Body: "first"})
	second := r.createComment(t, dash, NewComment{ParentID: mustParse(t, first.ID), Body: "second"})

	for name, reply := range map[string]Comment{"reply to the root": first, "reply to a reply": second} {
		if reply.ParentID != root.ID || reply.Version != 1 || reply.PanelID != "errors" || reply.Status != CommentOpen || !reply.PanelPresent {
			t.Errorf("%s = %+v, want parent %s on version 1, panel errors, open", name, reply, root.ID)
		}
	}
}

func TestReplyNeedsAParentOnTheSameDashboard(t *testing.T) {
	r := newRig(t, false)
	dash := r.createDashboard(t)
	other := r.createDashboard(t)
	foreign := r.createComment(t, other, NewComment{Version: 1, PanelID: new("rps"), Body: "elsewhere"})

	for name, parent := range map[string]string{
		"a comment on another dashboard": foreign.ID,
		"an unknown id":                  "00000000-0000-4000-8000-000000000000",
	} {
		_, err := r.comments.Create(t.Context(), testUser, dash, NewComment{ParentID: mustParse(t, parent), Body: "reply"})
		if !errors.Is(err, ErrCommentNotFound) {
			t.Errorf("reply to %s: err = %v, want ErrCommentNotFound", name, err)
		}
	}
	listed, _, err := r.comments.List(t.Context(), dash, CommentAll, "", 10, "")
	if err != nil || len(listed) != 0 {
		t.Fatalf("listed = %+v, err %v", listed, err)
	}
}

func TestReplyCannotRequestARevision(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	root := r.createComment(t, dash, NewComment{Version: 1, PanelID: new("rps"), Body: "root"})

	_, err := r.comments.Create(t.Context(), testUser, dash, NewComment{ParentID: mustParse(t, root.ID), Body: "reply", RequestRevision: true})
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidInputError", err)
	}
	if items, _, err := r.revisions.List(t.Context(), dash, 10, ""); err != nil || len(items) != 0 {
		t.Fatalf("queued = %v, err %v", items, err)
	}
	if listed, _, err := r.comments.List(t.Context(), dash, CommentAll, "", 10, ""); err != nil || len(listed) != 1 {
		t.Fatalf("listed = %+v, err %v", listed, err)
	}
}

func TestWholeDashboardCommentQueuesAWholeDashboardRevision(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)

	comment := r.createComment(t, dash, NewComment{Version: 1, Body: "tidy the layout", RequestRevision: true})
	if comment.PanelID != "" || !comment.PanelPresent || comment.Revision == nil || comment.Revision.Status != "queued" {
		t.Fatalf("comment = %+v", comment)
	}
	items, _, err := r.revisions.List(t.Context(), dash, 10, "")
	if err != nil || len(items) != 1 {
		t.Fatalf("queued = %v, err %v", items, err)
	}
	if items[0].ID != comment.Revision.ID || items[0].PanelID != "" || items[0].CommentID != comment.ID || items[0].Source != revisions.SourceComment {
		t.Fatalf("revision = %+v", items[0])
	}
	listed, _, err := r.comments.List(t.Context(), dash, CommentAll, "", 10, "")
	if err != nil || len(listed) != 1 || !listed[0].PanelPresent || listed[0].PanelID != "" {
		t.Fatalf("listed = %+v, err %v", listed, err)
	}
}

func TestCommentRejectsAnEmptyPanelIDAndAMissingVersion(t *testing.T) {
	r := newRig(t, false)
	dash := r.createDashboard(t)

	var invalid *InvalidInputError
	if _, err := r.comments.Create(t.Context(), testUser, dash, NewComment{Version: 1, PanelID: new(""), Body: "x"}); !errors.As(err, &invalid) {
		t.Errorf("empty panel id: err = %v, want InvalidInputError", err)
	}
	if _, err := r.comments.Create(t.Context(), testUser, dash, NewComment{Version: 1, PanelID: new("gone"), Body: "x"}); !errors.Is(err, ErrUnknownPanel) {
		t.Errorf("unknown panel: err = %v, want ErrUnknownPanel", err)
	}
	for name, panel := range map[string]*string{"whole dashboard": nil, "panel": new("rps")} {
		if _, err := r.comments.Create(t.Context(), testUser, dash, NewComment{Version: 2, PanelID: panel, Body: "x"}); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s on a missing version: err = %v, want ErrNotFound", name, err)
		}
	}
}

func TestSetStatusResolvesTheWholeThread(t *testing.T) {
	r := newRig(t, false)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	root := r.createComment(t, dash, NewComment{Version: 1, PanelID: new("rps"), Body: "root"})
	reply := r.createComment(t, dash, NewComment{ParentID: mustParse(t, root.ID), Body: "reply"})
	nested := r.createComment(t, dash, NewComment{ParentID: mustParse(t, reply.ID), Body: "nested"})
	setStatus := func(id string, status string, version *int) (Comment, bool, error) {
		return r.comments.SetStatus(t.Context(), testUser, dash, mustParse(t, id), status, version)
	}

	resolved, changed, err := setStatus(reply.ID, CommentResolved, new(2))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("the first resolve should report changed")
	}
	if resolved.ID != root.ID || resolved.ParentID != "" || resolved.Status != CommentResolved || resolved.ResolvedByVersion == nil || *resolved.ResolvedByVersion != 2 ||
		resolved.ResolvedAt == nil || resolved.ResolvedBy == nil {
		t.Fatalf("resolved = %+v", resolved)
	}

	again, changed, err := setStatus(nested.ID, CommentResolved, new(1))
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("a repeat resolve should not report changed")
	}
	if again.ID != root.ID || *again.ResolvedByVersion != 2 || !again.UpdatedAt.Equal(resolved.UpdatedAt) || !again.ResolvedAt.Equal(*resolved.ResolvedAt) {
		t.Fatalf("a repeat call changed the thread: %+v, was %+v", again, resolved)
	}

	listed, _, err := r.comments.List(t.Context(), dash, CommentResolved, "", 10, "")
	if err != nil || len(listed) != 3 {
		t.Fatalf("resolved list = %+v, err %v", listed, err)
	}
	for _, c := range listed {
		if c.Status != CommentResolved {
			t.Errorf("%s status = %s, want resolved", c.ID, c.Status)
		}
		if c.ID == root.ID {
			if c.ResolvedAt == nil || c.ResolvedBy == nil || c.ResolvedByVersion == nil || *c.ResolvedByVersion != 2 {
				t.Errorf("root = %+v", c)
			}
			continue
		}
		if c.ParentID != root.ID || c.ResolvedAt != nil || c.ResolvedBy != nil || c.ResolvedByVersion != nil {
			t.Errorf("reply = %+v, want parent %s and no resolved fields", c, root.ID)
		}
	}
	if open, _, err := r.comments.List(t.Context(), dash, CommentOpen, "", 10, ""); err != nil || len(open) != 0 {
		t.Fatalf("open list = %+v, err %v", open, err)
	}

	reopened, changed, err := setStatus(reply.ID, CommentOpen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("reopening a resolved thread should report changed")
	}
	if reopened.ID != root.ID || reopened.Status != CommentOpen || reopened.ResolvedByVersion != nil || reopened.ResolvedAt != nil || reopened.ResolvedBy != nil {
		t.Fatalf("reopened = %+v", reopened)
	}
	if open, _, err := r.comments.List(t.Context(), dash, CommentOpen, "", 10, ""); err != nil || len(open) != 3 {
		t.Fatalf("open list = %+v, err %v", open, err)
	}

	if _, changed, err := setStatus(root.ID, CommentOpen, nil); err != nil || changed {
		t.Fatalf("reopening an open thread: changed = %v, err = %v, want unchanged", changed, err)
	}
}

func TestSetStatusChecksTheResolvedByVersion(t *testing.T) {
	r := newRig(t, false)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	r.addVersion(t, dash, 2)
	onLatest := r.createComment(t, dash, NewComment{Version: 3, PanelID: new("rps"), Body: "latest"})
	onFirst := r.createComment(t, dash, NewComment{Version: 1, Body: "first"})
	setStatus := func(id string, status string, version *int) (Comment, error) {
		comment, _, err := r.comments.SetStatus(t.Context(), testUser, dash, mustParse(t, id), status, version)
		return comment, err
	}

	var invalid *InvalidInputError
	tests := []struct {
		name    string
		id      string
		status  string
		version *int
		want    func(error) bool
	}{
		{"above the latest version", onFirst.ID, CommentResolved, new(4), func(err error) bool { return errors.Is(err, ErrUnknownVersion) }},
		{"zero", onFirst.ID, CommentResolved, new(0), func(err error) bool { return errors.As(err, &invalid) }},
		{"negative", onFirst.ID, CommentResolved, new(-1), func(err error) bool { return errors.As(err, &invalid) }},
		{"with status open", onFirst.ID, CommentOpen, new(1), func(err error) bool { return errors.As(err, &invalid) }},
		{"below the thread's version", onLatest.ID, CommentResolved, new(2), func(err error) bool { return errors.As(err, &invalid) }},
		{"a missing comment with a version above the latest", "00000000-0000-4000-8000-000000000000", CommentResolved, new(99), func(err error) bool { return errors.Is(err, ErrCommentNotFound) }},
		{"a missing comment", "00000000-0000-4000-8000-000000000000", CommentResolved, nil, func(err error) bool { return errors.Is(err, ErrCommentNotFound) }},
	}
	for _, tt := range tests {
		if _, err := setStatus(tt.id, tt.status, tt.version); !tt.want(err) {
			t.Errorf("%s: err = %v", tt.name, err)
		}
	}
	listed, _, err := r.comments.List(t.Context(), dash, CommentOpen, "", 10, "")
	if err != nil || len(listed) != 2 {
		t.Fatalf("a rejected call changed a thread: %+v, err %v", listed, err)
	}

	for name, version := range map[string]*int{"the thread's own version": new(3), "no version": nil} {
		got, err := setStatus(onLatest.ID, CommentResolved, version)
		if err != nil || got.Status != CommentResolved {
			t.Errorf("%s: %+v, err %v", name, got, err)
		}
		if _, err := setStatus(onLatest.ID, CommentOpen, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReplyShowsTheStatusOfItsThreadAtInsertTime(t *testing.T) {
	r := newRig(t, false)
	dash := r.createDashboard(t)
	root := r.createComment(t, dash, NewComment{Version: 1, PanelID: new("rps"), Body: "root"})
	sqlDB := r.db.SQLDB(t)

	tx, err := sqlDB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(t.Context(), `UPDATE dashboard_comments SET status = 'resolved', resolved_at = now(), resolved_by_kind = 'user', resolved_by_id = gen_random_uuid(), resolved_by_name = 'x' WHERE id = $1::uuid`, root.ID); err != nil {
		t.Fatal(err)
	}

	type result struct {
		reply Comment
		err   error
	}
	done := make(chan result, 1)
	go func() {
		reply, err := r.comments.Create(t.Context(), testUser, dash, NewComment{ParentID: mustParse(t, root.ID), Body: "reply"})
		done <- result{reply, err}
	}()

	waiting := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && !waiting; time.Sleep(10 * time.Millisecond) {
		var n int
		if err := sqlDB.QueryRowContext(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		waiting = n > 0
	}
	if !waiting {
		_ = tx.Rollback()
		<-done
		t.Fatal("the reply did not wait for the thread's status change to commit")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.reply.Status != CommentResolved {
		t.Fatalf("reply status = %q, want resolved", got.reply.Status)
	}
}

func TestRequestingARevisionNeedsEditAccess(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	commenter := auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Bo", Role: auth.RoleCommenter}

	_, err := r.comments.Create(t.Context(), commenter, dash, NewComment{Version: 1, Body: "fix it", RequestRevision: true})
	if !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("a commenter requesting a revision gave %v, want folders.ErrForbidden", err)
	}
	if listed, _, err := r.comments.List(t.Context(), dash, CommentAll, "", 10, ""); err != nil || len(listed) != 0 {
		t.Fatalf("listed = %+v, err %v", listed, err)
	}
	if items, _, err := r.revisions.List(t.Context(), dash, 10, ""); err != nil || len(items) != 0 {
		t.Fatalf("queued = %v, err %v", items, err)
	}

	missing := mustParse(t, "55555555-5555-4555-8555-555555555555")
	_, err = r.comments.Create(t.Context(), testUser, missing, NewComment{Version: 1, Body: "fix it", RequestRevision: true})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing dashboard gave %v, want ErrNotFound", err)
	}
}

func TestCommentFailuresAnswerLikeTheRestOfTheDashboards(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		code    string
		message string
	}{
		{"role does not allow a revision", folders.ErrForbidden, http.StatusForbidden, "forbidden_role", "your role does not allow this"},
		{"no access to the dashboard", folders.ErrNoAccess, http.StatusForbidden, "forbidden_role", "you do not have access to this dashboard"},
		{"dashboard is gone", ErrNotFound, http.StatusNotFound, "not_found", ErrNotFound.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/x/comments", nil)
			(&Comments{dashboards: &Service{}}).fail(c, "create comment", tt.err)
			var body struct{ Error, Code string }
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tt.status || body.Code != tt.code || body.Error != tt.message {
				t.Errorf("answered %d %+v, want %d %s %q", rec.Code, body, tt.status, tt.code, tt.message)
			}
		})
	}
}

func TestConcurrentRevisionCommentsNeverExhaustThePool(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := range 4 * max(runtime.NumCPU(), 16) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.comments.Create(ctx, testUser, dash, NewComment{Version: 1, PanelID: new("rps"), Body: fmt.Sprintf("note %d", i), RequestRevision: true})
			if err != nil && !errors.Is(err, revisions.ErrTooManyRevisions) {
				t.Errorf("create %d: %v", i, err)
			}
		}()
	}
	wg.Wait()
}
