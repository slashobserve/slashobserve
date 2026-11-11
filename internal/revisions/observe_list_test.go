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
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	accountID  = "22222222-2222-4222-8222-222222222222"
	strangerID = "33333333-3333-4333-8333-333333333333"
)

var (
	account   = auth.Principal{Kind: auth.KindServiceAccount, ID: accountID, Name: "ci", Role: auth.RoleEditor}
	stranger  = auth.Principal{Kind: auth.KindServiceAccount, ID: strangerID, Name: "nobody", Role: auth.RoleEditor}
	lookalike = auth.Principal{Kind: auth.KindServiceAccount, ID: userID, Name: "same id as the user", Role: auth.RoleEditor}
)

func observeRouterFor(svc *revisions.Service, who auth.Principal) *gin.Engine {
	return observeRouterAs(svc, withRole(who))
}

func observeRouterAs(svc *revisions.Service, who auth.Principal) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	revisions.NewObserveHandler(svc, revisions.ObserveOptions{}).Register(auth.NewRoutes(r.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), who))
	})))
	return r
}

type listedPage struct {
	Requests []struct {
		ID             string  `json:"id"`
		Source         string  `json:"source"`
		DashboardID    *string `json:"dashboard_id"`
		DashboardTitle *string `json:"dashboard_title"`
	} `json:"requests"`
	NextCursor *string `json:"next_cursor"`
}

func (p listedPage) ids() []string {
	out := make([]string, len(p.Requests))
	for i, r := range p.Requests {
		out[i] = r.ID
	}
	return out
}

func listObserve(t *testing.T, r http.Handler, query string) listedPage {
	t.Helper()
	rec := call(r, http.MethodGet, "/api/v1/observe"+query, "")
	var page listedPage
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &page) != nil {
		t.Fatalf("GET /observe%s: %d %s", query, rec.Code, rec.Body)
	}
	return page
}

func observeFor(t *testing.T, svc *revisions.Service, who auth.Principal, prompt string) string {
	t.Helper()
	row, err := svc.SubmitObserve(context.Background(), who, prompt)
	if err != nil {
		t.Fatalf("observe %q: %v", prompt, err)
	}
	return row.ID.String()
}

func newest(ids ...string) []string {
	slices.Reverse(ids)
	return ids
}

func TestListObserveShowsOnlyTheCallersRequestsNewestFirst(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)

	first := observeFor(t, svc, user, "first")
	accountOwn := observeFor(t, svc, account, "account first")
	second := observeFor(t, svc, user, "second")
	same := observeFor(t, svc, lookalike, "same id, other kind")
	followUp, err := svc.SubmitFollowUp(context.Background(), dash, nil, "", "follow-up", user)
	if err != nil {
		t.Fatal(err)
	}
	accountSecond := observeFor(t, svc, account, "account second")

	for name, c := range map[string]struct {
		who  auth.Principal
		want []string
	}{
		"user":                 {user, newest(first, second, followUp.ID.String())},
		"service account":      {account, newest(accountOwn, accountSecond)},
		"same id other kind":   {lookalike, []string{same}},
		"account with no rows": {stranger, []string{}},
	} {
		if got := listObserve(t, observeRouterFor(svc, c.who), "").ids(); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}

