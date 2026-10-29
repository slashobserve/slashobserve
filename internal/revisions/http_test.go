package revisions_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func withRole(p auth.Principal) auth.Principal {
	p.Role = auth.RoleAdmin
	return p
}

func newRouter(svc *revisions.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc.Register(auth.NewRoutes(r.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), withRole(user)))
	})))
	return r
}

func newObserveRouter(svc *revisions.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	revisions.NewObserveHandler(svc, revisions.ObserveOptions{}).Register(auth.NewRoutes(r.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), withRole(user)))
	})))
	return r
}

func call(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not an error body: %s", rec.Body)
	}
	return body.Code
}

func revisionsPath(dash pgtype.UUID) string {
	return "/api/v1/dashboards/" + dash.String() + "/revisions"
}

func TestPostQueuesARevisionUntilTheDashboardIsFull(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 2)
	r := newRouter(svc)

	var location, id string
	for i := range revisions.MaxActive {
		rec := call(r, http.MethodPost, revisionsPath(dash), fmt.Sprintf(`{"prompt":"  add panel %d  ","base_version":2}`, i))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("post %d: %d %s", i, rec.Code, rec.Body)
		}
		var body struct {
			Revision map[string]any `json:"revision"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		rev := body.Revision
		if rev["status"] != "queued" || rev["source"] != "prompt" || rev["base_version"] != float64(2) || rev["comment_id"] != nil ||
			rev["panel_id"] != nil || rev["result_version"] != nil || rev["error"] != nil || rev["attempts"] != float64(0) {
			t.Fatalf("revision = %v", rev)
		}
		if rev["prompt"] != fmt.Sprintf("add panel %d", i) {
			t.Fatalf("the prompt must be trimmed: %v", rev["prompt"])
		}
		by, _ := rev["requested_by"].(map[string]any)
		if by["kind"] != "user" || by["id"] != userID || by["name"] != "Ada" || by["email"] != "ada@example.com" {
			t.Fatalf("requested_by = %v", by)
		}
		if i == 0 {
			id, _ = rev["id"].(string)
			location = rec.Header().Get("Location")
		}
	}
	if want := revisionsPath(dash) + "/" + id; location != want {
		t.Fatalf("Location = %q, want %q", location, want)
	}

	rec := call(r, http.MethodPost, revisionsPath(dash), `{"prompt":"one too many","base_version":2}`)
	if rec.Code != http.StatusTooManyRequests || errorCode(t, rec) != "too_many_revisions" {
		t.Fatalf("sixth post: %d %s", rec.Code, rec.Body)
	}
	items, _, err := svc.List(t.Context(), dash, 100, "")
	if err != nil || len(items) != revisions.MaxActive {
		t.Fatalf("stored %d, err %v", len(items), err)
	}

	rec = call(r, http.MethodGet, revisionsPath(dash)+"/"+id, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"`+id+`"`) {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
}

