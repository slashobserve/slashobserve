package revisions_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r observationRig) routerAs(who auth.Principal) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	opts := revisions.ObserveOptions{
		ShareTitle:         r.observations.RenameFromChat,
		ObservationVisible: r.observations.Visible,
	}
	revisions.NewObserveHandler(r.svc, opts).Register(auth.NewRoutes(router.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(datasource.WithAccess(auth.WithPrincipal(c.Request.Context(), who), datasource.Access{By: who}))
	})))
	return router
}

func TestARunNamesNoObservationTheCallerCannotOpen(t *testing.T) {
	r := newObservationRig(t)
	dash := seedDashboard(t, r.d, 1)
	row := investigate(t, r.svc, revisions.InvestigateParams{DashboardID: dash, Prompt: "why are checkout errors up"})
	id := row.ID
	named := `"observation_id":"` + row.ObservationID.String() + `"`
	router := r.routerAs(user)

	for _, step := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/observe/" + id.String(), ""},
		{http.MethodGet, "/api/v1/observe", ""},
		{http.MethodPatch, "/api/v1/observe/" + id.String(), `{"title":"Checkout errors"}`},
	} {
		rec := call(router, step.method, step.path, step.body)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), named) {
			t.Fatalf("%s %s before the move: %d %s", step.method, step.path, rec.Code, rec.Body)
		}
	}

	hidden := foldertest.Restricted(t, r.d, "On call", "oncall")
	r.d.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, row.ObservationID, hidden)
	for _, step := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/observe/" + id.String(), ""},
		{http.MethodGet, "/api/v1/observe", ""},
		{http.MethodPatch, "/api/v1/observe/" + id.String(), `{"title":"Renamed again"}`},
	} {
		rec := call(router, step.method, step.path, step.body)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || strings.Contains(body, row.ObservationID.String()) || !strings.Contains(body, `"observation_id":null`) {
			t.Errorf("%s %s after the move: %d %s", step.method, step.path, rec.Code, body)
		}
	}
}

func TestARunNamesNoSubjectTheCallerCannotOpen(t *testing.T) {
	r := newObservationRig(t)
	dash := seedDashboard(t, r.d, 1)
	about := investigate(t, r.svc, revisions.InvestigateParams{DashboardID: dash, Prompt: "why are checkout errors up"}).ObservationID
	ask := startTurn(t, r.svc, user, revisions.StartRequest{Prompt: text("what changed since"), ObservationID: text(about.String())})
	path := "/api/v1/observe/" + ask.String()
	router := r.routerAs(user)

	if rec := call(router, http.MethodGet, path, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"subject_observation_id":"`+about.String()+`"`) {
		t.Fatalf("before the move: %d %s", rec.Code, rec.Body)
	}
	hidden := foldertest.Restricted(t, r.d, "On call", "oncall")
	r.d.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, about, hidden)
	rec := call(router, http.MethodGet, path, "")
	if body := rec.Body.String(); rec.Code != http.StatusOK || strings.Contains(body, about.String()) || !strings.Contains(body, `"subject_observation_id":null`) {
		t.Fatalf("after the move: %d %s", rec.Code, body)
	}
}

func TestAChatNamesNoObservationTheCallerCannotOpen(t *testing.T) {
	r := newObservationRig(t)
	dash := seedDashboard(t, r.d, 1)
	about := investigate(t, r.svc, revisions.InvestigateParams{DashboardID: dash, Prompt: "why are checkout errors up"}).ObservationID
	ask := startTurn(t, r.svc, user, revisions.StartRequest{Prompt: text("what changed since"), ObservationID: text(about.String())})
	chat := chatOfTurn(t, r.d, ask)
	router := r.routerAs(user)
	paths := []string{"/api/v1/chats", "/api/v1/chats/" + chat}

	for _, path := range paths {
		if rec := call(router, http.MethodGet, path, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"subject_observation_id":"`+about.String()+`"`) || !strings.Contains(rec.Body.String(), `"observation_id":"`+about.String()+`"`) {
			t.Fatalf("GET %s before the move: %d %s", path, rec.Code, rec.Body)
		}
	}
	hidden := foldertest.Restricted(t, r.d, "On call", "oncall")
	r.d.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, about, hidden)
	for _, path := range paths {
		if rec := call(router, http.MethodGet, path, ""); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), about.String()) {
			t.Errorf("GET %s after the move: %d %s", path, rec.Code, rec.Body)
		}
	}
}