func TestListObserveWithNoRequestsIsAnEmptyArray(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	rec := call(observeRouterFor(svc, stranger), http.MethodGet, "/api/v1/observe", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"requests":[]`) || !strings.Contains(rec.Body.String(), `"next_cursor":null`) {
		t.Fatalf("empty list: %d %s", rec.Code, rec.Body)
	}
}

func TestListObservePagesWithoutOverlapAndEndsWithANullCursor(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	var created []string
	for _, prompt := range []string{"a", "b", "c"} {
		created = append(created, observeFor(t, svc, user, prompt))
	}
	row, err := svc.Submit(context.Background(), revisions.EnqueueParams{DashboardID: dash, Source: revisions.SourcePrompt, Prompt: "d", BaseVersion: 1, By: user})
	if err != nil {
		t.Fatal(err)
	}
	created = append(created, row.ID.String())
	r := observeRouterFor(svc, user)

	var walked []string
	query := "?limit=1"
	for pages := 0; ; pages++ {
		if pages > len(created) {
			t.Fatalf("paging did not end: %v", walked)
		}
		page := listObserve(t, r, query)
		if len(page.Requests) != 1 {
			t.Fatalf("page %d holds %d requests", pages, len(page.Requests))
		}
		walked = append(walked, page.ids()...)
		if page.NextCursor == nil {
			if pages != len(created)-1 {
				t.Fatalf("the last page came after %d pages, want %d", pages+1, len(created))
			}
			break
		}
		query = "?limit=1&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	if want := newest(created...); !slices.Equal(walked, want) {
		t.Fatalf("walked %v, want %v", walked, want)
	}
	if len(slices.Compact(slices.Sorted(slices.Values(walked)))) != len(walked) {
		t.Fatalf("a request appeared on two pages: %v", walked)
	}
}

func TestListObserveFiltersBySourceAndRefusesBadQueries(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	observe := observeFor(t, svc, user, "new dashboard")
	prompt, err := svc.Submit(context.Background(), revisions.EnqueueParams{DashboardID: dash, Source: revisions.SourcePrompt, Prompt: "follow-up", BaseVersion: 1, By: user})
	if err != nil {
		t.Fatal(err)
	}
	comment, err := svc.Submit(context.Background(), revisions.EnqueueParams{DashboardID: dash, Source: revisions.SourceComment, Prompt: "comment", BaseVersion: 1, By: user})
	if err != nil {
		t.Fatal(err)
	}
	r := observeRouterFor(svc, user)

	for source, want := range map[string]string{"observe": observe, "prompt": prompt.ID.String(), "comment": comment.ID.String()} {
		page := listObserve(t, r, "?source="+source)
		if got := page.ids(); !slices.Equal(got, []string{want}) || page.Requests[0].Source != source {
			t.Errorf("source=%s: got %+v, want %s", source, page.Requests, want)
		}
	}
	if got := listObserve(t, r, "").ids(); len(got) != 3 {
		t.Errorf("no filter: got %v", got)
	}

	for _, query := range []string{"?source=", "?source=bogus", "?cursor=!!!", "?limit=0", "?limit=101"} {
		rec := call(r, http.MethodGet, "/api/v1/observe"+query, "")
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_request" {
			t.Errorf("%s: %d %s", query, rec.Code, rec.Body)
		}
	}
}

func TestListObserveNamesTheDashboardOfASucceededRequest(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 2)
	done := observeFor(t, svc, user, "done")
	failed := observeFor(t, svc, user, "failed")
	d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_version = 2, dashboard_id = $2 WHERE id = $1", mustUUID(t, done), dash)
	d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed', error = 'nope' WHERE id = $1", mustUUID(t, failed))

	for _, row := range listObserve(t, observeRouterFor(svc, user), "").Requests {
		switch row.ID {
		case done:
			if row.DashboardID == nil || *row.DashboardID != dash.String() || row.DashboardTitle == nil || *row.DashboardTitle != "t" {
				t.Errorf("succeeded request = %+v", row)
			}
		case failed:
			if row.DashboardID != nil || row.DashboardTitle != nil {
				t.Errorf("failed request = %+v", row)
			}
		}
	}
}

func TestListObserveHidesTheTitleOfADashboardInAFolderTheCallerCannotSee(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	visible := seedDashboard(t, d, 2)
	hidden := seedDashboard(t, d, 2)
	foldertest.Move(t, d, hidden, foldertest.Restricted(t, d, "Private", "platform"))
	onVisible := observeFor(t, svc, account, "visible")
	onHidden := observeFor(t, svc, account, "hidden")
	d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_version = 2, dashboard_id = $2 WHERE id = $1", mustUUID(t, onVisible), visible)
	d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_version = 2, dashboard_id = $2 WHERE id = $1", mustUUID(t, onHidden), hidden)

	rows := listObserve(t, observeRouterAs(svc, account), "").Requests
	if len(rows) != 2 {
		t.Fatalf("listed %d requests, want 2", len(rows))
	}
	for _, row := range rows {
		switch row.ID {
		case onVisible:
			if row.DashboardID == nil || *row.DashboardID != visible.String() || row.DashboardTitle == nil || *row.DashboardTitle != "t" {
				t.Errorf("request on a visible dashboard = %+v", row)
			}
		case onHidden:
			if row.DashboardID == nil || *row.DashboardID != hidden.String() || row.DashboardTitle != nil {
				t.Errorf("request on a hidden dashboard = %+v", row)
			}
		}
	}
}

func TestSucceededObserveRowIsARevisionWithNoBaseVersion(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 2)
	id := observeFor(t, svc, user, "build it")
	d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_version = 2, dashboard_id = $2 WHERE id = $1", mustUUID(t, id), dash)

	rev := get(t, svc, dash, id)
	if rev.Source != revisions.SourceObserve || rev.BaseVersion != nil || rev.ResultVersion == nil || *rev.ResultVersion != 2 {
		t.Fatalf("Get = %+v", rev)
	}
	items, _, err := svc.List(context.Background(), dash, 10, "")
	if err != nil || len(items) != 1 || items[0].ID != id || items[0].BaseVersion != nil {
		t.Fatalf("List = %+v, %v", items, err)
	}

	r := newRouter(svc)
	for _, path := range []string{revisionsPath(dash) + "/" + id, revisionsPath(dash)} {
		rec := call(r, http.MethodGet, path, "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"base_version":null`) || !strings.Contains(rec.Body.String(), `"source":"observe"`) {
			t.Errorf("GET %s: %d %s", path, rec.Code, rec.Body)
		}
	}
}