func TestPostRefusesInvalidRequests(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 2)
	r := newRouter(svc)
	nilID := "00000000-0000-0000-0000-000000000000"

	for name, c := range map[string]struct {
		path, body string
		status     int
		code       string
	}{
		"missing prompt":       {revisionsPath(dash), `{"base_version":1}`, 400, "invalid_request"},
		"missing base version": {revisionsPath(dash), `{"prompt":"x"}`, 400, "invalid_request"},
		"blank prompt":         {revisionsPath(dash), `{"prompt":"  \n ","base_version":1}`, 400, "invalid_request"},
		"NUL in prompt":        {revisionsPath(dash), `{"prompt":"a\u0000b","base_version":1}`, 400, "invalid_request"},
		"bidi override":        {revisionsPath(dash), `{"prompt":"a\u202eb","base_version":1}`, 400, "invalid_request"},
		"long prompt":          {revisionsPath(dash), `{"prompt":"` + strings.Repeat("a", 4001) + `","base_version":1}`, 400, "invalid_request"},
		"zero base version":    {revisionsPath(dash), `{"prompt":"x","base_version":0}`, 400, "invalid_request"},
		"huge base version":    {revisionsPath(dash), `{"prompt":"x","base_version":2147483648}`, 400, "invalid_request"},
		"unknown field":        {revisionsPath(dash), `{"prompt":"x","base_version":1,"extra":1}`, 400, "invalid_request"},
		"not json":             {revisionsPath(dash), `nope`, 400, "invalid_request"},
		"base version unknown": {revisionsPath(dash), `{"prompt":"x","base_version":3}`, 404, "not_found"},
		"unknown dashboard":    {"/api/v1/dashboards/" + nilID + "/revisions", `{"prompt":"x","base_version":1}`, 404, "not_found"},
		"malformed dashboard":  {"/api/v1/dashboards/nope/revisions", `{"prompt":"x","base_version":1}`, 400, "invalid_request"},
	} {
		rec := call(r, http.MethodPost, c.path, c.body)
		if rec.Code != c.status || errorCode(t, rec) != c.code {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	items, _, err := svc.List(t.Context(), dash, 100, "")
	if err != nil || len(items) != 0 {
		t.Fatalf("a refused request was stored: %v, %v", items, err)
	}
}

func TestPostWithoutAnAgentAnswers503AndQueuesNothing(t *testing.T) {
	svc, d := newService(t, nil)
	dash := seedDashboard(t, d, 1)
	r := newRouter(svc)
	rec := call(r, http.MethodPost, revisionsPath(dash), `{"prompt":"x","base_version":1}`)
	if rec.Code != http.StatusServiceUnavailable || errorCode(t, rec) != "agent_unavailable" {
		t.Fatalf("post: %d %s", rec.Code, rec.Body)
	}
	rec = call(r, http.MethodGet, revisionsPath(dash), "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"revisions":[]`) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
}

func TestGetAndListRevisions(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash, other := seedDashboard(t, d, 1), seedDashboard(t, d, 1)
	r := newRouter(svc)
	var ids []string
	for i := range 3 {
		ids = append([]string{submit(t, svc, dash, fmt.Sprintf("p%d", i)).ID}, ids...)
	}
	foreign := submit(t, svc, other, "elsewhere")

	rec := call(r, http.MethodGet, revisionsPath(dash)+"?limit=2", "")
	var page struct {
		Revisions  []struct{ ID string } `json:"revisions"`
		NextCursor *string               `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if len(page.Revisions) != 2 || page.NextCursor == nil || page.Revisions[0].ID != ids[0] || page.Revisions[1].ID != ids[1] {
		t.Fatalf("first page = %+v", page)
	}
	rec = call(r, http.MethodGet, revisionsPath(dash)+"?limit=2&cursor="+*page.NextCursor, "")
	page.NextCursor = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Revisions) != 1 || page.NextCursor != nil || page.Revisions[0].ID != ids[2] {
		t.Fatalf("second page: %d %s", rec.Code, rec.Body)
	}

	for name, c := range map[string]struct {
		path   string
		status int
	}{
		"bad limit":                     {revisionsPath(dash) + "?limit=0", 400},
		"huge limit":                    {revisionsPath(dash) + "?limit=201", 400},
		"bad cursor":                    {revisionsPath(dash) + "?cursor=!!!", 400},
		"unknown dashboard":             {"/api/v1/dashboards/00000000-0000-0000-0000-000000000000/revisions", 404},
		"revision of another dashboard": {revisionsPath(dash) + "/" + foreign.ID, 404},
		"unknown revision":              {revisionsPath(dash) + "/00000000-0000-0000-0000-000000000000", 404},
		"malformed revision":            {revisionsPath(dash) + "/nope", 400},
	} {
		if rec := call(r, http.MethodGet, c.path, ""); rec.Code != c.status {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
}
