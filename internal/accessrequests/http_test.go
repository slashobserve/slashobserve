package accessrequests

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func serve(t *testing.T, r rig, by auth.Principal, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), by))
	})
	NewHandler(r.service, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/api/v1/access-requests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	return rec
}

func codeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return body.Code
}

func objectBody(kind string, id pgtype.UUID, note string) string {
	raw, _ := json.Marshal(map[string]any{"object": map[string]string{"kind": kind, "id": id.String()}, "note": note})
	return string(raw)
}

func TestHTTPRequestsCommentAccessAndListsItForTheGranter(t *testing.T) {
	r := newRig(t)
	folder := r.folderBy(t, "Payments", ben)
	d := r.dashboard(t, "Checkout", folder)

	rec := serve(t, r, ada, http.MethodPost, objectBody("dashboard", d, "on call"))
	var created RequestJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s, want 201", rec.Code, rec.Body)
	}
	if created.Object != (ObjectJSON{Kind: KindDashboard, ID: d.String(), Title: "Checkout"}) || created.RequestedBy.Name != "Ada" || created.Note != "on call" || created.CreatedAt == "" {
		t.Fatalf("created = %+v", created)
	}
	if strings.Contains(rec.Body.String(), ben.ID) || strings.Contains(rec.Body.String(), "granters") {
		t.Fatalf("the answer to the requester names a granter: %s", rec.Body)
	}

	rec = serve(t, r, ada, http.MethodPost, objectBody("dashboard", d, ""))
	var open struct {
		Error       string `json:"error"`
		Code        string `json:"code"`
		RequestedAt string `json:"requested_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &open); err != nil || rec.Code != http.StatusConflict || open.Code != CodeRequestOpen || open.RequestedAt != created.CreatedAt || !strings.Contains(open.Error, created.CreatedAt[:19]) {
		t.Fatalf("second create = %d %s, want 409 request_open at %s", rec.Code, rec.Body, created.CreatedAt)
	}

	rec = serve(t, r, ben, http.MethodGet, "")
	var list ListJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK || len(list.Items) != 1 || list.Items[0].ID != created.ID {
		t.Fatalf("Ben's list = %d %s, want Ada's request", rec.Code, rec.Body)
	}
	if rec := serve(t, r, ada, http.MethodGet, ""); rec.Code != http.StatusOK || rec.Body.String() != `{"items":[]}` {
		t.Fatalf("Ada's list = %d %s, want empty", rec.Code, rec.Body)
	}
}

func TestHTTPRefusesWithTheSameAnswersAsElsewhere(t *testing.T) {
	r := newRig(t)
	hidden := r.folderBy(t, "Hidden", editor)
	r.grant(t, hidden, cleo, "view")
	secret := r.dashboard(t, "Secret", hidden)
	general := r.dashboard(t, "Checkout", pgtype.UUID{})
	unknown := r.uuid(t, "99999999-9999-4999-8999-999999999999")

	cases := []struct {
		name   string
		by     auth.Principal
		body   string
		status int
		code   string
	}{
		{"a hidden dashboard", ada, objectBody("dashboard", secret, ""), http.StatusNotFound, "not_found"},
		{"an unknown dashboard", ada, objectBody("dashboard", unknown, ""), http.StatusNotFound, "not_found"},
		{"an unknown observation", ada, objectBody("observation", unknown, ""), http.StatusNotFound, "not_found"},
		{"an Editor", ben, objectBody("dashboard", general, ""), http.StatusConflict, CodeCanComment},
		{"a note on two lines", ada, objectBody("dashboard", general, "one\ntwo"), http.StatusBadRequest, CodeInvalidNote},
		{"a note too long", ada, objectBody("dashboard", general, strings.Repeat("x", 201)), http.StatusBadRequest, CodeInvalidNote},
		{"a folder", ada, objectBody("folder", general, ""), http.StatusBadRequest, "invalid_request"},
		{"no object", ada, `{"note":"hi"}`, http.StatusBadRequest, "invalid_request"},
		{"a bad id", ada, `{"object":{"kind":"dashboard","id":"nope"}}`, http.StatusBadRequest, "invalid_request"},
	}
	for _, tc := range cases {
		rec := serve(t, r, tc.by, http.MethodPost, tc.body)
		if rec.Code != tc.status || codeOf(t, rec) != tc.code {
			t.Errorf("%s: %d %s, want %d %s", tc.name, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
	if n := r.count(t, `SELECT count(*) FROM access_requests`); n != 0 {
		t.Fatalf("requests = %d, want none", n)
	}
}