type fakePanels struct {
	known   map[string]bool
	err     error
	asked   int
	version int
}

func (f *fakePanels) HasPanel(_ context.Context, _ pgtype.UUID, version int, panelID string) (bool, error) {
	f.asked++
	f.version = version
	return f.known[panelID], f.err
}

func newPanelService(t *testing.T, panels *fakePanels) (*revisions.Service, *dbtest.Database) {
	t.Helper()
	d := dbtest.New(t)
	return newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithPanels(panels)), d
}

func TestPostStoresAKnownPanelAndRefusesAnUnknownOne(t *testing.T) {
	longPanel := strings.Repeat("a", 64)
	panels := &fakePanels{known: map[string]bool{"errors": true, longPanel: true}}
	svc, d := newPanelService(t, panels)
	dash := seedDashboard(t, d, 2)
	r := newRouter(svc)

	for _, panel := range []string{"errors", longPanel} {
		rec := call(r, http.MethodPost, revisionsPath(dash), `{"prompt":"focus","base_version":1,"panel_id":"`+panel+`"}`)
		var body struct {
			Revision struct {
				ID      string  `json:"id"`
				PanelID *string `json:"panel_id"`
			} `json:"revision"`
		}
		if rec.Code != http.StatusAccepted || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Revision.PanelID == nil || *body.Revision.PanelID != panel {
			t.Fatalf("known panel %q: %d %s", panel, rec.Code, rec.Body)
		}
		if stored := get(t, svc, dash, body.Revision.ID); stored.PanelID != panel || stored.Source != revisions.SourcePrompt {
			t.Fatalf("stored = %+v", stored)
		}
	}
	if panels.version != 2 {
		t.Fatalf("the panel was checked against version %d, want the latest (2)", panels.version)
	}

	before := panels.asked
	rec := call(r, http.MethodPost, revisionsPath(dash), `{"prompt":"focus","base_version":1,"panel_id":"gone"}`)
	if rec.Code != http.StatusUnprocessableEntity || errorCode(t, rec) != "unknown_panel" {
		t.Fatalf("unknown panel: %d %s", rec.Code, rec.Body)
	}
	if panels.asked != before+1 {
		t.Fatalf("the panel check ran %d times", panels.asked-before)
	}
	if items, _, err := svc.List(context.Background(), dash, 10, ""); err != nil || len(items) != 2 {
		t.Fatalf("a refused request was stored: %d items, %v", len(items), err)
	}
}

func TestPostRefusesAnInvalidPanelIdBeforeAnyCheck(t *testing.T) {
	panels := &fakePanels{known: map[string]bool{}}
	svc, d := newPanelService(t, panels)
	dash := seedDashboard(t, d, 1)
	r := newRouter(svc)

	for name, panel := range map[string]string{
		"empty":         `""`,
		"65 characters": `"` + strings.Repeat("a", 65) + `"`,
		"NUL":           `"a\u0000b"`,
	} {
		rec := call(r, http.MethodPost, revisionsPath(dash), `{"prompt":"focus","base_version":1,"panel_id":`+panel+`}`)
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_request" {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	if panels.asked != 0 {
		t.Fatalf("an invalid panel id reached the panel check %d times", panels.asked)
	}
}

