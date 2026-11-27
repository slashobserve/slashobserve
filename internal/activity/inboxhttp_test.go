package activity

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api/apitest"
	"github.com/slashobserve/slashobserve/internal/auth"
)

var botKey = auth.Principal{Kind: auth.KindServiceAccount, ID: "99999999-9999-4999-8999-999999999999", Name: "ci", Role: auth.RoleAdmin}

func (r rig) call(t *testing.T, who auth.Principal, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), who))
	})
	NewHandler(r.store, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(group))
	req := httptest.NewRequestWithContext(t.Context(), method, "/api/v1"+path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return body.Code
}

func (r rig) validated(t *testing.T, rec *httptest.ResponseRecorder, method, path string) {
	t.Helper()
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc := apitest.Load(t)
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	check := httptest.NewRequestWithContext(t.Context(), method, "/api/v1"+path, bytes.NewBufferString(`{"seq":1}`))
	check.Header.Set("Content-Type", "application/json")
	route, params, err := router.FindRoute(check)
	if err != nil {
		t.Fatalf("%s %s is not documented: %v", method, path, err)
	}
	options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true, SkipSettingDefaults: true}
	input := &openapi3filter.RequestValidationInput{Request: check, PathParams: params, Route: route, Options: options}
	response := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: rec.Code, Header: rec.Header(), Options: options}
	response.SetBodyBytes(rec.Body.Bytes())
	if err := openapi3filter.ValidateResponse(t.Context(), response); err != nil {
		t.Fatalf("%s %s answered %d against the document: %v\n%s", method, path, rec.Code, err, rec.Body)
	}
}

func TestInboxRoutesRefuseKeys(t *testing.T) {
	r := newRig(t)
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/inbox", ""},
		{http.MethodPost, "/inbox/read", `{"seq":1}`},
	} {
		rec := r.call(t, botKey, c.method, c.path, c.body)
		if rec.Code != http.StatusForbidden || errorCode(t, rec) != "user_session_required" {
			t.Errorf("%s %s by a key = %d %s, want 403 user_session_required", c.method, c.path, rec.Code, rec.Body)
		}
	}
}

func TestInboxListRefusesBadParameters(t *testing.T) {
	r := newRig(t)
	for _, c := range []struct{ query, code string }{
		{"view=everyone", "invalid_view"},
		{"view=", "invalid_view"},
		{"filter=archived", "invalid_filter"},
		{"filter=", "invalid_filter"},
		{"cursor=not-a-cursor", "invalid_cursor"},
		{"limit=0", "invalid_limit"},
		{"limit=101", "invalid_limit"},
		{"limit=ten", "invalid_limit"},
	} {
		rec := r.call(t, mia, http.MethodGet, "/inbox?"+c.query, "")
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != c.code {
			t.Errorf("GET /inbox?%s = %d %s, want 400 %s", c.query, rec.Code, rec.Body, c.code)
		}
	}
}

func TestInboxReadRefusesBadBodiesAndUnknownSeqs(t *testing.T) {
	r := newRig(t)
	r.inboxUser(t, mia)
	for _, c := range []struct {
		body   string
		status int
		code   string
		name   string
	}{
		{`{}`, http.StatusBadRequest, "invalid_request", "a body without seq"},
		{`{"seq":0}`, http.StatusBadRequest, "invalid_request", "seq 0"},
		{`{"seq":1,"extra":true}`, http.StatusBadRequest, "invalid_request", "an unknown field"},
		{`{"seq":987654}`, http.StatusUnprocessableEntity, "unknown_seq", "a seq nothing has"},
	} {
		rec := r.call(t, mia, http.MethodPost, "/inbox/read", c.body)
		if rec.Code != c.status || errorCode(t, rec) != c.code {
			t.Errorf("%s: POST /inbox/read = %d %s, want %d %s", c.name, rec.Code, rec.Body, c.status, c.code)
		}
		r.validated(t, rec, http.MethodPost, "/inbox/read")
	}
}

