package threads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/forbidden"
)

type response struct {
	status int
	body   map[string]any
}

func (r *rig) call(t *testing.T, who auth.Principal, method, path string, body any) response {
	t.Helper()
	var reader *bytes.Reader
	switch b := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(b))
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), who))
	})
	NewThreads(r.service).Register(auth.NewRoutes(api))
	req := httptest.NewRequestWithContext(t.Context(), method, "/api/v1"+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	var parsed map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("%s %s: body %q: %v", method, path, rec.Body.String(), err)
	}
	return response{status: rec.Code, body: parsed}
}

func (r response) at(t *testing.T, path ...string) any {
	t.Helper()
	var current any = r.body
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%v has no %q in %v", path, key, r.body)
		}
		current = m[key]
	}
	return current
}

func (r response) expect(t *testing.T, status int, code string) {
	t.Helper()
	if r.status != status {
		t.Fatalf("status = %d, want %d: %v", r.status, status, r.body)
	}
	if code != "" && r.body["code"] != code {
		t.Fatalf("code = %v, want %s: %v", r.body["code"], code, r.body)
	}
}

func TestOpeningAThreadOnEachTargetAnswersThreadAndMessage(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	finding := r.createFinding(t, testUser, dash)
	global := r.createAnnotation(t, testUser, pgtype.UUID{})
	bo := r.createUser(t, "Bo Baker", "bo@example.com")

	onPanel := r.call(t, commenter, http.MethodPost, "/threads", map[string]any{
		"target": "dashboard:" + dash.String(), "panel_id": "rps", "body": "see @[x](user:" + bo.String() + ") and @observe",
	})
	onPanel.expect(t, http.StatusCreated, "")
	if got := onPanel.at(t, "thread", "target", "kind"); got != "dashboard" {
		t.Errorf("target kind = %v", got)
	}
	if onPanel.at(t, "thread", "dashboard_id") != dash.String() || onPanel.at(t, "thread", "version") != 1.0 || onPanel.at(t, "thread", "panel_id") != "rps" ||
		onPanel.at(t, "thread", "status") != "open" || onPanel.at(t, "thread", "panel_present") != true || onPanel.at(t, "thread", "revision") != nil ||
		onPanel.at(t, "thread", "resolved_actor") != nil {
		t.Errorf("thread = %v", onPanel.body["thread"])
	}
	if onPanel.at(t, "message", "id") != onPanel.at(t, "thread", "id") || onPanel.at(t, "message", "thread_id") != onPanel.at(t, "thread", "id") ||
		onPanel.at(t, "message", "actor", "label") != "Bo" || onPanel.at(t, "message", "can_edit") != true || onPanel.at(t, "message", "edited_at") != nil {
		t.Errorf("message = %v", onPanel.body["message"])
	}
	if mentions, _ := onPanel.at(t, "message", "mentions").([]any); len(mentions) != 2 {
		t.Errorf("mentions = %v", onPanel.at(t, "message", "mentions"))
	}

	onFinding := r.call(t, commenter, http.MethodPost, "/threads", map[string]any{"target": "finding:" + finding.String(), "body": "agreed"})
	onFinding.expect(t, http.StatusCreated, "")
	if onFinding.at(t, "thread", "dashboard_id") != nil || onFinding.at(t, "thread", "version") != nil || onFinding.at(t, "thread", "target", "id") != finding.String() {
		t.Errorf("finding thread = %v", onFinding.body["thread"])
	}

	onGlobal := r.call(t, commenter, http.MethodPost, "/threads", map[string]any{"target": "annotation:" + global.String(), "body": "noted"})
	onGlobal.expect(t, http.StatusCreated, "")
	if onGlobal.at(t, "thread", "target", "kind") != "annotation" {
		t.Errorf("annotation thread = %v", onGlobal.body["thread"])
	}
}