func TestPostTreatsANullPanelIdAsAbsent(t *testing.T) {
	panels := &fakePanels{known: map[string]bool{}}
	svc, d := newPanelService(t, panels)
	dash := seedDashboard(t, d, 1)
	r := newRouter(svc)

	for _, body := range []string{`{"prompt":"whole","base_version":1,"panel_id":null}`, `{"prompt":"whole","base_version":1}`} {
		rec := call(r, http.MethodPost, revisionsPath(dash), body)
		if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"panel_id":null`) {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if panels.asked != 0 {
		t.Fatalf("an absent panel id reached the panel check %d times", panels.asked)
	}
}

func TestPostAnswers404WhenTheDashboardVanishesDuringThePanelCheck(t *testing.T) {
	panels := &fakePanels{err: revisions.ErrDashboardNotFound}
	svc, d := newPanelService(t, panels)
	dash := seedDashboard(t, d, 1)

	rec := call(newRouter(svc), http.MethodPost, revisionsPath(dash), `{"prompt":"focus","base_version":1,"panel_id":"errors"}`)
	if rec.Code != http.StatusNotFound || errorCode(t, rec) != "not_found" {
		t.Fatalf("post: %d %s", rec.Code, rec.Body)
	}
}

func TestGetObserveHidesARequestWithoutADashboardFromOtherPrincipals(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 2)
	private := observeFor(t, svc, user, "private prompt")
	shared := observeFor(t, svc, user, "shared prompt")
	d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_version = 2, dashboard_id = $2 WHERE id = $1", mustUUID(t, shared), dash)

	for name, c := range map[string]struct {
		id   string
		who  auth.Principal
		want int
	}{
		"requester reads their own":              {private, user, http.StatusOK},
		"another user":                           {private, auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "b"}, http.StatusNotFound},
		"service account":                        {private, account, http.StatusNotFound},
		"same id other kind":                     {private, lookalike, http.StatusNotFound},
		"request with a dashboard, other reader": {shared, stranger, http.StatusOK},
	} {
		rec := call(observeRouterFor(svc, c.who), http.MethodGet, "/api/v1/observe/"+c.id, "")
		if rec.Code != c.want {
			t.Errorf("%s: %d %s, want %d", name, rec.Code, rec.Body, c.want)
		}
		if c.want == http.StatusNotFound && strings.Contains(rec.Body.String(), "private prompt") {
			t.Errorf("%s: the prompt leaked: %s", name, rec.Body)
		}
	}
}

func patchObserve(t *testing.T, r http.Handler, id, body string) {
	t.Helper()
	if rec := call(r, http.MethodPatch, "/api/v1/observe/"+id, body); rec.Code != http.StatusOK {
		t.Fatalf("PATCH /observe/%s %s: %d %s", id, body, rec.Code, rec.Body)
	}
}

func walkObserve(t *testing.T, r http.Handler, query string) []string {
	t.Helper()
	var walked []string
	cursor := ""
	for pages := 0; pages < 20; pages++ {
		page := listObserve(t, r, "?limit=1"+query+cursor)
		walked = append(walked, page.ids()...)
		if page.NextCursor == nil {
			return walked
		}
		cursor = "&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	t.Fatal("the list did not end")
	return nil
}

func TestListObservePutsPinnedRequestsFirstAndPagesAcrossThem(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	oldest := observeFor(t, svc, user, "oldest")
	middle := observeFor(t, svc, user, "middle")
	newestID := observeFor(t, svc, user, "newest")
	r := observeRouterFor(svc, user)
	patchObserve(t, r, oldest, `{"pinned":true}`)

	if got, want := walkObserve(t, r, ""), []string{oldest, newestID, middle}; !slices.Equal(got, want) {
		t.Fatalf("walked %v, want %v", got, want)
	}
	if got := listObserve(t, r, "?pinned=true").ids(); !slices.Equal(got, []string{oldest}) {
		t.Errorf("pinned=true: %v", got)
	}
	if got := listObserve(t, r, "?pinned=false").ids(); !slices.Equal(got, []string{newestID, middle}) {
		t.Errorf("pinned=false: %v", got)
	}
	patchObserve(t, r, oldest, `{"pinned":false}`)
	if got := listObserve(t, r, "").ids(); !slices.Equal(got, []string{newestID, middle, oldest}) {
		t.Errorf("after unpinning: %v", got)
	}
}

func TestListObserveStillTakesAnUnprefixedCursor(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	older := observeFor(t, svc, user, "older")
	observeFor(t, svc, user, "newer")
	r := observeRouterFor(svc, user)
	first := listObserve(t, r, "?limit=1")
	if first.NextCursor == nil || !strings.HasPrefix(*first.NextCursor, "u.") {
		t.Fatalf("first page cursor = %v", first.NextCursor)
	}
	old := strings.TrimPrefix(*first.NextCursor, "u.")
	if got := listObserve(t, r, "?limit=1&cursor="+url.QueryEscape(old)).ids(); !slices.Equal(got, []string{older}) {
		t.Fatalf("with an unprefixed cursor: %v", got)
	}
}

func TestListObserveSearchesTitlesAndPromptsLiterally(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	titled := observeFor(t, svc, user, "something else")
	percent := observeFor(t, svc, user, "orders 50% done")
	observeFor(t, svc, user, "500 errors")
	followUp, err := svc.SubmitFollowUp(context.Background(), dash, nil, "", "on a dashboard", user)
	if err != nil {
		t.Fatal(err)
	}
	r := observeRouterFor(svc, user)
	patchObserve(t, r, titled, `{"title":"Checkout latency"}`)
	cases := []struct {
		query string
		want  []string
	}{
		{"?q=CHECKOUT", []string{titled}},
		{"?q=" + url.QueryEscape("50%"), []string{percent}},
		{"?q=" + url.QueryEscape("rd_r"), []string{}},
		{"?dashboard_id=" + dash.String(), []string{followUp.ID.String()}},
	}
	for _, c := range cases {
		if got := listObserve(t, r, c.query).ids(); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.query, got, c.want)
		}
	}
	for _, bad := range []string{"?q=", "?q=" + strings.Repeat("a", 201), "?pinned=yes", "?dashboard_id=nope", "?source=suggest"} {
		if rec := call(r, http.MethodGet, "/api/v1/observe"+bad, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", bad, rec.Code, rec.Body)
		}
	}
}

func TestPatchObserveSetsAndClearsTheTitle(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	id := observeFor(t, svc, user, "checkout health by pod\nlast 6h")
	r := observeRouterFor(svc, user)
	title := func() (string, *string) {
		rec := call(r, http.MethodGet, "/api/v1/observe/"+id, "")
		var body struct {
			Request struct {
				Title    string  `json:"title"`
				Kind     string  `json:"kind"`
				PinnedAt *string `json:"pinned_at"`
			} `json:"request"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Request.Kind != "build" {
			t.Fatalf("GET: %d %s", rec.Code, rec.Body)
		}
		return body.Request.Title, body.Request.PinnedAt
	}
	if got, _ := title(); got != "checkout health by pod" {
		t.Errorf("default title = %q", got)
	}
	patchObserve(t, r, id, `{"title":"  Pods  ","pinned":true}`)
	if got, pinned := title(); got != "Pods" || pinned == nil {
		t.Errorf("after setting: %q pinned %v", got, pinned)
	}
	patchObserve(t, r, id, `{"title":null}`)
	if got, pinned := title(); got != "checkout health by pod" || pinned == nil {
		t.Errorf("after clearing: %q pinned %v", got, pinned)
	}
	for _, bad := range []string{`{}`, `{"title":"two\nlines"}`, `{"title":7}`, `{"title":"` + strings.Repeat("a", 121) + `"}`, `{"pinned":"yes"}`, `{"other":1}`} {
		if rec := call(r, http.MethodPatch, "/api/v1/observe/"+id, bad); rec.Code != http.StatusBadRequest {
			t.Errorf("PATCH %s: %d %s", bad, rec.Code, rec.Body)
		}
	}
}

