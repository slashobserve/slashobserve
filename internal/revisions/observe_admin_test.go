package revisions_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	adminID  = "44444444-4444-4444-8444-444444444444"
	viewerID = "55555555-5555-4555-8555-555555555555"
)

var (
	bea = auth.Principal{Kind: auth.KindUser, ID: adminID, Name: "Bea", Role: auth.RoleAdmin}
	vic = auth.Principal{Kind: auth.KindUser, ID: viewerID, Name: "Vic", Role: auth.RoleViewer}
)

type adminListed struct {
	Requests []struct {
		ID             string   `json:"id"`
		DashboardTitle *string  `json:"dashboard_title"`
		URL            *string  `json:"url"`
		AllowedActions []string `json:"allowed_actions"`
		State          string   `json:"state"`
		Build          *struct {
			Title *string `json:"title"`
		} `json:"build"`
		RequestedBy struct {
			Kind  string `json:"kind"`
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"requested_by"`
	} `json:"requests"`
	NextCursor *string `json:"next_cursor"`
}

func (p adminListed) ids() []string {
	out := make([]string, len(p.Requests))
	for i, r := range p.Requests {
		out[i] = r.ID
	}
	return out
}

func linkingRouterAs(svc *revisions.Service, who auth.Principal) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	opts := revisions.ObserveOptions{ObservationURL: func(id string) string { return "/o/" + id }}
	revisions.NewObserveHandler(svc, opts).Register(auth.NewRoutes(r.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), who))
	})))
	return r
}

func listAdmin(t *testing.T, who auth.Principal, svc *revisions.Service, query string) adminListed {
	t.Helper()
	rec := call(linkingRouterAs(svc, who), http.MethodGet, "/api/v1/observe"+query, "")
	var page adminListed
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &page) != nil {
		t.Fatalf("GET /observe%s: %d %s", query, rec.Code, rec.Body)
	}
	return page
}

func walkAdmin(t *testing.T, who auth.Principal, svc *revisions.Service, query string) []string {
	t.Helper()
	var walked []string
	cursor := ""
	for range 20 {
		page := listAdmin(t, who, svc, "?limit=1"+query+cursor)
		walked = append(walked, page.ids()...)
		if page.NextCursor == nil {
			return walked
		}
		cursor = "&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	t.Fatal("the list did not end")
	return nil
}

func setState(t *testing.T, d *dbtest.Database, id, state string) {
	t.Helper()
	switch state {
	case "failed":
		d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed', attempts = 1, started_at = now(), finished_at = now(), error = 'stopped', failure_code = 'no_progress' WHERE id = $1", id)
	case "canceled":
		d.Exec(t, "UPDATE agent_requests SET state = 'canceled', status = 'failed', attempts = 1, started_at = now(), finished_at = now(), error = 'canceled', failure_code = 'canceled' WHERE id = $1", id)
	default:
		t.Fatalf("unsupported state %s", state)
	}
}

func personalDashboardOf(t *testing.T, d *dbtest.Database, owner auth.Principal) pgtype.UUID {
	t.Helper()
	d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $2 || '@example.com', $2)`, owner.ID, owner.Name)
	dash := seedDashboard(t, d, 1)
	d.Exec(t, `UPDATE dashboards SET folder_id = (SELECT id FROM folders WHERE owner_id = $2::uuid) WHERE id = $1`, dash, owner.ID)
	return dash
}

func TestListObserveAllIsForAdminsOnly(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	observeFor(t, svc, user, "mine")

	for name, c := range map[string]struct {
		query string
		who   auth.Principal
		want  int
	}{
		"an Editor":                     {"?scope=all", asEditor(user), http.StatusForbidden},
		"a Viewer":                      {"?scope=all", vic, http.StatusForbidden},
		"an Editor service account":     {"?scope=all", account, http.StatusForbidden},
		"an Admin":                      {"?scope=all", bea, http.StatusOK},
		"an Admin service account":      {"?scope=all", auth.Principal{Kind: auth.KindServiceAccount, ID: accountID, Name: "ci", Role: auth.RoleAdmin}, http.StatusOK},
		"a Viewer asking for their own": {"?scope=mine", vic, http.StatusOK},
		"a Viewer with no scope":        {"", vic, http.StatusOK},
		"an unknown scope":              {"?scope=bogus", bea, http.StatusBadRequest},
		"an unknown state":              {"?state=bogus", bea, http.StatusBadRequest},
		"one unknown state among two":   {"?state=failed&state=bogus", bea, http.StatusBadRequest},
	} {
		rec := call(observeRouterAs(svc, c.who), http.MethodGet, "/api/v1/observe"+c.query, "")
		if rec.Code != c.want {
			t.Errorf("%s: %d %s, want %d", name, rec.Code, rec.Body, c.want)
		}
		if c.want == http.StatusForbidden && errorCode(t, rec) != forbidden.Code {
			t.Errorf("%s: code %q, want %q", name, errorCode(t, rec), forbidden.Code)
		}
	}
}

func TestListObserveAllListsEveryonesChatsNewestFirst(t *testing.T) {
	svc, d := newSuggestService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	adaRouter := observeRouterAs(svc, asEditor(user))

	first := observeFor(t, svc, user, "ada first")
	byBea := observeFor(t, svc, bea, "bea only")
	second := observeFor(t, svc, user, "ada second")
	suggestFor(t, svc, dash, user)
	archived := observeFor(t, svc, user, "ada archived")
	if rec := call(adaRouter, http.MethodDelete, "/api/v1/observe/"+archived, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body)
	}
	patchObserve(t, adaRouter, first, `{"pinned":true}`)

	want := newest(first, byBea, second)
	page := listAdmin(t, bea, svc, "?scope=all")
	if got := page.ids(); !slices.Equal(got, want) {
		t.Fatalf("scope=all = %v, want %v", got, want)
	}
	requesters := map[string]string{first: userID, byBea: adminID, second: userID}
	names := map[string]string{userID: "Ada", adminID: "Bea"}
	for _, r := range page.Requests {
		if r.RequestedBy.ID != requesters[r.ID] || r.RequestedBy.Name != names[r.RequestedBy.ID] || r.RequestedBy.Kind != "user" || r.RequestedBy.Email != "" {
			t.Errorf("%s: requested_by = %+v", r.ID, r.RequestedBy)
		}
	}
	if got := walkAdmin(t, bea, svc, "&scope=all"); !slices.Equal(got, want) {
		t.Errorf("walking scope=all one by one = %v, want %v", got, want)
	}
	if got := listAdmin(t, bea, svc, "").ids(); !slices.Equal(got, []string{byBea}) {
		t.Errorf("the Admin's own list = %v, want only %s", got, byBea)
	}
	if got := listAdmin(t, asEditor(user), svc, "").ids(); !slices.Equal(got, []string{first, second}) {
		t.Errorf("an Editor's own list keeps the pin first: %v, want %v", got, []string{first, second})
	}
}