func TestListingThreadsPagesByMessages(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	finding := r.createFinding(t, testUser, dash)
	targetValue := "finding:" + finding.String()
	opened := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": targetValue, "body": "first"})
	threadID := opened.at(t, "thread", "id").(string)
	r.call(t, commenter, http.MethodPost, "/threads/"+threadID+"/messages", map[string]any{"body": "second"}).expect(t, http.StatusCreated, "")
	r.call(t, testUser, http.MethodPost, "/threads/"+threadID+"/messages", map[string]any{"body": "third"}).expect(t, http.StatusCreated, "")
	other := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": targetValue, "body": "other thread"})
	other.expect(t, http.StatusCreated, "")

	first := r.call(t, viewer, http.MethodGet, "/threads?limit=2&target="+targetValue, nil)
	first.expect(t, http.StatusOK, "")
	messages := first.at(t, "messages").([]any)
	if len(messages) != 2 || len(first.at(t, "threads").([]any)) != 1 || first.at(t, "can_comment") != false {
		t.Fatalf("first page = %v", first.body)
	}
	cursor, _ := first.at(t, "next_cursor").(string)
	if cursor == "" {
		t.Fatalf("no cursor: %v", first.body)
	}

	second := r.call(t, viewer, http.MethodGet, "/threads?limit=2&target="+targetValue+"&cursor="+url.QueryEscape(cursor), nil)
	second.expect(t, http.StatusOK, "")
	if len(second.at(t, "messages").([]any)) != 2 || len(second.at(t, "threads").([]any)) != 2 || second.at(t, "next_cursor") != nil {
		t.Fatalf("second page = %v", second.body)
	}
	bodies := []string{}
	for _, page := range []response{first, second} {
		for _, m := range page.at(t, "messages").([]any) {
			bodies = append(bodies, m.(map[string]any)["body"].(string))
		}
	}
	if strings.Join(bodies, ",") != "first,second,third,other thread" {
		t.Errorf("messages in order = %v", bodies)
	}

	resolved := r.call(t, testUser, http.MethodPost, "/threads/"+threadID+"/resolve", nil)
	resolved.expect(t, http.StatusOK, "")
	open := r.call(t, viewer, http.MethodGet, "/threads?status=open&target="+targetValue, nil)
	if got := len(open.at(t, "messages").([]any)); got != 1 {
		t.Errorf("open messages = %d, want 1", got)
	}
}

func TestListingAThreadShowsItsQueuedRevision(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	comment := r.createComment(t, dash, NewComment{Version: 1, PanelID: new("rps"), Body: "change it", RequestRevision: true})

	page := r.call(t, testUser, http.MethodGet, "/threads?target=dashboard:"+dash.String(), nil)
	page.expect(t, http.StatusOK, "")
	threads := page.at(t, "threads").([]any)
	revision, _ := threads[0].(map[string]any)["revision"].(map[string]any)
	if len(threads) != 1 || revision["id"] != comment.Revision.ID || revision["status"] != "queued" {
		t.Fatalf("threads = %v", threads)
	}
}

func TestAViewerReadsAFindingThreadButCannotWriteToIt(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	finding := r.createFinding(t, testUser, dash)
	targetValue := "finding:" + finding.String()
	opened := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": targetValue, "body": "look"})
	threadID := opened.at(t, "thread", "id").(string)

	read := r.call(t, viewer, http.MethodGet, "/threads?target="+targetValue, nil)
	read.expect(t, http.StatusOK, "")
	if len(read.at(t, "messages").([]any)) != 1 || read.at(t, "can_comment") != false {
		t.Errorf("viewer page = %v", read.body)
	}
	if m := read.at(t, "messages").([]any)[0].(map[string]any); m["can_edit"] != false {
		t.Errorf("viewer can_edit = %v", m["can_edit"])
	}
	r.call(t, viewer, http.MethodPost, "/threads", map[string]any{"target": targetValue, "body": "no"}).expect(t, http.StatusForbidden, forbidden.Code)
	r.call(t, viewer, http.MethodPost, "/threads/"+threadID+"/messages", map[string]any{"body": "no"}).expect(t, http.StatusForbidden, forbidden.Code)
	r.call(t, viewer, http.MethodPost, "/threads/"+threadID+"/resolve", nil).expect(t, http.StatusForbidden, forbidden.Code)
	r.call(t, viewer, http.MethodPost, "/threads/"+threadID+"/reopen", nil).expect(t, http.StatusForbidden, forbidden.Code)
}