func TestOnlyTheRequesterChangesOrArchivesARequest(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	row, err := svc.SubmitFollowUp(context.Background(), dash, nil, "", "on a dashboard", user)
	if err != nil {
		t.Fatal(err)
	}
	id := row.ID.String()
	other := observeRouterFor(svc, account)
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		rec := call(other, method, "/api/v1/observe/"+id, `{"pinned":true}`)
		if rec.Code != http.StatusForbidden || errorCode(t, rec) != "forbidden" {
			t.Errorf("%s by another principal: %d %s", method, rec.Code, rec.Body)
		}
	}
	private := observeFor(t, svc, user, "no dashboard")
	if rec := call(other, http.MethodDelete, "/api/v1/observe/"+private, ""); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE of a request the caller cannot see: %d %s", rec.Code, rec.Body)
	}
}

func TestArchivedRequestsLeaveTheListButStayReadable(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	kept := observeFor(t, svc, user, "kept")
	archived := observeFor(t, svc, user, "archived")
	r := observeRouterFor(svc, user)
	for range 2 {
		if rec := call(r, http.MethodDelete, "/api/v1/observe/"+archived, ""); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("DELETE: %d %s", rec.Code, rec.Body)
		}
	}
	if got := listObserve(t, r, "").ids(); !slices.Equal(got, []string{kept}) {
		t.Errorf("list = %v", got)
	}
	rec := call(r, http.MethodGet, "/api/v1/observe/"+archived, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"archived_at":"`) || !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Errorf("GET of an archived request: %d %s", rec.Code, rec.Body)
	}
}

