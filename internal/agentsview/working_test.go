package agentsview

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/slashobserve/slashobserve/api/apitest"
	"github.com/slashobserve/slashobserve/internal/auth"
)

type workingItem struct {
	ID    string  `json:"id"`
	State string  `json:"state"`
	Mine  bool    `json:"mine"`
	Title *string `json:"title"`
	Agent struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"agent"`
	RequestedBy struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"requested_by"`
	Dashboard *struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"dashboard"`
	Observation *struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"observation"`
}

type workingPage struct {
	Items     []workingItem `json:"items"`
	Truncated bool          `json:"truncated"`
}

type activeRun struct {
	by            auth.Principal
	state         string
	source        string
	title         string
	prompt        string
	kind          string
	assignedID    string
	dashboardID   string
	observationID string
	minutesAgo    int
}

func (r *rig) activeRun(t *testing.T, in activeRun) string {
	t.Helper()
	state := in.state
	if state == "" {
		state = "queued"
	}
	source := in.source
	if source == "" && in.dashboardID != "" {
		source = "prompt"
	}
	if source == "" {
		source = "observe"
	}
	kind := in.kind
	if kind == "" {
		kind = "builtin"
	}
	prompt := in.prompt
	if prompt == "" {
		prompt = "p"
	}
	status, failure := state, ""
	switch state {
	case "canceled":
		status, failure = "failed", "canceled"
	case "failed":
		failure = "internal"
	}
	var id string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO agent_requests (source, title, prompt, requested_by_kind, requested_by_id, requested_by_name, status, state, failure_code, assigned_kind, assigned_id, dashboard_id, observation_id, base_version, created_at)
		VALUES ($1, nullif($2, ''), $3, 'user', $4::uuid, $5, $12, $6, nullif($13, ''), $7, nullif($8, '')::uuid, nullif($9, '')::uuid, nullif($10, '')::uuid, CASE WHEN $9 <> '' AND $1 NOT IN ('observe', 'investigate') THEN 1 END, now() - make_interval(mins => $11))
		RETURNING id::text`,
		source, in.title, prompt, in.by.ID, in.by.Name, state, kind, in.assignedID, in.dashboardID, in.observationID, in.minutesAgo, status, failure).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) dashboardBy(t *testing.T, title string, by auth.Principal, folderID string) string {
	t.Helper()
	var id string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, folder_id, created_by_kind, created_by_id, created_by_name, created_via)
		VALUES ($1, 1, coalesce(nullif($2, '')::uuid, general_folder_id()), 'user', $3::uuid, $4, 'direct') RETURNING id::text`,
		title, folderID, by.ID, by.Name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) observationBy(t *testing.T, title string, by auth.Principal) string {
	t.Helper()
	var id string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name)
		VALUES (general_folder_id(), $1, 'user', $2::uuid, $3) RETURNING id::text`, title, by.ID, by.Name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) restrictedFolder(t *testing.T) string {
	t.Helper()
	var id string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title) VALUES ('Secret') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `INSERT INTO folder_permissions (folder_id, group_name, level) VALUES ($1::uuid, 'secret-team', 'view')`, id)
	return id
}

func (r *rig) getWorking(by auth.Principal, query url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/working?"+query.Encode(), nil)
	req.Header.Set("X-Test-Principal", by.ID)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

func (r *rig) working(t *testing.T, by auth.Principal, query url.Values) workingPage {
	t.Helper()
	rec := r.getWorking(by, query)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /agents/working as %s = %d: %s", by.Name, rec.Code, rec.Body.String())
	}
	var out workingPage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func workingIDs(page workingPage) []string {
	out := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, item.ID)
	}
	return out
}

func TestWorkingListsTheCallersOwnRunWithItsTitle(t *testing.T) {
	r := newRig(t, Observe{})
	mine := r.activeRun(t, activeRun{by: ada, title: "Checkout latency", state: "running", minutesAgo: 1})
	r.activeRun(t, activeRun{by: bob, minutesAgo: 2})

	page := r.working(t, ada, url.Values{})
	if !slices.Equal(workingIDs(page), []string{mine}) || page.Truncated {
		t.Fatalf("Ada's page = %+v, want only her own run", page)
	}
	item := page.Items[0]
	if !item.Mine || item.Title == nil || *item.Title != "Checkout latency" || item.State != "running" {
		t.Fatalf("item = %+v, want her running run with its title", item)
	}
	if item.Agent.Kind != "builtin" || item.Agent.Name != "Observe Agent" || item.RequestedBy.Name != "Ada" {
		t.Fatalf("agent %+v requested_by %+v, want the Observe Agent for Ada", item.Agent, item.RequestedBy)
	}
}

func TestAnotherPersonsRunOnTheCallersThingsHidesItsTitle(t *testing.T) {
	r := newRig(t, Observe{})
	dashboard := r.dashboardBy(t, "Checkout", ada, "")
	observation := r.observationBy(t, "Slow checkout", ada)
	grant := r.grant(t, bob, "claude-code")
	onDashboard := r.activeRun(t, activeRun{by: bob, title: "Private words", kind: "grant", assignedID: grant, dashboardID: dashboard, minutesAgo: 2})
	onObservation := r.activeRun(t, activeRun{by: bob, source: "investigate", prompt: "Secret prompt", observationID: observation, minutesAgo: 1})

	page := r.working(t, ada, url.Values{})
	if !slices.Equal(workingIDs(page), []string{onObservation, onDashboard}) {
		t.Fatalf("ids = %v, want both of Bob's runs on Ada's things", workingIDs(page))
	}
	first, second := page.Items[0], page.Items[1]
	if first.Mine || first.Title != nil || first.Observation == nil || first.Observation.ID != observation || first.Observation.Title != "Slow checkout" || first.Agent.Name != "Observe Agent" {
		t.Fatalf("observation run = %+v, want not mine, no title, Ada's observation and the Observe Agent", first)
	}
	if second.Mine || second.Title != nil || second.Dashboard == nil || second.Dashboard.ID != dashboard || second.Agent.Kind != "grant" || second.Agent.Name != "claude-code" || second.RequestedBy.Name != "Bob" {
		t.Fatalf("dashboard run = %+v, want not mine, no title, Ada's dashboard and Bob's claude-code", second)
	}
	body := r.getWorking(ada, url.Values{}).Body.String()
	for _, secret := range []string{"Private words", "Secret prompt"} {
		if strings.Contains(body, secret) {
			t.Fatalf("the response carries %q from another person's request", secret)
		}
	}
	if got := r.working(t, bob, url.Values{}); !slices.Equal(workingIDs(got), []string{onObservation, onDashboard}) || !got.Items[0].Mine || got.Items[0].Title == nil || *got.Items[0].Title != "Secret prompt" {
		t.Fatalf("Bob's page = %+v, want his own two runs with titles", got)
	}
}

func TestARunOnAThingTheCallerCanNoLongerSeeIsNotListed(t *testing.T) {
	r := newRig(t, Observe{})
	secret := r.restrictedFolder(t)
	hidden := r.dashboardBy(t, "Payroll", ada, secret)
	open := r.dashboardBy(t, "Checkout", ada, "")
	r.activeRun(t, activeRun{by: bob, dashboardID: hidden, minutesAgo: 2})
	visible := r.activeRun(t, activeRun{by: bob, dashboardID: open, minutesAgo: 1})

	page := r.working(t, ada, url.Values{})
	if !slices.Equal(workingIDs(page), []string{visible}) {
		t.Fatalf("ids = %v, want only the run on the dashboard Ada can still view", workingIDs(page))
	}
	if strings.Contains(r.getWorking(ada, url.Values{}).Body.String(), hidden) {
		t.Fatal("the response names the hidden dashboard")
	}
}

func TestFinishedRunsAndSuggestionsAreNotListed(t *testing.T) {
	r := newRig(t, Observe{})
	dashboard := r.dashboardBy(t, "Checkout", ada, "")
	for _, state := range []string{"succeeded", "failed", "canceled"} {
		r.activeRun(t, activeRun{by: ada, state: state, source: "investigate"})
	}
	r.db.Exec(t, `INSERT INTO agent_requests (source, prompt, requested_by_kind, requested_by_id, requested_by_name, status, state, assigned_kind, dashboard_id, base_version, panel_id)
		VALUES ('suggest', 'p', 'user', $1::uuid, 'Ada', 'queued', 'queued', 'builtin', $2::uuid, 1, 'p1')`, ada.ID, dashboard)
	kept := r.activeRun(t, activeRun{by: ada, state: "running"})

	if got := r.working(t, ada, url.Values{}); !slices.Equal(workingIDs(got), []string{kept}) {
		t.Fatalf("ids = %v, want only the running run %s", workingIDs(got), kept)
	}
}

func TestWorkingSaysTruncatedWhenMoreRunsExistThanTheLimit(t *testing.T) {
	r := newRig(t, Observe{})
	newest := r.activeRun(t, activeRun{by: ada, minutesAgo: 1})
	r.activeRun(t, activeRun{by: ada, minutesAgo: 2})

	page := r.working(t, ada, url.Values{"limit": {"1"}})
	if !slices.Equal(workingIDs(page), []string{newest}) || !page.Truncated {
		t.Fatalf("page = %+v, want the newest run and truncated", page)
	}
	if rec := r.getWorking(ada, url.Values{"limit": {"51"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("limit 51 got %d %s, want 400", rec.Code, rec.Body.String())
	}
}

func TestKeysCannotListWorkingRuns(t *testing.T) {
	r := newRig(t, Observe{})
	rec := r.getWorking(bot, url.Values{})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"user_session_required"`) {
		t.Fatalf("a key got %d %s, want 403 user_session_required", rec.Code, rec.Body.String())
	}
}

