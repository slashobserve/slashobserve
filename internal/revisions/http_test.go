package revisions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func withRole(p auth.Principal) auth.Principal {
	p.Role = auth.RoleAdmin
	return p
}

func newRouter(svc *revisions.Service) *gin.Engine {
	return newRouterAs(svc, user)
}

func newRouterAs(svc *revisions.Service, caller auth.Principal) *gin.Engine {
	return newRouterWithRole(svc, withRole(caller))
}

func newRouterWithRole(svc *revisions.Service, caller auth.Principal) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc.Register(auth.NewRoutes(r.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(datasource.WithAccess(auth.WithPrincipal(c.Request.Context(), caller), datasource.Access{By: caller}))
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

func TestAnObserveRequestOfAHiddenDashboardIsNotFound(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	hidden := seedDashboard(t, d, 1)
	rev := submit(t, svc, hidden, "go")
	foldertest.Move(t, d, hidden, foldertest.Restricted(t, d, "Private", "nobody"))
	id := rev.ID

	for role, want := range map[auth.Role]int{auth.RoleEditor: http.StatusNotFound, auth.RoleAdmin: http.StatusOK} {
		r := gin.New()
		revisions.NewObserveHandler(svc, revisions.ObserveOptions{}).Register(auth.NewRoutes(r.Group("/api/v1", func(c *gin.Context) {
			c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), asRole(role)))
		})))
		rec := call(r, http.MethodGet, "/api/v1/observe/"+id, "")
		if rec.Code != want {
			t.Errorf("%s: status %d, want %d: %s", role, rec.Code, want, rec.Body)
		}
	}
}

func TestRevisionRoutesAnswerForbiddenRoleWithoutComment(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)

	for name, tc := range map[string]struct {
		role   auth.Role
		method string
		path   string
		body   string
		want   int
	}{
		"Viewer reads the list":                    {auth.RoleViewer, http.MethodGet, revisionsPath(dash), "", http.StatusOK},
		"Viewer may not follow up through observe": {auth.RoleViewer, http.MethodPost, "/api/v1/observe", `{"prompt":"go","kind":"revise","dashboard_id":"` + dash.String() + `"}`, http.StatusForbidden},
		"Commenter may not start a new dashboard":  {auth.RoleCommenter, http.MethodPost, "/api/v1/observe", `{"prompt":"go"}`, http.StatusForbidden},
		"Viewer may not start a new dashboard":     {auth.RoleViewer, http.MethodPost, "/api/v1/observe", `{"prompt":"go"}`, http.StatusForbidden},
		"Commenter follows up through observe":     {auth.RoleCommenter, http.MethodPost, "/api/v1/observe", `{"prompt":"go","kind":"revise","dashboard_id":"` + dash.String() + `"}`, http.StatusAccepted},
		"Commenter may not ask for a suggestion":   {auth.RoleCommenter, http.MethodPost, "/api/v1/dashboards/" + dash.String() + "/suggestions", `{"prompt":"go","base_version":1,"panel_id":"p1"}`, http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			r := gin.New()
			api := auth.NewRoutes(r.Group("/api/v1", func(c *gin.Context) {
				c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), asRole(tc.role)))
			}))
			svc.Register(api)
			revisions.NewObserveHandler(svc, revisions.ObserveOptions{}).Register(api)
			rec := call(r, tc.method, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.want, rec.Body)
			}
			if tc.want == http.StatusForbidden && errorCode(t, rec) != "forbidden_role" {
				t.Fatalf("code %q, want forbidden_role", errorCode(t, rec))
			}
		})
	}
}

func TestRevisionsCarryTheActionsTheCallerMayTake(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	stopped := submit(t, svc, dash, "fails with a build")
	claimed, _ := claimOne(t, svc)
	if _, err := d.Queries().UpsertAgentRequestBuild(reading(context.Background()), dbsqlc.UpsertAgentRequestBuildParams{
		AgentRequestID: claimed.ID, DashboardID: dash, BaseVersion: pgtype.Int4{Int32: 1, Valid: true}, Spec: []byte(`{}`), Panels: 1, ClaimEpoch: claimed.ClaimEpoch,
	}); err != nil {
		t.Fatal(err)
	}
	failed := submit(t, svc, dash, "fails without a build")
	queued := submit(t, svc, dash, "still waiting")
	d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed', failure_code = 'no_progress', error = 'stopped', lease_until = NULL, finished_at = now() WHERE id = ANY($1::uuid[])", []string{failed.ID, stopped.ID})

	cases := []struct {
		name   string
		caller auth.Principal
		router func(*revisions.Service, auth.Principal) *gin.Engine
		want   map[string][]string
	}{
		{"requester", user, newRouterAs, map[string][]string{
			failed.ID:  {"retry", "pin", "archive"},
			stopped.ID: {"keep_build", "continue", "retry", "pin", "archive"},
			queued.ID:  {"cancel", "pin", "archive"},
		}},
		{"requester who can only comment", asRole(auth.RoleCommenter), newRouterWithRole, map[string][]string{
			failed.ID:  {"retry", "pin", "archive"},
			stopped.ID: {"continue", "retry", "pin", "archive"},
			queued.ID:  {"cancel", "pin", "archive"},
		}},
		{"another editor", otherUser, newRouterAs, map[string][]string{
			failed.ID:  {"retry"},
			stopped.ID: {"retry"},
			queued.ID:  {},
		}},
	}
	for _, c := range cases {
		r := c.router(svc, c.caller)
		var page struct {
			Revisions []struct {
				ID             string   `json:"id"`
				AllowedActions []string `json:"allowed_actions"`
			} `json:"revisions"`
		}
		rec := call(r, http.MethodGet, revisionsPath(dash), "")
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || rec.Code != http.StatusOK || len(page.Revisions) != 3 {
			t.Fatalf("%s list: %d %s", c.name, rec.Code, rec.Body)
		}
		for _, rev := range page.Revisions {
			if !slices.Equal(rev.AllowedActions, c.want[rev.ID]) || rev.AllowedActions == nil {
				t.Errorf("%s list: %s allowed_actions = %v, want %v", c.name, rev.ID, rev.AllowedActions, c.want[rev.ID])
			}
		}
		for id, want := range c.want {
			var one struct {
				Revision struct {
					AllowedActions []string `json:"allowed_actions"`
				} `json:"revision"`
			}
			rec := call(r, http.MethodGet, revisionsPath(dash)+"/"+id, "")
			if err := json.Unmarshal(rec.Body.Bytes(), &one); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("%s get %s: %d %s", c.name, id, rec.Code, rec.Body)
			}
			if !slices.Equal(one.Revision.AllowedActions, want) || one.Revision.AllowedActions == nil {
				t.Errorf("%s get: %s allowed_actions = %v, want %v", c.name, id, one.Revision.AllowedActions, want)
			}
		}
	}
}