func TestCreateObserveChecksTheInvestigateFields(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	r := observeRouterFor(svc, user)
	dashboard := `"dashboard_id":"` + dash.String() + `"`
	cases := []struct {
		name, body, message string
	}{
		{"panel without investigate", `{"prompt":"p","panel_id":"a"}`, "panel_id, from and to need kind investigate"},
		{"window without investigate", `{"prompt":"p","kind":"build","from":"now-1h","to":"now"}`, "panel_id, from and to need kind investigate"},
		{"unknown kind", `{"prompt":"p","kind":"explain"}`, "kind must be build or investigate"},
		{"base version", `{"prompt":"p","kind":"investigate",` + dashboard + `,"base_version":1}`, "base_version is not allowed for kind investigate"},
		{"panel without dashboard", `{"prompt":"p","kind":"investigate","panel_id":"a"}`, "panel_id needs dashboard_id"},
		{"empty panel", `{"prompt":"p","kind":"investigate",` + dashboard + `,"panel_id":""}`, "panel_id must be 1 to 64 characters without NUL"},
		{"half a window", `{"prompt":"p","kind":"investigate","from":"now-1h"}`, "from and to go together"},
		{"not a time", `{"prompt":"p","kind":"investigate","from":"soon","to":"now"}`, "from and to must be relative times like now-1h or epoch milliseconds"},
		{"reversed window", `{"prompt":"p","kind":"investigate","from":"now","to":"now-1h"}`, "from must be before to"},
		{"long window", `{"prompt":"p","kind":"investigate","from":"now-500d","to":"now"}`, "the time range is too long"},
	}
	for _, c := range cases {
		rec := call(r, http.MethodPost, "/api/v1/observe", c.body)
		var body struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		if rec.Code != http.StatusBadRequest || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Error != c.message {
			t.Errorf("%s: %d %s, want 400 %q", c.name, rec.Code, rec.Body, c.message)
		}
	}
	rec := call(r, http.MethodPost, "/api/v1/observe", `{"prompt":"why","kind":"investigate","from":"now-2h","to":"now"}`)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"kind":"investigate"`) || !strings.Contains(rec.Body.String(), `"finding":null`) {
		t.Fatalf("an investigation: %d %s", rec.Code, rec.Body)
	}
	viewer := observeRouterAs(svc, auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Vic", Role: auth.RoleViewer})
	if rec := call(viewer, http.MethodPost, "/api/v1/observe", `{"prompt":"why","kind":"investigate"}`); rec.Code != http.StatusForbidden {
		t.Errorf("a viewer: %d %s", rec.Code, rec.Body)
	}
}