func TestTheWorkingResponseMatchesTheDocument(t *testing.T) {
	r := newRig(t, Observe{})
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc := apitest.Load(t)
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	dashboard := r.dashboardBy(t, "Checkout", ada, "")
	observation := r.observationBy(t, "Slow checkout", ada)
	r.activeRun(t, activeRun{by: ada, title: "Mine", state: "running", minutesAgo: 3})
	r.activeRun(t, activeRun{by: bob, dashboardID: dashboard, minutesAgo: 2})
	r.activeRun(t, activeRun{by: bob, source: "investigate", observationID: observation, minutesAgo: 1})

	for _, tc := range []struct {
		by    auth.Principal
		query url.Values
	}{{ada, url.Values{}}, {bob, url.Values{"limit": {"1"}}}, {root, url.Values{}}, {ada, url.Values{"limit": {"0"}}}, {bot, url.Values{}}} {
		rec := r.getWorking(tc.by, tc.query)
		check := httptest.NewRequest(http.MethodGet, "/api/v1/agents/working?"+tc.query.Encode(), nil)
		route, params, err := router.FindRoute(check)
		if err != nil {
			t.Fatalf("GET /agents/working is not documented: %v", err)
		}
		options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true, SkipSettingDefaults: true}
		input := &openapi3filter.RequestValidationInput{Request: check, PathParams: params, Route: route, Options: options}
		response := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: rec.Code, Header: rec.Header(), Options: options}
		response.SetBodyBytes(rec.Body.Bytes())
		if err := openapi3filter.ValidateResponse(t.Context(), response); err != nil {
			t.Fatalf("%s %s answered %d against the document: %v\n%s", tc.by.Name, tc.query.Encode(), rec.Code, err, rec.Body)
		}
	}
}