func TestListObserveFiltersByState(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	failed := observeFor(t, svc, user, "ada failed")
	canceled := observeFor(t, svc, user, "ada canceled")
	queued := observeFor(t, svc, user, "ada queued")
	beaFailed := observeFor(t, svc, bea, "bea failed")
	beaQueued := observeFor(t, svc, bea, "bea queued")
	setState(t, d, failed, "failed")
	setState(t, d, canceled, "canceled")
	setState(t, d, beaFailed, "failed")

	cases := []struct {
		name  string
		who   auth.Principal
		query string
		want  []string
	}{
		{"two states in the all scope", bea, "?scope=all&state=failed&state=canceled", newest(failed, canceled, beaFailed)},
		{"one state in the all scope", bea, "?scope=all&state=queued", newest(queued, beaQueued)},
		{"two states in the own scope", asEditor(user), "?state=failed&state=canceled", newest(failed, canceled)},
		{"one state in the own scope", bea, "?state=failed", []string{beaFailed}},
		{"no state keeps them all", asEditor(user), "", newest(failed, canceled, queued)},
	}
	for _, c := range cases {
		if got := listAdmin(t, c.who, svc, c.query).ids(); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestListObserveAllHidesWhatTheAdminCannotSee(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := personalDashboardOf(t, d, user)
	onPrivate, err := svc.Submit(context.Background(), revisions.EnqueueParams{DashboardID: dash, Source: revisions.SourcePrompt, Prompt: "private revision", BaseVersion: 1, By: user})
	if err != nil {
		t.Fatal(err)
	}
	insertFindingFor(t, d, onPrivate.ID)
	d.Exec(t, "UPDATE observations SET folder_id = (SELECT id FROM folders WHERE owner_id = $1::uuid) WHERE id = (SELECT observation_id FROM agent_requests WHERE id = $2)", userID, onPrivate.ID)
	d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', attempts = 1, started_at = now(), finished_at = now() WHERE id = $1", onPrivate.ID)
	d.Exec(t, `INSERT INTO agent_request_builds (agent_request_id, dashboard_id, base_version, spec, planned_panels, panels, saves, summary) VALUES ($1, $2, 1, '{}', 2, 1, 1, 'draft')`, onPrivate.ID, dash)
	own := observeFor(t, svc, bea, "bea own")

	page := listAdmin(t, bea, svc, "?scope=all")
	if got := page.ids(); !slices.Equal(got, newest(onPrivate.ID.String(), own)) {
		t.Fatalf("scope=all = %v", got)
	}
	private, mine := page.Requests[1], page.Requests[0]
	if private.DashboardTitle != nil || private.URL != nil || len(private.AllowedActions) != 0 || private.AllowedActions == nil {
		t.Errorf("another person's chat on a private dashboard: title %v, url %v, actions %v", private.DashboardTitle, private.URL, private.AllowedActions)
	}
	if private.Build != nil && private.Build.Title != nil {
		t.Errorf("the build title of another person's chat is shown: %v", *private.Build.Title)
	}
	if !slices.Contains(mine.AllowedActions, "cancel") {
		t.Errorf("the Admin's own chat lost its actions: %v", mine.AllowedActions)
	}

	ownerPage := listAdmin(t, asEditor(user), svc, "")
	if len(ownerPage.Requests) != 1 || ownerPage.Requests[0].DashboardTitle == nil || ownerPage.Requests[0].URL == nil || !strings.Contains(*ownerPage.Requests[0].URL, "/o/") {
		t.Errorf("the requester's own list = %+v", ownerPage.Requests)
	}
}

type adminRead struct {
	Request struct {
		ReadOnly       bool     `json:"read_only"`
		CanCancel      bool     `json:"can_cancel"`
		AllowedActions []string `json:"allowed_actions"`
		Prompt         string   `json:"prompt"`
	} `json:"request"`
}

func TestAdminReadsButCannotActOnSomeoneElsesChat(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithBuilds(&recordingBuilds{}))
	noDashboard := observeFor(t, svc, user, "chat without a dashboard")
	private, err := svc.Submit(context.Background(), revisions.EnqueueParams{DashboardID: personalDashboardOf(t, d, user), Source: revisions.SourcePrompt, Prompt: "chat on a private dashboard", BaseVersion: 1, By: user})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{noDashboard, private.ID.String()} {
		setState(t, d, id, "failed")
	}
	adminRouter := observeRouterAs(svc, bea)

	for name, id := range map[string]string{"without a dashboard": noDashboard, "on a private dashboard": private.ID.String()} {
		rec := call(adminRouter, http.MethodGet, "/api/v1/observe/"+id, "")
		var got adminRead
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
			t.Fatalf("%s: GET by an Admin: %d %s", name, rec.Code, rec.Body)
		}
		if !got.Request.ReadOnly || got.Request.CanCancel || got.Request.AllowedActions == nil || len(got.Request.AllowedActions) != 0 {
			t.Errorf("%s: read_only %v, can_cancel %v, allowed_actions %v", name, got.Request.ReadOnly, got.Request.CanCancel, got.Request.AllowedActions)
		}
		for _, w := range []struct{ method, path, body string }{
			{http.MethodPost, "/cancel", ""},
			{http.MethodPost, "/continue", ""},
			{http.MethodPost, "/retry", ""},
			{http.MethodPost, "/build/save", ""},
			{http.MethodPatch, "", `{"pinned":true}`},
			{http.MethodDelete, "", ""},
		} {
			if rec := call(adminRouter, w.method, "/api/v1/observe/"+id+w.path, w.body); rec.Code != http.StatusNotFound {
				t.Errorf("%s: %s %s by an Admin answered %d %s, want 404", name, w.method, w.path, rec.Code, rec.Body)
			}
		}
	}

	var own adminRead
	rec := call(observeRouterAs(svc, asEditor(user)), http.MethodGet, "/api/v1/observe/"+noDashboard, "")
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &own) != nil || own.Request.ReadOnly || !slices.Contains(own.Request.AllowedActions, "retry") {
		t.Errorf("the requester's GET: %d %s", rec.Code, rec.Body)
	}
	if rec := call(observeRouterAs(svc, asEditor(user)), http.MethodGet, "/api/v1/observe/"+private.ID.String(), ""); rec.Code != http.StatusOK {
		t.Errorf("the requester's GET on their private dashboard: %d %s", rec.Code, rec.Body)
	}
	for _, other := range []auth.Principal{asEditor(vic), account} {
		if rec := call(observeRouterAs(svc, other), http.MethodGet, "/api/v1/observe/"+noDashboard, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s reading a chat without a dashboard: %d, want 404", other.Name, rec.Code)
		}
	}
}