func TestAStoppedBuildOnAStarterDashboardIsNotOfferedKeepBuild(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	stopped := submit(t, svc, dash, "fails with a build")
	claimed, _ := claimOne(t, svc)
	if _, err := d.Queries().UpsertAgentRequestBuild(reading(context.Background()), dbsqlc.UpsertAgentRequestBuildParams{
		AgentRequestID: claimed.ID, DashboardID: dash, BaseVersion: pgtype.Int4{Int32: 1, Valid: true}, Spec: []byte(`{}`), Panels: 1, ClaimEpoch: claimed.ClaimEpoch,
	}); err != nil {
		t.Fatal(err)
	}
	d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed', failure_code = 'no_progress', error = 'stopped', lease_until = NULL, finished_at = now() WHERE id = $1", stopped.ID)
	d.Exec(t, "INSERT INTO starter_content (key, ref) VALUES ('audit/dashboards/test', $1)", dash.String())

	rec := call(newRouterAs(svc, user), http.MethodGet, revisionsPath(dash)+"/"+stopped.ID, "")
	var one struct {
		Revision struct {
			AllowedActions []string `json:"allowed_actions"`
		} `json:"revision"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &one); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	if slices.Contains(one.Revision.AllowedActions, "keep_build") {
		t.Fatalf("allowed_actions = %v, want no keep_build on a starter", one.Revision.AllowedActions)
	}
}

func TestAQueuedRevisionIsCreatedWithItsActions(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rec := call(newObserveRouter(svc), http.MethodPost, "/api/v1/observe", `{"prompt":"add a panel","kind":"revise","dashboard_id":"`+dash.String()+`"}`)
	var body struct {
		Request struct {
			AllowedActions []string `json:"allowed_actions"`
		} `json:"request"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusAccepted {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if want := []string{"cancel", "pin", "archive"}; !slices.Equal(body.Request.AllowedActions, want) {
		t.Errorf("allowed_actions = %v, want %v", body.Request.AllowedActions, want)
	}
}

func TestEveryRouteThatStartsAChangeOfAStarterAnswersProvisioned(t *testing.T) {
	svc, d := newSuggestService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	failed := submit(t, svc, dash, "made before it became a starter")
	setState(t, d, failed.ID, "failed")
	d.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ('audit/dashboards/test', $1)`, dash.String())
	editor := asRole(auth.RoleEditor)
	r := gin.New()
	api := auth.NewRoutes(r.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), editor))
	}))
	svc.Register(api)
	revisions.NewObserveHandler(svc, revisions.ObserveOptions{}).Register(api)
	before := runsOn(t, d, dash)

	for name, tc := range map[string]struct{ path, body string }{
		"a change through observe": {"/api/v1/observe", `{"prompt":"go","kind":"revise","dashboard_id":"` + dash.String() + `"}`},
		"a suggestion":             {"/api/v1/dashboards/" + dash.String() + "/suggestions", `{"version":1,"panel_id":"p1","prompt":"go"}`},
		"a retry":                  {"/api/v1/observe/" + failed.ID + "/retry", ""},
	} {
		rec := call(r, http.MethodPost, tc.path, tc.body)
		if rec.Code != http.StatusConflict || errorCode(t, rec) != folders.CodeProvisioned {
			t.Errorf("%s answered %d %s, want 409 provisioned", name, rec.Code, rec.Body)
		}
	}
	if after := runsOn(t, d, dash); after != before {
		t.Fatalf("the starter has %d runs after the refusals, want %d", after, before)
	}
}

func runsOn(t *testing.T, d *dbtest.Database, dash pgtype.UUID) int {
	t.Helper()
	var n int
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM agent_requests WHERE dashboard_id = $1`, dash).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