func TestAHiddenTargetOrThreadAnswersNotFound(t *testing.T) {
	r := newRig(t)
	detached := r.createFinding(t, testUser, pgtype.UUID{})
	targetValue := "finding:" + detached.String()
	opened := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": targetValue, "body": "private"})
	opened.expect(t, http.StatusCreated, "")
	threadID := opened.at(t, "thread", "id").(string)
	messageID := opened.at(t, "message", "id").(string)
	missing := "99999999-9999-4999-8999-999999999999"

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"list a finding of someone else without a dashboard", http.MethodGet, "/threads?target=" + targetValue, nil},
		{"open on it", http.MethodPost, "/threads", map[string]any{"target": targetValue, "body": "hi"}},
		{"list a missing dashboard", http.MethodGet, "/threads?target=dashboard:" + missing, nil},
		{"reply to its thread", http.MethodPost, "/threads/" + threadID + "/messages", map[string]any{"body": "hi"}},
		{"resolve its thread", http.MethodPost, "/threads/" + threadID + "/resolve", nil},
		{"reopen its thread", http.MethodPost, "/threads/" + threadID + "/reopen", nil},
		{"edit its message", http.MethodPatch, "/messages/" + messageID, map[string]any{"body": "hi"}},
		{"reply to a missing thread", http.MethodPost, "/threads/" + missing + "/messages", map[string]any{"body": "hi"}},
		{"edit a missing message", http.MethodPatch, "/messages/" + missing, map[string]any{"body": "hi"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r.call(t, commenter, tc.method, tc.path, tc.body).expect(t, http.StatusNotFound, "not_found")
		})
	}
}

func TestACommenterWritesToAGlobalAnnotationButAViewerCannot(t *testing.T) {
	r := newRig(t)
	global := r.createAnnotation(t, testUser, pgtype.UUID{})
	targetValue := "annotation:" + global.String()

	opened := r.call(t, commenter, http.MethodPost, "/threads", map[string]any{"target": targetValue, "body": "from a commenter"})
	opened.expect(t, http.StatusCreated, "")
	r.call(t, viewer, http.MethodPost, "/threads", map[string]any{"target": targetValue, "body": "no"}).expect(t, http.StatusForbidden, forbidden.Code)
	page := r.call(t, viewer, http.MethodGet, "/threads?target="+targetValue, nil)
	page.expect(t, http.StatusOK, "")
	if len(page.at(t, "messages").([]any)) != 1 {
		t.Errorf("viewer page = %v", page.body)
	}
}

func TestEditingAMessageThroughTheRoute(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	opened := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": "dashboard:" + dash.String(), "body": "draft"})
	messageID := opened.at(t, "message", "id").(string)

	edited := r.call(t, testUser, http.MethodPatch, "/messages/"+messageID, map[string]any{"body": "final @observe"})
	edited.expect(t, http.StatusOK, "")
	if edited.at(t, "message", "body") != "final @observe" || edited.at(t, "message", "edited_at") == nil || edited.at(t, "thread", "id") != messageID {
		t.Errorf("edited = %v", edited.body)
	}
	if mentions := edited.at(t, "message", "mentions").([]any); len(mentions) != 1 {
		t.Errorf("mentions = %v", mentions)
	}

	same := r.call(t, testUser, http.MethodPatch, "/messages/"+messageID, map[string]any{"body": "final @observe"})
	same.expect(t, http.StatusOK, "")
	if same.at(t, "message", "edited_at") != edited.at(t, "message", "edited_at") {
		t.Errorf("an unchanged body moved edited_at: %v then %v", edited.at(t, "message", "edited_at"), same.at(t, "message", "edited_at"))
	}

	r.call(t, commenter, http.MethodPatch, "/messages/"+messageID, map[string]any{"body": "mine now"}).expect(t, http.StatusForbidden, CodeNotAuthor)
	r.call(t, testUser, http.MethodPatch, "/messages/"+messageID, map[string]any{"body": "  "}).expect(t, http.StatusUnprocessableEntity, CodeInvalidBody)
}