type recordingBuilds struct {
	viewedBy []string
	saved    []string
}

func (b *recordingBuilds) ViewBuild(_ context.Context, by auth.Principal, requestID pgtype.UUID) (revisions.BuildView, error) {
	b.viewedBy = append(b.viewedBy, by.ID)
	return revisions.BuildView{RequestID: requestID.String(), Spec: json.RawMessage(`{}`), UpdatedAt: "2026-11-14T10:00:00Z"}, nil
}

func (b *recordingBuilds) SaveStoppedBuild(_ context.Context, by auth.Principal, requestID pgtype.UUID) (revisions.SavedBuild, error) {
	b.saved = append(b.saved, by.ID)
	return revisions.SavedBuild{DashboardID: requestID.String(), Version: 1}, nil
}

func TestAdminReadsAnotherPersonsBuildButCannotSaveIt(t *testing.T) {
	builds := &recordingBuilds{}
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithBuilds(builds))
	id := observeFor(t, svc, user, "chat with a stopped build")
	setState(t, d, id, "failed")
	build := "/api/v1/observe/" + id + "/build"

	if rec := call(observeRouterAs(svc, bea), http.MethodGet, build, ""); rec.Code != http.StatusOK {
		t.Fatalf("GET build by an Admin: %d %s", rec.Code, rec.Body)
	}
	if !slices.Equal(builds.viewedBy, []string{adminID}) {
		t.Errorf("the build was read for %v, want the Admin", builds.viewedBy)
	}
	if rec := call(observeRouterAs(svc, bea), http.MethodPost, build+"/save", ""); rec.Code != http.StatusNotFound {
		t.Errorf("POST build/save by an Admin: %d %s, want 404", rec.Code, rec.Body)
	}
	if rec := call(observeRouterAs(svc, asEditor(vic)), http.MethodGet, build, ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET build by another Editor: %d %s, want 404", rec.Code, rec.Body)
	}
	if len(builds.saved) != 0 || len(builds.viewedBy) != 1 {
		t.Errorf("after the refused calls: saved by %v, read for %v", builds.saved, builds.viewedBy)
	}
	if rec := call(observeRouterAs(svc, asEditor(user)), http.MethodPost, build+"/save", ""); rec.Code != http.StatusCreated {
		t.Errorf("POST build/save by the requester: %d %s, want 201", rec.Code, rec.Body)
	}
	if !slices.Equal(builds.saved, []string{userID}) {
		t.Errorf("the build was saved by %v, want the requester", builds.saved)
	}
}