func TestInboxListAndReadMatchTheDocumentAndEachOther(t *testing.T) {
	r := newRig(t)
	r.inboxUser(t, mia)
	o := r.observation(t, ola)
	thread := r.thread(t, o)
	mention := r.mention(t, o, thread, ola, mia.ID)
	plainEvent := r.append(t, Entry{Type: TypeObservationUpdated, TargetID: o, Owner: OnObservation(o), Actor: ola, Data: map[string]any{"title": "x"}})

	rec := r.call(t, mia, http.MethodGet, "/inbox?view=everything&limit=1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /inbox = %d %s", rec.Code, rec.Body)
	}
	r.validated(t, rec, http.MethodGet, "/inbox?view=everything&limit=1")
	var first struct {
		Items []struct {
			Seq    int64   `json:"seq"`
			Reason *string `json:"reason"`
			Unread bool    `json:"unread"`
			State  string  `json:"state"`
		} `json:"items"`
		NextCursor  *string `json:"next_cursor"`
		UnreadCount int     `json:"unread_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].Seq != plainEvent || first.Items[0].Reason != nil || !first.Items[0].Unread || first.Items[0].State != string(ItemUnread) || first.NextCursor == nil || first.UnreadCount != 1 {
		t.Fatalf("first page = %+v, want the plain event unread with a null reason, a next cursor and 1 unread", first)
	}

	next := "/inbox?view=everything&limit=1&cursor=" + url.QueryEscape(*first.NextCursor)
	rec = r.call(t, mia, http.MethodGet, next, "")
	r.validated(t, rec, http.MethodGet, next)
	if !strings.Contains(rec.Body.String(), `"reason":"mention"`) {
		t.Fatalf("second page %s, want the mention", rec.Body)
	}

	read := r.call(t, mia, http.MethodPost, "/inbox/read", `{"seq":`+strconv.FormatInt(mention, 10)+`}`)
	if read.Code != http.StatusNoContent {
		t.Fatalf("POST /inbox/read = %d %s", read.Code, read.Body)
	}
	r.validated(t, read, http.MethodPost, "/inbox/read")
	rec = r.call(t, mia, http.MethodGet, "/inbox", "")
	if !strings.Contains(rec.Body.String(), `"unread_count":0`) {
		t.Fatalf("after reading, GET /inbox = %s, want unread_count 0", rec.Body)
	}

	path := "/inbox/items/" + strconv.FormatInt(mention, 10)
	unread := r.call(t, mia, http.MethodPut, path, `{"unread":true}`)
	if unread.Code != http.StatusNoContent {
		t.Fatalf("PUT %s = %d %s", path, unread.Code, unread.Body)
	}
	r.validated(t, unread, http.MethodPut, path)
	rec = r.call(t, mia, http.MethodGet, "/inbox", "")
	if !strings.Contains(rec.Body.String(), `"unread_count":1`) {
		t.Fatalf("after marking unread, GET /inbox = %s, want unread_count 1", rec.Body)
	}
	if got := r.call(t, mia, http.MethodPut, path, `{}`).Code; got != http.StatusBadRequest {
		t.Fatalf("PUT without unread or done = %d, want 400", got)
	}
	if got := r.call(t, mia, http.MethodPut, path, `{"unread":true,"done":true}`).Code; got != http.StatusBadRequest {
		t.Fatalf("PUT with unread and done = %d, want 400", got)
	}
	done := r.call(t, mia, http.MethodPut, path, `{"done":true}`)
	if done.Code != http.StatusNoContent {
		t.Fatalf("PUT %s done = %d %s", path, done.Code, done.Body)
	}
	r.validated(t, done, http.MethodPut, path)
	rec = r.call(t, mia, http.MethodGet, "/inbox?filter=done", "")
	r.validated(t, rec, http.MethodGet, "/inbox?filter=done")
	if !strings.Contains(rec.Body.String(), `"unread_count":0`) || !strings.Contains(rec.Body.String(), `"reason":"mention"`) || !strings.Contains(rec.Body.String(), `"state":"done"`) {
		t.Fatalf("GET /inbox?filter=done = %s, want the mention in state done and unread_count 0", rec.Body)
	}
	rec = r.call(t, mia, http.MethodGet, "/inbox?filter=unread", "")
	r.validated(t, rec, http.MethodGet, "/inbox?filter=unread")
	if strings.Contains(rec.Body.String(), `"reason":"mention"`) {
		t.Fatalf("GET /inbox?filter=unread = %s, want no done item", rec.Body)
	}
	if got := r.call(t, mia, http.MethodPut, "/inbox/items/999999", `{"unread":true}`).Code; got != http.StatusUnprocessableEntity {
		t.Fatalf("PUT of an unknown seq = %d, want 422", got)
	}
	if got := r.call(t, botKey, http.MethodPut, path, `{"unread":true}`).Code; got != http.StatusForbidden {
		t.Fatalf("PUT by a key = %d, want 403", got)
	}
}

func TestInboxItemSchemaCarriesEveryActivityEventProperty(t *testing.T) {
	doc := apitest.Load(t)
	event := slices.Sorted(maps.Keys(doc.Components.Schemas["ActivityEvent"].Value.Properties))
	item := doc.Components.Schemas["InboxItem"].Value
	for _, name := range event {
		if _, ok := item.Properties[name]; !ok {
			t.Errorf("InboxItem lacks the activity event property %q", name)
		}
	}
	if got := len(item.Properties) - len(event); got != 6 {
		t.Errorf("InboxItem adds %d properties to the event, want reason, unread, state, target_title, excerpt and question", got)
	}
}