func TestResolvingAndReopeningThroughTheRoutes(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	opened := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": "dashboard:" + dash.String(), "version": 1, "body": "fix"})
	threadID := opened.at(t, "thread", "id").(string)

	resolved := r.call(t, commenter, http.MethodPost, "/threads/"+threadID+"/resolve", map[string]any{"version": 2})
	resolved.expect(t, http.StatusOK, "")
	if resolved.at(t, "thread", "status") != "resolved" || resolved.at(t, "thread", "resolved_by_version") != 2.0 ||
		resolved.at(t, "thread", "resolved_actor", "label") != "Bo" || resolved.at(t, "thread", "resolved_at") == nil {
		t.Errorf("resolved = %v", resolved.body)
	}
	again := r.call(t, commenter, http.MethodPost, "/threads/"+threadID+"/resolve", "{}")
	again.expect(t, http.StatusOK, "")
	if again.at(t, "thread", "resolved_at") != resolved.at(t, "thread", "resolved_at") {
		t.Errorf("a repeat resolve changed the thread: %v", again.body)
	}

	reopened := r.call(t, commenter, http.MethodPost, "/threads/"+threadID+"/reopen", nil)
	reopened.expect(t, http.StatusOK, "")
	if reopened.at(t, "thread", "status") != "open" || reopened.at(t, "thread", "resolved_at") != nil || reopened.at(t, "thread", "resolved_actor") != nil ||
		reopened.at(t, "thread", "resolved_by_version") != nil {
		t.Errorf("reopened = %v", reopened.body)
	}

	r.call(t, commenter, http.MethodPost, "/threads/"+threadID+"/resolve", map[string]any{"version": 3}).expect(t, http.StatusUnprocessableEntity, CodeUnknownVersion)
	r.call(t, commenter, http.MethodPost, "/threads/"+threadID+"/resolve", map[string]any{"version": 0}).expect(t, http.StatusUnprocessableEntity, CodeInvalidVersion)
}

func TestRefusedRequestsAnswerTheirOwnCodes(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	annotation := r.createAnnotation(t, testUser, dash)
	dashTarget := "dashboard:" + dash.String()
	tooMany := make([]string, MaxMentions+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("@[n](user:00000000-0000-4000-8000-%012d)", i)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   any
		status int
		code   string
	}{
		{"no target", http.MethodPost, "/threads", map[string]any{"body": "hi"}, http.StatusUnprocessableEntity, CodeInvalidTarget},
		{"target without a kind", http.MethodPost, "/threads", map[string]any{"target": dash.String(), "body": "hi"}, http.StatusUnprocessableEntity, CodeInvalidTarget},
		{"unknown kind", http.MethodPost, "/threads", map[string]any{"target": "run:" + dash.String(), "body": "hi"}, http.StatusUnprocessableEntity, CodeInvalidTarget},
		{"target without a uuid", http.MethodPost, "/threads", map[string]any{"target": "dashboard:nope", "body": "hi"}, http.StatusUnprocessableEntity, CodeInvalidTarget},
		{"list without a target", http.MethodGet, "/threads", nil, http.StatusUnprocessableEntity, CodeInvalidTarget},
		{"empty body", http.MethodPost, "/threads", map[string]any{"target": dashTarget, "body": " "}, http.StatusUnprocessableEntity, CodeInvalidBody},
		{"missing body", http.MethodPost, "/threads", map[string]any{"target": dashTarget}, http.StatusUnprocessableEntity, CodeInvalidBody},
		{"too many mentions", http.MethodPost, "/threads", map[string]any{"target": dashTarget, "body": strings.Join(tooMany, " ")}, http.StatusUnprocessableEntity, CodeTooManyMentions},
		{"unknown panel", http.MethodPost, "/threads", map[string]any{"target": dashTarget, "panel_id": "missing", "body": "hi"}, http.StatusUnprocessableEntity, CodeUnknownPanel},
		{"empty panel id", http.MethodPost, "/threads", map[string]any{"target": dashTarget, "panel_id": "", "body": "hi"}, http.StatusUnprocessableEntity, CodeInvalidPanelID},
		{"unknown version", http.MethodPost, "/threads", map[string]any{"target": dashTarget, "version": 9, "body": "hi"}, http.StatusUnprocessableEntity, CodeUnknownVersion},
		{"zero version", http.MethodPost, "/threads", map[string]any{"target": dashTarget, "version": 0, "body": "hi"}, http.StatusUnprocessableEntity, CodeInvalidVersion},
		{"version on an annotation", http.MethodPost, "/threads", map[string]any{"target": "annotation:" + annotation.String(), "version": 1, "body": "hi"}, http.StatusUnprocessableEntity, CodeInvalidTarget},
		{"unknown status filter", http.MethodGet, "/threads?target=" + dashTarget + "&status=done", nil, http.StatusUnprocessableEntity, CodeInvalidStatus},
		{"empty status filter", http.MethodGet, "/threads?target=" + dashTarget + "&status=", nil, http.StatusBadRequest, "invalid_request"},
		{"limit over the maximum", http.MethodGet, "/threads?target=" + dashTarget + "&limit=501", nil, http.StatusBadRequest, "invalid_request"},
		{"bad cursor", http.MethodGet, "/threads?target=" + dashTarget + "&cursor=junk", nil, http.StatusBadRequest, "invalid_request"},
		{"unknown field", http.MethodPost, "/threads", map[string]any{"target": dashTarget, "body": "hi", "request_revision": true}, http.StatusBadRequest, "invalid_request"},
		{"malformed json", http.MethodPost, "/threads", "{", http.StatusBadRequest, "invalid_request"},
		{"thread id that is not a uuid", http.MethodPost, "/threads/nope/reopen", nil, http.StatusBadRequest, "invalid_request"},
		{"body over 64 KiB", http.MethodPost, "/threads", map[string]any{"target": dashTarget, "body": strings.Repeat("a", 65<<10)}, http.StatusRequestEntityTooLarge, "too_large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r.call(t, testUser, tc.method, tc.path, tc.body).expect(t, tc.status, tc.code)
		})
	}
}

func TestEitherIdOfAThreadNamesItInTheRoutes(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	opened := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": "dashboard:" + dash.String(), "body": "root"})
	threadID := opened.at(t, "thread", "id").(string)
	reply := r.call(t, testUser, http.MethodPost, "/threads/"+threadID+"/messages", map[string]any{"body": "reply"})
	reply.expect(t, http.StatusCreated, "")
	replyID := reply.at(t, "message", "id").(string)
	if replyID == threadID || reply.at(t, "message", "thread_id") != threadID {
		t.Fatalf("reply = %v", reply.body)
	}

	byReply := r.call(t, testUser, http.MethodPost, "/threads/"+replyID+"/resolve", nil)
	byReply.expect(t, http.StatusOK, "")
	if byReply.at(t, "thread", "id") != threadID || byReply.at(t, "thread", "status") != "resolved" {
		t.Errorf("resolve by a message id = %v", byReply.body)
	}
}
